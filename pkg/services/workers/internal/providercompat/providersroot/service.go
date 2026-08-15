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
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/conductor"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/registry"
	providerstructured "github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/structured"
)

// Config captures the per-worker effects projected into one Providers root.
type Config struct {
	Factory          *workerprovider.Factory
	SkipPermissions  bool
	Logger           logging.Logger
	Publish          workerprovider.InferenceProgressPublisher
	FactoryDirectory string
	ProviderRegistry *providerregistry.Registry
	Conductor        *conductor.Conductor
}

// Service owns one factory-built provider attempt without retry or
// provider-graph assembly. It is an internal execution adapter, not the
// published Providers root.
type Service struct {
	config   Config
	provider inferencecontract.LegacyProvider
}

// NewService validates config and constructs one inert execution adapter.
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

func (s *Service) providerInstance() (inferencecontract.LegacyProvider, error) {
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
		Dispatch: dispatch,
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: request.Correlation.FactorySessionID,
			RuntimeID:        request.Correlation.RuntimeID,
			GenerationID:     request.Correlation.GenerationID,
			DispatchID:       request.Correlation.DispatchID,
			AttemptID:        request.Correlation.AttemptID,
			RequestID:        request.Correlation.RequestID,
			TraceID:          request.Correlation.TraceID,
		},
		RunnerID:                     strings.TrimSpace(request.RunnerID),
		ProjectID:                    strings.TrimSpace(request.ProjectID),
		WorkerType:                   strings.TrimSpace(request.WorkerType),
		WorkstationType:              strings.TrimSpace(request.WorkstationName),
		Model:                        strings.TrimSpace(request.Model),
		ModelOperation:               strings.TrimSpace(request.ModelOperation),
		ModelBindings:                workerModelOperationBindings(request.ModelBindings),
		ModelLocality:                strings.TrimSpace(request.ModelLocality),
		ReasoningEffort:              canonicalReasoningEffort(request.ReasoningEffort),
		ModelProvider:                modelProviderForProviderIdentity(providerID),
		SystemPrompt:                 request.SystemPrompt,
		UserMessage:                  request.UserMessage,
		InputTokens:                  cloneInputTokens(request.InputTokens),
		OutputSchema:                 request.OutputSchema,
		ToolExecutionMode:            workers.RunnerToolExecutionMode(request.ToolExecutionMode),
		RequiredOptionalCapabilities: runnerOptionalCapabilities(request.RequiredCapabilities),
		Command:                      request.Command,
		Args:                         append([]string(nil), request.Args...),
		FactoryDirectory:             request.FactoryDirectory,
		OutputContract:               request.OutputContract,
		OutputFormat:                 request.OutputFormat,
		StopToken:                    request.StopToken,
		DecisionEnvelope:             request.DecisionEnvelope,
		GoalRoutingDecisionEnvelope:  request.GoalRoutingDecisionEnvelope,
		SessionID:                    request.SessionID,
		WorkingDirectory:             request.WorkingDirectory,
		Worktree:                     request.Worktree,
		EnvVars:                      cloneMetadata(request.EnvVars),
		ProcessEnvironment:           append([]string(nil), request.ProcessEnvironment...),
		SkipPermissions:              request.SkipPermissions,
		ExecutionLogger:              request.ExecutionLogger,
	}
	return infer
}

func workerModelOperationBindings(values []providers.ResolvedModelOperationBinding) []workers.ResolvedModelOperationBinding {
	if values == nil {
		return nil
	}
	converted := make([]workers.ResolvedModelOperationBinding, len(values))
	for index, value := range values {
		converted[index] = workers.ResolvedModelOperationBinding{
			Slot:    value.Slot,
			Source:  workers.ModelOperationBindingSource(value.Source),
			Content: work.CloneWorkContentParts(value.Content),
		}
	}
	return converted
}

func canonicalReasoningEffort(value string) string {
	canonical, _ := providers.ReasoningEffort(value).Canonical()
	return canonical
}

