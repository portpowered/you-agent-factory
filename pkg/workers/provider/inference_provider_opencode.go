package provider

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	provideradapter "github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/workers/provider/adapter/opencode"
)

func (p *ScriptWrapProvider) executeNegotiatedOpenCode(
	ctx context.Context,
	req interfaces.ProviderInferenceRequest,
	logger logging.Logger,
) (interfaces.InferenceResponse, error) {
	if p.openCodeResolverErr != nil {
		return interfaces.InferenceResponse{}, p.openCodeResolutionError(req, p.openCodeResolverErr)
	}
	if p.openCodeResolver == nil {
		return interfaces.InferenceResponse{}, p.openCodeResolutionError(req, errors.New("OpenCode capability resolver is unavailable"))
	}
	decision, err := p.openCodeResolver.Resolve(ctx, string(interfaces.ModelProviderOpenCode))
	if err != nil {
		return interfaces.InferenceResponse{}, p.openCodeResolutionError(req, err)
	}
	p.publishOpenCodeCapability(req.Dispatch.DispatchID, decision.Capabilities(), nil)
	if err := validateOpenCodeNegotiatedCapabilities(req, decision); err != nil {
		return interfaces.InferenceResponse{}, p.openCodeRequestValidationError(req, err)
	}
	negotiated, err := opencodeadapter.NewNegotiatedAdapterForRequest(decision, p.openCodeResolver, req)
	if err != nil {
		return interfaces.InferenceResponse{}, p.openCodeResolutionError(req, err)
	}
	registry, err := provideradapter.NewRegistry(negotiated)
	if err != nil {
		return interfaces.InferenceResponse{}, p.openCodeResolutionError(req, err)
	}
	started := time.Now()
	result, executeErr := provideradapter.Execute(ctx, registry, p.openCodeAdapterRunner(), provideradapter.ExecuteInput{
		Provider: negotiated.Identity(),
		Command:  provideradapter.CommandContext{Request: req, SkipPermissions: p.SkipPermissions},
		Decoder:  provideradapter.DecoderContext{RunID: req.Dispatch.DispatchID, DispatchID: req.Dispatch.DispatchID},
		Publish:  p.publishOpenCodeDecoded(req.Dispatch.DispatchID),
	})
	duration := time.Since(started)
	diagnostics := commandDiagnostics(result.Request, result.Command, duration, result.Outcome == provideradapter.CommandOutcomeCanceled)
	for index := range result.CapabilityUpdates {
		update := result.CapabilityUpdates[index]
		p.publishOpenCodeCapability(req.Dispatch.DispatchID, update.Capabilities, &update.Diagnostic)
	}
	if result.Failure != nil {
		providerErr := providerErrorFromAdapterFailure(result.Failure, executeErr, diagnostics)
		logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result.Command, duration)...)
		p.publishOpenCodeFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
		return interfaces.InferenceResponse{}, providerErr
	}
	if executeErr != nil {
		providerErr := normalizeProviderExecutionError(req.ModelProvider, result.Command, executeErr, result.Response.ProviderSession, diagnostics)
		p.publishOpenCodeFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
		return interfaces.InferenceResponse{}, providerErr
	}

	response := result.Response
	response.Diagnostics = diagnostics
	logger.Info("inferencer: request completed",
		appendProviderSessionLogFields(providerLogFields(req, "output_len", len(response.Content)), response.ProviderSession)...)
	p.publishOpenCodeCompleted(req.Dispatch.DispatchID, response.ProviderSession, len(result.Drafts) > 0)
	return response, nil
}

func validateOpenCodeNegotiatedCapabilities(req interfaces.ProviderInferenceRequest, decision opencodeadapter.Decision) error {
	if decision.Mode == opencodeadapter.ModeStructured {
		return nil
	}
	for _, capability := range req.RequiredOptionalCapabilities {
		if capability == interfaces.RunnerOptionalCapabilityStructuredOutput {
			return errors.New("structured output is required but is not supported by the installed opencode runner")
		}
	}
	return nil
}

func (p *ScriptWrapProvider) openCodeRequestValidationError(req interfaces.ProviderInferenceRequest, err error) *ProviderError {
	providerErr := newProviderErrorWithDiagnostics(
		interfaces.WorkFailureTypePermanentBadRequest,
		err.Error(),
		err,
		nil,
		workDiagnosticsForInferenceRequest(req),
	)
	p.publishFailureFragment(req.Dispatch.DispatchID, nil, providerErr)
	return providerErr
}

func (p *ScriptWrapProvider) openCodeResolutionError(req interfaces.ProviderInferenceRequest, err error) *ProviderError {
	providerErr := normalizeProviderExecutionError(req.ModelProvider, CommandResult{}, err, nil, workDiagnosticsForInferenceRequest(req))
	p.publishFailureFragment(req.Dispatch.DispatchID, nil, providerErr)
	return providerErr
}

func providerErrorFromAdapterFailure(
	failure *provideradapter.FailureFacts,
	cause error,
	diagnostics *interfaces.WorkDiagnostics,
) *ProviderError {
	return &ProviderError{
		Family: failure.Family, Type: failure.Type, Message: failure.Message,
		ProviderSession: interfaces.CloneProviderSessionMetadata(failure.ProviderSession),
		Diagnostics:     interfaces.CloneWorkDiagnostics(diagnostics), Cause: cause,
	}
}

