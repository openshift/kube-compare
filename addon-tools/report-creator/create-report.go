package main

import (
	"fmt"
	"os"

	"github.com/openshift/kube-compare/addon-tools/report-creator/report"
)

func main() {
	cmd := report.NewCmd()
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "There was an error: '%s'", err)
		os.Exit(1)
	}
}
