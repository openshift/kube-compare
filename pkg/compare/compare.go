// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/gosimple/slug"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

		Exit status: 0 No differences were found. 1 Differences were found. >1 kubectl
		or diff failed with an error.

		Note: KUBECTL_EXTERNAL_DIFF, if used, is expected to follow that convention.

		Experimental: This command is under active development and may change without notice.
	`)

	compareExample = templates.Examples(`
		# Compare a known valid reference configuration with a live cluster:
		kubectl cluster-compare -r ./reference/metadata.yaml

		# Compare a known valid reference configuration with a local set of CRs:
		kubectl cluster-compare -r ./reference/metadata.yaml -f ./crsdir -R

		# Compare a known valid reference configuration with a live cluster and with a user config:
		kubectl cluster-compare -r ./reference/metadata.yaml -c ./user_config

		# Run a known valid reference configuration with a must-gather output:
		kubectl cluster-compare -r ./reference/metadata.yaml -f "must-gather*/*/cluster-scoped-resources","must-gather*/*/namespaces" -R

		# Extract a reference configuration from a container image and compare with a local set of CRs:
		kubectl cluster-compare -r container://<IMAGE>:<TAG>:/home/ztp/reference/metadata.yaml -f ./crsdir -R
	`)
)

const (
	noRefFileWasPassed    = "\"Reference config file is required\""
	refFileNotExistsError = "\"Reference config file doesn't exist\""
	emptyTypes            = "templates don't contain any types (kind) of resources that are supported by the cluster"
	DiffSeparator         = "**********************************\n"
	skipInvalidResources  = "Skipping %s Input contains additional files from supported file extensions" +
		" (json/yaml) that do not contain a valid resource, error: %s.\n In case this file is " +
		"expected to be a valid resource modify it accordingly. "
	DiffsFoundMsg           = "there are differences between the cluster CRs and the reference CRs"
	noTemplateForGeneration = "Requested user override generation but no entires for which template to generate overrides for"
	noReason                = "Reason required when generating overrides"
)

const (
	Json      string = "json"
	Yaml      string = "yaml"
	PatchYaml string = "generate-patches"
	Junit     string = "junit"
)

var OutputFormats = []string{Json, Yaml, PatchYaml, Junit}

type Options struct {
	CRs                resource.FilenameOptions
	ReferenceConfig    string
	diffConfigFileName string
	diffAll            bool
	verboseOutput      bool
	ShowManagedFields  bool
	OutputFormat       string

	builder        *resource.Builder
	correlator     *MultiCorrelator[ReferenceTemplate]
	metricsTracker *MetricsTracker
	templates      []ReferenceTemplate
	local          bool
	types          []string
	ref            Reference
	userConfig     UserConfig
	Concurrency    int

	userOverridesPath               string
	userOverridesCorrelator         Correlator[*UserOverride]
	userOverrides                   []*UserOverride
	newUserOverrides                []*UserOverride
	templatesToGenerateOverridesFor []string
	overrideReason                  string

	TmpDir string

	diff *diff.DiffProgram
	genericiooptions.IOStreams

	showTemplateFunctions bool
}

func NewCmd(f kcmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	options := NewOptions(streams)
	example := compareExample
	if strings.HasPrefix(filepath.Base(os.Args[0]), "oc-") {
		example = strings.ReplaceAll(compareExample, "kubectl", "oc")
	} else if !strings.HasPrefix(filepath.Base(os.Args[0]), "kubectl-") {
		example = strings.ReplaceAll(compareExample, "kubectl ", "")
	}

	cmd := &cobra.Command{
		Use:                   "cluster-compare -r <Reference File>",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Compare a reference configuration and a set of cluster configuration CRs."),
		Long:                  compareLong,
		Example:               example,
		Run: func(cmd *cobra.Command, args []string) {
			// Adjust klog to match '--verbose' flag
			klogVerbosity := "0"
			if options.verboseOutput {
				klogVerbosity = "1"
			}
			flagSet := flag.NewFlagSet("test", flag.ExitOnError)
			klog.InitFlags(flagSet)
			_ = flagSet.Parse([]string{"--v", klogVerbosity})

			if options.showTemplateFunctions {
				DisplayFuncmap(os.Stdout)
				return
			}

			// FIXME: Handle creation of temporary directory more gracefully. Right now,
			// kcmdutil.CheckDiffErr calls os.exit(), which does not run defer statements.
			// Maybe we can create an error handler of some kind, and run os.exit() in a PostRun() block.
			tmpDir, err := os.MkdirTemp("", "kube-compare")
			if err != nil {
				klog.Warningf("temporary directory could not be created %s", err)
			} else {
				options.TmpDir = tmpDir
				defer os.RemoveAll(options.TmpDir)
			}
			kcmdutil.CheckDiffErr(options.Complete(f, cmd, args))
			// `kubectl cluster-compare` propagates the error code from
			// `kubectl diff` that propagates the error code from
			// diff or `KUBECTL_EXTERNAL_DIFF`. Also, we
			// don't want to print an error if diff returns
			// error code 1, which simply means that changes
			// were found. We also don't want kubectl to
			// return 1 if there was a problem.
			if err := options.Run(); err != nil {
				// FIXME: Handle clean up of temporary directory more gracefully.
				// See above FIXME for details
				os.RemoveAll(options.TmpDir)
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
		kcmdutil.CheckDiffErr(kcmdutil.UsageErrorf(cmd, "%s", err.Error()))
		return nil
	})
	cmd.Flags().IntVar(&options.Concurrency, "concurrency", 4,
		"Number of objects to process in parallel when diffing against the live version. Larger number = faster,"+
			" but more memory, I/O and CPU over that shorter period of time.")
	kcmdutil.AddFilenameOptionFlags(cmd, &options.CRs, "contains the configuration to diff")
	cmd.Flags().StringVarP(&options.diffConfigFileName, "diff-config", "c", "", "Path to the user config file")
	cmd.Flags().StringVarP(&options.ReferenceConfig, "reference", "r", "", "Path to reference config file.")
	cmd.Flags().BoolVar(&options.ShowManagedFields, "show-managed-fields", options.ShowManagedFields, "If true, include managed fields in the diff.")
	cmd.Flags().BoolVarP(&options.diffAll, "all-resources", "A", options.diffAll,
		"If present, In live mode will try to match all resources that are from the types mentioned in the reference. "+
			"In local mode will try to match all resources passed to the command")
	cmd.Flags().BoolVarP(&options.verboseOutput, "verbose", "v", options.verboseOutput, "Increases the verbosity of the tool")

	cmd.Flags().StringVarP(&options.userOverridesPath, "overrides", "p", "", "Path to user overrides")
	cmd.Flags().StringSliceVar(&options.templatesToGenerateOverridesFor, "generate-override-for", []string{}, "Path for template file you wish to generate a override for")
	cmd.Flags().StringVar(&options.overrideReason, "override-reason", "", "Reason for generating the override")
	cmd.Flags().BoolVar(&options.showTemplateFunctions, "show-template-functions", options.showTemplateFunctions, "Show a list of all available template functions")
	cmd.Flags().StringVarP(&options.OutputFormat, "output", "o", "", fmt.Sprintf(`Output format. One of: (%s)`, strings.Join(OutputFormats, ", ")))
	kcmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"output",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			var comps []string
			for _, format := range OutputFormats {
				if strings.HasPrefix(format, toComplete) {
					comps = append(comps, format)
				}
			}
			return comps, cobra.ShellCompDirectiveNoFileComp
		},
	))

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
	var execErr exec.ExitError
	if ok := errors.As(err, &execErr); ok && execErr.ExitStatus() <= 1 {
		return execErr
	}
	return nil
}

func (o *Options) GetRefFS() (fs.FS, error) {
	referenceDir := filepath.Dir(o.ReferenceConfig)
	if isURL(o.ReferenceConfig) {
		// filepath.Dir removes one / from http://
		referenceDir = strings.Replace(referenceDir, "/", "//", 1)
		return HTTPFS{baseURL: referenceDir, httpGet: httpgetImpl}, nil
	}
	if isContainer(o.ReferenceConfig) {
		// filepath.Dir removes one / from container://
		referenceDir = strings.Replace(referenceDir, "/", "//", 1)
		if o.TmpDir != "" {
			if info, err := os.Stat(o.TmpDir); err == nil && info.IsDir() { // Does directory exist?
				containerPath, err := getReferencesFromContainer(referenceDir, o.TmpDir)
				if err != nil {
					return nil, err
				}
				return os.DirFS(containerPath), nil
			}
		}
		return nil, fmt.Errorf("temporary directory could not be accessed, see logs for details")
	}
	rootPath, err := filepath.Abs(referenceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	return os.DirFS(rootPath), nil
}

func (o *Options) Complete(f kcmdutil.Factory, cmd *cobra.Command, args []string) error {
	var err error
	o.builder = f.NewBuilder()

	if o.OutputFormat == PatchYaml {
		if len(o.templatesToGenerateOverridesFor) == 0 {
			return kcmdutil.UsageErrorf(cmd, noTemplateForGeneration)
		}

		if o.overrideReason == "" {
			return kcmdutil.UsageErrorf(cmd, noReason)
		}
	}

	if o.ReferenceConfig == "" {
		return kcmdutil.UsageErrorf(cmd, noRefFileWasPassed)
	}
	if _, err := os.Stat(o.ReferenceConfig); os.IsNotExist(err) && !isURL(o.ReferenceConfig) && !isContainer(o.ReferenceConfig) {
		return errors.New(refFileNotExistsError)
	}

	cfs, err := o.GetRefFS()
	if err != nil {
		return err
	}

	referenceFileName := filepath.Base(o.ReferenceConfig)
	o.ref, err = GetReference(cfs, referenceFileName)
	if err != nil {
		return err
	}

	if o.diffConfigFileName != "" {
		o.userConfig, err = parseDiffConfig(o.diffConfigFileName)
		if err != nil {
			return err
		}
	}
	o.templates, err = ParseTemplates(o.ref, cfs)
	if err != nil {
		return err
	}

	if o.userOverridesPath != "" {
		o.userOverrides, err = LoadUserOverrides(o.userOverridesPath)
		if err != nil {
			return err
		}
		o.newUserOverrides = append(o.newUserOverrides, o.userOverrides...)
	}

	err = o.setupCorrelators()
	if err != nil {
		return err
	}

	err = o.setupOverrideCorrelators()
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

// These fields are used by the GroupCorrelator who attempts to match templates based on the following priority order:
// apiVersion_name_namespace_kind. If no single match is found, it proceeds to trying matching by apiVersion_name_kind,
// then namespace_kind, and finally kind alone.
//
// For instance, consider a template resource with fixed apiVersion, name, and kind, but a templated namespace. The
// correlator will potentially match this template based on its fixed fields: apiVersion_name_kind.
var defaultFieldGroups = [][][]string{
	{{"apiVersion"}, {"metadata", "name"}, {"metadata", "namespace"}, {"kind"}},
	{{"apiVersion"}, {"metadata", "namespace"}, {"kind"}},
	{{"metadata", "name"}, {"metadata", "namespace"}, {"kind"}},
	{{"apiVersion"}, {"metadata", "name"}, {"kind"}},
	{{"metadata", "name"}, {"kind"}},
	{{"metadata", "namespace"}, {"kind"}},
	{{"apiVersion"}, {"kind"}},
	{{"kind"}},
}

// setupCorrelators initializes a chain of correlators based on the provided options.
// The correlation chain consists of base correlators wrapped with decorator correlators.
// This function configures the following base correlators:
//  1. ExactMatchCorrelator - Matches CRs based on pairs specifying, for each cluster CR, its matching template.
//     The pairs are read from the diff config and provided to the correlator.
//  2. GroupCorrelator - Matches CRs based on groups of fields that are similar in cluster resources and templates.
//
// The base correlators are combined using a MultiCorrelator, which attempts to match a template for each base correlator
// in the specified sequence.
func (o *Options) setupCorrelators() error {
	var correlators []Correlator[ReferenceTemplate]
	if len(o.userConfig.CorrelationSettings.ManualCorrelation.CorrelationPairs) > 0 {
		manualCorrelator, err := NewExactMatchCorrelator(o.userConfig.CorrelationSettings.ManualCorrelation.CorrelationPairs, o.templates)
		if err != nil {
			return err
		}
		correlators = append(correlators, manualCorrelator)
	}

	groupCorrelator, err := NewGroupCorrelator(defaultFieldGroups, o.templates)
	if err != nil {
		return err
	}

	correlators = append(correlators, groupCorrelator)

	o.correlator = NewMultiCorrelator(correlators)
	o.metricsTracker = NewMetricsTracker()
	return nil
}

func (o *Options) setupOverrideCorrelators() error {
	extactOverrideMatches := make(map[string]string)
	for _, uo := range o.userOverrides {
		if uo.ExactMatch != "" {
			extactOverrideMatches[uo.ExactMatch] = uo.GetIdentifier()
		}
	}

	correlators := make([]Correlator[*UserOverride], 0)
	if len(extactOverrideMatches) > 0 {
		manualOverrideCorrelator, err := NewExactMatchCorrelator(extactOverrideMatches, o.userOverrides)
		if err != nil {
			return err
		}
		correlators = append(correlators, manualOverrideCorrelator)
	}

	groupCorrelator, err := NewGroupCorrelator(defaultFieldGroups, o.userOverrides)
	if err != nil {
		return err
	}
	correlators = append(correlators, groupCorrelator)
	o.userOverridesCorrelator = NewMultiCorrelator(correlators)

	return nil
}

// setLiveSearchTypes creates a set of resources types to search the live cluster for in order to retrieve cluster resources.
// The types are gathered from the templates included in the reference. The set of types is filtered, so it will include only
// types supported by the live cluster in order to not raise errors by the visitor. In a case the reference includes types that
// are not supported by the user a warning will be created.
func (o *Options) setLiveSearchTypes(f kcmdutil.Factory) error {
	kindSet := make(map[string][]ReferenceTemplate)
	for _, t := range o.templates {
		kindSet[t.GetMetadata().GetKind()] = append(kindSet[t.GetMetadata().GetKind()], t)
	}

	c, err := f.ToDiscoveryClient()
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}
	SupportedTypes, err := getSupportedResourceTypes(c)
	if err != nil {
		return err
	}
	var notSupportedTypes []string
	o.types, notSupportedTypes = findAllRequestedSupportedTypes(SupportedTypes, kindSet)
	if len(o.types) == 0 {
		return errors.New(emptyTypes)
	}
	if len(notSupportedTypes) > 0 {
		sort.Strings(notSupportedTypes)
		klog.Warningf("Reference Contains Templates With Types (kind) Not Supported By Cluster: %s", strings.Join(notSupportedTypes, ", "))
	}

	return nil
}

// getSupportedResourceTypes retrieves a set of resource types that are supported by the cluster. For each supported
// resource type it will specify a list of groups where it exists.
func getSupportedResourceTypes(client discovery.CachedDiscoveryInterface) (map[string][]schema.GroupVersion, error) {
	resources := make(map[string][]schema.GroupVersion)
	_, lists, err := client.ServerGroupsAndResources()
	if err != nil {
		return resources, fmt.Errorf("failed to get clusters resource types: %w", err)
	}
	for _, list := range lists {
		if len(list.APIResources) != 0 {
			for _, res := range list.APIResources {
				gv := schema.GroupVersion{Group: res.Group, Version: res.Version}
				if !slices.Contains(resources[res.Kind], gv) {
					resources[res.Kind] = append(resources[res.Kind], gv)
				}
			}
		}
	}
	return resources, nil
}

func getExpectedGroups(templates []ReferenceTemplate) []schema.GroupVersion {
	groups := make([]schema.GroupVersion, 0)
	for _, t := range templates {
		gvk := t.GetMetadata().GroupVersionKind()
		gv := schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}
		if gvk.Group != "" && !slices.Contains(groups, gv) {
			groups = append(groups, gv)
		}
	}
	return groups
}

// findAllRequestedSupportedTypes divides the requested types in to two groups: supported types and unsupported types based on if they are specified as supported.
// The list of supported types will include the types in the form of {kind}.{group}.
func findAllRequestedSupportedTypes(supportedTypesWithGroups map[string][]schema.GroupVersion, requestedTypes map[string][]ReferenceTemplate) ([]string, []string) {
	var typesIncludingGroup []string
	var notSupportedTypes []string
	var badAPI []string
	for kind, templates := range requestedTypes {
		if _, ok := supportedTypesWithGroups[kind]; ok {
			expectedGroups := getExpectedGroups(templates)
			for _, gv := range supportedTypesWithGroups[kind] {
				index := slices.Index(expectedGroups, gv)
				if index > -1 {
					expectedGroups = slices.Delete(expectedGroups, index, index+1)
				}
				var supported string
				if gv.Group == "" {
					supported = kind
				} else {
					supported = strings.Join([]string{kind, gv.Version, gv.Group}, ".")
				}

				typesIncludingGroup = append(typesIncludingGroup, supported)
			}
			for _, gv := range expectedGroups {
				badAPI = append(badAPI, strings.Join([]string{kind, gv.Group + "/" + gv.Version}, "."))
			}
		} else {
			notSupportedTypes = append(notSupportedTypes, kind)
		}
	}
	if len(badAPI) > 0 {
		slices.Sort(badAPI)
		klog.Warningf(
			"There may be an issue with the API resources exposed by the cluster. Found kind but missing group/version for %s ",
			strings.Join(badAPI, ", "))
	}
	return typesIncludingGroup, notSupportedTypes
}

func extractPath(str string, pathIndex int) string {
	if split := strings.Split(str, " "); len(split) >= pathIndex {
		return split[pathIndex]
	}
	return "Unknown Path"
}

func findBestMatch(matches []*diffResult) *diffResult {
	var bestMatch *diffResult
	for _, match := range matches {
		klog.V(1).Infof(" - %s - Diff score: %d", match.temp.GetPath(), match.diffScore)
		if bestMatch == nil || match.diffScore < bestMatch.diffScore {
			bestMatch = match
		}
	}
	return bestMatch
}

func getBestMatchByLines(templates []ReferenceTemplate, cr *unstructured.Unstructured, userOverrides []*UserOverride, o *Options) (*diffResult, error) {
	matches := make([]*diffResult, 0)
	errs := make([]error, 0)

	for _, temp := range templates {
		templateOverrides := make([]*UserOverride, 0)
		for _, uo := range userOverrides {
			if uo.TemplatePath == "" || uo.TemplatePath == temp.GetPath() {
				templateOverrides = append(templateOverrides, uo)
			}
		}

		diffResult, err := diffAgainstTemplate(temp, cr, templateOverrides, o)
		if err != nil {
			var nomatch *DoNotMatch
			if errors.As(err, &nomatch) {
				klog.V(1).Infof("Template %s excluded itself from matching: %s", temp.GetPath(), nomatch.Reason)
				// Do not count this as an error, just skip its inclusion in the match list.
			} else {
				errs = append(errs, err)
			}
			continue
		}
		matches = append(matches, diffResult)
	}
	klog.V(1).Infof("Found %d matches for %s", len(matches), apiKindNamespaceName(cr))
	if len(matches) == 0 && len(errs) == 0 {
		// The caller expects an error return if there are no matches
		errs = append(errs, &DoNotMatch{Reason: "Excluded by all possible templates"})
	}
	return findBestMatch(matches), errors.Join(errs...)
}

type diffResult struct {
	output    *bytes.Buffer
	exitError exec.ExitError

	userOverride *UserOverride
	temp         ReferenceTemplate
	crname       string
	diffScore    int
}

func (d diffResult) IsDiff() bool {
	res := d.diffScore > 0
	if !res && d.exitError != nil && d.exitError.ExitStatus() == 1 {
		klog.Warningf("%s: Internally we found no difference but the external tool responded with an exit code of 1", d.crname)
	}
	if res && d.exitError == nil {
		klog.Warningf("%s: Internally we found a difference but the external tool responded with an exit code of 0", d.crname)
	}
	return res
}

func (d diffResult) DiffOutput() *bytes.Buffer {
	return d.output
}

func diffAgainstTemplate(temp ReferenceTemplate, clusterCR *unstructured.Unstructured, userOverrides []*UserOverride, o *Options) (*diffResult, error) {
	res := &diffResult{
		crname: apiKindNamespaceName(clusterCR),
		temp:   temp,
	}

	obj := InfoObject{
		templateSource:    temp.GetPath(),
		FieldsToOmit:      temp.GetFieldsToOmit(o.ref.GetFieldsToOmit()),
		allowMerge:        temp.GetConfig().GetAllowMerge(),
		userOverrides:     userOverrides,
		templateFieldConf: temp.GetConfig().GetInlineDiffFuncs(),
	}
	err := obj.initializeObjData(temp, clusterCR)
	if err != nil {
		return res, fmt.Errorf("template injection failed: %w", err)
	}

	differ, err := diff.NewDiffer("MERGED", "LIVE")
	diffOutput := new(bytes.Buffer)

	res.output = diffOutput
	if err != nil {
		return res, fmt.Errorf("failed to create diff instance: %w", err)
	}
	defer differ.TearDown()

	err = differ.Diff(obj, diff.Printer{}, o.ShowManagedFields)
	if err != nil {
		return res, fmt.Errorf("error occurered during diff: %w", err)
	}
	err = differ.Run(&diff.DiffProgram{Exec: exec.New(), IOStreams: genericiooptions.IOStreams{In: o.In, Out: diffOutput, ErrOut: o.ErrOut}})

	// If the diff tool runs without issues and detects differences at this level of the code, we would like to report that there are no issues
	var exitErr exec.ExitError
	if ok := errors.As(err, &exitErr); ok && exitErr.ExitStatus() <= 1 {
		res.exitError = exitErr
	} else if err != nil {
		return res, fmt.Errorf("diff exited with non-zero code: %w", err)
	}

	// Construct a diffScore based on a diffmatchpatch character-wise diff.
	// The score is used to decide which of multiple matches is the best match.
	dmp := diffmatchpatch.New()
	templateData, err := os.ReadFile(filepath.Join(differ.From.Dir.Name, obj.Name()))
	if err != nil {
		return res, fmt.Errorf("readback of diff From object: %w", err)
	}
	crData, err := os.ReadFile(filepath.Join(differ.To.Dir.Name, obj.Name()))
	if err != nil {
		return res, fmt.Errorf("readback of diff To object: %w", err)
	}
	diffs := dmp.DiffMain(string(templateData), string(crData), false)
	count := 0
	for _, diff := range diffs {
		if diff.Type != diffmatchpatch.DiffEqual {
			count += len(diff.Text)
		}
	}
	res.diffScore = count

	// Create the user override MergePatch for later use
	// TODO: This is based on obj.Live() and obj.Merged() which may be
	// marginally different than the differ-written files used in the two prior
	// steps.
	uo, err := CreateMergePatch(temp, &obj, o.overrideReason)
	if err != nil {
		return res, err
	}
	res.userOverride = uo

	return res, nil
}

// Run uses the factory to parse file arguments (in case of local mode) or gather all cluster resources matching
// templates types. For each Resource it finds the matching Resource template and
// injects, compares, and runs against differ.
func (o *Options) Run() error {
	diffs := make([]DiffSum, 0)
	numDiffCRs := 0
	numPatched := 0

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
		return fmt.Errorf("failed to collect resources: %w", err)
	}
	ignoreErrors := func(err error) bool {
		if strings.Contains(err.Error(), "Object 'Kind' is missing") {
			klog.Warningf(skipInvalidResources, extractPath(err.Error(), 3), "'Kind' is missing")
			return true
		}
		if strings.Contains(err.Error(), "error parsing") {
			// TODO: Fix this error message truncation
			klog.Warningf(skipInvalidResources, extractPath(err.Error(), 2), err.Error()[strings.LastIndex(err.Error(), ":"):])
			return true
		}
		return containOnly(err, []error{UnknownMatch{}, MergeError{}, InlineDiffError{}})
	}
	r.IgnoreErrors(ignoreErrors)

	infos, err := r.Infos()
	if err != nil {
		return fmt.Errorf("error occurred while trying to fetch resources: %w", err)
	}

	clusterCRs := make([]*unstructured.Unstructured, 0)
	uniqueIDs := make(map[string]bool)
	for _, info := range infos {
		clusterCRMapping, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(info.Object)
		obj := &unstructured.Unstructured{Object: clusterCRMapping}
		id := apiKindNamespaceName(obj)
		if _, exists := uniqueIDs[id]; !exists {
			klog.V(1).Infof("Loading object %s", id)
			clusterCRs = append(clusterCRs, obj)
			uniqueIDs[id] = true
		} else {
			klog.V(2).Infof("Skipping duplicate object %s", id)
		}
	}

	// Load all CRs for the lookup function:
	AllCRs = clusterCRs

	process := func(clusterCR *unstructured.Unstructured) error {
		temps, err := o.correlator.Match(clusterCR)
		if err != nil && (!containOnly(err, []error{UnknownMatch{}}) || o.diffAll) {
			o.metricsTracker.addUNMatch(clusterCR)
		}
		if err != nil {
			return err
		}

		userOverrides, err := o.userOverridesCorrelator.Match(clusterCR)
		if err != nil && !containOnly(err, []error{UnknownMatch{}}) {
			return err //nolint: wrapcheck
		}

		bestMatch, err := getBestMatchByLines(temps, clusterCR, userOverrides, o)
		if err != nil {
			var nomatch *DoNotMatch
			if errors.As(err, &nomatch) {
				klog.V(1).Infof("Skipping comparison of %s: doNotMatch returned by all templates", apiKindNamespaceName(clusterCR))
				return nil
			} else {
				o.metricsTracker.addUNMatch(clusterCR)
			}
			return err
		}

		o.metricsTracker.addMatch(bestMatch.temp)

		if bestMatch.IsDiff() {
			numDiffCRs += 1
		}

		if bestMatch.userOverride != nil && slices.Contains(o.templatesToGenerateOverridesFor, bestMatch.temp.GetPath()) {
			o.newUserOverrides = append(o.newUserOverrides, bestMatch.userOverride)
		}

		patched := ""

		reasons := make([]string, 0)
		if len(userOverrides) > 0 {
			patched = o.userOverridesPath
			for _, uo := range userOverrides {
				if uo.Reason != "" {
					reasons = append(reasons, uo.Reason)
				}
			}
			numPatched += 1
		}

		diffs = append(diffs, DiffSum{
			DiffOutput:         bestMatch.DiffOutput().String(),
			CorrelatedTemplate: bestMatch.temp.GetIdentifier(),
			CRName:             apiKindNamespaceName(clusterCR),
			Patched:            patched,
			OverrideReasons:    reasons,
			Description:        bestMatch.temp.GetDescription(),
		})
		return err
	}
	errs := make([]error, 0)
	for _, clusterCR := range clusterCRs {
		err := process(clusterCR)
		if err != nil && !ignoreErrors(err) {
			errs = append(errs, err)
		}
	}
	err = errors.Join(errs...)
	if err != nil {
		return fmt.Errorf("error occurred while trying to process resources: %w", err)
	}

	sum := newSummary(o.ref, o.metricsTracker, numDiffCRs, o.templates, numPatched)

	_, err = Output{Summary: sum, Diffs: &diffs, patches: o.newUserOverrides}.Print(o.OutputFormat, o.Out, o.verboseOutput)
	if err != nil {
		return err
	}

	// We will return exit code 1 in case there are differences between the reference CRs and cluster CRs.
	// The differences can be differences found in specific CRs or any validation issues.
	// As long as we're not generating a set of user overrides.
	if (numDiffCRs != 0 || len(sum.ValidationIssues) != 0) && o.OutputFormat != PatchYaml {
		return exec.CodeExitError{Err: errors.New(DiffsFoundMsg), Code: 1}
	}
	return nil
}

// InfoObject matches the diff.Object interface, it contains the objects that shall be compared.
type InfoObject struct {
	templateSource          string
	injectedObjFromTemplate *unstructured.Unstructured
	clusterObj              *unstructured.Unstructured
	FieldsToOmit            []*ManifestPathV1
	allowMerge              bool
	userOverrides           []*UserOverride
	templateFieldConf       map[string]inlineDiffType
}

// Live Returns the cluster version of the object
func (obj InfoObject) Live() runtime.Object {
	return obj.clusterObj
}

type MergeError struct {
	obj *InfoObject
	err error
}

func (e MergeError) Error() string {
	return fmt.Sprintf("failed to properly merge the manifests for %s some diff may be incorrect: %s", e.obj.Name(), e.err)
}

// Initialize obj.clusterCR and obj.injectedObjFromTemplate
func (obj *InfoObject) initializeObjData(temp ReferenceTemplate, clusterCR *unstructured.Unstructured) error {
	obj.clusterObj = clusterCR
	omitFields(obj.clusterObj.Object, obj.FieldsToOmit)

	var err error
	klog.V(1).Infof("Executing template %s", temp.GetPath())
	localRef, err := temp.Exec(clusterCR.Object)
	if err != nil {
		return err //nolint: wrapcheck
	}
	obj.injectedObjFromTemplate = localRef
	if obj.allowMerge {
		obj.injectedObjFromTemplate, err = MergeManifests(obj.injectedObjFromTemplate, obj.clusterObj)
		if err != nil {
			return &MergeError{obj: obj, err: err}
		}
	}

	for _, override := range obj.userOverrides {
		patched, err := override.Apply(obj.injectedObjFromTemplate, obj.clusterObj)
		if err != nil {
			return err
		}
		obj.injectedObjFromTemplate = patched
	}
	err = obj.runInlineDiffFuncs()
	if err != nil {
		return &InlineDiffError{obj: obj, err: err}
	}
	omitFields(obj.injectedObjFromTemplate.Object, obj.FieldsToOmit)
	return nil
}

// Merged Returns the Injected Reference Version of the Resource
func (obj InfoObject) Merged() (runtime.Object, error) {
	return obj.injectedObjFromTemplate, nil
}

type InlineDiffError struct {
	obj *InfoObject
	err error
}

func (e InlineDiffError) Error() string {
	return fmt.Sprintf("failed to properly run inline diff functions for %s some diff may be incorrect: %s", e.obj.Name(), e.err)
}

func (obj InfoObject) runInlineDiffFuncs() error {
	var errs []error

	// Sort the configured paths for reproducibility
	sortedPaths := make([]string, 0, len(obj.templateFieldConf))
	for pathToKey := range obj.templateFieldConf {
		sortedPaths = append(sortedPaths, pathToKey)
	}
	slices.Sort(sortedPaths)

	// Pass 1: Verify the DiffFn and record any capturegroup matches
	type DiffValues struct {
		value        string
		clusterValue string
		listedPath   []string
		pathToKey    string
		diffFn       InlineDiff
	}
	preprocessedValues := make([]DiffValues, 0, len(obj.templateFieldConf))
	sharedCapturegroups := CapturedValues{}
	for _, pathToKey := range sortedPaths {
		inlineDiffFunc := obj.templateFieldConf[pathToKey]
		listedPath, err := pathToList(pathToKey)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to parse path of field %s that uses inline diff func: %w", pathToKey, err))
			continue
		}
		value, exist, err := NestedString(obj.injectedObjFromTemplate.Object, listedPath...)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to acces value in template of field %s that uses inline diff func: %w", pathToKey, err))
			continue
		}
		if !exist {
			errs = append(errs, fmt.Errorf("failed to acces value in template of field %s that uses inline diff func: Not found", pathToKey))
			continue
		}
		clusterValue, exist, err := NestedString(obj.clusterObj.Object, listedPath...)
		if !exist {
			continue // if value does not appear in cluster CR then there will be a diff anyway and this is not an error
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to acces value in cluster cr of field %s that uses inline diff func: %w", pathToKey, err))
			continue
		}
		diffFn := InlineDiffs[inlineDiffFunc]
		err = diffFn.Validate(value)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to validate the inline diff for field %s, %w", pathToKey, err))
			continue
		}
		klog.V(1).Infof("Performing %s comparison for %s::%s", inlineDiffFunc, obj.templateSource, pathToKey)
		_, updatedCapturegroups := diffFn.Diff(value, clusterValue, sharedCapturegroups)
		sharedCapturegroups = updatedCapturegroups
		preprocessedValues = append(preprocessedValues, DiffValues{
			value:        value,
			clusterValue: clusterValue,
			listedPath:   listedPath,
			pathToKey:    pathToKey,
			diffFn:       diffFn,
		})
	}

	// Pass 2: Actually do the diff and substitute in any matching results
	for _, v := range preprocessedValues {
		patchedString, updatedCapturegroups := v.diffFn.Diff(v.value, v.clusterValue, sharedCapturegroups)
		sharedCapturegroups = updatedCapturegroups
		err := SetNestedString(obj.injectedObjFromTemplate.Object, patchedString, v.listedPath...)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to update value of inline diff func result for field %s, %w", v.pathToKey, err))
			continue
		}
	}
	return errors.Join(errs...)
}

func findFieldPaths(object map[string]any, fields []*ManifestPathV1) [][]string {
	result := make([][]string, 0)
	for _, f := range fields {
		if !f.IsPrefix {
			result = append(result, f.parts)
		} else {
			start := f.parts[:len(f.parts)-1]
			prefix := f.parts[len(f.parts)-1]

			val, _, _ := NestedField(object, start...)
			if mapping, ok := val.(map[string]any); ok {
				for key := range mapping {
					if strings.HasPrefix(key, prefix) {
						newPath := append([]string{}, start...)
						newPath = append(newPath, key)
						result = append(result, newPath)
					}
				}
			}
		}
	}

	return result
}

func omitFields(object map[string]any, fields []*ManifestPathV1) {
	fieldPaths := findFieldPaths(object, fields)

	for _, field := range fieldPaths {
		unstructured.RemoveNestedField(object, field...)
		for i := 0; i <= len(field); i++ {
			val, _, _ := NestedField(object, field[:len(field)-i]...)
			if mapping, ok := val.(map[string]any); ok && len(mapping) == 0 {
				unstructured.RemoveNestedField(object, field[:len(field)-i]...)
			}
		}
	}
}

// MergeManifests will return an attempt to update the localRef with the clusterCR. In the case of an error it will return an unmodified localRef.
func MergeManifests(localRef, clusterCR *unstructured.Unstructured) (updateLocalRef *unstructured.Unstructured, err error) {
	localRefData, err := json.Marshal(localRef)
	if err != nil {
		return localRef, fmt.Errorf("failed to marshal reference CR: %w", err)
	}

	clusterCRData, err := json.Marshal(clusterCR.Object)
	if err != nil {
		return localRef, fmt.Errorf("failed to marshal cluster CR: %w", err)
	}

	localRefUpdatedData, err := jsonpatch.MergePatch(clusterCRData, localRefData)
	if err != nil {
		return localRef, fmt.Errorf("failed to merge cluster and reference CRs: %w", err)
	}

	localRefUpdatedObj := make(map[string]any)
	err = json.Unmarshal(localRefUpdatedData, &localRefUpdatedObj)
	if err != nil {
		return localRef, fmt.Errorf("failed to unmarshal updated manifest: %w", err)
	}

	return &unstructured.Unstructured{Object: localRefUpdatedObj}, nil
}

func (obj InfoObject) Name() string {
	return slug.Make(apiKindNamespaceName(obj.clusterObj))
}
