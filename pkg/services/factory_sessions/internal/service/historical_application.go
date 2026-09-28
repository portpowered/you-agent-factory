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
// run. It has no live Factory Session or application runtime registration.
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
	opening, err := runtimeRequestForStart(request)
	if err != nil {
		return HistoricalApplicationInspection{}, false, err
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
	products, err := r.openRuntimeWithReplayInput(ctx, &opening, r.baseLogger, &input)
	if err != nil {
		return HistoricalApplicationInspection{}, false, err
	}
	view := products.application
	if view.HistoricalReplay == nil {
		if view.Resources.Close != nil {
			_ = view.Resources.Close()
		}
		return HistoricalApplicationInspection{}, false, fmt.Errorf("historical replay inspection is unavailable")
	}
	return HistoricalApplicationInspection{
		Replay:                 view.HistoricalReplay,
		Diagnostics:            view.Resources.Diagnostics,
		ReplayMetadataWarnings: append([]recordings.MetadataMismatchWarning(nil), view.ReplayMetadataWarnings...),
		ResumeRecoveryMetadata: view.ResumeRecoveryMetadata,
		Close:                  view.Resources.Close,
	}, true, nil
}
