package factory_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const factoryRuntimeRootPackage = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

var checkpointRootContractTypes = []reflect.Type{
	reflect.TypeOf(factory.CheckpointOutcome("")),
	reflect.TypeOf(factory.Checkpoint{}),
	reflect.TypeOf(factory.CaptureCheckpointRequest{}),
	reflect.TypeOf(factory.CaptureCheckpointResult{}),
	reflect.TypeOf(factory.LoadCheckpointRequest{}),
	reflect.TypeOf(factory.LoadCheckpointResult{}),
	reflect.TypeOf(factory.RestoreCheckpointRequest{}),
	reflect.TypeOf(factory.RestoreCheckpointResult{}),
}

var forbiddenCheckpointPeerImportPrefixes = []string{
	factoryRuntimeRootPackage + "/internal/services/checkpoint_recovery",
}

var checkpointPeerConsumerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/...",
	"github.com/portpowered/infinite-you/pkg/services/automations",
	"github.com/portpowered/infinite-you/pkg/services/recordings",
}

// peerCheckpointRootConsumer depends only on the published Factory Runtime root
// checkpoint vocabulary to recover execution state without CheckpointStore ports
// or Petri/JavaScript checkpoint strategy record types.
type peerCheckpointRootConsumer struct {
	runtime factory.Service
}

func (c peerCheckpointRootConsumer) recoverOpaqueCheckpoint(
	ctx context.Context,
	checkpointID string,
) (factory.Checkpoint, error) {
	captured, err := c.runtime.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: checkpointID})
	if err != nil {
		return factory.Checkpoint{}, err
	}
	if captured.Outcome != factory.CheckpointOutcomeCaptured {
		return factory.Checkpoint{}, factory.ErrCorruptCheckpoint
	}
	loaded, err := c.runtime.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{
		CheckpointID:          checkpointID,
		ExpectedSchemaVersion: captured.Checkpoint.SchemaVersion,
	})
	if err != nil {
		return factory.Checkpoint{}, err
	}
	if loaded.Outcome != factory.CheckpointOutcomeLoaded || !loaded.Compatible {
		return factory.Checkpoint{}, factory.ErrIncompatibleCheckpoint
	}
	restored, err := c.runtime.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{Checkpoint: loaded.Checkpoint})
	if err != nil {
		return factory.Checkpoint{}, err
	}
	if restored.Outcome != factory.CheckpointOutcomeRestored || restored.CheckpointID != checkpointID {
		return factory.Checkpoint{}, factory.ErrCorruptCheckpoint
	}
	return loaded.Checkpoint, nil
}

type checkpointRootPeerFake struct {
	captured factory.Checkpoint
}

func (fake *checkpointRootPeerFake) CaptureCheckpoint(
	_ context.Context,
	req factory.CaptureCheckpointRequest,
) (factory.CaptureCheckpointResult, error) {
	fake.captured = factory.Checkpoint{
		CheckpointID:  req.CheckpointID,
		SchemaVersion: 1,
		StrategyKind:  "runtime",
		Payload:       []byte(`{"factoryState":"PAUSED"}`),
	}
	return factory.CaptureCheckpointResult{
		Outcome:    factory.CheckpointOutcomeCaptured,
		Checkpoint: fake.captured,
	}, nil
}

func (fake *checkpointRootPeerFake) LoadCheckpoint(
	_ context.Context,
	req factory.LoadCheckpointRequest,
) (factory.LoadCheckpointResult, error) {
	if req.CheckpointID == "" || fake.captured.CheckpointID == "" {
		return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
	}
	if req.CheckpointID != fake.captured.CheckpointID {
		return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
	}
	compatible := req.ExpectedSchemaVersion == 0 ||
		req.ExpectedSchemaVersion == fake.captured.SchemaVersion
	return factory.LoadCheckpointResult{
		Outcome:    factory.CheckpointOutcomeLoaded,
		Checkpoint: fake.captured,
		Compatible: compatible,
	}, nil
}

