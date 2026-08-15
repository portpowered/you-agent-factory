package runtime

import (
	"context"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	unsafeExpectedArtifactPattern = "<invalid>"
	unrenderableExpectedArtifact  = "<unrenderable>"
)

func verifyExpectedArtifactsForDispatch(
	cfg *runtimeConfig,
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		return result
	}
	declarations := expectedArtifactDeclarationsForDispatch(cfg, request.Input.Dispatch)
	if len(declarations) == 0 {
		return result
	}
	verification := verifyRuntimeExpectedArtifactDeclarations(
		request.Target.Environment.WorkingDirectory,
		request.Input.Dispatch,
		declarations,
		cfg.expectedArtifactFileSystem,
	)
	if verification == nil || len(verification.Entries) == 0 {
		return result
	}
	message := expectedArtifactVerificationMessage(verification)
	result.Outcome = workers.ExecutionOutcomeFailed
	result.ArtifactVerification = verification
	result.Failure = &workers.ExecutionFailure{
		Family:  workers.WorkFailureFamilyTerminal,
		Type:    workers.WorkFailureTypeExpectedArtifactsUnsatisfied,
		Message: message,
		Detail: &workers.FailureDetail{
			Reason:  workers.WorkFailureTypeExpectedArtifactsUnsatisfied,
			Message: message,
		},
	}
	return result
}

func expectedArtifactDeclarationsForDispatch(
	cfg *runtimeConfig,
	dispatch work.WorkDispatch,
) []work.ExpectedArtifactDeclaration {
	if cfg == nil || cfg.net == nil {
		return nil
	}
	var declarations []work.ExpectedArtifactDeclaration
	seen := make(map[work.ExpectedArtifactDeclaration]struct{})
	for _, token := range workers.WorkDispatchInputTokens(dispatch) {
		workType := cfg.net.WorkTypes[token.Color.WorkTypeID]
		if workType == nil {
			continue
		}
		for _, declaration := range workType.ExpectedArtifacts {
			if _, exists := seen[declaration]; exists {
				continue
			}
			seen[declaration] = struct{}{}
			declarations = append(declarations, declaration)
		}
	}
	if transition := cfg.net.Transitions[dispatch.TransitionID]; transition != nil {
		for _, declaration := range transition.ExpectedArtifacts {
			if _, exists := seen[declaration]; exists {
				continue
			}
			seen[declaration] = struct{}{}
			declarations = append(declarations, declaration)
		}
	}
	return declarations
}

