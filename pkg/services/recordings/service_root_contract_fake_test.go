package recordings_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type peerRecordingSession struct {
	artifact       recordings.RecordingArtifactReference
	scope          recordings.CanonicalEventScope
	events         []recordings.CanonicalEvent
	flushedThrough *recordings.CanonicalEventCursor
	failures       []recordings.RecordingFailure
	finalizedAt    *time.Time
}

type peerReplayPlan struct {
	facts           recordings.ReplayPlanFacts
	events          []recordings.CanonicalEvent
	expectedThrough *recordings.CanonicalEventCursor
	selectedTick    int
	processed       int
}

// peerRootServiceFake is a peer-shaped Recordings root Service that imports
// only the published Recordings root package. It never imports another service
// contract or Recordings implementation packages.
type peerRootServiceFake struct {
	events              []recordings.CanonicalEvent
	streamGenerationID  string
	subscribeErr        error
	reconstructErr      error
	dashboardData       recordings.SimpleDashboardRenderData
	workstationRequests recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice
	validateReplayErr   error
	recordings          map[string]*peerRecordingSession
	nextRecordingID     int
	replayPlans         map[recordings.ReplayPlanHandle]*peerReplayPlan
	nextReplayPlan      int
}

var _ recordings.Service = (*peerRootServiceFake)(nil)

func (fake *peerRootServiceFake) Append(
	request recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	if !validPeerAppendEvent(request.Event) {
		return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
	}
	request.Event.Sequence = recordings.CanonicalEventSequence(len(fake.events))
	request.Event.Cursor = recordings.CanonicalEventCursor{
		StreamGenerationID: fake.streamGenerationID,
		Sequence:           request.Event.Sequence,
	}
	fake.events = append(fake.events, request.Event)
	return recordings.AppendRecordedEventResult{Event: request.Event}, nil
}

func validPeerAppendEvent(event recordings.CanonicalEvent) bool {
	return strings.TrimSpace(string(event.ID)) != "" &&
		strings.TrimSpace(string(event.Kind)) != "" &&
		!event.RecordedAt.IsZero() &&
		(event.Scope.FactorySessionID == "" ||
			strings.TrimSpace(event.Scope.FactorySessionID) != "") &&
		json.Valid([]byte(event.Payload))
}

func (fake *peerRootServiceFake) SubscribeFrom(
	_ context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if err := invalidSubscribeScope(request.Scope); err != nil {
		return recordings.SubscribeResult{}, err
	}
	if fake.subscribeErr != nil {
		return recordings.SubscribeResult{}, fake.subscribeErr
	}
	start := 0
	if request.Cursor != nil {
		if request.Cursor.StreamGenerationID == "" || request.Cursor.Sequence < 0 {
			return recordings.SubscribeResult{}, recordings.ErrInvalidReconnectCursor
		}
		if request.Cursor.StreamGenerationID != fake.streamGenerationID {
			return recordings.SubscribeResult{}, recordings.ErrReconnectCursorUnavailable
		}
		start = int(request.Cursor.Sequence) + 1
		if start > len(fake.events) {
			return recordings.SubscribeResult{}, recordings.ErrReconnectCursorExpired
		}
	}
	outcomes := make([]recordings.SubscriptionOutcome, 0, len(fake.events)-start)
	for _, event := range fake.events[start:] {
		outcomes = append(outcomes, recordings.SubscriptionOutcome{
			Kind:  recordings.SubscriptionEvent,
			Event: event,
		})
	}
	return recordings.SubscribeResult{
		Subscription: (&peerEventSubscription{outcomes: outcomes}).Next,
	}, nil
}

func invalidSubscribeScope(scope recordings.CanonicalEventScope) error {
	if scope.FactorySessionID != "" && strings.TrimSpace(scope.FactorySessionID) == "" {
		return recordings.ErrInvalidSubscribeScope
	}
	return nil
}

type peerEventSubscription struct {
	outcomes []recordings.SubscriptionOutcome
}

func (subscription *peerEventSubscription) Next(context.Context) recordings.SubscriptionOutcome {
	if len(subscription.outcomes) == 0 {
		return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
	}
	outcome := subscription.outcomes[0]
	subscription.outcomes = subscription.outcomes[1:]
	return outcome
}

