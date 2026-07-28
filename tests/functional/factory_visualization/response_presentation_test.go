package factory_visualization_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type rootPresentationTracker struct {
	factoryvisualization.Root
	openCalls atomic.Int32
}

func (tracker *rootPresentationTracker) OpenPresentation(
	ctx context.Context,
	req factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	tracker.openCalls.Add(1)
	return tracker.Root.OpenPresentation(ctx, req)
}

func (tracker *rootPresentationTracker) openCallsCount() int {
	return int(tracker.openCalls.Load())
}

// TestVisualizationResponsePresentationThroughPublicRootAfterLifecycle proves
// presentation sessions are not opened as a side effect of root.BuildProcess
// construction alone and that public Open/PresentProgress/Finalize/Close
// operations yield observable outcomes through the published Root.
func TestVisualizationResponsePresentationThroughPublicRootAfterLifecycle(t *testing.T) {
	t.Parallel()

	tracker := &visualizationEffectTracker{}
	var (
		visualizationRoot factoryvisualization.Root
		rootOnce          sync.Once
	)
	edges := serviceedges.Edges{
		FactoryVisualizationSink: factoryvisualization.SinkFunc(tracker.PresentFactoryView),
		FactoryVisualizationRootObserver: func(root factoryvisualization.Root) {
			rootOnce.Do(func() {
				visualizationRoot = &rootPresentationTracker{Root: root}
			})
		},
	}

	process := support.BuildProcess(t, edges)
	if process == nil {
		t.Fatal("BuildProcess() returned nil process, want inert composition")
	}

	dir := support.ScaffoldFactory(t, visualizationInertFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	defer server.Stop(t)

	support.WaitForStatus(t, server.URL(), 5*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})

	presentationTracker, ok := visualizationRoot.(*rootPresentationTracker)
	if !ok || presentationTracker == nil {
		t.Fatal("FactoryVisualizationRootObserver was not invoked, want composed public Root")
	}
	if presentationTracker.openCallsCount() != 0 {
		t.Fatalf(
			"OpenPresentation calls after runtime host startup = %d, want 0 before explicit presentation",
			presentationTracker.openCallsCount(),
		)
	}

	ctx := context.Background()

	_, err := presentationTracker.OpenPresentation(ctx, factoryvisualization.OpenPresentationRequest{})
	requirePresentationError(
		t,
		err,
		factoryvisualization.PresentationErrorInvalidInput,
		"OpenPresentation with missing parameters",
	)
	if presentationTracker.openCallsCount() != 1 {
		t.Fatalf(
			"OpenPresentation calls after invalid request = %d, want 1 explicit call",
			presentationTracker.openCallsCount(),
		)
	}

	opened, err := presentationTracker.OpenPresentation(ctx, factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryLossless,
	})
	if err != nil {
		t.Fatalf("OpenPresentation lossless: error = %v", err)
	}
	if opened.SessionID == "" {
		t.Fatal("OpenPresentation: SessionID is empty, want observable session identity")
	}
	if opened.Mode != factoryvisualization.PresentationDeliveryLossless {
		t.Fatalf(
			"OpenPresentation mode = %q, want %q",
			opened.Mode,
			factoryvisualization.PresentationDeliveryLossless,
		)
	}

	progress, err := presentationTracker.PresentProgress(ctx, factoryvisualization.PresentProgressRequest{
		SessionID: opened.SessionID,
		Records: []factoryvisualization.ProgressRecord{
			{Payload: []byte("alpha")},
			{Payload: []byte("beta")},
		},
	})
	if err != nil {
		t.Fatalf("PresentProgress: error = %v", err)
	}
	if progress.AcceptedCount != 2 {
		t.Fatalf("PresentProgress AcceptedCount = %d, want 2", progress.AcceptedCount)
	}

	finalized, err := presentationTracker.FinalizePresentation(ctx, factoryvisualization.FinalizePresentationRequest{
		SessionID: opened.SessionID,
		Terminal:  &factoryvisualization.TerminalWrite{Payload: []byte("omega")},
	})
	if err != nil {
		t.Fatalf("FinalizePresentation: error = %v", err)
	}
	if !finalized.Finalized {
		t.Fatal("FinalizePresentation: Finalized = false, want true")
	}
	if !finalized.ProgressSeen {
		t.Fatal("FinalizePresentation: ProgressSeen = false, want true")
	}

	_, err = presentationTracker.PresentProgress(ctx, factoryvisualization.PresentProgressRequest{
		SessionID: opened.SessionID,
		Records:   []factoryvisualization.ProgressRecord{{Payload: []byte("late")}},
	})
	requirePresentationError(
		t,
		err,
		factoryvisualization.PresentationErrorEnqueueAfterClose,
		"PresentProgress after finalize",
	)

	bestEffort, err := presentationTracker.OpenPresentation(ctx, factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryBestEffort,
	})
	if err != nil {
		t.Fatalf("OpenPresentation best-effort: error = %v", err)
	}
	_, err = presentationTracker.FinalizePresentation(ctx, factoryvisualization.FinalizePresentationRequest{
		SessionID: bestEffort.SessionID,
	})
	requirePresentationError(
		t,
		err,
		factoryvisualization.PresentationErrorFinalizeWithoutWriter,
		"FinalizePresentation without terminal writer",
	)

	closeSession, err := presentationTracker.OpenPresentation(ctx, factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryBestEffort,
	})
	if err != nil {
		t.Fatalf("OpenPresentation for close path: error = %v", err)
	}
	if _, err := presentationTracker.PresentProgress(ctx, factoryvisualization.PresentProgressRequest{
		SessionID: closeSession.SessionID,
		Records:   []factoryvisualization.ProgressRecord{{Payload: []byte("queued")}},
	}); err != nil {
		t.Fatalf("PresentProgress before close: error = %v", err)
	}
	closed, err := presentationTracker.ClosePresentation(ctx, factoryvisualization.ClosePresentationRequest{
		SessionID: closeSession.SessionID,
	})
	if err != nil {
		t.Fatalf("ClosePresentation: error = %v", err)
	}
	if closed.DroppedCount < 0 {
		t.Fatalf("ClosePresentation DroppedCount = %d, want non-negative", closed.DroppedCount)
	}
}

func requirePresentationError(
	t *testing.T,
	err error,
	kind factoryvisualization.PresentationErrorKind,
	label string,
) {
	t.Helper()
	var presErr *factoryvisualization.PresentationError
	if !errors.As(err, &presErr) || presErr.Kind != kind {
		t.Fatalf("%s: error = %v, want %s", label, err, kind)
	}
}
