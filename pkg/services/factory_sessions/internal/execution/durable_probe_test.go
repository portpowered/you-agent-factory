package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDurableProbeCanonicalValidation(t *testing.T) {
	t.Parallel()
	cases := []string{
		`{"Session":{"SessionID":"~default"}}`,
		`{"session":{"sessionid":" ~default "},"Events":[{"nested":[1,null,"§ —"]}],"Records":null}`,
		`{"Session":{"SessionID":"~default"},"Records":[{"kind":"canonical_factory_event","canonicalEvent":{"type":"WORK_CREATED"}}]}`,
		`{"Session":{"SessionID":"~default"},"Records":[{"kind":"petri_token_mutation","petriMutation":{}},{"kind":"petri_token_summary","petriSummary":{}},{"kind":"javascript_runtime","javascriptRecord":{}}]}`,
		`{"Session":{"SessionID":"wrong"}}`,
		`{"Session":{"SessionID":"~default"},"Dispatches":{}}`,
		`{"Session":{"SessionID":"~default"},"DispatchStatusTransitions":{"d":[{}]}}`,
		`{"Session":{"SessionID":"~default"},"DispatchJavaScript":{"d":false}}`,
		`{"Session":{"SessionID":"~default"},"SourceContent":3}`,
		`{"Session":{"SessionID":"~default"},"Events":[{"nested":[1,]}]}`,
		`{"Session":{"SessionID":"~default"},"Records":[{"kind":"future_kind"}]}`,
		`{"Session":{"SessionID":"~default"},"Records":[{"kind":"canonical_factory_event","canonicalEvent":{}}]}`,
		`{"Session":{"SessionID":"~default"},"Records":[{"kind":"petri_token_summary","petriSummary":{},"petriMutation":{}}]}`,
		`{"Session":{"SessionID":"~default"},"Records":[{"kind":"petri_token_summary","petriSummary":false}]}`,
		`{"Session":{"SessionID":"~default"},"unknown":{"nested":[true]}}`,
		`{"Session":{"SessionID":"wrong"},"Session":{"SessionID":"~default"},"Session":{}}`,
		`{"Session":{"SessionID":"~default"}} {}`,
		`null`,
		`[]`,
	}
	for _, snapshot := range cases {
		t.Run(snapshot, func(t *testing.T) {
			t.Parallel()
			var canonical PersistedRuntimeSessionState
			decodeErr := json.Unmarshal([]byte(snapshot), &canonical)
			want := decodeErr == nil && strings.TrimSpace(canonical.Session.SessionID) == "~default"
			store := &durableProbeStore{snapshot: []byte(snapshot)}
			service := &JavaScriptRuntimeService{persistence: store}
			got, err := service.HasDurableState(t.Context(), "~default")
			if got != want || (err == nil) != want {
				t.Fatalf("probe = %v, %v; canonical acceptance = %v (%v)", got, err, want, decodeErr)
			}
			if err != nil {
				var resumeErr *ResumeError
				if !errors.As(err, &resumeErr) || resumeErr.Outcome != ResumeOutcomeCorruptedPersistence {
					t.Fatalf("error lost typed corruption outcome: %v", err)
				}
			}
			if len(service.sessions) != 0 || store.writes != 0 || store.reads != 1 {
				t.Fatal("probe hydrated a session or mutated storage")
			}
		})
	}
}

