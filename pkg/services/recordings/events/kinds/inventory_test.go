package factoryeventkinds

import (
	"slices"
	"strings"
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// knownRuntimeEmissionAnchors is the maintainer-owned cross-check list of public
// FactoryEvent kinds with an existing runtime emission path. The public
// inventory must cover every anchor; new emission paths must extend both lists.
var knownRuntimeEmissionAnchors = map[recordings.FactoryEventType]string{
	recordings.FactoryEventTypeRunRequest:                    "pkg/services/recordings/events/event_history.go RecordRunRequest",
	recordings.FactoryEventTypeInitialStructureRequest:       "pkg/services/recordings/events/event_history.go RecordInitialStructure",
	recordings.FactoryEventTypeFactoryChange:                 "pkg/services/recordings/events/event_history.go RecordFactoryChange",
	recordings.FactoryEventTypeWorkRequest:                   "pkg/services/recordings/events/event_history.go RecordWorkRequest",
	recordings.FactoryEventTypeRelationshipChangeRequest:     "pkg/services/recordings/events/event_history.go RecordRelationshipChange",
	recordings.FactoryEventTypeDispatchRequest:               "pkg/services/recordings/events/event_history.go RecordWorkstationRequest",
	recordings.FactoryEventTypeDispatchResponse:              "pkg/services/recordings/events/event_history.go RecordWorkstationResponse",
	recordings.FactoryEventTypeFactoryStateResponse:          "pkg/services/recordings/events/event_history.go RecordFactoryStateChange",
	recordings.FactoryEventTypeRunResponse:                   "pkg/services/recordings/events/event_history.go RecordRunResponse",
	recordings.FactoryEventTypeWorkStateChange:               "pkg/services/recordings/events/event_history.go RecordWorkStateChange",
	recordings.FactoryEventTypeInferenceRequest:              "pkg/services/workers/provider/recording_provider.go",
	recordings.FactoryEventTypeInferenceResponse:             "pkg/services/workers/provider/recording_provider.go",
	recordings.FactoryEventTypeModelRequest:                  "pkg/services/workers/execution/recording/model.go",
	recordings.FactoryEventTypeModelResponse:                 "pkg/services/workers/execution/recording/model.go",
	recordings.FactoryEventTypeScriptRequest:                 "pkg/services/workers/executor/script.go",
	recordings.FactoryEventTypeScriptResponse:                "pkg/services/workers/executor/script.go",
	recordings.FactoryEventTypeAgentRunResponse:              "pkg/services/workers/executor/agentrun/events.go",
	recordings.FactoryEventTypeSessionStarted:                "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionStarted",
	recordings.FactoryEventTypeSessionPaused:                 "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionPaused",
	recordings.FactoryEventTypeSessionResumed:                "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionResumed",
	recordings.FactoryEventTypeSessionResultUpdated:          "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionResultUpdated",
	recordings.FactoryEventTypeSessionCompleted:              "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionCompleted",
	recordings.FactoryEventTypeSessionLifecycleControl:       "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionLifecycleControl",
	recordings.FactoryEventTypeOrchestratorPhaseChanged:      "pkg/services/recordings/events/event_history_orchestrator_progress.go RecordOrchestratorPhaseChanged",
	recordings.FactoryEventTypeOrchestratorCheckpointWritten: "pkg/services/recordings/events/event_history_orchestrator_progress.go RecordOrchestratorCheckpointWritten",
	recordings.FactoryEventTypeDispatchQueued:                "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordDispatchQueued",
	recordings.FactoryEventTypeDispatchInterrupted:           "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordDispatchInterrupted",
	recordings.FactoryEventTypeDispatchReconciled:            "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordDispatchReconciled",
	recordings.FactoryEventTypeArtifactCreated:               "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordArtifactCreated",
}

func TestPublicEmittableFactoryEventKinds_CoversKnownRuntimeEmissionAnchors(t *testing.T) {
	inventory := PublicEmittableFactoryEventKinds()
	inventoryByKind := make(map[recordings.FactoryEventType]PublicEmittableKind, len(inventory))
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
