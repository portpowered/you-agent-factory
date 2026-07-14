package provider

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/interfaces/responseevents"
	"github.com/portpowered/infinite-you/pkg/logging"
	agyadapter "github.com/portpowered/infinite-you/pkg/workers/provider/agy"
	provideradapter "github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

func (p *ScriptWrapProvider) executeAgy(
	ctx context.Context,
	req interfaces.ProviderInferenceRequest,
	logger logging.Logger,
) (interfaces.InferenceResponse, error) {
	factoryRoot := strings.TrimSpace(p.agyFactoryRoot)
	if factoryRoot == "" {
		return interfaces.InferenceResponse{}, p.agyRequestValidationError(req, errors.New("Agy factory root is unavailable"))
	}
	opts := []agyadapter.Option{}
	if p.agyAllocator != nil {
		opts = append(opts, agyadapter.WithAllocator(p.agyAllocator))
	}
	providerAdapter := agyadapter.NewAdapter(factoryRoot, opts...)
	registry, err := provideradapter.NewRegistry(providerAdapter)
	if err != nil {
		return interfaces.InferenceResponse{}, p.agyRequestValidationError(req, err)
	}
	runner, err := providerAdapter.PTYRunner()
	if err != nil {
		return interfaces.InferenceResponse{}, p.agyRequestValidationError(req, err)
	}
	started := time.Now()
	result, executeErr := provideradapter.Execute(ctx, registry, runner, provideradapter.ExecuteInput{
		Provider: providerAdapter.Identity(),
		Command:  provideradapter.CommandContext{Request: req, SkipPermissions: p.SkipPermissions, MaterializeOptions: p.MaterializeOptions},
		Decoder:  provideradapter.DecoderContext{RunID: req.Dispatch.DispatchID, DispatchID: req.Dispatch.DispatchID},
		ObserveDraft: func(draft responseevents.Draft) {
			if p.progressPublisher != nil {
				p.progressPublisher(CanonicalDraftFragment(draft.DispatchID, draft))
			}
		},
	})
	duration := time.Since(started)
	diagnostics := commandDiagnostics(result.Request, result.Command, duration, result.Outcome == provideradapter.CommandOutcomeCanceled)
	if result.Failure != nil {
		providerErr := providerErrorFromAdapterFailure(result.Failure, executeErr, diagnostics)
		logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result.Command, duration)...)
		p.publishAgyFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
		return interfaces.InferenceResponse{}, providerErr
	}
	if executeErr != nil {
		if orchestrated := agyadapter.ClassifyOrchestrationError(executeErr); orchestrated.Failure != nil {
			providerErr := providerErrorFromAdapterFailure(orchestrated.Failure, executeErr, diagnostics)
			logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result.Command, duration)...)
			p.publishAgyFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
			return interfaces.InferenceResponse{}, providerErr
		}
		providerErr := normalizeProviderExecutionError(req.ModelProvider, result.Command, executeErr, result.Response.ProviderSession, diagnostics)
		p.publishAgyFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
		return interfaces.InferenceResponse{}, providerErr
	}
	response := result.Response
	response.Diagnostics = diagnostics
	logger.Info("inferencer: request completed",
		appendProviderSessionLogFields(providerLogFields(req, "output_len", len(response.Content)), response.ProviderSession)...)
	p.publishAgyCompleted(req.Dispatch.DispatchID, response.ProviderSession, len(result.Drafts) > 0)
	return response, nil
}

func (p *ScriptWrapProvider) agyRequestValidationError(req interfaces.ProviderInferenceRequest, err error) *ProviderError {
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

func (p *ScriptWrapProvider) publishAgyCompleted(
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

func (p *ScriptWrapProvider) publishAgyFailure(dispatchID string, err error, skipCanonical bool) {
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
