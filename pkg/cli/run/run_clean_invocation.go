package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

func emitCleanInvocationOutcome(ctx context.Context, cfg RunConfig, runner factoryServiceRunner, runErr error, startedAt time.Time) error {
	logger := cleanInvocationLogger(cfg.Logger)
	provider, ok := runner.(engineStateSnapshotProvider)
	if !ok {
		if runErr == nil {
			err := &InvocationError{
				Code:    InvocationErrorCodeFailed,
				Message: "clean invocation result snapshot is unavailable",
			}
			recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
				StartedAt: startedAt,
				Err:       err,
			})
			return err
		}
		err := newInvocationErrorForRunFailure(runErr, nil)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Err:       err,
		})
		return err
	}
	snapshot, err := provider.GetEngineStateSnapshot(ctx)
	if err != nil {
		if runErr == nil {
			invocationErr := &InvocationError{
				Code:    InvocationErrorCodeFailed,
				Message: "clean invocation result snapshot is unavailable",
				Cause:   err,
			}
			recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
				StartedAt: startedAt,
				Err:       invocationErr,
			})
			return invocationErr
		}
		invocationErr := newInvocationErrorForRunFailure(runErr, nil)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Err:       invocationErr,
		})
		return invocationErr
	}
	if runErr != nil {
		invocationErr := newInvocationErrorForRunFailure(runErr, snapshot)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Snapshot:  snapshot,
			Err:       invocationErr,
		})
		return invocationErr
	}
	target, err := cleanInvocationWorkTargetFromFile(cfg.WorkFile)
	if err != nil {
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Snapshot:  snapshot,
			Err:       err,
		})
		return err
	}
	result, ok := cleanInvocationSuccessFromSnapshot(snapshot, target)
	if !ok {
		invocationErr := cleanInvocationFailureFromSnapshot(snapshot, target)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Snapshot:  snapshot,
			Target:    &target,
			Err:       invocationErr,
		})
		return invocationErr
	}
	if err := writeCleanInvocationSuccess(cfg, result); err != nil {
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Snapshot:  snapshot,
			Target:    &target,
			Err:       err,
		})
		return err
	}
	recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
		StartedAt: startedAt,
		Snapshot:  snapshot,
		Target:    &target,
		Success:   &result,
	})
	return nil
}

func newInvocationErrorForRunFailure(
	runErr error,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) error {
	switch {
	case errors.Is(runErr, context.DeadlineExceeded):
		return &InvocationError{
			Code:    InvocationErrorCodeTimeout,
			Message: "clean invocation timed out",
			Cause:   runErr,
		}
	case errors.Is(runErr, context.Canceled):
		return &InvocationError{
			Code:    InvocationErrorCodeCancelled,
			Message: "clean invocation cancelled",
			Cause:   runErr,
		}
	}

	if timeoutFailure, ok := cleanInvocationTimeoutFromSnapshot(snapshot); ok {
		return timeoutFailure
	}
	return &InvocationError{
		Code:    InvocationErrorCodeFailed,
		Message: "clean invocation failed",
		Cause:   runErr,
	}
}

func cleanInvocationFailureFromSnapshot(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) error {
	if timeoutFailure, ok := cleanInvocationTimeoutForTarget(snapshot, target); ok {
		return timeoutFailure
	}
	if failureReason, ok := cleanInvocationFailedForTarget(snapshot, target); ok {
		message := "clean invocation failed"
		if failureReason != "" {
			message = fmt.Sprintf("clean invocation failed: %s", failureReason)
		}
		return &InvocationError{
			Code:    InvocationErrorCodeFailed,
			Message: message,
		}
	}
	return &InvocationError{
		Code:    InvocationErrorCodeFailed,
		Message: fmt.Sprintf("clean invocation completed without a terminal success result for work %q", target.WorkID),
	}
}

