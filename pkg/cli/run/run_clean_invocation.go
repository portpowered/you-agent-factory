package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func emitCleanInvocationOutcome(ctx context.Context, cfg RunConfig, runner factoryServiceRunner, runErr error, startedAt time.Time) error {
	logger := cleanInvocationLogger(cfg.Logger)
	provider, ok := runner.(engineStateSnapshotProvider)
	if !ok {
		if runErr == nil {
			err := &InvocationError{
				Code:    InvocationErrorCodeFailed,
				Message: "clean invocation result snapshot is unavailable",
			}
			recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
				StartedAt: startedAt,
				Err:       err,
			})
			return err
		}
		err := newInvocationErrorForRunFailure(runErr, nil)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Err:       err,
		})
		return err
	}
	snapshot, err := provider.GetEngineStateSnapshot(ctx)
	if err != nil {
		if runErr == nil {
			invocationErr := &InvocationError{
				Code:    InvocationErrorCodeFailed,
				Message: "clean invocation result snapshot is unavailable",
				Cause:   err,
			}
			recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
				StartedAt: startedAt,
				Err:       invocationErr,
			})
			return invocationErr
		}
		invocationErr := newInvocationErrorForRunFailure(runErr, nil)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Err:       invocationErr,
		})
		return invocationErr
	}
	if runErr != nil {
		invocationErr := newInvocationErrorForRunFailure(runErr, snapshot)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Snapshot:  snapshot,
			Err:       invocationErr,
		})
		return invocationErr
	}
	target, err := cleanInvocationWorkTargetFromFile(cfg.WorkFile)
	if err != nil {
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Snapshot:  snapshot,
			Err:       err,
		})
		return err
	}
	result, ok := cleanInvocationSuccessFromSnapshot(snapshot, target)
	if !ok {
		invocationErr := cleanInvocationFailureFromSnapshot(snapshot, target)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Snapshot:  snapshot,
			Target:    &target,
			Err:       invocationErr,
		})
		return invocationErr
	}
	if err := writeCleanInvocationSuccess(cfg, result); err != nil {
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			StartedAt: startedAt,
			Snapshot:  snapshot,
			Target:    &target,
			Err:       err,
		})
		return err
	}
	recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
		StartedAt: startedAt,
		Snapshot:  snapshot,
		Target:    &target,
		Success:   &result,
	})
	return nil
}

func newInvocationErrorForRunFailure(
	runErr error,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) error {
	switch {
	case errors.Is(runErr, context.DeadlineExceeded):
		return &InvocationError{
			Code:    InvocationErrorCodeTimeout,
			Message: "clean invocation timed out",
			Cause:   runErr,
		}
	case errors.Is(runErr, context.Canceled):
		return &InvocationError{
			Code:    InvocationErrorCodeCancelled,
			Message: "clean invocation cancelled",
			Cause:   runErr,
		}
	}

	if timeoutFailure, ok := cleanInvocationTimeoutFromSnapshot(snapshot); ok {
		return timeoutFailure
	}
	return &InvocationError{
		Code:    InvocationErrorCodeFailed,
		Message: "clean invocation failed",
		Cause:   runErr,
	}
}

func cleanInvocationFailureFromSnapshot(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) error {
	if timeoutFailure, ok := cleanInvocationTimeoutForTarget(snapshot, target); ok {
		return timeoutFailure
	}
	if failureReason, ok := cleanInvocationFailedForTarget(snapshot, target); ok {
		message := "clean invocation failed"
		if failureReason != "" {
			message = fmt.Sprintf("clean invocation failed: %s", failureReason)
		}
		return &InvocationError{
			Code:    InvocationErrorCodeFailed,
			Message: message,
		}
	}
	return &InvocationError{
		Code:    InvocationErrorCodeFailed,
		Message: fmt.Sprintf("clean invocation completed without a terminal success result for work %q", target.WorkID),
	}
}

func cleanInvocationTimeoutFromSnapshot(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) (*InvocationError, bool) {
	if snapshot == nil {
		return nil, false
	}
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		failure := snapshot.DispatchHistory[i].FailureMetadata
		if failure != nil && failure.Type == interfaces.WorkFailureTypeTimeout {
			return &InvocationError{
				Code:    InvocationErrorCodeTimeout,
				Message: "clean invocation timed out",
			}, true
		}
	}
	return nil, false
}

func cleanInvocationTimeoutForTarget(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) (*InvocationError, bool) {
	if snapshot == nil {
		return nil, false
	}
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		completion := snapshot.DispatchHistory[i]
		if !cleanInvocationCompletionMatchesTarget(completion, target) {
			continue
		}
		if completion.FailureMetadata != nil && completion.FailureMetadata.Type == interfaces.WorkFailureTypeTimeout {
			return &InvocationError{
				Code:    InvocationErrorCodeTimeout,
				Message: "clean invocation timed out",
			}, true
		}
	}
	return nil, false
}

