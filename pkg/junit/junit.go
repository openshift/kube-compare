package junit

import (
	"encoding/xml"
	"fmt"
	"io"
)

// TestSuites is a collection of JUnit test suites.
type TestSuites struct {
	XMLName  xml.Name `xml:"testsuites"`
	Name     string   `xml:"name,attr,omitempty"`
	Tests    int      `xml:"tests,attr"`
	Failures int      `xml:"failures,attr"`
	Errors   int      `xml:"errors,attr"`
	Time     string   `xml:"time,attr"`
	Suites   []TestSuite
}

// TestSuite is a single JUnit test suite which may contain many
// testcases.
type TestSuite struct {
	XMLName    xml.Name   `xml:"testsuite"`
	Tests      int        `xml:"tests,attr"`
	Failures   int        `xml:"failures,attr"`
	Time       string     `xml:"time,attr"`
	Name       string     `xml:"name,attr"`
	Properties []Property `xml:"properties>property,omitempty"`
	TestCases  []TestCase
	Timestamp  string `xml:"timestamp,attr"`
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

func Marshal(suites TestSuites) ([]byte, error) {
	doc, err := xml.MarshalIndent(suites, "", "\t")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal junit xml: %w", err)
	}
	return append([]byte(xml.Header), doc...), nil
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
