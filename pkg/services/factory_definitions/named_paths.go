package factorydefinitions

import catalognamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"

// Named Factory path errors remain stable at the Factory Definitions root.
var (
	ErrInvalidName    = catalognamedpaths.ErrInvalidName
	ErrNotFound       = catalognamedpaths.ErrNotFound
	ErrLayoutNotFound = catalognamedpaths.ErrLayoutNotFound
)

// ValidateName validates one canonical named Factory display name.
func ValidateName(name string) error { return catalognamedpaths.ValidateName(name) }

// PathSegments maps a canonical name to its safe hierarchical layout.
func PathSegments(name string) ([]string, error) { return catalognamedpaths.PathSegments(name) }

func NameFromPathSegments(segments []string) (string, error) {
	return catalognamedpaths.NameFromPathSegments(segments)
}

func MapDir(rootDir, name string) (string, error) { return catalognamedpaths.MapDir(rootDir, name) }
