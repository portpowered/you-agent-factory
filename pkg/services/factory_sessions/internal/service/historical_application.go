package service

import (
	"context"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// HistoricalApplicationInspection is the read-only replay selected for a CLI
// run. A checkpoint retains its existing execution owner through the public
// Sessions route until cleanup, without a live Factory runtime registration.
type HistoricalApplicationInspection struct {
	Replay                 *factorysessions.HistoricalReplayInspection
	Diagnostics            factoryruntime.RuntimeLogDiagnostics
	ReplayMetadataWarnings []recordings.MetadataMismatchWarning
	ResumeRecoveryMetadata *recordings.ResumeRecoveryMetadata
	Close                  func() error
}

// InspectHistoricalApplication classifies a replay input before live session
// activation. Hosted or legacy V1 replays continue through canonical Start.
func (r *Root) InspectHistoricalApplication(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
) (HistoricalApplicationInspection, bool, error) {
	if r == nil || r.replayInputs == nil {
		return HistoricalApplicationInspection{}, false, fmt.Errorf("historical replay input service is required")
	}
	if request.RuntimeSelection == nil || strings.TrimSpace(request.RuntimeSelection.Recording.ReplayPath) == "" ||
		request.RuntimeSelection.Host.Port > 0 {
		return HistoricalApplicationInspection{}, false, nil
	}
	if strings.TrimSpace(request.FolderPath) == "" {
		return HistoricalApplicationInspection{}, false, &factorysessions.DetachedRequestError{Field: "folderPath", Message: "folder path is required"}
	}
	input, err := r.replayInputs.LoadReplayInput(recordings.LoadReplayInputRequest{
		Path: request.RuntimeSelection.Recording.ReplayPath,
	})
	if err != nil {
		return HistoricalApplicationInspection{}, false, err
	}
	if !selectsHistoricalReplayInspection(input) {
		return HistoricalApplicationInspection{}, false, nil
	}
	session := request
	selection := runtimeSelectionForStart(request)
	products, _, err := r.openRuntimeWithOptions(ctx, definitionRequestForStart(request), runtimeOwnerRequestForStart(request), &session, false, workerRequestForStart(request), recordingRequestForStart(request), selection.ModelCacheDirectory, selection.OperatorDefaults, r.baseLogger, nil, &input)
	if err != nil {
		return HistoricalApplicationInspection{}, false, err
	}
	if products.historicalReplay == nil {
		if products.closeArtifacts != nil {
			_ = products.closeArtifacts()
		}
		return HistoricalApplicationInspection{}, false, fmt.Errorf("historical replay inspection is unavailable")
	}
	release := func() {}
	if products.historicalReplay.Checkpoint != nil {
		binder, ok := r.SessionGateway.(interface {
			BindHistoricalExecution(string, durableexecution.Service) func()
		})
		if !ok {
			if products.closeArtifacts != nil {
				_ = products.closeArtifacts()
			}
			return HistoricalApplicationInspection{}, false, fmt.Errorf("historical replay Sessions routing is unavailable")
		}
		release = binder.BindHistoricalExecution(products.historicalReplay.Session.SessionID, products.replayExecution)
	}
	closeInspection := func() error {
		defer release()
		if products.closeArtifacts != nil {
			return products.closeArtifacts()
		}
		return nil
	}
	return HistoricalApplicationInspection{
		Replay:                 products.historicalReplay,
		ReplayMetadataWarnings: append([]recordings.MetadataMismatchWarning(nil), products.replayMetadataWarnings...),
		Close:                  closeInspection,
	}, true, nil
}
