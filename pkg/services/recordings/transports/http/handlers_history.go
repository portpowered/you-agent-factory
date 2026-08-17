package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

const durableHistorySessionPrefix = "dur-sess-"

// errHistoricalDispatchNotFound reports that no dispatch in a detached
// historical recording matched the requested id. It stays inside this
// transport because both the read and its 404 mapping are owned here.
var errHistoricalDispatchNotFound = errors.New("dispatch not found")

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
	response, err, legacy := a.factorySessionResult(r.Context(), string(sessionID), params)
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		if legacy {
			a.writeLegacyError(w, err, "failed to get factory session result")
			return
		}
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
	response, err, legacy := a.factorySessionDispatches(r.Context(), string(sessionID), params)
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		if legacy {
			a.writeLegacyError(w, err, "failed to list factory session dispatches")
			return
		}
		a.writeRootOrInternalError(w, recordingsHTTPOperationHistoricalRead, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
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
	response, err, legacy := a.factorySessionDispatch(r.Context(), string(sessionID), string(dispatchID))
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		if legacy {
			a.writeLegacyError(w, err, "failed to get factory session dispatch")
			return
		}
		if errors.Is(err, errHistoricalDispatchNotFound) {
			a.writeError(w, http.StatusNotFound, "factory session dispatch not found", "NOT_FOUND")
			return
		}
		a.writeRootOrInternalError(w, recordingsHTTPOperationHistoricalRead, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

func (a *Adapter) factorySessionResult(
	ctx context.Context,
	sessionID string,
	params factoryapi.GetFactorySessionResultsParams,
) (factoryapi.FactorySessionResult, error, bool) {
	if a.root == nil && a.hasLegacyHistory() {
		result, err := a.legacyResult(ctx, sessionID, params)
		return result, err, true
	}
	history, err := a.historicalRecording(ctx, sessionID)
	if err != nil {
		if isExpectedLiveFallback(err) && a.hasLegacyHistory() {
			result, legacyErr := a.legacyResult(ctx, sessionID, params)
			return result, legacyErr, true
		}
		return factoryapi.FactorySessionResult{}, err, false
	}
	result, err := historicalResultResponse(history, sessionID, params)
	return result, err, false
}

func (a *Adapter) factorySessionDispatches(
	ctx context.Context,
	sessionID string,
	params factoryapi.ListFactorySessionDispatchesParams,
) (factoryapi.ListFactorySessionDispatchesResponse, error, bool) {
	if a.root == nil && a.hasLegacyHistory() {
		result, err := a.legacyDispatches(ctx, sessionID, params)
		return result, err, true
	}
	history, err := a.historicalRecording(ctx, sessionID)
	if err != nil {
		if isExpectedLiveFallback(err) && a.hasLegacyHistory() {
			result, legacyErr := a.legacyDispatches(ctx, sessionID, params)
			return result, legacyErr, true
		}
		return factoryapi.ListFactorySessionDispatchesResponse{}, err, false
	}
	dispatches := make([]factorysessionmapping.HistoricalDispatchInput, 0, len(history.Dispatches))
	for _, dispatch := range history.Dispatches {
		if params.Status != nil && string(*params.Status) != string(dispatch.Status) {
			continue
		}
		dispatches = append(dispatches, historicalDispatchInput(dispatch))
	}
	return factorysessionmapping.HistoricalDispatchListToAPI(sessionID, dispatches), nil, false
}

func (a *Adapter) factorySessionDispatch(
	ctx context.Context,
	sessionID string,
	dispatchID string,
) (factoryapi.FactoryDispatch, error, bool) {
	if a.root == nil && a.hasLegacyHistory() {
		result, err := a.legacyDispatch(ctx, sessionID, dispatchID)
		return result, err, true
	}
	history, err := a.historicalRecording(ctx, sessionID)
	if err != nil {
		if isExpectedLiveFallback(err) && a.hasLegacyHistory() {
			result, legacyErr := a.legacyDispatch(ctx, sessionID, dispatchID)
			return result, legacyErr, true
		}
		return factoryapi.FactoryDispatch{}, err, false
	}
	for _, dispatch := range history.Dispatches {
		if dispatch.ID != dispatchID {
			continue
		}
		return factorysessionmapping.HistoricalDispatchDetailToAPI(
			sessionID, historicalDispatchInput(dispatch), historicalOrchestratorKind(dispatch),
		), nil, false
	}
	return factoryapi.FactoryDispatch{}, errHistoricalDispatchNotFound, false
}

func historicalDispatchInput(
	dispatch recordings.HistoricalDispatch,
) factorysessionmapping.HistoricalDispatchInput {
	return factorysessionmapping.HistoricalDispatchInput{
		ID:           dispatch.ID,
		Status:       string(dispatch.Status),
		DispatchKind: historicalDispatchKind(dispatch),
	}
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
	read := factorysessionmapping.HistoricalResultInput{
		SessionID:        sessionID,
		ResultStatus:     historicalResultStatus(result, state),
		SessionStatus:    historicalSessionStatus(result, state),
		Mode:             "final",
		IncludeArtifacts: params.IncludeArtifacts != nil && bool(*params.IncludeArtifacts),
	}
	if params.Mode != nil {
		read.Mode = string(*params.Mode)
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
			read.Failure = &factorysessionmapping.HistoricalFailureInput{
				Reason: string(failure.Reason), Message: failure.Message,
				PartialResultAvailable: len(state.SessionBracket.ResultSummary) > 0,
			}
		}
	}
	return factorysessionmapping.HistoricalResultToAPI(read), nil
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
