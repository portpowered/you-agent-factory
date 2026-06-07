package service

import (
	"context"
	"time"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
)

type invocationWaitTickResult struct {
	done          bool
	result        apisurface.FactoryInvocationResult
	err           error
	loggedLoading bool
}

func invocationWaitContext(ctx context.Context, timeoutMillis *int64) (context.Context, context.CancelFunc) {
	if timeoutMillis != nil && *timeoutMillis > 0 {
		return context.WithTimeout(ctx, time.Duration(*timeoutMillis)*time.Millisecond)
	}
	return ctx, func() {}
}

func (fs *FactoryService) isPackagedTTSSession(sessionID string) bool {
	runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
	return err == nil && runtimeCfg != nil && tts.IsPackagedFactory(runtimeCfg.FactoryConfig())
}

func (fs *FactoryService) processInvocationWaitTick(
	waitCtx context.Context,
	sessionID string,
	input sessionInvocationWaitInput,
	result apisurface.FactoryInvocationResult,
	packagedTTSInvocation bool,
	loggedPackagedLoading bool,
) invocationWaitTickResult {
	snapshot, err := fs.GetEngineStateSnapshotForSession(waitCtx, sessionID)
	if err != nil {
		waitResult, waitErr := fs.handleInvocationWaitError(result, err)
		return invocationWaitTickResult{done: true, result: waitResult, err: waitErr}
	}

	worldState, err := fs.sessionInvocationWorldState(waitCtx, sessionID, snapshot.TickCount)
	if err != nil {
		waitResult, waitErr := fs.handleInvocationWaitError(result, err)
		return invocationWaitTickResult{done: true, result: waitResult, err: waitErr}
	}

	activeWork := snapshotHasActiveWork(snapshot)
	if packagedTTSInvocation && activeWork && !loggedPackagedLoading {
		fs.logPackagedTTSInvocationLoading(sessionID, input)
		loggedPackagedLoading = true
	}

	selection, selectionErr := invocations.ResolvePrimaryResult(invocations.PrimaryResultSelectionInput{
		RequestID:        input.RequestID,
		InvocationReturn: input.InvocationReturn,
		WorldState:       worldState,
	})
	if selectionErr == nil {
		if packagedTTSInvocation {
			fs.logPackagedTTSInvocationCompleted(sessionID, input, selection)
		}
		return invocationWaitTickResult{
			done:   true,
			result: fs.handleInvocationSelectionSuccess(sessionID, input, selection),
		}
	}

	primaryErr, ok := selectionErr.(*invocations.PrimaryResultError)
	if !ok {
		return invocationWaitTickResult{done: true, err: selectionErr}
	}

	if _, exists := worldState.WorkRequestsByID[input.RequestID]; exists && !activeWork {
		return invocationWaitTickResult{
			done:   true,
			result: fs.resolveInvocationWaitTerminal(sessionID, input, worldState, packagedTTSInvocation, primaryErr),
		}
	}

	return invocationWaitTickResult{loggedLoading: loggedPackagedLoading}
}

func (fs *FactoryService) resolveInvocationWaitTerminal(
	sessionID string,
	input sessionInvocationWaitInput,
	worldState interfaces.FactoryWorldState,
	packagedTTSInvocation bool,
	primaryErr *invocations.PrimaryResultError,
) apisurface.FactoryInvocationResult {
	if packagedTTSInvocation {
		if _, failure := tts.ClassifyInvocationWait(worldState, input.RequestID, false); failure != nil {
			return fs.handlePackagedTTSInvocationFailure(sessionID, input, failure)
		}
	}
	return fs.handleInvocationUnresolvedPrimary(sessionID, input, primaryErr)
}

func (fs *FactoryService) invocationWaitTimedOut(
	sessionID string,
	input sessionInvocationWaitInput,
	result apisurface.FactoryInvocationResult,
	waitErr error,
) apisurface.FactoryInvocationResult {
	terminalResult := invocationContextTerminalResult(result, waitErr)
	fs.recordInvocationMetric(invocationMetricFailure, inputMetricLabels(input.InputSource))
	fs.logInvocationTerminalResult(sessionID, input, terminalResult)
	return terminalResult
}
