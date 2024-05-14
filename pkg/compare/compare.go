package compare

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/gosimple/slug"
	"github.com/openshift/oc/pkg/compare/groups"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/diff"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/utils/exec"
)

var (
	compareLong = templates.LongDesc(`
		Compare a known valid reference configuration and a set of specific cluster configuration CRs.
		
		The reference configuration consists of Resource templates. 
		Resource Templates are files that contain Resource definitions and with fixed and optional content. Optional content is represented as Go templates.
		The compare command will match each Resource in the cluster configuration to a Resource Template in the reference 
		configuration. Then, the templated Resource will be injected with the cluster Resource parameters. 
		For each cluster Resource, a diff between the Resource and its matching injected template will be presented
    	to the user.
		
		The input cluster configuration may be provided as an "offline" set of CRs or can be pulled from a live cluster.
		
		The Reference also includes a mandatory metadata.yaml file where all the Resource templates should be specified.
		The Resource templates can be divided into components. Each component and Resource template can be set as required,
		resulting in a report to the user in case one of them is missing.
		
		Each Resource definition should be in its own template file. 
		The input to the Go template is the "input cluster configuration" in order to allow expected user variable content
		to be synchronized between cluster CR and reference CR prior to the diff.
		The usage of all Go built-in functions is supported along with the functions in the Sprig library.
		All templates should always be valid YAML after template execution, even when injecting an empty mapping.
		Before using functions that can fail for nil values, always check that the value exists.

		It's possible to pass a user config that contains an option to specify manual matches between cluster resources
		and Resource templates. The matches can be added to the config as pairs of 
		apiVersion_kind_namespace_name: <Template File Name>. For resources that don't have a namespace the matches can 
		be added  as pairs of apiVersion_kind_name: <Template File Name>.

		KUBECTL_EXTERNAL_DIFF environment variable can be used to select your own diff
		command. Users can use external commands with params too, example:
		KUBECTL_EXTERNAL_DIFF="colordiff -N -u"
		
		 By default, the "diff" command available in your path will be run with the "-u"
		(unified diff) and "-N" (treat absent files as empty) options.
		
		 Exit status: 0 No differences were found. 1 Differences were found. >1 oc
		or diff failed with an error.
		
		 Note: KUBECTL_EXTERNAL_DIFF, if used, is expected to follow that convention.

		Experimental: This command is under active development and may change without notice.
	`)

	compareExample = templates.Examples(`
		# Compare a known valid reference configuration with a live cluster:
		oc adm compare -r ./reference
		
		# Compare a known valid reference configuration with a local set of CRs:
		oc adm compare -r ./reference -f ./crsdir -R

		# Compare a known valid reference configuration with a live cluster and with a user config:
		oc adm compare -r ./reference -c ./user_config

		# Run a known valid reference configuration with a must-gather output:
		oc adm compare -r ./reference -f "must-gather*/*/cluster-scoped-resources","must-gather*/*/namespaces" -R
	`)
)

const (
	ReferenceFileName        = "metadata.yaml"
	noReffDirectoryWasPassed = "\"Reference directory is required\""
	reffDirNotExistsError    = "\"Reference directory doesnt exist\""
	emptyTypes               = "Templates don't contain any types (kind) of resources that are supported by the cluster"
)

type Options struct {
	CRs                resource.FilenameOptions
	templatesDir       string
	diffConfigFileName string
	diffAll            bool

	builder     *resource.Builder
	corelator   *MetricsCorelatorDecorator
	templates   []*template.Template
	local       bool
	types       []string
	reff        Reference
	userConfig  UserConfig
	Concurrency int

	diff *diff.DiffProgram
	genericiooptions.IOStreams
}

