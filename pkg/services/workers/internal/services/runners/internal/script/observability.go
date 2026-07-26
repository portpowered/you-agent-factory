package script

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (r *runner) outputObserver(dispatchID string) platformprocess.OutputChunkObserver {
	var publishMu sync.Mutex
	return func(stream string, chunk []byte) {
		if len(chunk) == 0 {
			return
		}
		publishMu.Lock()
		defer publishMu.Unlock()
		r.publish(workers.ProgressFragment{
			DispatchID: dispatchID,
			Kind:       workers.ProgressFragmentKind,
			Type:       stream,
			Payload:    string(append([]byte(nil), chunk...)),
			Metadata:   map[string]string{"stream": stream},
		})
	}
}

func commandDiagnostics(
	request workers.CommandRequest,
	result workers.CommandResult,
	duration time.Duration,
) *workers.WorkDiagnostics {
	env := workers.ProjectCommandEnvForDiagnostics(request.Env)
	return &workers.WorkDiagnostics{
		Command: &workers.CommandDiagnostic{
			Command:    request.Command,
			Args:       append([]string(nil), request.Args...),
			Env:        cloneStringMap(env.Values),
			Stdout:     string(result.Stdout),
			Stderr:     string(result.Stderr),
			ExitCode:   result.ExitCode,
			Duration:   duration,
			WorkingDir: request.WorkDir,
		},
		Metadata: commandMetadata(request, env),
	}
}

func commandMetadata(
	request workers.CommandRequest,
	env workers.CommandEnvDiagnosticProjection,
) map[string]string {
	return map[string]string{
		"dispatch_id":                 request.DispatchID,
		"transition_id":               request.TransitionID,
		"current_chaining_trace_id":   request.CurrentChainingTraceID,
		"previous_chaining_trace_ids": strings.Join(request.PreviousChainingTraceIDs, ","),
		"request_id":                  request.Execution.RequestID,
		"env_count":                   strconv.Itoa(env.Count),
		"env_keys":                    strings.Join(env.Keys, ","),
	}
}

func scriptRequestID(dispatchID string) string {
	if dispatchID == "" {
		return fmt.Sprintf("script-request/%d", scriptAttempt)
	}
	return fmt.Sprintf("%s/script-request/%d", dispatchID, scriptAttempt)
}

func scriptRequestEvent(
	request workers.CommandRequest,
	requestID string,
	eventTime time.Time,
) workers.ScriptEvent {
	return scriptEvent(request, eventTime, &workers.ScriptRequestEventPayload{
		Args:            append([]string(nil), request.Args...),
		Attempt:         scriptAttempt,
		Command:         request.Command,
		DispatchID:      request.DispatchID,
		ScriptRequestID: requestID,
		TransitionID:    request.TransitionID,
	}, nil)
}

func scriptSuccessEvent(
	request workers.CommandRequest,
	requestID string,
	result workers.CommandResult,
	duration time.Duration,
	eventTime time.Time,
) workers.ScriptEvent {
	exitCode := result.ExitCode
	return scriptEvent(request, eventTime, nil, &workers.ScriptResponseEventPayload{
		Attempt:         scriptAttempt,
		DispatchID:      request.DispatchID,
		DurationMillis:  duration.Milliseconds(),
		ExitCode:        &exitCode,
		Outcome:         workers.ScriptExecutionOutcomeSucceeded,
		ScriptRequestID: requestID,
		Stderr:          string(result.Stderr),
		Stdout:          string(result.Stdout),
		TransitionID:    request.TransitionID,
	})
}

func scriptFailureEvent(
	request workers.CommandRequest,
	requestID string,
	result workers.CommandResult,
	duration time.Duration,
	outcome workers.ScriptExecutionOutcome,
	failureType workers.ScriptFailureType,
	eventTime time.Time,
) workers.ScriptEvent {
	var exitCode *int
	if outcome == workers.ScriptExecutionOutcomeFailedExitCode {
		value := result.ExitCode
		exitCode = &value
	}
	var failureTypePointer *workers.ScriptFailureType
	if failureType != "" {
		failureTypePointer = &failureType
	}
	return scriptEvent(request, eventTime, nil, &workers.ScriptResponseEventPayload{
		Attempt:         scriptAttempt,
		DispatchID:      request.DispatchID,
		DurationMillis:  duration.Milliseconds(),
		ExitCode:        exitCode,
		FailureType:     failureTypePointer,
		Outcome:         outcome,
		ScriptRequestID: requestID,
		Stderr:          string(result.Stderr),
		Stdout:          string(result.Stdout),
		TransitionID:    request.TransitionID,
	})
}

func scriptEvent(
	request workers.CommandRequest,
	eventTime time.Time,
	requestPayload *workers.ScriptRequestEventPayload,
	responsePayload *workers.ScriptResponseEventPayload,
) workers.ScriptEvent {
	kind := workers.ScriptEventKindRequest
	id := fmt.Sprintf("%s/%s", scriptRequestEventIDPrefix, scriptRequestID(request.DispatchID))
	if responsePayload != nil {
		kind = workers.ScriptEventKindResponse
		id = scriptResponseEventID(request.DispatchID)
	}
	return workers.ScriptEvent{
		ID:         id,
		Kind:       kind,
		EventTime:  eventTime.UTC(),
		Tick:       scriptEventTick(request.Execution),
		DispatchID: request.DispatchID,
		RequestID:  request.Execution.RequestID,
		TraceIDs:   nonEmptyStrings(request.Execution.TraceID),
		WorkIDs:    nonEmptyStrings(request.Execution.WorkIDs...),
		Request:    requestPayload,
		Response:   responsePayload,
	}
}

func scriptResponseEventID(dispatchID string) string {
	if dispatchID == "" {
		return fmt.Sprintf("%s/%d", scriptResponseEventIDPrefix, scriptAttempt)
	}
	return fmt.Sprintf("%s/%s/%d", scriptResponseEventIDPrefix, dispatchID, scriptAttempt)
}

func scriptEventTick(metadata work.ExecutionMetadata) int {
	if metadata.CurrentTick != 0 {
		return metadata.CurrentTick
	}
	return metadata.DispatchCreatedTick
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
