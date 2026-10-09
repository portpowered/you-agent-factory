package service

import (
	"context"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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

// InspectHistoricalApplication delegates replay inspection to the fixed opening owner.
func (r *Root) InspectHistoricalApplication(ctx context.Context, request factorysessions.SessionStartRequest) (HistoricalApplicationInspection, bool, error) {
	if r == nil {
		return HistoricalApplicationInspection{}, false, fmt.Errorf("historical replay input service is required")
	}
	return r.inspectHistorical(ctx, request)
}

// InspectHistoricalApplication classifies a replay input before live session
// activation. Hosted or legacy V1 replays continue through canonical Start.
func (r *RuntimeOpening) InspectHistoricalApplication(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
) (HistoricalApplicationInspection, bool, error) {
	if r == nil || r.snapshotSelection == nil || r.snapshotSelection.replayInputs == nil {
		return HistoricalApplicationInspection{}, false, fmt.Errorf("historical replay input service is required")
	}
	if request.RuntimeSelection == nil || strings.TrimSpace(request.RuntimeSelection.Recording.ReplayPath) == "" ||
		request.RuntimeSelection.Host.Port > 0 {
		return HistoricalApplicationInspection{}, false, nil
	}
	if strings.TrimSpace(request.FolderPath) == "" {
		return HistoricalApplicationInspection{}, false, &factorysessions.DetachedRequestError{Field: "folderPath", Message: "folder path is required"}
	}
	input, err := r.snapshotSelection.loadReplayInputForActivation(request.RuntimeSelection.Recording.ReplayPath, nil)
	if err != nil {
		return HistoricalApplicationInspection{}, false, err
	}
	if !selectsHistoricalReplayInspection(input) {
		return HistoricalApplicationInspection{}, false, nil
	}
	session := request
	selection := runtimeSelectionForStart(request)
	opening, err := r.prepareRuntimeOpening(ctx, definitionRequestForStart(request), runtimeOwnerRequestForStart(request), &session, false, workerRequestForStart(request), recordingRequestForStart(request), selection.ModelCacheDirectory, selection.OperatorDefaults, r.baseLogger, nil, &input)
	if err != nil {
		return HistoricalApplicationInspection{}, false, err
	}
	if opening.load.HistoricalReplay == nil {
		return HistoricalApplicationInspection{}, false, fmt.Errorf("historical replay inspection is unavailable")
	}
	replay, closeReplay, err := r.openHistoricalSessionRuntime(ctx, opening)
	if err != nil {
		return HistoricalApplicationInspection{}, false, err
	}
	inspection := replay.Inspection()
	release := func() {}
	if inspection.Checkpoint != nil {
		release, err = r.assembly.BindHistoricalOpening(inspection.Session.SessionID, replay)
		if err != nil {
			if closeReplay != nil {
				_ = closeReplay()
			}
			return HistoricalApplicationInspection{}, false, err
		}
	}
	closeInspection := func() error {
		defer release()
		if closeReplay != nil {
			return closeReplay()
		}
		return nil
	}
	return HistoricalApplicationInspection{
		Replay:                 &inspection,
		ReplayMetadataWarnings: append([]recordings.MetadataMismatchWarning(nil), opening.load.ReplayMetadataWarnings...),
		Close:                  closeInspection,
	}, true, nil
}
