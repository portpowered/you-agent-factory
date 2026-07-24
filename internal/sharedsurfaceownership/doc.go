// Package sharedsurfaceownership validates the Packaged Service Structure
// shared-surface ownership inventory (integration metadata only).
package sharedsurfaceownership

// Canonical repository-relative paths for the checked-in ownership model.
const (
	CanonicalInventoryRelPath = "docs/architecture/packaged-service-structure/shared-surface-ownership.json"
	CanonicalSchemaRelPath    = "docs/architecture/packaged-service-structure/shared-surface-ownership.schema.json"
	CanonicalModelDocRelPath  = "docs/architecture/packaged-service-structure/shared-surface-ownership-model.md"
)

// RequiredPSSI02SurfaceIDs are the shared OpenAPI/HTTP composition surfaces that
// must be inventoried under serial integrator lane PSS-I02.
var RequiredPSSI02SurfaceIDs = []string{
	"openapi.authored.entrypoint-and-fragments",
	"openapi.bundled.output",
	"openapi.generated.go-server-and-client",
	"openapi.generated.typescript-client",
	"http.toplevel.server-route-composition",
}

// RequiredPSSI03SurfaceIDs are the shared CLI composition surfaces that must be
// inventoried under serial integrator lane PSS-I03.
var RequiredPSSI03SurfaceIDs = []string{
	"cli.root.assembly",
	"cli.manifest.cobra-generation-authority",
	"cli.global.help-and-completion-composition",
}

// RequiredPSSI04SurfaceIDs are the shared MCP composition surfaces that must be
// inventoried under serial integrator lane PSS-I04.
var RequiredPSSI04SurfaceIDs = []string{
	"mcp.toplevel.server-composition",
	"mcp.registry.tool-discovery-catalog-composition",
}

// Diagnostic is one maintainer-readable validation finding.
type Diagnostic struct {
	Rule     string
	Path     string
	Message  string
	Document string
}
