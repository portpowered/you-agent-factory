// Package durable contains the provider-neutral Worker recording projection
// and replay policy used by Worker Sessions after a process restart.
package durable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	lifecycleSourceType           events.SourceType     = "worker_session_lifecycle"
	terminalSourceSequence        events.SourceSequence = 2
	terminalSourceEventID         events.SourceEventID  = "terminal"
	workerRecordingGenerationBase                       = "worker-recording/"
)

// WorkerRecordingHistoryReader returns the optional history capability from
// the Recordings-owned capture service.
func WorkerRecordingHistoryReader(recording recordings.WorkerSessionRecordingService) recordings.WorkerRecordingHistoryReader {
	if recording == nil {
		return nil
	}
	reader, ok := recording.(recordings.WorkerRecordingHistoryReader)
	if !ok || reader == nil {
		return nil
	}
	return reader
}

// WorkerProjection loads and replays one Worker history by its stable ID.
func WorkerProjection(
	recording recordings.WorkerSessionRecordingService,
	ctx context.Context,
	workerSessionID string,
) (recordings.WorkerRecordingProjection, bool, error) {
	reader := WorkerRecordingHistoryReader(recording)
	if reader == nil {
		return recordings.WorkerRecordingProjection{}, false, nil
	}
	workerSessionID = strings.TrimSpace(workerSessionID)
	snapshot, err := reader.LoadWorkerRecordingByWorkerSessionID(ctx, workerSessionID)
	if err != nil {
		if errors.Is(err, recordings.ErrWorkerRecordingReplay) || errors.Is(err, recordings.ErrMissingWorkerRecordingReader) {
			return recordings.WorkerRecordingProjection{}, false, nil
		}
		if errors.Is(err, recordings.ErrWorkerRecordingCorruptTail) || errors.Is(err, recordings.ErrWorkerRecordingCompatibility) {
			return recordings.WorkerRecordingProjection{}, true, workersessions.ErrObservationRecordingCorrupt
		}
		return recordings.WorkerRecordingProjection{}, true, workersessions.ErrObservationRecordingUnavailable
	}
	result, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
		Snapshot:        snapshot,
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		return recordings.WorkerRecordingProjection{}, true, workersessions.ErrObservationRecordingCorrupt
	}
	return result.Projection, true, nil
}

