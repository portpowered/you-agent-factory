package service

import (
	"context"
	"errors"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type selectedWorkRuntime struct {
	factory.Service
	read func(context.Context, string) (work.WorkerSessionWork, error)
}

func (r selectedWorkRuntime) ReadWorkerSessionWork(ctx context.Context, id string) (work.WorkerSessionWork, error) {
	return r.read(ctx, id)
}

func TestWorkerSessionWorkRuntimeResolutionDoesNotBindAdmissions(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	for _, sessionID := range []string{"session-a", "session-b"} {
		state.Register(sessionruntime.Registration{SessionID: sessionID, Handle: struct{}{}, Runtime: &factorysessions.LiveRuntime{
			Factory: selectedWorkRuntime{read: func(ctx context.Context, id string) (work.WorkerSessionWork, error) {
				return work.WorkerSessionWork{WorkID: id, Name: sessionID}, nil
			}},
		}})
	}
	assembly := &Assembly{state: state, beforeWorkAdmissionProjectionRegistration: func() { t.Fatal("selected resolution registered admissions") }}
	for _, sessionID := range []string{"session-a", "session-b"} {
		reader, err := assembly.ResolveWorkerSessionWorkRuntime(sessionID)
		if err != nil {
			t.Fatalf("resolve %s: %v", sessionID, err)
		}
		item, err := reader.ReadWorkerSessionWork(context.Background(), "work")
		if err != nil || item.WorkID != "work" || item.Name != sessionID {
			t.Fatalf("selected scope: %#v, %v", item, err)
		}
	}
	if len(assembly.workAdmissions) != 0 {
		t.Fatal("selected read created admission state")
	}
	if _, err := assembly.ResolveWorkerSessionWorkRuntime("missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing scope: %v", err)
	}
}

func TestWorkerSessionWorkRuntimeAdapterPropagatesSelectedFailures(t *testing.T) {
	t.Parallel()
	for _, want := range []error{work.ErrWorkNotFound, context.Canceled, context.DeadlineExceeded} {
		adapter := workRuntimeAdapter{runtime: selectedWorkRuntime{read: func(context.Context, string) (work.WorkerSessionWork, error) { return work.WorkerSessionWork{}, want }}}
		if item, err := adapter.ReadWorkerSessionWork(context.Background(), "work"); !errors.Is(err, want) || item != (work.WorkerSessionWork{}) {
			t.Fatalf("selected failure: %#v, %v", item, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := workRuntimeAdapter{runtime: selectedWorkRuntime{read: func(context.Context, string) (work.WorkerSessionWork, error) {
		t.Fatal("canceled read reached peer")
		return work.WorkerSessionWork{}, nil
	}}}
	if _, err := adapter.ReadWorkerSessionWork(ctx, "work"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := (workRuntimeAdapter{}).ReadWorkerSessionWork(context.Background(), "work"); !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("missing runtime: %v", err)
	}
}
