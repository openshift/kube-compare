#!/bin/bash
sed -i 's/RunE: func(_ \*cobra.Command, args \[\]string) error {/RunE: func(_ \*cobra.Command, _ \[\]string) error {/g' addon-tools/helm-convert/convert/convert.go
sed -i 's/RunE: func(_ \*cobra.Command, args \[\]string) error {/RunE: func(_ \*cobra.Command, _ \[\]string) error {/g' addon-tools/report-creator/report/create.go
sed -i '1i // Package main provides the tool.' addon-tools/helm-convert/helm-convert.go
sed -i '1i // Package main provides the tool.' addon-tools/report-creator/report-creator.go

sed -i '/^\/\/ Package main provides the CLI./d' cmd/kubectl-cluster_compare.go
sed -i '1i // Package main provides the CLI.' cmd/kubectl-cluster_compare.go

sed -i '/^\/\/ Package report provides report creation utilities./d' addon-tools/report-creator/report/create.go
sed -i '1i // Package report provides report creation utilities.' addon-tools/report-creator/report/create.go

# pkg/compare/capturegroupsInlineDiff.go
sed -i 's/		} else {/		}\n\t\t\/\/ Multiple matches detected, so call attention to them/g' pkg/compare/capturegroupsInlineDiff.go
sed -i '/\/\/ Multiple matches detected, so call attention to them/d' pkg/compare/capturegroupsInlineDiff.go
sed -i '/return fmt.Sprintf("(?<%s>=%s)", name, matches\[0\])/d' pkg/compare/capturegroupsInlineDiff.go
# wait, manual edit for capturegroupsInlineDiff.go is safer

