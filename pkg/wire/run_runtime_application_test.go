package wire

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type runtimeApplicationOpenerStub struct {
	open func(context.Context, *factorysessions.RuntimeOpeningRequest) (factorysessionwire.OpenedApplicationRuntime, error)
}

func (stub runtimeApplicationOpenerStub) OpenApplicationRuntime(ctx context.Context, request *factorysessions.RuntimeOpeningRequest) (factorysessionwire.OpenedApplicationRuntime, error) {
	return stub.open(ctx, request)
}

type runtimeApplicationCancelStub struct{}

func (*runtimeApplicationCancelStub) Cancel() {}

type runtimeApplicationProcessStub struct {
	ready <-chan factorysessions.RuntimeHostBinding
}

func (*runtimeApplicationProcessStub) Start(context.Context, context.Context) error { return nil }

func (*runtimeApplicationProcessStub) StartWorkers(context.Context) (factorysessions.RuntimeStop, error) {
	return func(context.Context) error { return nil }, nil
}

func (*runtimeApplicationProcessStub) RunTransport(context.Context, http.Handler) error { return nil }

func (*runtimeApplicationProcessStub) Stop(context.Context) error { return nil }

func (process *runtimeApplicationProcessStub) RuntimeHostReady() <-chan factorysessions.RuntimeHostBinding {
	return process.ready
}

func TestOpenWireApplicationPublishesCancellationToAdapter(t *testing.T) {
	t.Parallel()

	want := &runtimeApplicationCancelStub{}
	var got initializer.InvocationCancellation
	opener := runtimeApplicationOpenerStub{open: func(context.Context, *factorysessions.RuntimeOpeningRequest) (factorysessionwire.OpenedApplicationRuntime, error) {
		return factorysessionwire.OpenedApplicationRuntime{}, nil
	}}
	adapt := runRuntimeAdapter(func(opened factorysessionwire.OpenedApplicationRuntime, _ factorysessions.VisualizationSinkID) (factorysessions.BoundProcessComponents, error) {
		got = opened.Cancellation
		return factorysessions.BoundProcessComponents{}, nil
	})
	plan := factorysessionwire.LifecyclePlanOperation(func(factorysessionwire.LifecyclePlanRequest) (lifecycle.Plan, error) {
		return lifecycle.Plan{}, nil
	})

	if _, err := openWireApplicationWithCancellation(context.Background(), &factorysessions.RuntimeOpeningRequest{}, want, "", opener, adapt, plan); err != nil {
		t.Fatalf("openWireApplicationWithCancellation: %v", err)
	}
	if got != want {
		t.Fatalf("opened cancellation = %p, want %p", got, want)
	}
}

func TestBindWireLiveApplicationFailureClosesOnceWithCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("adapter failed")
	closeErr := errors.New("close failed")
	closes := 0
	opened := factorysessionwire.OpenedApplicationRuntime{}
	opened.Resources.Close = func() error {
		closes++
		return closeErr
	}
	adapt := runRuntimeAdapter(func(factorysessionwire.OpenedApplicationRuntime, factorysessions.VisualizationSinkID) (factorysessions.BoundProcessComponents, error) {
		return factorysessions.BoundProcessComponents{}, cause
	})
	plan := factorysessionwire.LifecyclePlanOperation(func(factorysessionwire.LifecyclePlanRequest) (lifecycle.Plan, error) {
		t.Fatal("plan must not run after adapter failure")
		return lifecycle.Plan{}, nil
	})

	_, err := bindWireLiveApplication(opened, "", adapt, plan)
	if !errors.Is(err, cause) {
		t.Fatalf("bind error = %v, want cause %v", err, cause)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("bind error = %v, want close error %v", err, closeErr)
	}
	if closes != 1 {
		t.Fatalf("close calls = %d, want 1", closes)
	}
}

func TestOpenWireHistoricalReplayBypassesAdapter(t *testing.T) {
	t.Parallel()

	replay := &factorysessions.HistoricalReplayInspection{}
	warnings := []recordings.MetadataMismatchWarning{{Key: "factory", Artifact: "a", Current: "b"}}
	metadata := &recordings.ResumeRecoveryMetadata{SourceRecordingID: "rec-1"}
	process := &runtimeApplicationProcessStub{}
	opener := runtimeApplicationOpenerStub{open: func(context.Context, *factorysessions.RuntimeOpeningRequest) (factorysessionwire.OpenedApplicationRuntime, error) {
		return factorysessionwire.OpenedApplicationRuntime{
			Process:                process,
			HistoricalReplay:       replay,
			ReplayMetadataWarnings: warnings,
			ResumeRecoveryMetadata: metadata,
		}, nil
	}}
	adapt := runRuntimeAdapter(func(factorysessionwire.OpenedApplicationRuntime, factorysessions.VisualizationSinkID) (factorysessions.BoundProcessComponents, error) {
		t.Fatal("adapter must not run for historical replay")
		return factorysessions.BoundProcessComponents{}, nil
	})
	plan := factorysessionwire.LifecyclePlanOperation(func(factorysessionwire.LifecyclePlanRequest) (lifecycle.Plan, error) {
		return lifecycle.Plan{}, nil
	})

	got, err := openWireApplicationWithCancellation(context.Background(), &factorysessions.RuntimeOpeningRequest{}, nil, "", opener, adapt, plan)
	if err != nil {
		t.Fatalf("openWireApplicationWithCancellation: %v", err)
	}
	if got.HistoricalReplay != replay {
		t.Fatalf("historical replay not preserved")
	}
	if len(got.ReplayMetadataWarnings) != 1 || got.ReplayMetadataWarnings[0].Key != "factory" {
		t.Fatalf("replay warnings = %+v, want factory", got.ReplayMetadataWarnings)
	}
	if got.ResumeRecoveryMetadata == nil || got.ResumeRecoveryMetadata.SourceRecordingID != "rec-1" {
		t.Fatalf("resume metadata = %+v, want rec-1", got.ResumeRecoveryMetadata)
	}
	if got.ResumeRecoveryMetadata == metadata {
		t.Fatalf("resume metadata must be cloned")
	}
}

func TestWireRuntimeReadyIsNilWithoutReadinessPort(t *testing.T) {
	t.Parallel()

	if got := wireRuntimeReady(nil); got != nil {
		t.Fatalf("wireRuntimeReady(nil) = %v, want nil", got)
	}
	if got := wireRuntimeReady(&runtimeApplicationProcessStub{}); got != nil {
		t.Fatalf("wireRuntimeReady without channel = %v, want nil", got)
	}
	ready := make(chan factorysessions.RuntimeHostBinding)
	process := &runtimeApplicationProcessStub{ready: ready}
	if got := wireRuntimeReady(process); got != ready {
		t.Fatalf("wireRuntimeReady did not return readiness channel")
	}
}

func TestCloseWireOpenedRuntimePreservesCauseWithoutClose(t *testing.T) {
	t.Parallel()

	cause := errors.New("binding failed")
	if got := closeWireOpenedRuntime(factorysessionwire.OpenedApplicationRuntime{}, cause); !errors.Is(got, cause) {
		t.Fatalf("closeWireOpenedRuntime() = %v, want cause", got)
	}
}
