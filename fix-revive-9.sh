#!/bin/bash
sed -i '1i // Package convert handles helm conversion.' addon-tools/helm-convert/convert/convert.go

sed -i '/^\/\/ Package main provides the CLI./d' cmd/kubectl-cluster_compare.go
sed -i '1i // Package main provides the CLI.\npackage main' cmd/kubectl-cluster_compare.go
sed -i '/package main/d' cmd/kubectl-cluster_compare.go
sed -i '1i // Package main provides the CLI.\npackage main' cmd/kubectl-cluster_compare.go

sed -i 's/DiffSeparator         = "\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\\n"/\/\/ DiffSeparator separates diff outputs.\n\tDiffSeparator         = "\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\\n"/g' pkg/compare/compare.go
sed -i 's/Yaml      string = "yaml"/\/\/ Yaml is yaml format.\n\tYaml      string = "yaml"/g' pkg/compare/compare.go

sed -i 's/MatchedMoreThanOne = "Should only match one but matched"/\/\/ MatchedMoreThanOne is returned when more than one matches.\n\tMatchedMoreThanOne = "Should only match one but matched"/g' pkg/compare/referenceV2.go

sed -i '/^\/\/ Package generate provides generation utilities./d' pkg/generate/config.go
sed -i '1i // Package generate provides generation utilities.\npackage generate' pkg/generate/config.go
sed -i '/package generate/d' pkg/generate/config.go
sed -i '1i // Package generate provides generation utilities.\npackage generate' pkg/generate/config.go

sed -i '/^\/\/ Package objectmeta provides object metadata utilities./d' pkg/objectmeta/server.go
sed -i '1i // Package objectmeta provides object metadata utilities.\npackage objectmeta' pkg/objectmeta/server.go
sed -i '/package objectmeta/d' pkg/objectmeta/server.go
sed -i '1i // Package objectmeta provides object metadata utilities.\npackage objectmeta' pkg/objectmeta/server.go

