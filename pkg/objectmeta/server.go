// SPDX-License-Identifier:Apache-2.0

package objectmeta

// ServerManagedMetadataKeys lists ObjectMeta JSON field names populated by the
// API server. They are not part of desired-state manifests and should be stripped
// when generating references and omitted during cluster-compare diffs.
//
// Keep in sync with k8s.io/apimachinery/pkg/util/managedfields/internal/stripmeta.go.
var ServerManagedMetadataKeys = []string{
	"resourceVersion",
	"uid",
	"creationTimestamp",
	"generation",
	"managedFields",
	"selfLink",
}

// MetadataPath returns the fieldsToOmit / ManifestPath path for a metadata key.
func MetadataPath(key string) string {
	return "metadata." + key
}