func NewCmd(f kcmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	options := NewOptions(streams)
	cmd := &cobra.Command{
		Use:                   "compare -r <Reference Directory>",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Compare a reference configuration and a set of cluster configuration CRs."),
		Long:                  compareLong,
		Example:               compareExample,
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckDiffErr(options.Complete(f, cmd, args))
			// `oc compare` propagates the error code from
			//`kubectl diff` that propagates the error code from
			// diff or `KUBECTL_EXTERNAL_DIFF`. Also, we
			// don't want to print an error if diff returns
			// error code 1, which simply means that changes
			// were found. We also don't want oc to
			// return 1 if there was a problem.
			if err := options.Run(); err != nil {
				if exitErr := diffError(err); exitErr != nil {
					kcmdutil.CheckErr(kcmdutil.ErrExit)
				}
				kcmdutil.CheckDiffErr(err)
			}
		},
	}

	// Flag errors exit with code 1, however according to the diff
	// command it means changes were found.
	// Thus, it should return status code greater than 1.
	cmd.SetFlagErrorFunc(func(command *cobra.Command, err error) error {
		kcmdutil.CheckDiffErr(kcmdutil.UsageErrorf(cmd, err.Error()))
		return nil
	})

	cmd.Flags().IntVar(&options.Concurrency, "concurrency", 4,
		"Number of objects to process in parallel when diffing against the live version. Larger number = faster,"+
			" but more memory, I/O and CPU over that shorter period of time.")
	kcmdutil.AddFilenameOptionFlags(cmd, &options.CRs, "contains the configuration to diff")
	cmd.Flags().StringVarP(&options.diffConfigFileName, "diff-config", "c", "", "Path to the user config file")
	cmd.Flags().StringVarP(&options.templatesDir, "reference", "r", "", "Path to directory including reference.")
	cmd.Flags().BoolVarP(&options.diffAll, "all-resources", "A", options.diffAll,
		"If present, In live mode will try to match all resources that are from the types mentioned in the reference. "+
			"In local mode will try to match all resources passed to the command")

	return cmd
}

func NewOptions(ioStreams genericiooptions.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
		diff: &diff.DiffProgram{
			Exec:      exec.New(),
			IOStreams: ioStreams,
		},
	}
}

// DiffError returns the ExitError if the status code is less than 1,
// nil otherwise.
func diffError(err error) exec.ExitError {
	if err, ok := err.(exec.ExitError); ok && err.ExitStatus() <= 1 {
		return err
	}
	return nil
}

func (o *Options) Complete(f kcmdutil.Factory, cmd *cobra.Command, args []string) error {
	var err error
	var fs fs.FS
	o.builder = f.NewBuilder()

	if o.templatesDir == "" {
		return kcmdutil.UsageErrorf(cmd, noReffDirectoryWasPassed)
	}
	if _, err := os.Stat(o.templatesDir); os.IsNotExist(err) && !isURL(o.templatesDir) {
		return fmt.Errorf(reffDirNotExistsError)
	}

	if isURL(o.templatesDir) {
		fs = HTTPFS{baseURL: o.templatesDir, httpGet: httpgetImpl}
	} else {
		rootPath, err := filepath.Abs(o.templatesDir)
		if err != nil {
			return err
		}
		fs = os.DirFS(rootPath)
	}

	o.reff, err = getReference(fs)
	if err != nil {
		return err
	}

	if o.diffConfigFileName != "" {
		o.userConfig, err = parseDiffConfig(o.diffConfigFileName)
		if err != nil {
			return err
		}
	}
	o.templates, err = parseTemplates(o.reff.getTemplates(), o.reff.TemplateFunctionFiles, fs)
	if err != nil {
		return err
	}

	err = o.setupCorelators()
	if err != nil {
		return err
	}

	if len(args) != 0 {
		return kcmdutil.UsageErrorf(cmd, "Unexpected args: %v", args)
	}
	err = o.CRs.RequireFilenameOrKustomize()

	if err == nil {
		o.local = true
		o.types = []string{}
		return nil
	}

	return o.setLiveSearchTypes(f)
}

