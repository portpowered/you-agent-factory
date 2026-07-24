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

// Required portfolio hold IDs for live Schema CLI and Standardized Providers
// contention. These are non-bypassable and do not seize ownership.
const (
	HoldSchemaCLIPR1262CLIManifestGeneration = "hold.schema-cli.pr-1262.cli-manifest-generation"
	HoldStandardizedProvidersConductor       = "hold.standardized-providers.conductor"
)

// RequiredPortfolioHoldIDs lists live portfolio holds that must appear when the
// inventory includes the complete shared-surface family set.
var RequiredPortfolioHoldIDs = []string{
	HoldSchemaCLIPR1262CLIManifestGeneration,
	HoldStandardizedProvidersConductor,
}

// RequiredPortfolioHoldSpec describes the actionable fields expected for a
// required live portfolio hold.
type RequiredPortfolioHoldSpec struct {
	HoldID              string
	ExternalOwnerSubstr string
	BlockedLaneSubstr   string
	ReleaseSubstr       string
}

// RequiredPortfolioHoldSpecs are the actionable hold contracts for FND-11.
var RequiredPortfolioHoldSpecs = []RequiredPortfolioHoldSpec{
	{
		HoldID:              HoldSchemaCLIPR1262CLIManifestGeneration,
		ExternalOwnerSubstr: "Schema CLI",
		BlockedLaneSubstr:   "PSS-I03",
		ReleaseSubstr:       "1262",
	},
	{
		HoldID:              HoldStandardizedProvidersConductor,
		ExternalOwnerSubstr: "Standardized Providers",
		BlockedLaneSubstr:   "PSS-I02",
		ReleaseSubstr:       "conductor",
	},
}

// ProtectedCompositionArtifactRelPaths are shared OpenAPI/CLI/MCP composition
// artifacts that inventory validation must never mutate. Validation is read-only
// integration-metadata checking; it does not regenerate contracts.
var ProtectedCompositionArtifactRelPaths = []string{
	"api/openapi-main.yaml",
	"api/openapi.yaml",
	"pkg/transports/http/generated/server.gen.go",
	"pkg/transports/http/client/client.gen.go",
	"ui/src/api/generated/openapi.ts",
	"contracts/cli/commands.json",
	"contracts/mcp/tools.json",
}

// Diagnostic is one maintainer-readable validation finding.
type Diagnostic struct {
	Rule     string
	Path     string
	Message  string
	Document string
}