func (fake *checkpointRootPeerFake) RestoreCheckpoint(
	_ context.Context,
	req factory.RestoreCheckpointRequest,
) (factory.RestoreCheckpointResult, error) {
	if req.Checkpoint.CheckpointID == "" || len(req.Checkpoint.Payload) == 0 {
		return factory.RestoreCheckpointResult{}, factory.ErrCorruptCheckpoint
	}
	if req.Checkpoint.SchemaVersion != fake.captured.SchemaVersion {
		return factory.RestoreCheckpointResult{}, factory.ErrIncompatibleCheckpoint
	}
	return factory.RestoreCheckpointResult{
		Outcome:      factory.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
	}, nil
}

func (fake *checkpointRootPeerFake) ControlPause(context.Context, factory.PauseRequest) (factory.PauseResult, error) {
	return factory.PauseResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (fake *checkpointRootPeerFake) ControlResume(context.Context, factory.ResumeRequest) (factory.ResumeResult, error) {
	return factory.ResumeResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (fake *checkpointRootPeerFake) ControlTerminate(context.Context, factory.TerminateRequest) (factory.TerminateResult, error) {
	return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (fake *checkpointRootPeerFake) ControlWaitToComplete(factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	done := make(chan struct{})
	close(done)
	return factory.WaitToCompleteResult{Done: done}
}
func (fake *checkpointRootPeerFake) ControlMoveWork(context.Context, factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	return factory.MoveWorkResult{}, nil
}
func (fake *checkpointRootPeerFake) Observe(context.Context, factory.ObserveRequest) (factory.ObserveResult, error) {
	return factory.ObserveResult{}, nil
}
func (fake *checkpointRootPeerFake) PlanDispatch(context.Context, factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{}, nil
}
func (fake *checkpointRootPeerFake) AcceptDispatchResult(context.Context, factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	return factory.AcceptDispatchResultResult{}, nil
}

func TestCheckpointPeerSurface_PeerRecoversThroughRootContractsOnly(t *testing.T) {
	t.Parallel()

	fake := &checkpointRootPeerFake{}
	peer := peerCheckpointRootConsumer{runtime: fake}
	checkpoint, err := peer.recoverOpaqueCheckpoint(context.Background(), "checkpoint-peer-1")
	if err != nil {
		t.Fatalf("recoverOpaqueCheckpoint() error = %v", err)
	}
	if checkpoint.CheckpointID != "checkpoint-peer-1" ||
		checkpoint.SchemaVersion <= 0 ||
		len(checkpoint.Payload) == 0 {
		t.Fatalf("checkpoint = %#v, want opaque root checkpoint", checkpoint)
	}
}

func TestCheckpointPeerSurface_PeerObservesTypedCheckpointFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want error
		call func(factory.Service) error
	}{
		{
			name: "not found",
			want: factory.ErrCheckpointNotFound,
			call: func(runtime factory.Service) error {
				_, err := runtime.LoadCheckpoint(context.Background(), factory.LoadCheckpointRequest{})
				return err
			},
		},
		{
			name: "corrupt",
			want: factory.ErrCorruptCheckpoint,
			call: func(runtime factory.Service) error {
				_, err := runtime.RestoreCheckpoint(context.Background(), factory.RestoreCheckpointRequest{})
				return err
			},
		},
		{
			name: "incompatible",
			want: factory.ErrIncompatibleCheckpoint,
			call: func(runtime factory.Service) error {
				_, err := runtime.RestoreCheckpoint(context.Background(), factory.RestoreCheckpointRequest{
					Checkpoint: factory.Checkpoint{
						CheckpointID: "checkpoint-peer-1", SchemaVersion: 99, Payload: []byte(`{}`),
					},
				})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &checkpointRootPeerFake{
				captured: factory.Checkpoint{
					CheckpointID: "checkpoint-peer-1", SchemaVersion: 1, Payload: []byte(`{}`),
				},
			}
			if err := test.call(fake); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
