package main

import (
	"fmt"
	"os"

	"github.com/openshift/kube-compare/addon-tools/helm-convert/convert"
)

func main() {
	cmd := convert.NewCmd()
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "There was an error: '%s'", err)
		os.Exit(1)
	}
}
