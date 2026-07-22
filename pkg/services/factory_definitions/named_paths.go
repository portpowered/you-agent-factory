package factorydefinitions

import namedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"

// Named Factory path errors remain stable at the Factory Definitions root.
var (
	ErrInvalidName    = namedpaths.ErrInvalidName
	ErrNotFound       = namedpaths.ErrNotFound
	ErrLayoutNotFound = namedpaths.ErrLayoutNotFound
)

// ValidateName validates one canonical named Factory display name.
func ValidateName(name string) error { return namedpaths.ValidateName(name) }

// PathSegments maps a canonical name to its safe hierarchical layout.
func PathSegments(name string) ([]string, error) { return namedpaths.PathSegments(name) }

func NameFromPathSegments(segments []string) (string, error) {
	return namedpaths.NameFromPathSegments(segments)
}

func MapDir(rootDir, name string) (string, error) { return namedpaths.MapDir(rootDir, name) }
