package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func emitCleanInvocationOutcome(ctx context.Context, cfg RunConfig, runner factoryServiceRunner, runErr error) error {
	provider, ok := runner.(engineStateSnapshotProvider)
	if !ok {
		if runErr == nil {
			return &InvocationError{
				Code:    InvocationErrorCodeFailed,
				Message: "clean invocation result snapshot is unavailable",
			}
		}
		return newInvocationErrorForRunFailure(runErr, nil)
	}
	snapshot, err := provider.GetEngineStateSnapshot(ctx)
	if err != nil {
		if runErr == nil {
			return &InvocationError{
				Code:    InvocationErrorCodeFailed,
				Message: "clean invocation result snapshot is unavailable",
				Cause:   err,
			}
		}
		return newInvocationErrorForRunFailure(runErr, nil)
	}
	if runErr != nil {
		return newInvocationErrorForRunFailure(runErr, snapshot)
	}
	target, err := cleanInvocationWorkTargetFromFile(cfg.WorkFile)
	if err != nil {
		return err
	}
	result, ok := cleanInvocationSuccessFromSnapshot(snapshot, target)
	if !ok {
		return cleanInvocationFailureFromSnapshot(snapshot, target)
	}
	return writeCleanInvocationSuccess(cfg, result)
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
