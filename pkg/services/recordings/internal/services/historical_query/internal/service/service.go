package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/jsoncompat"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

// Service owns read-only reconstruction of one existing recording artifact.
type Service struct {
	readArtifact recordings.RecordingReadFile
	projection   recordings.ProjectionService
}

var _ interface {
	QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
} = (*Service)(nil)

// New constructs the historical projection capability from the exact
// Recordings artifact-read effect and projection already selected by Wire.
func New(
	readArtifact recordings.RecordingReadFile,
	projection recordings.ProjectionService,
) *Service {
	return &Service{readArtifact: readArtifact, projection: projection}
}

// QueryHistoricalRecording reads and reduces only the selected artifact; it
// never consults the live ledger or records lifecycle activity.
func (service *Service) QueryHistoricalRecording(
	request recordings.HistoricalRecordingQueryRequest,
) (recordings.HistoricalRecordingQueryResult, error) {
	if _, err := validHistoricalRecordingIdentity(request.Recording); err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	if service == nil || service.projection == nil {
		return recordings.HistoricalRecordingQueryResult{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorUnavailable, request.Recording, "", nil,
		)
	}
	result, selectedTick, err := service.readHistoricalEvents(request)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	view, factoryState, err := service.reconstructHistoricalWorldState(result.Recording.Scope, result.Events, selectedTick)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, result.Recording, "", err,
		)
	}
	result.WorldState = view
	result.WorkstationRequests = service.projection.ProjectWorkstationRequests(factoryState)
	result.Dispatches, err = projectHistoricalDispatches(result.Recording, result.Events)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	return result, nil
}

// ReadHistoricalEvents shares the canonical artifact decoder and dispatch
// validation with the public historical query. Attribution needs these facts,
// but neither world reconstruction nor serialized workstation projections.
func (service *Service) ReadHistoricalEvents(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
	result, _, err := service.readHistoricalEvents(request)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	result.Dispatches, err = projectHistoricalDispatches(result.Recording, result.Events)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	return result, nil
}

func (service *Service) readHistoricalEvents(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, int, error) {
	identity, err := validHistoricalRecordingIdentity(request.Recording)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, 0, err
	}
	if service == nil || service.readArtifact == nil {
		return recordings.HistoricalRecordingQueryResult{}, 0, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorUnavailable, identity, "", nil,
		)
	}
	payload, err := service.readArtifact(string(identity.Artifact))
	if err != nil {
		kind := recordings.HistoricalRecordingQueryErrorUnavailable
		if errors.Is(err, os.ErrNotExist) {
			kind = recordings.HistoricalRecordingQueryErrorMissingHistory
		}
		return recordings.HistoricalRecordingQueryResult{}, 0, historicalQueryError(kind, identity, "", err)
	}
	return service.decodeHistoricalEvents(request, payload)
}

// DecodeHistoricalEvents validates already-read bytes with the same canonical
// decoder as artifact queries. Callers can retain compact derived facts without
// retaining the source history or reading a second snapshot during validation.
func (service *Service) DecodeHistoricalEvents(request recordings.HistoricalRecordingQueryRequest, payload []byte) (recordings.HistoricalRecordingQueryResult, error) {
	result, _, err := service.decodeHistoricalEvents(request, payload)

	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	result.Dispatches, err = projectHistoricalDispatches(result.Recording, result.Events)
	return result, err
}

func (service *Service) decodeHistoricalEvents(request recordings.HistoricalRecordingQueryRequest, payload []byte) (recordings.HistoricalRecordingQueryResult, int, error) {
	identity, err := validHistoricalRecordingIdentity(request.Recording)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, 0, err
	}
	// Reserved recording targets have no history until the first commit.
	if len(payload) == 0 {
		return recordings.HistoricalRecordingQueryResult{}, 0, historicalQueryError(recordings.HistoricalRecordingQueryErrorMissingHistory, identity, "", io.EOF)
	}
	events, selectedTick, status, ignoredJSONPaths, err := decodeHistoricalArtifact(payload, identity, request.InferFactorySessionScope)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, 0, err
	}
	identity.Scope = status.Scope
	return recordings.HistoricalRecordingQueryResult{
		Recording: identity, Status: status, Events: events,
		IgnoredJSONPaths: ignoredJSONPaths,
	}, selectedTick, nil
}