func verifyRuntimeExpectedArtifactDeclarations(
	workspace string,
	dispatch work.WorkDispatch,
	declarations []work.ExpectedArtifactDeclaration,
	fileSystem expectedArtifactFileSystem,
) *workers.ExpectedArtifactVerification {
	workspace = strings.TrimSpace(workspace)
	context := dispatch.ExpectedArtifactContext
	if context == nil {
		context = &work.ExpectedArtifactTemplateContext{}
	}
	entries := make([]workers.ExpectedArtifactVerificationEntry, 0, len(declarations))
	for index, declaration := range declarations {
		pattern, err := context.Render(declaration.Pattern, nil)
		if err != nil {
			entries = append(entries, runtimeExpectedArtifactFailureEntry(
				index, declaration.Name, unrenderableExpectedArtifact,
				workers.ExpectedArtifactVerificationReasonMissing,
			))
			continue
		}
		pattern, safe := safeRuntimeExpectedArtifactPattern(pattern)
		if !safe {
			entries = append(entries, runtimeExpectedArtifactFailureEntry(
				index, declaration.Name, unsafeExpectedArtifactPattern,
				workers.ExpectedArtifactVerificationReasonMissing,
			))
			continue
		}
		reason, satisfied := runtimeExpectedArtifactStatus(
			workspace, pattern, declaration.NonEmpty, fileSystem,
		)
		if !satisfied {
			entries = append(entries, runtimeExpectedArtifactFailureEntry(index, declaration.Name, pattern, reason))
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return &workers.ExpectedArtifactVerification{
		Code:    workers.WorkFailureTypeExpectedArtifactsUnsatisfied,
		Entries: entries,
	}
}

func runtimeExpectedArtifactFailureEntry(
	declarationIndex int,
	name string,
	pattern string,
	reason workers.ExpectedArtifactVerificationReason,
) workers.ExpectedArtifactVerificationEntry {
	return workers.ExpectedArtifactVerificationEntry{
		DeclarationIndex: declarationIndex + 1,
		Name:             strings.TrimSpace(name),
		Pattern:          pattern,
		Reason:           reason,
	}
}

func safeRuntimeExpectedArtifactPattern(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}
	portable := filepath.ToSlash(trimmed)
	if pathpkg.IsAbs(portable) || strings.HasPrefix(portable, "/") {
		return "", false
	}
	if len(portable) >= 2 && portable[1] == ':' && isASCIIAlphaRuntime(portable[0]) {
		return "", false
	}
	for _, segment := range strings.Split(portable, "/") {
		if segment == ".." {
			return "", false
		}
	}
	if _, err := pathpkg.Match(portable, ""); err != nil {
		return "", false
	}
	return portable, true
}

func runtimeExpectedArtifactStatus(
	workspace string,
	pattern string,
	nonEmpty bool,
	fileSystem expectedArtifactFileSystem,
) (workers.ExpectedArtifactVerificationReason, bool) {
	if fileSystem == nil || workspace == "" {
		return workers.ExpectedArtifactVerificationReasonMissing, false
	}
	if !strings.ContainsAny(pattern, "*?[") {
		return runtimeExpectedArtifactLiteralStatus(workspace, pattern, nonEmpty, fileSystem)
	}
	return runtimeExpectedArtifactGlobStatus(workspace, pattern, nonEmpty, fileSystem)
}

func runtimeExpectedArtifactLiteralStatus(
	workspace string,
	pattern string,
	nonEmpty bool,
	fileSystem expectedArtifactFileSystem,
) (workers.ExpectedArtifactVerificationReason, bool) {
	candidate := filepath.Join(workspace, filepath.FromSlash(pattern))
	if !runtimeExpectedArtifactPathWithinWorkspace(workspace, candidate, fileSystem) {
		return workers.ExpectedArtifactVerificationReasonMissing, false
	}
	info, err := fileSystem.Stat(candidate)
	if err != nil || info == nil || !info.Mode().IsRegular() {
		return workers.ExpectedArtifactVerificationReasonMissing, false
	}
	if nonEmpty && info.Size() == 0 {
		return workers.ExpectedArtifactVerificationReasonEmpty, false
	}
	return "", true
}

func runtimeExpectedArtifactGlobStatus(
	workspace string,
	pattern string,
	nonEmpty bool,
	fileSystem expectedArtifactFileSystem,
) (workers.ExpectedArtifactVerificationReason, bool) {
	matches, err := fileSystem.Glob(filepath.Join(workspace, filepath.FromSlash(pattern)))
	if err != nil {
		return workers.ExpectedArtifactVerificationReasonMissing, false
	}
	sort.Strings(matches)
	regularFiles := 0
	for _, match := range matches {
		if !runtimeExpectedArtifactPathWithinWorkspace(workspace, match, fileSystem) {
			continue
		}
		info, statErr := fileSystem.Stat(match)
		if statErr != nil || info == nil || !info.Mode().IsRegular() {
			continue
		}
		regularFiles++
		if nonEmpty && info.Size() == 0 {
			return workers.ExpectedArtifactVerificationReasonEmpty, false
		}
	}
	if regularFiles == 0 {
		return workers.ExpectedArtifactVerificationReasonMissing, false
	}
	return "", true
}

func runtimeExpectedArtifactPathWithinWorkspace(
	workspace string,
	candidate string,
	fileSystem expectedArtifactFileSystem,
) bool {
	resolvedWorkspace, err := fileSystem.EvalSymlinks(workspace)
	if err != nil {
		return false
	}
	resolvedCandidate, err := fileSystem.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(resolvedWorkspace), filepath.Clean(resolvedCandidate))
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func expectedArtifactVerificationMessage(verification *workers.ExpectedArtifactVerification) string {
	if verification == nil || len(verification.Entries) == 0 {
		return string(workers.WorkFailureTypeExpectedArtifactsUnsatisfied)
	}
	details := make([]string, 0, len(verification.Entries))
	for _, entry := range verification.Entries {
		details = append(details, fmt.Sprintf("%s=%s (%s)", entry.Name, entry.Pattern, entry.Reason))
	}
	return fmt.Sprintf("%s: %s", verification.Code, strings.Join(details, "; "))
}

func isASCIIAlphaRuntime(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}

func workFailureDetail(failure *workers.ExecutionFailure, message string) *workers.FailureDetail {
	detail := workers.CloneFailureDetail(failure.Detail)
	if detail != nil || message == "" {
		return detail
	}
	return &workers.FailureDetail{Reason: failure.Type, Message: message}
}

func workstationDispatchRequestForResult(
	request workers.WorkstationDispatchRequest,
	executeRequest workers.ExecuteRequest,
) workers.WorkstationDispatchRequest {
	request.Execution.RunnerID = strings.TrimSpace(executeRequest.Target.RunnerID)
	return request
}

// shouldPropagateFailureMetadata preserves the script workstation boundary:
// ordinary process failures are terminal Work results without retry metadata,
// while timeout and missing-executable failures retain their explicit
// classifications.
func shouldPropagateFailureMetadata(
	request workers.WorkstationDispatchRequest,
	failure *workers.ExecutionFailure,
) bool {
	if failure == nil || !isScriptWorkstationDispatch(request) {
		return true
	}
	switch failure.Type {
	case workers.WorkFailureTypeTimeout, workers.WorkFailureTypeMissingExecutable:
		return true
	default:
		return false
	}
}

func isScriptWorkstationDispatch(request workers.WorkstationDispatchRequest) bool {
	return strings.TrimSpace(request.Execution.RunnerID) == "script" ||
		strings.EqualFold(strings.TrimSpace(request.WorkstationName), "script-station")
}

func primaryOutputText(parts []work.WorkContentPart) string {
	for _, part := range parts {
		switch part.Type.Normalized() {
		case work.WorkContentPartTypeText:
			if strings.TrimSpace(part.Text) != "" {
				return part.Text
			}
		case work.WorkContentPartTypeJSON:
			if len(part.JSON) > 0 {
				return string(part.JSON)
			}
		case work.WorkContentPartTypeImage, work.WorkContentPartTypeAudio, work.WorkContentPartTypeBinary:
			if strings.TrimSpace(part.URL) != "" {
				return part.URL
			}
			if strings.TrimSpace(part.File) != "" {
				return part.File
			}
		}
	}
	return ""
}

func providerSessionFromContinuation(
	continuation *workers.ProviderContinuationRef,
) *providers.SessionMetadata {
	if continuation == nil {
		return nil
	}
	id := strings.TrimSpace(continuation.ProviderSessionID)
	if id == "" {
		id = strings.TrimSpace(continuation.ExternalRef)
	}
	if id == "" && strings.TrimSpace(continuation.Provider) == "" {
		return nil
	}
	normalized := continuation.Normalize()
	return &providers.SessionMetadata{
		Provider: providers.ID(normalized.Provider).CanonicalSessionProvider(),
		Kind:     normalized.Kind,
		ID:       id,
	}
}

// materializeWorkerOutputForDispatchWithProposal preserves the detached
// Execute proposal until Runtime has handed it to Work. Legacy WorkResult
// callers continue through the compatibility mapper above.
func materializeWorkerOutputForDispatchWithProposal(
	ctx context.Context,
	workService work.Service,
	net *state.Net,
	idGenerator work.RequestIDGenerator,
	request workerexecution.WorkstationDispatchRequest,
	result workerexecution.WorkResult,
	proposal *workerexecution.ProposedOutput,
) workerexecution.WorkResult {
	proposals := workerexecution.ProposedOutputFromLegacyWorkResult(result)
	fromDetachedOutput := proposal != nil
	if proposal != nil {
		proposals = proposal.Clone()
	}
	return applyMaterializedWorkerOutput(
		ctx,
		workService,
		net,
		idGenerator,
		request.Execution.Dispatch,
		result,
		proposals,
		fromDetachedOutput,
	)
}

func applyMaterializedWorkerOutput(
	ctx context.Context,
	workService work.Service,
	net *state.Net,
	idGenerator work.RequestIDGenerator,
	dispatch work.WorkDispatch,
	result workerexecution.WorkResult,
	proposals workerexecution.ProposedOutput,
	fromDetachedOutput bool,
) workerexecution.WorkResult {
	if result.Cancellation != nil || result.Outcome == workerexecution.OutcomeCanceled {
		result.Outcome = workerexecution.OutcomeCanceled
		if result.Cancellation == nil {
			result.Cancellation = &workerexecution.DispatchCancellation{Reason: workerexecution.DispatchCancellationReasonCanceled}
		}
		result.Output = ""
		result.StructuredResult = nil
		result.StructuredResultPresent = false
		result.OutputContent = nil
		result.RecordedOutputWork = nil
		return result
	}
	if len(proposals.ProposedWork) == 0 &&
		(!fromDetachedOutput || !hasMaterializableOutput(proposals)) {
		return result
	}

	if workService == nil {
		failed := result
		failed.Outcome = workerexecution.OutcomeFailed
		failed.RecordedOutputWork = nil
		failed.OutputContent = nil
		if strings.TrimSpace(failed.Error) == "" {
			failed.Error = "worker output materialization: Work service is required"
		} else {
			failed.Error = fmt.Sprintf("%s; worker output materialization: Work service is required", failed.Error)
		}
		return failed
	}

	materialized, err := workService.MaterializeWorkerOutput(ctx, work.MaterializeWorkerOutputRequest{
		Lineage:           lineageContextFromDispatch(dispatch),
		Primary:           proposals.Primary,
		Feedback:          proposals.Feedback,
		Classification:    proposals.Classification,
		ProposedWork:      workerexecution.WorkProposedItemsFromProposedWork(proposals.ProposedWork),
		ValidWorkTypes:    validWorkTypesFromNet(net),
		ValidStatesByType: validStatesByTypeFromNet(net),
		IDGenerator:       idGenerator,
	})
	if err != nil {
		failed := result
		failed.Outcome = workerexecution.OutcomeFailed
		failed.RecordedOutputWork = nil
		failed.OutputContent = nil
		if strings.TrimSpace(failed.Error) == "" {
			failed.Error = fmt.Sprintf("worker output materialization: %v", err)
		} else {
			failed.Error = fmt.Sprintf(
				"%s; worker output materialization: %v",
				failed.Error,
				err,
			)
		}
		return failed
	}

	next := result
	if fromDetachedOutput {
		next.OutputContent = work.CloneWorkContentParts(proposals.Primary)
	} else {
		// Legacy provider runners return their structured Work payload in the
		// text-shaped Output field. Leave OutputContent empty so the existing
		// token transformer can parse that compatibility representation instead
		// of treating the serialized JSON as literal TEXT content.
		next.OutputContent = nil
	}
	if text := strings.TrimSpace(materialized.PrimaryOutput); text != "" {
		next.Output = text
	}
	if feedback := strings.TrimSpace(materialized.Feedback); feedback != "" {
		next.Feedback = feedback
	}
	if classification := strings.TrimSpace(materialized.Classification); classification != "" {
		next.SelectedClassificationLabel = classification
	}
	next.RecordedOutputWork = append([]work.FactoryWorkItem(nil), materialized.MaterializedWork...)
	return next
}

func hasMaterializableOutput(proposals workerexecution.ProposedOutput) bool {
	return len(proposals.Primary) > 0 ||
		strings.TrimSpace(proposals.Feedback) != "" ||
		strings.TrimSpace(proposals.Classification) != ""
}

func validStatesByTypeFromNet(net *state.Net) map[string]map[string]bool {
	if net == nil {
		return nil
	}
	return state.ValidStatesByType(net.WorkTypes)
}

func lineageContextFromDispatch(dispatch work.WorkDispatch) work.MaterializationLineageContext {
	parent := ""
	if len(dispatch.Execution.WorkIDs) > 0 {
		parent = dispatch.Execution.WorkIDs[0]
	}
	return work.MaterializationLineageContext{
		DispatchID:               dispatch.DispatchID,
		RequestID:                dispatch.Execution.RequestID,
		SourceWorkIDs:            append([]string(nil), dispatch.Execution.WorkIDs...),
		CurrentChainingTraceID:   dispatch.CurrentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), dispatch.PreviousChainingTraceIDs...),
		ParentWorkID:             parent,
		TraceID:                  dispatch.Execution.TraceID,
	}
}

