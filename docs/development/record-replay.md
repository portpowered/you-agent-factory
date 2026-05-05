---
author: ralph agent
last modified: 2026, april, 21
doc-id: AGF-DOC-002
---

# Agent Factory Record And Replay

This document is the canonical maintainer guide for recording an Agent Factory run, replaying the artifact, interpreting replay outcomes, and promoting a recording into a regression test. For customer-facing usage and event examples, see [Record and replay a run](../record-replay.md) and [Understand a run timeline](../run-timeline.md).

## What It Covers

Record/replay captures factory-level runtime behavior so maintainers can reproduce customer failures without the customer's original local files or live provider and script side effects.

Use this guide when you need to:

- collect a replay artifact from `agent-factory run`
- replay an artifact from embedded configuration
- interpret replay success, metadata warnings, and divergence failures
- promote a recording into a durable test through `pkg/testutil`

## Artifact Contents

Record mode writes a versioned JSON artifact owned by `pkg/replay`. The current schema version is `agent-factory.replay.v1`.

A recording includes:

- `schemaVersion` and `recordedAt`
- `events`, an ordered array of generated `FactoryEvent` messages
- generated Factory configuration on `RUN_REQUEST.payload.factory`, including the normalized `factory.json`, worker definitions, workstation definitions, source directory, optional workflow ID, and config hash metadata
- work submissions as `WORK_REQUEST` events with the logical tick where the engine observed each request
- dispatch expectations as `DISPATCH_REQUEST` events with dispatch ID, transition identity, input work, resources, and event context
- completion delivery as `DISPATCH_RESPONSE` events with completion ID, dispatch ID, outcome, output, diagnostics, and the logical tick where the engine observed the result
- optional wall-clock metadata on run lifecycle events for investigation

The artifact does not persist top-level work request, submission, dispatch, or completion arrays. Runtime replay hooks reduce the generated event log instead.

Nested diagnostics are preserved on completed results when workers provide them. Provider diagnostics can include rendered prompt metadata and provider request/response metadata. Script diagnostics can include command input, stdin, arguments, environment projection, stdout, stderr, exit code, timeout, and panic details.

Command diagnostics do not preserve the raw full subprocess environment. Script workers and provider subprocesses use the shared `workers.ProjectCommandEnvForDiagnostics(...)` policy before diagnostics can be attached to a `WorkResult` and written to a replay artifact.

The command environment projection preserves:

- `env_count`, the number of valid environment entries supplied to the subprocess
- `env_keys`, the sorted set of environment key names
- raw values only for explicitly allowlisted low-risk diagnostic keys, such as `CI`, `GOOS`, `GOARCH`, `TERM`, and provider automation defaults like `GIT_TERMINAL_PROMPT`
- `<redacted>` for sensitive-looking key names, including names containing `TOKEN`, `SECRET`, `PASSWORD`, `PASS`, `KEY`, `CREDENTIAL`, `AUTH`, `ANTHROPIC`, `OPENAI`, or `GEMINI`
- `<metadata-only>` for non-allowlisted keys that are useful to identify by name but whose values should not be persisted

The focused regression proof for this policy is:

```bash
cd libraries/agent-factory
go test ./pkg/workers -run "TestProjectCommandEnvForDiagnostics_PreservesSafeMetadataAndRedactsSecrets|TestScriptExecutor_CommandDiagnosticsRedactSensitiveEnvWithoutChangingExecution|TestScriptWrapProvider_Infer_CommandDiagnosticsRedactSensitiveEnvWithoutChangingExecution" -count=1
```

These tests cover the projection and both subprocess diagnostic producers before the record/replay recorder can persist completed result diagnostics.

Recordings are not fully redacted. Treat replay artifacts as sensitive because they can contain prompts, payloads, stdout, stderr, provider request/response metadata, provider session metadata, and command environment key names or redaction markers.

## Record A Run

1. Choose an artifact path outside any directory that the factory watches for inputs.

   ```bash
   cd libraries/agent-factory
   agent-factory run --dir ./factory --record ./tmp/customer-run.replay.json
   ```

2. Let the run reach the failure or terminal behavior you need to preserve.

3. Keep the final artifact and any temporary artifact left beside it if the process was interrupted during a write. Record mode streams dirty artifacts during execution and performs a final flush on normal shutdown.

The default streaming flush interval is `250ms` inside the service recorder. The CLI currently uses that default.

## Replay A Recording

1. Run replay mode with the artifact path.

   ```bash
   cd libraries/agent-factory
   agent-factory run --replay ./tmp/customer-run.replay.json
   ```

2. Do not pass `--record` with `--replay`. The service rejects that combination before runtime startup.

Replay mode loads the generated `Factory` payload from the `RUN_REQUEST` event instead of requiring the original `factory.json`, worker `AGENTS.md`, workstation `AGENTS.md`, or input files to still exist. Current artifacts must include that `RUN_REQUEST.payload.factory` config; replay does not load a second config shape beside the generated event contract. The service installs replay-aware provider and command-runner implementations through the normal worker interfaces, then derives submissions from `WORK_REQUEST`, dispatch expectations from `DISPATCH_REQUEST`, and completions from `DISPATCH_RESPONSE` through the production-style engine, worker-pool, and result-ingestion path.

Replay timing is logical-tick based. Recorded wall-clock timestamps and physical durations are retained as diagnostics only; they do not decide when a completion becomes visible to the engine.

## Initial Topology Projection

