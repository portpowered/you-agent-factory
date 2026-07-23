package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/batchload"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func emitCleanInvocationOutcome(
	ctx context.Context,
	cfg RunConfig,
	runner factoryServiceRunner,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	runErr error,
	duration time.Duration,
) error {
	logger := cleanInvocationLogger(cfg.Logger)
	provider, ok := runner.(engineStateSnapshotProvider)
	if !ok {
		if runErr == nil {
			err := &InvocationError{
				Code:    InvocationErrorCodeFailed,
				Message: "clean invocation result snapshot is unavailable",
			}
			recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
				Duration: duration,
				Err:      err,
			})
			return err
		}
		err := newInvocationErrorForRunFailure(runErr, nil)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Err:      err,
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
				Duration: duration,
				Err:      invocationErr,
			})
			return invocationErr
		}
		invocationErr := newInvocationErrorForRunFailure(runErr, nil)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Err:      invocationErr,
		})
		return invocationErr
	}
	if runErr != nil {
		invocationErr := newInvocationErrorForRunFailure(runErr, snapshot)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Snapshot: snapshot,
			Err:      invocationErr,
		})
		return invocationErr
	}
	target, err := cleanInvocationWorkTargetFromFile(
		cfg.WorkRequestFileLoader,
		prepareWorkTarget,
		cfg.WorkFile,
	)
	if err != nil {
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Snapshot: snapshot,
			Err:      err,
		})
		return err
	}
	result, ok := cleanInvocationSuccessFromSnapshot(snapshot, target)
	if !ok {
		invocationErr := cleanInvocationFailureFromSnapshot(snapshot, target)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Snapshot: snapshot,
			Target:   &target,
			Err:      invocationErr,
		})
		return invocationErr
	}
	if err := writeCleanInvocationSuccess(cfg, result); err != nil {
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Snapshot: snapshot,
			Target:   &target,
			Err:      err,
		})
		return err
	}
	recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
		Duration: duration,
		Snapshot: snapshot,
		Target:   &target,
		Success:  &result,
	})
	return nil
}

func newInvocationErrorForRunFailure(
	runErr error,
	snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net],
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
	snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net],
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
	snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net],
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
	snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net],
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
	snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net],
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

func cleanInvocationWorkTargetFromFile(
	load work.RequestFileLoader,
	prepare work.SingleWorkTargetPreparation,
	workFile string,
) (cleanInvocationWorkTarget, error) {
	request, err := batchload.LoadFromFile(load, workFile)
	if err != nil {
		return cleanInvocationWorkTarget{}, err
	}
	if prepare == nil {
		return cleanInvocationWorkTarget{}, fmt.Errorf("clean invocation Work target preparation is required")
	}
	target, err := prepare(request)
	if err != nil {
		return cleanInvocationWorkTarget{}, err
	}
	return cleanInvocationWorkTarget{
		WorkID:       target.WorkID,
		WorkTypeName: target.WorkTypeID,
	}, nil
}

func cleanInvocationSuccessFromSnapshot(
	snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net],
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
	snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net],
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
	snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net],
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
		return fmt.Errorf("clean invocation output is required")
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
)

