//go:build !linux && !darwin && !windows

package compare

import "errors"

func renameNoReplace(_, _ string) error {
	return errors.New("atomic no-replace publication is unsupported on this platform")
}
