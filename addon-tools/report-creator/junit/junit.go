package junit

import (
	"encoding/xml"
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

func Write(out io.Writer, suites TestSuites) error {
	doc, err := xml.MarshalIndent(suites, "", "\t")
	if err != nil {
		return err
	}
	_, err = out.Write([]byte(xml.Header))
	if err != nil {
		return err
	}
	_, err = out.Write(doc)
	return err
}
