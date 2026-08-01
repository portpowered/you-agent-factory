package ownershipinventory

import (
	"slices"
	"strings"
)

// CommittedNestedServiceIDs is the closed nested-subservice set from the
// Packaged Service Structure committed target tree.
var CommittedNestedServiceIDs = []string{
	"automations/cron",
	"automations/filesystem_watchers",
	"automations/hosted_sources",
	"automations/reconciliation",
	"automations/script_pollers",
	"factory_definitions/authoring_layout",
	"factory_definitions/catalog",
	"factory_definitions/compilation",
	"factory_definitions/distribution",
	"factory_definitions/invocation_policy",
	"factory_definitions/snapshots_portability",
	"factory_definitions/validation",
	"factory_runtime/checkpoint_recovery",
	"factory_runtime/dispatch_planning",
	"factory_runtime/instance_host",
	"factory_runtime/orchestration",
	"factory_sessions/durable_execution",
	"factory_sessions/identity",
	"factory_sessions/invocation",
	"factory_sessions/live_runtime",
	"factory_sessions/response_stream",
	"factory_sessions/runtime_opening",
	"factory_visualization/activation_lifecycle",
	"factory_visualization/live_view_projection",
	"factory_visualization/response_event_presentation",
	"models/assets",
	"models/catalog",
	"models/inference",
	"models/runtime_host",
	"models/runtime_host/leases",
	"models/runtime_scopes",
	"operator_settings/document",
	"operator_settings/resolution",
	"provider_sessions/codex_reader",
	"provider_sessions/cursor_reader",
	"providers/catalog",
	"providers/execution",
	"recordings/artifacts_export",
	"recordings/canonical_ledger",
	"recordings/projection_query",
	"recordings/recording_lifecycle",
	"recordings/replay",
	"work/admission",
	"work/content_materialization",
	"work/content_staging",
	"work/state_access",
	"workers/runners",
	"workers/runners/agent",
	"workers/runners/hosted",
	"workers/runners/inference",
	"workers/runners/script",
	"workers/runtime_assembly",
	"workers/workstations",
}

// CommittedResponsibilityClusterIDs enumerates required large responsibility
// clusters that stay under an owner without becoming nested services.
var CommittedResponsibilityClusterIDs = []string{
	"automations/time_work_retry_policy",
	"factory_definitions/policy_modules",
	"factory_runtime/engine_tick_subsystems",
	"factory_runtime/orchestration_variants",
	"factory_sessions/request_validation_modules",
	"factory_visualization/sink_codec_adapters",
	"models/effect_adapters",
	"operator_settings/identity_input_modules",
	"provider_sessions/reader_support_modules",
	"providers/native_adapters",
	"recordings/projection_policy",
	"system_initialization/legacy_migration",
	"work/invocation_return_policy",
	"work/lineage_graph_modules",
	"workers/prompting_worktree",
	"workers/runner_registry",
}

// BuildOwnerRationales returns the stable-sorted rationale cards for every
// committed top-level owner and nested private subservice.
func BuildOwnerRationales() []OwnerRationaleCard {
	cards := append([]OwnerRationaleCard(nil), committedOwnerRationales()...)
	slices.SortFunc(cards, func(a, b OwnerRationaleCard) int {
		return strings.Compare(a.ServiceID, b.ServiceID)
	})
	return cards
}

// BuildResponsibilityClusters returns the stable-sorted responsibility-cluster
// inventory derived from the Packaged Service Structure resolved decisions.
func BuildResponsibilityClusters() []ResponsibilityCluster {
	clusters := append([]ResponsibilityCluster(nil), committedResponsibilityClusters()...)
	slices.SortFunc(clusters, func(a, b ResponsibilityCluster) int {
		if cmp := strings.Compare(a.Owner, b.Owner); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ClusterID, b.ClusterID)
	})
	return clusters
}

func committedOwnerRationales() []OwnerRationaleCard {
	out := make([]OwnerRationaleCard, 0, 65)
	out = append(out, committedAutomationsRationales()...)
	out = append(out, committedFactoryDefinitionsRationales()...)
	out = append(out, committedFactoryRuntimeRationales()...)
	out = append(out, committedFactorySessionsRationales()...)
	out = append(out, committedFactoryVisualizationRationales()...)
	out = append(out, committedModelsRationales()...)
	out = append(out, committedOperatorSettingsRationales()...)
	out = append(out, committedProviderSessionsRationales()...)
	out = append(out, committedProvidersRationales()...)
	out = append(out, committedRecordingsRationales()...)
	out = append(out, committedSystemInitializationRationales()...)
	out = append(out, committedWorkRationales()...)
	out = append(out, committedWorkersRationales()...)
	return out
}

