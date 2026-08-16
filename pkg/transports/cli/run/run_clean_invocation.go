package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/batchload"
)

func emitCleanInvocationOutcome(
	ctx context.Context,
	cfg RunConfig,
	runner factoryServiceRunner,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	runErr error,
	duration time.Duration,
) error {
	logger := cleanInvocationLogger(cfg.Logger)
	provider, ok := runner.(factoryruntime.CleanInvocationSnapshotProvider)
	if !ok {
		if runErr == nil {
			err := &InvocationError{
				Code:    InvocationErrorCodeFailed,
				Message: "clean invocation result snapshot is unavailable",
			}
			recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
				Duration: duration,
				Err:      err,
			})
			return err
		}
		err := newInvocationErrorForRunFailure(runErr, nil)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Err:      err,
		})
		return err
	}
	snapshotValue, err := provider.CleanInvocationSnapshot(ctx)
	if err != nil {
		if runErr == nil {
			invocationErr := &InvocationError{
				Code:    InvocationErrorCodeFailed,
				Message: "clean invocation result snapshot is unavailable",
				Cause:   err,
			}
			recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
				Duration: duration,
				Err:      invocationErr,
			})
			return invocationErr
		}
		invocationErr := newInvocationErrorForRunFailure(runErr, nil)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Err:      invocationErr,
		})
		return invocationErr
	}
	snapshot := &snapshotValue
	if runErr != nil {
		invocationErr := newInvocationErrorForRunFailure(runErr, snapshot)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Err:      invocationErr,
		})
		return invocationErr
	}
	target, err := cleanInvocationWorkTargetFromFile(
		cfg.WorkRequestFileLoader,
		prepareWorkTarget,
		cfg.WorkFile,
	)
	if err != nil {
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Err:      err,
		})
		return err
	}
	result, ok := cleanInvocationSuccessFromSnapshot(snapshot, target)
	if !ok {
		invocationErr := cleanInvocationFailureFromSnapshot(snapshot, target)
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Target:   &target,
			Err:      invocationErr,
		})
		return invocationErr
	}
	if err := writeCleanInvocationSuccess(cfg, result); err != nil {
		recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Target:   &target,
			Err:      err,
		})
		return err
	}
	recordCleanInvocationCompletion(logger, cfg, cleanInvocationCompletionLogInput{
		Duration: duration,
		Target:   &target,
		Success:  &result,
	})
	return nil
}

func newInvocationErrorForRunFailure(
	runErr error,
	snapshot *factoryruntime.CleanInvocationSnapshot,
) error {
	var recorded *InvocationError
	if errors.As(runErr, &recorded) {
		return recorded
	}
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
	snapshot *factoryruntime.CleanInvocationSnapshot,
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
	snapshot *factoryruntime.CleanInvocationSnapshot,
) (*InvocationError, bool) {
	if snapshot == nil {
		return nil, false
	}
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		failureType := snapshot.DispatchHistory[i].FailureType
		if failureType == string(workerexecution.WorkFailureTypeTimeout) {
			return &InvocationError{
				Code:    InvocationErrorCodeTimeout,
				Message: "clean invocation timed out",
			}, true
		}
	}
	return nil, false
}

func cleanInvocationTimeoutForTarget(
	snapshot *factoryruntime.CleanInvocationSnapshot,
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
		if completion.FailureType == string(workerexecution.WorkFailureTypeTimeout) {
			return &InvocationError{
				Code:    InvocationErrorCodeTimeout,
				Message: "clean invocation timed out",
			}, true
		}
	}
	return nil, false
}

func cleanInvocationFailedForTarget(
	snapshot *factoryruntime.CleanInvocationSnapshot,
	target cleanInvocationWorkTarget,
) (string, bool) {
	if snapshot == nil {
		return "", false
	}
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		completion := snapshot.DispatchHistory[i]
		if completion.Outcome != string(workerexecution.OutcomeFailed) {
			continue
		}
		if !cleanInvocationCompletionMatchesTarget(completion, target) {
			continue
		}
		return strings.TrimSpace(completion.Reason), true
	}
	for _, token := range snapshot.Work {
		if token.WorkID != target.WorkID || token.WorkTypeID != target.WorkTypeName {
			continue
		}
		if token.StateCategory == string(factoryruntime.StateCategoryFailed) {
			return "", true
		}
	}
	return "", false
}

