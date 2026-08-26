package runtime

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type runtimeInvocationPromptProvenanceService interface {
	InterpolatePromptWithProvenance(
		string,
		*work.InvocationArguments,
		interfaces.FileReader,
	) (string, []interfaces.InvocationSensitiveTextSpan, error)
}

// recordedTokenUsageFromDiagnostics preserves provider usage in the
// event-first Worker Session projection. Provider-session transcript storage
// is optional, but normalized response metadata is already part of the
// replay-safe dispatch diagnostics and must remain observable when that
// provider source is unavailable.
func recordedTokenUsageFromDiagnostics(diagnostics *workers.SafeWorkDiagnostics) *workersessions.TokenUsage {
	if diagnostics == nil || diagnostics.Provider == nil {
		return nil
	}
	metadata := diagnostics.Provider.ResponseMetadata
	input := recordedOptionalInt(metadata, workers.ProviderResponseMetadataInputTokens)
	cachedInput := recordedOptionalInt(metadata, workers.ProviderResponseMetadataCachedInputTokens)
	output := recordedOptionalInt(metadata, workers.ProviderResponseMetadataOutputTokens)
	reasoningOutput := recordedOptionalInt(metadata, workers.ProviderResponseMetadataReasoningOutputTokens)
	if input == nil && cachedInput == nil && output == nil && reasoningOutput == nil {
		return nil
	}
	var total *int
	if input != nil && output != nil && *input <= int(^uint(0)>>1)-*output {
		value := *input + *output
		total = &value
	}
	return &workersessions.TokenUsage{
		InputTokens:           input,
		CachedInputTokens:     cachedInput,
		OutputTokens:          output,
		ReasoningOutputTokens: reasoningOutput,
		TotalTokens:           total,
	}
}

func recordedOptionalInt(metadata map[string]string, key string) *int {
	value, ok := metadata[key]
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return nil
	}
	return &parsed
}

func recordedDispatchFact(
	dispatchID string,
	association recordedDispatchAssociation,
	requests map[string]recordedDispatchRequest,
	completed map[string]interfaces.FactoryWorldDispatchCompletion,
	providerSessions []interfaces.FactoryWorldProviderSessionRecord,
	active map[string]interfaces.FactoryWorldDispatch,
	events []interfaces.FactoryEvent,
) recordedDispatchObservation {
	fact := recordedDispatchObservation{
		workerSessionID: association.workerSessionID,
		dispatchID:      dispatchID,
		turnID:          association.turnID,
		model:           cloneRecordedString(association.model),
		reasoningEffort: cloneRecordedString(association.reasoningEffort),
		startedAt:       association.eventTime,
		state:           workersessions.StateStarting,
	}
	if request, ok := requests[dispatchID]; ok {
		fact.workIDs = append([]string(nil), request.workIDs...)
		fact.startedAt = request.startedAt
	}
	if dispatch, ok := active[dispatchID]; ok {
		fact.state = workersessions.StateRunning
		fact.startedAt = firstRecordedTime(dispatch.StartedAt, fact.startedAt)
		fact.workIDs = firstRecordedWorkIDs(dispatch.WorkItemIDs, fact.workIDs)
	}
	if dispatch, ok := completed[dispatchID]; ok {
		fact.state = recordedObservationState(dispatch.Result.Outcome)
		fact.startedAt = firstRecordedTime(dispatch.StartedAt, fact.startedAt)
		fact.endedAt = recordedDispatchEnd(dispatch, events, dispatchID)
		fact.workIDs = firstRecordedWorkIDs(dispatch.WorkItemIDs, fact.workIDs)
		fact.failure = recordedFailureWithDiagnostics(
			workers.WorkOutcome(dispatch.Result.Outcome),
			dispatch.Result.FailureDetail,
			dispatch.Result.FailureMetadata,
			fact.state,
			dispatch.Diagnostics,
		)
		fact.tokenUsage = recordedTokenUsageFromDiagnostics(dispatch.Diagnostics)
		fact.provider = cloneProviderMetadata(dispatch.ProviderSession)
	}
	for _, provider := range providerSessions {
		if provider.DispatchID != dispatchID {
			continue
		}
		fact.provider = cloneProviderMetadata(&provider.ProviderSession)
		fact.workIDs = firstRecordedWorkIDs(provider.WorkItemIDs, fact.workIDs)
		fact.failure = firstRecordedFailure(fact.failure, recordedFailureWithDiagnostics(
			workers.OutcomeFailed,
			provider.FailureDetail,
			nil,
			fact.state,
			provider.Diagnostics,
		))
		if fact.tokenUsage == nil {
			fact.tokenUsage = recordedTokenUsageFromDiagnostics(provider.Diagnostics)
		}
		break
	}
	if interruption, ok := recordedDispatchInterruption(events, dispatchID); ok && !fact.state.Terminal() {
		fact.state = workersessions.StateFailed
		fact.workIDs = firstRecordedWorkIDs(interruption.workIDs, fact.workIDs)
		endedAt := interruption.interruptedAt
		if endedAt.IsZero() {
			endedAt = interruption.eventTime
		}
		if !endedAt.IsZero() {
			endedAt = endedAt.UTC()
			fact.endedAt = &endedAt
		}
		reason := strings.TrimSpace(interruption.reason)
		if reason == "" {
			reason = "dispatch interrupted"
		}
		fact.failure = &workersessions.FailureCause{
			Kind:   workersessions.FailureCauseProcessGone,
			Detail: reason,
		}
	}
	fact.stateSequence, fact.stateSequenceKnown = recordedDispatchStateCursor(events, dispatchID)
	return fact
}

