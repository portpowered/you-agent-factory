package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCompactPetriTokenHistory_BoundsFiveHundredAndOneThousandLargeTerminalLifecycles(t *testing.T) {
	const snapshotLimit = 5 * 1024 * 1024
	body := strings.Repeat("large-worker-output-", 1_200)

	for _, count := range []int{500, 1_000} {
		t.Run(strconv.Itoa(count)+"-tokens", func(t *testing.T) {
			mutations := make([]interfaces.TokenMutationRecord, 0, count*3)
			for index := 0; index < count; index++ {
				mutations = append(mutations, largeTerminalTokenMutations(index, body)...)
			}

			before := encodePetriMutationSnapshot(t, mutations, nil)
			if count == 500 && len(before) <= snapshotLimit {
				t.Fatalf("uncompacted 500-token snapshot = %d bytes, want it to exceed %d", len(before), snapshotLimit)
			}

			retained, summaries := compactPetriTokenHistory(mutations, nil)
			after := encodePetriMutationSnapshot(t, retained, summaries)
			if len(after) > snapshotLimit {
				t.Fatalf("compacted %d-token snapshot = %d bytes, want <= %d", count, len(after), snapshotLimit)
			}
			if len(retained) != 0 || len(summaries) != count {
				t.Fatalf("compacted %d tokens into %d mutations and %d summaries, want 0 and %d", count, len(retained), len(summaries), count)
			}
			if strings.Contains(string(after), body) || strings.Contains(string(after), "_last_output") || strings.Contains(string(after), "structured_result") {
				t.Fatalf("compacted snapshot retained worker output fields")
			}
		})
	}
}

func TestCompactPetriTokenHistory_RetainsActiveAndReachableTerminalHistory(t *testing.T) {
	body := "active-output"
	mutations := largeTerminalTokenMutations(1, body)
	mutations[2].TransitionReachable = true

	retained, summaries := compactPetriTokenHistory(mutations, nil)
	if len(retained) != len(mutations) || len(summaries) != 0 {
		t.Fatalf("reachable terminal history = %d mutations, %d summaries, want lossless mutations and no summaries", len(retained), len(summaries))
	}

	mutations[2].Terminal = false
	mutations[2].TransitionReachable = false
	retained, summaries = compactPetriTokenHistory(mutations, nil)
	if len(retained) != len(mutations) || len(summaries) != 0 {
		t.Fatalf("non-terminal history = %d mutations, %d summaries, want lossless mutations and no summaries", len(retained), len(summaries))
	}
}

func TestCompactPetriTokenHistory_CompactsTerminalTokenWhenItIsConsumed(t *testing.T) {
	mutations := largeTerminalTokenMutations(2, "output")
	mutations = append(mutations, interfaces.TokenMutationRecord{
		DispatchID: "dispatch-2", TransitionID: "consume", Outcome: workerexecution.OutcomeAccepted,
		Type: interfaces.MutationConsume, TokenID: "token-2", FromPlace: "task:done",
		Terminal: true,
	})

	retained, summaries := compactPetriTokenHistory(mutations, nil)
	if len(retained) != 0 || len(summaries) != 1 {
		t.Fatalf("consumed terminal history = %d mutations, %d summaries, want 0 and 1", len(retained), len(summaries))
	}
	if !summaries[0].Retired || summaries[0].WorkID != "work-2" || summaries[0].State != "done" {
		t.Fatalf("consumed terminal summary = %#v, want retired work-2 in done", summaries[0])
	}
}

func TestCompactRuntimePetriHistory_MigratesLegacyTerminalSessionButPreservesInterruptedState(t *testing.T) {
	legacy := legacyTerminalTokenMutations(5)

	terminal := runtimeSessionState{session: SessionReadResult{
		OrchestratorKind: interfaces.OrchestratorKindPetri,
		Status:           LifecycleStatusSucceeded,
	}, petriMutations: clonePetriMutations(legacy)}
	compactRuntimePetriHistory(&terminal)
	if len(terminal.petriMutations) != 0 || len(terminal.petriSummaries) != 1 {
		t.Fatalf("legacy terminal history = %d mutations, %d summaries, want 0 and 1", len(terminal.petriMutations), len(terminal.petriSummaries))
	}
	if got := terminal.petriSummaries[0]; got.WorkID != "work-5" || got.State != "done" {
		t.Fatalf("legacy terminal summary = %#v, want work-5 at done", got)
	}

	interrupted := runtimeSessionState{session: SessionReadResult{
		OrchestratorKind: interfaces.OrchestratorKindPetri,
		Status:           LifecycleStatusInterrupted,
	}, petriMutations: clonePetriMutations(legacy)}
	compactRuntimePetriHistory(&interrupted)
	if len(interrupted.petriMutations) != len(legacy) || len(interrupted.petriSummaries) != 0 {
		t.Fatalf("interrupted legacy history = %d mutations, %d summaries, want %d and 0", len(interrupted.petriMutations), len(interrupted.petriSummaries), len(legacy))
	}
}

