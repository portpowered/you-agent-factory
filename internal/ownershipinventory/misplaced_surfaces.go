package ownershipinventory

import (
	"slices"
	"strings"
)

const (
	MisplacedGuardKindStandard     = "standard"
	MisplacedGuardKindAllowlist    = "allowlist"
	MisplacedGuardKindPackageGuard = "package_guard"
	MisplacedGuardKindBaseline     = "baseline"
	MisplacedGuardKindDiagnostic   = "diagnostic"

	MisplacedConcernProviderInference = "provider_inference"
	MisplacedConcernHostedPolling     = "hosted_polling"

	PublicSurfaceKindBehaviorTest  = "behavior_test"
	PublicSurfaceKindCLI           = "cli"
	PublicSurfaceKindHTTP          = "http"
	PublicSurfaceKindMCP           = "mcp"
	PublicSurfaceKindReplay        = "replay"
	PublicSurfaceKindVisualization = "visualization"

	OwnedRoleKindConstructor     = "constructor"
	OwnedRoleKindDatastore       = "datastore"
	OwnedRoleKindLifecycleRole   = "lifecycle_role"
	OwnedRoleKindProtocolAdapter = "protocol_adapter"
)

// RequiredMisplacedGuardIDs returns the stable-sorted IDs for every normative
// surface that still assigns provider inference or hosted polling to Workers.
func RequiredMisplacedGuardIDs() []string {
	ids := make([]string, 0, len(committedMisplacedGuards()))
	for _, entry := range committedMisplacedGuards() {
		ids = append(ids, entry.ID)
	}
	slices.Sort(ids)
	return ids
}

// RequiredPublicSurfaceIDs returns the stable-sorted IDs for inventoried
// behavior tests and public CLI/HTTP/MCP/replay/visualization surfaces.
func RequiredPublicSurfaceIDs() []string {
	ids := make([]string, 0, len(committedPublicSurfaces()))
	for _, entry := range committedPublicSurfaces() {
		ids = append(ids, entry.ID)
	}
	slices.Sort(ids)
	return ids
}

// RequiredOwnedRoleIDs returns the stable-sorted IDs for inventoried
// constructors, datastores, lifecycle roles, and protocol adapters.
func RequiredOwnedRoleIDs() []string {
	ids := make([]string, 0, len(committedOwnedRoles()))
	for _, entry := range committedOwnedRoles() {
		ids = append(ids, entry.ID)
	}
	slices.Sort(ids)
	return ids
}

// IsKnownDestination reports whether destination is in the closed inventory
// vocabulary (owners, families, Process Edges exception, or deletion queue).
func IsKnownDestination(destination string) bool {
	return isKnownDestination(destination)
}

// BuildMisplacedGuards returns the stable-sorted misplaced-guard burn-down list.
func BuildMisplacedGuards() []MisplacedGuardEntry {
	out := append([]MisplacedGuardEntry(nil), committedMisplacedGuards()...)
	slices.SortFunc(out, compareMisplacedGuards)
	return out
}

// BuildPublicSurfaces returns the stable-sorted public-surface owner map.
func BuildPublicSurfaces() []PublicSurfaceEntry {
	out := append([]PublicSurfaceEntry(nil), committedPublicSurfaces()...)
	slices.SortFunc(out, comparePublicSurfaces)
	return out
}

// BuildOwnedRoles returns the stable-sorted constructor/datastore/lifecycle/
// protocol-adapter ownership map.
func BuildOwnedRoles() []OwnedRoleEntry {
	out := append([]OwnedRoleEntry(nil), committedOwnedRoles()...)
	slices.SortFunc(out, compareOwnedRoles)
	return out
}

