# Dispatcher Throttle Pause Audit Closeout

## Scope

This closeout records the `US-001` audit for dispatcher-owned provider/model
throttle pause handling. The audit stays scoped to the current runtime-memory
pause model in `pkg/factory/subsystems/subsystem_dispatcher.go` and does not
propose any `INFERENCE_THROTTLE_GUARD` redesign.

## Evidence

- `DispatcherSubsystem.Execute` reconciles throttle pauses, derives enabled
  transitions, filters paused enabled transitions, and only then calls
  `d.sched.Select(enabled, d.schedulerSnapshot(snapshot))`.
- `filterPausedEnabledTransitions` is the first throttle-specific gate applied
  to the enabled transition set before scheduler selection.
- `filterPausedDecisions` applies the same provider/model pause lookup again,
  but only after scheduler selection has already completed.
- The scheduler interface contract is `Select(enabled []interfaces.EnabledTransition, snapshot ...) []interfaces.FiringDecision`,
  so schedulers only receive the dispatcher-filtered `enabled` slice.
- The concrete schedulers used by the dispatcher derive decisions directly from
  the provided `enabled` slice:
  - `pkg/factory/scheduler/fifo.go` iterates `for _, et := range enabled` and
    appends decisions from those entries only.
  - `pkg/factory/scheduler/work_queue.go` iterates `for _, et := range enabled`,
    builds candidates from those entries only, and appends decisions from those
    candidates only.
- The focused dispatcher tests already cover the behavior the cleanup must
  preserve:
  - `TestDispatcher_ThrottlePauseObservedWhenCronTransitionPausedBeforeScheduling`
    proves pause creation still surfaces runtime observability when a paused lane
    is filtered before scheduling.
  - `TestDispatcher_ThrottlePauseExcludesPausedLaneBeforeSchedulingSharedResource`
    proves the scheduler sees only the healthy competing lane and that the
    healthy lane still dispatches.
  - `TestDispatcher_ThrottlePauseExpiresAndAllowsDispatchAgain` and
    `TestDispatcher_ExpiredThrottlePauseObservedWhenSchedulerReturnsNoDecisions`
    cover pause expiry observability paths.

## Conclusion

Under the current scheduler contract and implementations, scheduler decisions
can only originate from the already-filtered enabled transition set passed into
`Select`. That makes the post-scheduler `filterPausedDecisions` path redundant
for provider/model throttle enforcement today.

The safe assumption behind that conclusion is narrow and explicit: dispatcher
schedulers must continue treating the `enabled` slice passed into `Select` as
their only transition input and must not synthesize decisions for transitions
outside that slice. If that assumption changes in the future, the scheduler
contract should be updated deliberately rather than preserved implicitly through
duplicate dispatcher pause filtering.