func recordedObservationFromFact(fact recordedDispatchObservation, clock factory.Clock) workersessions.Observation {
	state := fact.state
	if state == "" {
		state = workersessions.StateStarting
	}
	observation := workersessions.Observation{
		WorkerSessionID:          fact.workerSessionID,
		Model:                    cloneRecordedString(fact.model),
		ReasoningEffort:          cloneRecordedString(fact.reasoningEffort),
		TokenUsage:               cloneRecordedTokenUsage(fact.tokenUsage),
		ProviderSessionAvailable: fact.provider != nil && fact.provider.ID != "",
		WorkIDs:                  append([]string(nil), fact.workIDs...),
		TurnID:                   fact.turnID,
		AttemptID:                fact.dispatchID,
		State:                    state,
		ConfirmationState:        workersessions.ConfirmationStateUnconfirmed,
		StreamGenerationID:       fact.streamGenerationID,
		StateSequence:            fact.stateSequence,
		StateSequenceKnown:       fact.stateSequenceKnown,
		DurationBasis:            workersessions.DurationBasisUnavailable,
		Transcript:               workersessions.TranscriptAvailabilityUnavailable,
	}
	if fact.provider != nil {
		observation.ProviderSession = providerSessionRef(*fact.provider)
	}
	if !fact.startedAt.IsZero() {
		started := fact.startedAt.UTC()
		observation.StartedAt = &started
		if fact.endedAt != nil {
			ended := fact.endedAt.UTC()
			observation.EndedAt = &ended
			duration := ended.Sub(started)
			if duration < 0 {
				duration = 0
			}
			observation.Duration = &duration
			observation.DurationBasis = workersessions.DurationBasisRecordedTimestamps
		} else if !state.Terminal() && clock != nil {
			duration := clock.Now().Sub(started)
			if duration < 0 {
				duration = 0
			}
			observation.Duration = &duration
			observation.DurationBasis = workersessions.DurationBasisActiveClock
		}
	}
	if fact.failure != nil {
		failure := *fact.failure
		observation.Failure = &failure
	}
	return observation
}

func cloneRecordedTokenUsage(usage *workersessions.TokenUsage) *workersessions.TokenUsage {
	if usage == nil {
		return nil
	}
	clone := usage.Clone()
	return &clone
}

const detachedAgentRunTranscriptSummaryLimit = 160

