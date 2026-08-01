package wire

import catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"

// Named Factory path errors remain stable at the Factory Definitions root.
var (
	ErrInvalidName    = catalogwire.ErrInvalidName
	ErrNotFound       = catalogwire.ErrNotFound
	ErrLayoutNotFound = catalogwire.ErrLayoutNotFound
)

// ValidateName validates one canonical named Factory display name.
func ValidateName(name string) error { return catalogwire.ValidateName(name) }

// PathSegments maps a canonical name to its safe hierarchical layout.
func PathSegments(name string) ([]string, error) { return catalogwire.PathSegments(name) }

func NameFromPathSegments(segments []string) (string, error) {
	return catalogwire.NameFromPathSegments(segments)
}

func MapDir(rootDir, name string) (string, error) { return catalogwire.MapDir(rootDir, name) }
