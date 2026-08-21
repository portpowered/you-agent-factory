# Record and Replay

Use recording when you need a durable Factory Event history. Use replay for
historical inspection. Use resume to continue live Factory execution from a
recoverable recording and write a new successor recording.

`you docs record-replay` is the canonical guide for `--record`, `--replay`,
`--resume`, and `--no-record`, including their flag conflicts. See `you docs
javascript-workflows` for the supported JavaScript authoring contract and `you
docs authoring-factories` for the full factory setup workflow.

## Default Recording On Live Runs

Normal live `you run` invocations record a replay-compatible artifact by default
when you do not pass `--no-record` or `--replay`. A resume invocation uses a
generated path for its new successor recording unless you pass `--record`.

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

## Record, Replay, and Resume Modes

**Record mode** starts a live run and writes the observed runtime history to a
replay artifact. Use the default generated path, `--record <path>`, or rely on
default recording without extra flags.

**Replay mode** reads an existing artifact and uses the recorded runtime history
instead of dispatching live workers again. Pass `--replay <path>`. Replay is
historical and read-only: it does not continue Work or write a successor
recording.

**Resume mode** reads a recoverable Factory Event recording, reconstructs its
last valid Factory world state, and opens a new live Factory Session from that
state. Pass `--resume <path>`. Resume can dispatch eligible Work again and
writes a successor recording.

## Run Controls

| Flag | Effect |
|------|--------|
| *(none on a live run)* | Record to the generated path under `~/.you-agent-factory/recordings/` |
| `--no-record` | Skip the default recording for one invocation |
| `--record <path>` | Write the replay artifact to an explicit path you own |
| `--replay <path>` | Replay an existing artifact instead of starting a live run |
| `--resume <path>` | Continue live Factory execution from a recoverable recording |

### Incompatible Combinations

These flag pairs are rejected for the same invocation:

- `--record` with `--replay`
- `--no-record` with `--record`
- `--resume` with `--replay`
- `--resume` with `--no-record`

`--resume` and `--replay` are mutually exclusive. Replay is historical and
read-only; resume opens live Factory execution.

`--resume` with `--record` is allowed. The explicit path names the new
successor recording. A flag validation failure happens before a new Factory
Session starts, so there is no new session, Dispatch, FactoryArtifact, or
FactoryEvent to inspect.

## Resume Semantics

Resume reads the source recording without modifying it. It reconstructs the
last valid Factory world state, then opens a new live Factory Session. The
source and successor are separate recordings.

When `--record` is omitted, resume writes the successor to the generated
recording directory described above. Choose the successor path explicitly:

```bash
you run --resume ./recordings/source.json \
  --record ./recordings/successor.json
```

Resume restores Work from the recovered world state with these boundaries:

- Work in a terminal state is not dispatched again.
- Queued Work is restored and can become eligible for dispatch.
- Nonterminal Work that was in flight can receive a new live attempt after
  resume.
- A provider effect that completed before its completion event was recorded can
  run again. Use idempotent provider-side operations when duplicate attempts
  would matter.

This is not an exactly-once provider-effect guarantee. The recorded terminal
state prevents duplicate dispatch of completed Work, while an incomplete
provider attempt can be retried after recovery.

The runtime accepts an unfinalized recording when it has a valid complete event
prefix. It can also skip a truncated final event-stream block after earlier
complete events. Corruption in the middle of the stream, or a source with no
valid complete event, is rejected. Resume does not recover arbitrary damaged
JSON or reconstruct an in-memory provider process.

Resume requires a Factory Event recording with the Factory Definition needed to
open the live runtime. A portable JavaScript recording is an inspection and
replay artifact. It does not contain the JavaScript VM stack, closures, timers,
module state, or provider process, so `--resume` cannot continue that VM. Use
the live Factory Session resume controls and checkpoint state for a supported
JavaScript continuation.

## JavaScript Recording Overview

A JavaScript workflow is recorded from the canonical `FactorySession` public
history. Its portable replay artifact is a versioned event envelope, not a
JavaScript VM snapshot. When the corresponding fact exists, the envelope can
include:

- the Factory Session id and JavaScript orchestrator identity;
- the workflow source reference and source digest, arguments digest, and
  effective policy digest;
- ordered lifecycle, checkpoint-reference, artifact, result, and terminal
  `FactoryEvent` summaries;
- a public checkpoint reference, label, bounded summary, timestamp, and
  `FactoryArtifact` reference;
