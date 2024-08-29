// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift/kube-compare/pkg/testutils"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/rest/fake"
	"k8s.io/klog/v2"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/yaml"
)

var update = flag.Bool("update", false, "update .golden files")

const TestRefDirName = "reference"
const TestRefConfigFileName = "metadata.yaml"
const TestRefConfigFile = TestRefDirName + "/" + TestRefConfigFileName

var TestDirs = "testdata"

const ResourceDirName = "resources"

var userConfigFileName = "userconfig.yaml"
var defaultConcurrency = "4"

type checkType string

const (
	matchFile   checkType = "file"
	matchPrefix checkType = "prefix"
	matchRegex  checkType = "regex"
	matchYaml   checkType = "yaml"
)

type Check struct {
	checkType checkType
	value     string
	suffix    string
}

// withPrefixedSuffix returns a new check with the suffix
// variable prefixed with the supplied string
// this allow you to adjust the golden file fetched
// e.g. if the default is "err.golden" then check.withPrefixedSuffix("other_")
// the golden file fetched will be "other_err.golden"
func (c Check) withPrefixedSuffix(prefix string) Check {
	return Check{
		checkType: c.checkType,
		value:     c.value,
		suffix:    prefix + c.suffix,
	}
}

func (c Check) getPath(test Test, mode Mode) string {
	if c.value != "" {
		return path.Join(test.getTestDir(), c.value)
	}
	return path.Join(test.getTestDir(), string(mode.crSource)+c.suffix)
}