func validWorkTypesFromNet(net *state.Net) map[string]bool {
	if net == nil || len(net.WorkTypes) == 0 {
		return nil
	}
	valid := make(map[string]bool, len(net.WorkTypes))
	for id := range net.WorkTypes {
		valid[id] = true
	}
	return valid
}

type runtimeModelEventOwner interface {
	RuntimeOwnsModelEventRecording() bool
}

func prepareDetachedModelRecording(cfg *runtimeConfig, previous attemptPreparation) attemptPreparation {
	if !runtimeModelRecordingEnabled(cfg) {
		return previous
	}
	return func(ctx context.Context, request *workers.ExecuteRequest) (attemptTerminalFunc, error) {
		var previousTerminal attemptTerminalFunc
		var err error
		if previous != nil {
			previousTerminal, err = previous(ctx, request)
			if err != nil {
				return nil, err
			}
		}
		request.Input.PreparedRequestObserver = func(prepared workers.ExecuteRequest) {
			recordDetachedModelRequest(cfg, prepared)
		}
		return func(terminalContext context.Context, terminalRequest workers.ExecuteRequest, result workers.ExecuteResult, executeErr error) {
			recordDetachedModelResponse(cfg, terminalRequest, result, executeErr)
			if previousTerminal != nil {
				previousTerminal(terminalContext, terminalRequest, result, executeErr)
			}
		}, nil
	}
}

