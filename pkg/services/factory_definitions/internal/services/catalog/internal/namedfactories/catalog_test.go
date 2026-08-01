package namedfactories_test

import (
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	catalognamedfactories "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/internal/namedfactories"
	catalognamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/internal/namedpaths"
)

func TestNewRequiresAllExternalEffects(t *testing.T) {
	t.Parallel()

	fileSystem := platformfilesystem.Local{}
	paths, err := catalognamedpaths.New(fileSystem)
	if err != nil {
		t.Fatalf("New named paths: %v", err)
	}

	if _, err := catalognamedfactories.New(nil, fileSystem); err == nil || !strings.Contains(err.Error(), "path resolver is required") {
		t.Fatalf("New(nil, filesystem) error = %v, want required path resolver", err)
	}
	if _, err := catalognamedfactories.New(paths, nil); err == nil || !strings.Contains(err.Error(), "catalog filesystem is required") {
		t.Fatalf("New(paths, nil) error = %v, want required catalog filesystem", err)
	}
}