func TestCompactRuntimePetriHistory_LegacyMigrationLeavesExplicitReachableHistoryLossless(t *testing.T) {
	mutations := largeTerminalTokenMutations(6, "reachable-output")
	mutations[2].TransitionReachable = true
	state := runtimeSessionState{session: SessionReadResult{
		OrchestratorKind: interfaces.OrchestratorKindPetri,
		Status:           LifecycleStatusSucceeded,
	}, petriMutations: mutations}

	compactRuntimePetriHistory(&state)
	if len(state.petriMutations) != len(mutations) || len(state.petriSummaries) != 0 {
		t.Fatalf("reachable terminal history = %d mutations, %d summaries, want %d and 0", len(state.petriMutations), len(state.petriSummaries), len(mutations))
	}
}

func TestLegacyTerminalSnapshot_MigratesOnNextSuccessfulSaveAndReload(t *testing.T) {
	const sessionID = "dur-sess-dddddddddddddddddddddddddddddddd"
	store := &petriCompactionStore{}
	legacy := runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: interfaces.OrchestratorKindPetri,
		},
		result:         ResultReadResult{SessionID: sessionID, SessionStatus: LifecycleStatusSucceeded},
		petriMutations: legacyTerminalTokenMutations(7),
	}
	encoded, err := json.Marshal(persistedSnapshotFromRuntimeState(legacy))
	if err != nil {
		t.Fatalf("marshal legacy snapshot: %v", err)
	}
	if err := store.Save(sessionID, encoded); err != nil {
		t.Fatalf("seed legacy snapshot: %v", err)
	}

	service := &JavaScriptRuntimeService{
		persistence: store,
		sessions:    make(map[string]*runtimeSessionState),
	}
	read, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession legacy snapshot: %v", err)
	}
	if read.Status != LifecycleStatusSucceeded {
		t.Fatalf("legacy session status = %q, want SUCCEEDED", read.Status)
	}
	if err := service.RecordPetriSessionCompletion(sessionID, PetriSessionCompletion{Status: LifecycleStatusSucceeded}); err != nil {
		t.Fatalf("RecordPetriSessionCompletion migration: %v", err)
	}

	var migrated PersistedRuntimeSessionState
	saved, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("load migrated snapshot: %v", err)
	}
	if err := json.Unmarshal(saved, &migrated); err != nil {
		t.Fatalf("decode migrated snapshot: %v", err)
	}
	if len(migrated.Records) != 1 || migrated.Records[0].PetriSummary == nil {
		t.Fatalf("migrated records = %#v, want one Petri summary", migrated.Records)
	}
	if got := migrated.Records[0].PetriSummary; got.WorkID != "work-7" || got.State != "done" {
		t.Fatalf("migrated summary = %#v, want work-7 at done", got)
	}
	if strings.Contains(string(saved), "large-worker-output") || strings.Contains(string(saved), "structured_result") {
		t.Fatal("migrated snapshot retained legacy worker output")
	}
	loaded, err := service.snapshotSessionState(sessionID)
	if err != nil {
		t.Fatalf("snapshot migrated session: %v", err)
	}
	if len(loaded.petriMutations) != 0 || len(loaded.petriSummaries) != 1 {
		t.Fatalf("hot migrated history = %d mutations, %d summaries, want 0 and 1", len(loaded.petriMutations), len(loaded.petriSummaries))
	}
}