func cleanInvocationTimeoutFromSnapshot(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) (*InvocationError, bool) {
	if snapshot == nil {
		return nil, false
	}
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		failure := snapshot.DispatchHistory[i].FailureMetadata
		if failure != nil && failure.Type == interfaces.WorkFailureTypeTimeout {
			return &InvocationError{
				Code:    InvocationErrorCodeTimeout,
				Message: "clean invocation timed out",
			}, true
		}
	}
	return nil, false
}

func cleanInvocationTimeoutForTarget(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) (*InvocationError, bool) {
	if snapshot == nil {
		return nil, false
	}
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		completion := snapshot.DispatchHistory[i]
		if !cleanInvocationCompletionMatchesTarget(completion, target) {
			continue
		}
		if completion.FailureMetadata != nil && completion.FailureMetadata.Type == interfaces.WorkFailureTypeTimeout {
			return &InvocationError{
				Code:    InvocationErrorCodeTimeout,
				Message: "clean invocation timed out",
			}, true
		}
	}
	return nil, false
}

func cleanInvocationFailedForTarget(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) (string, bool) {
	if snapshot == nil {
		return "", false
	}
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		completion := snapshot.DispatchHistory[i]
		if completion.Outcome != interfaces.OutcomeFailed {
			continue
		}
		if !cleanInvocationCompletionMatchesTarget(completion, target) {
			continue
		}
		return strings.TrimSpace(completion.Reason), true
	}
	if snapshot.Topology == nil {
		return "", false
	}
	for _, token := range snapshot.Marking.Tokens {
		if token == nil {
			continue
		}
		if token.Color.WorkID != target.WorkID || token.Color.WorkTypeID != target.WorkTypeName {
			continue
		}
		if snapshot.Topology.StateCategoryForPlace(token.PlaceID) == state.StateCategoryFailed {
			return "", true
		}
	}
	return "", false
}

func cleanInvocationWorkTargetFromFile(workFile string) (cleanInvocationWorkTarget, error) {
	request, err := LoadWorkFile(workFile)
	if err != nil {
		return cleanInvocationWorkTarget{}, err
	}
	normalized, err := requests.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{})
	if err != nil {
		return cleanInvocationWorkTarget{}, err
	}
	if len(normalized) != 1 {
		return cleanInvocationWorkTarget{}, fmt.Errorf("clean invocation requires exactly one work item, got %d", len(normalized))
	}
	return cleanInvocationWorkTarget{
		WorkID:       normalized[0].WorkID,
		WorkTypeName: normalized[0].WorkTypeID,
	}, nil
}

func cleanInvocationSuccessFromSnapshot(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) (cleanInvocationSuccess, bool) {
	if snapshot == nil || snapshot.Topology == nil {
		return cleanInvocationSuccess{}, false
	}
	if result, ok := cleanInvocationSuccessFromTerminalTokens(snapshot, target); ok {
		return result, true
	}
	return cleanInvocationSuccessFromDispatchHistory(snapshot, target)
}

func cleanInvocationSuccessFromTerminalTokens(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) (cleanInvocationSuccess, bool) {
	tokens := make([]*interfaces.Token, 0, len(snapshot.Marking.Tokens))
	for _, token := range snapshot.Marking.Tokens {
		if token != nil {
			tokens = append(tokens, token)
		}
	}
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].ID < tokens[j].ID
	})
	for _, token := range tokens {
		if cleanInvocationTokenMatches(snapshot.Topology, token, target) {
			return cleanInvocationSuccessFromToken(token), true
		}
	}
	return cleanInvocationSuccess{}, false
}

func cleanInvocationSuccessFromDispatchHistory(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) (cleanInvocationSuccess, bool) {
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		completion := snapshot.DispatchHistory[i]
		if completion.Outcome != interfaces.OutcomeAccepted {
			continue
		}
		for _, mutation := range completion.OutputMutations {
			if cleanInvocationTokenMatches(snapshot.Topology, mutation.Token, target) {
				return cleanInvocationSuccessFromToken(mutation.Token), true
			}
		}
	}
	return cleanInvocationSuccess{}, false
}

