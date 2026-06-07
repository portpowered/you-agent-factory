package factorysessions

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ValidateInitNewFactoryNestedDir rejects init-new-factory when the canonical nested
// factory directory already exists with content that init cannot populate without
// overwrite or cleanup.
func ValidateInitNewFactoryNestedDir(resolvedFolder string) error {
	nestedFactoryDir := filepath.Join(resolvedFolder, interfaces.FactoryDir)
	info, err := os.Stat(nestedFactoryDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return NewValidationError(
			validationReasonUnreadable,
			"folderPath",
			fmt.Errorf("inspect nested factory directory %s: %w", nestedFactoryDir, err),
		)
	}
	if !info.IsDir() {
		return NewValidationError(
			validationReasonConflict,
			"folderPath",
			fmt.Errorf(
				"cannot initialize factory scaffold: %q exists and is not a directory",
				nestedFactoryDir,
			),
		)
	}

	entries, err := os.ReadDir(nestedFactoryDir)
	if err != nil {
		return NewValidationError(
			validationReasonUnreadable,
			"folderPath",
			fmt.Errorf("read nested factory directory %s: %w", nestedFactoryDir, err),
		)
	}
	if len(entries) > 0 {
		return NewValidationError(
			validationReasonConflict,
			"folderPath",
			fmt.Errorf(
				"cannot initialize factory scaffold: %q already exists with conflicting content",
				nestedFactoryDir,
			),
		)
	}
	return nil
}