func (fake *peerRootServiceFake) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	if request.SelectedTick < 0 {
		return recordings.ReconstructWorldStateResult{}, recordings.ErrInvalidProjectionInput
	}
	if fake.reconstructErr != nil {
		return recordings.ReconstructWorldStateResult{}, fake.reconstructErr
	}
	payload, err := json.Marshal(struct {
		SelectedTick int `json:"selectedTick"`
		EventCount   int `json:"eventCount"`
	}{
		SelectedTick: request.SelectedTick,
		EventCount:   len(request.Events),
	})
	if err != nil {
		return recordings.ReconstructWorldStateResult{}, err
	}
	through := recordings.CanonicalEventCursor{}
	if request.After != nil {
		through = *request.After
	}
	if len(request.Events) > 0 {
		through = request.Events[len(request.Events)-1].Cursor
	}
	return recordings.ReconstructWorldStateResult{WorldState: recordings.WorldStateView{
		SchemaVersion: recordings.WorldStateViewSchemaV1,
		Scope:         request.Scope,
		Through:       through,
		SelectedTick:  request.SelectedTick,
		Payload:       string(payload),
	}}, nil
}

func (fake *peerRootServiceFake) QuerySimpleDashboard(
	request recordings.SimpleDashboardQueryRequest,
) (recordings.SimpleDashboardQueryResult, error) {
	if request.WorldState.SchemaVersion != recordings.WorldStateViewSchemaV1 {
		return recordings.SimpleDashboardQueryResult{}, recordings.ErrUnsupportedProjectionView
	}
	return recordings.SimpleDashboardQueryResult{
		Data: fake.dashboardData,
	}, nil
}

func (fake *peerRootServiceFake) QueryWorkstationRequests(
	request recordings.WorkstationRequestsQueryRequest,
) (recordings.WorkstationRequestsQueryResult, error) {
	if request.WorldState.SchemaVersion != recordings.WorldStateViewSchemaV1 {
		return recordings.WorkstationRequestsQueryResult{}, recordings.ErrUnsupportedProjectionView
	}
	return recordings.WorkstationRequestsQueryResult{
		Projection: fake.workstationRequests,
	}, nil
}

func (fake *peerRootServiceFake) ValidateReconnectReplayFrom(
	_ recordings.ValidateReconnectReplayRequest,
) error {
	return fake.validateReplayErr
}

func (fake *peerRootServiceFake) ensureRecordings() {
	if fake.recordings == nil {
		fake.recordings = make(map[string]*peerRecordingSession)
	}
}

func (fake *peerRootServiceFake) recordingSession(
	id recordings.RecordingID,
) (*peerRecordingSession, error) {
	fake.ensureRecordings()
	if strings.TrimSpace(string(id)) == "" {
		return nil, recordings.ErrMissingRecordingTarget
	}
	session, ok := fake.recordings[string(id)]
	if !ok {
		return nil, recordings.ErrMissingRecordingTarget
	}
	return session, nil
}

func (fake *peerRootServiceFake) BindRecording(
	request recordings.BindRecordingRequest,
) (recordings.BindRecordingResult, error) {
	if strings.TrimSpace(string(request.Artifact)) == "" {
		return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
	}
	if request.Scope.FactorySessionID != "" &&
		strings.TrimSpace(request.Scope.FactorySessionID) == "" {
		return recordings.BindRecordingResult{}, recordings.ErrInvalidRecordingScope
	}
	fake.ensureRecordings()
	id := strings.TrimSpace(string(request.RecordingID))
	if id == "" {
		for {
			fake.nextRecordingID++
			id = "recording-" + strconv.Itoa(fake.nextRecordingID)
			if _, exists := fake.recordings[id]; !exists {
				break
			}
		}
	} else if existing, exists := fake.recordings[id]; exists {
		if existing.artifact != request.Artifact || existing.scope != request.Scope {
			return recordings.BindRecordingResult{}, recordings.ErrRecordingBindingConflict
		}
		return recordings.BindRecordingResult{
			Status: peerRecordingStatus(recordings.RecordingID(id), existing),
		}, nil
	}
	session := &peerRecordingSession{
		artifact: request.Artifact,
		scope:    request.Scope,
	}
	fake.recordings[id] = session
	return recordings.BindRecordingResult{
		Status: peerRecordingStatus(recordings.RecordingID(id), session),
	}, nil
}

