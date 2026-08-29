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
| 4 | `TestBuildProcessExecutesProviderAttemptThroughRuntimeRoot` | Runtime-root execution reaches the controlled provider once and preserves one done/zero failed Work. | `TestAgentSharedProcess/RuntimeRoot` | Story `...-001` |
| 5 | `TestFactoryRuntimeConcurrentSessionsShareWorkersWithoutCancellationLeakage` | Two Factory Sessions overlap with distinct prompt/correlation identity; canceling one does not leak into the survivor, which later completes, and active calls return to zero. | `TestConcurrencySharedProcess/Concurrent` | Story `...-003` |

No row is deleted by the migration. New checks for explicit session identity,
immutable route selection, text input lineage, and one shared process are
additive witnesses around rows 1–4.