func cleanInvocationTokenMatches(net *state.Net, token *interfaces.Token, target cleanInvocationWorkTarget) bool {
	if net == nil || token == nil {
		return false
	}
	if token.Color.DataType == interfaces.DataTypeResource {
		return false
	}
	if token.Color.WorkID != target.WorkID || token.Color.WorkTypeID != target.WorkTypeName {
		return false
	}
	return net.StateCategoryForPlace(token.PlaceID) == state.StateCategoryTerminal
}

func cleanInvocationSuccessFromToken(token *interfaces.Token) cleanInvocationSuccess {
	return cleanInvocationSuccess{
		Output:       string(token.Color.Payload),
		WorkID:       token.Color.WorkID,
		WorkTypeName: token.Color.WorkTypeID,
		TraceID:      token.Color.TraceID,
		SessionID:    defaultFactorySessionID,
	}
}

func writeCleanInvocationSuccess(cfg RunConfig, result cleanInvocationSuccess) error {
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}
	if cfg.JSON {
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = output.Write(data)
		return err
	}
	_, err := io.WriteString(output, result.Output)
	return err
}

func cleanInvocationCompletionMatchesTarget(
	completion interfaces.CompletedDispatch,
	target cleanInvocationWorkTarget,
) bool {
	for _, token := range completion.ConsumedTokens {
		if token.Color.WorkID == target.WorkID && token.Color.WorkTypeID == target.WorkTypeName {
			return true
		}
	}
	for _, mutation := range completion.OutputMutations {
		if mutation.Token == nil {
			continue
		}
		if mutation.Token.Color.WorkID == target.WorkID && mutation.Token.Color.WorkTypeID == target.WorkTypeName {
			return true
		}
	}
	return false
}

const (
	responseStreamProgressPrefix          = "[you:progress] "
	responseStreamPrimaryResultHeader     = "--- primary result ---"
	responseStreamInvocationOutcomeHeader = "--- invocation outcome ---"
	maxHumanProgressLineBytes             = 1024

	responseStreamJSONRecordProgress      = "progress"
	responseStreamJSONRecordStreamGap     = "stream_gap"
	responseStreamJSONRecordCompaction    = "compaction"
	responseStreamJSONRecordPrimaryResult = "primary_result"

	responseStreamTerminalBacklogReason = "terminal_output_backlog"
)

// responseStreamRenderer consumes internal SessionResponseStream segments and
// writes ordered progress output followed by the final invocation result.
type responseStreamRenderer interface {
	responseStreamEventSink
	stopProgressRendering()
	writeFinalInvocationResult(result apisurface.FactoryInvocationResult) error
}

func newResponseStreamRenderer(output io.Writer, jsonMode bool) responseStreamRenderer {
	if jsonMode {
		return newJSONResponseStreamRenderer(output)
	}
	return newHumanResponseStreamRenderer(output)
}

// humanResponseStreamRenderer prints ordered internal SessionResponseStream
// progress to stdout and keeps the final invocation primary result visually
// separate from transient progress output.
type humanResponseStreamRenderer struct {
	mu              sync.Mutex
	output          io.Writer
	progress        *responseStreamProgressWriter
	lastSequence    map[string]int64
	progressLines   int
	progressSeen    bool
	backlogNotified bool
}

func newHumanResponseStreamRenderer(output io.Writer) *humanResponseStreamRenderer {
	if output == nil {
		output = os.Stdout
	}
	return &humanResponseStreamRenderer{
		output:       output,
		progress:     newResponseStreamProgressWriter(output),
		lastSequence: make(map[string]int64),
	}
}

func (r *humanResponseStreamRenderer) stopProgressRendering() {
	if r == nil {
		return
	}
	r.progress.stopAndDrain()
}

func (r *humanResponseStreamRenderer) onStreamSegment(result factorysessions.SessionResponseStreamReadResult) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if result.BehindRetainedWindow {
		r.writeProgressLineLocked("earlier progress unavailable (stream resumed behind retained window)")
	}
	if result.Compaction != nil {
		r.writeProgressLineLocked(formatCompactionNotice(*result.Compaction))
	}
	for _, event := range result.Events {
		r.renderEventLocked(event)
	}
}

