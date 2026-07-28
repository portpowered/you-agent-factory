package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

var (
	errInvalidArtifactReadScope = errors.New("invalid artifact read scope")
	errInvalidArtifactReadID    = errors.New("invalid artifact read id")
)

// ArtifactListInput carries decoded HTTP inputs for one session-scoped artifact
// list operation owned by Recordings.
type ArtifactListInput struct {
	SessionID string
}

// ArtifactGetInput carries decoded HTTP inputs for one session-scoped artifact
// get operation owned by Recordings.
type ArtifactGetInput struct {
	SessionID  string
	ArtifactID string
}

// ArtifactListRequestFromAPI maps one public artifact list request into the
// accepted Recordings root recording-status query.
func ArtifactListRequestFromAPI(input ArtifactListInput) (recordings.RecordingStatusRequest, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return recordings.RecordingStatusRequest{}, fmt.Errorf(
			"%w: session id is required",
			errInvalidArtifactReadScope,
		)
	}
	return recordings.RecordingStatusRequest{
		RecordingID: recordings.RecordingID(sessionID),
	}, nil
}

// ArtifactGetRequestFromAPI maps one public artifact get request into the
// accepted Recordings root portable-artifact read request.
func ArtifactGetRequestFromAPI(input ArtifactGetInput) (recordings.ReadPortableArtifactRequest, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return recordings.ReadPortableArtifactRequest{}, fmt.Errorf(
			"%w: session id is required",
			errInvalidArtifactReadScope,
		)
	}
	artifactID := strings.TrimSpace(input.ArtifactID)
	if artifactID == "" {
		return recordings.ReadPortableArtifactRequest{}, fmt.Errorf(
			"%w: artifact id is required",
			errInvalidArtifactReadID,
		)
	}
	return recordings.ReadPortableArtifactRequest{
		RecordingID: recordings.RecordingID(sessionID),
		Reference:   recordings.RecordingArtifactReference(artifactID),
	}, nil
}

// ReconstructWorldStateRequestFromPortableArtifact maps one portable artifact
// into the Recordings root projection query used to derive artifact summaries.
func ReconstructWorldStateRequestFromPortableArtifact(
	artifact recordings.PortableArtifact,
) recordings.ReconstructWorldStateRequest {
	selectedTick := 0
	for _, event := range artifact.Events {
		if tick := int(event.FactoryTick); tick > selectedTick {
			selectedTick = tick
		}
	}
	return recordings.ReconstructWorldStateRequest{
		Scope:        artifact.Summary.Scope,
		Events:       append([]recordings.CanonicalEvent(nil), artifact.Events...),
		SelectedTick: selectedTick,
	}
}

// ArtifactStatesFromWorldStatePayload extracts detached artifact projections
// from one Recordings world-state payload.
func ArtifactStatesFromWorldStatePayload(payload string) ([]interfaces.FactorySessionArtifactState, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, nil
	}
	var state interfaces.FactoryWorldState
	if err := json.Unmarshal([]byte(payload), &state); err != nil {
		return nil, err
	}
	return artifactStatesFromWorldState(state), nil
}

func artifactStatesFromWorldState(
	state interfaces.FactoryWorldState,
) []interfaces.FactorySessionArtifactState {
	artifacts := append([]interfaces.FactorySessionArtifactState(nil), state.Artifacts...)
	if state.JavaScriptRuntime != nil && len(state.JavaScriptRuntime.Artifacts) > 0 {
		artifacts = append(artifacts, state.JavaScriptRuntime.Artifacts...)
	}
	return dedupeArtifactStates(artifacts)
}

func dedupeArtifactStates(
	artifacts []interfaces.FactorySessionArtifactState,
) []interfaces.FactorySessionArtifactState {
	if len(artifacts) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(artifacts))
	deduped := make([]interfaces.FactorySessionArtifactState, 0, len(artifacts))
	for _, artifact := range artifacts {
		id := strings.TrimSpace(artifact.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		artifact.ID = id
		deduped = append(deduped, artifact)
	}
	return deduped
}

