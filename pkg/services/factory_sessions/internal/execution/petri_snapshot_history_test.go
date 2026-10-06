package factorysessionexecution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func snapshotFeedbackMutation(id, output string) interfaces.TokenMutationRecord {
	return interfaces.TokenMutationRecord{
		Type: interfaces.MutationMove, TokenID: id, ToPlace: "task:review",
		TransitionID: "review", TransitionReachable: true,
		Token: &workers.Token{ID: id, State: "review", Color: workers.Color{
			WorkID: "work-feedback", WorkTypeID: "task",
			Tags:                    map[string]string{"_last_output": output},
			StructuredResultPresent: true,
		}, History: workers.History{TotalVisits: map[string]int{"process": 32}}},
	}
}

func TestPetriSnapshotHistoryBoundsFeedbackAndRoundTripsLatestState(t *testing.T) {
	t.Parallel()
	output := strings.Repeat("x", 50<<10)
	var firstBytes int
	for _, count := range []int{1, 8, 32} {
		store := &runtimeRecordingStore{}
		service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{Persistence: store})
		for visit := 0; visit < count; visit++ {
			mutation := snapshotFeedbackMutation("token-feedback", output)
			mutation.DispatchID = fmt.Sprintf("dispatch-%02d", visit)
			if err := service.RecordPetriTokenMutations("~default", []interfaces.TokenMutationRecord{mutation}); err != nil {
				t.Fatal(err)
			}
			mutation.Token.Color.Tags["_last_output"] = "caller-modified"
		}
		var snapshot PersistedRuntimeSessionState
		if err := json.Unmarshal(store.payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		loaded := runtimeStateFromPersistedSnapshot(snapshot)
		if len(loaded.petriMutations) != 1 {
			t.Fatalf("N=%d retained %d records", count, len(loaded.petriMutations))
		}
		latest := loaded.petriMutations[0]
		want := snapshotFeedbackMutation("token-feedback", output)
		want.DispatchID = fmt.Sprintf("dispatch-%02d", count-1)
		if !reflect.DeepEqual(latest, want) {
			t.Fatalf("N=%d latest token or metadata changed", count)
		}
		if err := service.persistSessionSnapshot(loaded); err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			firstBytes = len(store.payload)
		}
		if len(store.payload)-firstBytes > 4<<10 {
			t.Fatalf("N=%d bytes=%d baseline=%d", count, len(store.payload), firstBytes)
		}
		t.Logf("N=%d bytes=%d records=%d full_tokens=1", count, len(store.payload), len(snapshot.Records))
	}
}

func TestPetriSnapshotHistoryPreservesBranchesRetirementAndLatestTagDeletion(t *testing.T) {
	t.Parallel()
	old := snapshotFeedbackMutation("retired", "old")
	old.Token.Color.Tags["deleted"] = "old-key"
	latest := snapshotFeedbackMutation("retired", "")
	sibling := snapshotFeedbackMutation("sibling", "live-feedback")
	consume := interfaces.TokenMutationRecord{Type: interfaces.MutationConsume, TokenID: "retired", FromPlace: "task:review"}
	state := runtimeSessionState{petriMutations: []interfaces.TokenMutationRecord{old, latest, consume, sibling},
		events: []json.RawMessage{json.RawMessage(`{"type":"SESSION_STARTED","id":"original","context":{"sequence":1}}`)}}
	before := indexPetriTokenHistory(state.petriMutations, nil)
	compactRuntimePetriHistory(&state)
	if len(state.petriMutations) != 3 || len(state.petriSummaries) != 0 {
		t.Fatalf("branch history = %d mutations, %d summaries", len(state.petriMutations), len(state.petriSummaries))
	}
	encoded := encodePetriMutationSnapshot(t, state.petriMutations, nil)
	var snapshot PersistedRuntimeSessionState
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		t.Fatal(err)
	}
	loaded := runtimeStateFromPersistedSnapshot(snapshot)
	after := indexPetriTokenHistory(loaded.petriMutations, loaded.petriSummaries)
	for _, id := range []string{"retired", "sibling"} {
		if after.factsByTokenID[id].retired != before.factsByTokenID[id].retired ||
			!reflect.DeepEqual(after.factsByTokenID[id].summary, before.factsByTokenID[id].summary) {
			t.Fatalf("recovered branch %q changed", id)
		}
	}
	if !reflect.DeepEqual(loaded.petriMutations[0].Token.Color.Tags, latest.Token.Color.Tags) {
		t.Fatal("old tags resurrected")
	}
	if got := durableRecordsFromRuntimeState(state)[0].CanonicalEvent; !bytes.Equal(got, state.events[0]) {
		t.Fatal("canonical event changed")
	}
}