func (r *humanResponseStreamRenderer) writeFinalInvocationResult(
	result apisurface.FactoryInvocationResult,
) error {
	if r == nil {
		return fmt.Errorf("response-stream renderer is nil")
	}
	r.stopProgressRendering()
	r.progress.acquireOutputExclusive()
	defer r.progress.releaseOutputExclusive()
	if result.Status == factoryapi.InvocationTerminalStatusCompleted {
		text, err := invocationPrimaryResultText(result.PrimaryResult)
		if err != nil {
			return err
		}
		return r.writePrimaryResult(text)
	}
	return r.writeInvocationOutcome(result)
}

func (r *humanResponseStreamRenderer) writeInvocationOutcome(
	result apisurface.FactoryInvocationResult,
) error {
	if r == nil {
		return fmt.Errorf("response-stream renderer is nil")
	}
	lines := formatHumanInvocationOutcomeLines(result)
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.progressSeen {
		if _, err := fmt.Fprintln(r.output); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(r.output, responseStreamInvocationOutcomeHeader); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(r.output, line); err != nil {
			return err
		}
	}
	return nil
}

func formatHumanInvocationOutcomeLines(result apisurface.FactoryInvocationResult) []string {
	lines := []string{
		"status: " + string(result.Status),
	}
	if code := strings.TrimSpace(result.ErrorCode); code != "" {
		lines = append(lines, "error: "+code)
	}
	if message := strings.TrimSpace(result.Message); message != "" {
		lines = append(lines, "message: "+message)
	}
	if sessionID := strings.TrimSpace(result.SessionID); sessionID != "" {
		lines = append(lines, "session: "+sessionID)
	}
	if workID := strings.TrimSpace(result.WorkID); workID != "" {
		lines = append(lines, "workId: "+workID)
	}
	if workName := strings.TrimSpace(result.WorkName); workName != "" {
		lines = append(lines, "workName: "+workName)
	}
	if workState := strings.TrimSpace(result.WorkState); workState != "" {
		lines = append(lines, "workState: "+workState)
	}
	return lines
}

