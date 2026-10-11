package http

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func hasSubagentInput(request factoryapi.WorkerSessionStartRequest) bool {
	return request.Prompt != nil || request.Provider != nil || request.Model != nil ||
		request.ReasoningEffort != nil || request.WorkingRoot != nil || request.TimeoutMillis != nil
}

// startRequest translates the HTTP prompt form once into the existing Workers
// provider-invocation route. Selection remains owned by Settings and Providers;
// reservation and execution lifetime remain owned by Worker Sessions.
func (a *Adapter) startRequest(ctx context.Context, request factoryapi.WorkerSessionStartRequest) (workersessions.StartRequest, error) {
	if !hasSubagentInput(request) {
		return WorkerSessionStartRequestFromAPI(request)
	}
	start, err := subagentStartInput(request)
	if err != nil {
		return workersessions.StartRequest{}, err
	}
	if err := ctx.Err(); err != nil {
		return workersessions.StartRequest{}, err
	}
	if a.settings == nil || a.providers == nil || a.resolveHome == nil {
		return workersessions.StartRequest{}, fmt.Errorf("%w: subagent route resolution is unavailable", workersessions.ErrStartAdmissionFailed)
	}
	home, err := a.resolveHome()
	if err != nil {
		return workersessions.StartRequest{}, fmt.Errorf("%w: operator configuration home is unavailable", workersessions.ErrStartAdmissionFailed)
	}
	route, err := a.settings.ResolveSubagentRoute(ctx, a.settings.DefaultConfigPath(home), operatorsettings.SubagentRouteUpdate{
		Provider: request.Provider, Model: request.Model, ReasoningEffort: request.ReasoningEffort,
	})
	if err != nil {
		return workersessions.StartRequest{}, err
	}
	identity, err := a.providers.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: route.Provider})
	if err != nil {
		return workersessions.StartRequest{}, err
	}
	if err := a.providers.ValidateExecution(ctx, providers.ExecuteRequest{
		Provider: identity.ID, AttemptID: start.ID, Model: route.Model,
		ReasoningEffort: route.ReasoningEffort, UserMessage: *request.Prompt,
	}); err != nil {
		return workersessions.StartRequest{}, err
	}
	execution := &start.Execution.Execution
	execution.RunnerID, execution.ModelProvider = identity.ID.String(), identity.ID.String()
	execution.ExecutorProvider = identity.ID.String()
	execution.Model, execution.ReasoningEffort = route.Model, route.ReasoningEffort
	return start, ctx.Err()
}

func subagentStartInput(request factoryapi.WorkerSessionStartRequest) (workersessions.StartRequest, error) {
	invalid := func(message string) (workersessions.StartRequest, error) {
		return workersessions.StartRequest{}, fmt.Errorf("%w: %s", workersessions.ErrInvalidExecutionRequest, message)
	}
	if request.Execution != nil || request.Retry != nil || request.Prompt == nil || strings.TrimSpace(*request.Prompt) == "" {
		return invalid("supply either prompt or resolved execution; prompt does not accept retry")
	}
	for _, value := range []*string{request.Provider, request.Model, request.ReasoningEffort, request.WorkingRoot} {
		if value != nil && strings.TrimSpace(*value) == "" {
			return invalid("explicit route and workingRoot values must be non-empty")
		}
	}
	root := optionalString(request.WorkingRoot)
	if root != "" && !filepath.IsAbs(root) {
		return invalid("workingRoot must be an absolute path")
	}
	duration := time.Duration(0)
	if request.TimeoutMillis != nil {
		if *request.TimeoutMillis < 1 || *request.TimeoutMillis > 9223372036854 {
			return invalid("timeoutMillis must be positive and fit an execution duration")
		}
		duration = time.Duration(*request.TimeoutMillis) * time.Millisecond
	}
	start := workersessions.StartRequest{
		RequestID: strings.TrimSpace(request.RequestId), ID: strings.TrimSpace(request.WorkerSessionId),
		Execution: workers.WorkstationDispatchRequest{
			WorkstationName: workers.ProviderInvocationRoute,
			Execution: workers.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{DispatchID: strings.TrimSpace(request.WorkerSessionId), WorkstationName: workers.ProviderInvocationRoute,
					Execution: work.ExecutionMetadata{RequestID: strings.TrimSpace(request.RequestId)}},
				UserMessage: *request.Prompt, WorkingDirectory: root, WorkingDirectoryAuthored: root != "", Timeout: duration,
			},
		},
	}
	return start, start.Validate()
}
