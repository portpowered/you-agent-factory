package testutil

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

const providerErrorSmokePollInterval = 50 * time.Millisecond

// ProviderErrorSmokeOutcome captures the normalized observable state that
// provider-error smoke tests assert after a lane fails or requeues.
type ProviderErrorSmokeOutcome struct {
	Work         ProviderErrorSmokeWork
	FinalPlaceID string
	Token        interfaces.Token
	Dispatches   []interfaces.CompletedDispatch
	EngineState  *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
}

// ProviderErrorPauseIsolationOutcome captures the observable state proving that
// one throttled provider/model lane requeued without blocking another lane.
type ProviderErrorPauseIsolationOutcome struct {
	ThrottledLane  ProviderErrorSmokeOutcome
	UnaffectedLane ProviderErrorSmokeOutcome
	EngineState    *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
}

// WaitForThrottleRequeue waits until the seeded work requeues to its init lane
// after a throttled provider result pauses that provider/model lane.
func (h *ProviderErrorSmokeHarness) WaitForThrottleRequeue(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	work ProviderErrorSmokeWork,
	timeout time.Duration,
) ProviderErrorSmokeOutcome {
	t.Helper()

	return WaitForProviderErrorThrottleRequeue(t, serviceHarness, work, timeout)
}

// WaitForRetryableRequeue waits until the seeded work has a completed failed
// dispatch whose output mutations recreated the work in its initial place.
func (h *ProviderErrorSmokeHarness) WaitForRetryableRequeue(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	work ProviderErrorSmokeWork,
	timeout time.Duration,
) ProviderErrorSmokeOutcome {
	t.Helper()

	return WaitForProviderErrorRetryableRequeue(t, serviceHarness, work, timeout)
}

// WaitForProviderErrorThrottleRequeue waits until the seeded work requeues to
// its init lane after a throttled provider result pauses that provider/model lane.
func WaitForProviderErrorThrottleRequeue(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	work ProviderErrorSmokeWork,
	timeout time.Duration,
) ProviderErrorSmokeOutcome {
	t.Helper()

	return waitForProviderErrorOutcome(
		t,
		serviceHarness,
		work,
		work.WorkTypeID+":init",
		timeout,
		providerErrorThrottleRequeueMatches,
	)
}

// WaitForProviderErrorRetryableRequeue observes the durable dispatch-history
// requeue signal for retryable provider failures. Unlike throttled failures,
// non-throttled retryable failures may be redispatched immediately, so the live
// marking is not guaranteed to pause in the initial place long enough to poll.
func WaitForProviderErrorRetryableRequeue(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	work ProviderErrorSmokeWork,
	timeout time.Duration,
) ProviderErrorSmokeOutcome {
	t.Helper()

	finalPlaceID := work.WorkTypeID + ":init"
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		engineState, err := serviceHarness.GetEngineStateSnapshot()
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot() error = %v", err)
		}
		dispatches := providerErrorDispatchesForWork(engineState.DispatchHistory, work.WorkID)
		dispatch, token, ok := providerErrorRequeueDispatch(dispatches, finalPlaceID, work.WorkID)
		if !ok {
			time.Sleep(providerErrorSmokePollInterval)
			continue
		}

		return ProviderErrorSmokeOutcome{
			Work:         work,
			FinalPlaceID: finalPlaceID,
			Token:        token,
			Dispatches:   []interfaces.CompletedDispatch{dispatch},
			EngineState:  engineState,
		}
	}

	snap := serviceHarness.Marking()
	engineState, err := serviceHarness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("timed out waiting for provider-error retryable requeue for %s; marking=%+v; GetEngineStateSnapshot() error=%v", work.WorkID, snap.PlaceTokens, err)
	}
	t.Fatalf(
		"timed out waiting for provider-error retryable requeue for %s; marking=%+v; dispatch_history=%+v",
		work.WorkID,
		snap.PlaceTokens,
		engineState.DispatchHistory,
	)
	return ProviderErrorSmokeOutcome{}
}

