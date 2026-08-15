package testutil

import (
	"context"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProviderServiceAdapter gives legacy inference-shaped test doubles a
// Providers-root contract without reintroducing a production Workers provider
// port. It is intentionally test-only and translates only detached values.
type ProviderServiceAdapter struct {
	InferFunc func(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error)
}

func (adapter ProviderServiceAdapter) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (adapter ProviderServiceAdapter) GetProvider(_ context.Context, request providers.GetProviderRequest) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	return providers.GetProviderResult{Provider: providers.Descriptor{ID: request.ID}}, nil
}

func (adapter ProviderServiceAdapter) ResolveIdentity(_ context.Context, request providers.ResolveIdentityRequest) (providers.ResolveIdentityResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ResolveIdentityResult{}, err
	}
	return providers.ResolveIdentityResult{ID: providers.ID(strings.TrimSpace(request.Identity))}, nil
}

func (adapter ProviderServiceAdapter) ResolveSelection(_ context.Context, request providers.ResolveSelectionRequest) (providers.ResolveSelectionResult, error) {
	identity := request.Workstation
	if identity == "" {
		identity = request.Factory
	}
	if identity == "" {
		identity = request.ModelProvider
	}
	resolved, err := adapter.ResolveIdentity(context.Background(), providers.ResolveIdentityRequest{Identity: identity})
	if err != nil {
		return providers.ResolveSelectionResult{}, err
	}
	return providers.ResolveSelectionResult{Provider: resolved.ID}, nil
}

func (adapter ProviderServiceAdapter) ValidatePrerequisites(_ context.Context, request providers.ValidatePrerequisitesRequest) error {
	return request.Validate()
}

func (adapter ProviderServiceAdapter) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	return adapter.execute(ctx, request, nil)
}

func (adapter ProviderServiceAdapter) execute(
	ctx context.Context,
	request providers.ExecuteRequest,
	resume *providers.SessionRef,
) (providers.ExecuteResult, error) {
	if adapter.InferFunc == nil {
		return providers.ExecuteResult{}, providers.ErrExecuteFailed
	}
	dispatchID := request.Correlation.DispatchID
	if dispatchID == "" {
		dispatchID = request.AttemptID
	}
	response, err := adapter.InferFunc(ctx, workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      dispatchID,
			TransitionID:    request.TransitionID,
			WorkerType:      request.WorkerType,
			WorkstationName: request.WorkstationName,
			ProjectID:       request.ProjectID,
			InputTokens:     append([]any(nil), request.InputTokens...),
			InputBindings:   cloneStringSliceMap(request.InputBindings),
			Execution: work.ExecutionMetadata{
				RequestID: request.Correlation.RequestID,
				TraceID:   request.Correlation.TraceID,
				ReplayKey: request.Correlation.ReplayKey,
				WorkIDs:   append([]string(nil), request.Correlation.WorkIDs...),
			},
		},
		Correlation: workerexecution.ExecutionCorrelation{
			FactorySessionID: request.Correlation.FactorySessionID,
			RuntimeID:        request.Correlation.RuntimeID,
			GenerationID:     request.Correlation.GenerationID,
			DispatchID:       request.Correlation.DispatchID,
			AttemptID:        request.Correlation.AttemptID,
			RequestID:        request.Correlation.RequestID,
			TraceID:          request.Correlation.TraceID,
		},
		RunnerID:                     request.RunnerID,
		ProjectID:                    request.ProjectID,
		WorkerType:                   request.WorkerType,
		WorkstationType:              request.WorkstationName,
		Model:                        request.Model,
		ModelProvider:                request.Provider.String(),
		ReasoningEffort:              request.ReasoningEffort,
		SystemPrompt:                 request.SystemPrompt,
		UserMessage:                  request.UserMessage,
		InputTokens:                  append([]any(nil), request.InputTokens...),
		ModelOperation:               request.ModelOperation,
		ModelBindings:                workerModelOperationBindings(request.ModelBindings),
		OutputSchema:                 request.OutputSchema,
		ToolExecutionMode:            workerexecution.RunnerToolExecutionMode(request.ToolExecutionMode),
		RequiredOptionalCapabilities: runnerCapabilities(request.RequiredCapabilities),
		Command:                      request.Command,
		Args:                         append([]string(nil), request.Args...),
		FactoryDirectory:             request.FactoryDirectory,
		OutputContract:               request.OutputContract,
		OutputFormat:                 request.OutputFormat,
		StopToken:                    request.StopToken,
		DecisionEnvelope:             request.DecisionEnvelope,
		GoalRoutingDecisionEnvelope:  request.GoalRoutingDecisionEnvelope,
		ModelLocality:                request.ModelLocality,
		SessionID:                    request.SessionID,
		ResumeSession:                cloneSessionRef(resume),
		WorkingDirectory:             request.WorkingDirectory,
		Worktree:                     request.Worktree,
		EnvVars:                      cloneStringMap(request.EnvVars),
		ProcessEnvironment:           append([]string(nil), request.ProcessEnvironment...),
		SkipPermissions:              request.SkipPermissions,
		PrintTimeout:                 request.PrintTimeout,
		ExecutionLogger:              request.ExecutionLogger,
	})
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	result := providers.ExecuteResult{
		Content: response.Content,
		Outcome: providers.ExecuteOutcome(response.Outcome),
	}
	if response.ProviderSession != nil {
		result.SessionRef = &providers.SessionRef{
			Provider: providers.ID(response.ProviderSession.Provider),
			Kind:     response.ProviderSession.Kind,
			ID:       response.ProviderSession.ID,
		}
	}
	if response.Diagnostics != nil {
		metadata := cloneStringMap(response.Diagnostics.Metadata)
		if response.Diagnostics.Provider != nil {
			metadata = mergeStringMap(metadata, response.Diagnostics.Provider.ResponseMetadata)
		}
		result.Diagnostics = &providers.ExecuteDiagnostics{Metadata: metadata}
		if response.Diagnostics.Command != nil {
			result.Diagnostics.Command = &providers.ExecuteCommandDiagnostics{
				Command:    response.Diagnostics.Command.Command,
				Args:       append([]string(nil), response.Diagnostics.Command.Args...),
				Env:        cloneStringMap(response.Diagnostics.Command.Env),
				Stdin:      response.Diagnostics.Command.Stdin,
				Stdout:     response.Diagnostics.Command.Stdout,
				Stderr:     response.Diagnostics.Command.Stderr,
				ExitCode:   response.Diagnostics.Command.ExitCode,
				TimedOut:   response.Diagnostics.Command.TimedOut,
				DurationMS: response.Diagnostics.Command.Duration.Milliseconds(),
				WorkingDir: response.Diagnostics.Command.WorkingDir,
			}
		}
		if response.Diagnostics.Panic != nil {
			result.Diagnostics.Panic = &providers.ExecutePanicDiagnostics{
				Message: response.Diagnostics.Panic.Message,
				Stack:   response.Diagnostics.Panic.Stack,
			}
		}
	}
	return result, nil
}

