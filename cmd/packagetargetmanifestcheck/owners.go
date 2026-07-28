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
	case packagePath == "pkg/services/workers/agypty",
		packagePath == "pkg/services/workers/cliprovider",
		packagePath == "pkg/services/workers/provider",
		packagePath == "pkg/services/workers/provider_test",
		strings.HasPrefix(packagePath, "pkg/services/workers/provider/"),
		strings.HasPrefix(packagePath, "pkg/services/workers/provider_test/"):
		destination := "providers/internal/services/execution"
		if packagePath == "pkg/services/workers/provider/registry" {
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

	// Packages already under the committed private subservice container map to
	// that nested destination. Workers retain at the nested plan path; factory
	// definitions keeps catalog/validation canonical retain and marks other
	// committed subservices move until CLN cutover.
	if strings.HasPrefix(rest, "internal/services/") {
		sub := strings.TrimPrefix(rest, "internal/services/")
		subservice, _, _ := strings.Cut(sub, "/")
		if subservice != "" && isCommittedNestedSubservice(owner, subservice) {
			destination := owner + "/internal/services/" + subservice
			disposition := DispositionRetain
			if owner == "factory_definitions" && subservice != "catalog" && subservice != "validation" {
				disposition = DispositionMove
			}
			return moveOrRetainMapping(packagePath, destination, disposition), true
		}
	}

	destination, ok := nestedOwnerMoveDestination(owner, rest)
	if !ok {
		return PackageMapping{}, false
	}
	return moveOrRetainMapping(packagePath, destination, DispositionMove), true
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
// are not yet under <owner>/internal/services/<subservice>.
var nestedOwnerMoveRules = map[string][]nestedPathRule{
	"factory_sessions": {
		{exact: "internal/runtimeopening", prefix: "internal/runtimeopening/", dest: "factory_sessions/internal/services/runtime_opening"},
	},
	"factory_runtime": {
		{exact: "javascript", prefix: "javascript/", dest: "factory_runtime/internal/services/orchestration"},
		{prefix: "internal/orchestrators/", dest: "factory_runtime/internal/services/orchestration"},
		{prefix: "tooling/javascript/", dest: "factory_runtime/internal/services/orchestration"},
	},
	"work": {
		{exact: "materialize", prefix: "materialize/", dest: "work/internal/services/content_materialization"},
	},
	"workers": {
		{exact: "construction", prefix: "construction/", dest: "workers/internal/services/runtime_assembly"},
		{exact: "diagnostics", prefix: "diagnostics/", dest: "workers/internal"},
		{exact: "interface", prefix: "interface/", dest: "workers/internal"},
		{exact: "execution", prefix: "execution/", dest: "workers/internal/services/workstations"},
		{exact: "executor", prefix: "executor/", dest: "workers/internal/services/workstations"},
		{exact: "invocation", prefix: "invocation/", dest: "workers/internal/services/workstations"},
		{exact: "prompting", prefix: "prompting/", dest: "workers/internal/services/workstations"},
		{exact: "skippermissions", prefix: "skippermissions/", dest: "workers/internal/services/workstations"},
		{exact: "worktree", prefix: "worktree/", dest: "workers/internal/services/workstations"},
		{exact: "process", prefix: "process/", dest: "workers/internal/services/runners"},
		{exact: "runner", prefix: "runner/", dest: "workers/internal/services/runners"},
		{exact: "services/hosted_logic", prefix: "services/hosted_logic/", dest: "workers/internal/services/runners"},
		{exact: "services/inference", prefix: "services/inference/", dest: "workers/internal/services/runners"},
		{exact: "services/testing", prefix: "services/testing/", dest: "workers/internal/services/runners"},
		{exact: "services", prefix: "services/", dest: "workers/internal/services/runners"},
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
		{exact: "resource", prefix: "resource/", dest: "factory_definitions/internal/services/catalog"},
		{prefix: "internal/namedfactories", dest: "factory_definitions/internal/services/catalog"},
		{exact: "definition", prefix: "definition/", dest: "factory_definitions/internal/services/compilation"},
		{exact: "loading", prefix: "loading/", dest: "factory_definitions/internal/services/compilation"},
		{exact: "loadedsource", prefix: "loadedsource/", dest: "factory_definitions/internal/services/compilation"},
		{exact: "validation", prefix: "validation/", dest: "factory_definitions/internal/services/validation"},
		{exact: "snapshotcapture", prefix: "snapshotcapture/", dest: "factory_definitions/internal/services/snapshots_portability"},
		{exact: "portableconfig", prefix: "portableconfig/", dest: "factory_definitions/internal/services/snapshots_portability"},
		{exact: "editable", prefix: "editable/", dest: "factory_definitions/internal/services/snapshots_portability"},
		{exact: "packagedinstallation", prefix: "packagedinstallation/", dest: "factory_definitions/internal/services/distribution"},
		{exact: "packages", prefix: "packages/", dest: "factory_definitions/internal/services/distribution"},
		{exact: "decisionenvelope", prefix: "decisionenvelope/", dest: "factory_definitions/internal"},
		{exact: "invocationinterpolation", prefix: "invocationinterpolation/", dest: "factory_definitions/internal"},
		{exact: "invocationoutput", prefix: "invocationoutput/", dest: "factory_definitions/internal"},
		{exact: "invocationworktype", prefix: "invocationworktype/", dest: "factory_definitions/internal"},
		{exact: "quorumpolicy", prefix: "quorumpolicy/", dest: "factory_definitions/internal"},
		{exact: "workpropagation", prefix: "workpropagation/", dest: "factory_definitions/internal"},
		{exact: "workstationexecution", prefix: "workstationexecution/", dest: "factory_definitions/internal"},
		{exact: "ttsobservability", prefix: "ttsobservability/", dest: "factory_definitions/internal"},
		{exact: "runtimeconfig", prefix: "runtimeconfig/", dest: "factory_definitions/internal"},
		{exact: "replayconfig", prefix: "replayconfig/", dest: "factory_definitions/internal"},
		{exact: "contracts", prefix: "contracts/", dest: "factory_definitions/internal"},
		{exact: "workers", prefix: "workers/", dest: "factory_definitions/internal"},
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
