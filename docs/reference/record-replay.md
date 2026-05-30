# Record and Replay

Use record and replay when you want to capture a live factory run as a
replay-compatible artifact, locate the saved file after shutdown, or re-run
from a saved history without dispatching live workers again.

`you docs record-replay` is the canonical guide for `--record`, `--replay`, and
`--no-record`. See [Authoring Factories](authoring-factories.md) for the full
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
stdout, stderr, and diagnostic metadata.

The first version does not delete old artifacts automatically. Manage retention
in your own home directory or CI workspace. Do not commit generated replay
artifacts from real customer runs.

Maintainers who need the internal event-log and fixture workflow can use
[`docs/internal/development/record-replay.md`](../internal/development/record-replay.md);
customer runs only need the CLI flags above.

## Related

- [Config](config.md) — brief run-flag summary with pointers to this topic
- [Authoring Factories](authoring-factories.md) — full factory authoring workflow
- [Mock Workers](mock-workers.md) — deterministic runs without live provider calls
