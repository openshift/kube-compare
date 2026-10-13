//go:build darwin

package compare

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func renameNoReplace(oldPath, newPath string) error {
	if err := unix.RenamexNp(oldPath, newPath, unix.RENAME_EXCL); err != nil {
		return fmt.Errorf("renaming extracted reference without replacement: %w", err)
	}
	return nil
}
