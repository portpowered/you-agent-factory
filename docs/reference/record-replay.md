# Record and Replay

Use record and replay when you want to capture a live factory run as a
replay-compatible artifact, locate the saved file after shutdown, or re-run
from a saved history without dispatching live workers again.

`you docs record-replay` is the canonical guide for `--record`, `--replay`, and
`--no-record`. See `you docs authoring-factories` for the full
factory setup workflow.

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

Use the Factory Session id reported by the replay with the public durable
inspection commands:

```bash
you workflow status <session-id>
you workflow events <session-id>
you workflow artifacts <session-id>
you workflow result <session-id> --mode partial
you workflow result <session-id> --mode final
```

These reads expose the recorded public status, ordered `FactoryEvent`
summaries, `FactoryArtifact` summaries, and partial or final result
availability. A paused recording remains historical: it cannot be resumed or
treated as a live Factory Session. Use live durable-session inspection when you
need child-dispatch details or resumable checkpoint state.

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
recording. Secret counts are bounded metadata, not a list of secret names or
contents.

Artifact and event entries are inspection summaries. A hash or digest proves
consistency with referenced public data; it does not make omitted source,
artifact content, checkpoint state, provider output, or child-dispatch history
recoverable. Keep recordings private even with these omissions because public
results, labels, and event summaries can still contain customer information.

## Sensitivity and Retention

Replay artifacts are sensitive because they can contain prompts, payloads,
stdout, stderr, and diagnostic metadata.

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
