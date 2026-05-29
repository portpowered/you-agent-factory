package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrNamedFactoryIsCurrent reports that a named factory cannot be deleted
// because it is selected by .current-factory.
var ErrNamedFactoryIsCurrent = errors.New("cannot delete current factory")

// DeleteNamedFactory removes a persisted named factory directory under rootDir.
// It refuses to delete the factory referenced by .current-factory.
func DeleteNamedFactory(rootDir, name string) error {
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("factory root is required")
	}

	segment, err := safeFactoryLayoutSegment("factory", name)
	if err != nil {
		return err
	}

	factoryDir, err := ResolveNamedFactoryDir(rootDir, segment)
	if err != nil {
		return err
	}

	current, err := ReadCurrentFactoryPointer(rootDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read current factory pointer: %w", err)
	}
	if current == segment {
		return fmt.Errorf(
			"delete factory %q: %w: switch .current-factory to another factory first",
			segment,
			ErrNamedFactoryIsCurrent,
		)
	}

	if err := os.RemoveAll(factoryDir); err != nil {
		return fmt.Errorf("delete factory %q: %w", segment, err)
	}
	return nil
}
