package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

const durableHistorySessionPrefix = "dur-sess-"

func isDurableHistorySession(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), durableHistorySessionPrefix)
}

func (a *Adapter) historicalRecording(
	ctx context.Context,
	sessionID string,
) (recordings.HistoricalRecordingQueryResult, error) {
	if err := ctx.Err(); err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	status, err := a.invokeQueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordings.RecordingID(sessionID),
	})
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	artifact := strings.TrimSpace(string(status.Status.Artifact))
	if artifact == "" {
		return recordings.HistoricalRecordingQueryResult{}, &recordings.HistoricalRecordingQueryError{
			Kind:        recordings.HistoricalRecordingQueryErrorUnavailable,
			RecordingID: recordings.RecordingID(sessionID),
		}
	}
	result, err := a.invokeQueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: recordings.RecordingID(sessionID),
			Artifact:    recordings.RecordingArtifactReference(artifact),
			Scope:       recordings.CanonicalEventScope{FactorySessionID: sessionID},
		},
	})
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	return result, nil
}

// GetFactorySessionResults serves finalized durable result history from the
// immutable Recordings artifact rather than the live Factory Session runtime.
func (a *Adapter) GetFactorySessionResults(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.GetFactorySessionResultsParams,
) {
	if !isDurableHistorySession(string(sessionID)) {
		a.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	}
	if a.hasLegacyHistory() {
		response, err := a.legacyResult(r.Context(), string(sessionID), params)
		if shouldEndOnRequestContext(r.Context(), err) {
			return
		}
		if err != nil {
			a.writeLegacyError(w, err, "failed to get factory session result")
			return
		}
		a.writeJSON(w, http.StatusOK, response)
		return
	}
	result, err := a.historicalRecording(r.Context(), string(sessionID))
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		a.writeRootOrInternalError(w, recordingsHTTPOperationHistoricalRead, err)
		return
	}
	response, err := historicalResultResponse(result, string(sessionID), params)
	if err != nil {
		a.writeRootOrInternalError(w, recordingsHTTPOperationHistoricalRead, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

// ListFactorySessionDispatches serves the detached dispatch lifecycle
// projection produced by Recordings' historical query capability.
func (a *Adapter) ListFactorySessionDispatches(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	params factoryapi.ListFactorySessionDispatchesParams,
) {
	if !isDurableHistorySession(string(sessionID)) {
		a.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	}
	if a.hasLegacyHistory() {
		response, err := a.legacyDispatches(r.Context(), string(sessionID), params)
		if shouldEndOnRequestContext(r.Context(), err) {
			return
		}
		if err != nil {
			a.writeLegacyError(w, err, "failed to list factory session dispatches")
			return
		}
		a.writeJSON(w, http.StatusOK, response)
		return
	}
	result, err := a.historicalRecording(r.Context(), string(sessionID))
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		a.writeRootOrInternalError(w, recordingsHTTPOperationHistoricalRead, err)
		return
	}
	dispatches := make([]factorysessions.DispatchSummary, 0, len(result.Dispatches))
	for _, dispatch := range result.Dispatches {
		if params.Status != nil && string(*params.Status) != string(dispatch.Status) {
			continue
		}
		dispatches = append(dispatches, factorysessions.DispatchSummary{
			ID: dispatch.ID, Status: factorysessions.DispatchStatus(dispatch.Status),
			DispatchKind: historicalDispatchKind(dispatch),
		})
	}
	a.writeJSON(w, http.StatusOK, factorysessionmapping.ListDispatchesResponseToAPI(
		factorysessions.ListDispatchesResult{SessionID: string(sessionID), Dispatches: dispatches},
	))
}

// GetFactorySessionDispatch serves one detached historical dispatch fact.
func (a *Adapter) GetFactorySessionDispatch(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	dispatchID factoryapi.DispatchID,
) {
	if !isDurableHistorySession(string(sessionID)) {
		a.writeError(w, http.StatusNotFound, "factory session not found", "NOT_FOUND")
		return
	}
	if a.hasLegacyHistory() {
		response, err := a.legacyDispatch(r.Context(), string(sessionID), string(dispatchID))
		if shouldEndOnRequestContext(r.Context(), err) {
			return
		}
		if err != nil {
			a.writeLegacyError(w, err, "failed to get factory session dispatch")
			return
		}
		a.writeJSON(w, http.StatusOK, response)
		return
	}
	result, err := a.historicalRecording(r.Context(), string(sessionID))
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		a.writeRootOrInternalError(w, recordingsHTTPOperationHistoricalRead, err)
		return
	}
	for _, dispatch := range result.Dispatches {
		if dispatch.ID != string(dispatchID) {
			continue
		}
		response := factorysessionmapping.DispatchDetailResponseToAPI(factorysessions.DispatchDetail{
			DispatchSummary: factorysessions.DispatchSummary{
				ID: dispatch.ID, Status: factorysessions.DispatchStatus(dispatch.Status),
				DispatchKind: historicalDispatchKind(dispatch),
			},
			SessionID: string(sessionID), OrchestratorKind: historicalOrchestratorKind(dispatch),
		})
		a.writeJSON(w, http.StatusOK, response)
		return
	}
	a.writeError(w, http.StatusNotFound, "factory session dispatch not found", "NOT_FOUND")
}

