package testutils

import (
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var tempRegex *regexp.Regexp

func GetFile(t *testing.T, fileName, value string, update bool) string {
	if update {
		t.Log("update golden file")
		if err := os.WriteFile(fileName, []byte(value), 0644); err != nil { // nolint:gocritic,gosec
			t.Fatalf("test %s failed to update golden file: %s", fileName, err)
		}
	}
	result, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("test %s failed reading .golden file: %s", fileName, err)
	}
	return string(result)
}

func getTempRegex(t *testing.T) *regexp.Regexp {
	if tempRegex == nil {
		tDir, err := os.MkdirTemp("", "tempDirProbe")
		defer os.RemoveAll(tDir)
		require.NoError(t, err)
		tempRegex = regexp.MustCompile(path.Dir(tDir) + `/(?:LIVE|MERGED)-[0-9]*`)
	}
	return tempRegex
}

func RemoveInconsistentInfo(t *testing.T, text string) string {
	// remove diff tool generated temp directory path
	re := getTempRegex(t)
	text = re.ReplaceAllString(text, "TEMP")
	// remove diff datetime
	re = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}\s*\d{2}:\d{2}:\d{2}(:?\.\d{9} [+-]\d{4})?)`)
	text = re.ReplaceAllString(text, "DATE")
	pwd, err := os.Getwd()
	require.NoError(t, err)
	return strings.ReplaceAll(text, pwd, ".")
}
