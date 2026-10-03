// Package factorysession exposes MCP tool discovery for durable Factory Session
// operations backed by the shared factorysessionexecution service contract.
package factorysession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// Tool names use Factory Session vocabulary and align with durable REST routes.
const (
	ToolListSessions   = "you.factory_session.list"
	ToolValidateSource = "you.factory_session.validate_source"
	ToolStartSync      = "you.factory_session.start_sync"
	ToolStartAsync     = "you.factory_session.start_async"
	ToolGetSession     = "you.factory_session.get"
	ToolGetResult      = "you.factory_session.get_result"
	ToolListDispatches = "you.factory_session.list_dispatches"
	ToolListArtifacts  = "you.factory_session.list_artifacts"
	ToolControl        = "you.factory_session.control"
	ToolReadEvents     = "you.factory_session.read_events"
	ToolSubagent       = "you.subagent"
)

// Cleanup shares one small budget after invocation termination.
const subagentCleanupBudget = 17 * time.Second
const subagentCloseTimeout = 15 * time.Second

// subagentSnapshotTimeout bounds the best-effort timeout progress snapshot so
// capturing it never materially delays cleanup.
const subagentSnapshotTimeout = 2 * time.Second

// Non-cooperative service calls cannot be stopped by Go contexts. Keep at most
// this many abandoned calls of each kind in flight process-wide.
var subagentAdmissionSlots = make(chan struct{}, 16)
var subagentInvokeSlots = make(chan struct{}, 16)
var subagentSnapshotSlots = make(chan struct{}, 16)
var errSubagentCapacity = errors.New("subagent operation capacity exhausted")
var errSubagentModelProviderConflict = errors.New("selected model requires provider opencode, but a different provider was requested")

// subagentDefaultTimeoutMillis bounds a subagent wait when the caller omits timeoutMillis.
const subagentDefaultTimeoutMillis = int64((20 * time.Minute) / time.Millisecond)
const subagentMaxTimeoutMillis = int64((1<<63 - 1) / time.Millisecond)

// Stable error envelope fields shared by every dynamic workflow MCP tool.
var sharedErrorStableFields = []string{
	"error.code",
	"error.message",
	"error.retryable",
	"error.sessionId",
	"error.details",
}

// ToolDefinition is one discoverable MCP tool with typed schemas and documented
// stable response fields for success and error envelopes.
type ToolDefinition struct {
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	InputSchema         map[string]any `json:"inputSchema"`
	OutputSchema        map[string]any `json:"outputSchema"`
	SuccessStableFields []string       `json:"successStableFields"`
	ErrorStableFields   []string       `json:"errorStableFields"`
}