func historicalDispatchKind(dispatch recordings.HistoricalDispatch) string {
	if value := strings.TrimSpace(string(dispatch.DispatchKind)); value != "" {
		return value
	}
	return "PETRI_TRANSITION"
}

func historicalOrchestratorKind(dispatch recordings.HistoricalDispatch) string {
	if strings.HasPrefix(historicalDispatchKind(dispatch), "JAVASCRIPT_") {
		return "JAVASCRIPT"
	}
	return "PETRI"
}

func historicalResultResponse(
	result recordings.HistoricalRecordingQueryResult,
	sessionID string,
	params factoryapi.GetFactorySessionResultsParams,
) (factoryapi.FactorySessionResult, error) {
	state := recordings.FactoryWorldState{}
	if strings.TrimSpace(result.WorldState.Payload) != "" {
		if err := json.Unmarshal([]byte(result.WorldState.Payload), &state); err != nil {
			return factoryapi.FactorySessionResult{}, err
		}
	}
	read := factorysessions.ResultReadResult{
		SessionID:        sessionID,
		ResultStatus:     factorysessions.ResultStatus(historicalResultStatus(result, state)),
		SessionStatus:    factorysessions.LifecycleStatus(historicalSessionStatus(result, state)),
		Mode:             factorysessions.ResultMode("final"),
		IncludeArtifacts: params.IncludeArtifacts != nil && bool(*params.IncludeArtifacts),
	}
	if params.Mode != nil {
		read.Mode = factorysessions.ResultMode(*params.Mode)
	}
	if state.SessionBracket != nil {
		if len(state.SessionBracket.ResultSummary) > 0 {
			encoded, err := json.Marshal(state.SessionBracket.ResultSummary)
			if err != nil {
				return factoryapi.FactorySessionResult{}, err
			}
			read.PrimaryResult = encoded
		}
		read.ArtifactIDs = append([]string(nil), state.SessionBracket.ArtifactIDs...)
		if failure := state.SessionBracket.FailureDetail; failure != nil {
			read.Failure = &factorysessions.FailureSummary{
				Reason: string(failure.Reason), Message: failure.Message,
				PartialResultAvailable: len(state.SessionBracket.ResultSummary) > 0,
			}
		}
	}
	return factorysessionmapping.ResultResponseToAPI(read), nil
}

func historicalResultStatus(
	result recordings.HistoricalRecordingQueryResult,
	state recordings.FactoryWorldState,
) string {
	if state.SessionBracket != nil && strings.TrimSpace(state.SessionBracket.ResultStatus) != "" {
		return state.SessionBracket.ResultStatus
	}
	if result.Status.State == recordings.RecordingFailed {
		return "FAILED_WITH_PARTIAL"
	}
	return "FINAL"
}

func historicalSessionStatus(
	result recordings.HistoricalRecordingQueryResult,
	state recordings.FactoryWorldState,
) string {
	if state.SessionBracket != nil && strings.TrimSpace(state.SessionBracket.FinalStatus) != "" {
		return state.SessionBracket.FinalStatus
	}
	if result.Status.State == recordings.RecordingFailed {
		return "FAILED"
	}
	return "SUCCEEDED"
}