func writeHumanInvocationOutcome(
	output io.Writer,
	progressSeen bool,
	result apisurface.FactoryInvocationResult,
) error {
	lines := formatHumanInvocationOutcomeLines(result)

	if progressSeen {
		if _, err := fmt.Fprintln(output); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, responseStreamInvocationOutcomeHeader); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(output, line); err != nil {
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

func writeHumanPrimaryResult(output io.Writer, progressSeen bool, text string) error {
	if progressSeen {
		if _, err := fmt.Fprintln(output); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(output, responseStreamPrimaryResultHeader); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(output, text)
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

const (
	factoryEventJSONRecordType                 = "factory_event"
	factoryEventJSONInvocationResultRecordType = "invocation_result"
)

type factoryEventRenderer interface {
	PresentFactoryEvents([]interfaces.FactoryEvent)
	stopProgressRendering()
	writeFinalInvocationResult(apisurface.FactoryInvocationResult) error
}

func invocationFactoryEventRenderer(
	cfg RunConfig,
	presentation factoryvisualization.ResponsePresentation,
) factoryEventRenderer {
	if !isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		return nil
	}
	if cfg.JSONOutput {
		return newJSONFactoryEventRenderer(cfg.Output, presentation)
	}
	return newHumanFactoryEventRenderer(cfg.Output, presentation)
}

type factoryEventStream interface {
	PresentFactoryEvents([]interfaces.FactoryEvent)
	Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
	CloseAndDrain() error
}

type humanFactoryEventRenderer struct {
	stream factoryEventStream
}

func newHumanFactoryEventRenderer(
	output io.Writer,
	presentation factoryvisualization.ResponsePresentation,
) *humanFactoryEventRenderer {
	if output == nil {
		panic("Factory Event output is nil")
	}
	if presentation == nil {
		panic("Factory Event presentation service is nil")
	}
	return &humanFactoryEventRenderer{stream: presentation.OpenBestEffortFactoryEventStream(
		output,
		formatHumanFactoryEvent,
	)}
}

func formatHumanFactoryEvent(event interfaces.FactoryEvent) ([]byte, bool) {
	var message string
	switch event.Type {
	case interfaces.FactoryEventTypeWorkRequest:
		message = formatHumanWorkAccepted(event)
	case interfaces.FactoryEventTypeSessionStarted:
		message = "Factory Session started"
	case interfaces.FactoryEventTypeSessionCompleted:
		message = formatHumanSessionCompleted(event)
	case interfaces.FactoryEventTypeDispatchQueued:
		message = formatHumanDispatchQueued(event)
	case interfaces.FactoryEventTypeDispatchRequest:
		message = formatHumanDispatchStarted(event)
	case interfaces.FactoryEventTypeDispatchResponse:
		message = formatHumanDispatchCompleted(event)
	case interfaces.FactoryEventTypeDispatchInterrupted:
		message = formatHumanDispatchInterrupted(event)
	case interfaces.FactoryEventTypeInferenceRequest:
		message = formatHumanInferenceStarted(event)
	case interfaces.FactoryEventTypeInferenceResponse:
		message = formatHumanInferenceCompleted(event)
	case interfaces.FactoryEventTypeOrchestratorPhaseChanged:
		message = formatHumanOrchestratorPhase(event)
	case interfaces.FactoryEventTypeOrchestratorCheckpointWritten:
		message = formatHumanOrchestratorCheckpoint(event)
	case interfaces.FactoryEventTypeSessionResultUpdated:
		message = formatHumanResultUpdated(event)
	default:
		return nil, false
	}
	sequence := event.Context.Sequence
	if event.Context.SessionSequence != nil {
		sequence = *event.Context.SessionSequence
	}
	return []byte(fmt.Sprintf("[%d] %s", sequence, message)), true
}

func formatHumanWorkAccepted(event interfaces.FactoryEvent) string {
	payload, ok := decodeFactoryEventPayload[work.WorkRequestEventPayload](event)
	if !ok || len(payload.Works) == 0 {
		return withHumanLifecycleSubject("work accepted", firstFactoryEventWorkID(event))
	}
	if len(payload.Works) > 1 {
		return fmt.Sprintf("work accepted: %d items", len(payload.Works))
	}
	subject := payload.Works[0].Name
	if strings.TrimSpace(subject) == "" {
		subject = payload.Works[0].WorkID
	}
	return withHumanLifecycleSubject("work accepted", subject)
}

func formatHumanSessionCompleted(event interfaces.FactoryEvent) string {
	payload, ok := decodeFactoryEventPayload[interfaces.FactorySessionCompletedEventPayload](event)
	if !ok || payload.FinalStatus == "" {
		return "Factory Session completed"
	}
	message := "Factory Session completed: " + string(payload.FinalStatus)
	if payload.FailureDetail != nil {
		message = withHumanLifecycleFailure(message, payload.FailureDetail.Message)
	}
	return message
}

func formatHumanDispatchQueued(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.DispatchQueuedEventPayload](event)
	subject := stringPointerValue(payload.Label)
	if subject == "" {
		subject = stringPointerValue(event.Context.DispatchID)
	}
	return withHumanLifecycleSubject("workstation queued", subject)
}

func formatHumanDispatchStarted(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.DispatchRequestEventPayload](event)
	return withHumanLifecycleSubject("workstation started", payload.TransitionID)
}

func formatHumanDispatchCompleted(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[workerexecution.DispatchResponseEventPayload](event)
	label := "workstation completed"
	if payload.Outcome == workerexecution.OutcomeFailed {
		label = "workstation failed"
	}
	message := withHumanLifecycleSubject(label, payload.TransitionID)
	if payload.Outcome != "" && payload.Outcome != workerexecution.OutcomeAccepted && payload.Outcome != workerexecution.OutcomeFailed {
		message += " (" + string(payload.Outcome) + ")"
	}
	if payload.FailureDetail != nil {
		message = withHumanLifecycleFailure(message, payload.FailureDetail.Message)
	}
	return message
}

func formatHumanDispatchInterrupted(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.DispatchInterruptedEventPayload](event)
	return withHumanLifecycleFailure(
		withHumanLifecycleSubject("workstation interrupted", stringPointerValue(event.Context.DispatchID)),
		payload.Reason,
	)
}

