// SPDX-License-Identifier:Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/openshift/kube-compare/pkg/compare"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	version = "unreleased"
	date    = "unknown"
)

func main() {
	ioStreams := genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	configFlags := genericclioptions.NewConfigFlags(true)
	f := kcmdutil.NewFactory(configFlags)
	compareCmd := compare.NewCmd(f, ioStreams)
	compareCmd.Version = fmt.Sprintf("%s (%s)", version, date)
	if err := compareCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
