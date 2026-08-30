# MAP-001 agent and concurrency assertion ledger

This ledger freezes the five existing executable rows before the shared-process
restructure. The row identity and current assertion intent come from the
checked-in C01 inventory. The post-migration witness names are the contract for
the retained behavior; the concurrency row remains owned by story `...-003`.

| Row | Current witness | Current assertion intent | Post-migration witness | Owner/status |
| --- | --- | --- | --- | --- |
| 1 | `TestAgentRunnerDeliveryRemainsInertThroughRootBuildProcessConstruction` | A root-built process is non-nil and exposes canonical `claude`/`codex` identities while unknown providers fail before runtime effects. | `TestAgentSharedProcess/Inert` | Story `...-001` |
| 2 | `TestBuildProcessExecutesModelWorkerThroughConvergedWorkersService` | One controlled provider call produces one done Work, no failed Work, and an accepted `process` dispatch containing the exact output marker. | `TestAgentSharedProcess/Codex` | Story `...-001` |
| 3 | `TestBuildProcessResolvesRegisteredAgentThroughProviders` | Registered-agent selection reaches the controlled provider once and preserves done/failed Work counts plus accepted dispatch output. | `TestAgentSharedProcess/Registered` | Story `...-001` |
| 4 | `TestBuildProcessExecutesProviderAttemptThroughRuntimeRoot` | Runtime-root execution reaches the controlled provider once, preserves one done/zero failed Work, and proves the public Factory Session stream, Worker Session attempt, response Run/Event, dispatch, request, and Work correlations. | `TestAgentSharedProcess/RuntimeRoot` | Story `...-001` |
| 5 | `TestFactoryRuntimeConcurrentSessionsShareWorkersWithoutCancellationLeakage` | Two Factory Sessions overlap with distinct prompt/correlation identity; canceling one does not leak into the survivor, which later completes, and active calls return to zero. | `TestConcurrencySharedProcess/Concurrent` | Story `...-003` |

No row is deleted by the migration. New checks for explicit session identity,
immutable route selection, text input lineage, and one shared process are
additive witnesses around rows 1–4.

## Additive agent matrix witnesses

These checks expand the requested AG-05 through AG-14 behavior matrix without
reclassifying the five retained executable rows above.

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
| AG-14 | Intentional child failure remains visible while cleanup reports deleted sessions, closed stream/process/listener, zero active route/call, and absent roots. | `TestAgentSharedProcess/Cleanup` | Story `...-002` |
