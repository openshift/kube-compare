// SPDX-License-Identifier:Apache-2.0

package compare

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/openshift/kube-compare/pkg/groups"
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

var TestDirs = "testdata"

const ResourceDirName = "resources"

var userConfigFileName = "userconfig.yaml"
var defaultConcurrency = "4"

type checkType string

const (
	matchFile   checkType = "file"
	matchPrefix checkType = "prefix"
	matchRegex  checkType = "regex"
)

type Check struct {
	checkType checkType
	value     string
	checkOut  bool
}

func (c Check) getPath(test Test, mode Mode) string {
	if c.value != "" {
		return path.Join(test.getTestDir(), c.value)
	}
	suffix := "err.golden"
	if c.checkOut {
		suffix = "out.golden"
	}
	return path.Join(test.getTestDir(), string(mode.crSource)+suffix)
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
		require.Regexp(t, c.value, string(value))
	}
}

func checkFile(t *testing.T, fileName, value string) {
	if *update {
		t.Log("update golden file")
		if err := os.WriteFile(fileName, []byte(value), 0644); err != nil {
			t.Fatalf("test %s failed to update golden file: %s", fileName, err)
		}
	}
	expected, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("test %s failed reading .golden file: %s", fileName, err)
	}
	require.Equal(t, string(expected), string(value))
}

var defaultCheckOut = Check{
	checkType: matchFile,
	checkOut:  true,
}
var defaultCheckErr = Check{
	checkType: matchFile,
}

type CRSource string

const (
	Local CRSource = "local"
	Live  CRSource = "live"
)

type ReffType string

const (
	LocalReff ReffType = "LocalReff"
	URL       ReffType = "URL"
)

type Mode struct {
	crSource   CRSource
	reffSource ReffType
}

func (m *Mode) String() string {
	if m.reffSource == URL {
		return fmt.Sprintf("%s-%s", m.crSource, m.reffSource)
	}
	return string(m.crSource)
}

var DefaultMode = Mode{crSource: Local, reffSource: LocalReff}

type Checks struct {
	Out Check
	Err Check
}

var defaultChecks = Checks{
	Out: defaultCheckOut,
	Err: defaultCheckErr,
}

type Test struct {
	name                  string
	leaveTemplateDirEmpty bool
	mode                  []Mode
	shouldPassUserConfig  bool
	shouldDiffAll         bool
	outputFormat          string
	checks                Checks
}

func (test *Test) getTestDir() string {
	return path.Join(TestDirs, strings.ReplaceAll(test.name, " ", ""))
}