func committedMisplacedGuards() []MisplacedGuardEntry {
	return []MisplacedGuardEntry{
		{
			ID:                "allowlist:pkgboundary:inferencecontract-leaf-effect",
			Kind:              MisplacedGuardKindAllowlist,
			SurfacePath:       "cmd/pkgboundarycheck/main.go",
			CurrentOwnerClaim: "workers",
			MisplacedConcern:  MisplacedConcernProviderInference,
			ReplacementOwner:  "providers",
			Note:              "Package-boundary leaf-effect allowlist still treats pkg/services/workers/provider/inferencecontract as the Workers-owned provider inference port.",
		},
		{
			ID:                "baseline:architecture:workers-and-providers-row",
			Kind:              MisplacedGuardKindBaseline,
			SurfacePath:       "docs/architecture/architecture.md",
			CurrentOwnerClaim: "workers",
			MisplacedConcern:  MisplacedConcernProviderInference,
			ReplacementOwner:  "providers",
			Note:              "Architecture ownership table still places provider adapters and inference under the Workers family instead of Providers Execution.",
		},
		{
			ID:                "baseline:architecture:workers-hosted-logic",
			Kind:              MisplacedGuardKindBaseline,
			SurfacePath:       "docs/architecture/architecture.md",
			CurrentOwnerClaim: "workers",
			MisplacedConcern:  MisplacedConcernHostedPolling,
			ReplacementOwner:  "automations",
			Note:              "Architecture ownership table still places hosted logic under Workers; hosted polling/source reconciliation moves to Automation Hosted Sources.",
		},
		{
			ID:                "diagnostic:pkgboundary:retired-hostedworkers",
			Kind:              MisplacedGuardKindDiagnostic,
			SurfacePath:       "cmd/pkgboundarycheck/main.go",
			CurrentOwnerClaim: "workers",
			MisplacedConcern:  MisplacedConcernHostedPolling,
			ReplacementOwner:  "automations",
			Note:              "Retired pkg/hostedworkers diagnostic still directs maintainers to a Workers hosted path instead of Automation Hosted Sources for polling/reconciliation.",
		},
		{
			ID:                "package_guard:pkgboundary:converged-hosted-logic",
			Kind:              MisplacedGuardKindPackageGuard,
			SurfacePath:       "cmd/pkgboundarycheck/main.go",
			CurrentOwnerClaim: "workers",
			MisplacedConcern:  MisplacedConcernHostedPolling,
			ReplacementOwner:  "automations",
			Note:              "convergedServiceSubpackageRoots still assigns pkg/services/workers/services/hosted_logic to workers.",
		},
		{
			ID:                "package_guard:pkgboundary:converged-workers-provider",
			Kind:              MisplacedGuardKindPackageGuard,
			SurfacePath:       "cmd/pkgboundarycheck/main.go",
			CurrentOwnerClaim: "workers",
			MisplacedConcern:  MisplacedConcernProviderInference,
			ReplacementOwner:  "providers",
			Note:              "convergedServiceSubpackageRoots still assigns pkg/services/workers/provider to workers.",
		},
		{
			ID:                "standard:general-backend-standards:hosted-polling",
			Kind:              MisplacedGuardKindStandard,
			SurfacePath:       "docs/internal/standards/code/general-backend-standards.md",
			CurrentOwnerClaim: "workers",
			MisplacedConcern:  MisplacedConcernHostedPolling,
			ReplacementOwner:  "automations",
			Note:              "Normative backend standard still directs retired pkg/hostedworkers imports to pkg/services/workers/services/hosted_logic instead of Automation Hosted Sources.",
		},
		{
			ID:                "standard:general-backend-standards:provider-inference",
			Kind:              MisplacedGuardKindStandard,
			SurfacePath:       "docs/internal/standards/code/general-backend-standards.md",
			CurrentOwnerClaim: "workers",
			MisplacedConcern:  MisplacedConcernProviderInference,
			ReplacementOwner:  "providers",
			Note:              "Normative backend standard still names pkg/services/workers/provider/inferencecontract as the provider inference leaf-effect owner.",
		},
	}
}