// setupCorelators initializes a chain of corelators based on the provided options.
// The correlation chain consists of base corelators wrapped with decorator corelators.
// This function configures the following base corelators:
//  1. ExactMatchCorelator - Matches CRs based on pairs specifying, for each cluster CR, its matching template.
//     The pairs are read from the diff config and provided to the corelator.
//  2. GroupCorelator - Matches CRs based on groups of fields that are similar in cluster resources and templates.
//
// The base corelators are combined using a MultiCorelator, which attempts to match a template for each base corelator
// in the specified sequence. The MultiCorelator is further wrapped with a MetricsCorelatorDecorator.
// This decorator not only correlates templates but also records metrics, allowing retrieval that then can be used to create a summary.
func (o *Options) setupCorelators() error {
	var corelators []Corelator
	if len(o.userConfig.CorrelationSettings.ManualCorrelation.CorrelationPairs) > 0 {
		manualCorelator, err := NewExactMatchCorelator(o.userConfig.CorrelationSettings.ManualCorrelation.CorrelationPairs, o.templates)
		if err != nil {
			return err
		}
		corelators = append(corelators, manualCorelator)
	}

	// These fields are used by the GroupCorelator who attempts to match templates based on the following priority order:
	// apiVersion_name_namespace_kind. If no single match is found, it proceeds to trying matching by apiVersion_name_kind,
	// then namespace_kind, and finally kind alone.
	//
	// For instance, consider a template resource with fixed apiVersion, name, and kind, but a templated namespace. The
	// correlator will potentially match this template based on its fixed fields: apiVersion_name_kind.
	var fieldGroups = [][][]string{
		{{"apiVersion"}, {"metadata", "name"}, {"metadata", "namespace"}, {"kind"}},
		{{"apiVersion"}, {"metadata", "namespace"}, {"kind"}},
		{{"metadata", "name"}, {"metadata", "namespace"}, {"kind"}},
		{{"apiVersion"}, {"metadata", "name"}, {"kind"}},
		{{"metadata", "name"}, {"kind"}},
		{{"metadata", "namespace"}, {"kind"}},
		{{"apiVersion"}, {"kind"}},
		{{"kind"}},
	}
	groupCorelator, err := NewGroupCorelator(fieldGroups, o.templates)
	if err != nil {
		return err
	}

	corelators = append(corelators, groupCorelator)

	var erorrsToIgnore []error

	if !o.diffAll {
		erorrsToIgnore = []error{UnknownMatch{}}
	}
	o.corelator = NewMetricsCorelatorDecorator(NewMultiCorealtor(corelators), o.reff.Parts, erorrsToIgnore)
	return nil
}

// setLiveSearchTypes creates a set of resources types to search the live cluster for in order to retrieve cluster resources.
// The types are gathered from the templates included in the reference. The set of types is filtered, so it will include only
// types supported by the live cluster in order to not raise errors by the visitor. In a case the reference includes types that
// are not supported by the user a warning will be created.
func (o *Options) setLiveSearchTypes(f kcmdutil.Factory) error {
	requestedTypes, err := groups.Divide(o.templates, func(element *unstructured.Unstructured) ([]int, error) {
		return []int{0}, nil
	}, extractMetadata, createGroupHashFunc([][]string{{"kind"}}))
	if err != nil {
		return err
	}
	c, err := f.ToDiscoveryClient()
	if err != nil {
		return err
	}
	SupportedTypes, err := getSupportedResourceTypes(c)
	if err != nil {
		return err
	}
	var notSupportedTypes []string
	o.types, notSupportedTypes = findAllRequestedSupportedTypes(SupportedTypes, requestedTypes[0])
	if len(o.types) == 0 {
		return fmt.Errorf(emptyTypes)
	}
	if len(notSupportedTypes) > 0 {
		sort.Strings(notSupportedTypes)
		klog.Warningf("Reference Contains Templates With Types (kind) Not Supported By Cluster: %s", strings.Join(notSupportedTypes, ", "))
	}

	return nil
}

// getSupportedResourceTypes retrieves a set of resource types that are supported by the cluster. For each supported
// resource type it will specify a list of groups where it exists.
func getSupportedResourceTypes(client discovery.CachedDiscoveryInterface) (map[string][]string, error) {
	resources := make(map[string][]string)
	lists, err := client.ServerPreferredResources()
	if err != nil {
		return resources, err
	}
	for _, list := range lists {
		if len(list.APIResources) != 0 {
			for _, res := range list.APIResources {
				resources[res.Kind] = append(resources[res.Kind], res.Group)
			}
		}
	}
	return resources, nil

}

// findAllRequestedSupportedTypes divides the requested types in to two groups: supported types and unsupported types based on if they are specified as supported.
// The list of supported types will include the types in the form of {kind}.{group}.
func findAllRequestedSupportedTypes(supportedTypesWithGroups map[string][]string, requestedTypes map[string][]*template.Template) ([]string, []string) {
	var typesIncludingGroup []string
	var notSupportedTypes []string
	for kind, _ := range requestedTypes {
		if _, ok := supportedTypesWithGroups[kind]; ok {
			for _, group := range supportedTypesWithGroups[kind] {
				typesIncludingGroup = append(typesIncludingGroup, strings.Join([]string{kind, group}, "."))
			}
		} else {
			notSupportedTypes = append(notSupportedTypes, kind)
		}
	}
	return typesIncludingGroup, notSupportedTypes
}

