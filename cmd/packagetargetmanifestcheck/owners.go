package main

import "slices"

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
		"historical_query",
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
	"chat_sessions":         {},
	"events":                {},
}

func productOwnerSet(vocabulary DestinationVocabulary) map[string]struct{} {
	set := make(map[string]struct{}, len(vocabulary.ProductOwners))
	for _, owner := range vocabulary.ProductOwners {
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
