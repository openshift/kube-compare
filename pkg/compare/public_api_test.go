package compare_test

import (
	"io/fs"
	"testing"

	"github.com/openshift/kube-compare/pkg/compare"
	"github.com/stretchr/testify/require"
)

type legacyGetRefFS func(*compare.Options) (fs.FS, error)

var _ legacyGetRefFS = (*compare.Options).GetRefFS

func TestGetRefFSPublicSourceCompatibility(t *testing.T) {
	options := &compare.Options{ReferenceConfig: t.TempDir() + "/metadata.yaml"}
	_, err := options.GetRefFS()
	require.NoError(t, err)
}