func (fake *peerRootServiceFake) RecordRecordingEvent(
	request recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.RecordRecordingEventResult{}, err
	}
	if session.finalizedAt != nil {
		return recordings.RecordRecordingEventResult{}, recordings.ErrRecordingWriteRejected
	}
	if !validPeerRecordingEvent(session, request.Event) {
		return recordings.RecordRecordingEventResult{}, recordings.ErrInvalidRecordingEvent
	}
	session.events = append(session.events, request.Event)
	return recordings.RecordRecordingEventResult{
		Status: peerRecordingStatus(request.RecordingID, session),
	}, nil
}

func (fake *peerRootServiceFake) RecordRecordingError(
	request recordings.RecordRecordingErrorRequest,
) (recordings.RecordRecordingErrorResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.RecordRecordingErrorResult{}, err
	}
	if session.finalizedAt != nil {
		return recordings.RecordRecordingErrorResult{}, recordings.ErrRecordingWriteRejected
	}
	if strings.TrimSpace(request.Failure.Code) == "" ||
		strings.TrimSpace(request.Failure.Message) == "" {
		return recordings.RecordRecordingErrorResult{}, recordings.ErrInvalidRecordingFailure
	}
	session.failures = append(session.failures, request.Failure)
	return recordings.RecordRecordingErrorResult{
		Status: peerRecordingStatus(request.RecordingID, session),
	}, nil
}

func (fake *peerRootServiceFake) FlushRecording(
	request recordings.FlushRecordingRequest,
) (recordings.FlushRecordingResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.FlushRecordingResult{}, err
	}
	if len(session.events) > 0 {
		cursor := session.events[len(session.events)-1].Cursor
		session.flushedThrough = &cursor
	}
	return recordings.FlushRecordingResult{
		Status: peerRecordingStatus(request.RecordingID, session),
	}, nil
}

func (fake *peerRootServiceFake) FinishRecording(
	request recordings.FinishRecordingRequest,
) (recordings.FinishRecordingResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.FinishRecordingResult{}, err
	}
	if session.finalizedAt == nil {
		finishedAt := request.FinishedAt.UTC()
		session.finalizedAt = &finishedAt
	}
	return recordings.FinishRecordingResult{
		Status: peerRecordingStatus(request.RecordingID, session),
	}, nil
}

func (fake *peerRootServiceFake) QueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.RecordingStatusResult{}, err
	}
	return recordings.RecordingStatusResult{
		Status: peerRecordingStatus(request.RecordingID, session),
	}, nil
}

func validPeerRecordingEvent(
	session *peerRecordingSession,
	event recordings.CanonicalEvent,
) bool {
	if !validPeerAppendEvent(event) || event.Scope != session.scope ||
		event.Sequence < 0 || event.Cursor.StreamGenerationID == "" ||
		event.Cursor.Sequence != event.Sequence {
		return false
	}
	if len(session.events) == 0 {
		return true
	}
	previous := session.events[len(session.events)-1]
	return event.Cursor.StreamGenerationID == previous.Cursor.StreamGenerationID &&
		event.Sequence > previous.Sequence
}

func peerRecordingStatus(
	id recordings.RecordingID,
	session *peerRecordingSession,
) recordings.RecordingStatusFacts {
	state := recordings.RecordingActive
	if len(session.failures) > 0 {
		state = recordings.RecordingFailed
	} else if session.finalizedAt != nil {
		state = recordings.RecordingFinalized
	}
	status := recordings.RecordingStatusFacts{
		RecordingID:    id,
		Artifact:       session.artifact,
		Scope:          session.scope,
		State:          state,
		AcceptedEvents: len(session.events),
		Failures:       append([]recordings.RecordingFailure(nil), session.failures...),
	}
	if len(session.events) > 0 {
		cursor := session.events[len(session.events)-1].Cursor
		status.LastEvent = &cursor
	}
	if session.flushedThrough != nil {
		cursor := *session.flushedThrough
		status.FlushedThrough = &cursor
	}
	if session.finalizedAt != nil {
		finalizedAt := *session.finalizedAt
		status.FinalizedAt = &finalizedAt
	}
	return status
}