func TestPetriSnapshotSizeRejectionRetainsPendingLatestAndReportsRecovery(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.DebugLevel)
	store := &runtimeRecordingStore{}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{Persistence: store})
	service.persistenceWarningLogger = zap.New(core)
	small := snapshotFeedbackMutation("token-a", "good")
	if err := service.RecordPetriTokenMutations("~default", []interfaces.TokenMutationRecord{small}); err != nil {
		t.Fatal(err)
	}
	lastGood := append([]byte(nil), store.payload...)
	service.persistedSnapshotMaxBytes = len(lastGood) + 4096
	for visit := 0; visit < 32; visit++ {
		err := service.RecordPetriTokenMutations("~default", []interfaces.TokenMutationRecord{
			snapshotFeedbackMutation("token-a", strings.Repeat("x", 50<<10)),
			snapshotFeedbackMutation("token-b", "pending-sibling"),
		})
		var sizeErr *SnapshotSizeLimitError
		if !errors.As(err, &sizeErr) || sizeErr.SessionID != "~default" {
			t.Fatalf("rejection = %v", err)
		}
	}
	if !bytes.Equal(lastGood, store.payload) || store.saveCalls != 1 {
		t.Fatal("rejection replaced last-good file")
	}
	if len(service.pendingPetriHistory["~default"].mutations) != 2 || len(service.sessions["~default"].petriMutations) != 1 {
		t.Fatal("pending history grew or became visible before saving")
	}
	if _, err := service.GetSession(context.Background(), "~default"); err != nil {
		t.Fatalf("last-good session unreadable: %v", err)
	}
	if err := service.RecordPetriTokenMutations("~default", []interfaces.TokenMutationRecord{small}); err != nil {
		t.Fatal(err)
	}
	if len(service.pendingPetriHistory) != 0 || len(service.sessions["~default"].petriMutations) != 2 {
		t.Fatal("fitting save lost unpublished sibling or retained pending state")
	}
	if logs.FilterMessage("durable Factory Session persistence degraded").Len() != 1 || logs.FilterMessage("durable Factory Session persistence recovered").Len() != 1 {
		t.Fatal("degradation/recovery episode diagnostics missing or repeated")
	}
	degraded := logs.FilterMessage("durable Factory Session persistence degraded").All()[0].ContextMap()
	if degraded["persistence_degraded"] != true || degraded["session_id"] != "~default" {
		t.Fatalf("diagnostic fields = %#v", degraded)
	}
}

func TestPetriSnapshotWriterFaultDoesNotPublishOrCreatePendingState(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{Persistence: store})
	if err := service.RecordPetriTokenMutations("~default", []interfaces.TokenMutationRecord{snapshotFeedbackMutation("token-a", "good")}); err != nil {
		t.Fatal(err)
	}
	lastGood := append([]byte(nil), store.payload...)
	fault := errors.New("atomic writer unavailable")
	store.saveErr = fault
	if err := service.RecordPetriTokenMutations("~default", []interfaces.TokenMutationRecord{snapshotFeedbackMutation("token-a", "bad")}); !errors.Is(err, fault) {
		t.Fatalf("writer failure = %v", err)
	}
	if len(service.pendingPetriHistory) != 0 || !bytes.Equal(lastGood, store.payload) || service.sessions["~default"].petriMutations[0].Token.Color.Tags["_last_output"] != "good" {
		t.Fatal("writer fault changed state")
	}
}

func TestPetriSnapshotCompletionCanSavePreviouslyRejectedTokenState(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{Persistence: store})
	small := snapshotFeedbackMutation("feedback", "last-good")
	if err := service.RecordPetriTokenMutations("~default", []interfaces.TokenMutationRecord{small}); err != nil {
		t.Fatal(err)
	}
	service.persistedSnapshotMaxBytes = len(store.payload) + 4096
	large := snapshotFeedbackMutation("feedback", strings.Repeat("x", 50<<10))
	// Legacy active records have no topology facts. Once the session becomes
	// non-resumable, its existing terminal migration can discard their bodies.
	large.TransitionReachable = false
	var sizeErr *SnapshotSizeLimitError
	if err := service.RecordPetriTokenMutations("~default", []interfaces.TokenMutationRecord{large}); !errors.As(err, &sizeErr) {
		t.Fatalf("oversized mutation = %v", err)
	}
	if err := service.RecordPetriSessionCompletion("~default", PetriSessionCompletion{Status: LifecycleStatusSucceeded}); err != nil {
		t.Fatal(err)
	}
	if len(service.pendingPetriHistory) != 0 || len(service.sessions["~default"].petriSummaries) != 1 || len(service.sessions["~default"].petriMutations) != 0 {
		t.Fatal("non-resumable terminal save did not compact pending state")
	}
}