func (c Check) hasErrorFile(test Test, mode Mode) bool {
	if _, err := os.Stat(c.getPath(test, mode)); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func (c Check) check(t *testing.T, test Test, mode Mode, value string) {
	switch c.checkType {
	case matchFile:
		checkFile(t, c.getPath(test, mode), value)
	case matchPrefix:
		require.Conditionf(t,
			func() bool { return strings.HasPrefix(value, c.value) },
			"value %s does not start with %s", value, c.value)
	case matchRegex:
		require.Regexp(t, c.value, value)
	case matchYaml:
		expected := testutils.GetFile(t, c.getPath(test, mode), value, *update)
		require.YAMLEq(t, expected, value)
	}
}

func checkFile(t *testing.T, fileName, value string) {
	expected := testutils.GetFile(t, fileName, value, *update)
	require.Equal(t, expected, value)
}

const (
	defaultOutSuffix = "out.golden"
	defualtErrSuffix = "err.golden"
)

var defaultCheckOut = Check{
	checkType: matchFile,
	suffix:    defaultOutSuffix,
}
var defaultCheckErr = Check{
	checkType: matchFile,
	suffix:    defualtErrSuffix,
}

type CRSource string

const (
	Local CRSource = "local"
	Live  CRSource = "live"
)

type RefType string

const (
	LocalRef RefType = "LocalRef"
	URL      RefType = "URL"
)

type Mode struct {
	crSource  CRSource
	refSource RefType
}

func (m *Mode) String() string {
	if m.refSource == URL {
		return fmt.Sprintf("%s-%s", m.crSource, m.refSource)
	}
	return string(m.crSource)
}

var DefaultMode = Mode{crSource: Local, refSource: LocalRef}

type Checks struct {
	Out Check
	Err Check
}

// withPrefixedSuffix Calls withPrefixedSuffix on each check
// it produces a new set of checks which point to a different
// set of golden files. see Check.withPrefixedSuffix for defails.
func (c Checks) withPrefixedSuffix(suffixPrefix string) Checks {
	return Checks{
		Out: c.Out.withPrefixedSuffix(suffixPrefix),
		Err: c.Err.withPrefixedSuffix(suffixPrefix),
	}
}

var defaultChecks = Checks{
	Out: defaultCheckOut,
	Err: defaultCheckErr,
}

type Test struct {
	name                  string
	leaveTemplateDirEmpty bool
	mode                  []Mode
	userConfigFileName    string
	shouldDiffAll         bool
	outputFormat          string
	checks                Checks
	verboseOutput         bool
}

func (test *Test) getTestDir() string {
	return path.Join(TestDirs, strings.ReplaceAll(test.name, " ", ""))
}

func (test Test) Clone() Test {
	newMode := make([]Mode, 0)
	copy(newMode, test.mode)
	return Test{
		name:                  test.name,
		leaveTemplateDirEmpty: test.leaveTemplateDirEmpty,
		mode:                  test.mode,
		userConfigFileName:    test.userConfigFileName,
		shouldDiffAll:         test.shouldDiffAll,
		outputFormat:          test.outputFormat,
		checks:                test.checks,
		verboseOutput:         test.verboseOutput,
	}
}

func (test Test) withModes(modes []Mode) Test {
	newTest := test.Clone()
	newTest.mode = modes
	return newTest
}

func (test Test) skipReferenceFlag() Test {
	newTest := test.Clone()
	newTest.leaveTemplateDirEmpty = true
	return newTest
}

func (test Test) withChecks(checks Checks) Test {
	newTest := test.Clone()
	newTest.checks = checks
	return newTest
}

func (test Test) withUserConfig(userConfigFileName string) Test {
	newTest := test.Clone()
	newTest.userConfigFileName = userConfigFileName
	return newTest
}

func (test Test) diffAll() Test {
	newTest := test.Clone()
	newTest.shouldDiffAll = true
	return newTest
}

func (test Test) withVerboseOutput() Test {
	newTest := test.Clone()
	newTest.verboseOutput = true
	return newTest
}

func (test Test) withOutputFormat(outputFormat string) Test {
	newTest := test.Clone()
	newTest.outputFormat = outputFormat
	return newTest
}

func defaultTest(name string) Test {
	return Test{
		name:   name,
		mode:   []Mode{DefaultMode},
		checks: defaultChecks,
	}
}

func matchErrorRegexCheck(msg string) Check {
	return Check{
		checkType: matchRegex,
		value:     strings.Join([]string{`error: ` + msg, `error code:2`}, "\n"),
	}
}

const ExpectedPanic = "Expected Error Test Case"

// TestCompareRun ensures that Run command calls the right actions
// and returns the expected error.
func TestCompareRun(t *testing.T) {
	tests := []Test{
		defaultTest("No Input").
			skipReferenceFlag(),
		defaultTest("Reference Config File Doesnt Exist"),
		defaultTest("Reference Config File Isnt Valid YAML"),
		defaultTest("Reference Contains Templates That Dont Exist"),
		defaultTest("Reference Contains Templates That Dont Parse"),
		defaultTest("Reference Contains Function Templates That Dont Parse"),
		defaultTest("Template Isnt YAML After Execution With Empty Map"),
		defaultTest("Template Has No Kind").
			withModes([]Mode{{Live, LocalRef}}),
		defaultTest("Two Templates With Same apiVersion Kind Name Namespace"),
		defaultTest("Two Templates With Same Kind Namespace"),
		defaultTest("User Config Doesnt Exist").
			withUserConfig(userConfigFileName).
			withChecks(Checks{Out: defaultCheckOut,
				Err: matchErrorRegexCheck(
					`User Config File not found. error: open ` +
						`.*testdata/UserConfigDoesntExist/userconfig.yaml: no such file or directory`,
				),
			}),
		defaultTest("User Config Isnt Correct YAML").
			withUserConfig(userConfigFileName),
		defaultTest("User Config Manual Correlation Contains Template That Doesnt Exist").
			withUserConfig(userConfigFileName),
		defaultTest("Test Local Resource File Doesnt exist").
			withModes([]Mode{{Local, LocalRef}}),
		defaultTest("Templates Contain Kind That Is Not Recognizable In Live Cluster").
			withModes([]Mode{{Live, LocalRef}, {Live, URL}}),
		defaultTest("All Required Templates Exist And There Are No Diffs").
			withModes([]Mode{{Live, LocalRef}, {Local, LocalRef}, {Local, URL}, {Live, URL}}),
		defaultTest("Diff in Custom Omitted Fields Isnt Shown").
			withModes([]Mode{{Live, LocalRef}, {Local, LocalRef}, {Local, URL}}),
		defaultTest("Diff in Custom Omitted Fields Isnt Shown All Quoted"),
		defaultTest("Diff in Custom Omitted Fields Isnt Shown Leading Dot"),
		defaultTest("Diff in Custom Omitted Fields Isnt Shown Non Default"),
		defaultTest("Diff in Custom Omitted Fields Isnt Shown Prefix"),
		defaultTest("Custom Fields To Omit Default Key Not Found"),
		defaultTest("Custom Fields To Omit Ref Entry Not Found"),
		defaultTest("When Using Diff All Flag - All Unmatched Resources Appear In Summary").
			diffAll(),
		defaultTest("Manual Correlation Matches Are Prioritized Over Group Correlation").
			withModes([]Mode{{Live, LocalRef}, {Local, LocalRef}}).
			withUserConfig(userConfigFileName),
		defaultTest("Only Required Resources Of Required Component Are Reported Missing (Optional Resources Not Reported)").
			withModes([]Mode{{Live, LocalRef}, {Local, LocalRef}}),
		defaultTest("Required Resources Of Optional Component Are Not Reported Missing").
			withModes([]Mode{{Live, LocalRef}, {Local, LocalRef}}),
		defaultTest("Required Resources Of Optional Component Are Reported Missing If At Least One Of Resources In Group Is Included").
			withModes([]Mode{{Live, LocalRef}, {Local, LocalRef}}),
		defaultTest("Ref Template In Sub Dir Not Reported Missing").
			withModes([]Mode{{Live, LocalRef}, {Local, LocalRef}, {Local, URL}}),
		defaultTest("Ref Template In Sub Dir Works With Manual Correlation").
			withModes([]Mode{{Live, LocalRef}, {Local, LocalRef}, {Local, URL}}).
			withUserConfig(userConfigFileName),
		defaultTest("Ref With Template Functions Renders As Expected").
			withModes([]Mode{{Live, LocalRef}, {Local, LocalRef}, {Local, URL}}),
		defaultTest("YAML Output").
			withOutputFormat(Yaml).
			withChecks(Checks{Err: defaultCheckErr,
				Out: Check{checkType: matchYaml, suffix: defaultOutSuffix},
			}),
		defaultTest("JSON Output").
			withOutputFormat(Json),
		defaultTest("Check Ignore Unspecified Fields Config"),
		defaultTest("Check Merging Does Not Overwrite Template Config"),
		defaultTest("NoDiffs"),
		defaultTest("SomeDiffs"),
		defaultTest("NoDiffs").
			withVerboseOutput().
			withChecks(defaultChecks.withPrefixedSuffix("withVebosityFlag")),
		defaultTest("SomeDiffs").
			withVerboseOutput().
			withChecks(defaultChecks.withPrefixedSuffix("withVebosityFlag")),
		defaultTest("Invalid Resources Are Skipped"),
		defaultTest("Ref Contains Templates With Function Templates In Same File"),
		defaultTest("Machine Configs Catch All"),
	}

	tf := cmdtesting.NewTestFactory()
	testFlags := flag.NewFlagSet("test", flag.ContinueOnError)
	klog.InitFlags(testFlags)
	klog.LogToStderr(false)
	_ = testFlags.Parse([]string{"--skip_headers"})
	for _, test := range tests {
		for i, mode := range test.mode {
			t.Run(test.name+"-"+mode.String(), func(t *testing.T) {
				IOStream, _, out, _ := genericiooptions.NewTestIOStreams()
				klog.SetOutputBySeverity("INFO", out)
				cmd := getCommand(t, &test, i, tf, &IOStream) // nolint:gosec

				hasCheckedError := false
				cmdutil.BehaviorOnFatal(func(str string, code int) {
					errorStr := fmt.Sprintf("%s\nerror code:%d\n", testutils.RemoveInconsistentInfo(t, str), code)
					test.checks.Err.check(t, test, mode, errorStr)
					hasCheckedError = true
					panic(ExpectedPanic)
				})

				defer func() {
					r := recover()
					if s, ok := r.(string); r != nil && (!ok || s != ExpectedPanic) {
						t.Fatalf("test paniced: %v", r)
					}
					if !hasCheckedError && test.checks.Err.hasErrorFile(test, mode) {
						t.Fatalf("Unchecked error file %s", test.checks.Err.getPath(test, mode))
					}
					test.checks.Out.check(t, test, mode, testutils.RemoveInconsistentInfo(t, out.String()))
				}()
				cmd.Run(cmd, []string{})
			})
		}
	}
}

func getCommand(t *testing.T, test *Test, modeIndex int, tf *cmdtesting.TestFactory, streams *genericiooptions.IOStreams) *cobra.Command {
	mode := test.mode[modeIndex]
	cmd := NewCmd(tf, *streams)
	require.NoError(t, cmd.Flags().Set("concurrency", defaultConcurrency))
	if test.shouldDiffAll {
		require.NoError(t, cmd.Flags().Set("all-resources", "true"))
	}
	if test.userConfigFileName != "" {
		require.NoError(t, cmd.Flags().Set("diff-config", path.Join(test.getTestDir(), test.userConfigFileName)))
	}
	if test.outputFormat != "" {
		require.NoError(t, cmd.Flags().Set("output", test.outputFormat))
	}
	if test.verboseOutput {
		require.NoError(t, cmd.Flags().Set("verbose", "true"))
	}
	resourcesDir := path.Join(test.getTestDir(), ResourceDirName)
	switch mode.crSource {
	case Local:
		require.NoError(t, cmd.Flags().Set("filename", resourcesDir))
		require.NoError(t, cmd.Flags().Set("recursive", "true"))
	case Live:
		discoveryResources, resources := getResources(t, resourcesDir)
		updateTestDiscoveryClient(tf, discoveryResources)
		setClient(t, resources, tf)
	}
	switch mode.refSource {
	case URL:
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := os.ReadFile(path.Join(test.getTestDir(), TestRefDirName, r.RequestURI))
			require.NoError(t, err)
			_, err = fmt.Fprint(w, string(body))
			require.NoError(t, err)
		}))
		require.NoError(t, cmd.Flags().Set("reference", svr.URL+"/"+TestRefConfigFileName))
		t.Cleanup(func() {
			svr.Close()
		})

	case LocalRef:
		if !test.leaveTemplateDirEmpty {
			require.NoError(t, cmd.Flags().Set("reference", path.Join(test.getTestDir(), TestRefConfigFile)))
		}
	}
	return cmd
}