// ArtifactListResponseToAPI encodes detached artifact projections into the
// public list response shape.
func ArtifactListResponseToAPI(
	sessionID string,
	artifacts []interfaces.FactorySessionArtifactState,
) factoryapi.ListFactorySessionArtifactsResponse {
	response := factoryapi.ListFactorySessionArtifactsResponse{
		SessionId: sessionID,
		Artifacts: []factoryapi.FactorySessionArtifactSummary{},
	}
	if len(artifacts) == 0 {
		return response
	}
	summaries := make([]factoryapi.FactorySessionArtifactSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		summaries = append(summaries, artifactSummaryToAPI(sessionID, artifact))
	}
	response.Artifacts = summaries
	return response
}

// ArtifactDetailResponseToAPI encodes one detached artifact projection into the
// public detail response shape.
func ArtifactDetailResponseToAPI(
	sessionID string,
	artifact interfaces.FactorySessionArtifactState,
) factoryapi.FactorySessionArtifactDetail {
	response := factoryapi.FactorySessionArtifactDetail{
		SessionId:  sessionID,
		Id:         strings.TrimSpace(artifact.ID),
		Kind:       artifactKindToAPI(artifact.Kind),
		Visibility: artifactVisibilityToAPI(artifact.Visibility),
	}
	if label := strings.TrimSpace(artifact.Label); label != "" {
		response.Label = &label
	}
	if summary := strings.TrimSpace(artifact.Summary); summary != "" {
		response.Summary = &summary
	}
	if contentHash := strings.TrimSpace(artifact.ContentHash); contentHash != "" {
		response.ContentHash = &contentHash
	}
	if artifact.SizeBytes > 0 {
		size := artifact.SizeBytes
		response.SizeBytes = &size
	}
	if !artifact.CapturedAt.IsZero() {
		capturedAt := artifact.CapturedAt.UTC()
		response.CreatedAt = &capturedAt
	}
	if dispatchID := artifactCaptureMetadata(artifact, "sourceDispatchId"); dispatchID != "" {
		response.DispatchId = &dispatchID
	}
	if auditMode := strings.TrimSpace(artifact.AuditMode); auditMode != "" {
		mode := factoryapi.FactoryArtifactAuditMode(auditMode)
		response.AuditMode = &mode
	}
	if counts := artifactRedactionCountsToAPI(artifact.RedactionCounts); counts != nil {
		response.RedactionCounts = counts
	}
	if ref := artifactRetrievalRefToAPI(sessionID, artifact.ID); ref != nil {
		response.ContentRef = ref
	}
	if metadata := artifactCaptureMetadataToAPI(artifact); metadata != nil {
		response.CaptureMetadata = metadata
	}
	return response
}

func artifactSummaryToAPI(
	sessionID string,
	artifact interfaces.FactorySessionArtifactState,
) factoryapi.FactorySessionArtifactSummary {
	response := factoryapi.FactorySessionArtifactSummary{
		Id:         strings.TrimSpace(artifact.ID),
		Kind:       artifactKindToAPI(artifact.Kind),
		Visibility: artifactVisibilityToAPI(artifact.Visibility),
	}
	if label := strings.TrimSpace(artifact.Label); label != "" {
		response.Label = &label
	}
	if contentHash := strings.TrimSpace(artifact.ContentHash); contentHash != "" {
		response.ContentHash = &contentHash
	}
	if artifact.SizeBytes > 0 {
		size := artifact.SizeBytes
		response.SizeBytes = &size
	}
	if !artifact.CapturedAt.IsZero() {
		capturedAt := artifact.CapturedAt.UTC()
		response.CreatedAt = &capturedAt
	}
	if dispatchID := artifactCaptureMetadata(artifact, "sourceDispatchId"); dispatchID != "" {
		response.DispatchId = &dispatchID
	}
	if auditMode := strings.TrimSpace(artifact.AuditMode); auditMode != "" {
		mode := factoryapi.FactoryArtifactAuditMode(auditMode)
		response.AuditMode = &mode
	}
	if counts := artifactRedactionCountsToAPI(artifact.RedactionCounts); counts != nil {
		response.RedactionCounts = counts
	}
	if ref := artifactRetrievalRefToAPI(sessionID, artifact.ID); ref != nil {
		response.RetrievalRef = ref
	}
	return response
}