func runtimeModelRecordingEnabled(cfg *runtimeConfig) bool {
	if cfg == nil || cfg.eventHistory == nil {
		return false
	}
	owner, ok := cfg.executeService.(runtimeModelEventOwner)
	if !ok || !owner.RuntimeOwnsModelEventRecording() {
		return false
	}
	_, ok = cfg.eventHistory.(recordings.WorkerEventRecorder)
	return ok
}

func recordDetachedModelRequest(cfg *runtimeConfig, request workers.ExecuteRequest) {
	recorder := runtimeModelRecorder(cfg)
	if recorder == nil || !isModelExecution(request) {
		return
	}
	attempt := request.Attempt.Number
	if attempt <= 0 {
		attempt = 1
	}
	modelRequestID := detachedModelRequestID(request.Correlation.DispatchID, attempt)
	recorder.RecordModelEvent(workers.ModelEvent{
		ID: "factory-event/model-request/" + modelRequestID, Kind: workers.ModelEventKindRequest,
		EventTime: cfg.clock.Now(), Tick: modelEventTick(request),
		DispatchID: request.Correlation.DispatchID, RequestID: request.Correlation.RequestID,
		TraceIDs: nonEmptyStrings(request.Correlation.TraceID), WorkIDs: append([]string(nil), request.Input.Dispatch.Execution.WorkIDs...),
		Request: &workers.ModelRequestEventPayload{
			ModelRequestID: modelRequestID, Attempt: attempt,
			Operation: strings.TrimSpace(request.Input.ModelOperation), Worker: executionWorkerName(request),
			Model: strings.TrimSpace(request.Target.Model.Name), ProviderLocality: strings.TrimSpace(request.Target.Model.Locality),
			WorkingDirectory: optionalString(request.Target.Environment.WorkingDirectory), Worktree: optionalString(request.Target.Workspace.Worktree),
		},
	})
}

