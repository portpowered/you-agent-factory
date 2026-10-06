package factorysessionexecution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// missingChildExecutor is returned when a live child composition was opened
// without its required Workers Execute capability. Keeping this as an
// explicit executor makes the wiring failure immediate and attributable to
// the child session instead of constructing a nil-capability executor that
// can leave a durable session waiting forever.
type missingChildExecutor struct {
	sessionID string
}

func (e missingChildExecutor) Execute(
	ctx context.Context,
	_ factory.JavaScriptChildExecutionRequest,
) (factory.JavaScriptChildExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return factory.JavaScriptChildExecutionResult{}, err
	}
	return factory.JavaScriptChildExecutionResult{}, fmt.Errorf(
		"child session %q cannot execute: Workers Execute capability is required",
		strings.TrimSpace(e.sessionID),
	)
}

func (s *JavaScriptRuntimeService) childExecutorHooksForStart(mode, sessionID string, mockWorkers *workers.MockWorkersConfig, attemptStarter factorysessions.WorkerAttemptStarter, resourceAdmission factory.ResourceCapacityLeaseAdmission, progressPublisher workers.ProgressPublisher) factory.JavaScriptRuntimeHooks {
	hooks := factory.JavaScriptRuntimeHooks{
		OnRecord: func(record factory.JavaScriptRuntimeRecord) {
			s.applyRunningRuntimeRecord(sessionID, record)
		},
	}
	if mode != ChildExecutorModeLive {
		return hooks
	}
	// Capture this opening's handles before any later opening or replacement.
	binding := s.workerExecutionBinding(sessionID)
	hooks.NewChildExecutor = func(childSessionID string, records factory.JavaScriptChildRecordSink, policy factory.JavaScriptPolicy) factory.JavaScriptChildExecutor {
		workingDir := s.projectRootForSession(sessionID)
		// Runtime-backed and standalone children use the fixed Workers Execute
		// operation. Request handles retain the owning session's policy and routes.
		if binding != nil {
			executor := newChildWorkerExecutor(
				childSessionID,
				binding.execute,
				records,
				s.childValues,
				s.observeWorkerDispatch,
				workingDir,
				policy.MaxRetries,
			)
			executor.maxWorkerDuration = childWorkerDurationFromPolicy(policy)
			executor.resourceLeaseAcquirer = binding.resourceLeaseAcquirer
			if resourceAdmission != nil {
				executor.resourceLeaseAcquirer = func(ctx context.Context, request factory.ResourceCapacityLeaseRequest) (*childResourceLease, error) {
					lease, err := resourceAdmission.AcquireResourceCapacityLease(ctx, request)
					if err != nil || lease == nil {
						return nil, err
					}
					return &childResourceLease{factoryRevision: lease.FactoryRevision, release: lease.Release}, nil
				}
			}
			executor.runtimeID = binding.runtimeID
			executor.generationID = binding.generationID
			executor.providerOverride = binding.providerOverride
			executor.mockWorkers = binding.mockWorkers.Clone()
			if mockWorkers != nil {
				executor.mockWorkers = mockWorkers.Clone()
			}
			executor.commandRunnerOverride = binding.commandRunnerOverride
			executor.attemptStarter = binding.attemptStarter
			if attemptStarter != nil {
				executor.attemptStarter = childWorkerAttemptStarter(attemptStarter)
			}
			executor.publish = binding.publish
			if progressPublisher != nil {
				// Publish directly to this child's owner. The selected runtime
				// bridge may belong to another durable owner, so it cannot route
				// this dispatch on our behalf. Do not register the dispatch here:
				// a bridge back to this service would otherwise publish it twice.
				executor.observe = nil
				executor.publish = s.childProgressPublisher(childSessionID, progressPublisher)
			}
			return executor
		}
		return missingChildExecutor{sessionID: childSessionID}
	}
	return hooks
}

func (s *JavaScriptRuntimeService) childProgressPublisher(sessionID string, selected workers.ProgressPublisher) childWorkerProgressPublisher {
	return func(workerDispatchID string, fragment workers.ProgressFragment) {
		if strings.TrimSpace(fragment.DispatchID) == "" {
			fragment.DispatchID = workerDispatchID
		}
		if strings.TrimSpace(fragment.Correlation.DispatchID) == "" {
			fragment.Correlation.DispatchID = workerDispatchID
		}
		if state := s.liveSessionState(sessionID); state != nil {
			s.sessionProgressPublisher(sessionID, state)(fragment)
		}
		selected(fragment)
	}
}

