// SPDX-License-Identifier:Apache-2.0

package main

import (
	"os"

	"github.com/openshift/kube-compare/pkg/compare"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func main() {
	ioStreams := genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	configFlags := genericclioptions.NewConfigFlags(true)
	f := kcmdutil.NewFactory(configFlags)
	compareCmd := compare.NewCmd(f, ioStreams)
	if err := compareCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