// WaitForFailedAfterBoundedRetries waits until the seeded work reaches its
// failed place after exhausting the expected number of provider attempts.
func (h *ProviderErrorSmokeHarness) WaitForFailedAfterBoundedRetries(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	work ProviderErrorSmokeWork,
	timeout time.Duration,
) ProviderErrorSmokeOutcome {
	t.Helper()

	return WaitForProviderErrorFailedAfterBoundedRetries(t, serviceHarness, work, timeout)
}

// WaitForProviderErrorFailedAfterBoundedRetries waits until the seeded work
// reaches its failed place after exhausting provider retries.
func WaitForProviderErrorFailedAfterBoundedRetries(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	work ProviderErrorSmokeWork,
	timeout time.Duration,
) ProviderErrorSmokeOutcome {
	t.Helper()

	return waitForProviderErrorOutcome(
		t,
		serviceHarness,
		work,
		work.WorkTypeID+":failed",
		timeout,
		func(token interfaces.Token, dispatches []interfaces.CompletedDispatch) bool {
			if len(dispatches) == 0 {
				return false
			}
			last := dispatches[len(dispatches)-1]
			return dispatchProducesWorkTokenInPlace(last, token.PlaceID, token.Color.WorkID)
		},
	)
}

// WaitForPauseIsolation waits until the throttled lane requeues to init while
// the unaffected lane reaches complete in the same running factory.
func (h *ProviderErrorSmokePauseIsolationHarness) WaitForPauseIsolation(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	throttledWork ProviderErrorSmokeWork,
	unaffectedWork ProviderErrorSmokeWork,
	timeout time.Duration,
) ProviderErrorPauseIsolationOutcome {
	t.Helper()

	return WaitForProviderErrorPauseIsolation(t, serviceHarness, throttledWork, unaffectedWork, timeout)
}

// WaitForProviderErrorPauseIsolation waits until the throttled lane requeues to
// init while the unaffected lane reaches complete.
func WaitForProviderErrorPauseIsolation(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	throttledWork ProviderErrorSmokeWork,
	unaffectedWork ProviderErrorSmokeWork,
	timeout time.Duration,
) ProviderErrorPauseIsolationOutcome {
	t.Helper()

	deadline := time.Now().Add(timeout)
	throttledPlaceID := throttledWork.WorkTypeID + ":init"
	unaffectedPlaceID := unaffectedWork.WorkTypeID + ":complete"

	for time.Now().Before(deadline) {
		marking := serviceHarness.Marking()
		throttledToken, throttledFound := findProviderErrorToken(marking, throttledPlaceID, throttledWork.WorkID)
		unaffectedToken, unaffectedFound := findProviderErrorToken(marking, unaffectedPlaceID, unaffectedWork.WorkID)
		if !throttledFound || !unaffectedFound {
			time.Sleep(providerErrorSmokePollInterval)
			continue
		}

		engineState, err := serviceHarness.GetEngineStateSnapshot()
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot() error = %v", err)
		}
		throttledDispatches := providerErrorDispatchesForWork(engineState.DispatchHistory, throttledWork.WorkID)
		unaffectedDispatches := providerErrorDispatchesForWork(engineState.DispatchHistory, unaffectedWork.WorkID)
		if !providerErrorThrottleRequeueMatches(throttledToken, throttledDispatches) {
			time.Sleep(providerErrorSmokePollInterval)
			continue
		}
		if !providerErrorCompletedMatches(unaffectedDispatches) {
			time.Sleep(providerErrorSmokePollInterval)
			continue
		}

		return ProviderErrorPauseIsolationOutcome{
			ThrottledLane: ProviderErrorSmokeOutcome{
				Work:         throttledWork,
				FinalPlaceID: throttledPlaceID,
				Token:        throttledToken,
				Dispatches:   throttledDispatches,
				EngineState:  engineState,
			},
			UnaffectedLane: ProviderErrorSmokeOutcome{
				Work:         unaffectedWork,
				FinalPlaceID: unaffectedPlaceID,
				Token:        unaffectedToken,
				Dispatches:   unaffectedDispatches,
				EngineState:  engineState,
			},
			EngineState: engineState,
		}
	}

	snap := serviceHarness.Marking()
	t.Fatalf(
		"timed out waiting for provider-error pause isolation between %s and %s; marking=%+v",
		throttledWork.WorkID,
		unaffectedWork.WorkID,
		snap.PlaceTokens,
	)
	return ProviderErrorPauseIsolationOutcome{}
}