// TestCompareRun ensures that Run command calls the right actions
// and returns the expected error.
func TestCompareRun(t *testing.T) {
	tests := []Test{
		{
			name:                  "No Input",
			mode:                  []Mode{DefaultMode},
			leaveTemplateDirEmpty: true,
			checks:                defaultChecks,
		},
		{
			name:   "Reffernce Directory Doesnt Exist",
			mode:   []Mode{DefaultMode},
			checks: defaultChecks,
		},
		{
			name: "Reffernce Config File Doesnt Exist",
			mode: []Mode{DefaultMode},
			checks: Checks{
				Out: defaultCheckOut,
				Err: Check{
					checkType: matchRegex,
					value: strings.TrimSpace(`
error: Reference config file not found. error: open .*metadata.yaml: no such file or directory 
error code:2`),
				},
			},
		},
		{
			name:   "Reffernce Config File Isnt Valid YAML",
			mode:   []Mode{DefaultMode},
			checks: defaultChecks,
		},
		{
			name:   "Reference Contains Templates That Dont Exist",
			mode:   []Mode{DefaultMode},
			checks: defaultChecks,
		},
		{
			name:   "Reference Contains Templates That Dont Parse",
			mode:   []Mode{DefaultMode},
			checks: defaultChecks,
		},
		{
			name:   "Reference Contains Function Templates That Dont Parse",
			mode:   []Mode{DefaultMode},
			checks: defaultChecks,
		},
		{
			name:   "Template Isnt YAML After Execution With Empty Map",
			mode:   []Mode{DefaultMode},
			checks: defaultChecks,
		},
		{
			name:   "Template Has No Kind",
			mode:   []Mode{{Live, LocalReff}},
			checks: defaultChecks,
		},
		{
			name:   "Two Templates With Same apiVersion Kind Name Namespace",
			mode:   []Mode{DefaultMode},
			checks: defaultChecks,
		},
		{
			name:   "Two Templates With Same Kind Namespace",
			mode:   []Mode{DefaultMode},
			checks: defaultChecks,
		},
		{
			name:                 "User Config Doesnt Exist",
			shouldPassUserConfig: true,
			mode:                 []Mode{DefaultMode},
			checks: Checks{
				Out: defaultCheckOut,
				Err: Check{
					checkType: matchRegex,
					value: strings.TrimSpace(`
error: User Config File not found. error: open .*testdata/UserConfigDoesntExist/userconfig.yaml: no such file or directory 
error code:2`),
				},
			},
		},
		{
			name:                 "User Config Isnt Correct YAML",
			shouldPassUserConfig: true,
			mode:                 []Mode{DefaultMode},
			checks:               defaultChecks,
		},
		{
			name:                 "User Config Manual Correlation Contains Template That Doesnt Exist",
			shouldPassUserConfig: true,
			mode:                 []Mode{DefaultMode},
			checks:               defaultChecks,
		},
		{
			name:   "Test Local Resource File Doesnt exist",
			mode:   []Mode{{Local, LocalReff}},
			checks: defaultChecks,
		},
		{
			name:   "Templates Contain Kind That Is Not Recognizable In Live Cluster",
			mode:   []Mode{{Live, LocalReff}, {Live, URL}},
			checks: defaultChecks,
		},
		{
			name:   "All Required Templates Exist And There Are No Diffs",
			mode:   []Mode{{Live, LocalReff}, {Local, LocalReff}, {Local, URL}, {Live, URL}},
			checks: defaultChecks,
		},
		{
			name:   "Diff in Custom Omitted Fields Isnt Shown",
			mode:   []Mode{{Live, LocalReff}, {Local, LocalReff}, {Local, URL}},
			checks: defaultChecks,
		},
		{
			name:          "When Using Diff All Flag - All Unmatched Resources Appear In Summary",
			mode:          []Mode{DefaultMode},
			checks:        defaultChecks,
			shouldDiffAll: true,
		},
		{
			name:   "Only Resources That Were Not Matched Because Multiple Matches Appear In Summary",
			mode:   []Mode{DefaultMode},
			checks: defaultChecks,
		},
		{
			name:                 "Manual Correlation Matches Are Prioritized Over Group Correlation",
			mode:                 []Mode{{Live, LocalReff}, {Local, LocalReff}},
			shouldPassUserConfig: true,
			checks:               defaultChecks,
		},
		{
			name:   "Only Required Resources Of Required Component Are Reported Missing (Optional Resources Not Reported)",
			mode:   []Mode{{Live, LocalReff}, {Local, LocalReff}},
			checks: defaultChecks,
		},
		{
			name:   "Required Resources Of Optional Component Are Not Reported Missing",
			mode:   []Mode{{Live, LocalReff}, {Local, LocalReff}},
			checks: defaultChecks,
		},
		{
			name:   "Required Resources Of Optional Component Are Reported Missing If At Least One Of Resources In Group Is Included",
			mode:   []Mode{{Live, LocalReff}, {Local, LocalReff}},
			checks: defaultChecks,
		},
		{
			name:   "Reff Template In Sub Dir Not Reported Missing",
			mode:   []Mode{{Live, LocalReff}, {Local, LocalReff}, {Local, URL}},
			checks: defaultChecks,
		},
		{
			name:                 "Reff Template In Sub Dir Works With Manual Correlation",
			mode:                 []Mode{{Live, LocalReff}, {Local, LocalReff}, {Local, URL}},
			checks:               defaultChecks,
			shouldPassUserConfig: true,
		},
		{
			name:   "Reff With Template Functions Renders As Expected",
			mode:   []Mode{{Live, LocalReff}, {Local, LocalReff}, {Local, URL}},
			checks: defaultChecks,
		},
		{
			name:         "YAML Output",
			mode:         []Mode{DefaultMode},
			outputFormat: string(Yaml),
			checks:       defaultChecks,
		},
		{
			name:         "JSON Output",
			mode:         []Mode{DefaultMode},
			outputFormat: string(Json),
			checks:       defaultChecks,
		},
	}
	tf := cmdtesting.NewTestFactory()
	testFlags := flag.NewFlagSet("test", flag.ContinueOnError)
	klog.InitFlags(testFlags)
	klog.LogToStderr(false)
	_ = testFlags.Parse([]string{"--skip_headers"})
	for _, test := range tests {
		for i, mode := range test.mode {
			t.Run(test.name+mode.String(), func(t *testing.T) {
				IOStream, _, out, _ := genericiooptions.NewTestIOStreams()
				klog.SetOutputBySeverity("INFO", out)
				cmd := getCommand(t, &test, i, tf, &IOStream)
				cmdutil.BehaviorOnFatal(func(str string, code int) {
					errorStr := fmt.Sprintf("%s \nerror code:%d\n", removeInconsistentInfo(t, str), code)
					test.checks.Err.check(t, test, mode, errorStr)
					panic("Expected Error Test Case")
				})
				defer func() {
					_ = recover()
					test.checks.Out.check(t, test, mode, removeInconsistentInfo(t, out.String()))
				}()
				cmd.Run(cmd, []string{})
			})
		}
	}
}