// ToolErrorEnvelope is the stable MCP failure shape for Factory Session tools.
type ToolErrorEnvelope struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	SessionID string         `json:"sessionId,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// ToolResponse wraps one tool outcome with either a typed result or a stable error.
type ToolResponse[T any] struct {
	Result *T                 `json:"result,omitempty"`
	Error  *ToolErrorEnvelope `json:"error,omitempty"`
}

// MarshalJSON encodes one tool definition for MCP hosts and mock clients.
func (t ToolDefinition) MarshalJSON() ([]byte, error) {
	type alias ToolDefinition
	return json.Marshal(alias(t))
}

// ValidateSource runs the canonical Factory preview contract for the
// you.factory_session.validate_source MCP tool without provider execution.
func ValidateSource(
	ctx context.Context,
	workflows factoryruntime.WorkflowPreviewOperation,
	input factoryapi.FactoryPreviewRequest,
) ToolResponse[factoryapi.FactoryPreviewResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.FactoryPreviewResult](ctx); done {
		return response
	}
	previewInput, err := apisurface.FactoryPreviewInputFromAPI(input)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}

	if workflows == nil {
		envelope := requestValidationErrorEnvelope(fmt.Errorf("workflow preview is unavailable"))
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	workflowPreview, err := workflows.PreviewWorkflow(ctx, previewInput)
	if err != nil {
		if envelope, ok := contextRequestErrorEnvelope(err); ok {
			return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
		}
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	preview := apisurface.FactoryPreviewResultFromPreview(workflowPreview)
	if !preview.Valid {
		envelope := validationErrorEnvelopeFromPreview(preview)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	return ToolResponse[factoryapi.FactoryPreviewResult]{Result: &preview}
}

// SubagentInput is the simplified request accepted by you.subagent.
type SubagentInput struct {
	Prompt          string `json:"prompt"`
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	WorkingRoot     string `json:"workingRoot,omitempty"`
	TimeoutMillis   *int64 `json:"timeoutMillis,omitempty"`
}

// SubagentResult returns the child response as readable text with the identity
// of the Factory Session used for the invocation.
type SubagentResult struct {
	SessionID string `json:"sessionId"`
	Status    string `json:"status"`
	Text      string `json:"text,omitempty"`
}

// Subagent invokes the packaged @you/subagent Factory through the canonical
// Factory Sessions Service. One explicitly requested provider selection is
// canonicalized through the Providers-owned resolver before the Factory Session
// starts, so an unknown identifier is rejected without dispatching work and
// without claiming any execution happened.
func Subagent(
	ctx context.Context,
	target factorysessionexecution.Service,
	workingRoot string,
	generateID factorysessionexecution.SessionIDGenerator,
	resolveProvider ProviderIdentityResolver,
	input SubagentInput,
) ToolResponse[SubagentResult] {
	if err := validateSubagentRequest(ctx, target, generateID, input); err != nil {
		envelope := subagentRequestErrorEnvelope(err, ctx, target, generateID)
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	timeoutMillis := subagentDefaultTimeoutMillis
	if input.TimeoutMillis != nil {
		timeoutMillis = *input.TimeoutMillis
	}
	callCtx, stopCall := context.WithTimeout(ctx, time.Duration(timeoutMillis)*time.Millisecond)
	defer stopCall()
	selected, providerErr := resolveSubagentProvider(callCtx, resolveProvider, input.Provider, input.Model)
	if providerErr != nil {
		return subagentProviderSelectionFailure(providerErr, input.Provider)
	}
	input.Provider = selected
	requestID := generateID()
	select {
	case subagentAdmissionSlots <- struct{}{}:
	default:
		return subagentCapacityFailure("", requestID)
	}
	releaseAdmission := true
	defer func() {
		if releaseAdmission {
			<-subagentAdmissionSlots
		}
	}()
	if input.WorkingRoot != "" {
		workingRoot = input.WorkingRoot
	}
	started, err, startAbandoned := startSubagentSession(callCtx, target, workingRoot, requestID)
	if startAbandoned {
		releaseAdmission = false // The Start goroutine owns the slot and any late cleanup.
	}
	if err != nil {
		return subagentStartFailure(err, requestID, timeoutMillis, input)
	}
	if started.SessionID == "" {
		return subagentExecutionFailure(fmt.Errorf("subagent Factory Session identity is missing"))
	}
	result, invokeErr := invokeSubagent(callCtx, target, started.SessionID, workingRoot, requestID, timeoutMillis, input)
	progress, closeErr := cleanupSubagent(ctx, target, started.SessionID, result, invokeErr, &releaseAdmission)
	if closeErr != nil {
		return subagentCleanupError(closeErr, started.SessionID, requestID)
	}
	if invokeErr != nil {
		return subagentClosedFailure(subagentInvokeError(invokeErr, started.SessionID, requestID, timeoutMillis, input, progress))
	}
	if result.Status != factorysessionexecution.InvocationTerminalStatusCompleted {
		return subagentClosedFailure(subagentTerminalFailure(started.SessionID, result, timeoutMillis, input, progress))
	}
	response := SubagentResult{
		SessionID: started.SessionID,
		Status:    string(result.Status),
		Text:      subagentPrimaryText(result.PrimaryResult),
	}
	if strings.TrimSpace(response.Text) == "" {
		envelope := ToolErrorEnvelope{
			Code:      "factory_session.subagent.empty_result",
			Message:   "subagent completed without returning any text result; the ACP peer may have closed without producing output",
			SessionID: started.SessionID,
			Details:   map[string]any{"status": string(result.Status)},
		}
		return subagentClosedFailure(ToolResponse[SubagentResult]{Error: &envelope})
	}
	return ToolResponse[SubagentResult]{Result: &response}
}

func subagentStartFailure(err error, requestID string, timeoutMillis int64, input SubagentInput) ToolResponse[SubagentResult] {
	if errors.Is(err, context.DeadlineExceeded) {
		response := subagentInvocationTimeout("", requestID, timeoutMillis, input, nil)
		response.Error.Details["phase"] = "start"
		return response
	}
	if errors.Is(err, errSubagentCapacity) {
		return subagentCapacityFailure("", requestID)
	}
	return subagentExecutionFailure(err)
}

func startSubagentSession(ctx context.Context, target factorysessionexecution.Service, workingRoot, requestID string) (factorysessionexecution.SessionStartResult, error, bool) {
	return boundedSubagentStart(ctx, target, factorysessionexecution.SessionStartRequest{
		Mode:           factorysessionexecution.SessionOperationModeLive,
		ActivationOnly: true,
		Correlation:    factorysessionexecution.SessionOperationCorrelation{RequestID: requestID},
		Definition: factorysessionexecution.SessionDefinitionSelection{
			FactoryID: factorydefinitions.PackagedSubagentFactoryName,
		},
		Source: factorysessionexecution.Source{
			Kind:      factoryruntime.WorkflowSourceKindFactoryID,
			FactoryID: factorydefinitions.PackagedSubagentFactoryName,
		},
		Args:       map[string]any{"workingRoot": workingRoot},
		FolderPath: workingRoot,
		RuntimeSelection: &factorysessionexecution.SessionRuntimeSelection{
			ExecutionBaseDir: workingRoot,
			Mode:             factorysessionexecution.SessionRuntimeModeService,
		},
	})
}

func invokeSubagent(callCtx context.Context, target factorysessionexecution.Service, sessionID, workingRoot, requestID string, timeoutMillis int64, input SubagentInput) (factorysessionexecution.InvocationResult, error) {
	args := subagentInvocationArgs(input)
	args["workingRoot"] = workingRoot
	if callCtx.Err() != nil {
		return factorysessionexecution.InvocationResult{}, callCtx.Err()
	}
	return boundedSubagentCall(callCtx, subagentInvokeSlots, func() (factorysessionexecution.InvocationResult, error) {
		return target.Invoke(callCtx, factorysessionexecution.SessionInvokeRequest{
			SessionID:   sessionID,
			Correlation: factorysessionexecution.SessionOperationCorrelation{RequestID: requestID},
			Args:        args,
			Wait:        factorysessionexecution.SessionOperationWait{TimeoutMillis: timeoutMillis, CancelOnTimeout: true},
		})
	})
}

func cleanupSubagent(ctx context.Context, target factorysessionexecution.Service, sessionID string, result factorysessionexecution.InvocationResult, invokeErr error, releaseAdmission *bool) (map[string]any, error) {
	cleanupCtx, stopCleanup := context.WithTimeout(context.WithoutCancel(ctx), subagentCleanupBudget)
	defer stopCleanup()
	// A timeout snapshot is taken before cleanup closes the live Factory
	// Session, because the closed session can no longer report live progress.
	var progress map[string]any
	if subagentInvocationTimedOut(invokeErr, result) {
		progress = subagentProgressSnapshot(cleanupCtx, target, sessionID)
	}
	// Close runs on a detached context so caller cancellation cannot skip
	// cleanup. The deadline keeps cooperative close paths from waiting forever.
	closeCtx, cancelClose := context.WithTimeout(cleanupCtx, subagentCloseTimeout)
	closeResult := make(chan error, 1)
	*releaseAdmission = false // The close goroutine now owns the admission slot.
	go func() {
		defer func() { <-subagentAdmissionSlots }()
		_, err := target.Control(closeCtx, factorysessionexecution.SessionControlRequest{
			SessionID: sessionID,
			Mode:      factorysessionexecution.SessionOperationModeLive,
			Operation: factorysessionexecution.SessionControlClose,
		})
		closeResult <- err
	}()
	var closeErr error
	select {
	case closeErr = <-closeResult:
	case <-closeCtx.Done():
		closeErr = closeCtx.Err()
	}
	cancelClose()
	return progress, closeErr
}

// boundedSubagentStart hands a timed-out start's admission slot to its worker.
// A late successful start is closed there, even after the MCP response returns.
func boundedSubagentStart(
	ctx context.Context,
	target factorysessionexecution.Service,
	request factorysessionexecution.SessionStartRequest,
) (factorysessionexecution.SessionStartResult, error, bool) {
	type outcome struct {
		value factorysessionexecution.SessionStartResult
		err   error
	}
	completed := make(chan outcome)
	go func() {
		started, err := target.Start(ctx, request)
		select {
		case completed <- outcome{value: started, err: err}:
		case <-ctx.Done():
			defer func() { <-subagentAdmissionSlots }()
			if started.SessionID == "" {
				return
			}
			closeCtx, stopClose := context.WithTimeout(context.WithoutCancel(ctx), subagentCloseTimeout)
			defer stopClose()
			_, _ = target.Control(closeCtx, factorysessionexecution.SessionControlRequest{
				SessionID: started.SessionID, Mode: factorysessionexecution.SessionOperationModeLive,
				Operation: factorysessionexecution.SessionControlClose,
			})
		}
	}()
	select {
	case got := <-completed:
		return got.value, got.err, false
	case <-ctx.Done():
		return factorysessionexecution.SessionStartResult{}, ctx.Err(), true
	}
}

func subagentRequestErrorEnvelope(err error, ctx context.Context, target factorysessionexecution.Service, generateID factorysessionexecution.SessionIDGenerator) ToolErrorEnvelope {
	if errors.Is(err, errMissingRequestContext) || ctx == nil || ctx.Err() != nil || target == nil || generateID == nil {
		return executionErrorEnvelope(err)
	}
	return ToolErrorEnvelope{Code: errorCodeBadRequest, Message: err.Error(), Retryable: false}
}

func subagentClosedFailure(response ToolResponse[SubagentResult]) ToolResponse[SubagentResult] {
	envelope := response.Error
	envelope.Message += "; you.subagent cleanup closed the live Factory Session"
	if envelope.Details == nil {
		envelope.Details = make(map[string]any)
	}
	envelope.Details["sessionClosed"] = true
	envelope.Details["sessionIdPurpose"] = "Use sessionId for log correlation; you.factory_session.get may return session.not_found."
	const inspectBeforeRetry = "Inspect the workspace for partial edits and check provider logs using sessionId and any requestId, traceId, or workId."
	if action, ok := envelope.Details["suggestedAction"].(string); ok && action != "" {
		if envelope.Code == "factory_session.subagent.provider_misconfigured" && strings.HasPrefix(action, "Check that Pi's selected model endpoint") {
			envelope.Details["suggestedAction"] = action + " " + inspectBeforeRetry
		} else {
			envelope.Details["suggestedAction"] = inspectBeforeRetry + " " + action
		}
	} else {
		envelope.Details["suggestedAction"] = inspectBeforeRetry
	}
	return response
}

// subagentInvocationTimedOut reports whether one invocation outcome is a
// timeout, covering both a returned context deadline and a terminal TIMED_OUT
// invocation result.
func subagentInvocationTimedOut(invokeErr error, result factorysessionexecution.InvocationResult) bool {
	if invokeErr != nil {
		return errors.Is(invokeErr, context.DeadlineExceeded)
	}
	return result.Status == factorysessionexecution.InvocationTerminalStatusTimedOut
}

// subagentProgressSnapshot reads a best-effort live Factory Session progress
// projection through the owner contract so a timeout keeps bounded runtime
// context after cleanup closes the session. The read runs on a short detached
// context, and any failure degrades to an unavailable marker instead of
// changing the reported outcome or delaying cleanup.
func subagentProgressSnapshot(ctx context.Context, target factorysessionexecution.Service, sessionID string) map[string]any {
	snapshotCtx, cancelSnapshot := context.WithTimeout(ctx, subagentSnapshotTimeout)
	defer cancelSnapshot()
	projection, err := boundedSubagentCall(snapshotCtx, subagentSnapshotSlots, func() (factorysessionexecution.SessionProjection, error) {
		return target.GetFactorySession(snapshotCtx, sessionID)
	})
	if err != nil || snapshotCtx.Err() != nil {
		return map[string]any{"available": false}
	}
	details := subagentProgressDetails(projection)
	if activity := subagentLastProviderActivity(snapshotCtx, target, sessionID); activity != nil {
		details["lastObservedProviderActivity"] = activity
	}
	return details
}

func boundedSubagentCall[T any](ctx context.Context, slots chan struct{}, call func() (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return zero, ctx.Err()
	default:
		return zero, errSubagentCapacity
	}
	if err := ctx.Err(); err != nil {
		<-slots
		return zero, err
	}
	type outcome struct {
		value T
		err   error
	}
	result := make(chan outcome, 1)
	go func() {
		defer func() { <-slots }()
		value, err := call()
		result <- outcome{value: value, err: err}
	}()
	select {
	case got := <-result:
		return got.value, got.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// subagentProgressDetails copies numeric counts and an allowlisted script
// status. Projection strings can contain authored or provider-controlled text.
func subagentProgressDetails(projection factorysessionexecution.SessionProjection) map[string]any {
	details := map[string]any{"available": true}
	runtime := projection.Runtime
	if runtime.Progress.InFlightCount > 0 {
		details["inFlightDispatches"] = runtime.Progress.InFlightCount
	}
	if counts := subagentProgressWorkCounts(runtime.Progress.Categories); counts != nil {
		details["workCounts"] = counts
	}
	if runtime.JavaScript != nil {
		switch runtime.JavaScript.ScriptStatus {
		case "IDLE", "RUNNING", "PAUSED", "FINISHED", "FAILED":
			details["scriptStatus"] = string(runtime.JavaScript.ScriptStatus)
		}
		children := runtime.JavaScript.ChildDispatchCounts
		if children.Completed > 0 || children.Queued > 0 || children.Running > 0 {
			details["childDispatchCounts"] = map[string]any{
				"completed": children.Completed,
				"queued":    children.Queued,
				"running":   children.Running,
			}
		}
	}
	return details
}

// subagentProgressWorkCounts returns work counts only when the live
// projection reports at least one item in any category.
func subagentProgressWorkCounts(categories factorysessionexecution.RuntimeStatusCategories) map[string]any {
	if categories.Initial == 0 && categories.Processing == 0 && categories.Terminal == 0 && categories.Failed == 0 {
		return nil
	}
	return map[string]any{
		"initial":    categories.Initial,
		"processing": categories.Processing,
		"terminal":   categories.Terminal,
		"failed":     categories.Failed,
	}
}

// subagentAttachProgress records one captured snapshot on a timeout envelope.
func subagentAttachProgress(details map[string]any, progress map[string]any) {
	if len(progress) > 0 {
		details["progress"] = progress
	}
}

// subagentProviderErrorObservation reports whether the bounded timeout progress
// captured a terminal provider error event as the last observed provider
// activity, and whether a provider session reference was observed alongside it.
// The observation is timing evidence only: it does not prove the error caused
// the timeout, it is not a terminal Factory failure, and no provider payload or
// reference value leaves this edge. A real timed-out dispatch can be observed
// with a provider session reference, so the reference never suppresses the
// observation.
func subagentProviderErrorObservation(progress map[string]any) (errorObserved, providerSessionObserved bool) {
	activity, ok := progress["lastObservedProviderActivity"].(map[string]any)
	if !ok {
		return false, false
	}
	if activity["kind"] != string(workers.KindError) || activity["phase"] != string(workers.PhaseFailed) {
		return false, false
	}
	return true, activity["providerSessionObserved"] == true
}

// subagentTimeoutProviderErrorEvidence refines the timeout message and action
// when the progress snapshot captured a terminal provider error. The default
// retry guidance assumes a slow model; an observed provider error does not
// support that remedy, so the action points at provider logs instead without
// claiming a cause. The action states the observed provider session reference
// only in the form actually observed, and never asserts its absence when one
// was seen.
func subagentTimeoutProviderErrorEvidence(envelope *ToolErrorEnvelope, progress map[string]any) {
	errorObserved, providerSessionObserved := subagentProviderErrorObservation(progress)
	if !errorObserved {
		return
	}
	envelope.Message = "subagent timed out after a provider error was observed; workspace edits may have occurred"
	envelope.Details["suggestedAction"] = subagentProviderErrorSuggestedAction(providerSessionObserved)
}

func subagentProviderErrorSuggestedAction(providerSessionObserved bool) string {
	observed := "a provider session reference was observed"
	if !providerSessionObserved {
		observed = "no provider session reference was observed"
	}
	return "The last observed provider activity was an error before the deadline and " + observed +
		"; a longer timeout may not resolve an observed provider error."
}

// subagentPrimaryText joins the text parts of a completed invocation with
// newlines, ignoring non-text content parts.
func subagentPrimaryText(parts []work.WorkContentPart) string {
	text := ""
	for _, part := range parts {
		if part.Type == work.WorkContentPartTypeText {
			if text != "" {
				text += "\n"
			}
			text += part.Text
		}
	}
	return text
}

func subagentInvocationArgs(input SubagentInput) map[string]any {
	args := map[string]any{"input": input.Prompt}
	if input.Provider != "" {
		args["workerProvider"] = input.Provider
	} else if strings.HasPrefix(input.Model, "opencode/") {
		args["workerProvider"] = "opencode"
	}
	if input.Model != "" {
		args["workerModel"] = input.Model
	}
	if input.ReasoningEffort != "" {
		args["workerReasoningEffort"] = input.ReasoningEffort
	}
	return args
}

// resolveSubagentProvider canonicalizes one explicitly requested provider
// selection through the resolver bound by production composition from the
// authoritative Providers catalog. The catalog stays the only accepted source
// of provider ids and aliases, so dynamically registered providers are
// accepted without this adapter holding a whitelist. An omitted selection keeps
// operator and provider defaults. Explicit selections require the bound catalog.
func resolveSubagentProvider(ctx context.Context, resolveProvider ProviderIdentityResolver, provider, model string) (string, error) {
	selected := strings.TrimSpace(provider)
	if selected == "" {
		return selected, nil
	}
	if resolveProvider == nil {
		return "", errors.New("subagent provider catalog resolver is unavailable")
	}
	canonical, err := resolveProvider(ctx, selected)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(model, "opencode/") && !strings.EqualFold(canonical, "opencode") {
		return "", errSubagentModelProviderConflict
	}
	return canonical, nil
}

// subagentProviderSelectionFailure reports one explicit provider selection the
// Providers catalog cannot resolve. Both envelopes are pre-dispatch: they carry
// no session identity, no partial-execution claim, and no private provider or
// resolver text because no Factory Session started and no dispatch reached
// Workers. The unknown-provider envelope is not retryable and stays actionable
// about the selected provider. Any other catalog failure is an internal
// configuration or availability problem rather than a rejected request, so it
// returns its own static envelope instead of the shared execution mapping,
// which copies arbitrary resolver text into the message and reason.
func subagentProviderSelectionFailure(err error, provider string) ToolResponse[SubagentResult] {
	if errors.Is(err, errSubagentModelProviderConflict) {
		envelope := ToolErrorEnvelope{Code: errorCodeBadRequest, Message: errSubagentModelProviderConflict.Error(), Retryable: false}
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	if envelope, ok := contextRequestErrorEnvelope(err); ok {
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	if !errors.Is(err, providers.ErrUnknownProvider) {
		return subagentProviderCatalogUnavailable(provider)
	}
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.provider_not_found",
		Message:   "selected subagent provider is not a known model provider; no Factory Session was started",
		Retryable: false,
		Details: map[string]any{
			"provider":        provider,
			"reason":          "UNKNOWN_PROVIDER",
			"suggestedAction": "Use a model provider id or alias from the configured Providers catalog, or omit provider to use operator defaults.",
		},
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}

// subagentProviderCatalogUnavailable reports one catalog resolution failure that
// is not a rejected selection, such as an unreadable or unbound catalog. The
// resolver error can wrap credentials, absolute paths, or catalog-authored
// text, so this envelope is fixed vocabulary only: it never embeds the error, a
// session identity, or a partial-execution claim. The failure is an internal
// configuration problem rather than a transient provider error, so retrying the
// same selection without an operator fix cannot help.
func subagentProviderCatalogUnavailable(provider string) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.provider_catalog_unavailable",
		Message:   "provider catalog resolution unavailable",
		Retryable: false,
		Details: map[string]any{
			"provider":        provider,
			"reason":          "PROVIDER_CATALOG_UNAVAILABLE",
			"suggestedAction": "Inspect the configured Providers catalog and this MCP server's provider catalog binding, then retry with a configured provider id or alias, or omit provider to use operator defaults.",
		},
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func validateSubagentRequest(ctx context.Context, target factorysessionexecution.Service, generateID factorysessionexecution.SessionIDGenerator, input SubagentInput) error {
	switch {
	case ctx == nil:
		return errMissingRequestContext
	case ctx.Err() != nil:
		return ctx.Err()
	case target == nil:
		return errors.New("Factory Session target execution service is unavailable")
	case generateID == nil:
		return errors.New("subagent request ID generator is unavailable")
	case strings.TrimSpace(input.Prompt) == "":
		return fmt.Errorf("prompt is required")
	case input.TimeoutMillis != nil && *input.TimeoutMillis <= 0:
		return fmt.Errorf("timeoutMillis must be greater than zero")
	case input.TimeoutMillis != nil && *input.TimeoutMillis > subagentMaxTimeoutMillis:
		return fmt.Errorf("timeoutMillis exceeds the supported range")
	default:
		return nil
	}
}

func subagentExecutionFailure(err error) ToolResponse[SubagentResult] {
	envelope := executionErrorEnvelope(err)
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentCapacityFailure(sessionID, requestID string) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.capacity_exhausted",
		Message:   "subagent execution capacity is temporarily exhausted",
		Retryable: true,
		SessionID: sessionID,
		Details: map[string]any{
			"requestId":       requestID,
			"suggestedAction": "Wait for active calls to finish and retry. If capacity remains exhausted, inspect or restart the MCP server.",
		},
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentCleanupError(err error, sessionID, requestID string) ToolResponse[SubagentResult] {
	if errors.Is(err, context.DeadlineExceeded) {
		return subagentCleanupTimeout(sessionID, requestID)
	}
	return subagentCleanupFailure(sessionID, requestID)
}

func subagentCleanupTimeout(sessionID, requestID string) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.cleanup_timed_out",
		Message:   "subagent session cleanup exceeded its deadline after execution; workspace edits may have occurred",
		Retryable: false,
		SessionID: sessionID,
		Details: map[string]any{
			"partialEffectsPossible": true,
			"requestId":              requestID,
			"suggestedAction":        "Inspect the workspace and Factory Session before retrying.",
		},
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentCleanupFailure(sessionID, requestID string) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.cleanup_failed",
		Message:   "subagent session cleanup failed after execution; workspace edits may have occurred",
		SessionID: sessionID,
		Details:   map[string]any{"partialEffectsPossible": true, "requestId": requestID},
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentInvocationFailure(sessionID, requestID string) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.invocation_failed",
		Message:   "subagent invocation failed before producing a result",
		SessionID: sessionID,
		Details:   map[string]any{"partialEffectsPossible": true, "requestId": requestID},
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentInvokeError(err error, sessionID, requestID string, timeoutMillis int64, input SubagentInput, progress map[string]any) ToolResponse[SubagentResult] {
	if errors.Is(err, errSubagentCapacity) {
		return subagentCapacityFailure(sessionID, requestID)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return subagentInvocationTimeout(sessionID, requestID, timeoutMillis, input, progress)
	}
	return subagentInvocationFailure(sessionID, requestID)
}

func subagentInvocationTimeout(sessionID, requestID string, timeoutMillis int64, input SubagentInput, progress map[string]any) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.timed_out",
		Message:   "subagent timed out before producing a result; workspace edits may have occurred",
		Retryable: false,
		SessionID: sessionID,
		Details: map[string]any{
			"partialEffectsPossible": true,
			"requestId":              requestID,
			"suggestedAction":        "If you retry, use another configured model or a longer timeout.",
		},
	}
	if input.Provider != "" {
		envelope.Details["provider"] = input.Provider
	}
	if input.Model != "" {
		envelope.Details["model"] = input.Model
	}
	if timeoutMillis > 0 {
		envelope.Details["timeoutMillis"] = timeoutMillis
	}
	subagentAttachProgress(envelope.Details, progress)
	subagentTimeoutProviderErrorEvidence(&envelope, progress)
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentTerminalFailure(sessionID string, result factorysessionexecution.InvocationResult, timeoutMillis int64, input SubagentInput, progress map[string]any) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.execution_failed",
		Message:   "subagent execution failed before producing a result",
		SessionID: sessionID,
		Details:   map[string]any{"status": string(result.Status)},
	}
	if result.ErrorCode != "" {
		envelope.Details["invocationCode"] = result.ErrorCode
	}
	// Cleanup closes the live Factory Session for every terminal result, so the
	// runtime correlation identities it published are recorded on the envelope
	// before the outcome branch decides timeout evidence or failure
	// classification. They are fixed runtime vocabulary, so they carry no
	// provider, prompt, or workspace content, and an unpublished identity is
	// omitted rather than reported as empty.
	if result.RequestID != "" {
		envelope.Details["requestId"] = result.RequestID
	}
	if result.TraceID != "" {
		envelope.Details["traceId"] = result.TraceID
	}
	if result.WorkID != "" {
		envelope.Details["workId"] = result.WorkID
	}
	if result.Status == factorysessionexecution.InvocationTerminalStatusTimedOut {
		envelope.Code = "factory_session.subagent.timed_out"
		envelope.Message = "subagent timed out before producing a result; workspace edits may have occurred"
		envelope.Details["partialEffectsPossible"] = true
		envelope.Details["suggestedAction"] = "If you retry, use another configured model or a longer timeout."
		if input.Provider != "" {
			envelope.Details["provider"] = input.Provider
		}
		if input.Model != "" {
			envelope.Details["model"] = input.Model
		}
		if timeoutMillis > 0 {
			envelope.Details["timeoutMillis"] = timeoutMillis
		}
		subagentAttachProgress(envelope.Details, progress)
		subagentTimeoutProviderErrorEvidence(&envelope, progress)
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	subagentClassifyFailure(&envelope, workers.WorkFailureType(result.FailureReason), input.Provider)
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentClassifyFailure(envelope *ToolErrorEnvelope, reason workers.WorkFailureType, provider string) {
	switch reason {
	case workers.WorkFailureTypeThrottled:
		// Keep recovery guidance fixed; cleanup adds partial-edit inspection
		// and log correlation without exposing provider response text.
		envelope.Code = "factory_session.subagent.provider_throttled"
		envelope.Message = "provider is temporarily unavailable due to usage or capacity limits"
		envelope.Retryable = true
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeThrottled)
		envelope.Details["suggestedAction"] = "Wait for the provider usage or capacity limit to clear, then retry the same request. If the limit persists, select another available configured model or provider."
	case workers.WorkFailureTypeAuthFailure:
		envelope.Code = "factory_session.subagent.provider_auth_failure"
		envelope.Message = "provider authentication failed"
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeAuthFailure)
	case workers.WorkFailureTypePermanentBadRequest:
		envelope.Code = "factory_session.subagent.provider_request_rejected"
		envelope.Message = "provider rejected the subagent request"
		envelope.Retryable = false
		envelope.Details["failureReason"] = string(workers.WorkFailureTypePermanentBadRequest)
		envelope.Details["suggestedAction"] = "Verify the selected model against the provider's advertised models and request settings before retrying."
	case workers.WorkFailureTypeTimeout:
		envelope.Code = "factory_session.subagent.provider_timeout"
		envelope.Message = "provider request timed out"
		envelope.Retryable = true
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeTimeout)
	case workers.WorkFailureTypeMisconfigured:
		envelope.Code = "factory_session.subagent.provider_misconfigured"
		envelope.Message = "subagent provider is misconfigured; check provider setup and capabilities"
		envelope.Retryable = false
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeMisconfigured)
		if provider == "pi" {
			envelope.Message = "Pi subagent provider is misconfigured; check that Pi's selected model endpoint is running and reachable"
			envelope.Details["suggestedAction"] = "Check that Pi's selected model endpoint is running and reachable. Check Pi setup and capabilities. Run `pi --version`; pi-acp requires Pi 0.81.0+. Upgrade via `npm install -g @earendil-works/pi-coding-agent@latest` if needed."
		} else {
			envelope.Details["suggestedAction"] = "Verify the provider configuration and ensure the required executable or model is available"
		}
	case workers.WorkFailureTypeMissingExecutable:
		envelope.Code = "factory_session.subagent.provider_executable_missing"
		envelope.Message = "required provider executable is unavailable"
		envelope.Retryable = false
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeMissingExecutable)
		envelope.Details["suggestedAction"] = "Install the provider command on PATH or update its configured executable path, then retry."
	case workers.WorkFailureTypeInternalServerError:
		envelope.Code = "factory_session.subagent.provider_internal_error"
		envelope.Message = "provider encountered an internal error"
		envelope.Retryable = true
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeInternalServerError)
		envelope.Details["suggestedAction"] = "Check provider status and logs, then retry when the provider is available."
	case workers.WorkFailureTypeUnknown:
		envelope.Code = "factory_session.subagent.provider_unknown_failure"
		envelope.Message = "provider failed for an unknown reason"
		envelope.Retryable = false
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeUnknown)
		envelope.Details["suggestedAction"] = "Check provider logs and configuration before retrying."
	case "":
		envelope.Details["suggestedAction"] = "Check provider logs and configuration before retrying."
	}
}

// subagentLastProviderActivity reads the retained response events already owned
// by the live Factory Session. Only fixed vocabulary and the runtime-owned
// event timestamp leave this edge.
func subagentLastProviderActivity(ctx context.Context, target factorysessionexecution.Service, sessionID string) map[string]any {
	events, err := boundedSubagentCall(ctx, subagentSnapshotSlots, func() ([]factorysessionexecution.FactoryResponseEvent, error) {
		cursor, err := target.SubscribeFactoryResponseEvents(ctx, factorysessionexecution.ResponseEventSubscriptionRequest{SessionID: sessionID})
		if err != nil {
			return nil, err
		}
		if cursor == nil {
			return nil, errors.New("response event cursor unavailable")
		}
		defer cursor.Detach()
		return cursor.Drain()
	})
	if err != nil || ctx.Err() != nil {
		return nil
	}
	activity := make(map[string]any)
	for _, event := range events {
		if event.ProviderSessionRef != "" {
			activity["providerSessionObserved"] = true
		}
		if event.Provenance.Provider == "" || event.Kind.Validate() != nil || event.Phase.Validate() != nil || event.RecordedAt.IsZero() {
			continue
		}
		activity["kind"] = string(event.Kind)
		activity["phase"] = string(event.Phase)
		activity["observedAt"] = event.RecordedAt
	}
	if len(activity) == 0 {
		return nil
	}
	return activity
}
