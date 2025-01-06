package main

import (
	"fmt"
	"os"

	"github.com/openshift/kube-compare/addon-tools/report-creator/report"
)

var (
	version = "unreleased"
	date    = "unknown"
)

func main() {
	cmd := report.NewCmd()
	cmd.Version = fmt.Sprintf("%s (%s)", version, date)
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "There was an error: '%s'", err)
		os.Exit(1)
	}
}