func recordDetachedAgentRunResponse(
	cfg *runtimeConfig,
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
	executeErr error,
) {
	if cfg == nil || cfg.eventHistory == nil || !runtimeRequestUsesAgentRun(cfg, request) {
		return
	}
	recorder, ok := cfg.eventHistory.(recordings.WorkerEventRecorder)
	if !ok || recorder == nil {
		return
	}
	dispatchID := strings.TrimSpace(request.Correlation.DispatchID)
	if dispatchID == "" {
		return
	}

	transcript := make([]workers.AgentRunTranscriptEntry, 0, 3)
	recordedSystemPrompt, recordedUserMessage := recordedAgentRunPrompts(request)
	appendTranscriptEntry(&transcript, "system", recordedSystemPrompt)
	appendTranscriptEntry(&transcript, "user", recordedUserMessage)
	appendTranscriptEntry(&transcript, "assistant", primaryOutputText(result.Output.Primary))
	safeDiagnostics := workers.SafeWorkDiagnosticsFromWorkDiagnostics(result.Diagnostics.ToWorkDiagnostics())
	if safeDiagnostics == nil {
		safeDiagnostics = &workers.SafeWorkDiagnostics{}
	}
	safeDiagnostics.AgentRun = &workers.SafeAgentRunDiagnostic{
		ExecutionBehavior: workers.AgentRunExecutionBehavior,
		Transcript:        transcript,
	}
	diagnostics, err := workers.SafeWorkDiagnosticsEventPayload(safeDiagnostics)
	if err != nil {
		return
	}
	outcome := result.Outcome
	if outcome == "" && executeErr != nil {
		outcome = workers.ExecutionOutcomeFailed
	}
	recorder.RecordAgentRunEvent(workers.AgentRunResponseEvent{
		ID:                         fmt.Sprintf("factory-event/agent-run-response/%s", dispatchID),
		DispatchID:                 dispatchID,
		EventTime:                  cfg.clock.Now(),
		Tick:                       detachedExecutionTick(request.Input.Dispatch.Execution),
		DeclaredSecretJSONPointers: agentRunSecretJSONPointers(request, transcript),
		Payload: workers.AgentRunResponseEventPayload{
			AgentRunID:     fmt.Sprintf("%s/agent-run/1", dispatchID),
			Diagnostics:    diagnostics,
			DurationMillis: result.Metrics.Duration.Milliseconds(),
			Outcome:        string(outcome),
		},
	})
}

func agentRunSecretJSONPointers(
	request workers.ExecuteRequest,
	transcript []workers.AgentRunTranscriptEntry,
) []string {
	redaction := request.Target.Prompt.Redaction
	if redaction == nil || !agentRunPromptRedactionFailClosed(request, redaction) {
		return nil
	}
	pointers := make([]string, 0, len(transcript))
	for index, entry := range transcript {
		if (entry.Role != "system" || !redaction.RedactSystemPrompt) &&
			(entry.Role != "user" || !redaction.RedactUserMessage) {
			continue
		}
		pointers = append(pointers, fmt.Sprintf("/diagnostics/agentRun/transcript/%d/summary", index))
	}
	return pointers
}

func recordedAgentRunPrompts(request workers.ExecuteRequest) (string, string) {
	systemPrompt := request.Target.Prompt.SystemPrompt
	userMessage := request.Target.Prompt.UserMessage
	redaction := request.Target.Prompt.Redaction
	if redaction == nil || agentRunPromptRedactionFailClosed(request, redaction) {
		return systemPrompt, userMessage
	}
	if redaction.RedactSystemPrompt {
		systemPrompt = redaction.SystemPrompt
	}
	if redaction.RedactUserMessage {
		userMessage = redaction.UserMessage
	}
	return systemPrompt, userMessage
}

func agentRunPromptRedactionFailClosed(
	request workers.ExecuteRequest,
	redaction *workers.PromptRedaction,
) bool {
	if redaction == nil || redaction.FailClosed {
		return redaction != nil && redaction.FailClosed
	}
	return (redaction.RedactSystemPrompt &&
		request.Target.Prompt.SystemPrompt != "" && redaction.SystemPrompt == "") ||
		(redaction.RedactUserMessage &&
			request.Target.Prompt.UserMessage != "" && redaction.UserMessage == "")
}

func detachedExecutionTick(metadata work.ExecutionMetadata) int {
	if metadata.CurrentTick != 0 {
		return metadata.CurrentTick
	}
	return metadata.DispatchCreatedTick
}

func runtimeRequestUsesAgentRun(cfg *runtimeConfig, request workers.ExecuteRequest) bool {
	lookup, ok := runtimeDefinitionLookup(cfg)
	if !ok || lookup == nil {
		return false
	}
	workstation, found := lookup.Workstation(strings.TrimSpace(request.Target.WorkstationName))
	return found && workstation != nil && interfaces.IsAgentRunWorkstationType(workstation.Type)
}