func recordDetachedModelResponse(cfg *runtimeConfig, request workers.ExecuteRequest, result workers.ExecuteResult, executeErr error) {
	recorder := runtimeModelRecorder(cfg)
	if recorder == nil || !isModelExecution(request) {
		return
	}
	attempt := request.Attempt.Number
	if attempt <= 0 {
		attempt = 1
	}
	modelRequestID := detachedModelRequestID(request.Correlation.DispatchID, attempt)
	continuation := cloneRuntimeContinuation(result.Continuation)
	payload := workers.ModelResponseEventPayload{
		ModelRequestID: modelRequestID, Attempt: attempt,
		Operation: strings.TrimSpace(request.Input.ModelOperation), Worker: executionWorkerName(request),
		Model: strings.TrimSpace(request.Target.Model.Name), ProviderLocality: strings.TrimSpace(request.Target.Model.Locality),
		DurationMillis: result.Metrics.Duration.Milliseconds(), Continuation: continuation,
		Bindings: resolvedModelBindings(request.Input.ModelBindings),
	}
	if !continuationHasSessionIdentity(continuation) {
		payload.ProviderSession = providerSessionFromExecuteResult(request, result)
	}
	if detachedModelProviderSucceeded(result, executeErr) {
		payload.Outcome = workers.InferenceOutcomeSucceeded
		content := detachedModelResponseContent(result.Output.Primary)
		if len(content) > 0 {
			payload.OutputContent = &content
		} else if output := strings.TrimSpace(primaryOutputText(result.Output.Primary)); output != "" {
			payload.OutputPreview = &output
		}
	} else {
		payload.Outcome = workers.InferenceOutcomeFailed
		payload.FailureDetail = modelFailureDetailFromExecute(result, executeErr)
	}
	if result.Diagnostics != nil {
		safeDiagnostics := workers.SafeWorkDiagnosticsFromWorkDiagnostics(result.Diagnostics.ToWorkDiagnostics())
		payload.Diagnostics, _ = workers.SafeWorkDiagnosticsEventPayload(safeDiagnostics)
		if string(payload.Diagnostics) == "null" {
			payload.Diagnostics = nil
		}
	}
	recorder.RecordModelEvent(workers.ModelEvent{
		ID:   fmt.Sprintf("factory-event/model-response/%s/%d", request.Correlation.DispatchID, attempt),
		Kind: workers.ModelEventKindResponse, EventTime: cfg.clock.Now(), Tick: modelEventTick(request),
		DispatchID: request.Correlation.DispatchID, RequestID: request.Correlation.RequestID,
		TraceIDs: nonEmptyStrings(request.Correlation.TraceID), WorkIDs: append([]string(nil), request.Input.Dispatch.Execution.WorkIDs...),
		Response: &payload,
	})
}