// ListWorkerRecordingProjections forwards a bounded catalog query.
func ListWorkerRecordingProjections(
	recording recordings.WorkerSessionRecordingService,
	ctx context.Context,
	request recordings.WorkerRecordingListRequest,
) (recordings.WorkerRecordingListResult, error) {
	reader := WorkerRecordingHistoryReader(recording)
	if reader == nil {
		return recordings.WorkerRecordingListResult{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.ListWorkerRecordingProjections(ctx, request)
}

// LoadWorkerRecordingByWorkerSessionID forwards a durable Worker-ID lookup.
func LoadWorkerRecordingByWorkerSessionID(
	recording recordings.WorkerSessionRecordingService,
	ctx context.Context,
	workerSessionID string,
) (recordings.WorkerRecordingSnapshot, error) {
	reader := WorkerRecordingHistoryReader(recording)
	if reader == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.LoadWorkerRecordingByWorkerSessionID(ctx, workerSessionID)
}

// WorkerProjections reads all catalog pages within the fixed read bound.
func WorkerProjections(
	recording recordings.WorkerSessionRecordingService,
	ctx context.Context,
	request recordings.WorkerRecordingListRequest,
) ([]recordings.WorkerRecordingProjection, error) {
	reader := WorkerRecordingHistoryReader(recording)
	if reader == nil {
		return nil, nil
	}
	projections := make([]recordings.WorkerRecordingProjection, 0)
	nextToken := request.NextToken
	for page := 0; page < 256; page++ {
		request.NextToken = nextToken
		result, err := reader.ListWorkerRecordingProjections(ctx, request)
		if err != nil {
			if errors.Is(err, recordings.ErrMissingWorkerRecordingReader) {
				return nil, nil
			}
			return nil, workersessions.ErrObservationRecordingUnavailable
		}
		projections = append(projections, result.Projections...)
		if result.NextToken == "" || len(result.Projections) == 0 {
			break
		}
		nextToken = result.NextToken
	}
	return projections, nil
}

// WorkerState derives execution state from the recorded terminal fact while
// preserving an execution terminal retained by a degraded recording.
func WorkerState(projection recordings.WorkerRecordingProjection) (workersessions.State, error) {
	terminal := projection.Terminal
	if terminal == nil {
		terminal = projection.ExecutionTerminal
	}
	if terminal == nil {
		return workersessions.StateRunning, nil
	}
	state := workersessions.State(strings.TrimSpace(terminal.Status))
	if !state.Valid() || !state.Terminal() {
		return "", fmt.Errorf("durable Worker terminal status %q is invalid", terminal.Status)
	}
	return state, nil
}

type openingFacts struct {
	payload workers.SessionPayload
	valid   bool
}

func opening(records []events.Record) openingFacts {
	if len(records) == 0 {
		return openingFacts{}
	}
	var draft workers.Draft
	if json.Unmarshal(records[0].Payload, &draft) != nil || draft.Kind != workers.KindSession {
		return openingFacts{}
	}
	var payload workers.SessionPayload
	if json.Unmarshal(draft.Payload, &payload) != nil {
		return openingFacts{}
	}
	return openingFacts{payload: payload, valid: true}
}

func terminalFailure(records []events.Record, state workersessions.State) *workersessions.FailureCause {
	if state != workersessions.StateFailed || len(records) == 0 {
		return nil
	}
	for index := len(records) - 1; index >= 0; index-- {
		var draft workers.Draft
		if json.Unmarshal(records[index].Payload, &draft) != nil || draft.Kind != workers.KindSession || !isTerminalLifecycleRecord(records[index]) {
			continue
		}
		var payload terminalSessionPayload
		if json.Unmarshal(draft.Payload, &payload) != nil || strings.TrimSpace(payload.FailureCause) == "" {
			return nil
		}
		cause := workersessions.FailureCause{
			Kind:                 workersessions.FailureCauseKind(payload.FailureCause),
			Detail:               strings.TrimSpace(payload.FailureDetail),
			AgentRunFailureClass: strings.TrimSpace(payload.AgentRunFailureClass),
		}
		if cause.Validate() != nil {
			return nil
		}
		return &cause
	}
	return nil
}

type terminalSessionPayload struct {
	Status               string `json:"status,omitempty"`
	FailureCause         string `json:"failureCause,omitempty"`
	FailureDetail        string `json:"failureDetail,omitempty"`
	AgentRunFailureClass string `json:"agentRunFailureClass,omitempty"`
}

// Observation projects durable source-native records into the Worker
// Sessions customer contract without consulting a provider session store.
func Observation(projection recordings.WorkerRecordingProjection) (workersessions.Observation, error) {
	state, err := WorkerState(projection)
	if err != nil {
		return workersessions.Observation{}, workersessions.ErrObservationRecordingCorrupt
	}
	details := observationDetailsFor(projection)
	if details.attemptID == "" {
		return workersessions.Observation{}, workersessions.ErrObservationRecordingCorrupt
	}
	usage := observationUsage(projection.Records)
	if details.model == nil {
		details.model = usage.model
	}
	observation := workersessions.Observation{
		WorkerSessionID:            projection.WorkerSessionID,
		PredecessorWorkerSessionID: details.predecessor,
		SuccessorWorkerSessionID:   details.successor,
		Model:                      details.model,
		ReasoningEffort:            details.reasoning,
		Direct:                     details.factorySessionID == "",
		FactorySessionID:           details.factorySessionID,
		WorkIDs:                    details.workIDs,
		TurnID:                     details.turnID,
		AttemptID:                  details.attemptID,
		State:                      state,
		ConfirmationState:          workersessions.ConfirmationStateConfirmed,
		StartedAt:                  details.startedAt,
		DurationBasis:              workersessions.DurationBasisUnavailable,
		TokenUsage:                 usage.tokens,
		Transcript:                 workersessions.TranscriptAvailabilityUnavailable,
		RecordingHealth:            projection.Status,
		RecordingHealthReason:      recordingReason(projection),
		Failure:                    terminalFailure(projection.Records, state),
	}
	if err := observation.Validate(); err != nil {
		return workersessions.Observation{}, fmt.Errorf("validate durable Worker observation: %w", err)
	}
	return observation, nil
}

type observationDetails struct {
	factorySessionID string
	workIDs          []string
	attemptID        string
	turnID           string
	model            *string
	reasoning        *string
	startedAt        *time.Time
	predecessor      string
	successor        string
}

func observationDetailsFor(projection recordings.WorkerRecordingProjection) observationDetails {
	details := observationDetails{
		factorySessionID: strings.TrimSpace(projection.FactorySessionID),
		workIDs:          append([]string(nil), projection.WorkIDs...),
		attemptID:        strings.TrimSpace(projection.AttemptID),
	}
	facts := opening(projection.Records)
	if !facts.valid {
		return details
	}
	if details.factorySessionID == "" {
		details.factorySessionID = strings.TrimSpace(facts.payload.FactorySessionID)
	}
	if len(details.workIDs) == 0 {
		details.workIDs = append([]string(nil), facts.payload.WorkIDs...)
	}
	if details.attemptID == "" {
		details.attemptID = strings.TrimSpace(facts.payload.AttemptID)
	}
	details.turnID = strings.TrimSpace(facts.payload.TurnID)
	details.model = optionalString(facts.payload.Model)
	details.reasoning = optionalString(facts.payload.ReasoningEffort)
	if facts.payload.StartedAt != nil {
		startedAt := *facts.payload.StartedAt
		details.startedAt = &startedAt
	}
	if facts.payload.Lineage != nil {
		details.predecessor = facts.payload.Lineage.PredecessorWorkerSessionID
		details.successor = facts.payload.Lineage.SuccessorWorkerSessionID
	}
	return details
}

type usageDetails struct {
	tokens *workersessions.TokenUsage
	model  *string
}

type usageProjectionPayload struct {
	InputTokens           *int64 `json:"inputTokens"`
	CachedInputTokens     *int64 `json:"cachedInputTokens"`
	OutputTokens          *int64 `json:"outputTokens"`
	ReasoningOutputTokens *int64 `json:"reasoningOutputTokens"`
	TotalTokens           *int64 `json:"totalTokens"`
	Model                 string `json:"model"`
}

func observationUsage(records []events.Record) usageDetails {
	var usage usageDetails
	for _, record := range records {
		var draft workers.Draft
		if json.Unmarshal(record.Payload, &draft) != nil || draft.Kind != workers.KindUsage || draft.Phase != workers.PhaseUpdated {
			continue
		}
		var payload usageProjectionPayload
		if json.Unmarshal(draft.Payload, &payload) != nil {
			continue
		}
		if payload.InputTokens == nil && payload.CachedInputTokens == nil && payload.OutputTokens == nil &&
			payload.ReasoningOutputTokens == nil && payload.TotalTokens == nil {
			continue
		}
		usage.tokens = &workersessions.TokenUsage{
			InputTokens:           int64PointerToInt(payload.InputTokens),
			CachedInputTokens:     int64PointerToInt(payload.CachedInputTokens),
			OutputTokens:          int64PointerToInt(payload.OutputTokens),
			ReasoningOutputTokens: int64PointerToInt(payload.ReasoningOutputTokens),
			TotalTokens:           int64PointerToInt(payload.TotalTokens),
		}
		usage.model = optionalString(payload.Model)
	}
	return usage
}

func int64PointerToInt(value *int64) *int {
	if value == nil {
		return nil
	}
	converted := int(*value)
	return &converted
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func recordingReason(projection recordings.WorkerRecordingProjection) string {
	if reason := strings.TrimSpace(projection.Degradation); reason != "" {
		return reason
	}
	return strings.TrimSpace(projection.InterruptionReason)
}

// ObservationStartedAt returns the opening timestamp retained by the
// provider-neutral source-native history.
func ObservationStartedAt(projection recordings.WorkerRecordingProjection) time.Time {
	facts := opening(projection.Records)
	if facts.valid && facts.payload.StartedAt != nil {
		return *facts.payload.StartedAt
	}
	return time.Time{}
}

// Transcript maps durable source-native drafts into the finished transcript
// contract and carries the same recording-health classification as Observation.
func Transcript(projection recordings.WorkerRecordingProjection) (workersessions.ReadTranscriptResult, error) {
	state, err := WorkerState(projection)
	if err != nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationRecordingCorrupt
	}
	observation, err := Observation(projection)
	if err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	result := workersessions.ReadTranscriptResult{
		WorkerSessionID:       projection.WorkerSessionID,
		WorkIDs:               append([]string(nil), observation.WorkIDs...),
		TurnID:                observation.TurnID,
		AttemptID:             observation.AttemptID,
		State:                 state,
		Entries:               transcriptEntries(projection.Records),
		RecordingHealth:       projection.Status,
		RecordingHealthReason: observation.RecordingHealthReason,
	}
	if err := result.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("validate durable Worker transcript: %w", err)
	}
	return result, nil
}

func transcriptEntries(records []events.Record) []workersessions.TranscriptEntry {
	entries := make([]workersessions.TranscriptEntry, 0, len(records))
	for index, record := range records {
		entry := transcriptEntry(record, index)
		if entry.Type == "" {
			entry.Type = workersessions.TranscriptSystemEvent
		}
		entries = append(entries, entry)
	}
	return entries
}

func transcriptEntry(record events.Record, order int) workersessions.TranscriptEntry {
	entry := workersessions.TranscriptEntry{
		Order:      order,
		SourceType: stringPointer(string(record.SourceType)),
		LineNumber: intPointer(order + 1),
	}
	var draft workers.Draft
	if json.Unmarshal(record.Payload, &draft) != nil {
		return entry
	}
	entry.Status = stringPointer(string(draft.Phase))
	return transcriptPayload(entry, draft)
}

func transcriptPayload(entry workersessions.TranscriptEntry, draft workers.Draft) workersessions.TranscriptEntry {
	switch draft.Kind {
	case workers.KindMessage:
		return messageTranscriptEntry(entry, draft.Payload)
	case workers.KindReasoning:
		return reasoningTranscriptEntry(entry, draft.Payload)
	case workers.KindTool:
		return toolTranscriptEntry(entry, draft.Payload)
	case workers.KindError:
		return errorTranscriptEntry(entry, draft.Payload)
	case workers.KindProgress:
		return progressTranscriptEntry(entry, draft.Payload)
	case workers.KindSession, workers.KindRun, workers.KindTurn, workers.KindUsage, workers.KindPlan, workers.KindFileChange, workers.KindStreamGap:
		entry.Type = workersessions.TranscriptSystemEvent
	}
	return entry
}

func messageTranscriptEntry(entry workersessions.TranscriptEntry, data []byte) workersessions.TranscriptEntry {
	var payload workers.MessagePayload
	if json.Unmarshal(data, &payload) != nil {
		return entry
	}
	if strings.EqualFold(payload.Role, "user") {
		entry.Type = workersessions.TranscriptUserMessage
	} else {
		entry.Type = workersessions.TranscriptAssistantMessage
	}
	entry.Text = messageText(payload)
	return entry
}

func reasoningTranscriptEntry(entry workersessions.TranscriptEntry, data []byte) workersessions.TranscriptEntry {
	entry.Type = workersessions.TranscriptReasoning
	var payload workers.ReasoningPayload
	if json.Unmarshal(data, &payload) != nil {
		return entry
	}
	value := payload.Summary
	if value == "" {
		value = payload.SummaryDelta
	}
	entry.Summary = stringPointer(value)
	return entry
}

func toolTranscriptEntry(entry workersessions.TranscriptEntry, data []byte) workersessions.TranscriptEntry {
	var payload workers.ToolPayload
	if json.Unmarshal(data, &payload) != nil {
		return entry
	}
	entry.CallID = stringPointer(payload.ToolCallID)
	entry.Name = stringPointer(payload.ToolName)
	if payload.ResultSummary != nil {
		entry.Output = stringPointer(string(payload.ResultSummary))
		entry.Type = workersessions.TranscriptToolOutput
		return entry
	}
	entry.Type = workersessions.TranscriptToolCall
	if payload.ArgumentsSummary != nil {
		entry.Arguments = stringPointer(string(payload.ArgumentsSummary))
	}
	return entry
}

func errorTranscriptEntry(entry workersessions.TranscriptEntry, data []byte) workersessions.TranscriptEntry {
	entry.Type = workersessions.TranscriptSystemEvent
	var payload workers.ErrorPayload
	if json.Unmarshal(data, &payload) == nil {
		entry.Summary = stringPointer(payload.Message)
	}
	return entry
}

func progressTranscriptEntry(entry workersessions.TranscriptEntry, data []byte) workersessions.TranscriptEntry {
	entry.Type = workersessions.TranscriptSystemEvent
	var payload workers.ProgressPayload
	if json.Unmarshal(data, &payload) == nil {
		value := payload.Message
		if value == "" {
			value = payload.Label
		}
		entry.Summary = stringPointer(value)
	}
	return entry
}

func messageText(payload workers.MessagePayload) *string {
	parts := make([]string, 0, len(payload.ContentBlocks))
	for _, block := range payload.ContentBlocks {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	value := strings.Join(parts, "")
	return &value
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func intPointer(value int) *int { return &value }

// ObservationStream creates a bounded replay subscription. recordDelivery is
// supplied by Worker Sessions so durable replay uses the same event
// representation as the live stream without importing the owning package.
func ObservationStream(
	_ context.Context,
	projection recordings.WorkerRecordingProjection,
	limit int,
	cursor *workersessions.ObservationCursor,
	recordDelivery func(events.Record, bool, string) workersessions.ObservationDelivery,
) (workersessions.ObservationSubscription, error) {
	if recordDelivery == nil {
		return workersessions.ObservationSubscription{}, fmt.Errorf("durable Worker observation delivery is required")
	}
	if err := validateObservationCursor(cursor, projection.WorkerSessionID, projection.LastPosition); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	if limit == 0 {
		limit = workersessions.DefaultObservationStreamLimit
	}
	subscription := &observationSubscription{
		projection:      projection,
		workerSessionID: projection.WorkerSessionID,
		generationID:    WorkerStreamGenerationForIdentity(projection.WorkerSessionID),
		deliver:         recordDelivery,
		limit:           limit,
	}
	if cursor != nil {
		for subscription.index < len(projection.Records) && uint64(projection.Records[subscription.index].ID.Position) <= cursor.Position {
			subscription.index++
		}
	}
	return workersessions.ObservationSubscription{NextFunc: subscription.Next, CloseFunc: subscription.Close}, nil
}

func validateObservationCursor(cursor *workersessions.ObservationCursor, workerSessionID string, lastPosition events.AggregateSequence) error {
	if cursor == nil {
		return nil
	}
	if strings.TrimSpace(cursor.WorkerSessionID) != "" && cursor.WorkerSessionID != workerSessionID {
		return workersessions.ErrObservationCursorForeign
	}
	if cursor.StreamGenerationID != "" && cursor.StreamGenerationID != WorkerStreamGenerationForIdentity(workerSessionID) {
		if strings.HasPrefix(cursor.StreamGenerationID, workerRecordingGenerationBase) &&
			strings.TrimPrefix(cursor.StreamGenerationID, workerRecordingGenerationBase) != workerSessionID {
			return workersessions.ErrObservationCursorForeign
		}
		return workersessions.ErrObservationCursorUnavailable
	}
	if cursor.Position > uint64(lastPosition) {
		return workersessions.ErrObservationCursorFuture
	}
	return nil
}

type observationSubscription struct {
	mu              sync.Mutex
	projection      recordings.WorkerRecordingProjection
	workerSessionID string
	generationID    string
	deliver         func(events.Record, bool, string) workersessions.ObservationDelivery
	index           int
	eventsEmitted   int
	limit           int
	summarySent     bool
	closed          bool
}

func (subscription *observationSubscription) Next(ctx context.Context) workersessions.ObservationDelivery {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: workersessions.ErrObservationCanceled}
	}
	subscription.mu.Lock()
	defer subscription.mu.Unlock()
	if subscription.closed {
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
	}
	if subscription.index < len(subscription.projection.Records) {
		record := subscription.projection.Records[subscription.index].Detached()
		subscription.index++
		subscription.eventsEmitted++
		delivery := subscription.deliver(record, false, subscription.workerSessionID)
		delivery.Event.Cursor.StreamGenerationID = subscription.generationID
		return delivery
	}
	if !subscription.summarySent {
		subscription.summarySent = true
		complete := subscription.projection.Status == recordings.WorkerRecordingStatusComplete
		reason := recordingReason(subscription.projection)
		if reason == "" {
			reason = replayReason(subscription.projection)
		}
		return workersessions.ObservationDelivery{
			Kind: workersessions.ObservationDeliveryReplaySummary,
			Summary: &workersessions.ReplaySummary{
				Complete:      complete,
				Reason:        reason,
				EventsEmitted: subscription.eventsEmitted,
			},
		}
	}
	subscription.closed = true
	return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
}

func (subscription *observationSubscription) Close() {
	if subscription == nil {
		return
	}
	subscription.mu.Lock()
	subscription.closed = true
	subscription.mu.Unlock()
}

func replayReason(projection recordings.WorkerRecordingProjection) string {
	switch projection.Status {
	case recordings.WorkerRecordingStatusComplete:
		return "session-completed"
	case recordings.WorkerRecordingStatusDegraded:
		return "recording-degraded"
	default:
		return "recording-incomplete"
	}
}

// WorkerStreamGenerationForIdentity is stable across process restarts.
func WorkerStreamGenerationForIdentity(workerSessionID string) string {
	return workerRecordingGenerationBase + strings.TrimSpace(workerSessionID)
}

func isTerminalLifecycleRecord(record events.Record) bool {
	return record.SourceType == lifecycleSourceType &&
		record.SourceSequence >= terminalSourceSequence && record.SourceEventID == terminalSourceEventID
}