// childWorkerProgressBridge keeps provider-owned terminal fragments from
// racing the canonical Execute result. Provider runners may publish a
// STREAM_COMPLETED/STREAM_FAILED fragment before Execute returns, while a
// plain provider, model, harness, cancellation, or timeout may publish none.
// Buffering that one fragment lets the child publish exactly one final
// response outcome after the retry budget and Execute result are known.
type childWorkerProgressBridge struct {
	mu         sync.Mutex
	publish    childWorkerProgressPublisher
	dispatchID string
	pending    *workers.ProgressFragment
	authored   bool
	terminal   bool
}

func newChildWorkerProgressBridge(
	publish childWorkerProgressPublisher,
	dispatchID string,
) *childWorkerProgressBridge {
	return &childWorkerProgressBridge{
		publish:    publish,
		dispatchID: dispatchID,
	}
}

func (b *childWorkerProgressBridge) publishProgress(fragment workers.ProgressFragment) {
	if b == nil || b.publish == nil {
		return
	}
	fragment = b.normalize(fragment)
	b.mu.Lock()
	if b.terminal {
		b.mu.Unlock()
		return
	}
	if isChildTerminalProgress(fragment) {
		b.pending = &fragment
		b.mu.Unlock()
		return
	}
	if fragment.Kind == workers.ResponseFragmentKind || fragment.CanonicalDraft != nil {
		b.authored = true
	}
	publish := b.publish
	dispatchID := b.dispatchID
	b.mu.Unlock()
	publish(dispatchID, fragment)
}

// publishResultContent preserves final-only Worker output when a provider
// supplied no streaming response fragments during this attempt.
func (b *childWorkerProgressBridge) publishResultContent(result workers.ExecuteResult) {
	if b == nil || b.publish == nil || !childExecutionSucceeded(result.Outcome) {
		return
	}
	b.mu.Lock()
	if b.terminal || b.authored {
		b.mu.Unlock()
		return
	}
	b.authored = true
	b.mu.Unlock()
	for _, part := range result.Output.Primary {
		if part.Type != work.WorkContentPartTypeText || strings.TrimSpace(part.Text) == "" {
			continue
		}
		b.publish(b.dispatchID, workers.ProgressFragment{
			DispatchID: b.dispatchID, Correlation: result.Correlation,
			Kind: workers.ResponseFragmentKind, Type: "message.delta", Payload: part.Text,
		})
	}
}

func (b *childWorkerProgressBridge) resetAttempt() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.terminal {
		b.mu.Unlock()
		return
	}
	b.pending = nil
	b.mu.Unlock()
}

func (b *childWorkerProgressBridge) publishTerminal(
	result workers.ExecuteResult,
	executeErr error,
) {
	if b == nil || b.publish == nil {
		return
	}
	b.mu.Lock()
	if b.terminal {
		b.mu.Unlock()
		return
	}
	b.terminal = true
	pending := b.pending
	b.pending = nil
	b.mu.Unlock()
	if pending != nil && childTerminalProgressMatches(*pending, result, executeErr) {
		b.publish(b.dispatchID, *pending)
		return
	}
	b.publish(b.dispatchID, childTerminalProgressFromResult(b.dispatchID, result, executeErr))
}

func (b *childWorkerProgressBridge) normalize(fragment workers.ProgressFragment) workers.ProgressFragment {
	if strings.TrimSpace(fragment.DispatchID) == "" {
		fragment.DispatchID = b.dispatchID
	}
	if strings.TrimSpace(fragment.Correlation.DispatchID) == "" {
		fragment.Correlation.DispatchID = b.dispatchID
	}
	return fragment
}

func isChildTerminalProgress(fragment workers.ProgressFragment) bool {
	return fragment.Kind == workers.CompletedFragmentKind || fragment.Kind == workers.FailedFragmentKind
}