func formatHumanInferenceStarted(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[workerexecution.InferenceRequestEventPayload](event)
	return withHumanLifecycleAttempt("inference started", payload.Attempt)
}

func formatHumanInferenceCompleted(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[workerexecution.InferenceResponseEventPayload](event)
	label := "inference completed"
	if payload.Outcome == workerexecution.InferenceOutcomeFailed {
		label = "inference failed"
	}
	message := withHumanLifecycleAttempt(label, payload.Attempt)
	if payload.FailureDetail != nil {
		message = withHumanLifecycleFailure(message, payload.FailureDetail.Message)
	}
	return message
}

func formatHumanOrchestratorPhase(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.OrchestratorPhaseChangedEventPayload](event)
	message := "workflow phase"
	if phase := boundedHumanProgressPayload(stringPointerValue(event.Context.PhaseName)); phase != "" {
		message += " " + phase
	}
	if payload.PhaseStatus != "" {
		message += ": " + string(payload.PhaseStatus)
	}
	return message
}

func formatHumanOrchestratorCheckpoint(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.OrchestratorCheckpointWrittenEventPayload](event)
	message := withHumanLifecycleSubject("workflow checkpoint written", payload.Label)
	if payload.ResumabilityStatus != "" {
		message += " (" + string(payload.ResumabilityStatus) + ")"
	}
	return message
}

func formatHumanResultUpdated(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.FactorySessionResultUpdatedEventPayload](event)
	message := "final output updated"
	if payload.ResultStatus != "" {
		message += ": " + string(payload.ResultStatus)
	}
	return message
}

func decodeFactoryEventPayload[T any](event interfaces.FactoryEvent) (T, bool) {
	var payload T
	if len(event.Payload) == 0 || json.Unmarshal(event.Payload, &payload) != nil {
		return payload, false
	}
	return payload, true
}

func firstFactoryEventWorkID(event interfaces.FactoryEvent) string {
	if event.Context.WorkIDs == nil || len(*event.Context.WorkIDs) == 0 {
		return ""
	}
	return (*event.Context.WorkIDs)[0]
}

func withHumanLifecycleSubject(label, subject string) string {
	if subject = boundedHumanProgressPayload(subject); subject != "" {
		return label + ": " + subject
	}
	return label
}