func (r *humanResponseStreamRenderer) writePrimaryResult(text string) error {
	if r == nil {
		return fmt.Errorf("response-stream renderer is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.progressSeen {
		if _, err := fmt.Fprintln(r.output); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(r.output, responseStreamPrimaryResultHeader); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(r.output, text)
	return err
}

func (r *humanResponseStreamRenderer) renderEventLocked(event responsestream.Event) {
	dispatchKey := strings.TrimSpace(event.DispatchID)
	if dispatchKey == "" {
		dispatchKey = "_"
	}
	if event.Sequence > 0 && event.Sequence <= r.lastSequence[dispatchKey] {
		return
	}
	if event.Sequence > 0 {
		r.lastSequence[dispatchKey] = event.Sequence
	}

	switch event.Kind {
	case responsestream.EventKindCompactionSignal:
		if event.Compaction != nil {
			r.writeProgressLineLocked(formatCompactionNotice(*event.Compaction))
		}
		return
	case responsestream.EventKindStreamCompleted, responsestream.EventKindStreamFailed:
		return
	case responsestream.EventKindResponseFragment:
		return
	case responsestream.EventKindProgressFragment:
		if !humanProgressRenderableType(event.Type) {
			return
		}
		payload := boundedHumanProgressPayload(event.Payload)
		if payload == "" {
			return
		}
		r.writeProgressLineLocked(payload)
	}
}

func humanProgressRenderableType(eventType responsestream.EventType) bool {
	switch eventType {
	case responsestream.EventTypeStarted,
		responsestream.EventTypeProgress,
		responsestream.EventTypeFailed,
		responsestream.EventTypeCanceled,
		responsestream.EventTypeUnknown:
		return true
	default:
		return false
	}
}

func boundedHumanProgressPayload(payload string) string {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return ""
	}
	if maxHumanProgressLineBytes <= 0 || len([]byte(trimmed)) <= maxHumanProgressLineBytes {
		return trimmed
	}
	bytes := []byte(trimmed)
	return strings.TrimSpace(string(bytes[:maxHumanProgressLineBytes])) + "..."
}

func formatCompactionNotice(summary responsestream.CompactionSummary) string {
	reason := strings.TrimSpace(string(summary.Reason))
	if reason == "" {
		reason = "compacted"
	}
	if summary.DroppedSequenceCount > 0 {
		return fmt.Sprintf(
			"stream %s (%d earlier events omitted)",
			strings.ToLower(reason),
			summary.DroppedSequenceCount,
		)
	}
	return fmt.Sprintf("stream %s", strings.ToLower(reason))
}

func (r *humanResponseStreamRenderer) writeProgressLineLocked(payload string) {
	if strings.TrimSpace(payload) == "" {
		return
	}
	line := responseStreamProgressPrefix + payload
	if !r.progress.enqueue([]byte(line)) {
		r.emitTerminalBacklogNoticeLocked()
		return
	}
	r.progressSeen = true
	r.progressLines++
}

func (r *humanResponseStreamRenderer) emitTerminalBacklogNoticeLocked() {
	if r.backlogNotified {
		return
	}
	r.backlogNotified = true
	dropped := r.progress.droppedProgressLines()
	if dropped <= 0 {
		dropped = 1
	}
	notice := fmt.Sprintf(
		"%s%s (%d progress lines dropped)",
		responseStreamProgressPrefix,
		"terminal output backlog",
		dropped,
	)
	r.progress.enqueueNotice([]byte(notice))
	r.progressSeen = true
}

// jsonResponseStreamRenderer emits newline-delimited JSON records for internal
// SessionResponseStream progress and the final invocation result.
type jsonResponseStreamRenderer struct {
	mu              sync.Mutex
	output          io.Writer
	progress        *responseStreamProgressWriter
	lastSequence    map[string]int64
	progressSeen    bool
	backlogNotified bool
}

func newJSONResponseStreamRenderer(output io.Writer) *jsonResponseStreamRenderer {
	if output == nil {
		output = os.Stdout
	}
	return &jsonResponseStreamRenderer{
		output:       output,
		progress:     newResponseStreamProgressWriter(output),
		lastSequence: make(map[string]int64),
	}
}

func (r *jsonResponseStreamRenderer) stopProgressRendering() {
	if r == nil {
		return
	}
	r.progress.stopAndDrain()
}

func (r *jsonResponseStreamRenderer) onStreamSegment(result factorysessions.SessionResponseStreamReadResult) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if result.BehindRetainedWindow {
		r.writeRecordLocked(responseStreamJSONStreamGapRecord{
			RecordType: responseStreamJSONRecordStreamGap,
			Reason:     "behind_retained_window",
		})
	}
	if result.Compaction != nil {
		r.writeCompactionLocked(*result.Compaction)
	}
	for _, event := range result.Events {
		r.renderEventLocked(event)
	}
}

func (r *jsonResponseStreamRenderer) writeFinalInvocationResult(
	result apisurface.FactoryInvocationResult,
) error {
	if r == nil {
		return fmt.Errorf("response-stream renderer is nil")
	}
	r.stopProgressRendering()
	r.progress.acquireOutputExclusive()
	defer r.progress.releaseOutputExclusive()
	return r.writeRecord(responseStreamJSONPrimaryResultRecord{
		RecordType: responseStreamJSONRecordPrimaryResult,
		Invocation: apisurface.InvocationResponseFromResult(result),
	})
}

