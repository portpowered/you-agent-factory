# Record and Replay

Use record and replay when you want to capture a live factory run as a
replay-compatible artifact, locate the saved file after shutdown, or re-run
from a saved history without dispatching live workers again.

`you docs record-replay` is the canonical guide for `--record`, `--replay`, and
`--no-record`, including JavaScript-orchestrated Factory Sessions. See `you docs
javascript-workflows` for the supported JavaScript authoring contract and `you
docs authoring-factories` for the full factory setup workflow.

## Default Recording On Live Runs

Normal live `you run` invocations record a replay-compatible artifact by default
when you do not pass `--record` or `--replay`.

The generated artifact root is:

```text
~/.you-agent-factory/recordings/YYYY-MM/YYYY-MM-DD/
```

The top-level session for a normal run writes a filename shaped like:

```text
factory-session-~default-HHMMSS-<unique-id>.json
```

Independent factory sessions opened later in the same service lifetime keep the
same directory contract but replace `~default` with the owning session ID so
their histories stay isolated in separate replay artifacts.

On shutdown, the CLI reports the resolved generated path with:

```text
Recording saved: <path>
```

That message makes the artifact easy to find after a failure or an unexpected
run.

## Record Mode vs Replay Mode

**Record mode** starts a live run and writes the observed runtime history to a
replay artifact. Use the default generated path, `--record <path>`, or rely on
default recording without extra flags.

**Replay mode** reads an existing artifact and uses the recorded runtime history
instead of dispatching live workers again. Pass `--replay <path>`.

## Run Controls

| Flag | Effect |
|------|--------|
| *(none on a live run)* | Record to the generated path under `~/.you-agent-factory/recordings/` |
| `--no-record` | Skip the default recording for one invocation |
| `--record <path>` | Write the replay artifact to an explicit path you own |
| `--replay <path>` | Replay an existing artifact instead of starting a live run |

### Incompatible Combinations

These flag pairs are rejected for the same invocation:

- `--record` with `--replay`
- `--no-record` with `--record`

A flag validation failure happens before a new Factory Session starts, so there
is no new session, Dispatch, FactoryArtifact, or FactoryEvent to inspect.

## JavaScript Recording Contract

A JavaScript workflow is recorded as the same canonical `FactorySession`
history used by other orchestrators. The replay artifact is an event envelope,
not a JavaScript VM snapshot. When the corresponding fact exists, the durable
history can include:

- the Factory Session id and JavaScript orchestrator identity;
- the workflow source reference and source digest, effective policy digest, and
  argument-schema digest recorded at session start;
- ordered lifecycle, phase, checkpoint-reference, Dispatch, artifact, result,
  and terminal `FactoryEvent` summaries;
- checkpoint labels, resumability status, snapshot digests, and
  `FactoryArtifact` references;
- final, partial, or failed result availability and bounded failure detail; and
- artifact audit mode, capture metadata, and redaction counts where those facts
  were emitted by the run.

These are high-level replay facts. A recording does **not** promise raw VM
state, goja/runtime phase internals, raw checkpoint state bodies, a private
child-dispatch list, provider transcripts, or provider reasoning. Inspect live
or reconstructed work through the Factory Session, `Dispatch`,
`FactoryArtifact`, and `FactoryEvent` surfaces. A digest proves identity or
compatibility; it cannot be used to recover the source, arguments, policy, or
checkpoint body.

### Checkpoints and resume state

`workflow.checkpoint({label, state})` accepts JSON-compatible application state.
The runtime persists approved state through its checkpoint store and publishes
a checkpoint artifact/reference plus a bounded summary. It does not serialize
the JavaScript stack, closures, timers, module state, or host runtime.

On an approved resume path, `workflow.resumeState()` exposes the application
state associated with the selected checkpoint. On a fresh start it returns
`undefined`. The replay file's public checkpoint event contains the reference,
digest, label, and resumability facts, not a supported raw checkpoint-body
interface. Use Factory Session resume and artifact APIs rather than reading or
editing replay JSON to inject resume state.

### Replay observations

Replay reconstructs canonical Factory Session lifecycle status and the recorded
event and artifact summaries. A completed recording can reconstruct final
result availability; an interrupted or failed recording can reconstruct its
terminal/partial status, safe failure detail, and the checkpoint or artifact
references emitted before termination. Older recordings expose only the facts
their events actually contain.

Replay never dispatches live child work. It applies recorded Dispatch request
and response facts and therefore does not contact model providers, rerun script
workers, or resume a JavaScript VM. To continue from checkpoint application
state, use the supported Factory Session resume path; to reproduce fresh live
work, start a new Factory Session.

## Example Commands

Record to an explicit path you control:

```bash
you run --dir ./factory --record ./docs/examples/sample-run.replay.json
```

Replay that artifact later:

```bash
you run --dir ./factory --replay ./docs/examples/sample-run.replay.json
```

Run live without writing the default recording:

```bash
you run --dir ./factory --no-record
```

Combine recording with mock workers and startup work using the checked-in
examples:

```bash
you run --dir ./examples/write-code-review \
  --with-mock-workers ./docs/examples/mock-workers.json \
  --work ./docs/examples/startup-work.json \
  --record ./docs/examples/sample-run.replay.json
```

```bash
you run --dir ./examples/write-code-review \
  --replay ./docs/examples/sample-run.replay.json
```

[`docs/examples/README.md`](../examples/README.md) documents how the example
files fit together.

## Sensitivity and Retention

Replay artifacts are sensitive because they can contain prompts, payloads,
stdout, stderr, result summaries, and diagnostic metadata. Redaction metadata
describes redaction that occurred; it is not a guarantee that every
customer-authored payload is safe to disclose. Treat the whole artifact as
sensitive, avoid putting secrets in workflow arguments or artifacts, and review
an explicitly recorded file before sharing it.

The first version does not delete old artifacts automatically. Manage retention
in your own home directory or CI workspace. Do not commit generated replay
artifacts from real customer runs.

Maintainers who need the internal event-log and fixture workflow can use
`you docs record-replay`;
customer runs only need the CLI flags above.

## Related

- `you docs config` — brief run-flag summary with pointers to this topic
- `you docs authoring-factories` — full factory authoring workflow
- `you docs mock-workers` — deterministic runs without live provider calls