func waitForProviderErrorOutcome(
	t *testing.T,
	serviceHarness *ServiceTestHarness,
	work ProviderErrorSmokeWork,
	finalPlaceID string,
	timeout time.Duration,
	matches func(token interfaces.Token, dispatches []interfaces.CompletedDispatch) bool,
) ProviderErrorSmokeOutcome {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		marking := serviceHarness.Marking()
		token, found := findProviderErrorToken(marking, finalPlaceID, work.WorkID)
		if !found {
			time.Sleep(providerErrorSmokePollInterval)
			continue
		}

		engineState, err := serviceHarness.GetEngineStateSnapshot()
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot() error = %v", err)
		}
		dispatches := providerErrorDispatchesForWork(engineState.DispatchHistory, work.WorkID)
		if !matches(token, dispatches) {
			time.Sleep(providerErrorSmokePollInterval)
			continue
		}

		return ProviderErrorSmokeOutcome{
			Work:         work,
			FinalPlaceID: finalPlaceID,
			Token:        token,
			Dispatches:   dispatches,
			EngineState:  engineState,
		}
	}

	snap := serviceHarness.Marking()
	engineState, err := serviceHarness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("timed out waiting for provider-error outcome in %s for %s; marking=%+v; GetEngineStateSnapshot() error=%v", finalPlaceID, work.WorkID, snap.PlaceTokens, err)
	}
	t.Fatalf(
		"timed out waiting for provider-error outcome in %s for %s; marking=%+v; dispatch_history=%+v",
		finalPlaceID,
		work.WorkID,
		snap.PlaceTokens,
		engineState.DispatchHistory,
	)
	return ProviderErrorSmokeOutcome{}
}

func providerErrorThrottleRequeueMatches(
	token interfaces.Token,
	dispatches []interfaces.CompletedDispatch,
) bool {
	if len(token.History.FailureLog) != 1 {
		return false
	}
	return len(dispatches) == 1 && dispatches[0].Outcome == interfaces.OutcomeFailed
}

func providerErrorRequeueDispatch(
	dispatches []interfaces.CompletedDispatch,
	placeID string,
	workID string,
) (interfaces.CompletedDispatch, interfaces.Token, bool) {
	for _, dispatch := range dispatches {
		if dispatch.Outcome != interfaces.OutcomeFailed {
			continue
		}
		for _, mutation := range dispatch.OutputMutations {
			if mutation.ToPlace != placeID || mutation.Token == nil {
				continue
			}
			if mutation.Token.Color.WorkID != workID {
				continue
			}
			return dispatch, *mutation.Token, true
		}
	}
	return interfaces.CompletedDispatch{}, interfaces.Token{}, false
}

func providerErrorCompletedMatches(dispatches []interfaces.CompletedDispatch) bool {
	if len(dispatches) == 0 {
		return false
	}
	return dispatches[len(dispatches)-1].Outcome == interfaces.OutcomeAccepted
}

func findProviderErrorToken(marking *petri.MarkingSnapshot, placeID, workID string) (interfaces.Token, bool) {
	for _, token := range marking.Tokens {
		if token.PlaceID == placeID && token.Color.WorkID == workID {
			return *token, true
		}
	}
	return interfaces.Token{}, false
}

func providerErrorDispatchesForWork(history []interfaces.CompletedDispatch, workID string) []interfaces.CompletedDispatch {
	dispatches := make([]interfaces.CompletedDispatch, 0, len(history))
	for _, dispatch := range history {
		if dispatchConsumesWork(dispatch, workID) {
			dispatches = append(dispatches, dispatch)
		}
	}
	return dispatches
}

func dispatchConsumesWork(dispatch interfaces.CompletedDispatch, workID string) bool {
	for _, token := range dispatch.ConsumedTokens {
		if token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func dispatchProducesWorkTokenInPlace(dispatch interfaces.CompletedDispatch, placeID, workID string) bool {
	for _, mutation := range dispatch.OutputMutations {
		if mutation.ToPlace != placeID || mutation.Token == nil {
			continue
		}
		if mutation.Token.Color.WorkID == workID {
			return true
		}
	}
	return false
}
