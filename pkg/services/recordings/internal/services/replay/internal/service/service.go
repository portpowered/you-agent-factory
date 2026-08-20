package service

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordingsreplay "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay"
)

type neutralReplayPlan struct {
	facts           recordings.ReplayPlanFacts
	events          []recordings.CanonicalEvent
	expectedThrough *recordings.CanonicalEventCursor
	selectedTick    int
	processed       int
}

// Service keeps neutral replay load/plan/observe behavior behind the
// Recordings-owned replay capability.
type Service struct {
	lifecycle             recordinglifecycle.Service
	projection            recordings.ProjectionService
	readFile              recordings.RecordingReadFile
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder
	mu                    sync.Mutex
	replayPlans           map[recordings.ReplayPlanHandle]*neutralReplayPlan
	nextPlan              int
}

var _ recordingsreplay.Service = (*Service)(nil)

// New constructs the private replay owner.
func New(
	lifecycle recordinglifecycle.Service,
	projection recordings.ProjectionService,
) *Service {
	return NewWithResumeSource(lifecycle, projection, nil, nil)
}

// NewWithResumeSource constructs the private replay owner with the optional
// read-side source used by explicit resume intent. Neutral replay remains
// independent of this source and retains its finalized-only contract.
func NewWithResumeSource(
	lifecycle recordinglifecycle.Service,
	projection recordings.ProjectionService,
	readFile recordings.RecordingReadFile,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
) *Service {
	return &Service{
		lifecycle:             lifecycle,
		projection:            projection,
		readFile:              readFile,
		decodeFactorySnapshot: decodeFactorySnapshot,
		replayPlans:           make(map[recordings.ReplayPlanHandle]*neutralReplayPlan),
	}
}

func (service *Service) LoadReplayRecording(
	request recordings.LoadReplayRecordingRequest,
) (recordings.LoadReplayRecordingResult, error) {
	snapshot, err := service.lifecycle.Snapshot(request.RecordingID)
	if errors.Is(err, recordings.ErrMissingRecordingTarget) {
		return recordings.LoadReplayRecordingResult{}, recordings.ErrReplayRecordingNotFound
	}
	if err != nil {
		return recordings.LoadReplayRecordingResult{}, err
	}
	if snapshot.Status.FinalizedAt == nil {
		return recordings.LoadReplayRecordingResult{}, recordings.ErrReplayRecordingNotFinalized
	}
	return recordings.LoadReplayRecordingResult{
		Recording: recordings.ReplayRecordingFacts{
			RecordingID: request.RecordingID,
			Scope:       snapshot.Status.Scope,
			Events:      append([]recordings.CanonicalEvent(nil), snapshot.Events...),
		},
	}, nil
}

func (service *Service) LoadReplayRecordingForResume(
	request recordings.LoadReplayRecordingForResumeRequest,
) (recordings.LoadReplayRecordingForResumeResult, error) {
	snapshot, err := service.lifecycle.Snapshot(request.RecordingID)
	if errors.Is(err, recordings.ErrMissingRecordingTarget) {
		return recordings.LoadReplayRecordingForResumeResult{}, recordings.ErrReplayRecordingNotFound
	}
	if err != nil {
		return recordings.LoadReplayRecordingForResumeResult{}, err
	}
	events, skippedTrailingBlocks, err := service.resumeEvents(request.RecordingID, snapshot)
	if err != nil {
		return recordings.LoadReplayRecordingForResumeResult{}, err
	}
	return recordings.LoadReplayRecordingForResumeResult{
		Recording: recordings.ReplayRecordingFacts{
			RecordingID: request.RecordingID,
			Scope:       snapshot.Status.Scope,
			Events:      events,
		},
		RecoveredEventCount:   len(events),
		Truncated:             skippedTrailingBlocks > 0,
		SkippedTrailingBlocks: skippedTrailingBlocks,
	}, nil
}

func (service *Service) resumeEvents(
	recordingID recordings.RecordingID,
	snapshot recordinglifecycle.Snapshot,
) ([]recordings.CanonicalEvent, int, error) {
	if service.readFile == nil || service.decodeFactorySnapshot == nil {
		return append([]recordings.CanonicalEvent(nil), snapshot.Events...), 0, nil
	}
	path := string(snapshot.Status.Artifact)
	if strings.TrimSpace(path) == "" {
		return nil, 0, fmt.Errorf("resume recording source is missing for %q", recordingID)
	}
	data, err := service.readFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read resume recording %q: %w", path, err)
	}
	parsed, err := replayimpl.ArtifactFromEventStream(
		bytes.NewReader(data),
		service.decodeFactorySnapshot,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("parse resume recording %q: %w", path, err)
	}
	events, err := canonicalEventsFromResumeArtifact(
		recordingID,
		snapshot.Status.Scope,
		snapshot.Events,
		parsed.Artifact.Events,
	)
	if err != nil {
		return nil, 0, err
	}
	return events, parsed.SkippedTrailingBlocks, nil
}

