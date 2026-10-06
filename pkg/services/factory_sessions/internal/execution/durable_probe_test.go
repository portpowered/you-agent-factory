package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"
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

type durableProbeStore struct {
	snapshot []byte
	failure  error
	reads    int
	writes   int
}

func (store *durableProbeStore) Load(string) ([]byte, error) {
	store.reads++
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
			if _, err := inspectDurableSnapshot(snapshot); err != nil {
				b.Fatal(err)
			}
		}
	})
}