func (service *Service) reconstructHistoricalWorldState(
	scope recordings.CanonicalEventScope,
	events []recordings.CanonicalEvent,
	selectedTick int,
) (recordings.WorldStateView, recordings.FactoryWorldState, error) {
	// Each artifact decoder has validated the complete canonical event stream.
	// Retain the owner-level state for derived read models rather than decode
	// the JSON view we just serialized at the published response boundary.
	legacyEvents := make([]factorydefinitions.FactoryEvent, len(events))
	for index, event := range events {
		legacyEvents[index] = canonical.FactoryEventFromCanonical(event)
	}
	state, err := service.projection.ReconstructFactoryWorldState(legacyEvents, selectedTick)
	if err != nil {
		return recordings.WorldStateView{}, recordings.FactoryWorldState{}, err
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return recordings.WorldStateView{}, recordings.FactoryWorldState{}, err
	}
	through := recordings.CanonicalEventCursor{}
	if len(events) > 0 {
		through = events[len(events)-1].Cursor
	}
	return recordings.WorldStateView{
		SchemaVersion: recordings.WorldStateViewSchemaV1,
		Scope:         scope,
		Through:       through,
		SelectedTick:  selectedTick,
		Payload:       string(payload),
	}, state, nil
}

func validHistoricalRecordingIdentity(
	identity recordings.HistoricalRecordingIdentity,
) (recordings.HistoricalRecordingIdentity, error) {
	if strings.TrimSpace(string(identity.RecordingID)) == "" ||
		strings.TrimSpace(string(identity.Artifact)) == "" {
		return recordings.HistoricalRecordingIdentity{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorInvalidRequest, identity, "", nil,
		)
	}
	return identity, nil
}

type legacyArtifactDocument struct {
	SchemaVersion string                            `json:"schemaVersion"`
	RecordedAt    time.Time                         `json:"recordedAt"`
	Events        []factorydefinitions.FactoryEvent `json:"events"`
}

func decodeHistoricalArtifact(
	payload []byte,
	identity recordings.HistoricalRecordingIdentity,
	inferScope bool,
) ([]recordings.CanonicalEvent, int, recordings.RecordingStatusFacts, []string, error) {
	if replayimpl.IsReplayV2Artifact(payload) {
		return decodeReplayV2Artifact(payload, identity, inferScope)
	}
	var header struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(payload, &header); err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, nil, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err,
		)
	}
	if header.SchemaVersion == string(recordings.PortableArtifactSchemaV1) {
		return decodePortableArtifact(payload, identity, inferScope)
	}
	if header.SchemaVersion != factorydefinitions.ReplayV1SourceFormat {
		return nil, 0, recordings.RecordingStatusFacts{}, nil, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", nil,
		)
	}
	events, selectedTick, status, err := decodeLegacyArtifact(payload, identity, inferScope)
	return events, selectedTick, status, nil, err
}

func decodePortableArtifact(
	payload []byte,
	identity recordings.HistoricalRecordingIdentity,
	inferScope bool,
) ([]recordings.CanonicalEvent, int, recordings.RecordingStatusFacts, []string, error) {
	var artifact recordings.PortableArtifact
	diagnostics, err := jsoncompat.Decode(payload, &artifact)
	if err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, nil, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err,
		)
	}
	if inferScope {
		identity, err = recordedScope(identity, artifact.Summary.Scope.FactorySessionID)
		if err != nil {
			return nil, 0, recordings.RecordingStatusFacts{}, nil, err
		}
	}
	if err := validatePortableArtifact(artifact, identity); err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, nil, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err,
		)
	}
	events := append([]recordings.CanonicalEvent(nil), artifact.Events...)
	if err := validateHistoricalEvents(identity, events); err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, nil, err
	}
	selectedTick := maxHistoricalTick(events)
	status := historicalRecordingStatus(identity, artifact.Summary.State, events)
	status.Failures = append([]recordings.RecordingFailure(nil), artifact.Summary.Failures...)
	return events, selectedTick, status, diagnostics.Paths(), nil
}