func childTerminalProgressMatches(
	fragment workers.ProgressFragment,
	result workers.ExecuteResult,
	executeErr error,
) bool {
	switch fragment.Kind {
	case workers.CompletedFragmentKind:
		return executeErr == nil && childExecutionSucceeded(result.Outcome)
	case workers.FailedFragmentKind:
		// A provider's failed fragment can be less specific than the
		// canonical Execute result (for example STREAM_FAILED versus a typed
		// timeout or cancellation). Always synthesize the canonical terminal
		// for unhappy outcomes so the durable response keeps that classification.
		return false
	default:
		return false
	}
}

func childTerminalProgressFromResult(
	dispatchID string,
	result workers.ExecuteResult,
	executeErr error,
) workers.ProgressFragment {
	fragment := workers.ProgressFragment{
		DispatchID:   dispatchID,
		Correlation:  result.Correlation,
		Provider:     childProviderName(result),
		Continuation: (result.Continuation).ClonePtr(),
	}
	if strings.TrimSpace(fragment.Correlation.DispatchID) == "" {
		fragment.Correlation.DispatchID = dispatchID
	}
	fragment.Kind = workers.CompletedFragmentKind
	fragment.Type = "COMPLETED"
	fragment.ExternalEventType = "STREAM_COMPLETED"
	if !childExecutionSucceeded(result.Outcome) || executeErr != nil {
		fragment.Kind = workers.FailedFragmentKind
		fragment.Type = "FAILED"
		fragment.ExternalEventType = "STREAM_FAILED"
		if result.Outcome == workers.ExecutionOutcomeCanceled || errors.Is(executeErr, context.Canceled) {
			fragment.Type = "CANCELED"
		}
		fragment.Payload = childExecutionDiagnostic(result, executeErr)
		if result.Failure != nil {
			fragment.Metadata = map[string]string{
				"work_failure_type": string(result.Failure.Type),
				"retryable":         fmt.Sprintf("%t", result.Failure.RetryHint),
			}
		}
	}
	return fragment
}

func childProviderName(result workers.ExecuteResult) string {
	provider, _ := childProviderSession(result)
	if provider != "" {
		return provider
	}
	if result.Diagnostics != nil && result.Diagnostics.Provider != nil {
		return canonicalChildProvider(result.Diagnostics.Provider.Provider)
	}
	return ""
}

func childExecutionDiagnostic(result workers.ExecuteResult, executeErr error) string {
	return childFailureDiagnostic(result, executeErr, factory.JavaScriptChildExecutionRequest{})
}

// normalizeChildExecuteFailure keeps a typed Worker/Provider error typed when a
// child executor returns it as the operation error rather than embedding the
// failure on ExecuteResult. Only an error with no typed Worker detail takes the
// terminal/unknown fallback path.
func normalizeChildExecuteFailure(
	result workers.ExecuteResult,
	executeErr error,
) workers.ExecuteResult {
	if result.Failure != nil || executeErr == nil {
		return result
	}
	if providerErr := workers.NormalizeProviderExecutionError(executeErr); providerErr != nil {
		metadata := workers.WorkFailureMetadataFromProviderError(providerErr)
		decision := workers.FailureDecisionFromMetadata(metadata)
		failureType := workers.WorkFailureTypeUnknown
		family := workers.WorkFailureFamilyTerminal
		if metadata != nil {
			failureType = metadata.Type
			family = metadata.Family
		}
		message := strings.TrimSpace(providerErr.Message)
		if providerErr.ProviderFailureKind == "" {
			message = ""
		}
		if message == "" {
			message = childSafeFailureMessage(failureType)
		}
		result.Failure = &workers.ExecutionFailure{
			Type:                            failureType,
			Family:                          family,
			Message:                         message,
			RetryHint:                       decision.Retryable,
			ProviderFailureKind:             providerErr.ProviderFailureKind,
			ProviderContinuationFailureKind: providerErr.ProviderContinuationFailureKind,
			ProviderContinuationOutcome:     providerErr.ProviderContinuationOutcome,
			Detail:                          &workers.FailureDetail{Reason: failureType, Message: message},
		}
		if result.Diagnostics == nil {
			result.Diagnostics = providerErr.Diagnostics.ToSafeDiagnostics()
		}
		if result.Continuation == nil {
			result.Continuation = (providerErr.Continuation).ClonePtr()
		}
		return result
	}

	message := strings.TrimSpace(executeErr.Error())
	if message == "" {
		message = "Provider execution failed."
	}
	result.Failure = &workers.ExecutionFailure{
		Type:    workers.WorkFailureTypeUnknown,
		Family:  workers.WorkFailureFamilyTerminal,
		Message: message,
		Detail:  &workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: message},
	}
	return result
}

