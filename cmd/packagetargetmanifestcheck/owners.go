package main

import (
	"fmt"
	"slices"
	"strings"
)

// committedNestedSubservices is the plan's closed nested destination set for
// each product owner. Destinations may only use these <owner>/internal/services/<subservice>
// names; deeper nesting is recorded under the parent subservice destination.
var committedNestedSubservices = map[string][]string{
	"factory_definitions": {
		"catalog",
		"authoring_layout",
		"compilation",
		"validation",
		"snapshots_portability",
		"distribution",
		"invocation_policy",
	},
	"factory_sessions": {
		"identity",
		"live_runtime",
		"durable_execution",
		"invocation",
		"response_stream",
		"runtime_opening",
	},
	"factory_runtime": {
		"orchestration",
		"instance_host",
		"dispatch_planning",
		"checkpoint_recovery",
	},
	"work": {
		"admission",
		"content_staging",
		"content_materialization",
		"state_access",
	},
	"workers": {
		"runtime_assembly",
		"workstations",
		"runners",
	},
	"providers": {
		"catalog",
		"execution",
	},
	"provider_sessions": {
		"codex_reader",
		"cursor_reader",
	},
	"models": {
		"runtime_scopes",
		"catalog",
		"assets",
		"runtime_host",
		"inference",
	},
	"automations": {
		"reconciliation",
		"cron",
		"script_pollers",
		"filesystem_watchers",
		"hosted_sources",
	},
	"recordings": {
		"canonical_ledger",
		"projection_query",
		"recording_lifecycle",
		"replay",
		"artifacts_export",
	},
	"factory_visualization": {
		"activation_lifecycle",
		"live_view_projection",
		"response_event_presentation",
	},
	"operator_settings": {
		"document",
		"resolution",
	},
	"system_initialization": {},
}

func productOwnerSet() map[string]struct{} {
	owners := closedDestinationVocabulary().ProductOwners
	set := make(map[string]struct{}, len(owners))
	for _, owner := range owners {
		set[owner] = struct{}{}
	}
	return set
}

func isCommittedNestedSubservice(owner, subservice string) bool {
	allowed, ok := committedNestedSubservices[owner]
	if !ok {
		return false
	}
	return slices.Contains(allowed, subservice)
}

// mapCommittedOwnerPackage maps one inventory path that belongs to a committed
// product owner (including Providers extraction sources under workers) to its
// plan-tree destination. Returns ok=false for non-owner residuals (edges,
// platform, transports, wire, root, initializer).
func mapCommittedOwnerPackage(packagePath string) (PackageMapping, bool) {
	packagePath = strings.TrimSpace(packagePath)
	if packagePath == "" {
		return PackageMapping{}, false
	}

	if mapping, ok := mapProvidersExtraction(packagePath); ok {
		return mapping, true
	}

	owner, rest, ok := splitServicesOwnerPath(packagePath)
	if !ok {
		return PackageMapping{}, false
	}
	if _, isOwner := productOwnerSet()[owner]; !isOwner {
		return PackageMapping{}, false
	}

	if mapping, ok := mapKnownNestedOwnerPackage(owner, packagePath, rest); ok {
		return mapping, true
	}

	return PackageMapping{
		PackagePath: packagePath,
		Disposition: DispositionRetain,
		Destination: owner,
	}, true
}

func splitServicesOwnerPath(packagePath string) (owner, rest string, ok bool) {
	const prefix = "pkg/services/"
	if !strings.HasPrefix(packagePath, prefix) {
		return "", "", false
	}
	remainder := strings.TrimPrefix(packagePath, prefix)
	parts := strings.SplitN(remainder, "/", 2)
	if parts[0] == "" {
		return "", "", false
	}
	owner = parts[0]
	if len(parts) == 1 {
		return owner, "", true
	}
	return owner, parts[1], true
}

func mapProvidersExtraction(packagePath string) (PackageMapping, bool) {
	switch {
	case packagePath == "pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty",
		packagePath == "pkg/services/providers/internal/services/execution/internal/provider",
		strings.HasPrefix(packagePath, "pkg/services/providers/internal/services/execution/internal/provider/"),
		packagePath == "pkg/services/providers/internal/services/execution/internal/provider_test",
		strings.HasPrefix(packagePath, "pkg/services/providers/internal/services/execution/internal/provider_test/"):
		destination := "providers/internal/services/execution"
		if packagePath == "pkg/services/providers/internal/services/execution/internal/provider/registry" {
			destination = "providers/internal/services/catalog"
		}
		return PackageMapping{
			PackagePath: packagePath,
			Disposition: DispositionMove,
			Destination: destination,
		}, true
	default:
		return PackageMapping{}, false
	}
}