func TestLegacyInterruptedSnapshot_RemainsLosslessOnSuccessfulSave(t *testing.T) {
	const sessionID = "dur-sess-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	store := &petriCompactionStore{}
	state := runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusInterrupted,
			OrchestratorKind: interfaces.OrchestratorKindPetri,
		},
		petriMutations: legacyTerminalTokenMutations(8),
	}
	service := &JavaScriptRuntimeService{persistence: store}
	if err := service.persistSessionSnapshot(state); err != nil {
		t.Fatalf("persist interrupted legacy snapshot: %v", err)
	}
	var persisted PersistedRuntimeSessionState
	if err := json.Unmarshal(store.payload, &persisted); err != nil {
		t.Fatalf("decode interrupted snapshot: %v", err)
	}
	var mutationCount int
	for _, record := range persisted.Records {
		if record.Kind == DurableRecordKindPetriTokenMutation {
			mutationCount++
		}
	}
	if mutationCount != len(state.petriMutations) {
		t.Fatalf("interrupted persisted mutation count = %d, want %d", mutationCount, len(state.petriMutations))
	}
}

func TestRecordPetriTokenMutations_PersistsCompactedCandidateAndPublishesAfterSave(t *testing.T) {
	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	store := &petriCompactionStore{}
	service := &JavaScriptRuntimeService{
		persistence: store,
		sessions: map[string]*runtimeSessionState{
			sessionID: {
				session: SessionReadResult{SessionID: sessionID, Status: LifecycleStatusRunning},
			},
		},
	}

	if err := service.RecordPetriTokenMutations(sessionID, largeTerminalTokenMutations(3, "large-output")); err != nil {
		t.Fatalf("RecordPetriTokenMutations: %v", err)
	}
	if store.saveCalls != 1 {
		t.Fatalf("save calls = %d, want 1", store.saveCalls)
	}
	if len(service.sessions[sessionID].petriMutations) != 0 || len(service.sessions[sessionID].petriSummaries) != 1 {
		t.Fatalf("published state = %d mutations, %d summaries, want 0 and 1", len(service.sessions[sessionID].petriMutations), len(service.sessions[sessionID].petriSummaries))
	}
	var persisted PersistedRuntimeSessionState
	if err := json.Unmarshal(store.payload, &persisted); err != nil {
		t.Fatalf("decode persisted compacted state: %v", err)
	}
	if len(persisted.Records) != 1 || persisted.Records[0].PetriSummary == nil || persisted.Records[0].PetriSummary.WorkID != "work-3" {
		t.Fatalf("persisted records = %#v, want work-3 summary", persisted.Records)
	}
}

func TestRecordPetriTokenMutations_SerializesBoundedHistoryAcrossOneThousandLifecycles(t *testing.T) {
	const sessionID = "dur-sess-cccccccccccccccccccccccccccccccc"
	const snapshotLimit = 5 * 1024 * 1024
	body := strings.Repeat("worker-output-", 900)
	store := &petriCompactionStore{}
	service := &JavaScriptRuntimeService{
		persistence: store,
		sessions: map[string]*runtimeSessionState{
			sessionID: {
				session: SessionReadResult{SessionID: sessionID, Status: LifecycleStatusRunning},
			},
		},
	}

	for index := 0; index < 1_000; index++ {
		if err := service.RecordPetriTokenMutations(sessionID, largeTerminalTokenMutations(index, body)); err != nil {
			t.Fatalf("RecordPetriTokenMutations(%d): %v", index, err)
		}
		if index == 499 && len(store.payload) > snapshotLimit {
			t.Fatalf("persisted 500-token snapshot = %d bytes, want <= %d", len(store.payload), snapshotLimit)
		}
	}
	if len(store.payload) > snapshotLimit {
		t.Fatalf("persisted 1,000-token snapshot = %d bytes, want <= %d", len(store.payload), snapshotLimit)
	}
	if len(service.sessions[sessionID].petriMutations) != 0 || len(service.sessions[sessionID].petriSummaries) != 1_000 {
		t.Fatalf("hot history = %d mutations, %d summaries, want 0 and 1,000", len(service.sessions[sessionID].petriMutations), len(service.sessions[sessionID].petriSummaries))
	}
}

func TestRecordPetriTokenMutations_SaveFailureDoesNotPublishCompactedState(t *testing.T) {
	const sessionID = "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	wantErr := errors.New("checkpoint unavailable")
	store := &petriCompactionStore{saveErr: wantErr}
	initial := largeTerminalTokenMutations(4, "large-output")
	service := &JavaScriptRuntimeService{
		persistence: store,
		sessions: map[string]*runtimeSessionState{
			sessionID: {
				session:        SessionReadResult{SessionID: sessionID, Status: LifecycleStatusRunning},
				petriMutations: clonePetriMutations(initial),
			},
		},
	}

	err := service.RecordPetriTokenMutations(sessionID, []interfaces.TokenMutationRecord{initial[2]})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RecordPetriTokenMutations error = %v, want %v", err, wantErr)
	}
	if len(service.sessions[sessionID].petriMutations) != len(initial) || len(service.sessions[sessionID].petriSummaries) != 0 {
		t.Fatalf("state after failed save = %d mutations, %d summaries, want original %d and 0", len(service.sessions[sessionID].petriMutations), len(service.sessions[sessionID].petriSummaries), len(initial))
	}
}

