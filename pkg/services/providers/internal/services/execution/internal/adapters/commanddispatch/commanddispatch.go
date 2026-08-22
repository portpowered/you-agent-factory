package commanddispatch

import (
	"context"
	"errors"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// WithRequestContext carries the request-owned dispatch metadata through a
// platform-shaped command effect so an owner-private adapter can restore it.
func WithRequestContext(ctx context.Context, request providers.ExecuteRequest) context.Context {
	if ctx == nil {
		return nil
	}
	dispatchID := request.Correlation.DispatchID
	if dispatchID == "" {
		dispatchID = request.AttemptID
	}
	return work.WithCommandDispatch(ctx, work.WorkDispatch{
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
	})
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(values))
	for key, items := range values {
		clone[key] = append([]string(nil), items...)
	}
	return clone
}

// workFailureType* mirror the Workers normalized failure vocabulary that
// execution diagnostics are keyed by. Providers does not depend on Workers, so
// the values are repeated here as literals exactly as the ACP daemon already
// does for its missing-executable classification.
const (
	workFailureTypeMetadataKey    = "work-failure-type"
	workFailureTypeCommandTooLong = "command_line_too_long"
	workFailureTypeMisconfigured  = "misconfigured"
)

// StartFailure translates a subprocess that never started into a declared
// provider failure. The host reports an unstarted process as an ordinary error
// with no exit status and no output, so without this translation execution
// normalization sees only an unattributed native error and the operator is told
// nothing beyond "provider execution failed". An over-long command line is
// named separately because it is the one start failure the caller repairs by
// moving argument content off argv rather than by fixing the environment.
func StartFailure(err error) (providers.ExecuteFailure, bool) {
	var startErr *platformprocess.CommandStartError
	if !errors.As(err, &startErr) || startErr == nil {
		return providers.ExecuteFailure{}, false
	}
	failureType := workFailureTypeMisconfigured
	if startErr.OverCommandLineLimit() {
		failureType = workFailureTypeCommandTooLong
	}
	return providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindMisconfigured,
		Message: startErr.Error(),
		Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{
			workFailureTypeMetadataKey: failureType,
		}},
	}, true
}

// Request returns a Providers subprocess request with attempt correlation
// preserved for composition-owned effect adapters.
func Request(
	request providers.ExecuteRequest,
	command providerservice.CommandRequest,
) providerservice.CommandRequest {
	command.AttemptID = request.AttemptID
	command.WorkerType = request.WorkerType
	command.WorkstationName = request.WorkstationName
	command.Execution = work.ExecutionMetadata{
		RequestID: request.Correlation.RequestID,
		TraceID:   request.Correlation.TraceID,
		ReplayKey: request.Correlation.ReplayKey,
		WorkIDs:   append([]string(nil), request.Correlation.WorkIDs...),
	}
	command.ExecutionLogger = request.ExecutionLogger
	command.ProcessLifecycleObserver = request.ProcessLifecycleObserver
	return command
}