// detachedModelProviderSucceeded keeps provider evidence separate from the
// later Work/output-contract decision. Workers validates the authored output
// contract after the provider runner returns; a contract failure must not
// rewrite an otherwise successful provider response as FAILED.
func detachedModelProviderSucceeded(result workers.ExecuteResult, executeErr error) bool {
	if executeErr != nil {
		return false
	}
	if result.Failure == nil {
		return true
	}
	return result.Failure.Type == workers.WorkFailureTypePermanentBadRequest &&
		strings.EqualFold(strings.TrimSpace(result.Failure.Message), "output contract failed")
}

// detachedModelResponseContent restores structured content that arrived at
// the request-scoped Execute boundary as one text part. The Work materializer
// still consumes the raw worker response through WorkResult.Output, while the
// canonical MODEL_RESPONSE event must expose the same audio/file reference as
// structured output.
func detachedModelResponseContent(parts []work.WorkContentPart) []work.WorkContentPart {
	cloned := work.CloneWorkContentParts(parts)
	if len(cloned) != 1 || cloned[0].Type.Normalized() != work.WorkContentPartTypeText {
		return cloned
	}
	raw := strings.TrimSpace(cloned[0].Text)
	if raw == "" {
		return cloned
	}
	decoded, err := work.ContentFromWorkerOutput(raw)
	if err != nil || len(decoded) == 0 {
		return cloned
	}
	if len(decoded) == 1 && decoded[0].Type.Normalized() == work.WorkContentPartTypeText && decoded[0].Text == raw {
		return cloned
	}
	return decoded
}

func runtimeModelRecorder(cfg *runtimeConfig) recordings.WorkerEventRecorder {
	if !runtimeModelRecordingEnabled(cfg) {
		return nil
	}
	recorder, _ := cfg.eventHistory.(recordings.WorkerEventRecorder)
	return recorder
}

func isModelExecution(request workers.ExecuteRequest) bool {
	return strings.TrimSpace(request.Target.Model.Name) != "" &&
		!strings.EqualFold(strings.TrimSpace(request.Target.RunnerID), "script") &&
		!strings.EqualFold(strings.TrimSpace(request.Target.RunnerID), "inference")
}

func detachedModelRequestID(dispatchID string, attempt int) string {
	return fmt.Sprintf("%s/model-request/%d", strings.TrimSpace(dispatchID), attempt)
}

