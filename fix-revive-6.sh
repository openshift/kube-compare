#!/bin/bash
sed -i 's/DiffSeparator         = "\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\n"/\/\/ DiffSeparator separates diff outputs.\n\tDiffSeparator         = "\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\*\\n"/g' pkg/compare/compare.go