func removeInconsistentInfo(t *testing.T, text string) string {
	//remove diff tool generated temp directory path
	re := regexp.MustCompile(`\/tmp\/(?:LIVE|MERGED)-[0-9]*`)
	text = re.ReplaceAllString(text, "TEMP")
	//remove diff datetime
	re = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}\s*\d{2}:\d{2}:\d{2}\.\d{9} [+-]\d{4})`)
	text = re.ReplaceAllString(text, "DATE")
	pwd, err := os.Getwd()
	require.NoError(t, err)
	return strings.ReplaceAll(text, pwd, ".")
}

func getCommand(t *testing.T, test *Test, modeIndex int, tf *cmdtesting.TestFactory, streams *genericiooptions.IOStreams) *cobra.Command {
	mode := test.mode[modeIndex]
	cmd := NewCmd(tf, *streams)
	require.NoError(t, cmd.Flags().Set("concurrency", defaultConcurrency))
	if test.shouldDiffAll {
		require.NoError(t, cmd.Flags().Set("all-resources", "true"))
	}
	if test.shouldPassUserConfig {
		require.NoError(t, cmd.Flags().Set("diff-config", path.Join(test.getTestDir(), userConfigFileName)))
	}
	if test.outputFormat != "" {
		require.NoError(t, cmd.Flags().Set("output", test.outputFormat))
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
	switch mode.reffSource {
	case URL:
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := os.ReadFile(path.Join(test.getTestDir(), TestRefDirName, r.RequestURI))
			require.NoError(t, err)
			_, err = fmt.Fprint(w, string(body))
			require.NoError(t, err)
		}))
		require.NoError(t, cmd.Flags().Set("reference", svr.URL))
		t.Cleanup(func() {
			svr.Close()
		})

	case LocalReff:
		if !test.leaveTemplateDirEmpty {
			require.NoError(t, cmd.Flags().Set("reference", path.Join(test.getTestDir(), TestRefDirName)))
		}
	}
	return cmd
}

func setClient(t *testing.T, resources []*unstructured.Unstructured, tf *cmdtesting.TestFactory) {
	resourcesByType, _ := groups.Divide(resources, func(element *unstructured.Unstructured) ([]int, error) {
		return []int{0}, nil
	}, func(e *unstructured.Unstructured) (*unstructured.Unstructured, error) {
		return e, nil
	}, createGroupHashFunc([][]string{{"kind"}}))
	resourcesByKind := lo.MapKeys(resourcesByType[0], func(value []*unstructured.Unstructured, key string) string {
		//Converted to URL Path Format:
		return fmt.Sprintf("/%ss", strings.ToLower(key))
	})
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
				return err
			}
			data := make(map[string]any)
			err = yaml.Unmarshal(buf, &data)
			if err != nil {
				return fmt.Errorf("test Input isnt yaml")
			}
			r := unstructured.Unstructured{Object: data}
			resources = append(resources, &r)
			rL = append(rL, v1.APIResource{Name: r.GetName(), Kind: r.GetKind(), Version: r.GetAPIVersion()})
			return nil
		}))
	return rL, resources
}

func updateTestDiscoveryClient(tf *cmdtesting.TestFactory, discoveryResources []v1.APIResource) {
	disccoveryClient := cmdtesting.NewFakeCachedDiscoveryClient()
	ResourceList := v1.APIResourceList{APIResources: discoveryResources}
	disccoveryClient.Resources = append(disccoveryClient.Resources, &ResourceList)
	disccoveryClient.PreferredResources = append(disccoveryClient.PreferredResources, &ResourceList)
	tf.WithDiscoveryClient(disccoveryClient)
}