Live event history and replay-compatible `/events` paths must use the same `INITIAL_STRUCTURE_REQUEST` projection contract:

1. Build the Petri net from the effective `config.FactoryConfig` with `service.ConfigMapper`.
2. Build runtime metadata from the already-loaded runtime config in live mode, or from the `RUN_REQUEST` generated `Factory` payload with `replay.RuntimeConfigFromGeneratedFactory(...)` in replay mode.
3. Call `factory.ProjectInitialStructure(net, runtimeConfig)` and stream or record that payload as the canonical `factory.InitialStructurePayload`.

Replay projection must not reread the original factory `AGENTS.md` files when the serialized Factory payload is present. The event payload is authoritative for worker provider/model metadata, workstation metadata, and runtime constraints. If artifact metadata differs from the current checkout's loadable config, report a warning and keep projecting from the artifact's serialized Factory.

The focused contract proof is:

```bash
cd libraries/agent-factory
go test ./pkg/replay -run TestRuntimeConfigFromGeneratedFactory_ProjectsReplayInitialTopologyFromFactory -count=1
```

## Interpret Outcomes

### Success

Replay success means the current runtime observed the recorded submissions, dispatches, side effects, and completions without material divergence. The replay should reach the same terminal state that the artifact describes.

### Metadata Warnings

Replay may log `replay artifact metadata differs from current checkout` with category `config_mismatch`. This warning means the artifact's recorded config hash metadata differs from the current checkout's loadable local config.

Replay still runs from the embedded artifact config. Treat the warning as investigation context rather than replay failure.

### Divergence

Replay stops with `replay divergence` when current behavior no longer represents the recorded run. The structured report includes the mismatch category, logical tick, dispatch ID when available, expected event ID, observed event ID when available, expected event summary, and observed event summary.

Current divergence categories include:

- `missing_dispatch`: a recorded dispatch was not observed before replay advanced past its recorded tick
- `dispatch_mismatch`: transition identity, worker/workstation identity, execution metadata, or consumed token lineage differed
- `unknown_completion`: the artifact contains a completion for a dispatch the runtime did not create
- `side_effect_mismatch`: a provider or command-runner replay request could not be matched to recorded side-effect behavior
- `config_mismatch`: config metadata differs from the current checkout; this is logged as a warning when replay can still proceed from embedded config

When replay diverges, keep the artifact and the divergence report together. The report is the first place to look for the earliest material mismatch.

## Promote A Recording Into A Regression Test

Use `pkg/testutil` so regression tests exercise service replay mode, embedded config loading, replay side effects, worker-pool delivery, and structured divergence reporting.

1. Commit the replay artifact under the relevant `testdata` directory after any required manual review for sensitive content.

2. Load and validate the artifact in the test.

   ```go
   artifact := testutil.LoadReplayArtifact(t, artifactPath)
   if countReplayEvents(artifact.Events, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
   	t.Fatal("expected replay fixture artifact to contain dispatches")
   }
   ```

3. Assert successful replay for the customer regression.

   ```go
   h := testutil.AssertReplaySucceeds(t, artifactPath, 10*time.Second)
   h.Service.Assert().HasTokenInPlace("task:complete")
   ```

4. For intentional mismatch coverage, mutate a copied artifact and assert structured divergence from the owning replay contract test or replay-lane helper rather than expanding shared `pkg/testutil` with a replay-only wrapper.

   ```go
   h := testutil.NewReplayHarness(t, divergentPath)
   err := h.RunUntilComplete(10 * time.Second)
   var divergence *replay.DivergenceError
   if !errors.As(err, &divergence) {
   	t.Fatalf("error = %v, want replay divergence", err)
   }
   report := divergence.Report
   if report.Category != replay.DivergenceCategoryDispatchMismatch {
   	t.Fatalf("category = %q", report.Category)
   }
   ```

See `tests/functional_test/replay_regression_harness_test.go` for the current production-style harness pattern.

## Manual Smoke Flow

The adhoc fixture has an opt-in record/replay smoke test for local investigation:

```bash
cd libraries/agent-factory
AGENT_FACTORY_ADHOC_RECORD_REPLAY=1 go test -v ./tests/adhoc -run TestAdHocRecordReplaySmoke -count=1
```

Set `AGENT_FACTORY_ADHOC_ARTIFACT=<path>` to keep the generated artifact at a known path. Set `AGENT_FACTORY_ADHOC_DIR=<factory-dir>` to run the smoke flow against another compatible fixture.

## Related Code

- `pkg/replay` owns the artifact schema, validation, recorder, replay side effects, logical delivery planning, and divergence reports.
- `pkg/service` wires record and replay modes into runtime construction.
- `pkg/testutil/replay_harness.go` owns the reusable regression harness helpers.
- `tests/adhoc/main_adhoc_test.go` owns the opt-in manual record/replay smoke flow.

## References

- [Agent Factory intent](../../../docs/intents/agent-factory.md)
- [Library development process](../../../docs/processes/libraries-development.md)
- [Architecture](./development/architecture.md)
- [Adhoc fixture README](../tests/adhoc/factory/README.md)

## Changelog

- 2026-04-16 - Documented replay-compatible `INITIAL_STRUCTURE_REQUEST` topology projection from embedded runtime config.
- 2026-04-12 - Documented the shared command environment diagnostic projection used before replay artifact persistence.
- 2026-04-10 - Initial maintainer guide for record/replay collection, replay interpretation, and regression promotion.