func appendTranscriptEntry(
	transcript *[]workers.AgentRunTranscriptEntry,
	role string,
	content string,
) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	if len(content) > detachedAgentRunTranscriptSummaryLimit {
		content = content[:detachedAgentRunTranscriptSummaryLimit] + "..."
	}
	*transcript = append(*transcript, workers.AgentRunTranscriptEntry{
		Role:    role,
		Summary: content,
	})
}

type runtimePromptFieldProvenance struct {
	resolved  string
	spans     []interfaces.InvocationSensitiveTextSpan
	available bool
	invalid   bool
}

func (provenance runtimePromptFieldProvenance) sensitive() bool {
	return len(provenance.spans) > 0
}

func (provenance runtimePromptFieldProvenance) clone() runtimePromptFieldProvenance {
	provenance.spans = append([]interfaces.InvocationSensitiveTextSpan(nil), provenance.spans...)
	return provenance
}

type runtimeWorkstationPromptProvenanceSet struct {
	body           runtimePromptFieldProvenance
	promptTemplate runtimePromptFieldProvenance
}

func runtimeWorkerPromptProvenance(
	cfg *runtimeConfig,
	worker *interfaces.FactoryWorkerConfig,
	invocation *work.InvocationArguments,
) runtimePromptFieldProvenance {
	if worker == nil {
		return runtimePromptFieldProvenance{}
	}
	authored := worker.Body
	if provenance, found := runtimeAuthoredPromptProvenance(cfg, worker.Name, true); found && provenance.Body != "" {
		authored = provenance.Body
	}
	return runtimePromptFieldProvenanceForText(cfg, authored, invocation)
}

func runtimeWorkstationPromptProvenance(
	cfg *runtimeConfig,
	workstation *interfaces.FactoryWorkstationConfig,
	invocation *work.InvocationArguments,
) runtimeWorkstationPromptProvenanceSet {
	if workstation == nil {
		return runtimeWorkstationPromptProvenanceSet{}
	}
	body := workstation.Body
	promptTemplate := workstation.PromptTemplate
	if provenance, found := runtimeAuthoredPromptProvenance(cfg, workstation.Name, false); found {
		if provenance.Body != "" {
			body = provenance.Body
		}
		if provenance.PromptTemplate != "" {
			promptTemplate = provenance.PromptTemplate
		}
	}
	if source, found := runtimePromptSource(cfg, workstation.Name, false); found && !source.IsTemplate {
		if authoredBody, ok := runtimePromptSourceContent(cfg, workstation.Name, false, false); ok {
			body = authoredBody
		}
	}
	return runtimeWorkstationPromptProvenanceSet{
		body:           runtimePromptFieldProvenanceForText(cfg, body, invocation),
		promptTemplate: runtimePromptFieldProvenanceForText(cfg, promptTemplate, invocation),
	}
}

func runtimeAuthoredPromptProvenance(
	cfg *runtimeConfig,
	name string,
	worker bool,
) (interfaces.RuntimePromptProvenance, bool) {
	if cfg == nil || cfg.runtimeConfig == nil {
		return interfaces.RuntimePromptProvenance{}, false
	}
	lookup, ok := cfg.runtimeConfig.(interfaces.RuntimePromptProvenanceLookup)
	if !ok || lookup == nil {
		return interfaces.RuntimePromptProvenance{}, false
	}
	if worker {
		return lookup.WorkerPromptProvenance(name)
	}
	return lookup.WorkstationPromptProvenance(name)
}

func runtimePromptFieldProvenanceForText(
	cfg *runtimeConfig,
	authored string,
	invocation *work.InvocationArguments,
) runtimePromptFieldProvenance {
	if cfg == nil || cfg.invocationInterpolation == nil || authored == "" {
		return runtimePromptFieldProvenance{}
	}
	service, ok := cfg.invocationInterpolation.(runtimeInvocationPromptProvenanceService)
	if !ok || service == nil {
		if invocationHasSensitiveArgument(invocation) {
			return runtimePromptFieldProvenance{available: true, invalid: true}
		}
		return runtimePromptFieldProvenance{}
	}
	resolved, spans, err := service.InterpolatePromptWithProvenance(
		authored,
		invocation,
		cfg.invocationFileReader,
	)
	provenance := runtimePromptFieldProvenance{
		resolved:  resolved,
		spans:     append([]interfaces.InvocationSensitiveTextSpan(nil), spans...),
		available: true,
	}
	if err != nil {
		provenance.invalid = true
	}
	return provenance
}

