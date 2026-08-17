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
	identity, err := validHistoricalRecordingIdentity(request.Recording)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	if service == nil || service.readArtifact == nil || service.projection == nil {
		return recordings.HistoricalRecordingQueryResult{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorUnavailable, identity, "", nil,
		)
	}
	payload, err := service.readArtifact(string(identity.Artifact))
	if err != nil {
		kind := recordings.HistoricalRecordingQueryErrorUnavailable
		if errors.Is(err, os.ErrNotExist) {
			kind = recordings.HistoricalRecordingQueryErrorMissingHistory
		}
		return recordings.HistoricalRecordingQueryResult{}, historicalQueryError(kind, identity, "", err)
	}
	events, selectedTick, status, err := decodeHistoricalArtifact(payload, identity)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	state, err := canonical.ReconstructWorldState(service.projection, recordings.ReconstructWorldStateRequest{
		Scope:        identity.Scope,
		Events:       events,
		SelectedTick: selectedTick,
	})
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err,
		)
	}
	var factoryState recordings.FactoryWorldState
	if err := json.Unmarshal([]byte(state.WorldState.Payload), &factoryState); err != nil {
		return recordings.HistoricalRecordingQueryResult{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err,
		)
	}
	workstationRequests := service.projection.ProjectWorkstationRequests(factoryState)
	dispatches, err := projectHistoricalDispatches(identity, events)
	if err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	return recordings.HistoricalRecordingQueryResult{
		Recording:           identity,
		Status:              status,
		Events:              append([]recordings.CanonicalEvent(nil), events...),
		WorldState:          state.WorldState,
		WorkstationRequests: workstationRequests,
		Dispatches:          dispatches,
	}, nil
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
) ([]recordings.CanonicalEvent, int, recordings.RecordingStatusFacts, error) {
	var header struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(payload, &header); err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err,
		)
	}
	if header.SchemaVersion == string(recordings.PortableArtifactSchemaV1) {
		return decodePortableArtifact(payload, identity)
	}
	if header.SchemaVersion != factorydefinitions.ReplayV1SourceFormat {
		return nil, 0, recordings.RecordingStatusFacts{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", nil,
		)
	}
	return decodeLegacyArtifact(payload, identity)
}

func decodePortableArtifact(
	payload []byte,
	identity recordings.HistoricalRecordingIdentity,
) ([]recordings.CanonicalEvent, int, recordings.RecordingStatusFacts, error) {
	var artifact recordings.PortableArtifact
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
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
	if err := validatePortableArtifact(artifact, identity); err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, historicalQueryError(
			recordings.HistoricalRecordingQueryErrorCorruptHistory, identity, "", err,
		)
	}
	events := append([]recordings.CanonicalEvent(nil), artifact.Events...)
	if err := validateHistoricalEvents(identity, events); err != nil {
		return nil, 0, recordings.RecordingStatusFacts{}, err
	}
	selectedTick := maxHistoricalTick(events)
	status := historicalRecordingStatus(identity, artifact.Summary.State, events)
	status.Failures = append([]recordings.RecordingFailure(nil), artifact.Summary.Failures...)
	return events, selectedTick, status, nil
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
