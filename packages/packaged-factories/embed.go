// Package packagedfactories provides read-only access to the authored source
// files for the first-party Factories shipped with You Agent Factory.
package packagedfactories

import (
	"embed"
	"io/fs"
)

//go:embed factories
var authored embed.FS

// Source returns the embedded, read-only authored source tree.
//
// Paths use forward slashes and are relative to this package, beginning with
// "factories/". Factory catalog, validation, installation, and lifecycle policy
// remain owned by their existing backend services.
func Source() fs.FS {
	return authored
}