func invocationHasSensitiveArgument(invocation *work.InvocationArguments) bool {
	if invocation == nil {
		return false
	}
	for _, argument := range invocation.Arguments {
		if argument.Sensitive {
			return true
		}
	}
	return false
}

func validateRuntimePromptProvenance(
	provenance *runtimePromptFieldProvenance,
	actual string,
) {
	if provenance == nil || !provenance.available || provenance.invalid {
		return
	}
	if provenance.resolved != actual {
		provenance.invalid = true
	}
}

func renderRuntimePrompt(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	tokens []workers.Token,
	workflowContext *workers.Context,
	inputs []workers.WorkInput,
	invocation *work.InvocationArguments,
) error {
	if selection == nil {
		return nil
	}
	if selection.interpolationError != nil {
		return wrapRuntimePromptRenderError(selection.interpolationError)
	}
	if err := interpolateRuntimePromptTemplate(cfg, selection, invocation); err != nil {
		return wrapRuntimePromptRenderError(err)
	}
	if err := renderRuntimePromptMessage(cfg, selection, tokens, workflowContext, inputs); err != nil {
		return wrapRuntimePromptRenderError(err)
	}
	if err := resolveRuntimeTemplateFields(cfg, selection, tokens, workflowContext); err != nil {
		return wrapRuntimePromptRenderError(err)
	}
	selection.promptRedaction = buildRuntimePromptRedaction(cfg, selection, tokens, workflowContext)
	return nil
}

func interpolateRuntimePromptTemplate(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	invocation *work.InvocationArguments,
) error {
	if cfg == nil || cfg.invocationInterpolation == nil ||
		selection.promptTemplate == "" || selection.promptTemplateInterpolated {
		return nil
	}
	provenance := runtimePromptFieldProvenanceForText(cfg, selection.promptTemplate, invocation)
	interpolated, err := cfg.invocationInterpolation.InterpolateWorkstationConfig(
		interfaces.FactoryWorkstationConfig{PromptTemplate: selection.promptTemplate},
		invocation,
		cfg.invocationFileReader,
	)
	if err != nil {
		return fmt.Errorf("interpolate workstation prompt: %w", err)
	}
	selection.promptTemplate = interpolated.PromptTemplate
	validateRuntimePromptProvenance(&provenance, selection.promptTemplate)
	if provenance.available {
		selection.userPromptProvenance = provenance
	}
	selection.promptTemplateInterpolated = true
	return nil
}

func renderRuntimePromptMessage(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	tokens []workers.Token,
	workflowContext *workers.Context,
	inputs []workers.WorkInput,
) error {
	if selection.userMessage != "" || selection.promptTemplate == "" {
		return nil
	}
	if cfg == nil || cfg.promptRenderer == nil {
		// Legacy test and adapter callers may not provide the optional renderer.
		// Preserve their detached execution behavior by using the same payload
		// fallback as an empty authored prompt.
		selection.userMessage = workInputMessage(inputs)
		return nil
	}
	rendered, err := cfg.promptRenderer.RenderPrompt(
		selection.promptTemplate,
		tokens,
		workflowContext,
	)
	if err != nil {
		return fmt.Errorf("render workstation prompt: %w", err)
	}
	selection.userMessage = rendered
	return nil
}

func resolveRuntimeTemplateFields(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	tokens []workers.Token,
	workflowContext *workers.Context,
) error {
	if cfg == nil || cfg.templateFieldResolver == nil {
		return nil
	}
	resolved, err := cfg.templateFieldResolver.ResolveTemplateFields(
		selection.workingDirectory,
		selection.environment,
		tokens,
		workflowContext,
		selection.worktree,
	)
	if err != nil {
		return fmt.Errorf("resolve workstation execution fields: %w", err)
	}
	if resolved != nil {
		selection.workingDirectory = resolved.WorkingDirectory
		selection.worktree = resolved.Worktree
		selection.environment = cloneRuntimeStringMap(resolved.Env)
	}
	return nil
}

