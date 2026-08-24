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

Replay compares the recording with the current effective Factory Definition.
When the Factory Definition, workers, workstations, or runtime configuration
has drifted, human-readable mode writes one concise warning to stdout and
structured component details remain available in stderr diagnostics. The drift
is non-fatal: a successful replay still exits with status 0 and uses the
recorded inputs as the replay authority.

**Resume mode** reads a recoverable Factory Event recording, reconstructs its
last valid Factory world state, and opens a new live Factory Session from that
state. Pass `--resume <path>`. Recorded non-terminal Work is seeded back into
the live scheduler, including Work that was queued or in flight when the
source process stopped. Resume writes a separate successor recording.
It does not preserve the original process-local queue or provider process as a
live object.

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
read-only. Resume opens live Factory execution.

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

Resume restores the recorded Work state with these boundaries:

- Work in a terminal state is not dispatched again.
- Recorded non-terminal Work is re-admitted to the successor's live scheduler.
  This includes a Work item whose dispatch was active when the source stopped.
- Only Work and dispatch state represented by recorded Factory Events is
  reconstructed. Work that was submitted but never reached the recording is
  not recoverable from that recording.
- A completed dispatch remains completed in the successor. A dispatch without
  a recorded completion is reconciled as interrupted and may run again in the
  successor.

The completed-dispatch rule is an exactly-once expectation for recorded Factory
dispatch lineage. It is not an exactly-once provider-effect guarantee.
A provider may perform a side effect before the process stops without
publishing a completion event. Resume may repeat that provider attempt.
Make provider-side operations idempotent when duplicate attempts matter.

### Process-kill recovery boundary

The live queue and provider process are not stored as running processes in a
recording. A process kill can nevertheless leave a valid, unfinalized
recording with the admitted Work and dispatch facts needed for recovery.
`--resume` reads that valid prefix, restores the recorded Work placement, marks
an incomplete dispatch as interrupted, and writes a successor. A completed
dispatch in the prefix is preserved rather than executed again.

Do not resubmit Work merely because the source process stopped. First resume
the recording and inspect the source and successor separately. Resubmit only
Work that was never admitted into the recording, or when the recording is not
recoverable. See `you docs operations` for the operational recovery sequence.

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

## Verify record, kill, and resume from a fresh binary

The following PowerShell procedure is a self-contained smoke journey for the
Factory Event resume path. It builds a fresh binary and creates an isolated
temporary Factory and recording directory. It uses no `--with-server` or
`--listen` flag, so it does not bind port `7437` or any other port.
It requires Go and Python 3 on `PATH`. The script mock emits the Codex JSON
protocol. The authored workers use the Codex provider parser.

Plain text from a script does not create a provider completion.

Run these setup commands from the repository root:

```powershell
$proofBinary = Join-Path ([System.IO.Path]::GetTempPath()) ("you-rsm8-" + [guid]::NewGuid().ToString("N") + ".exe")
go build -o $proofBinary .\cmd\factory

$proofRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("you-rsm8-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $proofRoot | Out-Null
Set-Location $proofRoot
New-Item -ItemType Directory -Force -Path .\factory\workers\first, .\factory\workers\second | Out-Null
$proofHome = Join-Path $proofRoot "home"
New-Item -ItemType Directory -Force -Path $proofHome | Out-Null
$originalHome = $env:HOME
$originalUserProfile = $env:USERPROFILE
$env:HOME = $proofHome
$env:USERPROFILE = $proofHome

function Write-Utf8NoBom([string] $path, [string] $content) {
    [System.IO.File]::WriteAllText(
        (Join-Path $proofRoot $path),
        $content,
        [System.Text.UTF8Encoding]::new($false)
    )
}
```

Create the minimal two-stage Factory, one Work item, and deterministic worker
helper:

```powershell
Write-Utf8NoBom .\factory\factory.json @'
{
  "name": "rsm8-resume",
  "workTypes": [
    {
      "name": "task",
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "processing", "type": "PROCESSING"},
        {"name": "complete", "type": "TERMINAL"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "workers": [
    {"name": "first"},
    {"name": "second"}
  ],
  "workstations": [
    {
      "name": "step-one",
      "behavior": "STANDARD",
      "worker": "first",
      "inputs": [{"workType": "task", "state": "init"}],
      "outputs": [{"workType": "task", "state": "processing"}],
      "onFailure": [{"workType": "task", "state": "failed"}]
    },
    {
      "name": "step-two",
      "behavior": "STANDARD",
      "worker": "second",
      "inputs": [{"workType": "task", "state": "processing"}],
      "outputs": [{"workType": "task", "state": "complete"}],
      "onFailure": [{"workType": "task", "state": "failed"}]
    }
  ]
}
'@

Write-Utf8NoBom .\factory\workers\first\AGENTS.md @'
---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: CODEX
stopToken: COMPLETE
---
Execute the task.
'@
Copy-Item .\factory\workers\first\AGENTS.md .\factory\workers\second\AGENTS.md

Write-Utf8NoBom .\work.json @'
{
  "requestId": "rsm8-cli-proof",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "resume-me-once",
      "workTypeName": "task",
      "state": "init",
      "payload": {"subject": "record kill resume"}
    }
  ]
}
'@

Write-Utf8NoBom .\worker.py @'
import json
from pathlib import Path
import time

count_file = Path("worker-count")
started = Path("worker-started")
release = Path("worker-release")
count = int(count_file.read_text() or "0") + 1 if count_file.exists() else 1
count_file.write_text(str(count))

def complete():
    messages = [
        {"type": "turn.started"},
        {
            "type": "item.completed",
            "item": {
                "id": "message-final",
                "type": "agent_message",
                "text": "mock worker accepted\nCOMPLETE",
            },
        },
        {"type": "turn.completed", "usage": {"input_tokens": 1, "output_tokens": 1}},
    ]
    for message in messages:
        print(json.dumps(message))

if count == 1:
    complete()
elif not release.exists():
    started.touch()
    time.sleep(300)
else:
    complete()
'@

Write-Utf8NoBom .\mock-workers.json @'
{
  "mockWorkers": [
    {
      "runType": "script",
      "scriptConfig": {
        "command": "python",
        "args": ["worker.py"],
        "workingDirectory": ".",
        "timeout": "10m"
      }
    }
  ]
}
'@
```

