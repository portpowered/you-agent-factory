---
author: Agent Factory Team
last-modified: 2026-08-22
doc-id: agent-factory/models
---

# Models

Use this guide to discover models, check readiness, install managed local
assets, and invoke a model directly. For model-backed Factory authoring, this
page explains the boundary between direct model operations and
`INFERENCE_WORKER` plus `INFERENCE_RUN`; detailed worker, workstation, and
resource fields remain in their owning guides.

Use `you docs providers` for agent worker/provider selection, configured model
roles, effort choices, modality and tool limits, and AGY or ACP behavior. This
page is the canonical guide for managed model discovery, readiness, pull, and
direct inference operations.

## Discover And Inspect Models

Start a Factory service before using discovery, inspection, or pull commands.
They query its `/models` surface; use the global `--server` option when the
service is not at the default address.

```bash
you models list
you models inspect OMNIVOICE_Q4_K_M
```

`list` summarizes each model's provider locality, supported operations and
modalities, managed-runtime readiness and lifecycle, and resource count.
`inspect` shows one model's worker capabilities and readiness diagnostics. Add
the global `--json` flag when scripts need the structured response.

Use these managed-runtime fields when deciding what to do next:

| `readinessState` | Meaning |
| --- | --- |
| `READY` | Invocation can proceed. |
| `MISSING` | Required managed assets are not installed; pull the model. |
| `LOADING` | Assets exist but the runtime is still starting; wait and inspect again. |
| `FAILED` | Startup or health checks failed; inspect diagnostics and service logs. |
| `UNSUPPORTED` | This installation cannot manage the requested runtime. |

`lifecycleState` distinguishes installation from loading, including
`NOT_INSTALLED`, `INSTALLING`, `INSTALLED`, `LOADING`, and `LOADED`.
For managed local models, Models derives the two fields together from the
observed cache, active pull, and host facts:

- no verified required assets is `MISSING` / `NOT_INSTALLED`;
- an active pull with incomplete assets is `LOADING` / `INSTALLING`;
- verified required assets are `READY` / `INSTALLED`, or `READY` / `LOADED`
  when the host is loaded; and
- a failed observation is `FAILED` with a lifecycle consistent with the
  installed evidence.

The service does not publish `READY` with `NOT_INSTALLED`.

## Pull A Managed Local Model

Pull supported local assets into the service's managed cache:

```bash
you models pull OMNIVOICE_Q4_K_M
you --json models pull OMNIVOICE_Q4_K_M
```

The command is synchronous for managed local assets: it remains active until
source transfer, byte/checksum verification, and cache publication reach a
terminal success or failure. It does not report success while the backing
download is still running. The result reports the pull outcome, resulting
readiness, cache path, revision, and downloaded files. Common outcomes include
`ALREADY_READY`, `INSTALLED_SUCCESSFULLY`, `ALREADY_PRESENT`, `STILL_LOADING`,
`TIMED_OUT`, `SOURCE_FETCH_FAILED`, and `UNSUPPORTED_RUNTIME`. A successful
pull normally leaves the model `READY` / `INSTALLED` (or `READY` / `LOADED` if
the host is already loaded); a concurrent `inspect` may observe
`LOADING` / `INSTALLING` while the pull is active.

If pull fails, use the returned outcome and diagnostics rather than editing the
managed cache. Check network and source credentials for `SOURCE_FETCH_FAILED`,
retry a timed-out pull, and verify the configured backend command and health
endpoint when installed assets enter `FAILED`.

## Inspect And Remove The Managed Model Cache

The managed model cache stores installed local-model files by model and revision.
When no cache directory is configured, Models resolves the root from the current
user's home directory:

| Platform | Default managed cache layout |
| --- | --- |
| macOS or Linux | `$HOME/.agent-factory/models/<MODEL>/<revision>/` |
| Windows | `C:\Users\<USER>\.agent-factory\models\<MODEL>\<revision>\` |

`INFINITE_YOU_OMNIVOICE_CACHE_DIR` selects a different managed cache root for the
process. The selected root is the parent of each model and revision directory.

Use discovery and inspection to account for installed files:

```bash
you models list
you models inspect OMNIVOICE_Q4_K_M
you --json models list
you --json models inspect OMNIVOICE_Q4_K_M
```

`models list` reports the installed revision, exact `cacheBytes`, and a readable
cache size. `models inspect` also reports the resolved `cachePath`. Missing cache
facts are explicit, while `readinessState` and `lifecycleState` keep their normal
managed-runtime meanings.

The byte count sums regular files recursively within the selected revision. It
does not follow symbolic links, and it does not count data outside that revision.

Remove one installed revision only when you no longer need its local files:

```bash
you models remove OMNIVOICE_Q4_K_M
you --json models remove OMNIVOICE_Q4_K_M
```

Removal names the model, revision, and validated `cachePath`. It measures regular
files before deletion, removes only that revision, verifies that the path is gone,
and reports `bytesRemoved`. `MODEL_CACHE_NOT_FOUND` means no installed cache exists.
`MODEL_CACHE_IN_USE` means an active host or invocation still holds the cache.

Removal is always customer-initiated. The managed disk cache has no automatic
eviction, time-to-live policy, or background cleanup. Runtime host unloading does
not remove the managed files.

### Managed Model Storage Finding

The managed model cache remains under `.agent-factory/models`. Operator Settings
uses `~/.you-agent-factory/config.json`, Factory Definitions uses
`~/.you-agent-factory/factories`, and Recordings uses
`~/.you-agent-factory/recordings`.

This separation matches the service ownership boundaries in the repository, but
the repository does not explain the different directory names.

The placement is therefore not proven intentional. This change keeps the existing
cache root and does not move data. Track the storage decision in [issue #2201](https://github.com/portpowered/you-agent-factory/issues/2201).

## Invoke A Model Directly

Direct invocation runs one named operation through the current `./factory`
configuration. It does not start a full Factory workflow.

Built-in model names are `llm`, `asr`, `tts`, and `embed`. Use an uppercase
operation and bind inputs with repeatable `--input slot=value` flags.

Use `@path` to bind file bytes. Models detects the media type from the path and
file content, then validates it against the input slot before model execution.
Use inline values for text, and valid JSON values for JSON slots.

### Operation contracts

| Operation | Required inputs | Optional inputs | Outputs |
| --- | --- | --- | --- |
| `OMNI` | `prompt:TEXT` | `image:IMAGE` (repeatable), `audio:AUDIO`, `video:VIDEO`, `parameters:JSON` | `text:TEXT`, optional `usage:JSON` |
| `EMBED` | `text:TEXT` | `parameters:JSON` | `embedding:JSON` |
| `TTS` | `text:TEXT` | `voice:AUDIO`, `parameters:JSON` | `audio:AUDIO` |
| `ASR` | `audio:AUDIO` | `prompt:TEXT`, `parameters:JSON` | `transcript:TEXT`, `segments:JSON` |

`ASR` preserves backend segment timestamps in the `segments` JSON output. Each
segment contains `id`, `start`, `end`, and `text` fields.

### Transcribe audio

Map every ASR output explicitly:

```bash
you models invoke asr --operation ASR \
  --input audio=@meeting.wav \
  --output transcript=meeting.txt \
  --output segments=meeting.json