func cleanInvocationFailedForTarget(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) (string, bool) {
	if snapshot == nil {
		return "", false
	}
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		completion := snapshot.DispatchHistory[i]
		if completion.Outcome != interfaces.OutcomeFailed {
			continue
		}
		if !cleanInvocationCompletionMatchesTarget(completion, target) {
			continue
		}
		return strings.TrimSpace(completion.Reason), true
	}
	if snapshot.Topology == nil {
		return "", false
	}
	for _, token := range snapshot.Marking.Tokens {
		if token == nil {
			continue
		}
		if token.Color.WorkID != target.WorkID || token.Color.WorkTypeID != target.WorkTypeName {
			continue
		}
		if snapshot.Topology.StateCategoryForPlace(token.PlaceID) == state.StateCategoryFailed {
			return "", true
		}
	}
	return "", false
}

func cleanInvocationWorkTargetFromFile(workFile string) (cleanInvocationWorkTarget, error) {
	request, err := LoadWorkFile(workFile)
	if err != nil {
		return cleanInvocationWorkTarget{}, err
	}
	normalized, err := requests.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{})
	if err != nil {
		return cleanInvocationWorkTarget{}, err
	}
	if len(normalized) != 1 {
		return cleanInvocationWorkTarget{}, fmt.Errorf("clean invocation requires exactly one work item, got %d", len(normalized))
	}
	return cleanInvocationWorkTarget{
		WorkID:       normalized[0].WorkID,
		WorkTypeName: normalized[0].WorkTypeID,
	}, nil
}

func cleanInvocationSuccessFromSnapshot(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) (cleanInvocationSuccess, bool) {
	if snapshot == nil || snapshot.Topology == nil {
		return cleanInvocationSuccess{}, false
	}
	if result, ok := cleanInvocationSuccessFromTerminalTokens(snapshot, target); ok {
		return result, true
	}
	return cleanInvocationSuccessFromDispatchHistory(snapshot, target)
}

func cleanInvocationSuccessFromTerminalTokens(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) (cleanInvocationSuccess, bool) {
	tokens := make([]*interfaces.Token, 0, len(snapshot.Marking.Tokens))
	for _, token := range snapshot.Marking.Tokens {
		if token != nil {
			tokens = append(tokens, token)
		}
	}
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].ID < tokens[j].ID
	})
	for _, token := range tokens {
		if cleanInvocationTokenMatches(snapshot.Topology, token, target) {
			return cleanInvocationSuccessFromToken(token), true
		}
	}
	return cleanInvocationSuccess{}, false
}

func cleanInvocationSuccessFromDispatchHistory(
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	target cleanInvocationWorkTarget,
) (cleanInvocationSuccess, bool) {
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		completion := snapshot.DispatchHistory[i]
		if completion.Outcome != interfaces.OutcomeAccepted {
			continue
		}
		for _, mutation := range completion.OutputMutations {
			if cleanInvocationTokenMatches(snapshot.Topology, mutation.Token, target) {
				return cleanInvocationSuccessFromToken(mutation.Token), true
			}
		}
	}
	return cleanInvocationSuccess{}, false
}

func cleanInvocationTokenMatches(net *state.Net, token *interfaces.Token, target cleanInvocationWorkTarget) bool {
	if net == nil || token == nil {
		return false
	}
	if token.Color.DataType == interfaces.DataTypeResource {
		return false
	}
	if token.Color.WorkID != target.WorkID || token.Color.WorkTypeID != target.WorkTypeName {
		return false
	}
	return net.StateCategoryForPlace(token.PlaceID) == state.StateCategoryTerminal
}

func cleanInvocationSuccessFromToken(token *interfaces.Token) cleanInvocationSuccess {
	return cleanInvocationSuccess{
		Output:       string(token.Color.Payload),
		WorkID:       token.Color.WorkID,
		WorkTypeName: token.Color.WorkTypeID,
		TraceID:      token.Color.TraceID,
		SessionID:    defaultFactorySessionID,
	}
}

func writeCleanInvocationSuccess(cfg RunConfig, result cleanInvocationSuccess) error {
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}
	if cfg.JSON {
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = output.Write(data)
		return err
	}
	_, err := io.WriteString(output, result.Output)
	return err
}

func cleanInvocationCompletionMatchesTarget(
	completion interfaces.CompletedDispatch,
	target cleanInvocationWorkTarget,
) bool {
	for _, token := range completion.ConsumedTokens {
		if token.Color.WorkID == target.WorkID && token.Color.WorkTypeID == target.WorkTypeName {
			return true
		}
	}
	for _, mutation := range completion.OutputMutations {
		if mutation.Token == nil {
			continue
		}
		if mutation.Token.Color.WorkID == target.WorkID && mutation.Token.Color.WorkTypeID == target.WorkTypeName {
			return true
		}
	}
	return false
}