func modelEventTick(request workers.ExecuteRequest) int {
	if request.Input.Dispatch.Execution.CurrentTick != 0 {
		return request.Input.Dispatch.Execution.CurrentTick
	}
	return request.Input.Dispatch.Execution.DispatchCreatedTick
}

func executionWorkerName(request workers.ExecuteRequest) string {
	if name := strings.TrimSpace(request.Target.WorkerName); name != "" {
		return name
	}
	return strings.TrimSpace(request.Target.WorkerType)
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	value = strings.TrimSpace(value)
	return &value
}

func nonEmptyStrings(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}

func resolvedModelBindings(bindings []workers.ResolvedModelOperationBinding) *[]workers.ResolvedModelOperationBinding {
	if len(bindings) == 0 {
		return nil
	}
	clone := workers.CloneResolvedModelOperationBindings(bindings)
	return &clone
}

func providerSessionFromExecuteResult(
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
) *providers.SessionMetadata {
	if continuation := result.Continuation; continuation != nil {
		id := strings.TrimSpace(continuation.ProviderSessionID)
		if id == "" {
			id = strings.TrimSpace(continuation.ExternalRef)
		}
		if id != "" || strings.TrimSpace(continuation.Provider) != "" {
			return &providers.SessionMetadata{Provider: continuation.Provider, Kind: providers.SessionIDKind, ID: id}
		}
	}

	// Native provider failures and timeouts can occur before a provider emits a
	// session id. Preserve the selected provider identity in the model event so
	// the failed attempt remains attributable without inventing a session id.
	provider := strings.TrimSpace(request.Target.Provider.ID)
	if provider == "" {
		provider = strings.TrimSpace(request.Target.Provider.Alias)
	}
	if provider == "" {
		provider = strings.TrimSpace(request.Target.Model.Provider)
	}
	if provider == "" {
		return nil
	}
	return &providers.SessionMetadata{
		Provider: providers.ID(provider).CanonicalSessionProvider(),
		Kind:     providers.SessionIDKind,
	}
}

func continuationHasSessionIdentity(continuation *workers.ProviderContinuationRef) bool {
	if continuation == nil {
		return false
	}
	return strings.TrimSpace(continuation.ProviderSessionID) != "" ||
		strings.TrimSpace(continuation.ExternalRef) != ""
}

func modelFailureDetailFromExecute(result workers.ExecuteResult, executeErr error) *workers.FailureDetail {
	if result.Failure != nil {
		if result.Failure.Detail != nil {
			clone := *result.Failure.Detail
			return &clone
		}
		return &workers.FailureDetail{Reason: result.Failure.Type, Message: result.Failure.Message}
	}
	if executeErr != nil {
		return &workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: "The model request failed without an available explanation."}
	}
	return nil
}

// resolveRuntimeModelOperationBindings projects authored inference operation
// bindings into the detached Workers request. Factory Runtime owns this
// Definitions-to-execution translation because Workers must not depend on
// Factory Definitions internals.
func resolveRuntimeModelOperationBindings(
	workstation *interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	inputTokens []workers.Token,
) ([]workers.ResolvedModelOperationBinding, error) {
	if workstation == nil || worker == nil || !interfaces.IsInferenceRunWorkstationType(workstation.Type) {
		return nil, nil
	}
	operationName := strings.TrimSpace(workstation.Operation)
	if operationName == "" {
		return nil, nil
	}
	operation, found := runtimeModelOperationByName(worker.Operations, operationName)
	if !found {
		return nil, fmt.Errorf("worker %q does not declare operation %q", worker.Name, operationName)
	}

	authoredBindings := make(map[string]interfaces.ModelOperationBinding, len(workstation.OperationBindings))
	for _, binding := range workstation.OperationBindings {
		if slot := strings.TrimSpace(binding.Slot); slot != "" {
			authoredBindings[slot] = binding
		}
	}
	inputTokens = runtimeNonResourceTokens(inputTokens)
	resolved := make([]workers.ResolvedModelOperationBinding, 0, len(operation.Inputs))
	for _, input := range operation.Inputs {
		binding, ok := authoredBindings[input.Name]
		if !ok {
			binding = interfaces.ModelOperationBinding{
				Slot: input.Name,
				Selector: &interfaces.ModelOperationBindingSelector{
					Slot: input.Name,
				},
			}
		}
		current := resolveRuntimeModelOperationBinding(binding, inputTokens)
		if len(current.Content) == 0 && input.Required {
			return nil, fmt.Errorf("required slot %q could not be resolved for operation %q", input.Name, operationName)
		}
		resolved = append(resolved, current)
	}
	return resolved, nil
}

