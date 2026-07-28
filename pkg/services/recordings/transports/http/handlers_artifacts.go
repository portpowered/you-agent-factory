package http

import (
	"context"
	"errors"
	"net/http"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ListFactorySessionArtifacts decodes one session-scoped artifact list request,
// invokes the accepted Recordings root, and encodes the public HTTP success
// response from detached artifact projections.
func (a *Adapter) ListFactorySessionArtifacts(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
) {
	input := ArtifactListInput{SessionID: string(sessionID)}
	statusRequest, err := ArtifactListRequestFromAPI(input)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid artifact read scope", "BAD_REQUEST")
		return
	}
	if requestContextEnded(r.Context()) {
		return
	}
	artifacts, err := a.loadArtifactProjections(r.Context(), statusRequest.RecordingID)
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		a.writeRootOrInternalError(w, recordingsHTTPOperationArtifactRead, err)
		return
	}
	a.writeJSON(w, http.StatusOK, ArtifactListResponseToAPI(input.SessionID, artifacts))
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
	artifacts, err := a.loadArtifactProjections(r.Context(), readRequest.RecordingID)
	if shouldEndOnRequestContext(r.Context(), err) {
		return
	}
	if err != nil {
		a.writeRootOrInternalError(w, recordingsHTTPOperationArtifactRead, err)
		return
	}
	artifact, ok := findArtifactStateByID(artifacts, input.ArtifactID)
	if !ok {
		a.writeError(w, http.StatusNotFound, "factory session artifact not found", "NOT_FOUND")
		return
	}
	a.writeJSON(w, http.StatusOK, ArtifactDetailResponseToAPI(input.SessionID, artifact))
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

