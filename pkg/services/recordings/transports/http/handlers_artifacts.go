package http

import (
	"context"
	"errors"
	"net/http"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// errHistoricalArtifactNotFound reports that no artifact in a detached
// historical world state matched the requested id. It stays inside this
// transport because both the read and its 404 mapping are owned here.
var errHistoricalArtifactNotFound = errors.New("factory session artifact not found")

// ListFactorySessionArtifacts decodes one session-scoped artifact list request,
// invokes the accepted Recordings root, and encodes the public HTTP success
// response from detached artifact projections.
func (a *Adapter) ListFactorySessionArtifacts(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	input := ArtifactListInput{SessionID: string(sessionID)}
	if a.root == nil && a.legacyHistory == nil {
		a.writeError(w, http.StatusNotFound, "factory session artifact not found", "NOT_FOUND")
		return
	}
	statusRequest, err := ArtifactListRequestFromAPI(input)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid artifact read scope", "BAD_REQUEST")
		return
	}
	if requestContextEnded(r.Context()) {
		return
	}
	response, err, legacy := a.factorySessionArtifacts(r.Context(), input.SessionID, statusRequest.RecordingID)
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		if legacy {
			a.writeLegacyError(w, err, "failed to list factory session artifacts")
			return
		}
		a.writeRootOrInternalError(w, recordingsHTTPOperationArtifactRead, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

// GetFactorySessionArtifact decodes one session-scoped artifact get request,
// invokes the accepted Recordings root, and encodes the public HTTP success
// response from detached artifact projections.
func (a *Adapter) GetFactorySessionArtifact(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	artifactID factoryapi.ArtifactID,
) {
	input := ArtifactGetInput{
		SessionID:  string(sessionID),
		ArtifactID: string(artifactID),
	}
	if a.root == nil && a.legacyHistory == nil {
		a.writeError(w, http.StatusNotFound, "factory session artifact not found", "NOT_FOUND")
		return
	}
	readRequest, err := ArtifactGetRequestFromAPI(input)
	if err != nil {
		if errors.Is(err, errInvalidArtifactReadID) {
			a.writeError(w, http.StatusBadRequest, "invalid artifact id", "BAD_REQUEST")
			return
		}
		a.writeError(w, http.StatusBadRequest, "invalid artifact read scope", "BAD_REQUEST")
		return
	}
	if requestContextEnded(r.Context()) {
		return
	}
	response, err, legacy := a.factorySessionArtifact(
		r.Context(), input.SessionID, readRequest.RecordingID, input.ArtifactID,
	)
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		if legacy {
			a.writeLegacyError(w, err, "failed to get factory session artifact")
			return
		}
		if errors.Is(err, errHistoricalArtifactNotFound) {
			a.writeError(w, http.StatusNotFound, "factory session artifact not found", "NOT_FOUND")
			return
		}
		a.writeRootOrInternalError(w, recordingsHTTPOperationArtifactRead, err)
		return
	}
	a.writeJSON(w, http.StatusOK, response)
}

func (a *Adapter) factorySessionArtifact(
	ctx context.Context,
	sessionID string,
	recordingID recordings.RecordingID,
	artifactID string,
) (factoryapi.FactorySessionArtifactDetail, error, bool) {
	if a.shouldUseLegacyArtifact(sessionID) {
		response, err := a.legacyArtifact(ctx, sessionID, artifactID)
		return response, err, true
	}
	artifacts, err := a.loadArtifactProjections(ctx, recordingID)
	if err != nil {
		return a.fallbackArtifact(ctx, sessionID, artifactID, err)
	}
	artifact, ok := findArtifactStateByID(artifacts, artifactID)
	if !ok {
		return factoryapi.FactorySessionArtifactDetail{}, errHistoricalArtifactNotFound, false
	}
	return ArtifactDetailResponseToAPI(sessionID, artifact), nil, false
}

func (a *Adapter) fallbackArtifact(
	ctx context.Context,
	sessionID string,
	artifactID string,
	err error,
) (factoryapi.FactorySessionArtifactDetail, error, bool) {
	if !isDurableHistorySession(sessionID) || !isExpectedLiveFallback(err) || !a.hasLegacyHistory() {
		return factoryapi.FactorySessionArtifactDetail{}, err, false
	}
	response, legacyErr := a.legacyArtifact(ctx, sessionID, artifactID)
	return response, legacyErr, true
}

func (a *Adapter) shouldUseLegacyArtifact(sessionID string) bool {
	return isDurableHistorySession(sessionID) && a.root == nil && a.hasLegacyHistory()
}

func (a *Adapter) factorySessionArtifacts(
	ctx context.Context,
	sessionID string,
	recordingID recordings.RecordingID,
) (factoryapi.ListFactorySessionArtifactsResponse, error, bool) {
	if isDurableHistorySession(sessionID) && a.root == nil && a.hasLegacyHistory() {
		result, err := a.legacyArtifacts(ctx, sessionID)
		return result, err, true
	}
	artifacts, err := a.loadArtifactProjections(ctx, recordingID)
	if err != nil {
		if isDurableHistorySession(sessionID) && isExpectedLiveFallback(err) && a.hasLegacyHistory() {
			result, legacyErr := a.legacyArtifacts(ctx, sessionID)
			return result, legacyErr, true
		}
		return factoryapi.ListFactorySessionArtifactsResponse{}, err, false
	}
	return ArtifactListResponseToAPI(sessionID, artifacts), nil, false
}

func (a *Adapter) loadArtifactProjections(
	ctx context.Context,
	recordingID recordings.RecordingID,
) ([]interfaces.FactorySessionArtifactState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := a.invokeQueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordingID,
	}); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	built, err := a.invokeBuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reconstructed, err := a.invokeReconstructWorldState(
		ReconstructWorldStateRequestFromPortableArtifact(built.Artifact),
	)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ArtifactStatesFromWorldStatePayload(reconstructed.WorldState.Payload)
}