func runtimeModelOperationByName(
	operations []interfaces.ModelOperation,
	name string,
) (interfaces.ModelOperation, bool) {
	for _, operation := range operations {
		if strings.TrimSpace(operation.Name) == name {
			return operation, true
		}
	}
	return interfaces.ModelOperation{}, false
}

func resolveRuntimeModelOperationBinding(
	binding interfaces.ModelOperationBinding,
	inputTokens []workers.Token,
) workers.ResolvedModelOperationBinding {
	resolved := workers.ResolvedModelOperationBinding{
		Slot:   binding.Slot,
		Source: workers.ModelOperationBindingSourceOmitted,
	}
	if !runtimeModelOperationSelectorIsEmpty(binding.Selector) {
		if part, ok := runtimeMatchingModelOperationContentPart(inputTokens, binding.Selector); ok {
			resolved.Source = workers.ModelOperationBindingSourceInput
			resolved.Content = []work.WorkContentPart{part}
			return resolved
		}
	}
	if len(binding.Config) > 0 {
		resolved.Source = workers.ModelOperationBindingSourceConfig
		resolved.Content = work.CloneWorkContentParts(binding.Config)
		return resolved
	}
	if len(binding.DefaultContent) > 0 {
		resolved.Source = workers.ModelOperationBindingSourceDefault
		resolved.Content = work.CloneWorkContentParts(binding.DefaultContent)
	}
	return resolved
}

func runtimeMatchingModelOperationContentPart(
	inputTokens []workers.Token,
	selector *interfaces.ModelOperationBindingSelector,
) (work.WorkContentPart, bool) {
	for _, token := range inputTokens {
		for _, part := range token.Color.Content {
			if runtimeModelOperationSelectorMatches(part, selector) {
				return part, true
			}
		}
	}
	return work.WorkContentPart{}, false
}

func runtimeModelOperationSelectorMatches(
	part work.WorkContentPart,
	selector *interfaces.ModelOperationBindingSelector,
) bool {
	if selector == nil {
		return false
	}
	if value := strings.TrimSpace(selector.Slot); value != "" && strings.TrimSpace(part.Slot) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Label); value != "" && strings.TrimSpace(part.Label) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Type); value != "" && runtimeModelOperationContentType(part) != value {
		return false
	}
	if value := strings.TrimSpace(selector.Role); value != "" && strings.TrimSpace(part.Role) != value {
		return false
	}
	return true
}

func runtimeModelOperationContentType(part work.WorkContentPart) string {
	switch part.Type.Normalized() {
	case work.WorkContentPartTypeText:
		return interfaces.ModelOperationContentTypeText
	case work.WorkContentPartTypeImage:
		return interfaces.ModelOperationContentTypeImage
	case work.WorkContentPartTypeAudio:
		return interfaces.ModelOperationContentTypeAudio
	case work.WorkContentPartTypeJSON:
		return interfaces.ModelOperationContentTypeJSON
	case work.WorkContentPartTypeBinary:
		return interfaces.ModelOperationContentTypeBinary
	default:
		return strings.TrimSpace(string(part.Type))
	}
}

func runtimeModelOperationSelectorIsEmpty(
	selector *interfaces.ModelOperationBindingSelector,
) bool {
	if selector == nil {
		return true
	}
	return strings.TrimSpace(selector.Slot) == "" &&
		strings.TrimSpace(selector.Label) == "" &&
		strings.TrimSpace(selector.Type) == "" &&
		strings.TrimSpace(selector.Role) == ""
}

func runtimeNonResourceTokens(tokens []workers.Token) []workers.Token {
	if len(tokens) == 0 {
		return nil
	}
	filtered := make([]workers.Token, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == workers.DataTypeResource {
			continue
		}
		filtered = append(filtered, token)
	}
	return filtered
}
