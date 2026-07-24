// Package sharedsurfaceownership validates the Packaged Service Structure
// shared-surface ownership inventory (integration metadata only).
package sharedsurfaceownership

// Canonical repository-relative paths for the checked-in ownership model.
const (
	CanonicalInventoryRelPath = "docs/architecture/packaged-service-structure/shared-surface-ownership.json"
	CanonicalSchemaRelPath    = "docs/architecture/packaged-service-structure/shared-surface-ownership.schema.json"
	CanonicalModelDocRelPath  = "docs/architecture/packaged-service-structure/shared-surface-ownership-model.md"
)

// Diagnostic is one maintainer-readable validation finding.
type Diagnostic struct {
	Rule     string
	Path     string
	Message  string
	Document string
}