const runtimePromptRedactionMarker = "<redacted>"

func buildRuntimePromptRedaction(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	tokens []workers.Token,
	workflowContext *workers.Context,
) *workers.PromptRedaction {
	if selection == nil {
		return nil
	}
	system := selection.systemPromptProvenance
	user := selection.userPromptProvenance
	if !system.available && !user.available {
		return nil
	}
	redaction := &workers.PromptRedaction{}
	applyRuntimeSystemPromptRedaction(selection, &system, redaction)
	applyRuntimeUserPromptRedaction(cfg, selection, &user, tokens, workflowContext, redaction)
	if !redaction.RedactSystemPrompt && !redaction.RedactUserMessage && !redaction.FailClosed {
		return nil
	}
	return redaction
}

func applyRuntimeSystemPromptRedaction(
	selection *runtimeExecutionSelection,
	provenance *runtimePromptFieldProvenance,
	redaction *workers.PromptRedaction,
) {
	if provenance.invalid {
		redaction.FailClosed = true
		redaction.RedactSystemPrompt = selection.systemPrompt != ""
	}
	if !provenance.sensitive() {
		return
	}
	redaction.RedactSystemPrompt = true
	safe, ok := redactRuntimePromptText(selection.systemPrompt, provenance.spans)
	if !ok {
		redaction.FailClosed = true
		return
	}
	redaction.SystemPrompt = safe
}

func applyRuntimeUserPromptRedaction(
	cfg *runtimeConfig,
	selection *runtimeExecutionSelection,
	provenance *runtimePromptFieldProvenance,
	tokens []workers.Token,
	workflowContext *workers.Context,
	redaction *workers.PromptRedaction,
) {
	if provenance.invalid {
		redaction.FailClosed = true
		redaction.RedactUserMessage = selection.userMessage != ""
	}
	if !provenance.sensitive() {
		return
	}
	redaction.RedactUserMessage = true
	safeTemplate, ok := redactRuntimePromptText(selection.promptTemplate, provenance.spans)
	if !ok || cfg == nil || cfg.promptRenderer == nil {
		redaction.FailClosed = true
		return
	}
	safe, err := cfg.promptRenderer.RenderPrompt(safeTemplate, tokens, workflowContext)
	if err != nil {
		redaction.FailClosed = true
		return
	}
	redaction.UserMessage = safe
}

func redactRuntimePromptText(
	value string,
	spans []interfaces.InvocationSensitiveTextSpan,
) (string, bool) {
	if len(spans) == 0 || !utf8.ValidString(value) {
		return "", false
	}
	ordered := append([]interfaces.InvocationSensitiveTextSpan(nil), spans...)
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].Start != ordered[right].Start {
			return ordered[left].Start < ordered[right].Start
		}
		return ordered[left].End < ordered[right].End
	})
	var builder strings.Builder
	cursor := 0
	for _, span := range ordered {
		if span.Start < cursor || span.Start < 0 || span.End <= span.Start || span.End > len(value) {
			return "", false
		}
		if !utf8.ValidString(value[span.Start:span.End]) ||
			(span.Start > 0 && !utf8.ValidString(value[:span.Start])) ||
			(span.End < len(value) && !utf8.ValidString(value[:span.End])) {
			return "", false
		}
		builder.WriteString(value[cursor:span.Start])
		builder.WriteString(runtimePromptRedactionMarker)
		cursor = span.End
	}
	builder.WriteString(value[cursor:])
	return builder.String(), true
}

func completedFlushWatermarkReader(
	ledger recordings.RuntimeLedger,
) recordings.CompletedFlushWatermarkReader {
	if ledger == nil {
		return nil
	}
	reader, _ := ledger.(recordings.CompletedFlushWatermarkReader)
	return reader
}

// annotateRecordedFact attaches the canonical stream identity to the state
// cursor before the detached observation crosses the runtime boundary.
func (s *recordedWorkerSessionObservation) annotateRecordedFact(
	fact recordedDispatchObservation,
) recordedDispatchObservation {
	if s == nil || s.ledger == nil {
		return fact
	}
	fact.streamGenerationID = strings.TrimSpace(s.ledger.StreamGenerationID())
	return fact
}

type completedFlushWatermarkSample struct {
	generationID string
	watermark    recordings.CanonicalEventCursor
	available    bool
}

