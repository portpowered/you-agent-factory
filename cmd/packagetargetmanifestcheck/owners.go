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
		strings.HasPrefix(packagePath, "pkg/services/workers/provider/"):
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

	// Packages already under the committed private subservice container retain
	// that nested destination.
	if strings.HasPrefix(rest, "internal/services/") {
		sub := strings.TrimPrefix(rest, "internal/services/")
		subservice, _, _ := strings.Cut(sub, "/")
		if subservice != "" && isCommittedNestedSubservice(owner, subservice) {
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionRetain,
				Destination: owner + "/internal/services/" + subservice,
			}, true
		}
	}

	switch owner {
	case "factory_sessions":
		if rest == "internal/runtimeopening" || strings.HasPrefix(rest, "internal/runtimeopening/") {
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "factory_sessions/internal/services/runtime_opening",
			}, true
		}
	case "factory_runtime":
		if rest == "javascript" ||
			strings.HasPrefix(rest, "javascript/") ||
			strings.HasPrefix(rest, "internal/orchestrators/") ||
			strings.HasPrefix(rest, "tooling/javascript/") {
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "factory_runtime/internal/services/orchestration",
			}, true
		}
	case "work":
		if rest == "materialize" || strings.HasPrefix(rest, "materialize/") {
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "work/internal/services/content_materialization",
			}, true
		}
	case "workers":
		switch {
		case rest == "services/hosted_logic" || strings.HasPrefix(rest, "services/hosted_logic/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			}, true
		case rest == "services/inference" || strings.HasPrefix(rest, "services/inference/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			}, true
		case rest == "services/testing" || strings.HasPrefix(rest, "services/testing/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "workers/internal/services/runners",
			}, true
		}
	case "provider_sessions":
		switch {
		case rest == "codex" || strings.HasPrefix(rest, "codex/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "provider_sessions/internal/services/codex_reader",
			}, true
		case rest == "cursor" || strings.HasPrefix(rest, "cursor/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "provider_sessions/internal/services/cursor_reader",
			}, true
		}
	case "models":
		switch {
		case rest == "internal/catalog" || strings.HasPrefix(rest, "internal/catalog/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "models/internal/services/catalog",
			}, true
		case rest == "internal/assets" || strings.HasPrefix(rest, "internal/assets/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "models/internal/services/assets",
			}, true
		case rest == "internal/host" || strings.HasPrefix(rest, "internal/host/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "models/internal/services/runtime_host",
			}, true
		case rest == "internal/inference" || strings.HasPrefix(rest, "internal/inference/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "models/internal/services/inference",
			}, true
		}
	case "recordings":
		switch {
		case rest == "events" || strings.HasPrefix(rest, "events/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "recordings/internal/services/canonical_ledger",
			}, true
		case rest == "projections" || strings.HasPrefix(rest, "projections/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "recordings/internal/services/projection_query",
			}, true
		case rest == "replay" || strings.HasPrefix(rest, "replay/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "recordings/internal/services/replay",
			}, true
		case rest == "artifacts" || strings.HasPrefix(rest, "artifacts/"):
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "recordings/internal/services/artifacts_export",
			}, true
		}
	case "automations":
		if rest == "timework" || strings.HasPrefix(rest, "timework/") {
			return PackageMapping{
				PackagePath: packagePath,
				Disposition: DispositionMove,
				Destination: "automations/internal/services/cron",
			}, true
		}
	}

	return PackageMapping{}, false
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
