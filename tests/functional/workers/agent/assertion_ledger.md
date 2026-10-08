# MAP-001 agent and concurrency assertion ledger

This ledger identifies the retained customer behavior after shared-process
consolidation. Constructor-only provider inventory and deliberately failing
test-executable cleanup probes are removed from the functional lane. Public
provider validation, cancellation, session deletion and recovery remain.

| Row | Current witness | Current assertion intent | Post-migration witness | Owner/status |
| --- | --- | --- | --- | --- |
| 2 | `TestBuildProcessExecutesModelWorkerThroughConvergedWorkersService` | One controlled provider call produces one done Work, no failed Work, and an accepted `process` dispatch containing the exact output marker. | `TestAgentSharedProcess/Codex` | Story `...-001` |
| 3 | `TestBuildProcessResolvesRegisteredAgentThroughProviders` | Registered-agent selection reaches the controlled provider once and preserves done/failed Work counts plus accepted dispatch output. | `TestAgentSharedProcess/Registered` | Story `...-001` |
| 4 | `TestBuildProcessExecutesProviderAttemptThroughRuntimeRoot` | Runtime-root execution reaches the controlled provider once, preserves one done/zero failed Work, and proves the public Factory Session stream, Worker Session attempt, response Run/Event, dispatch, request, and Work correlations. | `TestAgentSharedProcess/RuntimeRoot` | Story `...-001` |
| 5 | `TestFactoryRuntimeConcurrentSessionsShareWorkersWithoutCancellationLeakage` | Two Factory Sessions overlap with distinct prompt/correlation identity; canceling one does not leak into the survivor, which later completes, and active calls return to zero. | `TestConcurrencySharedProcess/Concurrent` | Story `...-003` |

Explicit session identity, immutable command-route selection, text input lineage
and customer-visible Work and event outcomes remain. The Agent journey runs in
parallel with other packages; its adverse cases precede Recovery on the same
host so the latter continues to prove recovery after failure and cancellation.

## Additive agent matrix witnesses

These checks cover the customer-facing Agent behavior matrix.

| Matrix case | Observable assertion | Post-migration witness | Owner/status |
| --- | --- | --- | --- |
| AG-05 | Claude selection reaches the controlled Claude route once and preserves accepted Work/output without a Codex route. | `TestAgentSharedProcess/Claude` | Story `...-002` |
| AG-06 | Unknown provider fails before session/dispatch/provider effects with an actionable diagnostic. | `TestAgentSharedProcess/Invalid/UnknownProvider` | Story `...-002` |
| AG-07 | Missing Worker reference fails isolated validation before session/dispatch/provider effects. | `TestAgentSharedProcess/Invalid/MalformedConfiguration` | Story `...-002` |
| AG-08 | Characterize the current empty Work behavior: the no-content request returns HTTP 201 with `accepted=true` and request/Work identity, then a later valid request succeeds in the same explicit session. | `TestAgentSharedProcess/Empty` | Story `...-002` |
| AG-09 | Minimum single-part Work produces one Work/dispatch/attempt with the exact input marker and no duplicate. | `TestAgentSharedProcess/Minimum` | Story `...-002` |
| AG-10 | Typed provider failure produces the current failed Work/dispatch classification without fallback and leaves the session closable. | `TestAgentSharedProcess/Failure` | Story `...-002` |
| AG-11 | Deterministic timeout produces terminal timeout observations with retries on the same immutable route, no fallback, and zero active calls. | `TestAgentSharedProcess/Timeout` | Story `...-002` |
| AG-12 | Canceling the held call produces the current terminal response-stream cancellation diagnostic and no active provider call. | `TestAgentSharedProcess/Cancel` | Story `...-002` |
| AG-13 | A fresh explicit session accepts clean Codex input/output after adverse cases without a prior marker. | `TestAgentSharedProcess/Recovery` | Story `...-002` |