func TestMatchCurrentBoardWorkRejectsChangedFacts(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"matching", "state", "payload", "tags", "request", "relations", "missing work", "empty witness", "canceled read"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			item := work.FactoryWorkItem{ID: "work", WorkTypeID: "task", State: "waiting", DisplayName: "saved", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "§ —"}}, Tags: map[string]string{"keep": "yes"}}
			token := workers.Token{ID: "token", State: item.State, Color: workers.Color{WorkID: item.ID, WorkTypeID: item.WorkTypeID, DataType: workers.DataTypeWork, Name: item.DisplayName, RequestID: "request", Content: item.Content, Payload: []byte("§ —"), Tags: item.Tags, Relations: []work.Relation{{Type: work.RelationType("DEPENDS_ON"), TargetWorkID: "prerequisite", RequiredState: "done"}}}}
			snapshot := PersistedRuntimeSessionState{Session: SessionReadResult{SessionID: "~default"}, Records: []DurableSessionRecord{{Kind: DurableRecordKindPetriTokenMutation, PetriMutation: &factorydefinitions.TokenMutationRecord{Type: factorydefinitions.MutationCreate, Token: &token}}}}
			if name == "empty witness" {
				snapshot.Records = nil
			}
			data, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			store := &durableProbeStore{snapshot: data}
			state := &factorydefinitions.FactoryWorldState{WorkItemsByID: map[string]work.FactoryWorkItem{}, WorkRequestsByID: map[string]factorydefinitions.WorkRequestPayload{"request": {WorkItems: []work.FactoryWorkItem{item}}}, RelationsByWorkID: map[string][]work.FactoryRelation{"work": {{Type: "DEPENDS_ON", TargetWorkID: "prerequisite", RequiredState: "done"}}}}
			switch name {
			case "state":
				item.State = "done"
			case "payload":
				item.Content = []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "altered"}}
			case "tags":
				item.Tags = map[string]string{"keep": "no"}
			case "request":
				delete(state.WorkRequestsByID, "request")
			case "relations":
				state.RelationsByWorkID = nil
			}
			if name != "missing work" {
				state.WorkItemsByID[item.ID] = item
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if name == "canceled read" {
				store.onLoad = cancel
			}
			service := &JavaScriptRuntimeService{persistence: store}
			matched, err := service.MatchCurrentBoardWork(ctx, "~default", state)
			if name == "canceled read" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation=%v", err)
				}
			} else if err != nil || matched != (name == "matching") {
				t.Fatalf("matched=%v error=%v", matched, err)
			}
			if store.writes != 0 || len(service.sessions) != 0 {
				t.Fatal("matching mutated or hydrated durable state")
			}
		})
	}
}

func TestCurrentBoardFactsExcludeOnlySynthesizedPetriLifecycle(t *testing.T) {
	t.Parallel()
	store := &durableProbeStore{snapshot: []byte(`{"Session":{"SessionID":"~default","OrchestratorKind":"PETRI"},"Events":[{"id":"session-started/~default","type":"SESSION_STARTED","payload":{}},{"id":"session-result-updated/~default","type":"SESSION_RESULT_UPDATED","payload":{}},{"id":"recorded-start","type":"SESSION_STARTED","payload":{}},{"id":"work","type":"WORK_REQUEST","payload":{}}]}`)}
	service := &JavaScriptRuntimeService{persistence: store}
	facts, err := service.LoadCurrentBoardFacts(t.Context(), "~default")
	if err != nil || len(facts) != 2 || facts[0].Id != "recorded-start" || facts[1].Id != "work" {
		t.Fatalf("recorded witness=%v, %v", facts, err)
	}
}

func TestHasDurableStateStorageAndCancellation(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{fs.ErrNotExist, errors.New("secret read failure")} {
		store := &durableProbeStore{failure: failure}
		service := &JavaScriptRuntimeService{persistence: store}
		got, err := service.HasDurableState(t.Context(), "~default")
		if got || (err == nil) != errors.Is(failure, fs.ErrNotExist) {
			t.Fatalf("storage failure = %v, %v", got, err)
		}
		if err != nil && strings.Contains(err.Error(), "secret") {
			t.Fatal("probe exposed storage diagnostic")
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := service.HasDurableState(ctx, "~default"); !errors.Is(err, context.Canceled) || store.reads != 1 {
			t.Fatalf("canceled probe read storage: %v", err)
		}
	}
}

func TestHasDurableStateCanceledDuringRead(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		snapshot string
		failure  error
	}{
		{name: "valid", snapshot: `{"Session":{"SessionID":"~default"}}`},
		{name: "corrupt", snapshot: `{"secret":"do not print"`},
		{name: "missing", failure: fs.ErrNotExist},
		{name: "unreadable", failure: errors.New("secret storage failure")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			store := &durableProbeStore{
				snapshot: []byte(test.snapshot), failure: test.failure, onLoad: cancel,
			}
			service := &JavaScriptRuntimeService{persistence: store}
			got, err := service.HasDurableState(ctx, "~default")
			if got || !errors.Is(err, context.Canceled) {
				t.Fatalf("probe canceled during read = %v, %v; want false, context.Canceled", got, err)
			}
			if len(service.sessions) != 0 || store.writes != 0 || store.reads != 1 {
				t.Fatal("canceled probe hydrated a session or mutated storage")
			}
		})
	}
}