// Run uses the factory to parse file arguments (in case of local mode) or gather all cluster resources matching
// templates types. For each Resource it finds the matching Resource template and
// injects, compares, and runs against differ.
func (o *Options) Run() error {
	showManagedFields := false

	differ, err := diff.NewDiffer("LIVE", "MERGED")
	if err != nil {
		return err
	}
	defer differ.TearDown()

	printer := diff.Printer{}
	r := o.builder.
		Unstructured().
		VisitorConcurrency(o.Concurrency).
		AllNamespaces(true).
		LocalParam(o.local).
		FilenameParam(false, &o.CRs).
		ResourceTypes(o.types...).
		SelectAllParam(!o.local).
		ContinueOnError().
		Flatten().
		Do()
	if err := r.Err(); err != nil {
		return err
	}
	r.IgnoreErrors(func(err error) bool {
		return containOnly(err, []error{MultipleMatches{}, UnknownMatch{}})
	})
	err = r.Visit(func(info *resource.Info, err error) error {
		clusterCRMapping, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(info.Object)
		clusterCR := unstructured.Unstructured{Object: clusterCRMapping}

		temp, err := o.corelator.Match(&clusterCR)
		if err != nil {
			return err
		}

		localRef, err := executeYAMLTemplate(temp, clusterCR.Object)
		if err != nil {
			return err
		}

		obj := InfoObject{
			injectedObjfromTemplate: localRef,
			clusterobj:              &clusterCR,
			FieldsToOmit:            o.reff.FieldsToOmit,
		}

		err = differ.Diff(obj, printer, showManagedFields)

		return err
	})

	if err != nil {
		return err
	}

	err = differ.Run(o.diff)
	a := newSummary(&o.reff, o.corelator)
	_, _ = o.Out.Write([]byte(a.String()))
	return err
}

// InfoObject matches the diff.Object interface, it contains the objects that shall be compared.
type InfoObject struct {
	injectedObjfromTemplate *unstructured.Unstructured
	clusterobj              *unstructured.Unstructured
	FieldsToOmit            [][]string
}

// Live Returns the cluster version of the object
func (obj InfoObject) Live() runtime.Object {
	omitFields(obj.clusterobj.Object, obj.FieldsToOmit)
	return obj.clusterobj
}

// Merged Returns the Injected Reference Version of the Resource
func (obj InfoObject) Merged() (runtime.Object, error) {
	omitFields(obj.injectedObjfromTemplate.Object, obj.FieldsToOmit)
	return obj.injectedObjfromTemplate, nil
}

func omitFields(object map[string]any, fields [][]string) {
	for _, field := range fields {
		unstructured.RemoveNestedField(object, field...)
		for i := 0; i <= len(field); i++ {
			val, _, _ := unstructured.NestedFieldNoCopy(object, field[:len(field)-i]...)
			if mapping, ok := val.(map[string]any); ok && len(mapping) == 0 {
				unstructured.RemoveNestedField(object, field[:len(field)-i]...)
			}
		}
	}
	return
}

func (obj InfoObject) Name() string {
	return slug.Make(apiKindNamespaceName(obj.clusterobj))
}

// summary Contains all info included in the summary output of the compare command
type summary struct {
	RequiredCRS  map[string]map[string][]string
	NumMissing   int
	UnmatchedCRS []string
}

func newSummary(reference *Reference, c *MetricsCorelatorDecorator) *summary {
	s := summary{}
	s.RequiredCRS, s.NumMissing = reference.getMissingCRs(c.MatchedTemplatesNames)
	s.UnmatchedCRS = lo.Map(c.UnMatchedCRs, func(r *unstructured.Unstructured, i int) string {
		return apiKindNamespaceName(r)
	})
	return &s
}

func (s summary) String() string {
	t := `
Summary
{{- if ne (len  .RequiredCRS) 0 }}
Missing required CRs: {{.NumMissing}} 
{{ toYaml .RequiredCRS}}
{{- else}}
No CRs are missing
{{- end }}
{{- if ne (len  .UnmatchedCRS) 0 }}
CRs are unmatched: {{len  .UnmatchedCRS}}
{{ toYaml .UnmatchedCRS}}
{{- else}}
No CRs are unmatched
{{- end }}
`
	var buf bytes.Buffer
	tmpl, _ := template.New("summary").Funcs(template.FuncMap{"toYaml": toYAML}).Parse(t)
	_ = tmpl.Execute(&buf, s)
	return buf.String()
}