func (fake *peerRootServiceFake) LoadReplayRecording(
	request recordings.LoadReplayRecordingRequest,
) (recordings.LoadReplayRecordingResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil {
		return recordings.LoadReplayRecordingResult{}, recordings.ErrReplayRecordingNotFound
	}
	if session.finalizedAt == nil {
		return recordings.LoadReplayRecordingResult{}, recordings.ErrReplayRecordingNotFinalized
	}
	return recordings.LoadReplayRecordingResult{
		Recording: recordings.ReplayRecordingFacts{
			RecordingID: request.RecordingID,
			Scope:       session.scope,
			Events:      append([]recordings.CanonicalEvent(nil), session.events...),
		},
	}, nil
}

func validPeerReplayPlan(request recordings.CreateReplayPlanRequest) error {
	if request.SchemaVersion != recordings.ReplayPlanSchemaV1 ||
		request.Timing != recordings.ReplayTimingOrderOnly ||
		request.SelectedTick < 0 {
		return recordings.ErrUnsupportedReplayPlan
	}
	if strings.TrimSpace(string(request.Recording.RecordingID)) == "" {
		return recordings.ErrCorruptReplayInput
	}
	previous := recordings.CanonicalEventSequence(-1)
	generationID := ""
	for index, event := range request.Recording.Events {
		if !validPeerReplayEvent(
			request.Recording.Scope,
			event,
			recordings.CanonicalEventSequence(index),
			previous,
			generationID,
		) {
			return recordings.ErrCorruptReplayInput
		}
		previous = event.Sequence
		generationID = event.Cursor.StreamGenerationID
	}
	return nil
}

func validPeerReplayEvent(
	scope recordings.CanonicalEventScope,
	event recordings.CanonicalEvent,
	expected recordings.CanonicalEventSequence,
	previous recordings.CanonicalEventSequence,
	generationID string,
) bool {
	return validPeerAppendEvent(event) &&
		event.Scope == scope &&
		event.Cursor.Sequence == event.Sequence &&
		event.Cursor.StreamGenerationID != "" &&
		(scope.FactorySessionID != "" || event.Sequence == expected) &&
		(scope.FactorySessionID == "" || event.Sequence > previous) &&
		(generationID == "" || event.Cursor.StreamGenerationID == generationID)
}

func (fake *peerRootServiceFake) CreateReplayPlan(
	request recordings.CreateReplayPlanRequest,
) (recordings.CreateReplayPlanResult, error) {
	if err := validPeerReplayPlan(request); err != nil {
		return recordings.CreateReplayPlanResult{}, err
	}
	if fake.replayPlans == nil {
		fake.replayPlans = make(map[recordings.ReplayPlanHandle]*peerReplayPlan)
	}
	fake.nextReplayPlan++
	handle := recordings.ReplayPlanHandle("peer-replay-" + strconv.Itoa(fake.nextReplayPlan))
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
	fake.replayPlans[handle] = &peerReplayPlan{
		facts:           facts,
		events:          append([]recordings.CanonicalEvent(nil), request.Recording.Events...),
		expectedThrough: expected,
		selectedTick:    request.SelectedTick,
	}
	return recordings.CreateReplayPlanResult{Plan: facts}, nil
}

func (fake *peerRootServiceFake) ObserveReplay(
	request recordings.ObserveReplayRequest,
) (recordings.ObserveReplayResult, error) {
	plan, ok := fake.replayPlans[request.Plan]
	if !ok {
		return recordings.ObserveReplayResult{}, recordings.ErrReplayPlanNotFound
	}
	if plan.processed < len(plan.events) {
		plan.processed++
	}
	reduced, err := fake.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
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
		if divergence := peerReplayDivergence(plan, observation.Through); divergence != nil {
			observation.Kind = recordings.ReplayDiverged
			observation.Divergence = divergence
		}
	}
	return recordings.ObserveReplayResult{Observation: observation}, nil
}