func mapKnownNestedOwnerPackage(owner, packagePath, rest string) (PackageMapping, bool) {
	if rest == "" {
		return PackageMapping{}, false
	}

	if mapping, ok := mapLegacyServiceImplementationPackage(owner, packagePath, rest); ok {
		return mapping, true
	}

	if owner == "factory_definitions" && rest == "internal" {
		return moveOrRetainMapping(packagePath, owner+"/internal", DispositionRetain), true
	}
	if owner == "work" && rest == "internal" {
		return moveOrRetainMapping(packagePath, owner, DispositionRetain), true
	}
	if owner == "workers" && rest == "internal/service" {
		return moveOrRetainMapping(packagePath, owner+"/internal", DispositionMove), true
	}
	if owner == "factory_definitions" && strings.HasPrefix(rest, "internal/lifecycle") {
		return moveOrRetainMapping(packagePath, owner+"/internal", DispositionRetain), true
	}

	// Packages already under the committed private subservice container retain
	// that nested destination unless the subservice is still transitional IMP debt.
	if strings.HasPrefix(rest, "internal/services/") {
		sub := strings.TrimPrefix(rest, "internal/services/")
		subservice, _, _ := strings.Cut(sub, "/")
		if subservice != "" && isCommittedNestedSubservice(owner, subservice) {
			return moveOrRetainMapping(packagePath, owner+"/internal/services/"+subservice, DispositionRetain), true
		}
	}

	if owner == "factory_runtime" && factoryRuntimeCanonicalRetainRest(rest) {
		return moveOrRetainMapping(packagePath, owner, DispositionRetain), true
	}

	destination, ok := nestedOwnerMoveDestination(owner, rest)
	if !ok {
		return PackageMapping{}, false
	}
	return moveOrRetainMapping(packagePath, destination, DispositionMove), true
}

func factoryRuntimeCanonicalRetainRest(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case rest == "internal" || strings.HasPrefix(rest, "internal/host"):
		return true
	case strings.HasPrefix(rest, "internal/testkit"):
		return true
	case strings.HasPrefix(rest, "internal/exhaustiontests"):
		return true
	case strings.HasPrefix(rest, "internal/services/orchestration"):
		return true
	case strings.HasPrefix(rest, "internal/services/instance_host"):
		return true
	case strings.HasPrefix(rest, "internal/services/dispatch_planning"):
		return true
	case strings.HasPrefix(rest, "internal/services/checkpoint_recovery"):
		return true
	default:
		return false
	}
}

func moveOrRetainMapping(packagePath, destination, disposition string) PackageMapping {
	return PackageMapping{
		PackagePath: packagePath,
		Disposition: disposition,
		Destination: destination,
	}
}

// mapLegacyServiceImplementationPackage marks transitional public service/
// implementation facades for fold into owner/internal. Canonical service roots
// allow only wire, internal, and transports child directories.
func mapLegacyServiceImplementationPackage(owner, packagePath, rest string) (PackageMapping, bool) {
	if rest != "service" && !strings.HasPrefix(rest, "service/") {
		return PackageMapping{}, false
	}
	return moveOrRetainMapping(packagePath, owner+"/internal", DispositionMove), true
}

type nestedPathRule struct {
	exact  string
	prefix string
	dest   string
}

func nestedOwnerMoveDestination(owner, rest string) (destination string, ok bool) {
	rules, exists := nestedOwnerMoveRules[owner]
	if !exists {
		return "", false
	}
	for _, rule := range rules {
		if rest == rule.exact || (rule.prefix != "" && strings.HasPrefix(rest, rule.prefix)) {
			return rule.dest, true
		}
	}
	return "", false
}