func withHumanLifecycleAttempt(label string, attempt int) string {
	if attempt > 0 {
		return fmt.Sprintf("%s (attempt %d)", label, attempt)
	}
	return label
}

func withHumanLifecycleFailure(message, failure string) string {
	if failure = boundedHumanProgressPayload(failure); failure != "" {
		return message + " — " + failure
	}
	return message
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (renderer *humanFactoryEventRenderer) PresentFactoryEvents(events []interfaces.FactoryEvent) {
	if renderer != nil {
		renderer.stream.PresentFactoryEvents(events)
	}
}

func (renderer *humanFactoryEventRenderer) stopProgressRendering() {
	if renderer != nil {
		_ = renderer.stream.CloseAndDrain()
	}
}

func (renderer *humanFactoryEventRenderer) writeFinalInvocationResult(
	result apisurface.FactoryInvocationResult,
) error {
	if renderer == nil {
		return fmt.Errorf("Factory Event renderer is nil")
	}
	_, err := renderer.stream.Finalize(func(writer io.Writer, progressSeen bool) error {
		if result.Status == interfaces.InvocationTerminalStatusCompleted {
			text, textErr := invocationPrimaryResultText(result.PrimaryResult)
			if textErr != nil {
				return textErr
			}
			return writeHumanPrimaryResult(writer, progressSeen, text)
		}
		return writeHumanInvocationOutcome(writer, progressSeen, result)
	})
	return err
}

type jsonFactoryEventRenderer struct {
	stream factoryEventStream
}

func newJSONFactoryEventRenderer(
	output io.Writer,
	presentation factoryvisualization.ResponsePresentation,
) *jsonFactoryEventRenderer {
	if output == nil {
		panic("Factory Event output is nil")
	}
	if presentation == nil {
		panic("Factory Event presentation service is nil")
	}
	return &jsonFactoryEventRenderer{stream: presentation.OpenLosslessFactoryEventStream(
		output,
		func(event interfaces.FactoryEvent) ([]byte, bool) {
			event, ok := factoryEventForPublicPresentation(event)
			if !ok {
				return nil, false
			}
			encoded, err := json.Marshal(factoryEventJSONRecord{
				RecordType: factoryEventJSONRecordType,
				Event:      event,
			})
			return encoded, err == nil
		},
	)}
}

func (renderer *jsonFactoryEventRenderer) PresentFactoryEvents(events []interfaces.FactoryEvent) {
	if renderer != nil {
		renderer.stream.PresentFactoryEvents(events)
	}
}

func (renderer *jsonFactoryEventRenderer) stopProgressRendering() {
	if renderer != nil {
		_ = renderer.stream.CloseAndDrain()
	}
}

func (renderer *jsonFactoryEventRenderer) writeFinalInvocationResult(
	result apisurface.FactoryInvocationResult,
) error {
	if renderer == nil {
		return fmt.Errorf("Factory Event renderer is nil")
	}
	first, err := renderer.stream.Finalize(func(writer io.Writer, _ bool) error {
		encoded, encodeErr := json.Marshal(factoryEventJSONInvocationResultRecord{
			RecordType: factoryEventJSONInvocationResultRecordType,
			Response:   apisurface.InvocationResponseFromResult(result),
		})
		if encodeErr != nil {
			return fmt.Errorf("marshal Factory Event terminal record: %w", encodeErr)
		}
		encoded = append(encoded, '\n')
		written, writeErr := writer.Write(encoded)
		if writeErr == nil && written != len(encoded) {
			writeErr = io.ErrShortWrite
		}
		return writeErr
	})
	if !first {
		return fmt.Errorf("Factory Event invocation result already written")
	}
	return err
}

type factoryEventJSONRecord struct {
	RecordType string                  `json:"recordType"`
	Event      interfaces.FactoryEvent `json:"event"`
}

type factoryEventJSONInvocationResultRecord struct {
	RecordType string                        `json:"recordType"`
	Response   factoryapi.InvocationResponse `json:"response"`
}