func committedPublicSurfaces() []PublicSurfaceEntry {
	return []PublicSurfaceEntry{
		{
			ID:               "behavior_test:provider-functionaltests",
			Kind:             PublicSurfaceKindBehaviorTest,
			SurfacePath:      "pkg/services/workers/provider/functionaltests",
			ReplacementOwner: "providers",
			Note:             "Provider compatibility/behavior tests follow the Providers Execution successor after cutover.",
		},
		{
			ID:               "behavior_test:workers-hosted-poller",
			Kind:             PublicSurfaceKindBehaviorTest,
			SurfacePath:      "pkg/services/workers/services/hosted_logic",
			ReplacementOwner: "automations",
			Note:             "Hosted poller/source behavior coverage under Workers hosted_logic moves with Automation Hosted Sources; Workers retains hosted runner execution tests only.",
		},
		{
			ID:               "cli:docs-workers-provider-topics",
			Kind:             PublicSurfaceKindCLI,
			SurfacePath:      "docs/reference/workers.md",
			ReplacementOwner: "providers",
			Note:             "Provider enumeration/modelProvider CLI docs burn down to Providers Catalog; Workers docs retain selection/retry/runner policy only.",
		},
		{
			ID:               "cli:you-providers-list",
			Kind:             PublicSurfaceKindCLI,
			SurfacePath:      "pkg/transports/cli",
			ReplacementOwner: "providers",
			Note:             "you providers list and provider-facing CLI adapters consume Providers Catalog as the durable owner.",
		},
		{
			ID:               "http:openapi-worker-provider-fields",
			Kind:             PublicSurfaceKindHTTP,
			SurfacePath:      "api/components",
			ReplacementOwner: "providers",
			Note:             "OpenAPI provider identity/capability fields stay transport-shaped but durable domain owner is Providers.",
		},
		{
			ID:               "mcp:factory-session-tools",
			Kind:             PublicSurfaceKindMCP,
			SurfacePath:      "pkg/transports/mcp",
			ReplacementOwner: "factory_sessions",
			Note:             "MCP Factory Session tools adapt Factory Sessions; they are not Workers-owned provider surfaces.",
		},
		{
			ID:               "replay:recordings-replay",
			Kind:             PublicSurfaceKindReplay,
			SurfacePath:      "pkg/services/recordings/wire",
			ReplacementOwner: "recordings",
			Note:             "Replay and artifact reconstruction remain Recordings-owned public surfaces through recordings/wire after DEL-REC removed the transitional replay/ shim.",
		},
		{
			ID:               "visualization:factory-visualization",
			Kind:             PublicSurfaceKindVisualization,
			SurfacePath:      "pkg/services/factory_visualization",
			ReplacementOwner: "factory_visualization",
			Note:             "Dashboard/live-view visualization remains Factory Visualization-owned.",
		},
	}
}