func validatePortableArtifact(
	artifact recordings.PortableArtifact,
	identity recordings.HistoricalRecordingIdentity,
) error {
	if err := validatePortableArtifactSummary(artifact, identity); err != nil {
		return err
	}
	if err := validatePortableArtifactCursors(artifact); err != nil {
		return err
	}
	return validatePortableArtifactIntegrity(artifact)
}

func validatePortableArtifactSummary(
	artifact recordings.PortableArtifact,
	identity recordings.HistoricalRecordingIdentity,
) error {
	summary := artifact.Summary
	if artifact.SchemaVersion != recordings.PortableArtifactSchemaV1 ||
		summary.RecordingID != identity.RecordingID ||
		(summary.Reference != "" && summary.Reference != identity.Artifact) ||
		summary.Scope != identity.Scope || !summary.Available ||
		summary.EventCount != len(artifact.Events) ||
		(summary.State != recordings.RecordingFinalized && summary.State != recordings.RecordingFailed) {
		return errors.New("portable artifact summary is inconsistent")
	}
	return nil
}

func validatePortableArtifactCursors(artifact recordings.PortableArtifact) error {
	summary := artifact.Summary
	if len(artifact.Events) == 0 {
		if summary.FirstCursor != nil || summary.LastCursor != nil {
			return errors.New("portable artifact empty cursor bounds are invalid")
		}
	} else if summary.FirstCursor == nil || summary.LastCursor == nil ||
		*summary.FirstCursor != artifact.Events[0].Cursor ||
		*summary.LastCursor != artifact.Events[len(artifact.Events)-1].Cursor {
		return errors.New("portable artifact cursor bounds are invalid")
	}
	return nil
}

func validatePortableArtifactIntegrity(artifact recordings.PortableArtifact) error {
	if artifact.Integrity.Algorithm != recordings.PortableArtifactIntegritySHA256 {
		return errors.New("portable artifact integrity algorithm is invalid")
	}
	expected, err := portableArtifactDigest(artifact)
	if err != nil || artifact.Integrity.Digest != expected {
		return errors.New("portable artifact integrity digest is invalid")
	}
	return nil
}

func portableArtifactDigest(artifact recordings.PortableArtifact) (string, error) {
	artifact.Integrity.Digest = ""
	payload, err := json.Marshal(artifact)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return recordings.PortableArtifactIntegritySHA256 + ":" + hex.EncodeToString(digest[:]), nil
}

func decodeLegacyArtifact(
	payload []byte,
	identity recordings.HistoricalRecordingIdentity,
	inferScope bool,
) ([]recordings.CanonicalEvent, int, recordings.RecordingStatusFacts, error) {
	var artifact legacyArtifactDocument
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&artifact); err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err,
		)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, 0, recordings.RecordingStatusFacts{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err,
		)
	}
	if artifact.SchemaVersion != factorydefinitions.ReplayV1SourceFormat || artifact.RecordedAt.IsZero() {
		return nil, 0, recordings.RecordingStatusFacts{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", nil,
		)
	}
	if inferScope {
		var err error
		identity, err = recordedEventScope(identity, artifact.Events)
		if err != nil {
			return nil, 0, recordings.RecordingStatusFacts{}, err
		}
	}
	events := make([]recordings.CanonicalEvent, len(artifact.Events))
	generationID := "historical-recording/" + string(identity.RecordingID)
	for index, event := range artifact.Events {
		canonicalEvent := canonical.CanonicalEventFromFactory(event, generationID)
		if event.SchemaVersion != factorydefinitions.FactoryEventSchemaVersionV1 ||
			event.Context.Sequence != index || event.Context.Tick < 0 ||
			canonicalEvent.Scope != identity.Scope || !canonical.ValidAppendEvent(canonicalEvent) {
			return nil, 0, recordings.RecordingStatusFacts{}, historicalQueryError(
				recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, canonicalEvent.ID, nil,
			)
		}
		events[index] = canonicalEvent
	}
	if err := validateHistoricalEvents(identity, events); err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, err
	}
	return events, maxHistoricalTick(events), historicalRecordingStatus(identity, recordings.RecordingFinalized, events), nil
}

