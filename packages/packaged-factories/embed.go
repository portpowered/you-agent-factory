// Package packagedfactories provides read-only access to the source and
// generated publication files for the first-party Factories shipped with You
// Agent Factory.
package packagedfactories

import (
	"embed"
	"io/fs"
)

//go:embed factories generated schemas
var packageFiles embed.FS

// Source returns the embedded, read-only authored source tree.
//
// Paths use forward slashes and are relative to this package, beginning with
// "factories/" or "schemas/". Factory catalog, validation, installation, and
// lifecycle policy remain owned by their existing backend services. Bytes
// returned by filesystem reads are detached and may be modified without
// affecting later reads.
func Source() fs.FS {
	return packageFiles
}

// Published returns the embedded, read-only files included in the npm package.
//
// Paths use forward slashes and are relative to this package. The published
// contract includes "factories/", "generated/", and "schemas/". Reading this
// filesystem performs no initialization, persistence, or lifecycle operation.
// Bytes returned by filesystem reads are detached and caller-owned.
func Published() fs.FS {
	return packageFiles
}