// nestedOwnerMoveRules encodes plan-tree move destinations for packages that
// are not yet under <owner>/internal/services/<subservice>. Work unexpected
// siblings are confirmed in work-top-level-inventory.md (INV-WORK-TOPLEVEL).
var nestedOwnerMoveRules = map[string][]nestedPathRule{
	"factory_sessions": {
		{exact: "internal/runtimeopening", prefix: "internal/runtimeopening/", dest: "factory_sessions/internal/services/runtime_opening"},
	},
	"factory_runtime": {
		{exact: "internal", dest: "factory_runtime/internal"},
		{exact: "internal/host", prefix: "internal/host/", dest: "factory_runtime/internal"},
		{exact: "javascript", prefix: "javascript/", dest: "factory_runtime/internal/services/orchestration"},
		{prefix: "internal/orchestrators/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "tooling", prefix: "tooling/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "checkpointstore", prefix: "checkpointstore/", dest: "factory_runtime/internal/services/checkpoint_recovery"},
		{exact: "checkpointsummary", prefix: "checkpointsummary/", dest: "factory_runtime/internal/services/checkpoint_recovery"},
		{exact: "build", prefix: "build/", dest: "factory_runtime/internal/services/instance_host"},
		{exact: "internal", dest: "factory_runtime/internal"},
		{exact: "internal/factorystatus", prefix: "internal/factorystatus/", dest: "factory_runtime/internal"},
		{exact: "internal/host", prefix: "internal/host/", dest: "factory_runtime/internal"},
		{exact: "internal/legacysnapshot", prefix: "internal/legacysnapshot/", dest: "factory_runtime/internal"},
		{exact: "internal/rootobservation", prefix: "internal/rootobservation/", dest: "factory_runtime/internal"},
		{exact: "internal/service", prefix: "internal/service/", dest: "factory_runtime/internal"},
		{exact: "testdata", prefix: "testdata/", dest: "factory_runtime/internal"},
		{exact: "context", prefix: "context/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "definitionmapping", prefix: "definitionmapping/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "engine", prefix: "engine/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "metrics", prefix: "metrics/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "orchestrationowner", prefix: "orchestrationowner/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "orchestratorcontract", prefix: "orchestratorcontract/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "replayhooks", prefix: "replayhooks/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "runtime", prefix: "runtime/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "runtimecontract", prefix: "runtimecontract/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "scheduler", prefix: "scheduler/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "state", prefix: "state/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "subsystems", prefix: "subsystems/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "throttle", prefix: "throttle/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "token", prefix: "token/", dest: "factory_runtime/internal/services/orchestration"},
		{exact: "token_transformer", prefix: "token_transformer/", dest: "factory_runtime/internal/services/orchestration"},
	},
	"work": {
		{exact: "internal/contenturl", prefix: "internal/contenturl/", dest: "work/internal"},
		{exact: "internal/invocationreturnpolicy", prefix: "internal/invocationreturnpolicy/", dest: "work/internal"},
		{exact: "internal/lineagegraph", prefix: "internal/lineagegraph/", dest: "work/internal"},
		{exact: "internal/proposalmaterialization", prefix: "internal/proposalmaterialization/", dest: "work/internal"},
		{exact: "internal/requestadmission", prefix: "internal/requestadmission/", dest: "work/internal"},
		{exact: "internal/stateaccessquery", prefix: "internal/stateaccessquery/", dest: "work/internal"},
		{exact: "materialize", prefix: "materialize/", dest: "work/internal/services/content_materialization"},
	},
	"operator_settings": {
		{exact: "testdata", prefix: "testdata/", dest: "operator_settings/internal"},
		{exact: "internal", dest: "operator_settings/internal"},
		{exact: "internal/construct", prefix: "internal/construct/", dest: "operator_settings/internal"},
		{exact: "internal/identityinputinventory", prefix: "internal/identityinputinventory/", dest: "operator_settings/internal"},
		{exact: "internal/service", prefix: "internal/service/", dest: "operator_settings/internal"},
		{exact: "internal/testlink", prefix: "internal/testlink/", dest: "operator_settings/internal"},
		{exact: "internal/testproviders", prefix: "internal/testproviders/", dest: "operator_settings/internal"},
	},
	"workers": {
		{exact: "internal", dest: "workers/internal"},
		{exact: "internal/diagnostics", prefix: "internal/diagnostics/", dest: "workers/internal"},
		{exact: "internal/interface", prefix: "internal/interface/", dest: "workers/internal"},
		{exact: "internal/testhelpers", prefix: "internal/testhelpers/", dest: "workers/internal"},
		{exact: "construction", prefix: "construction/", dest: "workers/internal/services/runtime_assembly"},
		{exact: "prompting", prefix: "prompting/", dest: "workers/internal/services/workstations"},
		{exact: "worktree", prefix: "worktree/", dest: "workers/internal/services/workstations"},
		{exact: "skippermissions", prefix: "skippermissions/", dest: "workers/internal/services/workstations"},
		{exact: "diagnostics", prefix: "diagnostics/", dest: "workers/internal"},
		{exact: "execution", prefix: "execution/", dest: "workers/internal/services/workstations"},
		{exact: "executor", prefix: "executor/", dest: "workers/internal/services/workstations"},
		{exact: "invocation", prefix: "invocation/", dest: "workers/internal/services/workstations"},
		{exact: "process", prefix: "process/", dest: "workers/internal/services/runners"},
		{exact: "runner", prefix: "runner/", dest: "workers/internal/services/runners"},
		{exact: "draftvalidation", prefix: "draftvalidation/", dest: "workers/internal/services/workstations"},
		{exact: "envdiagnostics", prefix: "envdiagnostics/", dest: "workers/internal/services/workstations"},
		{exact: "inferencefailure", prefix: "inferencefailure/", dest: "workers/internal/services/workstations"},
		{exact: "workstationpool", prefix: "workstationpool/", dest: "workers/internal/services/workstations"},
		{exact: "interface", prefix: "interface/", dest: "workers/internal"},
		{exact: "services", prefix: "services/", dest: "workers/internal/services/runners"},
		{exact: "services/hosted_logic", prefix: "services/hosted_logic/", dest: "workers/internal/services/runners"},
		{exact: "services/inference", prefix: "services/inference/", dest: "workers/internal/services/runners"},
		{exact: "services/testing", prefix: "services/testing/", dest: "workers/internal/services/runners"},
		{prefix: "services/", dest: "workers/internal/services/runners"},
	},
	"provider_sessions": {
		{exact: "codex", prefix: "codex/", dest: "provider_sessions/internal/services/codex_reader"},
		{exact: "cursor", prefix: "cursor/", dest: "provider_sessions/internal/services/cursor_reader"},
	},
	"models": {
		{exact: "internal/catalog", prefix: "internal/catalog/", dest: "models/internal/services/catalog"},
		{exact: "internal/assets", prefix: "internal/assets/", dest: "models/internal/services/assets"},
		{exact: "internal/host", prefix: "internal/host/", dest: "models/internal/services/runtime_host"},
		{exact: "internal/inference", prefix: "internal/inference/", dest: "models/internal/services/inference"},
	},
	"recordings": {
		{exact: "internal/artifacts", prefix: "internal/artifacts/", dest: "recordings/internal/services/artifacts_export"},
		{exact: "internal/events", prefix: "internal/events/", dest: "recordings/internal/services/canonical_ledger"},
		{exact: "internal/projections", prefix: "internal/projections/", dest: "recordings/internal/services/projection_query"},
		{exact: "internal/replay", prefix: "internal/replay/", dest: "recordings/internal/services/replay"},
		{exact: "events", prefix: "events/", dest: "recordings/internal/services/canonical_ledger"},
		{exact: "projections", prefix: "projections/", dest: "recordings/internal/services/projection_query"},
		{exact: "replay", prefix: "replay/", dest: "recordings/internal/services/replay"},
		{exact: "artifacts", prefix: "artifacts/", dest: "recordings/internal/services/artifacts_export"},
	},
	"automations": {
		{exact: "timework", prefix: "timework/", dest: "automations/internal/services/cron"},
	},
	"factory_definitions": {
		{exact: "authoredlayout", prefix: "authoredlayout/", dest: "factory_definitions/internal/services/authoring_layout"},
		{exact: "namedfactories", prefix: "namedfactories/", dest: "factory_definitions/internal/services/catalog"},
		{exact: "namedpaths", prefix: "namedpaths/", dest: "factory_definitions/internal/services/catalog"},
		{exact: "persistence", prefix: "persistence/", dest: "factory_definitions/internal/services/catalog"},
		{exact: "resource", prefix: "resource/", dest: "factory_definitions/internal/services/validation"},
		{exact: "definition", prefix: "definition/", dest: "factory_definitions/internal"},
		{exact: "loading", prefix: "loading/", dest: "factory_definitions/internal/services/compilation"},
		{exact: "loadedsource", prefix: "loadedsource/", dest: "factory_definitions/internal/services/compilation"},
		{exact: "validation", prefix: "validation/", dest: "factory_definitions/internal/services/validation"},
		{exact: "snapshotcapture", prefix: "snapshotcapture/", dest: "factory_definitions/internal/services/snapshots_portability"},
		{exact: "portableconfig", prefix: "portableconfig/", dest: "factory_definitions/internal/services/snapshots_portability"},
		{exact: "editable", prefix: "editable/", dest: "factory_definitions/internal/services/snapshots_portability"},
		{exact: "replayconfig", prefix: "replayconfig/", dest: "factory_definitions/internal/services/snapshots_portability"},
		{exact: "packagedinstallation", prefix: "packagedinstallation/", dest: "factory_definitions/internal/services/distribution"},
		{exact: "packages/goal", prefix: "packages/goal/", dest: "factory_definitions/internal/services/invocation_policy"},
		{exact: "packages", prefix: "packages/", dest: "factory_definitions/internal/services/distribution"},
		{exact: "decisionenvelope", prefix: "decisionenvelope/", dest: "factory_definitions/internal/services/invocation_policy"},
		{exact: "invocationinterpolation", prefix: "invocationinterpolation/", dest: "factory_definitions/internal/services/invocation_policy"},
		{exact: "invocationoutput", prefix: "invocationoutput/", dest: "factory_definitions/internal/services/invocation_policy"},
		{exact: "invocationworktype", prefix: "invocationworktype/", dest: "factory_definitions/internal/services/invocation_policy"},
		{exact: "quorumpolicy", prefix: "quorumpolicy/", dest: "factory_definitions/internal/services/invocation_policy"},
		{exact: "workpropagation", prefix: "workpropagation/", dest: "factory_definitions/internal/services/invocation_policy"},
		{exact: "workstationexecution", prefix: "workstationexecution/", dest: "factory_definitions/internal/services/invocation_policy"},
		{exact: "ttsobservability", prefix: "ttsobservability/", dest: "factory_definitions/internal/services/invocation_policy"},
		{exact: "runtimeconfig", prefix: "runtimeconfig/", dest: "factory_definitions/internal"},
		{exact: "namevalue", prefix: "namevalue/", dest: "factory_definitions/internal/services/validation"},
		{exact: "workers", prefix: "workers/", dest: "factory_definitions/internal/services/validation"},
		{exact: "clonetests", prefix: "clonetests/", dest: "factory_definitions/internal"},
		{exact: "systeminitializationtests", prefix: "systeminitializationtests/", dest: "factory_definitions/internal"},
		{prefix: "internal/testcomposition", dest: "factory_definitions/internal"},
	},
}

func buildCommittedOwnerPackages(inventory []string) ([]PackageMapping, error) {
	rows := make([]PackageMapping, 0, len(inventory))
	for _, packagePath := range inventory {
		mapping, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			continue
		}
		rows = append(rows, mapping)
	}
	if err := ensureAllProductOwnersPresent(rows); err != nil {
		return nil, err
	}
	slices.SortFunc(rows, func(a, b PackageMapping) int {
		return strings.Compare(a.PackagePath, b.PackagePath)
	})
	return rows, nil
}

