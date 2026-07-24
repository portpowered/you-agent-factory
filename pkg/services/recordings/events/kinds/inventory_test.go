package factoryeventkinds

import (
	"slices"
	"strings"
	"testing"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// knownRuntimeEmissionAnchors is the maintainer-owned cross-check list of public
// FactoryEvent kinds with an existing runtime emission path. The public
// inventory must cover every anchor; new emission paths must extend both lists.
var knownRuntimeEmissionAnchors = map[factorycontracts.FactoryEventType]string{
	factorycontracts.FactoryEventTypeRunRequest:                    "pkg/services/recordings/events/event_history.go RecordRunRequest",
	factorycontracts.FactoryEventTypeInitialStructureRequest:       "pkg/services/recordings/events/event_history.go RecordInitialStructure",
	factorycontracts.FactoryEventTypeFactoryChange:                 "pkg/services/recordings/events/event_history.go RecordFactoryChange",
	factorycontracts.FactoryEventTypeWorkRequest:                   "pkg/services/recordings/events/event_history.go RecordWorkRequest",
	factorycontracts.FactoryEventTypeRelationshipChangeRequest:     "pkg/services/recordings/events/event_history.go RecordRelationshipChange",
	factorycontracts.FactoryEventTypeDispatchRequest:               "pkg/services/recordings/events/event_history.go RecordWorkstationRequest",
	factorycontracts.FactoryEventTypeDispatchResponse:              "pkg/services/recordings/events/event_history.go RecordWorkstationResponse",
	factorycontracts.FactoryEventTypeFactoryStateResponse:          "pkg/services/recordings/events/event_history.go RecordFactoryStateChange",
	factorycontracts.FactoryEventTypeRunResponse:                   "pkg/services/recordings/events/event_history.go RecordRunResponse",
	factorycontracts.FactoryEventTypeWorkStateChange:               "pkg/services/recordings/events/event_history.go RecordWorkStateChange",
	factorycontracts.FactoryEventTypeInferenceRequest:              "pkg/services/workers/provider/recording_provider.go",
	factorycontracts.FactoryEventTypeInferenceResponse:             "pkg/services/workers/provider/recording_provider.go",
	factorycontracts.FactoryEventTypeModelRequest:                  "pkg/services/workers/execution/recording/model.go",
	factorycontracts.FactoryEventTypeModelResponse:                 "pkg/services/workers/execution/recording/model.go",
	factorycontracts.FactoryEventTypeScriptRequest:                 "pkg/services/workers/executor/script.go",
	factorycontracts.FactoryEventTypeScriptResponse:                "pkg/services/workers/executor/script.go",
	factorycontracts.FactoryEventTypeAgentRunResponse:              "pkg/services/workers/executor/agentrun/events.go",
	factorycontracts.FactoryEventTypeSessionStarted:                "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionStarted",
	factorycontracts.FactoryEventTypeSessionPaused:                 "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionPaused",
	factorycontracts.FactoryEventTypeSessionResumed:                "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionResumed",
	factorycontracts.FactoryEventTypeSessionResultUpdated:          "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionResultUpdated",
	factorycontracts.FactoryEventTypeSessionCompleted:              "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionCompleted",
	factorycontracts.FactoryEventTypeSessionLifecycleControl:       "pkg/services/recordings/events/event_history_session_lifecycle.go RecordSessionLifecycleControl",
	factorycontracts.FactoryEventTypeOrchestratorPhaseChanged:      "pkg/services/recordings/events/event_history_orchestrator_progress.go RecordOrchestratorPhaseChanged",
	factorycontracts.FactoryEventTypeOrchestratorCheckpointWritten: "pkg/services/recordings/events/event_history_orchestrator_progress.go RecordOrchestratorCheckpointWritten",
	factorycontracts.FactoryEventTypeDispatchQueued:                "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordDispatchQueued",
	factorycontracts.FactoryEventTypeDispatchInterrupted:           "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordDispatchInterrupted",
	factorycontracts.FactoryEventTypeDispatchReconciled:            "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordDispatchReconciled",
	factorycontracts.FactoryEventTypeArtifactCreated:               "pkg/services/recordings/events/event_history_dispatch_lifecycle.go RecordArtifactCreated",
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

func TestExcludedNonPublicFactoryEventKinds_IncludesRequiredExclusions(t *testing.T) {
	excluded := ExcludedNonPublicFactoryEventKinds()
	byKey := make(map[string]ExcludedNonPublicKind, len(excluded))
	for _, entry := range excluded {
		byKey[entry.Category+"\x00"+entry.Name] = entry
	}

	for _, required := range RequiredExcludedNonPublicKinds() {
		key := required.Category + "\x00" + required.Name
		if _, ok := byKey[key]; !ok {
			t.Fatalf(
				"required exclusion %q in category %q silently omitted from ExcludedNonPublicFactoryEventKinds",
				required.Name,
				required.Category,
			)
		}
	}
}

func TestExcludedNonPublicFactoryEventKinds_IsSortedAndStable(t *testing.T) {
	excluded := ExcludedNonPublicFactoryEventKinds()
	if !slices.IsSortedFunc(excluded, compareExcludedNonPublicKind) {
		t.Fatal("ExcludedNonPublicFactoryEventKinds must return entries sorted by category then name")
	}

	second := ExcludedNonPublicFactoryEventKinds()
	if !slices.Equal(excluded, second) {
		t.Fatal("ExcludedNonPublicFactoryEventKinds must return an identical ordered list on repeated calls")
	}
}

func TestContractOnlyFactoryEventKinds_IsSortedAndStable(t *testing.T) {
	kinds := ContractOnlyFactoryEventKinds()
	if !slices.IsSortedFunc(kinds, func(a, b ContractOnlyKind) int {
		switch {
		case a.Kind < b.Kind:
			return -1
		case a.Kind > b.Kind:
			return 1
		default:
			return 0
		}
	}) {
		t.Fatal("ContractOnlyFactoryEventKinds must return kinds sorted by FactoryEventType")
	}

	second := ContractOnlyFactoryEventKinds()
	if !slices.Equal(kinds, second) {
		t.Fatal("ContractOnlyFactoryEventKinds must return an identical ordered list on repeated calls")
	}
}

func TestValidateFactoryEventKindInventory_CanonicalInventoriesPass(t *testing.T) {
	if err := ValidateFactoryEventKindInventory(FactoryEventKindInventory{
		PublicEmittable: PublicEmittableFactoryEventKinds(),
		Excluded:        ExcludedNonPublicFactoryEventKinds(),
		ContractOnly:    ContractOnlyFactoryEventKinds(),
	}); err != nil {
		t.Fatalf("canonical Recordings-owned inventories must validate: %v", err)
	}
}

func TestValidateFactoryEventKindInventory_FailsClosed(t *testing.T) {
	canonical := FactoryEventKindInventory{
		PublicEmittable: PublicEmittableFactoryEventKinds(),
		Excluded:        ExcludedNonPublicFactoryEventKinds(),
		ContractOnly:    ContractOnlyFactoryEventKinds(),
	}

	t.Run("empty_public_evidence", func(t *testing.T) {
		input := cloneFactoryEventKindInventory(canonical)
		input.PublicEmittable[0].EmissionEvidence = "   "
		if err := ValidateFactoryEventKindInventory(input); err == nil {
			t.Fatal("expected empty public emission evidence to fail validation")
		}
	})

	t.Run("duplicate_public_kind", func(t *testing.T) {
		input := cloneFactoryEventKindInventory(canonical)
		input.PublicEmittable = append(input.PublicEmittable, input.PublicEmittable[0])
		if err := ValidateFactoryEventKindInventory(input); err == nil {
			t.Fatal("expected duplicate public kind to fail validation")
		}
	})

	t.Run("unsorted_public_kinds", func(t *testing.T) {
		input := cloneFactoryEventKindInventory(canonical)
		if len(input.PublicEmittable) < 2 {
			t.Fatal("need at least two public kinds to prove sort validation")
		}
		input.PublicEmittable[0], input.PublicEmittable[len(input.PublicEmittable)-1] =
			input.PublicEmittable[len(input.PublicEmittable)-1], input.PublicEmittable[0]
		if err := ValidateFactoryEventKindInventory(input); err == nil {
			t.Fatal("expected unsorted public inventory to fail validation")
		}
	})

	t.Run("silently_omitted_required_exclusion", func(t *testing.T) {
		input := cloneFactoryEventKindInventory(canonical)
		filtered := make([]ExcludedNonPublicKind, 0, len(input.Excluded))
		for _, entry := range input.Excluded {
			if entry.Name == "FactoryResponseEvent" {
				continue
			}
			filtered = append(filtered, entry)
		}
		input.Excluded = filtered
		if err := ValidateFactoryEventKindInventory(input); err == nil {
			t.Fatal("expected silently omitted FactoryResponseEvent exclusion to fail validation")
		}
	})

	t.Run("empty_contract_only_evidence", func(t *testing.T) {
		input := cloneFactoryEventKindInventory(canonical)
		if len(input.ContractOnly) == 0 {
			t.Fatal("need at least one contract-only kind to prove evidence validation")
		}
		input.ContractOnly[0].Evidence = ""
		if err := ValidateFactoryEventKindInventory(input); err == nil {
			t.Fatal("expected empty contract-only evidence to fail validation")
		}
	})

	t.Run("unsorted_contract_only_kinds", func(t *testing.T) {
		input := cloneFactoryEventKindInventory(canonical)
		if len(input.ContractOnly) < 2 {
			t.Fatal("need at least two contract-only kinds to prove sort validation")
		}
		input.ContractOnly[0], input.ContractOnly[len(input.ContractOnly)-1] =
			input.ContractOnly[len(input.ContractOnly)-1], input.ContractOnly[0]
		if err := ValidateFactoryEventKindInventory(input); err == nil {
			t.Fatal("expected unsorted contract-only inventory to fail validation")
		}
	})
}

func cloneFactoryEventKindInventory(input FactoryEventKindInventory) FactoryEventKindInventory {
	return FactoryEventKindInventory{
		PublicEmittable: append([]PublicEmittableKind{}, input.PublicEmittable...),
		Excluded:        append([]ExcludedNonPublicKind{}, input.Excluded...),
		ContractOnly:    append([]ContractOnlyKind{}, input.ContractOnly...),
	}
}

func compareExcludedNonPublicKind(a, b ExcludedNonPublicKind) int {
	switch {
	case a.Category < b.Category:
		return -1
	case a.Category > b.Category:
		return 1
	case a.Name < b.Name:
		return -1
	case a.Name > b.Name:
		return 1
	default:
		return 0
	}
}