- final, partial, failed, or unavailable result facts and bounded failure
  detail; and
- explicit omission flags plus a bounded redacted-secret count.

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

Replay never dispatches live child work and the portable envelope does not
contain a child-dispatch list. Replay therefore does not contact model
providers, rerun script workers, or resume a JavaScript VM. A portable replay
recording cannot be passed to `--resume`; use the live Factory Session resume
controls when a supported checkpoint continuation is available.

## Example Commands

Run a JavaScript factory and use the default recording path:

```bash
you run --factory ./workflow.js
```

The CLI prints `Recording saved: <path>` when the run shuts down. To choose the
portable recording path explicitly instead:

```bash
you run --record ./recordings/workflow-run.json --factory ./workflow.js
```

Replay that JavaScript Factory Session without starting live child work:

```bash
you run --replay ./recordings/workflow-run.json --factory ./workflow.js
```

The workflow source selects the factory shape; replayed status, events,
artifacts, checkpoints, and result availability come from the recording. Replay
does not invoke a provider, dispatch a child, or execute the JavaScript source.

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

## Inspect a Replayed JavaScript Factory Session

Use the Factory Session id reported by the replay with the canonical session
read and inspection routes:

```bash
you session show <session-id>
curl http://localhost:7437/factory-sessions/<session-id>/events
curl http://localhost:7437/factory-sessions/<session-id>/artifacts
curl http://localhost:7437/factory-sessions/<session-id>/results?mode=partial
curl http://localhost:7437/factory-sessions/<session-id>/results?mode=final
```

These reads expose the recorded public status, ordered `FactoryEvent`
summaries, `FactoryArtifact` summaries, and partial or final result
availability. A portable recording remains historical and cannot be passed to
`--resume` or treated as a live Factory Session. Use live durable-session
inspection when you need child-dispatch details or resumable checkpoint state.

## JavaScript Recording Contract

A portable JavaScript recording is a versioned, high-level Factory Session
envelope. Its field groups are:

| Field group | What it records |
|-------------|-----------------|
| `recordingKind`, `schemaVersion`, `replayCompatibilityVersion` | Contract identity, envelope shape, and replay compatibility |
| `session` | Factory Session id, recorded lifecycle status, and `JAVASCRIPT` orchestrator kind |
| `source` | Source reference and content hash; source contents are not embedded |
| `argumentsDigest`, `policyHash` | Digests for identity and consistency checks, not raw arguments or policy secrets |
| `artifacts` | Public artifact identity, kind, visibility, label, content hash, size, and creation time |
| `events` | Canonical event id, type, sequence, timestamp, and bounded artifact or checkpoint references |
| `checkpoint` | Optional public checkpoint reference and summary, never the checkpoint state body |
| `result` | Recorded public partial/final result, availability or safe failure summary, and integrity references |
| `redaction` | Applied omission flags and a bounded count of redacted secrets |

Replay validates the recording kind, schema and compatibility versions,
identity, hashes and digests, event ordering, and summary references before it
projects any public state. Unsupported compatibility versions fail with
supported-version or migration guidance. Malformed or inconsistent recordings
fail closed instead of returning a partially trusted Factory Session.

### Privacy and portability limits

JavaScript recordings deliberately omit raw JavaScript runtime state, raw
checkpoint bodies, provider transcripts, and child-dispatch lists. Redaction
metadata reports those omissions without copying removed values into the
recording. Secret counts are bounded to 1,000,000 (values at or above that
limit are reported as 1,000,000), not a list of secret names or contents.

Artifact and event entries are inspection summaries. A hash or digest proves
consistency with referenced public data; it does not make omitted source,
artifact content, checkpoint state, provider output, or child-dispatch history
recoverable. Keep recordings private even with these omissions because public
results, labels, and event summaries can still contain customer information.

## Sensitivity and Retention

Replay artifacts remain sensitive. The portable JavaScript envelope omits raw
runtime output and provider transcripts, but its public results, artifact
labels, checkpoint summaries, event summaries, and safe diagnostics can still
contain customer information. Legacy non-JavaScript replay artifacts may also
contain prompts, payloads, stdout, stderr, and diagnostic metadata. Redaction
metadata describes the omissions that were applied; it is not a guarantee that
every customer-authored public value is safe to disclose. Treat the whole
artifact as sensitive, avoid putting secrets in workflow arguments or
artifacts, and review an explicitly recorded file before sharing it.

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