func committedResponsibilityClusters() []ResponsibilityCluster {
	return []ResponsibilityCluster{
		cluster("automations", "time_work_retry_policy", "Time-work parsing and retry classification", "Plain internal modules for scheduling/value policy; not nested services."),
		cluster("factory_definitions", "policy_modules", "Decision-envelope and compatibility policy modules", "Stateless Definition policy derivations remain parent-internal modules."),
		cluster("factory_runtime", "engine_tick_subsystems", "Engine tick loop and ordered subsystems", "Plain private implementation sharing engine state; not independently addressable services."),
		cluster("factory_runtime", "orchestration_variants", "Petri and JavaScript orchestration variants", "Internal Orchestration variants sharing Runtime lifecycle/checkpoints; not top-level peers."),
		cluster("factory_sessions", "request_validation_modules", "Request preparation and validation modules", "Stateless normalization/validation remain plain modules under Sessions."),
		cluster("factory_visualization", "sink_codec_adapters", "Sink/codec/source adapters", "Protocol mechanics stay private modules or Visualization-owned transport adapters."),
		cluster("models", "effect_adapters", "Source/cache/process/HTTP effect adapters", "Construction-time effect ports without product authority or root interfaces."),
		cluster("operator_settings", "identity_input_modules", "Identity inventory and input-index parsing", "Pure document discovery/parsing modules without independent authority."),
		cluster("provider_sessions", "reader_support_modules", "Path/blob/token/transcript support modules", "Format implementation details beneath owning readers."),
		cluster("providers", "native_adapters", "Native provider adapters", "Codex/Claude/Cursor/OpenCode/Gemini/Kiro/Pi/Agy variants remain Execution-private modules."),
		cluster("recordings", "projection_policy", "Projection policy helpers", "Pure projection helpers stay under Recordings; Projection is not a top-level peer in this program."),
		cluster("system_initialization", "legacy_migration", "Legacy Factory migration module", "Deletion-only private module, not a nested subservice; removed when input inventory reaches zero."),
		cluster("work", "invocation_return_policy", "Invocation input and return-policy modules", "Deterministic parent-internal transformations exposed only through the Work root."),
		cluster("work", "lineage_graph_modules", "Lineage and dependency-graph modules", "Deterministic projections over Work contracts used by State Access; no separate store."),
		cluster("workers", "prompting_worktree", "Prompting and worktree preparation", "Supporting modules under Workstation Execution without independent lifecycle."),
		cluster("workers", "runner_registry", "Runner contract and registry mechanics", "Private Workers registry backing runner subservices; not a separate product owner."),
	}
}

func topLevel(owner, targetPath, authority, stateStore, lifecycle, consumers, transactionBoundary, failureRecovery string) OwnerRationaleCard {
	return OwnerRationaleCard{
		ServiceID:           owner,
		Owner:               owner,
		Kind:                RationaleKindTopLevel,
		TargetPath:          targetPath,
		Authority:           authority,
		StateStore:          stateStore,
		Lifecycle:           lifecycle,
		Consumers:           consumers,
		TransactionBoundary: transactionBoundary,
		FailureRecovery:     failureRecovery,
	}
}

func nested(serviceID, owner, parent, targetPath, authority, stateStore, lifecycle, consumers, transactionBoundary, failureRecovery string) OwnerRationaleCard {
	return OwnerRationaleCard{
		ServiceID:           serviceID,
		Owner:               owner,
		Kind:                RationaleKindNested,
		ParentServiceID:     parent,
		TargetPath:          targetPath,
		Authority:           authority,
		StateStore:          stateStore,
		Lifecycle:           lifecycle,
		Consumers:           consumers,
		TransactionBoundary: transactionBoundary,
		FailureRecovery:     failureRecovery,
	}
}

func cluster(owner, clusterID, name, note string) ResponsibilityCluster {
	return ResponsibilityCluster{
		Owner:     owner,
		ClusterID: clusterID,
		Name:      name,
		Note:      note,
	}
}