func peerReplayDivergence(
	plan *peerReplayPlan,
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

func (fake *peerRootServiceFake) BuildPortableArtifact(
	request recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	session, err := fake.recordingSession(request.RecordingID)
	if err != nil || session.finalizedAt == nil {
		return recordings.BuildPortableArtifactResult{}, recordings.ErrPortableArtifactUnavailable
	}
	status := peerRecordingStatus(request.RecordingID, session)
	summary := recordings.PortableArtifactSummary{
		RecordingID: status.RecordingID,
		Reference:   status.Artifact,
		Scope:       status.Scope,
		State:       status.State,
		EventCount:  len(session.events),
		Failures:    append([]recordings.RecordingFailure{}, status.Failures...),
		Available:   true,
	}
	if len(session.events) > 0 {
		first := session.events[0].Cursor
		last := session.events[len(session.events)-1].Cursor
		summary.FirstCursor = &first
		summary.LastCursor = &last
	}
	artifact := recordings.PortableArtifact{
		SchemaVersion: recordings.PortableArtifactSchemaV1,
		Summary:       summary,
		Events:        append([]recordings.CanonicalEvent{}, session.events...),
		Integrity: recordings.PortableArtifactIntegrity{
			Algorithm: recordings.PortableArtifactIntegritySHA256,
		},
	}
	artifact.Integrity.Digest = peerPortableArtifactDigest(artifact)
	if err := peerValidatePortableArtifact(artifact); err != nil {
		return recordings.BuildPortableArtifactResult{}, err
	}
	return recordings.BuildPortableArtifactResult{Artifact: artifact}, nil
}

func (fake *peerRootServiceFake) ValidatePortableArtifact(
	request recordings.ValidatePortableArtifactRequest,
) (recordings.ValidatePortableArtifactResult, error) {
	if err := peerValidatePortableArtifact(request.Artifact); err != nil {
		return recordings.ValidatePortableArtifactResult{}, err
	}
	return recordings.ValidatePortableArtifactResult{Summary: request.Artifact.Summary}, nil
}

func (fake *peerRootServiceFake) EncodePortableArtifact(
	request recordings.EncodePortableArtifactRequest,
) (recordings.EncodePortableArtifactResult, error) {
	if err := peerValidatePortableArtifact(request.Artifact); err != nil {
		return recordings.EncodePortableArtifactResult{}, err
	}
	payload, err := json.Marshal(request.Artifact)
	if err != nil {
		return recordings.EncodePortableArtifactResult{}, recordings.ErrInvalidPortableArtifact
	}
	return recordings.EncodePortableArtifactResult{Payload: payload}, nil
}

func (fake *peerRootServiceFake) DecodePortableArtifact(
	request recordings.DecodePortableArtifactRequest,
) (recordings.DecodePortableArtifactResult, error) {
	if len(request.Payload) == 0 {
		return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidPortableArtifact
	}
	var artifact recordings.PortableArtifact
	if err := json.Unmarshal(request.Payload, &artifact); err != nil {
		return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidPortableArtifact
	}
	if _, err := fake.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: artifact,
	}); err != nil {
		return recordings.DecodePortableArtifactResult{}, err
	}
	return recordings.DecodePortableArtifactResult{Artifact: artifact}, nil
}

func (fake *peerRootServiceFake) SummarizePortableArtifact(
	request recordings.SummarizePortableArtifactRequest,
) (recordings.SummarizePortableArtifactResult, error) {
	if err := peerValidatePortableArtifact(request.Artifact); err != nil {
		return recordings.SummarizePortableArtifactResult{}, err
	}
	return recordings.SummarizePortableArtifactResult{
		Summary: request.Artifact.Summary,
	}, nil
}

func peerValidatePortableArtifact(artifact recordings.PortableArtifact) error {
	if artifact.SchemaVersion != recordings.PortableArtifactSchemaV1 {
		return recordings.ErrUnsupportedPortableArtifactSchema
	}
	if artifact.Summary.RecordingID == "" || !artifact.Summary.Available ||
		artifact.Summary.EventCount != len(artifact.Events) {
		return recordings.ErrInvalidPortableArtifact
	}
	for index, event := range artifact.Events {
		if event.Scope != artifact.Summary.Scope ||
			event.Cursor.Sequence != event.Sequence ||
			(index > 0 && event.Sequence != artifact.Events[index-1].Sequence+1) {
			return recordings.ErrInvalidPortableArtifactOrder
		}
	}
	if artifact.Integrity.Algorithm != recordings.PortableArtifactIntegritySHA256 ||
		artifact.Integrity.Digest != peerPortableArtifactDigest(artifact) {
		return recordings.ErrInvalidPortableArtifactIntegrity
	}
	return nil
}

func peerPortableArtifactDigest(artifact recordings.PortableArtifact) string {
	artifact.Integrity.Digest = ""
	payload, _ := json.Marshal(artifact)
	digest := sha256.Sum256(payload)
	return recordings.PortableArtifactIntegritySHA256 + ":" +
		hex.EncodeToString(digest[:])
}