func childFailureDiagnostic(
	result workers.ExecuteResult,
	executeErr error,
	req factory.JavaScriptChildExecutionRequest,
) string {
	message := childFailureMessage(result, executeErr)
	provider := childProviderName(result)
	if provider == "" {
		provider = canonicalChildProvider(req.ModelProvider)
	}
	if provider == "" && req.ExecutorProvider != "" &&
		!strings.EqualFold(strings.TrimSpace(req.ExecutorProvider), workers.ExecutorProviderACP) &&
		!strings.EqualFold(strings.TrimSpace(req.ExecutorProvider), "SCRIPT_WRAP") {
		provider = canonicalChildProvider(req.ExecutorProvider)
	}
	return formatChildProviderFailure(provider, message)
}

func childFailureMessage(result workers.ExecuteResult, executeErr error) string {
	message := ""
	if result.Failure != nil && result.Failure.Detail != nil {
		message = strings.TrimSpace(result.Failure.Detail.Message)
	}
	if message == "" && result.Failure != nil {
		message = strings.TrimSpace(result.Failure.Message)
	}
	if message == "" && executeErr != nil {
		message = strings.TrimSpace(executeErr.Error())
	}
	if message == "" {
		if result.Outcome == workers.ExecutionOutcomeCanceled {
			message = "Provider execution canceled."
		} else {
			message = "Provider execution failed."
		}
	}
	return message
}

func childSafeFailureMessage(reason workers.WorkFailureType) string {
	switch reason {
	case workers.WorkFailureTypeAuthFailure:
		return "Provider authentication failed."
	case workers.WorkFailureTypePermanentBadRequest:
		return "Provider rejected the request as invalid."
	case workers.WorkFailureTypeThrottled:
		return "Provider is temporarily unavailable due to usage or capacity limits."
	case workers.WorkFailureTypeInternalServerError:
		return "Provider encountered a temporary server error."
	case workers.WorkFailureTypeTimeout:
		return "Provider request timed out."
	case workers.WorkFailureTypeMisconfigured:
		return "Provider command could not be started."
	default:
		return "Provider execution failed."
	}
}

func canonicalChildProvider(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return ""
	}
	canonical := providers.ID(strings.ToLower(provider)).CanonicalSessionProvider()
	if canonical == "" {
		return strings.ToLower(provider)
	}
	return canonical
}

func formatChildProviderFailure(provider, message string) string {
	provider = canonicalChildProvider(provider)
	message = strings.TrimSpace(message)
	if provider == "" || message == "" {
		return message
	}
	display := provider
	if runes := []rune(display); len(runes) > 0 {
		display = strings.ToUpper(string(runes[0])) + string(runes[1:])
	}
	if strings.HasPrefix(strings.ToLower(message), strings.ToLower(display)+":") {
		return message
	}
	return fmt.Sprintf("%s: %s", display, message)
}

func effectiveChildSkipPermissions(req factory.JavaScriptChildExecutionRequest) bool {
	if req.Permissions != "" {
		return req.Permissions == factory.JavaScriptChildPermissionSkipPermissions
	}
	return req.SkipPermissions
}

func childRunnerID(executorProvider, modelProvider string) (string, error) {
	executorProvider = strings.TrimSpace(executorProvider)
	modelProvider = strings.TrimSpace(modelProvider)
	if strings.EqualFold(executorProvider, workers.ExecutorProviderACP) {
		if modelProvider == "" {
			return "", fmt.Errorf("executorProvider ACP requires modelProvider to name an ACP integration")
		}
		return modelProvider, nil
	}
	if executorProvider != "" && !strings.EqualFold(executorProvider, "SCRIPT_WRAP") {
		return executorProvider, nil
	}
	if modelProvider != "" {
		return modelProvider, nil
	}
	// The legacy Factory Runtime selection resolved an otherwise unspecified
	// provider child to the Codex runner. Preserve that public default before
	// the detached Workers request is validated; an injected provider edge may
	// intentionally have no catalog identity of its own.
	return workers.RunnerIDCodex, nil
}

// failedChild preserves the Workers-owned failure classification on the record,
// which is what dispatch inspection reads to explain why a child failed.
