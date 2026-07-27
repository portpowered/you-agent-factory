package service

import (
	"context"
	"encoding/json"
	"io/fs"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestDurableRestartReconstructsPublishedProjections(t *testing.T) {
	t.Parallel()

	store := newRestartMemoryStore()
	clock := restartClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	owner, err := New(newRestartBackedExecution(t, clock, store))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	started, err := owner.StartSync(context.Background(), factorysessions.DurableStartRequest{
		RequestID: "request-restart-owner-001",
		Source: factorysessions.Source{
			Kind: factoryruntime.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &factorysessions.InlineWorkflowSource{
				Dialect:      "you-workflow-v1",
				InlineSource: `return { subject: args.subject };`,
			},
		},
		Args: map[string]any{"subject": "restart"},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.Status != string(factorysessions.LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", started.Status)
	}

	ctx := context.Background()
	sessionID := started.SessionID

	wantSession, err := owner.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession before restart: %v", err)
	}
	wantResult, err := owner.GetResult(ctx, sessionID, factorysessions.ResultRequest{
		Mode: factorysessions.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult before restart: %v", err)
	}
	wantDispatches, err := owner.ListDispatches(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListDispatches before restart: %v", err)
	}
	wantEvents, err := owner.ReadEvents(ctx, sessionID, factorysessions.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents before restart: %v", err)
	}

	restartedOwner, err := New(newRestartBackedExecution(t, clock, store))
	if err != nil {
		t.Fatalf("New restarted owner: %v", err)
	}

	gotSession, err := restartedOwner.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession after restart: %v", err)
	}
	gotResult, err := restartedOwner.GetResult(ctx, sessionID, factorysessions.ResultRequest{
		Mode: factorysessions.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult after restart: %v", err)
	}
	gotDispatches, err := restartedOwner.ListDispatches(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListDispatches after restart: %v", err)
	}
	gotEvents, err := restartedOwner.ReadEvents(ctx, sessionID, factorysessions.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after restart: %v", err)
	}

	if !reflect.DeepEqual(gotSession, wantSession) {
		t.Fatalf("session changed across restart:\ngot  %#v\nwant %#v", gotSession, wantSession)
	}
	assertRestartResultMatches(t, gotResult, wantResult)
	if !reflect.DeepEqual(gotDispatches, wantDispatches) {
		t.Fatalf("dispatches changed across restart:\ngot  %#v\nwant %#v", gotDispatches, wantDispatches)
	}
	assertRestartEventsMatch(t, gotEvents.Events, wantEvents.Events)
}

func assertRestartResultMatches(t *testing.T, got, want factorysessions.ResultReadResult) {
	t.Helper()
	gotCopy, wantCopy := got, want
	gotCopy.PrimaryResult = nil
	wantCopy.PrimaryResult = nil
	if !reflect.DeepEqual(gotCopy, wantCopy) {
		t.Fatalf("result metadata changed across restart:\ngot  %#v\nwant %#v", gotCopy, wantCopy)
	}
	var gotPrimary, wantPrimary any
	if err := json.Unmarshal(got.PrimaryResult, &gotPrimary); err != nil {
		t.Fatalf("decode restarted primary result: %v", err)
	}
	if err := json.Unmarshal(want.PrimaryResult, &wantPrimary); err != nil {
		t.Fatalf("decode live primary result: %v", err)
	}
	if !reflect.DeepEqual(gotPrimary, wantPrimary) {
		t.Fatalf("primary result changed across restart: got %#v want %#v", gotPrimary, wantPrimary)
	}
}

func assertRestartEventsMatch(t *testing.T, got, want []json.RawMessage) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event count changed across restart: got %d want %d", len(got), len(want))
	}
	for index := range want {
		var gotEvent, wantEvent any
		if err := json.Unmarshal(got[index], &gotEvent); err != nil {
			t.Fatalf("decode restarted event %d: %v", index, err)
		}
		if err := json.Unmarshal(want[index], &wantEvent); err != nil {
			t.Fatalf("decode live event %d: %v", index, err)
		}
		if !reflect.DeepEqual(gotEvent, wantEvent) {
			t.Fatalf("event %d changed across restart: got %#v want %#v", index, gotEvent, wantEvent)
		}
	}
}

func newRestartBackedExecution(
	t *testing.T,
	clock restartClock,
	store *restartMemoryStore,
) factorysessions.ExecutionService {
	t.Helper()
	workflows := factoryruntimefixtures.ScriptedJavaScriptWorkflows{}
	return factorysessionexecution.NewJavaScriptRuntimeService(
		t.TempDir(),
		factorysessionexecution.ChildExecutorModeFake,
		nil,
		store,
		clock,
		restartSyncWaitScheduler{},
		checkpointfixtures.CheckpointSummariesFixture{
			BuildResult:  checkpointfixtures.ResumableCheckpointSummaryResult(),
			LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult(),
		},
		workflows,
		workflows,
		workflows,
		nil,
		factoryruntime.JavaScriptWorkerSettings{},
		restartRecordingWriter{},
		func() string { return "dur-sess-restart-owner-aaaaaaaaaaaaaaaaaaaaaaaa" },
		nil, nil, nil,
	)
}

type restartClock struct {
	now time.Time
}

func (c restartClock) Now() time.Time { return c.now }

type restartSyncWaitScheduler struct{}

func (restartSyncWaitScheduler) Now() time.Time { return time.Now() }

func (restartSyncWaitScheduler) After(duration time.Duration) <-chan time.Time {
	return time.After(duration)
}

type restartRecordingWriter struct{}

func (restartRecordingWriter) Write(path string, value recordings.PortableRecording) error {
	if err := recordings.ValidatePortableRecording(value); err != nil {
		return err
	}
	_ = path
	return nil
}

type restartMemoryStore struct {
	mu        sync.Mutex
	snapshots map[string][]byte
}

func newRestartMemoryStore() *restartMemoryStore {
	return &restartMemoryStore{snapshots: make(map[string][]byte)}
}

func (s *restartMemoryStore) Save(sessionID string, encoded []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[sessionID] = append([]byte(nil), encoded...)
	return nil
}

func (s *restartMemoryStore) Load(sessionID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	encoded, ok := s.snapshots[sessionID]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), encoded...), nil
}

var _ runtimepersist.Store = (*restartMemoryStore)(nil)