func TestDurableProbeCanceledWhileValidatingHistory(t *testing.T) {
	t.Parallel()
	// The context controls cancellation at a validation boundary, without a
	// goroutine or a timing-dependent large fixture. No storage effect is needed.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	controlled := &durableProbeContext{Context: ctx, cancel: cancel, remaining: 5}
	snapshot := []byte(`{"Session":{"SessionID":"~default"},"Records":[
		{"kind":"canonical_factory_event","canonicalEvent":{"type":"WORK_CREATED"}},
		{"kind":"future_kind"}
	]}`)
	id, err := inspectDurableSnapshot(controlled, snapshot)
	if id != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled history validation = %q, %v; want empty identity, context.Canceled", id, err)
	}
}

type durableProbeContext struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (ctx *durableProbeContext) Err() error {
	ctx.remaining--
	if ctx.remaining == 0 {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

type durableProbeStore struct {
	snapshot []byte
	failure  error
	reads    int
	writes   int
	onLoad   func()
}

func (store *durableProbeStore) Load(string) ([]byte, error) {
	store.reads++
	if store.onLoad != nil {
		store.onLoad()
	}
	return store.snapshot, store.failure
}

func (store *durableProbeStore) Save(string, []byte) error {
	store.writes++
	return nil
}

func BenchmarkDurableProbe(b *testing.B) {
	record := `{"kind":"canonical_factory_event","canonicalEvent":{"type":"WORK_CREATED","payload":"` + strings.Repeat("x", 1024) + `"}}`
	snapshot := []byte(`{"Session":{"SessionID":"~default"},"Records":[` + strings.TrimSuffix(strings.Repeat(record+",", 32), ",") + `]}`)
	b.Run("canonical-full-decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var state PersistedRuntimeSessionState
			if err := json.Unmarshal(snapshot, &state); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("discard-history-probe", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := inspectDurableSnapshot(b.Context(), snapshot); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestLoadCurrentBoardFactsReadsCanonicalWitnessWithoutHydration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, snapshot string
		valid          bool
	}{
		{"legacy events", `{"Session":{"SessionID":"~default"},"Events":[{"id":"a","type":"WORK_CREATED","payload":{"text":"§ —"}}]}`, true},
		{"tagged records", `{"Session":{"SessionID":"~default"},"Records":[{"kind":"canonical_factory_event","canonicalEvent":{"id":"a","type":"WORK_CREATED","payload":{"text":"§ —"}}}]}`, true},
		{"foreign identity", `{"Session":{"SessionID":"other"}}`, false},
		{"invalid fact", `{"Session":{"SessionID":"~default"},"Events":[{"secret":"private"}]}`, false},
		{"malformed", `{"secret":`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &durableProbeStore{snapshot: []byte(tc.snapshot)}
			service := &JavaScriptRuntimeService{persistence: store}
			got, err := service.LoadCurrentBoardFacts(t.Context(), "~default")
			if (err == nil) != tc.valid {
				t.Fatalf("witness=%v, %v", got, err)
			}
			if tc.valid && (len(got) != 1 || got[0].Id != "a" || !strings.Contains(string(got[0].Payload), "§ —")) {
				t.Fatalf("lost canonical facts: %#v", got)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("exposed payload")
			}
			if store.reads != 1 || store.writes != 0 || len(service.sessions) != 0 {
				t.Fatal("witness read mutated or hydrated state")
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	store := &durableProbeStore{onLoad: cancel}
	service := &JavaScriptRuntimeService{persistence: store}
	if _, err := service.LoadCurrentBoardFacts(ctx, "~default"); !errors.Is(err, context.Canceled) {
		t.Fatalf("read cancellation=%v", err)
	}
}