// sampleCompletedFlushWatermark captures the one durability boundary used by
// a read response. Keeping the sample separate from decoration prevents a
// response assembled from recorded and live observations from observing two
// different flush completions.
func (s *recordedWorkerSessionObservation) sampleCompletedFlushWatermark() completedFlushWatermarkSample {
	if s == nil || s.ledger == nil || s.durability == nil {
		return completedFlushWatermarkSample{}
	}
	generationID := strings.TrimSpace(s.ledger.StreamGenerationID())
	if generationID == "" {
		return completedFlushWatermarkSample{}
	}
	watermark, ok := s.durability.CompletedFlushWatermark(generationID)
	if !ok || strings.TrimSpace(watermark.StreamGenerationID) != generationID {
		return completedFlushWatermarkSample{generationID: generationID}
	}
	return completedFlushWatermarkSample{
		generationID: generationID,
		watermark:    watermark,
		available:    true,
	}
}

// applyConfirmation decorates observations with a previously sampled
// completed-flush watermark. The process-local registry remains the fallback
// source, so every observation starts UNCONFIRMED and can become CONFIRMED
// only when its recorded state cursor is known and covered by the
// generation-matching watermark.
func (s *recordedWorkerSessionObservation) applyConfirmation(
	observations []workersessions.Observation,
	sample completedFlushWatermarkSample,
) {
	if len(observations) == 0 {
		return
	}
	for index := range observations {
		observations[index].ConfirmationState = workersessions.ConfirmationStateUnconfirmed
	}
	if !sample.available {
		return
	}
	for index := range observations {
		observation := &observations[index]
		if observation.StateSequenceKnown &&
			observation.StreamGenerationID == sample.generationID &&
			observation.StateSequence <= int64(sample.watermark.Sequence) {
			observation.ConfirmationState = workersessions.ConfirmationStateConfirmed
		}
	}
}

func (s *recordedWorkerSessionObservation) confirmedObservation(
	observation workersessions.Observation,
) workersessions.Observation {
	values := []workersessions.Observation{observation}
	s.applyConfirmation(values, s.sampleCompletedFlushWatermark())
	return values[0]
}

// recordedDispatchStateCursor returns the latest canonical dispatch lifecycle
// event for the dispatch. It is the cursor responsible for the projected
// Worker Session state or terminal outcome, rather than merely the association
// event that made the Worker Session addressable.
func recordedDispatchStateCursor(
	events []interfaces.FactoryEvent,
	dispatchID string,
) (int64, bool) {
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return 0, false
	}
	var (
		sequence int64
		found    bool
	)
	for _, event := range cloneAndSortFactoryEvents(events) {
		if stringPointerValue(event.Context.DispatchID) != dispatchID {
			continue
		}
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchRequest,
			interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			interfaces.FactoryEventTypeDispatchQueued,
			interfaces.FactoryEventTypeDispatchResponse,
			interfaces.FactoryEventTypeDispatchInterrupted,
			interfaces.FactoryEventTypeDispatchReconciled:
			sequence = int64(event.Context.Sequence)
			found = true
		}
	}
	return sequence, found
}

// cloneFactoryEventsInOrder keeps a detached copy without changing the
// append-order prefix used to decide whether restored state is still current.
func cloneFactoryEventsInOrder(events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
	if len(events) == 0 {
		return nil
	}
	cloned := make([]interfaces.FactoryEvent, len(events))
	for index, event := range events {
		cloned[index] = event.Clone()
	}
	return cloned
}

func (s *recordedWorkerSessionObservation) projectRecordedWorldState(
	ctx context.Context,
	events []interfaces.FactoryEvent,
	ordered []interfaces.FactoryEvent,
	selectedTick int,
) (interfaces.FactoryWorldState, error) {
	if restored, ok := restoredWorldStateForEvents(s.restoredWorldState, s.restoredEventPrefix, events); ok {
		return *restored, nil
	}
	if s == nil || s.projector == nil {
		return interfaces.FactoryWorldState{}, workersessions.ErrObservationProjectionUnavailable
	}
	world, err := s.projector(ordered, selectedTick)
	if err != nil {
		return interfaces.FactoryWorldState{}, workersessions.ErrObservationProjectionUnavailable
	}
	return world, nil
}