func runnerOptionalCapabilities(values []string) []workers.RunnerOptionalCapability {
	if len(values) == 0 {
		return nil
	}
	capabilities := make([]workers.RunnerOptionalCapability, len(values))
	for index, value := range values {
		capabilities[index] = workers.RunnerOptionalCapability(value)
	}
	return capabilities
}

func modelProviderForProviderIdentity(providerID string) string {
	switch workers.NormalizeRunnerID(providerID) {
	case workers.RunnerIDCodex:
		return string(modelprovider.ProviderCodex)
	case string(modelprovider.ProviderClaude):
		return string(modelprovider.ProviderClaude)
	case workers.RunnerIDAntigravity:
		return string(modelprovider.ProviderAntigravity)
	default:
		return providerID
	}
}

func workDispatch(request providers.ExecuteRequest) work.WorkDispatch {
	dispatchID := strings.TrimSpace(request.Correlation.DispatchID)
	if dispatchID == "" {
		dispatchID = strings.TrimSpace(request.AttemptID)
	}
	dispatch := work.WorkDispatch{
		DispatchID:      dispatchID,
		TransitionID:    strings.TrimSpace(request.TransitionID),
		WorkerType:      strings.TrimSpace(request.WorkerType),
		WorkstationName: strings.TrimSpace(request.WorkstationName),
		ProjectID:       strings.TrimSpace(request.ProjectID),
		InputTokens:     cloneInputTokens(request.InputTokens),
		InputBindings:   cloneStringSliceMap(request.InputBindings),
		Execution: work.ExecutionMetadata{
			RequestID: request.Correlation.RequestID,
			ReplayKey: request.Correlation.ReplayKey,
			TraceID:   request.Correlation.TraceID,
			WorkIDs:   append([]string(nil), request.Correlation.WorkIDs...),
		},
	}
	return dispatch
}

func executeResult(
	response workers.InferenceResponse,
	providerID providers.ID,
) providers.ExecuteResult {
	result := providers.ExecuteResult{
		Content: response.Content,
		Outcome: providers.ExecuteOutcome(response.Outcome),
	}
	if response.Continuation != nil {
		if reference, err := response.Continuation.ToSessionRef(); err == nil {
			reference.Provider = providerID
			result.SessionRef = &reference
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
		if response.Diagnostics.Command != nil {
			result.Diagnostics.Command = providersCommandDiagnostics(response.Diagnostics.Command)
		}
		if response.Diagnostics.Panic != nil {
			result.Diagnostics.Panic = &providers.ExecutePanicDiagnostics{
				Message: response.Diagnostics.Panic.Message,
				Stack:   response.Diagnostics.Panic.Stack,
			}
		}
		if duration := metadata[workers.ProviderResponseMetadataDurationMS]; duration != "" {
			if millis, err := strconv.ParseInt(duration, 10, 64); err == nil {
				result.Diagnostics.DurationMillis = millis
			}
		}
	}
	return result
}

func providersCommandDiagnostics(command *workers.CommandDiagnostic) *providers.ExecuteCommandDiagnostics {
	if command == nil {
		return nil
	}
	return &providers.ExecuteCommandDiagnostics{
		Command:    command.Command,
		Args:       append([]string(nil), command.Args...),
		Env:        cloneMetadata(command.Env),
		Stdin:      command.Stdin,
		Stdout:     command.Stdout,
		Stderr:     command.Stderr,
		ExitCode:   command.ExitCode,
		TimedOut:   command.TimedOut,
		DurationMS: command.Duration.Milliseconds(),
		WorkingDir: command.WorkingDir,
	}
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
	if providerErr.Continuation != nil {
		if reference, err := providerErr.Continuation.ToSessionRef(); err == nil {
			failure.SessionRef = &reference
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
		if providerErr.Diagnostics.Command != nil {
			failure.Diagnostics.Command = providersCommandDiagnostics(providerErr.Diagnostics.Command)
		}
		if providerErr.Diagnostics.Panic != nil {
			failure.Diagnostics.Panic = &providers.ExecutePanicDiagnostics{
				Message: providerErr.Diagnostics.Panic.Message,
				Stack:   providerErr.Diagnostics.Panic.Stack,
			}
		}
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

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string][]string, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
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
