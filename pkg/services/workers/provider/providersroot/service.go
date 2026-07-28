// Package providersroot adapts the validated provider-worker factory onto the
// public Providers root contract for Agent Runner dispatch.
package providersroot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
	providerstructured "github.com/portpowered/infinite-you/pkg/services/workers/provider/structured"
)

// Config captures the per-worker effects projected into one Providers root.
type Config struct {
	Factory           *workerprovider.Factory
	SkipPermissions   bool
	Logger            logging.Logger
	Publish           workerprovider.InferenceProgressPublisher
	FactoryDirectory  string
	ProviderRegistry  *providerregistry.Registry
	Conductor         *conductor.Conductor
}

// Service implements providers.Service by delegating Execute to one
// factory-built provider attempt without retry or provider-graph assembly.
type Service struct {
	config   Config
	provider inferencecontract.Provider
}

var _ providers.Service = (*Service)(nil)

// NewService validates config and constructs one inert Providers root.
func NewService(config Config) (*Service, error) {
	if config.Factory == nil {
		return nil, fmt.Errorf("construct Providers root: provider factory is required")
	}
	return &Service{config: config}, nil
}

func (s *Service) ListProviders(
	_ context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (s *Service) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.ID.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	return providers.GetProviderResult{}, providers.ErrUnknownProvider
}

func (s *Service) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: err.Error(),
		}
	}
	if s.shouldRouteConductor(request.Provider.String()) {
		return s.executeViaConductor(ctx, request)
	}
	provider, err := s.providerInstance()
	if err != nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: err.Error(),
		}
	}
	response, err := provider.Infer(ctx, inferenceRequest(request))
	if err != nil {
		return providers.ExecuteResult{}, mapExecuteError(err)
	}
	return executeResult(response, request.Provider), nil
}

func (s *Service) providerInstance() (inferencecontract.Provider, error) {
	if s.provider != nil {
		return s.provider, nil
	}
	var responseExecutor workerprovider.ResponseStreamExecutor
	if s.config.Publish != nil {
		responseExecutor = providerstructured.NewExecutor()
	}
	provider, err := s.config.Factory.New(
		s.config.SkipPermissions,
		logging.EnsureLogger(s.config.Logger),
		s.config.Publish,
		responseExecutor,
	)
	if err != nil {
		return nil, err
	}
	s.provider = provider
	return provider, nil
}

func inferenceRequest(request providers.ExecuteRequest) workers.ProviderInferenceRequest {
	providerID := request.Provider.String()
	dispatch := workDispatch(request)
	infer := workers.ProviderInferenceRequest{
		Dispatch:           dispatch,
		WorkerType:         strings.TrimSpace(request.WorkerType),
		WorkstationType:    strings.TrimSpace(request.WorkstationName),
		Model:              strings.TrimSpace(request.Model),
		ModelProvider:      modelProviderForProviderIdentity(providerID),
		SystemPrompt:       request.SystemPrompt,
		UserMessage:        request.UserMessage,
		InputTokens:        cloneInputTokens(request.InputTokens),
		OutputSchema:       request.OutputSchema,
		WorkingDirectory:   request.WorkingDirectory,
		Worktree:           request.Worktree,
		EnvVars:            cloneMetadata(request.EnvVars),
		ProcessEnvironment: append([]string(nil), request.ProcessEnvironment...),
	}
	if request.ResumeSession != nil {
		infer.SessionID = request.ResumeSession.ID
	}
	return infer
}

func modelProviderForProviderIdentity(providerID string) string {
	switch workers.NormalizeRunnerID(providerID) {
	case workers.RunnerIDCodex:
		return string(modelprovider.ProviderCodex)
	case string(modelprovider.ProviderClaude):
		return string(modelprovider.ProviderClaude)
	case workers.RunnerIDGemini:
		return string(modelprovider.ProviderGemini)
	case workers.RunnerIDKiro:
		return string(modelprovider.ProviderKiro)
	case "cursor", workers.RunnerIDCursorCLI:
		return string(modelprovider.ProviderCursor)
	case workers.RunnerIDOpenCode:
		return string(modelprovider.ProviderOpenCode)
	case workers.RunnerIDPi:
		return string(modelprovider.ProviderPi)
	case workers.RunnerIDAgy:
		return string(modelprovider.ProviderAgy)
	default:
		return providerID
	}
}