Start the source run and wait until the first dispatch is durably recorded and
the second dispatch has entered the helper. The source process remains alive
while the helper sleeps:

```powershell
$source = Start-Process -FilePath $proofBinary -ArgumentList @(
  "run", "--dir", "./factory", "--with-mock-workers", "./mock-workers.json",
  "--work", "./work.json", "--record", "./source-recording.json"
) -WorkingDirectory $proofRoot -RedirectStandardOutput .\source.stdout.log `
  -RedirectStandardError .\source.stderr.log -WindowStyle Hidden -PassThru
$sourcePid = $source.Id

$ready = $false
$deadline = (Get-Date).AddSeconds(60)
while (-not $ready -and (Get-Date) -lt $deadline) {
    if ((Test-Path .\source-recording.json) -and (Test-Path .\worker-started)) {
        try {
            $sourceEnvelope = Get-Content -Raw .\source-recording.json | ConvertFrom-Json
            $sourceEvents = @($sourceEnvelope.events)
            $requests = @($sourceEvents | Where-Object { $_.type -eq "DISPATCH_REQUEST" })
            $responses = @($sourceEvents | Where-Object { $_.type -eq "DISPATCH_RESPONSE" })
            $ready = $requests.Count -eq 2 -and $responses.Count -eq 1
        } catch {
            $ready = $false
        }
    }
    if (-not $ready) { Start-Sleep -Milliseconds 500 }
}
if (-not $ready) { throw "source recording did not reach the kill boundary" }
Get-Content .\worker-count
```

Kill the source process tree, then inspect the source prefix and replay it as
historical data:

```powershell
taskkill /PID $sourcePid /T /F

function Show-RecordingFacts([string] $path) {
    $envelope = Get-Content -Raw $path | ConvertFrom-Json
    $events = @($envelope.events)
    $requests = @($events | Where-Object { $_.type -eq "DISPATCH_REQUEST" })
    $responses = @($events | Where-Object { $_.type -eq "DISPATCH_RESPONSE" })
    $completed = @($events | Where-Object { $_.type -eq "SESSION_COMPLETED" })
    $transitions = @($responses | ForEach-Object { $_.payload.transitionId })
    $states = @($responses | ForEach-Object { $_.payload.outputWork[0].state.name })
    Write-Output ("{0} events={1} dispatchRequests={2} dispatchResponses={3} responseTransitions={4} workStates={5} sessionCompleted={6}" -f `
        $path, $events.Count, $requests.Count, $responses.Count, ($transitions -join ","), ($states -join ","), $completed.Count)
}

Show-RecordingFacts .\source-recording.json
& $proofBinary run --dir ./factory --replay ./source-recording.json
Write-Output ("source replay exit=" + $LASTEXITCODE)
if ($LASTEXITCODE -ne 0) { throw "source replay failed" }
```

Release the helper and resume into a new recording. The existing
`worker-count` is `2`, so a successful successor invokes only the interrupted
dispatch as invocation `3`:

```powershell
New-Item -ItemType File -Force .\worker-release | Out-Null
& $proofBinary run --dir ./factory --with-mock-workers ./mock-workers.json `
  --resume ./source-recording.json --record ./successor-recording.json
if ($LASTEXITCODE -ne 0) { throw "resume failed" }

Show-RecordingFacts .\source-recording.json
Show-RecordingFacts .\successor-recording.json
Write-Output ("worker-count=" + (Get-Content -Raw .\worker-count))
& $proofBinary run --dir ./factory --replay ./successor-recording.json
Write-Output ("successor replay exit=" + $LASTEXITCODE)
if ($LASTEXITCODE -ne 0) { throw "successor replay failed" }
$env:HOME = $originalHome
$env:USERPROFILE = $originalUserProfile
```

The observed facts from the fresh binary were:

```text
source-recording.json events=14 dispatchRequests=2 dispatchResponses=1 responseTransitions=step-one workStates=processing sessionCompleted=0
successor-recording.json events=24 dispatchRequests=3 dispatchResponses=2 responseTransitions=step-one,step-two workStates=processing,complete sessionCompleted=1
worker-count=3
source replay exit=0
successor replay exit=0
```

The source is an unfinalized but valid prefix. The successor carries the
completed `step-one` response forward. It adds one resumed `step-two` dispatch
and reaches terminal `complete` Work. A portable JavaScript recording has a
different artifact class. Confirm its `--resume` rejection before using this
journey.

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