func workerModelOperationBindings(values []providers.ResolvedModelOperationBinding) []workerexecution.ResolvedModelOperationBinding {
	if values == nil {
		return nil
	}
	converted := make([]workerexecution.ResolvedModelOperationBinding, len(values))
	for index, value := range values {
		converted[index] = workerexecution.ResolvedModelOperationBinding{
			Slot:    value.Slot,
			Source:  workerexecution.ModelOperationBindingSource(value.Source),
			Content: work.CloneWorkContentParts(value.Content),
		}
	}
	return converted
}

func (adapter ProviderServiceAdapter) ControlAttempt(_ context.Context, request providers.ControlAttemptRequest) (providers.ControlAttemptResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ControlAttemptResult{}, err
	}
	return providers.ControlAttemptResult{Provider: request.Provider, AttemptID: request.AttemptID, Action: request.Action, Outcome: providers.ControlOutcomeUnsupported}, nil
}

func (adapter ProviderServiceAdapter) Continue(ctx context.Context, request providers.ContinueRequest) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	result, err := adapter.execute(ctx, request.Attempt, &request.Reference)
	if err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{Reference: request.Reference, Outcome: providers.ContinuationOutcomeResumed, Result: result}, nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

func mergeStringMap(base, overlay map[string]string) map[string]string {
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

func cloneSessionRef(reference *providers.SessionRef) *providers.SessionRef {
	if reference == nil {
		return nil
	}
	cloned := reference.Clone()
	return &cloned
}

func runnerCapabilities(values []string) []workerexecution.RunnerOptionalCapability {
	if len(values) == 0 {
		return nil
	}
	capabilities := make([]workerexecution.RunnerOptionalCapability, len(values))
	for index, value := range values {
		capabilities[index] = workerexecution.RunnerOptionalCapability(value)
	}
	return capabilities
}
