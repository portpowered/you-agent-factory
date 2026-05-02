---
author: ralph agent
last modified: 2026, april, 21
doc-id: AGF-DOC-004
---

# Record And Replay A Run

After completing this guide, you will be able to save an Agent Factory run as one event timeline, replay it later, and use event ids and ticks to understand any divergence.

If you are replaying or inspecting older artifacts, read [Migrate event type names](event-vocabulary-migration.md) before updating stored event filters or fixtures.

## Prerequisites

- The `agent-factory` binary installed or built from `libraries/agent-factory`.
- A factory directory that can run with local mock workers or your configured provider.
- An artifact path outside any watched input directory.

## Record A Run

1. Start the factory with `--record`.

```bash
cd libraries/agent-factory
agent-factory run --dir ./examples/basic/factory --with-mock-workers --record ./tmp/basic-run.replay.json
```

2. Let the run reach the behavior you want to keep.

3. Keep the artifact at `./tmp/basic-run.replay.json`.

The artifact stores:

- `schemaVersion`, which identifies the replay artifact schema.
- `recordedAt`, which records when the artifact was written.
- `events`, an ordered array of `FactoryEvent` messages.
- `RUN_REQUEST.payload.factory`, the generated `Factory` config payload replay uses when the original factory files are unavailable.

The `events` array is the same customer-visible timeline served by `/events`: `RUN_REQUEST`, `INITIAL_STRUCTURE_REQUEST`, `WORK_REQUEST`, `RELATIONSHIP_CHANGE_REQUEST` when batch relations exist, `DISPATCH_REQUEST`, `INFERENCE_REQUEST`, `INFERENCE_RESPONSE`, `DISPATCH_RESPONSE`, `FACTORY_STATE_RESPONSE`, and `RUN_RESPONSE` where those events occur in the run.

The `RUN_REQUEST` event embeds the effective runtime configuration used by replay. Current artifacts store each workstation once in the canonical `workstations` map with canonical `behavior` and runtime executor `type`; replay does not depend on a separate `workstation_configs` map.

Recordings can contain rendered inference prompts, payloads, command output, provider response text, provider metadata, and environment key names or redaction markers. Treat them as sensitive files. The prompt in `INFERENCE_REQUEST.payload.prompt` is intentional so operators can debug the exact provider attempt, but replay fixtures must still avoid raw command stdin and raw environment values in dispatch diagnostics.

## Replay The Recording

1. Run replay mode with the artifact path.

```bash
cd libraries/agent-factory
agent-factory run --replay ./tmp/basic-run.replay.json
```

2. Do not pass `--record` with `--replay`.

Replay uses the recorded timeline to re-run the factory. It rebuilds runtime config from `RUN_REQUEST.payload.factory`, submits work from `WORK_REQUEST` events, checks dispatches against `DISPATCH_REQUEST` events, ignores `INFERENCE_REQUEST` and `INFERENCE_RESPONSE` for deterministic side-effect delivery, delivers completed work from `DISPATCH_RESPONSE` events, and compares current behavior with the recorded event ids and ticks.

## Read A Divergence

Replay stops when the current run no longer matches the recorded timeline. The error identifies the first event where behavior diverged.

```text
replay divergence: category=dispatch_mismatch tick=3 dispatch_id=dispatch-process-001 expected_event_id=evt-dispatch-001 expected="worker=copy-editor workstation=doc-processor" observed="worker=reviewer workstation=doc-review"
```

Use the reported `expected_event_id` and `tick` to find the recorded event, then compare it with the current run's observed event.

Common categories are:

| Category | Meaning |
|----------|---------|
| `missing_dispatch` | A dispatch expected from the recorded timeline did not occur before replay advanced past that tick. |
| `dispatch_mismatch` | A current dispatch differed from the recorded dispatch. |
| `unknown_completion` | The artifact contains a completion for a dispatch the current run did not create. |
| `side_effect_mismatch` | A provider or command side effect could not be matched to the recorded behavior. |
| `config_mismatch` | Recorded configuration metadata differs from the current checkout. Replay may continue when the artifact contains enough embedded configuration. |

## Inspect The Same Run In The Dashboard

The dashboard reads the same event timeline as replay. Start a factory with the HTTP API enabled, then open:

```text
http://127.0.0.1:7437/dashboard/ui
```

When investigating a replay failure, compare dashboard work ids and dispatch ids with the recorded event ids instead of treating the dashboard as a separate source of truth.

## Common Errors

| Error | Cause | Resolution |
|-------|-------|------------|
| `cannot use --record with --replay` | The command asked the factory to record and replay at the same time. | Run either record mode or replay mode, not both in one process. |
| `unsupported replay artifact schemaVersion` | The artifact schema is not supported by the current binary. | Record a fresh artifact with the current binary or use a migrated fixture. |
| The artifact changes while the run is still active | Record mode streams recoverable writes during long runs. | Wait for normal shutdown before treating the artifact as final. |

## Next Steps

- [Migrate event type names](event-vocabulary-migration.md)
- [Understand a run timeline](run-timeline.md)
- [Author workflows](authoring-workflows.md)
- [Live dashboard](development/live-dashboard.md)