func (r *jsonResponseStreamRenderer) renderEventLocked(event responsestream.Event) {
	switch event.Kind {
	case responsestream.EventKindCompactionSignal:
		if event.Compaction != nil {
			r.writeCompactionLocked(*event.Compaction)
		}
		return
	case responsestream.EventKindStreamCompleted, responsestream.EventKindStreamFailed:
		return
	case responsestream.EventKindResponseFragment:
		return
	case responsestream.EventKindProgressFragment:
		if !humanProgressRenderableType(event.Type) {
			return
		}
		if strings.TrimSpace(event.Payload) == "" {
			return
		}
		dispatchKey := strings.TrimSpace(event.DispatchID)
		if dispatchKey == "" {
			dispatchKey = "_"
		}
		if event.Sequence > 0 && event.Sequence <= r.lastSequence[dispatchKey] {
			return
		}
		if event.Sequence > 0 {
			r.lastSequence[dispatchKey] = event.Sequence
		}
		record := responseStreamJSONProgressRecord{
			RecordType: responseStreamJSONRecordProgress,
			Sequence:   event.Sequence,
			Kind:       string(event.Kind),
			EventType:  string(event.Type),
			Payload:    event.Payload,
		}
		if dispatchID := strings.TrimSpace(event.DispatchID); dispatchID != "" {
			record.DispatchID = &dispatchID
		}
		r.writeRecordLocked(record)
	}
}

func (r *jsonResponseStreamRenderer) writeCompactionLocked(summary responsestream.CompactionSummary) {
	r.writeRecordLocked(responseStreamJSONCompactionRecord{
		RecordType:            responseStreamJSONRecordCompaction,
		Reason:                string(summary.Reason),
		DroppedSequenceCount:  summary.DroppedSequenceCount,
		FirstRetainedSequence: summary.FirstRetainedSequence,
		LastDroppedSequence:   summary.LastDroppedSequence,
	})
}

func (r *jsonResponseStreamRenderer) writeRecord(record any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.writeRecordLocked(record)
}

func (r *jsonResponseStreamRenderer) writeRecordLocked(record any) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal response-stream JSON record: %w", err)
	}
	if recordTypeOf(record) == responseStreamJSONRecordPrimaryResult {
		_, err = fmt.Fprintln(r.output, string(encoded))
		return err
	}
	if !r.progress.enqueue(encoded) {
		r.emitTerminalBacklogNoticeLocked()
		return nil
	}
	r.progressSeen = true
	return nil
}

func (r *jsonResponseStreamRenderer) emitTerminalBacklogNoticeLocked() {
	if r.backlogNotified {
		return
	}
	r.backlogNotified = true
	dropped := r.progress.droppedProgressLines()
	if dropped <= 0 {
		dropped = 1
	}
	encoded, err := json.Marshal(responseStreamJSONStreamGapRecord{
		RecordType:           responseStreamJSONRecordStreamGap,
		Reason:               responseStreamTerminalBacklogReason,
		DroppedProgressLines: dropped,
	})
	if err != nil {
		return
	}
	r.progress.enqueueNotice(encoded)
	r.progressSeen = true
}

func recordTypeOf(record any) string {
	switch typed := record.(type) {
	case responseStreamJSONProgressRecord:
		return typed.RecordType
	case responseStreamJSONStreamGapRecord:
		return typed.RecordType
	case responseStreamJSONCompactionRecord:
		return typed.RecordType
	case responseStreamJSONPrimaryResultRecord:
		return typed.RecordType
	default:
		return ""
	}
}

type responseStreamJSONProgressRecord struct {
	RecordType string  `json:"recordType"`
	Sequence   int64   `json:"sequence,omitempty"`
	DispatchID *string `json:"dispatchId,omitempty"`
	Kind       string  `json:"kind"`
	EventType  string  `json:"eventType"`
	Payload    string  `json:"payload"`
}

type responseStreamJSONStreamGapRecord struct {
	RecordType           string `json:"recordType"`
	Reason               string `json:"reason"`
	DroppedProgressLines int    `json:"droppedProgressLines,omitempty"`
}

type responseStreamJSONCompactionRecord struct {
	RecordType            string `json:"recordType"`
	Reason                string `json:"reason"`
	DroppedSequenceCount  int    `json:"droppedSequenceCount,omitempty"`
	FirstRetainedSequence int64  `json:"firstRetainedSequence,omitempty"`
	LastDroppedSequence   int64  `json:"lastDroppedSequence,omitempty"`
}

type responseStreamJSONPrimaryResultRecord struct {
	RecordType string                        `json:"recordType"`
	Invocation factoryapi.InvocationResponse `json:"invocation"`
}
