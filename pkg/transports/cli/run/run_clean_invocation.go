package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
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
		if failure != nil && failure.Type == workerexecution.WorkFailureTypeTimeout {
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
		if completion.FailureMetadata != nil && completion.FailureMetadata.Type == workerexecution.WorkFailureTypeTimeout {
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
		if completion.Outcome != workerexecution.OutcomeFailed {
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
	normalized, err := requests.NormalizeWorkRequest(request, work.WorkRequestNormalizeOptions{})
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
	tokens := make([]*factorytoken.Token, 0, len(snapshot.Marking.Tokens))
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
		if completion.Outcome != workerexecution.OutcomeAccepted {
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

func cleanInvocationTokenMatches(net *state.Net, token *factorytoken.Token, target cleanInvocationWorkTarget) bool {
	if net == nil || token == nil {
		return false
	}
	if token.Color.DataType == factorytoken.DataTypeResource {
		return false
	}
	if token.Color.WorkID != target.WorkID || token.Color.WorkTypeID != target.WorkTypeName {
		return false
	}
	return net.StateCategoryForPlace(token.PlaceID) == state.StateCategoryTerminal
}

func cleanInvocationSuccessFromToken(token *factorytoken.Token) cleanInvocationSuccess {
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
	responseStreamPrimaryResultHeader     = "--- primary result ---"
	responseStreamInvocationOutcomeHeader = "--- invocation outcome ---"
	maxHumanProgressLineBytes             = 1024

	responseStreamJSONRecordResponseEvent    = "response_event"
	responseStreamJSONRecordInvocationResult = "invocation_result"
)

// responseStreamRenderer writes ordered canonical progress output followed by
// the final invocation result.
type responseStreamRenderer interface {
	stopProgressRendering()
	writeFinalInvocationResult(result apisurface.FactoryInvocationResult) error
}

// humanResponseStreamRenderer prints canonical response-event progress to
// stdout and keeps the final invocation result separate from transient output.
type humanResponseStreamRenderer struct {
	mu                   sync.Mutex
	output               io.Writer
	progress             *responseStreamProgressWriter
	finalOnce            sync.Once
	finalErr             error
	lastResponseSequence int64
	progressLines        int
	progressSeen         bool
	backlogNotified      bool
}

func newHumanResponseStreamRenderer(output io.Writer) *humanResponseStreamRenderer {
	if output == nil {
		output = os.Stdout
	}
	return &humanResponseStreamRenderer{
		output:   output,
		progress: newResponseStreamProgressWriter(output),
	}
}

func (r *humanResponseStreamRenderer) stopProgressRendering() {
	if r == nil {
		return
	}
	r.progress.stopAndDrain()
}

func (r *humanResponseStreamRenderer) writeFinalInvocationResult(
	result apisurface.FactoryInvocationResult,
) error {
	if r == nil {
		return fmt.Errorf("response-stream renderer is nil")
	}
	r.finalOnce.Do(func() {
		r.finalErr = r.writeFinalInvocationResultOnce(result)
	})
	return r.finalErr
}

func (r *humanResponseStreamRenderer) writeFinalInvocationResultOnce(
	result apisurface.FactoryInvocationResult,
) error {
	r.stopProgressRendering()
	r.progress.acquireOutputExclusive()
	defer r.progress.releaseOutputExclusive()
	if result.Status == interfaces.InvocationTerminalStatusCompleted {
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

func boundedHumanProgressPayload(payload string) string {
	trimmed := normalizeHumanProgressField(payload)
	if trimmed == "" {
		return ""
	}
	if maxHumanProgressLineBytes <= 0 || len(trimmed) <= maxHumanProgressLineBytes {
		return trimmed
	}
	const omissionMarker = "..."
	budget := maxHumanProgressLineBytes - len(omissionMarker)
	if budget <= 0 {
		return omissionMarker
	}
	end := 0
	for end < len(trimmed) {
		_, size := utf8.DecodeRuneInString(trimmed[end:])
		if end+size > budget {
			break
		}
		end += size
	}
	return strings.TrimSpace(trimmed[:end]) + omissionMarker
}

func normalizeHumanProgressField(value string) string {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(normalized), " ")
}

func (r *humanResponseStreamRenderer) writeProgressLineLocked(payload string) {
	if strings.TrimSpace(payload) == "" {
		return
	}
	if !r.progress.enqueue([]byte(payload)) {
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
}

// jsonResponseStreamRenderer emits canonical response-event NDJSON followed by
// the shared invocation response.
type jsonResponseStreamRenderer struct {
	mu           sync.Mutex
	writer       *canonicalResponseStreamWriter
	finalWritten bool
}

func newJSONResponseStreamRenderer(output io.Writer) *jsonResponseStreamRenderer {
	if output == nil {
		output = os.Stdout
	}
	return &jsonResponseStreamRenderer{
		writer: newCanonicalResponseStreamWriter(output),
	}
}

func (r *jsonResponseStreamRenderer) stopProgressRendering() {
	if r == nil {
		return
	}
	_ = r.writer.closeAndDrain()
}

func (r *jsonResponseStreamRenderer) onResponseEvents(events []responseevents.FactoryResponseEvent) {
	if r == nil {
		return
	}
	for _, event := range events {
		r.writeRecord(responseStreamJSONResponseEventRecord{
			RecordType: responseStreamJSONRecordResponseEvent,
			Event:      event,
		})
	}
}

func (r *jsonResponseStreamRenderer) writeFinalInvocationResult(
	result apisurface.FactoryInvocationResult,
) error {
	if r == nil {
		return fmt.Errorf("response-stream renderer is nil")
	}
	r.mu.Lock()
	if r.finalWritten {
		r.mu.Unlock()
		return fmt.Errorf("response-stream invocation result already written")
	}
	err := r.writeRecordLocked(responseStreamJSONInvocationResultRecord{
		RecordType: responseStreamJSONRecordInvocationResult,
		Invocation: apisurface.InvocationResponseFromResult(result),
	})
	if err == nil {
		r.finalWritten = true
	}
	r.mu.Unlock()
	if err != nil {
		return err
	}
	return r.writer.closeAndDrain()
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
	return r.writer.enqueue(encoded)
}

type responseStreamJSONResponseEventRecord struct {
	RecordType string                              `json:"recordType"`
	Event      responseevents.FactoryResponseEvent `json:"event"`
}

type responseStreamJSONInvocationResultRecord struct {
	RecordType string                        `json:"recordType"`
	Invocation factoryapi.InvocationResponse `json:"invocation"`
}