func setClient(t *testing.T, resources []*unstructured.Unstructured, tf *cmdtesting.TestFactory) {
	resourcesByKind := make(map[string][]*unstructured.Unstructured)
	for _, t := range resources {
		key := fmt.Sprintf("/%ss", strings.ToLower(t.GetKind()))
		resourcesByKind[key] = append(resourcesByKind[key], t)
	}
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			switch p, m := req.URL.Path, req.Method; {
			case m == "GET":
				a := unstructured.Unstructured{}
				exampleResource := resourcesByKind[p][0]
				a.SetKind(exampleResource.GetKind() + "List")
				a.SetAPIVersion(exampleResource.GetAPIVersion())
				a.SetResourceVersion(exampleResource.GetResourceVersion())

				requestedResources := lo.Map(resourcesByKind[p], func(value *unstructured.Unstructured, index int) any {
					return value.Object
				})

				require.NoError(t, unstructured.SetNestedSlice(a.Object, requestedResources, "items"))
				b, _ := a.MarshalJSON()
				bodyRC := io.NopCloser(bytes.NewReader(b))
				return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, nil
			default:
				t.Fatalf("unexpected request: %#v\n%#v", req.URL, req)
				return nil, nil
			}
		}),
	}
}

func getResources(t *testing.T, resourcesDir string) ([]v1.APIResource, []*unstructured.Unstructured) {
	var resources []*unstructured.Unstructured
	var rL []v1.APIResource
	require.NoError(t, filepath.Walk(resourcesDir,
		func(path string, info os.FileInfo, err error) error {
			if path == resourcesDir {
				return nil
			}
			if err != nil {
				return err
			}
			buf, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to load test file %s: %w", path, err)
			}
			data := make(map[string]any)
			err = yaml.Unmarshal(buf, &data)
			if err != nil {
				return errors.New("test Input is not yaml")
			}
			r := unstructured.Unstructured{Object: data}
			resources = append(resources, &r)
			rL = append(rL, v1.APIResource{Name: r.GetName(), Kind: r.GetKind(), Version: r.GetAPIVersion()})
			return nil
		}))
	return rL, resources
}

func updateTestDiscoveryClient(tf *cmdtesting.TestFactory, discoveryResources []v1.APIResource) {
	discoveryClient := cmdtesting.NewFakeCachedDiscoveryClient()
	ResourceList := v1.APIResourceList{APIResources: discoveryResources}
	discoveryClient.Resources = append(discoveryClient.Resources, &ResourceList)
	discoveryClient.PreferredResources = append(discoveryClient.PreferredResources, &ResourceList)
	tf.WithDiscoveryClient(discoveryClient)
}
