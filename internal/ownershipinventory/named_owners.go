package ownershipinventory

import (
	"slices"
	"strings"
)

// RequiredNamedOwners are the PRD-named owners that must be confirmed onto the
// committed 13-owner tree without another discovery or decomposition task.
var RequiredNamedOwners = []string{
	"factory_visualization",
	"operator_settings",
	"provider_sessions",
	"providers",
	"recordings",
	"system_initialization",
}

// NamedOwnerNestedSubservices is the reviewed nested-subservice map for each
// required named owner from the Packaged Service Structure target tree.
var NamedOwnerNestedSubservices = map[string][]string{
	"providers": {
		"providers/catalog",
		"providers/execution",
	},
	"provider_sessions": {
		"provider_sessions/codex_reader",
		"provider_sessions/cursor_reader",
	},
	"operator_settings": {
		"operator_settings/document",
		"operator_settings/resolution",
	},
	"system_initialization": {},
	"factory_visualization": {
		"factory_visualization/activation_lifecycle",
		"factory_visualization/live_view_projection",
		"factory_visualization/response_event_presentation",
	},
	"recordings": {
		"recordings/artifacts_export",
		"recordings/canonical_ledger",
		"recordings/projection_query",
		"recordings/recording_lifecycle",
		"recordings/replay",
	},
}

// BuildNamedOwnerConfirmations returns the stable-sorted named-owner freeze
// that maps PRD-named owners onto the committed tree without reopening discovery.
func BuildNamedOwnerConfirmations() []NamedOwnerConfirmation {
	confirmations := append([]NamedOwnerConfirmation(nil), committedNamedOwnerConfirmations()...)
	slices.SortFunc(confirmations, compareNamedOwnerConfirmations)
	return confirmations
}

func committedNamedOwnerConfirmations() []NamedOwnerConfirmation {
	return []NamedOwnerConfirmation{
		{
			Owner:             "factory_visualization",
			DisplayName:       "Factory Visualization",
			TargetPath:        "pkg/services/factory_visualization",
			Status:            NamedOwnerStatusConfirmed,
			NestedSubservices: slices.Clone(NamedOwnerNestedSubservices["factory_visualization"]),
			Note:              "Retain top-level visualization; nested activation, live view, and presentation maps are committed. Sink/codec adapters stay as responsibility clusters, not alternate owners.",
		},
		{
			Owner:             "operator_settings",
			DisplayName:       "Operator Settings",
			TargetPath:        "pkg/services/operator_settings",
			Status:            NamedOwnerStatusConfirmed,
			NestedSubservices: slices.Clone(NamedOwnerNestedSubservices["operator_settings"]),
			Note:              "Retain one Operator Settings owner; provider settings remain Document/Resolution content, not a competing top-level Provider Settings or Providers catalog service.",
		},
		{
			Owner:             "provider_sessions",
			DisplayName:       "Provider Sessions",
			TargetPath:        "pkg/services/provider_sessions",
			Status:            NamedOwnerStatusConfirmed,
			NestedSubservices: slices.Clone(NamedOwnerNestedSubservices["provider_sessions"]),
			Note:              "Retain read-only Provider Sessions with Codex/Cursor reader nested maps. Does not enumerate or execute providers.",
		},
		{
			Owner:             "providers",
			DisplayName:       "Providers",
			TargetPath:        "pkg/services/providers",
			Status:            NamedOwnerStatusConfirmed,
			NestedSubservices: slices.Clone(NamedOwnerNestedSubservices["providers"]),
			ResidualPackageRules: []ResidualPackageRule{
				{
					PackagePrefix: "pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty",
					Destination:   "providers",
					Disposition:   DispositionMove,
					Note:          "Agy/PTY adapter packages move into Providers native adapters under the singular Providers root.",
				},
				{
					PackagePrefix: "pkg/services/workers/cliprovider",
					Destination:   "providers",
					Disposition:   DispositionMove,
					Note:          "CLI provider registry/enumeration packages move into Providers Catalog; no CLI-specific competing registry remains.",
				},
				{
					PackagePrefix: "pkg/services/providers/internal/services/execution/internal/provider",
					Destination:   "providers",
					Disposition:   DispositionMove,
					Note:          "Workers provider execution/adapter packages move into Providers Execution; catalog and execution stay nested under Providers, not alternate top-level owners.",
				},
				{
					PackagePrefix: "pkg/services/providers/internal/services/execution/internal/provider_test",
					Destination:   "providers",
					Disposition:   DispositionMove,
					Note:          "Provider test/support packages follow the Providers extraction successor.",
				},
			},
			Note: "Add Providers as the singular catalog+execution owner. Capabilities remain Catalog queries; native adapters remain responsibility clusters—not competing provider catalog/execution top-level abstractions.",
		},
		{
			Owner:             "recordings",
			DisplayName:       "Recordings",
			TargetPath:        "pkg/services/recordings",
			Status:            NamedOwnerStatusConfirmed,
			NestedSubservices: slices.Clone(NamedOwnerNestedSubservices["recordings"]),
			Note:              "Retain one Recordings family. Projection/Replay/Artifacts stay nested; they are not promoted to competing top-level owners in this program.",
		},
		{
			Owner:             "system_initialization",
			DisplayName:       "System Bootstrap",
			TargetPath:        "pkg/services/system_initialization",
			Status:            NamedOwnerStatusConfirmed,
			NestedSubservices: slices.Clone(NamedOwnerNestedSubservices["system_initialization"]),
			Note:              "Retain System Bootstrap with no nested subservice. Legacy migration remains a deletion-only responsibility cluster, not a discovery-open nested service.",
		},
	}
}

func compareNamedOwnerConfirmations(a, b NamedOwnerConfirmation) int {
	return strings.Compare(a.Owner, b.Owner)
}

func packageMatchesResidualPrefix(packagePath, prefix string) bool {
	return packagePath == prefix || strings.HasPrefix(packagePath, prefix+"/")
}


