package factoryeventkinds

import (
	"slices"
	"strings"
	"testing"

	factorycontracts "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// knownRuntimeEmissionAnchors is the maintainer-owned cross-check list of public
// FactoryEvent kinds with an existing runtime emission path. The public
// inventory must cover every anchor; new emission paths must extend both lists.
var knownRuntimeEmissionAnchors = map[factorycontracts.FactoryEventType]string{
	factorycontracts.FactoryEventTypeRunRequest:                    "pkg/factory/events/event_history.go RecordRunRequest",
	factorycontracts.FactoryEventTypeInitialStructureRequest:       "pkg/factory/events/event_history.go RecordInitialStructure",
	factorycontracts.FactoryEventTypeFactoryChange:                 "pkg/factory/events/event_history.go RecordFactoryChange",
	factorycontracts.FactoryEventTypeWorkRequest:                   "pkg/factory/events/event_history.go RecordWorkRequest",
	factorycontracts.FactoryEventTypeRelationshipChangeRequest:     "pkg/factory/events/event_history.go RecordRelationshipChange",
	factorycontracts.FactoryEventTypeDispatchRequest:               "pkg/factory/events/event_history.go RecordWorkstationRequest",
	factorycontracts.FactoryEventTypeDispatchResponse:              "pkg/factory/events/event_history.go RecordWorkstationResponse",
	factorycontracts.FactoryEventTypeFactoryStateResponse:          "pkg/factory/events/event_history.go RecordFactoryStateChange",
	factorycontracts.FactoryEventTypeRunResponse:                   "pkg/factory/events/event_history.go RecordRunResponse",
	factorycontracts.FactoryEventTypeWorkStateChange:               "pkg/factory/events/event_history.go RecordWorkStateChange",
	factorycontracts.FactoryEventTypeInferenceRequest:              "pkg/workers/provider/recording_provider.go",
	factorycontracts.FactoryEventTypeInferenceResponse:             "pkg/workers/provider/recording_provider.go",
	factorycontracts.FactoryEventTypeModelRequest:                  "pkg/service/factory_runtime_state.go",
	factorycontracts.FactoryEventTypeModelResponse:                 "pkg/service/factory_runtime_state.go",
	factorycontracts.FactoryEventTypeScriptRequest:                 "pkg/workers/executor/script.go",
	factorycontracts.FactoryEventTypeScriptResponse:                "pkg/workers/executor/script.go",
	factorycontracts.FactoryEventTypeAgentRunResponse:              "pkg/workers/executor/agentrun/events.go",
	factorycontracts.FactoryEventTypeSessionStarted:                "pkg/factory/events/event_history_session_lifecycle.go RecordSessionStarted",
	factorycontracts.FactoryEventTypeSessionPaused:                 "pkg/factory/events/event_history_session_lifecycle.go RecordSessionPaused",
	factorycontracts.FactoryEventTypeSessionResumed:                "pkg/factory/events/event_history_session_lifecycle.go RecordSessionResumed",
	factorycontracts.FactoryEventTypeSessionResultUpdated:          "pkg/factory/events/event_history_session_lifecycle.go RecordSessionResultUpdated",
	factorycontracts.FactoryEventTypeSessionCompleted:              "pkg/factory/events/event_history_session_lifecycle.go RecordSessionCompleted",
	factorycontracts.FactoryEventTypeSessionLifecycleControl:       "pkg/factory/events/event_history_session_lifecycle.go RecordSessionLifecycleControl",
	factorycontracts.FactoryEventTypeOrchestratorPhaseChanged:      "pkg/factory/events/event_history_orchestrator_progress.go RecordOrchestratorPhaseChanged",
	factorycontracts.FactoryEventTypeOrchestratorCheckpointWritten: "pkg/factory/events/event_history_orchestrator_progress.go RecordOrchestratorCheckpointWritten",
	factorycontracts.FactoryEventTypeDispatchQueued:                "pkg/factory/events/event_history_dispatch_lifecycle.go RecordDispatchQueued",
	factorycontracts.FactoryEventTypeDispatchInterrupted:           "pkg/factory/events/event_history_dispatch_lifecycle.go RecordDispatchInterrupted",
	factorycontracts.FactoryEventTypeDispatchReconciled:            "pkg/factory/events/event_history_dispatch_lifecycle.go RecordDispatchReconciled",
	factorycontracts.FactoryEventTypeArtifactCreated:               "pkg/factory/events/event_history_dispatch_lifecycle.go RecordArtifactCreated",
}

func TestPublicEmittableFactoryEventKinds_CoversKnownRuntimeEmissionAnchors(t *testing.T) {
	inventory := PublicEmittableFactoryEventKinds()
	inventoryByKind := make(map[factorycontracts.FactoryEventType]PublicEmittableKind, len(inventory))
	for _, entry := range inventory {
		if prior, ok := inventoryByKind[entry.Kind]; ok {
			t.Fatalf("duplicate public inventory kind %q: %q and %q", entry.Kind, prior.EmissionEvidence, entry.EmissionEvidence)
		}
		if strings.TrimSpace(entry.EmissionEvidence) == "" {
			t.Fatalf("public inventory kind %q missing emission evidence", entry.Kind)
		}
		inventoryByKind[entry.Kind] = entry
	}

	for kind, anchor := range knownRuntimeEmissionAnchors {
		entry, ok := inventoryByKind[kind]
		if !ok {
			t.Fatalf("known runtime-emitted public kind %q (%s) missing from PublicEmittableFactoryEventKinds inventory", kind, anchor)
		}
		if !strings.Contains(entry.EmissionEvidence, strings.Split(anchor, " ")[0]) {
			t.Fatalf("public inventory evidence for %q = %q, want to reference %q", kind, entry.EmissionEvidence, anchor)
		}
	}

	if len(inventory) != len(knownRuntimeEmissionAnchors) {
		t.Fatalf("public inventory length = %d, want exactly %d known runtime emission anchors", len(inventory), len(knownRuntimeEmissionAnchors))
	}
}

func TestPublicEmittableFactoryEventKinds_IsSortedAndStable(t *testing.T) {
	kinds := PublicEmittableFactoryEventKinds()
	if !slices.IsSortedFunc(kinds, func(a, b PublicEmittableKind) int {
		switch {
		case a.Kind < b.Kind:
			return -1
		case a.Kind > b.Kind:
			return 1
		default:
			return 0
		}
	}) {
		t.Fatal("PublicEmittableFactoryEventKinds must return kinds sorted by FactoryEventType")
	}

	second := PublicEmittableFactoryEventKinds()
	if len(second) != len(kinds) {
		t.Fatalf("inventory length changed between calls: first=%d second=%d", len(kinds), len(second))
	}
	for i := range kinds {
		if kinds[i] != second[i] {
			t.Fatalf("inventory entry[%d] changed between calls: first=%+v second=%+v", i, kinds[i], second[i])
		}
	}
}

func TestExcludedNonPublicFactoryEventKinds_HasEvidenceForEveryEntry(t *testing.T) {
	excluded := ExcludedNonPublicFactoryEventKinds()
	seen := make(map[string]struct{}, len(excluded))
	for _, entry := range excluded {
		if strings.TrimSpace(entry.Name) == "" {
			t.Fatal("excluded inventory entry missing name")
		}
		if strings.TrimSpace(entry.Category) == "" {
			t.Fatalf("excluded kind %q missing category", entry.Name)
		}
		if strings.TrimSpace(entry.Evidence) == "" {
			t.Fatalf("excluded kind %q missing exclusion evidence", entry.Name)
		}
		key := entry.Category + "\x00" + entry.Name
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate excluded inventory entry %q in category %q", entry.Name, entry.Category)
		}
		seen[key] = struct{}{}
	}
}