func workDispatch(request providers.ExecuteRequest) work.WorkDispatch {
	dispatch := work.WorkDispatch{
		DispatchID: strings.TrimSpace(request.AttemptID),
		WorkerType: strings.TrimSpace(request.WorkerType),
		WorkstationName: strings.TrimSpace(request.WorkstationName),
	}
	return dispatch
}

func executeResult(
	response workers.InferenceResponse,
	providerID providers.ID,
) providers.ExecuteResult {
	result := providers.ExecuteResult{Content: response.Content}
	if response.ProviderSession != nil {
		result.SessionRef = &providers.SessionRef{
			Provider: providerID,
			Kind:     response.ProviderSession.Kind,
			ID:       response.ProviderSession.ID,
		}
	}
	if response.Diagnostics != nil {
		metadata := cloneMetadata(response.Diagnostics.Metadata)
		if response.Diagnostics.Provider != nil &&
			len(response.Diagnostics.Provider.ResponseMetadata) > 0 {
			metadata = mergeMetadata(
				metadata,
				response.Diagnostics.Provider.ResponseMetadata,
			)
		}
		result.Diagnostics = &providers.ExecuteDiagnostics{
			Metadata: metadata,
		}
		if duration := metadata[workers.ProviderResponseMetadataDurationMS]; duration != "" {
			if millis, err := strconv.ParseInt(duration, 10, 64); err == nil {
				result.Diagnostics.DurationMillis = millis
			}
		}
	}
	return result
}

func mapExecuteError(err error) error {
	if failure, ok := executeFailureFromProvider(err); ok {
		return failure
	}
	if errors.Is(err, context.Canceled) {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindCanceled,
			Message: err.Error(),
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindTimeout,
			Message: err.Error(),
		}
	}
	return providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: err.Error(),
	}
}

func executeFailureFromProvider(err error) (providers.ExecuteFailure, bool) {
	providerErr := workerprovider.NormalizeProviderExecutionError(err)
	if providerErr == nil {
		return providers.ExecuteFailure{}, false
	}
	failure := providers.ExecuteFailure{
		Kind:    failureKindForProviderType(providerErr.Type),
		Message: providerErr.Message,
	}
	if providerErr.ProviderSession != nil {
		failure.SessionRef = &providers.SessionRef{
			Provider: providers.ID(
				workers.CanonicalProviderSessionProvider(providerErr.ProviderSession.Provider),
			),
			Kind: providerErr.ProviderSession.Kind,
			ID:   providerErr.ProviderSession.ID,
		}
	}
	if providerErr.Diagnostics != nil {
		metadata := cloneMetadata(providerErr.Diagnostics.Metadata)
		if providerErr.Diagnostics.Provider != nil &&
			len(providerErr.Diagnostics.Provider.ResponseMetadata) > 0 {
			metadata = mergeMetadata(
				metadata,
				providerErr.Diagnostics.Provider.ResponseMetadata,
			)
		}
		failure.Diagnostics = &providers.ExecuteDiagnostics{Metadata: metadata}
	}
	return failure, true
}

func failureKindForProviderType(
	failureType workers.WorkFailureType,
) providers.ExecuteFailureKind {
	switch failureType {
	case workers.WorkFailureTypeAuthFailure:
		return providers.ExecuteFailureKindAuthentication
	case workers.WorkFailureTypePermanentBadRequest:
		return providers.ExecuteFailureKindInvalidRequest
	case workers.WorkFailureTypeThrottled:
		return providers.ExecuteFailureKindThrottled
	case workers.WorkFailureTypeTimeout:
		return providers.ExecuteFailureKindTimeout
	case workers.WorkFailureTypeMisconfigured,
		workers.WorkFailureTypeMissingExecutable,
		workers.WorkFailureTypeCommandLineTooLong,
		workers.WorkFailureTypeInternalServerError:
		return providers.ExecuteFailureKindDependency
	default:
		return providers.ExecuteFailureKindUnknown
	}
}

func cloneMetadata(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneInputTokens(values []any) []any {
	if values == nil {
		return nil
	}
	return append([]any(nil), values...)
}

func mergeMetadata(base, overlay map[string]string) map[string]string {
	if len(overlay) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]string, len(overlay))
	}
	for key, value := range overlay {
		base[key] = value
	}
	return base
}