func decodeReplayV2Artifact(
	payload []byte,
	identity recordings.HistoricalRecordingIdentity,
	inferScope bool,
) ([]recordings.CanonicalEvent, int, recordings.RecordingStatusFacts, []string, error) {
	stream, err := replayimpl.ParseReplayV2(payload)
	if err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, nil, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err,
		)
	}
	if inferScope {
		identity, err = recordedEventScope(identity, stream.Events)
		if err != nil {
			return nil, 0, recordings.RecordingStatusFacts{}, nil, err
		}
	}
	events := make([]recordings.CanonicalEvent, len(stream.Events))
	generationID := "historical-recording/" + string(identity.RecordingID)
	for index, event := range stream.Events {
		canonicalEvent := canonical.CanonicalEventFromFactory(event, generationID)
		if event.SchemaVersion != factorydefinitions.FactoryEventSchemaVersionV1 ||
			event.Context.Sequence != index || event.Context.Tick < 0 ||
			canonicalEvent.Scope != identity.Scope || !canonical.ValidAppendEvent(canonicalEvent) {
			return nil, 0, recordings.RecordingStatusFacts{}, nil, historicalQueryError(
				recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, canonicalEvent.ID, nil,
			)
		}
		events[index] = canonicalEvent
	}
	if err := validateHistoricalEvents(identity, events); err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, nil, err
	}
	state := recordings.RecordingActive
	status := historicalRecordingStatus(identity, state, events)
	if stream.Terminal != nil {
		status.State = recordings.RecordingFinalized
		if strings.EqualFold(stream.Terminal.TerminalState, string(recordings.RecordingFailed)) ||
			strings.EqualFold(stream.Terminal.TerminalState, "FAILED") {
			status.State = recordings.RecordingFailed
		}
		finishedAt := stream.Terminal.FinishedAt
		status.FinalizedAt = &finishedAt
		for _, code := range stream.Terminal.FlushDiagnostics.FailureCodes {
			status.Failures = append(status.Failures, recordings.RecordingFailure{
				Code:       code,
				Message:    "recording persistence failure",
				RecordedAt: finishedAt,
			})
		}
	}
	return events, maxHistoricalTick(events), status, nil, nil
}

func validateHistoricalEvents(identity recordings.HistoricalRecordingIdentity, events []recordings.CanonicalEvent) error {
	for index, event := range events {
		if !canonical.ValidAppendEvent(event) || event.Scope != identity.Scope ||
			event.Cursor.Sequence != event.Sequence || event.Sequence < 0 {
			return historicalQueryError(
				recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, event.ID, nil,
			)
		}
		if index > 0 && (events[index-1].Cursor.StreamGenerationID != event.Cursor.StreamGenerationID ||
			events[index-1].Sequence >= event.Sequence) {
			return historicalQueryError(
				recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, event.ID, nil,
			)
		}
	}
	if err := canonical.ValidateProjectionEvents(identity.Scope, nil, events); err != nil {
		return historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", fmt.Errorf("validate canonical order: %w", err),
		)
	}
	return nil
}

func maxHistoricalTick(events []recordings.CanonicalEvent) int {
	selectedTick := 0
	for _, event := range events {
		if event.FactoryTick > selectedTick {
			selectedTick = event.FactoryTick
		}
	}
	return selectedTick
}

func historicalRecordingStatus(
	identity recordings.HistoricalRecordingIdentity,
	state recordings.RecordingLifecycleState,
	events []recordings.CanonicalEvent,
) recordings.RecordingStatusFacts {
	status := recordings.RecordingStatusFacts{
		RecordingID:    identity.RecordingID,
		Artifact:       identity.Artifact,
		Scope:          identity.Scope,
		State:          state,
		AcceptedEvents: len(events),
	}
	if len(events) > 0 {
		last := events[len(events)-1].Cursor
		status.LastEvent = &last
		status.FlushedThrough = &last
	}
	return status
}

func historicalQueryError(
	kind recordings.HistoricalRecordingQueryErrorKind,
	identity recordings.HistoricalRecordingIdentity,
	eventID recordings.CanonicalEventID,
	cause error,
) error {
	return &recordings.HistoricalRecordingQueryError{
		Kind:        kind,
		RecordingID: identity.RecordingID,
		EventID:     eventID,
		Cause:       cause,
	}
}