func artifactKindToAPI(kind string) factoryapi.FactoryArtifactKind {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case string(factoryapi.FactoryArtifactKindFINALRESULT):
		return factoryapi.FactoryArtifactKindFINALRESULT
	case string(factoryapi.FactoryArtifactKindCHILDRESULT):
		return factoryapi.FactoryArtifactKindCHILDRESULT
	case string(factoryapi.FactoryArtifactKindFINDING):
		return factoryapi.FactoryArtifactKindFINDING
	case string(factoryapi.FactoryArtifactKindPATCH):
		return factoryapi.FactoryArtifactKindPATCH
	case string(factoryapi.FactoryArtifactKindLOG):
		return factoryapi.FactoryArtifactKindLOG
	case string(factoryapi.FactoryArtifactKindDATASET):
		return factoryapi.FactoryArtifactKindDATASET
	case string(factoryapi.FactoryArtifactKindCHECKPOINT):
		return factoryapi.FactoryArtifactKindCHECKPOINT
	case string(factoryapi.FactoryArtifactKindWORKTREESUMMARY):
		return factoryapi.FactoryArtifactKindWORKTREESUMMARY
	default:
		return factoryapi.FactoryArtifactKindLOG
	}
}

func artifactVisibilityToAPI(visibility string) factoryapi.FactoryArtifactVisibility {
	switch strings.ToUpper(strings.TrimSpace(visibility)) {
	case string(factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT):
		return factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT
	default:
		return factoryapi.FactoryArtifactVisibilityPUBLIC
	}
}

func artifactRedactionCountsToAPI(
	counts map[string]int,
) *factoryapi.FactoryArtifactRedactionCounts {
	if len(counts) == 0 {
		return nil
	}
	response := &factoryapi.FactoryArtifactRedactionCounts{}
	if value, ok := counts["secrets"]; ok && value > 0 {
		secrets := int32(value)
		response.Secrets = &secrets
	}
	if value, ok := counts["paths"]; ok && value > 0 {
		paths := int32(value)
		response.Paths = &paths
	}
	if value, ok := counts["tokens"]; ok && value > 0 {
		tokens := int32(value)
		response.Tokens = &tokens
	}
	if response.Secrets == nil && response.Paths == nil && response.Tokens == nil {
		return nil
	}
	return response
}

func artifactRetrievalRefToAPI(
	sessionID string,
	artifactID string,
) *factoryapi.FactorySessionArtifactRetrievalRef {
	artifactID = strings.TrimSpace(artifactID)
	sessionID = strings.TrimSpace(sessionID)
	if artifactID == "" || sessionID == "" {
		return nil
	}
	method := factoryapi.GET
	href := fmt.Sprintf("/factory-sessions/%s/artifacts/%s", sessionID, artifactID)
	return &factoryapi.FactorySessionArtifactRetrievalRef{
		Href:   href,
		Method: &method,
	}
}

func artifactCaptureMetadata(
	artifact interfaces.FactorySessionArtifactState,
	key string,
) string {
	if len(artifact.CaptureMetadata) == 0 {
		return ""
	}
	return strings.TrimSpace(artifact.CaptureMetadata[key])
}

func artifactCaptureMetadataToAPI(
	artifact interfaces.FactorySessionArtifactState,
) *factoryapi.FactoryArtifactCaptureMetadata {
	if len(artifact.CaptureMetadata) == 0 && artifact.CapturedAt.IsZero() {
		return nil
	}
	response := &factoryapi.FactoryArtifactCaptureMetadata{}
	if !artifact.CapturedAt.IsZero() {
		capturedAt := artifact.CapturedAt.UTC()
		response.CapturedAt = &capturedAt
	}
	if dispatchID := artifactCaptureMetadata(artifact, "sourceDispatchId"); dispatchID != "" {
		response.SourceDispatchId = &dispatchID
	}
	if mimeType := artifactCaptureMetadata(artifact, "mimeType"); mimeType != "" {
		response.MimeType = &mimeType
	}
	if response.CapturedAt == nil &&
		response.SourceDispatchId == nil &&
		response.MimeType == nil {
		return nil
	}
	return response
}

func findArtifactStateByID(
	artifacts []interfaces.FactorySessionArtifactState,
	artifactID string,
) (interfaces.FactorySessionArtifactState, bool) {
	trimmed := strings.TrimSpace(artifactID)
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ID) == trimmed {
			return artifact, true
		}
	}
	return interfaces.FactorySessionArtifactState{}, false
}