func canonicalEventsFromResumeArtifact(
	recordingID recordings.RecordingID,
	scope recordings.CanonicalEventScope,
	retained []recordings.CanonicalEvent,
	factoryEvents []factorydefinitions.FactoryEvent,
) ([]recordings.CanonicalEvent, error) {
	generationID := "resume-recording/" + string(recordingID)
	if len(retained) > 0 && retained[0].Cursor.StreamGenerationID != "" {
		generationID = retained[0].Cursor.StreamGenerationID
	}
	events := make([]recordings.CanonicalEvent, len(factoryEvents))
	for index, event := range factoryEvents {
		canonicalEvent := canonical.CanonicalEventFromFactory(event, generationID)
		if canonicalEvent.Scope != (recordings.CanonicalEventScope{}) &&
			canonicalEvent.Scope != scope {
			return nil, fmt.Errorf(
				"resume recording event %q has scope %#v, want %#v: %w",
				event.Id,
				canonicalEvent.Scope,
				scope,
				recordings.ErrCorruptReplayInput,
			)
		}
		canonicalEvent.Scope = scope
		canonicalEvent.Cursor.StreamGenerationID = generationID
		events[index] = canonicalEvent
	}
	for _, event := range events {
		if !canonical.ValidAppendEvent(event) {
			return nil, fmt.Errorf("resume recording contains invalid event %q: %w", event.ID, recordings.ErrCorruptReplayInput)
		}
	}
	if err := canonical.ValidateProjectionEvents(scope, nil, events); err != nil {
		return nil, fmt.Errorf("validate resume recording event order: %w", err)
	}
	return events, nil
}

func validateNeutralReplayPlan(request recordings.CreateReplayPlanRequest) error {
	if request.SchemaVersion != recordings.ReplayPlanSchemaV1 ||
		request.Timing != recordings.ReplayTimingOrderOnly ||
		request.SelectedTick < 0 {
		return recordings.ErrUnsupportedReplayPlan
	}
	if strings.TrimSpace(string(request.Recording.RecordingID)) == "" {
		return recordings.ErrCorruptReplayInput
	}
	for _, event := range request.Recording.Events {
		if !canonical.ValidAppendEvent(event) {
			return recordings.ErrCorruptReplayInput
		}
	}
	if err := canonical.ValidateProjectionEvents(
		request.Recording.Scope,
		nil,
		request.Recording.Events,
	); err != nil {
		return recordings.ErrCorruptReplayInput
	}
	return nil
}

func (service *Service) CreateReplayPlan(
	request recordings.CreateReplayPlanRequest,
) (recordings.CreateReplayPlanResult, error) {
	if err := validateNeutralReplayPlan(request); err != nil {
		return recordings.CreateReplayPlanResult{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	service.nextPlan++
	handle := recordings.ReplayPlanHandle("replay-plan-" + strconv.Itoa(service.nextPlan))
	facts := recordings.ReplayPlanFacts{
		Handle:        handle,
		RecordingID:   request.Recording.RecordingID,
		Scope:         request.Recording.Scope,
		TotalEvents:   len(request.Recording.Events),
		SchemaVersion: request.SchemaVersion,
		Timing:        request.Timing,
	}
	var expected *recordings.CanonicalEventCursor
	if request.ExpectedThrough != nil {
		cursor := *request.ExpectedThrough
		expected = &cursor
	}
	service.replayPlans[handle] = &neutralReplayPlan{
		facts:           facts,
		events:          append([]recordings.CanonicalEvent(nil), request.Recording.Events...),
		expectedThrough: expected,
		selectedTick:    request.SelectedTick,
	}
	return recordings.CreateReplayPlanResult{Plan: facts}, nil
}

func (service *Service) ObserveReplay(
	request recordings.ObserveReplayRequest,
) (recordings.ObserveReplayResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	plan, ok := service.replayPlans[request.Plan]
	if !ok {
		return recordings.ObserveReplayResult{}, recordings.ErrReplayPlanNotFound
	}
	if plan.processed < len(plan.events) {
		plan.processed++
	}
	reduced, err := canonical.ReconstructWorldState(service.projection, recordings.ReconstructWorldStateRequest{
		Scope:        plan.facts.Scope,
		Events:       append([]recordings.CanonicalEvent(nil), plan.events[:plan.processed]...),
		SelectedTick: plan.selectedTick,
	})
	if err != nil {
		return recordings.ObserveReplayResult{}, err
	}
	observation := recordings.ReplayObservation{
		Kind:            recordings.ReplayProgress,
		Plan:            request.Plan,
		ProcessedEvents: plan.processed,
		TotalEvents:     len(plan.events),
		WorldState:      reduced.WorldState,
	}
	if plan.processed > 0 {
		cursor := plan.events[plan.processed-1].Cursor
		observation.Through = &cursor
	}
	if plan.processed == len(plan.events) {
		observation.Kind = recordings.ReplayCompleted
		if replayDivergence := replayPlanDivergence(plan, observation.Through); replayDivergence != nil {
			observation.Kind = recordings.ReplayDiverged
			observation.Divergence = replayDivergence
		}
	}
	return recordings.ObserveReplayResult{Observation: observation}, nil
}

func replayPlanDivergence(
	plan *neutralReplayPlan,
	through *recordings.CanonicalEventCursor,
) *recordings.ReplayDivergenceFacts {
	if plan.expectedThrough == nil {
		return nil
	}
	actual := recordings.CanonicalEventCursor{}
	if through != nil {
		actual = *through
	}
	if actual == *plan.expectedThrough {
		return nil
	}
	return &recordings.ReplayDivergenceFacts{
		Expected: *plan.expectedThrough,
		Observed: actual,
	}
}