func cleanInvocationWorkTargetFromFile(
	load work.RequestFileLoader,
	prepare work.SingleWorkTargetPreparation,
	workFile string,
) (cleanInvocationWorkTarget, error) {
	request, err := batchload.LoadFromFile(load, workFile)
	if err != nil {
		return cleanInvocationWorkTarget{}, err
	}
	if prepare == nil {
		return cleanInvocationWorkTarget{}, fmt.Errorf("clean invocation Work target preparation is required")
	}
	target, err := prepare(request)
	if err != nil {
		return cleanInvocationWorkTarget{}, err
	}
	return cleanInvocationWorkTarget{
		WorkID:       target.WorkID,
		WorkTypeName: target.WorkTypeID,
	}, nil
}

func cleanInvocationSuccessFromSnapshot(
	snapshot *factoryruntime.CleanInvocationSnapshot,
	target cleanInvocationWorkTarget,
) (cleanInvocationSuccess, bool) {
	if snapshot == nil {
		return cleanInvocationSuccess{}, false
	}
	if result, ok := cleanInvocationSuccessFromTerminalTokens(snapshot, target); ok {
		return result, true
	}
	return cleanInvocationSuccessFromDispatchHistory(snapshot, target)
}

func cleanInvocationSuccessFromTerminalTokens(
	snapshot *factoryruntime.CleanInvocationSnapshot,
	target cleanInvocationWorkTarget,
) (cleanInvocationSuccess, bool) {
	tokens := append([]factoryruntime.CleanInvocationWork(nil), snapshot.Work...)
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].WorkID != tokens[j].WorkID {
			return tokens[i].WorkID < tokens[j].WorkID
		}
		return tokens[i].TraceID < tokens[j].TraceID
	})
	for _, token := range tokens {
		if cleanInvocationTokenMatches(token, target) {
			return cleanInvocationSuccessFromToken(token), true
		}
	}
	return cleanInvocationSuccess{}, false
}

func cleanInvocationSuccessFromDispatchHistory(
	snapshot *factoryruntime.CleanInvocationSnapshot,
	target cleanInvocationWorkTarget,
) (cleanInvocationSuccess, bool) {
	for i := len(snapshot.DispatchHistory) - 1; i >= 0; i-- {
		completion := snapshot.DispatchHistory[i]
		if completion.Outcome != string(workerexecution.OutcomeAccepted) {
			continue
		}
		for _, output := range completion.Outputs {
			if cleanInvocationTokenMatches(output, target) {
				return cleanInvocationSuccessFromToken(output), true
			}
		}
	}
	return cleanInvocationSuccess{}, false
}

func cleanInvocationTokenMatches(token factoryruntime.CleanInvocationWork, target cleanInvocationWorkTarget) bool {
	if token.DataType == string(workerexecution.DataTypeResource) {
		return false
	}
	if token.WorkID != target.WorkID || token.WorkTypeID != target.WorkTypeName {
		return false
	}
	return token.StateCategory == string(factoryruntime.StateCategoryTerminal)
}

func cleanInvocationSuccessFromToken(token factoryruntime.CleanInvocationWork) cleanInvocationSuccess {
	return cleanInvocationSuccess{
		Output:       token.Output,
		WorkID:       token.WorkID,
		WorkTypeName: token.WorkTypeID,
		TraceID:      token.TraceID,
		SessionID:    defaultFactorySessionID,
	}
}

func writeCleanInvocationSuccess(cfg RunConfig, result cleanInvocationSuccess) error {
	output := cfg.Output
	if output == nil {
		return fmt.Errorf("clean invocation output is required")
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
	completion factoryruntime.CleanInvocationDispatch,
	target cleanInvocationWorkTarget,
) bool {
	for _, token := range completion.Consumed {
		if token.WorkID == target.WorkID && token.WorkTypeID == target.WorkTypeName {
			return true
		}
	}
	for _, token := range completion.Outputs {
		if token.WorkID == target.WorkID && token.WorkTypeID == target.WorkTypeName {
			return true
		}
	}
	return false
}
