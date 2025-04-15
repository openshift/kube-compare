package junit

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"
)

// TestSuites is a collection of JUnit test suites.
type TestSuites struct {
	XMLName  xml.Name `xml:"testsuites"`
	Name     string   `xml:"name,attr,omitempty"`
	Tests    int      `xml:"tests,attr"`
	Failures int      `xml:"failures,attr"`
	Errors   int      `xml:"errors,attr"`
	Skipped  int      `xml:"skipped,attr"`
	Time     string   `xml:"time,attr"`
	Suites   []TestSuite
}

func NewTestSuites(name string) *TestSuites {
	testSuites := TestSuites{
		Name: name,
		Time: time.Now().Format(time.RFC3339),
	}
	return &testSuites
}

func (id *TestSuites) AddSuite(suite TestSuite) {
	id.Suites = append(id.Suites, suite)
	id.Tests += suite.Tests
	id.Failures += suite.Failures
	id.Skipped += suite.Skipped
}

func (id *TestSuites) WithSuite(suite TestSuite) *TestSuites {
	id.AddSuite(suite)
	return id
}

// TestSuite is a single JUnit test suite which may contain many
// testcases.
type TestSuite struct {
	XMLName    xml.Name   `xml:"testsuite"`
	Tests      int        `xml:"tests,attr"`
	Failures   int        `xml:"failures,attr"`
	Skipped    int        `xml:"skipped,attr"`
	Time       string     `xml:"time,attr"`
	Name       string     `xml:"name,attr"`
	Properties []Property `xml:"properties>property,omitempty"`
	TestCases  []TestCase
	Timestamp  string `xml:"timestamp,attr"`
}

func (id *TestSuite) AddCase(tcase TestCase) {
	id.TestCases = append(id.TestCases, tcase)
	id.Tests += 1
	if tcase.Failure != nil {
		id.Failures += 1
	} else if tcase.SkipMessage != nil {
		id.Skipped += 1
	}
}

func (id *TestSuite) WithCase(tcase TestCase) *TestSuite {
	id.AddCase(tcase)
	return id
}

// TestCase is a single test case with its result.
type TestCase struct {
	XMLName     xml.Name     `xml:"testcase"`
	Classname   string       `xml:"classname,attr"`
	Name        string       `xml:"name,attr"`
	Time        string       `xml:"time,attr"`
	SkipMessage *SkipMessage `xml:"skipped,omitempty"`
	Properties  []Property   `xml:"properties>property,omitempty"`
	Failure     *Failure     `xml:"failure,omitempty"`
}

// SkipMessage contains the reason why a testcase was skipped.
type SkipMessage struct {
	Message string `xml:"message,attr"`
}

// Property represents a key/value pair used to define properties.
type Property struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// Failure contains data related to a failed test.
type Failure struct {
	Message  string `xml:"message,attr"`
	Type     string `xml:"type,attr"`
	Contents string `xml:",chardata"`
}

func NewTestSuite(name string) TestSuite {
	timestamp := time.Now().Format(time.RFC3339)
	return TestSuite{
		Name:      name,
		Timestamp: timestamp,
		Time:      timestamp,
	}
}

func Marshal(suites TestSuites) ([]byte, error) {
	doc, err := xml.MarshalIndent(suites, "", "\t")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal junit xml: %w", err)
	}
	return append([]byte(xml.Header), append(doc, "\n"...)...), nil
}

func Write(out io.Writer, suites TestSuites) error {
	content, err := Marshal(suites)
	if err != nil {
		return err
	}
	_, err = out.Write(content)
	if err != nil {
		return fmt.Errorf("failed to write junit report: %w", err)
	}
	return nil
}
