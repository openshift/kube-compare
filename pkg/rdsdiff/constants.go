// SPDX-License-Identifier:Apache-2.0

package rdsdiff

// Paths under the telco-reference configuration root (e.g. telco-ran/configuration).
const (
	// CRSPath is the directory containing PolicyGenerator policy YAMLs and source-crs copy.
	// Matches telco-reference: configuration/argocd/example/acmpolicygenerator
	CRSPath       = "argocd/example/acmpolicygenerator"
	SourceCRSPath = "source-crs"
)

// ListOfCRsForSNO is the list of SNO policy filenames used by PolicyGenerator.
var ListOfCRsForSNO = []string{
	"acm-common-ranGen.yaml",
	"acm-example-sno-site.yaml",
	"acm-group-du-sno-ranGen.yaml",
	"acm-group-du-sno-validator-ranGen.yaml",
}