func ensureAllProductOwnersPresent(rows []PackageMapping) error {
	seen := make(map[string]struct{}, len(closedDestinationVocabulary().ProductOwners))
	for _, row := range rows {
		root, _, ok := splitDestination(row.Destination)
		if !ok {
			continue
		}
		if _, isOwner := productOwnerSet()[root]; isOwner {
			seen[root] = struct{}{}
		}
	}
	var missing []string
	for _, owner := range closedDestinationVocabulary().ProductOwners {
		if _, ok := seen[owner]; !ok {
			missing = append(missing, owner)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("committed owner mappings missing destinations for: %s", strings.Join(missing, ", "))
	}
	return nil
}

func mergeOwnerPackageRows(existing []PackageMapping, ownerRows []PackageMapping) []PackageMapping {
	ownerPaths := make(map[string]struct{}, len(ownerRows))
	for _, row := range ownerRows {
		ownerPaths[row.PackagePath] = struct{}{}
	}
	merged := make([]PackageMapping, 0, len(existing)+len(ownerRows))
	for _, row := range existing {
		if _, isOwnerRow := ownerPaths[row.PackagePath]; isOwnerRow {
			continue
		}
		if _, ok := mapCommittedOwnerPackage(row.PackagePath); ok {
			// Drop stale owner rows that are no longer emitted.
			continue
		}
		merged = append(merged, row)
	}
	merged = append(merged, ownerRows...)
	slices.SortFunc(merged, func(a, b PackageMapping) int {
		return strings.Compare(a.PackagePath, b.PackagePath)
	})
	return merged
}
