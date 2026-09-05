package service

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

// durableWorkerProjection loads one source-native Worker history through the
// Recordings-owned history capability. The bool distinguishes an absent
// durable catalog from a configured catalog that found this identity.
func (r *registry) durableWorkerProjection(ctx context.Context, workerSessionID string) (recordings.WorkerRecordingProjection, bool, error) {
	reader := r.workerRecordingHistoryReader()
	if reader == nil {
		return recordings.WorkerRecordingProjection{}, false, nil
	}
	snapshot, err := reader.LoadWorkerRecordingByWorkerSessionID(ctx, strings.TrimSpace(workerSessionID))
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
		WorkerSessionID: strings.TrimSpace(workerSessionID),
	})
	if err != nil {
		return recordings.WorkerRecordingProjection{}, true, workersessions.ErrObservationRecordingCorrupt
	}
	return result.Projection, true, nil
}

func (r *registry) workerRecordingHistoryReader() recordings.WorkerRecordingHistoryReader {
	if r == nil || r.recording == nil {
		return nil
	}
	reader, ok := r.recording.(recordings.WorkerRecordingHistoryReader)
	if !ok || reader == nil {
		return nil
	}
	return reader
}

// ListWorkerRecordingProjections exposes the Recordings-owned catalog through
// the per-runtime Worker Sessions capability. The runtime read wrapper uses
// this narrow forwarding seam after a process restart; the registry still
// remains the owner of Worker Session projection policy.
func (r *registry) ListWorkerRecordingProjections(
	ctx context.Context,
	request recordings.WorkerRecordingListRequest,
) (recordings.WorkerRecordingListResult, error) {
	reader := r.workerRecordingHistoryReader()
	if reader == nil {
		return recordings.WorkerRecordingListResult{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.ListWorkerRecordingProjections(ctx, request)
}

// LoadWorkerRecordingByWorkerSessionID forwards the durable Worker-ID lookup
// without exposing the file catalog to Factory Runtime.
func (r *registry) LoadWorkerRecordingByWorkerSessionID(
	ctx context.Context,
	workerSessionID string,
) (recordings.WorkerRecordingSnapshot, error) {
	reader := r.workerRecordingHistoryReader()
	if reader == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.LoadWorkerRecordingByWorkerSessionID(ctx, workerSessionID)
}

func (r *registry) durableWorkerProjections(ctx context.Context, request recordings.WorkerRecordingListRequest) ([]recordings.WorkerRecordingProjection, error) {
	reader := r.workerRecordingHistoryReader()
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

func durableWorkerState(projection recordings.WorkerRecordingProjection) (workersessions.State, error) {
	terminal := projection.Terminal
	if terminal == nil {
		// A DEGRADED recording may retain an authoritative execution terminal
		// even when the source-native terminal record itself was lost. Preserve
		// the Worker lifecycle state without fabricating a missing record.
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

type durableOpeningFacts struct {
	payload workers.SessionPayload
	draft   workers.Draft
	valid   bool
}

func durableOpening(records []events.Record) durableOpeningFacts {
	if len(records) == 0 {
		return durableOpeningFacts{}
	}
	var draft workers.Draft
	if json.Unmarshal(records[0].Payload, &draft) != nil {
		return durableOpeningFacts{}
	}
	var payload workers.SessionPayload
	if draft.Kind != workers.KindSession || json.Unmarshal(draft.Payload, &payload) != nil {
		return durableOpeningFacts{}
	}
	return durableOpeningFacts{payload: payload, draft: draft, valid: true}
}

func durableTerminalFailure(records []events.Record, state workersessions.State) *workersessions.FailureCause {
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

func durableObservation(projection recordings.WorkerRecordingProjection) (workersessions.Observation, error) {
	state, err := durableWorkerState(projection)
	if err != nil {
		return workersessions.Observation{}, workersessions.ErrObservationRecordingCorrupt
	}
	facts := durableOpening(projection.Records)
	factorySessionID := strings.TrimSpace(projection.FactorySessionID)
	workIDs := append([]string(nil), projection.WorkIDs...)
	attemptID := strings.TrimSpace(projection.AttemptID)
	turnID := ""
	var model, reasoning *string
	var startedAt *time.Time
	var predecessor, successor string
	if facts.valid {
		if factorySessionID == "" {
			factorySessionID = strings.TrimSpace(facts.payload.FactorySessionID)
		}
		if len(workIDs) == 0 {
			workIDs = append([]string(nil), facts.payload.WorkIDs...)
		}
		if attemptID == "" {
			attemptID = strings.TrimSpace(facts.payload.AttemptID)
		}
		turnID = strings.TrimSpace(facts.payload.TurnID)
		if strings.TrimSpace(facts.payload.Model) != "" {
			value := strings.TrimSpace(facts.payload.Model)
			model = &value
		}
		if strings.TrimSpace(facts.payload.ReasoningEffort) != "" {
			value := strings.TrimSpace(facts.payload.ReasoningEffort)
			reasoning = &value
		}
		if facts.payload.StartedAt != nil {
			value := *facts.payload.StartedAt
			startedAt = &value
		}
		if facts.payload.Lineage != nil {
			predecessor = facts.payload.Lineage.PredecessorWorkerSessionID
			successor = facts.payload.Lineage.SuccessorWorkerSessionID
		}
	}
	if attemptID == "" {
		return workersessions.Observation{}, workersessions.ErrObservationRecordingCorrupt
	}

	var tokenUsage *workersessions.TokenUsage
	var usageModel string
	for _, record := range projection.Records {
		var draft workers.Draft
		if json.Unmarshal(record.Payload, &draft) != nil {
			continue
		}
		usage, modelValue, ok := usageProjectionFromDraft(draft)
		if ok {
			tokenUsage = usage
			usageModel = modelValue
		}
	}
	if model == nil && strings.TrimSpace(usageModel) != "" {
		value := strings.TrimSpace(usageModel)
		model = &value
	}
	reason := strings.TrimSpace(projection.Degradation)
	if reason == "" {
		reason = strings.TrimSpace(projection.InterruptionReason)
	}
	observation := workersessions.Observation{
		WorkerSessionID:            projection.WorkerSessionID,
		PredecessorWorkerSessionID: predecessor,
		SuccessorWorkerSessionID:   successor,
		Model:                      model,
		ReasoningEffort:            reasoning,
		Direct:                     factorySessionID == "",
		FactorySessionID:           factorySessionID,
		WorkIDs:                    workIDs,
		TurnID:                     turnID,
		AttemptID:                  attemptID,
		State:                      state,
		ConfirmationState:          workersessions.ConfirmationStateConfirmed,
		StartedAt:                  startedAt,
		DurationBasis:              workersessions.DurationBasisUnavailable,
		TokenUsage:                 tokenUsage,
		Transcript:                 workersessions.TranscriptAvailabilityUnavailable,
		RecordingHealth:            projection.Status,
		RecordingHealthReason:      reason,
		Failure:                    durableTerminalFailure(projection.Records, state),
	}
	if err := observation.Validate(); err != nil {
		return workersessions.Observation{}, fmt.Errorf("validate durable Worker observation: %w", err)
	}
	return observation, nil
}

func durableObservationStartedAt(projection recordings.WorkerRecordingProjection) time.Time {
	facts := durableOpening(projection.Records)
	if facts.valid && facts.payload.StartedAt != nil {
		return *facts.payload.StartedAt
	}
	return time.Time{}
}

func applyDurableProjection(projected *workersessions.Observation, projection recordings.WorkerRecordingProjection) {
	if projected == nil {
		return
	}
	projected.RecordingHealth = projection.Status
	projected.RecordingHealthReason = strings.TrimSpace(projection.Degradation)
	if projected.RecordingHealthReason == "" {
		projected.RecordingHealthReason = strings.TrimSpace(projection.InterruptionReason)
	}
	if projected.FactorySessionID == "" {
		projected.FactorySessionID = strings.TrimSpace(projection.FactorySessionID)
	}
	if len(projected.WorkIDs) == 0 {
		projected.WorkIDs = append([]string(nil), projection.WorkIDs...)
	}
	if projected.AttemptID == "" {
		projected.AttemptID = strings.TrimSpace(projection.AttemptID)
	}
	if projected.ConfirmationState == workersessions.ConfirmationStateUnconfirmed {
		projected.ConfirmationState = workersessions.ConfirmationStateConfirmed
	}
}

func durableTranscript(projection recordings.WorkerRecordingProjection) (workersessions.ReadTranscriptResult, error) {
	state, err := durableWorkerState(projection)
	if err != nil {
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationRecordingCorrupt
	}
	observation, err := durableObservation(projection)
	if err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	result := workersessions.ReadTranscriptResult{
		WorkerSessionID:       projection.WorkerSessionID,
		WorkIDs:               append([]string(nil), observation.WorkIDs...),
		TurnID:                observation.TurnID,
		AttemptID:             observation.AttemptID,
		State:                 state,
		Entries:               durableTranscriptEntries(projection.Records),
		RecordingHealth:       projection.Status,
		RecordingHealthReason: observation.RecordingHealthReason,
	}
	if err := result.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("validate durable Worker transcript: %w", err)
	}
	return result, nil
}

func durableTranscriptEntries(records []events.Record) []workersessions.TranscriptEntry {
	entries := make([]workersessions.TranscriptEntry, 0, len(records))
	for index, record := range records {
		entry := durableTranscriptEntry(record, index)
		if entry.Type == "" {
			entry.Type = workersessions.TranscriptSystemEvent
		}
		entries = append(entries, entry)
	}
	return entries
}

func durableTranscriptEntry(record events.Record, order int) workersessions.TranscriptEntry {
	entry := workersessions.TranscriptEntry{
		Order:      order,
		SourceType: durableString(string(record.SourceType)),
		LineNumber: durableInt(order + 1),
	}
	var draft workers.Draft
	if json.Unmarshal(record.Payload, &draft) != nil {
		return entry
	}
	entry.Status = durableString(string(draft.Phase))
	switch draft.Kind {
	case workers.KindMessage:
		var payload workers.MessagePayload
		if json.Unmarshal(draft.Payload, &payload) == nil {
			if strings.EqualFold(payload.Role, "user") {
				entry.Type = workersessions.TranscriptUserMessage
			} else {
				entry.Type = workersessions.TranscriptAssistantMessage
			}
			entry.Text = durableMessageText(payload)
		}
	case workers.KindReasoning:
		entry.Type = workersessions.TranscriptReasoning
		var payload workers.ReasoningPayload
		if json.Unmarshal(draft.Payload, &payload) == nil {
			value := payload.Summary
			if value == "" {
				value = payload.SummaryDelta
			}
			entry.Summary = durableString(value)
		}
	case workers.KindTool:
		var payload workers.ToolPayload
		if json.Unmarshal(draft.Payload, &payload) == nil {
			entry.CallID = durableString(payload.ToolCallID)
			entry.Name = durableString(payload.ToolName)
			if payload.ResultSummary != nil {
				entry.Output = durableString(string(payload.ResultSummary))
				entry.Type = workersessions.TranscriptToolOutput
			} else {
				entry.Type = workersessions.TranscriptToolCall
				if payload.ArgumentsSummary != nil {
					entry.Arguments = durableString(string(payload.ArgumentsSummary))
				}
			}
		}
	case workers.KindError:
		entry.Type = workersessions.TranscriptSystemEvent
		var payload workers.ErrorPayload
		if json.Unmarshal(draft.Payload, &payload) == nil {
			entry.Summary = durableString(payload.Message)
		}
	case workers.KindProgress:
		entry.Type = workersessions.TranscriptSystemEvent
		var payload workers.ProgressPayload
		if json.Unmarshal(draft.Payload, &payload) == nil {
			value := payload.Message
			if value == "" {
				value = payload.Label
			}
			entry.Summary = durableString(value)
		}
	case workers.KindSession, workers.KindRun, workers.KindTurn, workers.KindUsage, workers.KindPlan, workers.KindFileChange, workers.KindStreamGap:
		entry.Type = workersessions.TranscriptSystemEvent
	}
	return entry
}

func durableMessageText(payload workers.MessagePayload) *string {
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

func durableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func durableInt(value int) *int { return &value }

type durableObservationSubscription struct {
	mu              sync.Mutex
	projection      recordings.WorkerRecordingProjection
	workerSessionID string
	generationID    string
	index           int
	eventsEmitted   int
	limit           int
	summarySent     bool
	closed          bool
}

func durableObservationStream(
	ctx context.Context,
	projection recordings.WorkerRecordingProjection,
	limit int,
	cursor *workersessions.ObservationCursor,
) (workersessions.ObservationSubscription, error) {
	if err := validateDurableObservationCursor(cursor, projection.WorkerSessionID, projection.LastPosition); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	if limit == 0 {
		limit = workersessions.DefaultObservationStreamLimit
	}
	generationID := durableWorkerStreamGeneration(projection)
	start := 0
	if cursor != nil {
		for start < len(projection.Records) && uint64(projection.Records[start].ID.Position) <= cursor.Position {
			start++
		}
	}
	subscription := &durableObservationSubscription{
		projection:      projection,
		workerSessionID: projection.WorkerSessionID,
		generationID:    generationID,
		index:           start,
		limit:           limit,
	}
	return workersessions.ObservationSubscription{NextFunc: subscription.Next, CloseFunc: subscription.Close}, nil
}

func validateDurableObservationCursor(cursor *workersessions.ObservationCursor, workerSessionID string, lastPosition events.AggregateSequence) error {
	if cursor == nil {
		return nil
	}
	if strings.TrimSpace(cursor.WorkerSessionID) != "" && cursor.WorkerSessionID != workerSessionID {
		return workersessions.ErrObservationCursorForeign
	}
	if cursor.StreamGenerationID != "" && cursor.StreamGenerationID != durableWorkerStreamGenerationForIdentity(workerSessionID) {
		if strings.HasPrefix(cursor.StreamGenerationID, "worker-recording/") &&
			strings.TrimPrefix(cursor.StreamGenerationID, "worker-recording/") != workerSessionID {
			return workersessions.ErrObservationCursorForeign
		}
		return workersessions.ErrObservationCursorUnavailable
	}
	if cursor.Position > uint64(lastPosition) {
		return workersessions.ErrObservationCursorFuture
	}
	return nil
}

func (subscription *durableObservationSubscription) Next(ctx context.Context) workersessions.ObservationDelivery {
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
		delivery := observationRecordDelivery(record, false, subscription.workerSessionID)
		delivery.Event.Cursor.StreamGenerationID = subscription.generationID
		return delivery
	}
	if !subscription.summarySent {
		subscription.summarySent = true
		complete := subscription.projection.Status == recordings.WorkerRecordingStatusComplete
		reason := strings.TrimSpace(subscription.projection.Degradation)
		if reason == "" {
			reason = strings.TrimSpace(subscription.projection.InterruptionReason)
		}
		if reason == "" {
			reason = replayReasonForDurableProjection(subscription.projection)
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

func (subscription *durableObservationSubscription) Close() {
	if subscription == nil {
		return
	}
	subscription.mu.Lock()
	subscription.closed = true
	subscription.mu.Unlock()
}

func replayReasonForDurableProjection(projection recordings.WorkerRecordingProjection) string {
	if projection.Status == recordings.WorkerRecordingStatusComplete {
		return "session-completed"
	}
	if projection.Status == recordings.WorkerRecordingStatusDegraded {
		return "recording-degraded"
	}
	return "recording-incomplete"
}

func durableWorkerStreamGeneration(projection recordings.WorkerRecordingProjection) string {
	return durableWorkerStreamGenerationForIdentity(projection.WorkerSessionID)
}

// The generation is stable for a Worker Session identity across process
// restarts. The durable catalog already scopes the stream by Worker ID, so no
// process-local Events generation is invented for sidecar replay.
func durableWorkerStreamGenerationForIdentity(workerSessionID string) string {
	return "worker-recording/" + strings.TrimSpace(workerSessionID)
}