func restoredWorldStateForEvents(
	state *interfaces.FactoryWorldState,
	prefix []interfaces.FactoryEvent,
	events []interfaces.FactoryEvent,
) (*interfaces.FactoryWorldState, bool) {
	if state == nil || len(prefix) == 0 || len(events) < len(prefix) {
		return nil, false
	}
	for index := range prefix {
		if !sameFactoryEventIdentity(prefix[index], events[index]) {
			return nil, false
		}
	}
	for _, event := range events[len(prefix):] {
		if factoryEventRequiresWorkerSessionProjection(*state, event) {
			return nil, false
		}
	}
	return state, true
}

func sameFactoryEventIdentity(left, right interfaces.FactoryEvent) bool {
	if left.Id != "" || right.Id != "" {
		return left.Id == right.Id
	}
	return left.Type == right.Type &&
		left.Context.Tick == right.Context.Tick &&
		left.Context.Sequence == right.Context.Sequence &&
		left.Context.EventTime.Equal(right.Context.EventTime)
}

func factoryEventRequiresWorkerSessionProjection(
	state interfaces.FactoryWorldState,
	event interfaces.FactoryEvent,
) bool {
	switch event.Type {
	case interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventTypeInitialStructureRequest,
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeSessionLifecycleControl,
		interfaces.FactoryEventTypeSessionPaused,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionResumed,
		interfaces.FactoryEventTypeSessionCompleted,
		interfaces.FactoryEventTypeRunResponse:
		return false
	case interfaces.FactoryEventTypeWorkRequest:
		return !restoredWorkRequestEventIsKnown(state, event)
	default:
		return true
	}
}

func restoredWorkRequestEventIsKnown(
	state interfaces.FactoryWorldState,
	event interfaces.FactoryEvent,
) bool {
	workIDs := pointerStringSlice(event.Context.WorkIDs)
	for _, workID := range workIDs {
		if _, ok := state.WorkItemsByID[workID]; !ok {
			return false
		}
	}
	requestID := stringPointerValue(event.Context.RequestID)
	if requestID != "" {
		for key, request := range state.WorkRequestsByID {
			if key == requestID || request.RequestID == requestID {
				return len(workIDs) > 0 || len(request.WorkItems) > 0
			}
		}
		return false
	}
	return len(workIDs) > 0
}

func newRecordedWorkerSessionObservationWithRecording(
	live workersessions.Service,
	ledger recordings.RuntimeLedger,
	projector factory.WorldStateProjector,
	clock factory.Clock,
	providerSessions providersessions.Service,
	replayEvents []interfaces.FactoryEvent,
	recordingID string,
	recordingReader recordings.WorkerRecordingReader,
	factorySessionIDs ...string,
) workersessions.Service {
	return newRecordedWorkerSessionObservationWithRestoredState(
		live, ledger, projector, clock, providerSessions, replayEvents, recordingID,
		recordingReader, nil, nil, factorySessionIDs...,
	)
}

func newRecordedWorkerSessionObservationWithRestoredState(
	live workersessions.Service,
	ledger recordings.RuntimeLedger,
	projector factory.WorldStateProjector,
	clock factory.Clock,
	providerSessions providersessions.Service,
	replayEvents []interfaces.FactoryEvent,
	recordingID string,
	recordingReader recordings.WorkerRecordingReader,
	restoredWorldState *interfaces.FactoryWorldState,
	restoredEventPrefix []interfaces.FactoryEvent,
	factorySessionIDs ...string,
) workersessions.Service {
	factorySessionID := ""
	if len(factorySessionIDs) > 0 {
		factorySessionID = strings.TrimSpace(factorySessionIDs[0])
	}
	return &recordedWorkerSessionObservation{
		Service:             live,
		ledger:              ledger,
		durability:          completedFlushWatermarkReader(ledger),
		projector:           projector,
		clock:               clock,
		providerSessions:    providerSessions,
		replayEvents:        cloneAndSortFactoryEvents(replayEvents),
		restoredWorldState:  restoredWorldState,
		restoredEventPrefix: cloneFactoryEventsInOrder(restoredEventPrefix),
		recordingID:         strings.TrimSpace(recordingID),
		recordingReader:     recordingReader,
		factorySessionID:    factorySessionID,
	}
}
