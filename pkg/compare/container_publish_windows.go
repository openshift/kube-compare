//go:build windows

package compare

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func renameNoReplace(oldPath, newPath string) error {
	oldPathPointer, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return fmt.Errorf("encoding source publication path: %w", err)
	}
	newPathPointer, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return fmt.Errorf("encoding destination publication path: %w", err)
	}
	if err := windows.MoveFile(oldPathPointer, newPathPointer); err != nil {
		return fmt.Errorf("renaming extracted reference without replacement: %w", err)
	}
	return nil
}