type bufferedAdapterRunner struct{ runner CommandRunner }

func (r bufferedAdapterRunner) Run(
	ctx context.Context,
	req workerprocess.CommandRequest,
	observe func(provideradapter.Observation) error,
) (workerprocess.CommandResult, error) {
	result, err := r.runner.Run(ctx, req)
	if len(result.Stdout) > 0 {
		if observeErr := observe(provideradapter.Observation{Stream: provideradapter.OutputStreamStdout, Chunk: result.Stdout}); observeErr != nil {
			return result, observeErr
		}
	}
	if len(result.Stderr) > 0 {
		if observeErr := observe(provideradapter.Observation{Stream: provideradapter.OutputStreamStderr, Chunk: result.Stderr}); observeErr != nil {
			return result, observeErr
		}
	}
	return result, err
}

type execAdapterRunner struct{ logger logging.Logger }

func (r execAdapterRunner) Run(
	ctx context.Context,
	req workerprocess.CommandRequest,
	observe func(provideradapter.Observation) error,
) (workerprocess.CommandResult, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var observeMu sync.Mutex
	var observeErr error
	result, err := (workerprocess.StreamingExecCommandRunner{
		Logger: r.logger,
		Observer: func(stream string, chunk []byte) {
			observeMu.Lock()
			defer observeMu.Unlock()
			if observeErr != nil {
				return
			}
			observeErr = observe(provideradapter.Observation{Stream: provideradapter.OutputStream(stream), Chunk: chunk})
			if observeErr != nil {
				cancel()
			}
		},
	}).Run(runCtx, req)
	return result, errors.Join(err, observeErr)
}

func (p *ScriptWrapProvider) openCodeAdapterRunner() provideradapter.StreamingCommandRunner {
	switch runner := p.exec.(type) {
	case workerprocess.ExecCommandRunner, *workerprocess.ExecCommandRunner:
		return execAdapterRunner{logger: logging.EnsureLogger(p.Logger)}
	case InferenceProgressPublishingCommandRunner:
		return execAdapterRunner{logger: logging.EnsureLogger(runner.Logger)}
	case *InferenceProgressPublishingCommandRunner:
		if runner != nil {
			return execAdapterRunner{logger: logging.EnsureLogger(runner.Logger)}
		}
	default:
	}
	return bufferedAdapterRunner{runner: p.commandExec()}
}

func (p *ScriptWrapProvider) publishOpenCodeDecoded(dispatchID string) func(provideradapter.DecodeResult) {
	return func(decoded provideradapter.DecodeResult) {
		if p == nil || p.progressPublisher == nil {
			return
		}
		for _, draft := range decoded.Drafts {
			p.progressPublisher(CanonicalDraftFragment(draft.DispatchID, draft))
		}
		for _, diagnostic := range decoded.Diagnostics {
			p.progressPublisher(InferenceProgressFragment{
				DispatchID: dispatchID, Kind: ProgressFragmentKind, Type: NormalizedEventTypeProgress,
				Payload: diagnostic.Message, ExternalEventType: diagnostic.Code,
				Metadata: map[string]string{"runner_id": string(interfaces.ModelProviderOpenCode), "diagnostic_code": diagnostic.Code},
			})
		}
	}
}

func (p *ScriptWrapProvider) publishOpenCodeCapability(
	dispatchID string,
	capabilities provideradapter.Capabilities,
	diagnostic *provideradapter.Diagnostic,
) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	mode, fidelity, message, eventType := "structured", "normalized", "OpenCode selected structured output mode.", "provider_capability_selected"
	if capabilities.FinalOnly {
		mode, fidelity, message = "final_only", "final_only", "OpenCode selected final-only output mode."
	}
	metadata := map[string]string{
		"runner_id": string(interfaces.ModelProviderOpenCode), "selected_mode": mode, "fidelity": fidelity,
	}
	if diagnostic != nil {
		message, eventType = diagnostic.Message, diagnostic.Code
		metadata["downgrade_reason"] = "unsupported_format"
	}
	p.progressPublisher(InferenceProgressFragment{
		DispatchID: dispatchID, Kind: ProgressFragmentKind, Type: NormalizedEventTypeProgress,
		Payload: message, ExternalEventType: eventType, Metadata: metadata,
	})
}

func (p *ScriptWrapProvider) publishOpenCodeCompleted(
	dispatchID string,
	providerSession *interfaces.ProviderSessionMetadata,
	skipCanonical bool,
) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	fragment := CompletedFragment(dispatchID, providerSession)
	fragment.CanonicalEventAlreadyPublished = skipCanonical
	p.progressPublisher(fragment)
}

func (p *ScriptWrapProvider) publishOpenCodeFailure(dispatchID string, err error, skipCanonical bool) {
	if p == nil || p.progressPublisher == nil {
		return
	}
	message := ""
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		message = providerErr.Message
	}
	var providerSession *interfaces.ProviderSessionMetadata
	if providerErr != nil {
		providerSession = providerErr.ProviderSession
	}
	fragment := FailedFragment(dispatchID, providerSession, message)
	fragment.CanonicalEventAlreadyPublished = skipCanonical
	p.progressPublisher(fragment)
}