```

The transcript file uses `text/plain`. The segments file uses
`application/json`. Both files are published atomically after all outputs are
validated.

Use explicit output mappings with JSON when a script needs named output metadata
and artifact references:

```bash
you --json models invoke asr --operation ASR \
  --input audio=@meeting.wav \
  --output transcript=meeting.txt \
  --output segments=meeting.json
```

The JSON response includes both named outputs, their media types, sizes, and
artifact references. A multi-output ASR invocation without explicit mappings or
`--json` fails before download or backend activation and lists `transcript` and
`segments` as the required output slots.

### Synthesize speech

With no output mapping, TTS writes only raw audio bytes to standard output.
Diagnostics remain on standard error, so shell redirection and pipes are safe:

```bash
you models invoke tts --operation TTS --input text="Read the release summary." > speech.wav
```

The `--text` and unqualified `--output <path>` spellings remain compatibility
aliases for direct TTS. They use the same model readiness, request, and output
path as the named generic input:

```bash
you models invoke tts --operation TTS --text "Read the release summary." --output speech.wav
```

The unqualified `--output` form writes the backend-declared audio media type to
the named file. It is a file-output alias, not a named output mapping.

Use JSON mode when a script needs output metadata instead of audio bytes:

```bash
you --json models invoke tts --operation TTS \
  --input text="Read the release summary." \
  --output audio=speech.wav
```

### Input and output failures

An empty assignment, unknown slot, unreadable file, unsupported media type, or
duplicate non-repeatable input fails with an actionable error before download
or backend activation. A missing `audio` input fails before ASR execution.

Use `--output slot=path` once for each output slot. Do not reuse a destination
path. The command rejects incomplete, duplicate, or unknown output mappings
before it writes a partial file.

Metadata mode returns request identity and explicit execution state when JSON is
requested without an output path or explicit output mappings, for example
`{"modelName":"OMNIVOICE_Q4_K_M","operation":"TTS","mode":"VALIDATION_ONLY","validationOnly":true,"inferenceExecuted":false}`.
It validates the request but does not execute inference or return model output.
An output path or explicit output mapping selects execution behavior.

Invocation is readiness-gated. For `MISSING`, pull the model. For `LOADING`,
wait and inspect again. For `FAILED`, use the inspect diagnostics and service
logs to correct runtime startup or health failures. `MODEL_NOT_AVAILABLE` and
the managed-runtime failure details identify the model and readiness state;
successful output is not emitted for a failed invocation.

## Direct Operations Versus Factory Execution

`you models invoke` is a bounded direct operation: it selects one configured
model capability and returns that operation's result. Use a Factory when Work
must be routed, scheduled, retried, observed through Factory Session events, or
combined with other steps.

In authored Factory configuration:

- an `INFERENCE_WORKER` declares the model, locality, operation, input slots,
  and output slots;
- an `INFERENCE_RUN` workstation selects that worker and operation and maps
  submitted `WorkContent` into the declared slots;
- a `MODEL` resource declares a managed local runtime dependency and capacity.

For TTS, the worker declares an uppercase `TTS` operation with required `TEXT`
input and `AUDIO` output. Submitted Work supplies the utterance; the Factory
Session owns dispatch and the primary result. This is inference behavior, not
an agent loop. Legacy `MODEL_WORKER` and `MODEL_INVOKE` values remain migration
inputs, but new Factory configuration should use `INFERENCE_WORKER` and
`INFERENCE_RUN`.

Use `you docs providers` for agent provider/model selection and limits. Use
`you docs workers` for worker capabilities, `you docs workstations` for routing
and bindings, `you docs resources` for managed capacity, `you docs config` for
the minimum Factory contract, and `you docs run` for complete Factory
invocation shapes.