func TestPetriTokenSummary_RoundTripsThroughTaggedDurableHistory(t *testing.T) {
	snapshot := PersistedRuntimeSessionState{Records: []DurableSessionRecord{{
		Kind: DurableRecordKindPetriTokenSummary,
		PetriSummary: &PetriTokenSummary{
			TokenID: "token-summary", WorkID: "work-summary", WorkTypeID: "task",
			PlaceID: "task:done", State: "done", Outcome: workerexecution.OutcomeAccepted,
		},
	}}}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal summary snapshot: %v", err)
	}
	var decoded PersistedRuntimeSessionState
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal summary snapshot: %v", err)
	}
	hydrated := runtimeStateFromPersistedSnapshot(decoded)
	if len(hydrated.petriMutations) != 0 || len(hydrated.petriSummaries) != 1 || hydrated.petriSummaries[0].WorkID != "work-summary" {
		t.Fatalf("hydrated summary state = %d mutations, %#v", len(hydrated.petriMutations), hydrated.petriSummaries)
	}
	resaved := persistedSnapshotFromRuntimeState(hydrated)
	if len(resaved.Records) != 1 || resaved.Records[0].PetriSummary == nil {
		t.Fatalf("resaved summary records = %#v", resaved.Records)
	}
}

func largeTerminalTokenMutations(index int, body string) []interfaces.TokenMutationRecord {
	tokenID := "token-" + itoa(index)
	workID := "work-" + itoa(index)
	token := &workerexecution.Token{
		ID:    tokenID,
		State: "init",
		Color: workerexecution.Color{
			Name:       "large Work",
			RequestID:  "request-" + itoa(index),
			WorkID:     workID,
			WorkTypeID: "task",
			DataType:   workerexecution.DataTypeWork,
			TraceID:    "trace-" + itoa(index),
			Tags:       map[string]string{"_last_output": body},
			Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: body}},
			Payload:    []byte(body),
			StructuredResult: map[string]any{
				"body": body,
			},
			StructuredResultPresent: true,
		},
	}
	return []interfaces.TokenMutationRecord{
		{
			DispatchID: "dispatch-" + itoa(index), TransitionID: "submit", Outcome: workerexecution.OutcomeAccepted,
			Type: interfaces.MutationCreate, TokenID: tokenID, ToPlace: "task:init", Token: token,
		},
		{
			DispatchID: "dispatch-" + itoa(index), TransitionID: "process", Outcome: workerexecution.OutcomeAccepted,
			Type: interfaces.MutationMove, TokenID: tokenID, FromPlace: "task:init", ToPlace: "task:processing",
		},
		{
			DispatchID: "dispatch-" + itoa(index), TransitionID: "process", Outcome: workerexecution.OutcomeAccepted,
			Type: interfaces.MutationMove, TokenID: tokenID, FromPlace: "task:processing", ToPlace: "task:done",
			Terminal: true,
		},
	}
}

func legacyTerminalTokenMutations(index int) []interfaces.TokenMutationRecord {
	mutations := largeTerminalTokenMutations(index, "large-worker-output")
	for mutationIndex := range mutations {
		mutations[mutationIndex].Terminal = false
		mutations[mutationIndex].TransitionReachable = false
	}
	return mutations
}

func encodePetriMutationSnapshot(t *testing.T, mutations []interfaces.TokenMutationRecord, summaries []PetriTokenSummary) []byte {
	t.Helper()
	snapshot := persistedSnapshotFromRuntimeState(runtimeSessionState{
		petriMutations: mutations,
		petriSummaries: summaries,
	})
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal Petri snapshot: %v", err)
	}
	return encoded
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

type petriCompactionStore struct {
	saveCalls int
	saveErr   error
	payload   []byte
}

func (s *petriCompactionStore) Save(_ string, encoded []byte) error {
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.payload = append([]byte(nil), encoded...)
	return nil
}

func (s *petriCompactionStore) Load(string) ([]byte, error) {
	if len(s.payload) == 0 {
		return nil, errors.New("not found")
	}
	return append([]byte(nil), s.payload...), nil
}
