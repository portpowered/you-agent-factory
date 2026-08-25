package factorysessionexecution

import (
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

func (*petriCompactionStore) Load(string) ([]byte, error) {
	return nil, errors.New("not found")
}
