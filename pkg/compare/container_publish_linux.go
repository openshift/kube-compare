//go:build linux

package compare

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func renameNoReplace(oldPath, newPath string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("renaming extracted reference without replacement: %w", err)
	}
	return nil
}
