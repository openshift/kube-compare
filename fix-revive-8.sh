#!/bin/bash
sed -i 's/Json/JSON/g' pkg/compare/output.go
sed -i 's/Json/JSON/g' pkg/compare/compare_test.go
sed -i 's/ApiVersion/APIVersion/g' pkg/compare/useroverride.go

# For container_test.go: the signature was reverted back previously, let's look at the func signature