func committedOwnedRoles() []OwnedRoleEntry {
	return []OwnedRoleEntry{
		{ID: "constructor:automations", Kind: OwnedRoleKindConstructor, Name: "Automations service constructor", Destination: "automations", Note: "Wire constructs Automations at the Automations root."},
		{ID: "constructor:factory_definitions", Kind: OwnedRoleKindConstructor, Name: "Factory Definitions service constructor", Destination: "factory_definitions", Note: "Wire constructs Factory Definitions at the Definitions root."},
		{ID: "constructor:factory_runtime", Kind: OwnedRoleKindConstructor, Name: "Factory Runtime service constructor", Destination: "factory_runtime", Note: "Wire constructs Factory Runtime at the Runtime root."},
		{ID: "constructor:factory_sessions", Kind: OwnedRoleKindConstructor, Name: "Factory Sessions service constructor", Destination: "factory_sessions", Note: "Wire constructs Factory Sessions at the Sessions root."},
		{ID: "constructor:factory_visualization", Kind: OwnedRoleKindConstructor, Name: "Factory Visualization service constructor", Destination: "factory_visualization", Note: "Wire constructs Factory Visualization at the Visualization root."},
		{ID: "constructor:models", Kind: OwnedRoleKindConstructor, Name: "Models service constructor", Destination: "models", Note: "Wire constructs Models via models/wire only."},
		{ID: "constructor:operator_settings", Kind: OwnedRoleKindConstructor, Name: "Operator Settings service constructor", Destination: "operator_settings", Note: "Wire constructs Operator Settings at the Operator Settings root."},
		{ID: "constructor:provider_sessions", Kind: OwnedRoleKindConstructor, Name: "Provider Sessions service constructor", Destination: "provider_sessions", Note: "Wire constructs Provider Sessions at the Provider Sessions root."},
		{ID: "constructor:providers", Kind: OwnedRoleKindConstructor, Name: "Providers service constructor", Destination: "providers", Note: "Providers root constructor is the durable destination for Workers provider extraction."},
		{ID: "constructor:recordings", Kind: OwnedRoleKindConstructor, Name: "Recordings service constructor", Destination: "recordings", Note: "Wire constructs Recordings at the Recordings root."},
		{ID: "constructor:system_initialization", Kind: OwnedRoleKindConstructor, Name: "System Bootstrap service constructor", Destination: "system_initialization", Note: "Wire constructs System Bootstrap at the System Bootstrap root."},
		{ID: "constructor:work", Kind: OwnedRoleKindConstructor, Name: "Work service constructor", Destination: "work", Note: "Wire constructs Work at the Work root."},
		{ID: "constructor:workers", Kind: OwnedRoleKindConstructor, Name: "Workers service constructor", Destination: "workers", Note: "Wire constructs Workers at the Workers root for selection/retry/runners only."},
		{ID: "datastore:automationStore", Kind: OwnedRoleKindDatastore, Name: "automationStore", Destination: "automations", Note: "structures.md Automation datastore."},
		{ID: "datastore:definitionStore", Kind: OwnedRoleKindDatastore, Name: "definitionStore", Destination: "factory_definitions", Note: "structures.md Factory Definition datastore."},
		{ID: "datastore:ledgerStore", Kind: OwnedRoleKindDatastore, Name: "ledgerStore", Destination: "recordings", Note: "structures.md Session Ledger datastore."},
		{ID: "datastore:modelStore", Kind: OwnedRoleKindDatastore, Name: "modelStore", Destination: "models", Note: "structures.md Model Runtime datastore."},
		{ID: "datastore:providerSessionStore", Kind: OwnedRoleKindDatastore, Name: "providerSessionStore", Destination: "provider_sessions", Note: "structures.md Provider Session datastore."},
		{ID: "datastore:runtimeStore", Kind: OwnedRoleKindDatastore, Name: "runtimeStore", Destination: "factory_runtime", Note: "structures.md Factory Runtime datastore."},
		{ID: "datastore:sessionStore", Kind: OwnedRoleKindDatastore, Name: "sessionStore", Destination: "factory_sessions", Note: "structures.md Factory Session datastore."},
		{ID: "datastore:workStore", Kind: OwnedRoleKindDatastore, Name: "workStore", Destination: "work", Note: "structures.md Work datastore."},
		{ID: "datastore:workerStore", Kind: OwnedRoleKindDatastore, Name: "workerStore", Destination: "workers", Note: "structures.md Worker Execution datastore."},
		{ID: "lifecycle_role:automations-hosted-sources", Kind: OwnedRoleKindLifecycleRole, Name: "Automation Hosted Sources lifecycle", Destination: "automations", Note: "Hosted polling/source reconciliation lifecycle moves to Automations."},
		{ID: "lifecycle_role:factory-session", Kind: OwnedRoleKindLifecycleRole, Name: "Factory Session lifecycle", Destination: "factory_sessions", Note: "Desired/observed session lifecycle stays Factory Sessions-owned."},
		{ID: "lifecycle_role:initializer", Kind: OwnedRoleKindLifecycleRole, Name: "Initializer process lifecycle", Destination: "initializer", Note: "Process start/stop/join remains Initializer-owned."},
		{ID: "lifecycle_role:provider-session", Kind: OwnedRoleKindLifecycleRole, Name: "Provider Session lifecycle", Destination: "provider_sessions", Note: "Provider session transcript/lifecycle stays Provider Sessions-owned."},
		{ID: "lifecycle_role:providers-execution", Kind: OwnedRoleKindLifecycleRole, Name: "Providers Execution attempt lifecycle", Destination: "providers", Note: "Normalized provider inference attempt lifecycle moves to Providers Execution."},
		{ID: "lifecycle_role:recordings", Kind: OwnedRoleKindLifecycleRole, Name: "Recordings lifecycle", Destination: "recordings", Note: "Ledger/recording lifecycle stays Recordings-owned."},
		{ID: "lifecycle_role:workers-runners", Kind: OwnedRoleKindLifecycleRole, Name: "Workers runner attempt lifecycle", Destination: "workers", Note: "Work selection/retry and runner attempts remain Workers-owned."},
		{ID: "protocol_adapter:cli", Kind: OwnedRoleKindProtocolAdapter, Name: "CLI transport adapter", Destination: "transports", Note: "CLI protocol adapter stays under transports; domain owners remain separate."},
		{ID: "protocol_adapter:http", Kind: OwnedRoleKindProtocolAdapter, Name: "HTTP/REST/SSE transport adapter", Destination: "transports", Note: "HTTP protocol adapter stays under transports."},
		{ID: "protocol_adapter:mcp", Kind: OwnedRoleKindProtocolAdapter, Name: "MCP transport adapter", Destination: "transports", Note: "MCP protocol adapter stays under transports."},
		{ID: "protocol_adapter:visualization-sink", Kind: OwnedRoleKindProtocolAdapter, Name: "Factory Visualization presentation adapter", Destination: "factory_visualization", Note: "Visualization presentation adapters remain Factory Visualization-owned."},
	}
}

func compareMisplacedGuards(a, b MisplacedGuardEntry) int {
	return strings.Compare(a.ID, b.ID)
}

func comparePublicSurfaces(a, b PublicSurfaceEntry) int {
	return strings.Compare(a.ID, b.ID)
}

func compareOwnedRoles(a, b OwnedRoleEntry) int {
	if cmp := strings.Compare(a.Kind, b.Kind); cmp != 0 {
		return cmp
	}
	return strings.Compare(a.ID, b.ID)
}
