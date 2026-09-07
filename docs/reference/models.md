---
author: Agent Factory Team
last-modified: 2026-09-06
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

Without an explicit `--server`, discovery, inspection, pull, and removal use
the local Models composition. You do not need to start `you server` first.
Set `--server` only to use a reachable running service at that address.

```bash
you models list
you models inspect llm
you models inspect asr
you models inspect tts
you models inspect embed
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

### Check Download Size Before Pulling

The built-in names resolve to pinned model payloads. These approximate decimal
sizes exclude the additional platform-specific backend and runtime files.

| Name | Operation | Pinned model payload |
| --- | --- | ---: |
| `llm` | `OMNI` | 5.0 GB |
| `asr` | `ASR` | 148 MB |
| `tts` | `TTS` | 1.714 GB |
| `embed` | `EMBED` | 1.21 GB |

Run `you --json models inspect <name>` to confirm the pinned source before a
pull. After installation, `cacheBytes` reports the exact managed cache size.

Warning: Pulling `tts` downloads an approximately 1.714 GB three-file model
bundle. Backend and runtime files need additional disk space. Inspect `tts`
before pulling or invoking it.

### Built-in TTS bundle identity

The built-in `tts` model uses one immutable three-file bundle. The bundle contains one model, one tokenizer, and one voice.

| Role | File | Size in bytes | SHA-256 |
| --- | --- | ---: | --- |
| `model` | `vibevoice-realtime-0.5B-q8_0.gguf` | `1699832128` | `5251e3f0386d1056a90c61b6c7359a4775da44dd19402499bef1989c4b5c653a` |
| `tokenizer` | `tokenizer.gguf` | `5922368` | `37dc3b722d5677e37e29a57df55aa05c485116eeb5459e57ff8dde616b4986f6` |
| `voice` | `voice-en-Carter_man.gguf` | `8472448` | `b15cd8b9cae6ee2c3d20b0ee6e7bfe93f13489f8b63b6834e9bbf0dfabf6505a` |

The immutable model source is `hf://mudler/vibevoice.cpp-models/vibevoice-realtime-0.5B-q8_0.gguf@a67807e65e3002e187179a856e96043f75060bc9`.
The publication revision is `a67807e65e3002e187179a856e96043f75060bc9`.
The base model is `microsoft/VibeVoice-Realtime-0.5B`, under the `MIT` license.

The private backend identity uses `localai-vibevoice` from
`https://github.com/mudler/vibevoice.cpp` at commit
`000e37282bc5bb09edc20f7047a47924122ba3a0`.
The LocalAI source commit is `b224c96db6f4b87306a33a808650bfce63b12588`.
The protocol source is `backend/backend.proto` at revision
`ad62c6df07ae1169eb14411a565a689cd996b19c`.

The published backend artifacts use these target identities:

| Target | Artifact | Size in bytes | SHA-256 | Accelerator |
| --- | --- | ---: | --- | --- |
| `darwin-arm64` | `localai-backend-localai-vibevoice-darwin-arm64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz` | `9200265` | `624385483a7c67804ff546ed8649e35c4e7122b833f318ff4d1cf2d44d9f2752` | `metal` |
| `linux-amd64` | `localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz` | `14976678` | `8a8ae6b816e4eb4b7088a7e5c7ef291dbd657f6f38f930b5471b9a73fb056bcb` | `cpu` |
| `windows-amd64` | `localai-backend-localai-vibevoice-windows-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.zip` | `10757902` | `8f3c14212948be34c930e9a790af7757460cb2f6bb6a0de80d5b9f95b71e8646` | `cpu` |

The role manifest is authored at
`pkg/services/models/internal/artifacts/localai-model-role-artifacts.json`.
The private protocol subset is authored at
`pkg/services/models/internal/backends/localai/backend_subset.proto`.
Regenerate `backend_subset.pb.go` with `protoc` `6.31.1` and
`protoc-gen-go` `1.36.7`; do not edit the generated file by hand.

The public `tts` name, `TTS` operation, input and output slots, and lifecycle
remain unchanged. A different immutable revision does not reuse this bundle's
cache. After an upgrade, run `you models inspect tts`, then `you models pull tts`
when the readiness state is `MISSING`.

To roll back, restore the catalog source and private role metadata from one
reviewed revision. Remove a newer `tts` cache before invoking the restored
revision. Do not mix a catalog revision with another revision's role metadata.

## Pull A Managed Local Model

Pull supported local assets into the service's managed cache:

```bash
you models pull llm
you --json models pull embed
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
you models inspect llm
you --json models list
you --json models inspect llm
```

`models list` reports the installed revision, exact `cacheBytes`, and a readable
cache size. `models inspect` also reports the resolved `cachePath`. Missing cache
facts are explicit, while `readinessState` and `lifecycleState` keep their normal
managed-runtime meanings.

The byte count sums regular files recursively within the selected revision. It
does not follow symbolic links, and it does not count data outside that revision.

Remove one installed revision only when you no longer need its local files:

```bash
you models remove llm
you --json models remove embed
```

Removal names the model, revision, and validated `cachePath`. It measures regular
files before deletion, removes only that revision, verifies that the path is gone,
and reports `bytesRemoved`. `MODEL_CACHE_NOT_FOUND` means no installed cache exists.
`MODEL_CACHE_IN_USE` means an active host or invocation still holds the cache.

Removal is always customer-initiated. The managed disk cache has no automatic
eviction, time-to-live policy, or background cleanup. Runtime host unloading does
not remove the managed files.

### Managed Model Storage

The managed model cache remains under `.agent-factory/models`. Operator Settings
uses `~/.you-agent-factory/config.json`, Factory Definitions uses
`~/.you-agent-factory/factories`, and Recordings uses
`~/.you-agent-factory/recordings`.

These directories keep model assets separate from settings, Factory Definitions,
and Recordings.

## Invoke A Model Directly

Direct invocation requires a valid Current Factory at
`./factory/factory.json`. The path is relative to the working directory.
It does not start a full Factory workflow.

Without an explicit `--server`, direct invocation uses the local Models
composition. An explicit `--server` must identify a reachable service.

Use an uppercase operation and bind inputs with repeatable
`--input slot=value` flags. Legacy `--text` and unqualified `--output <path>`
spellings remain supported for direct text and audio operations.

If `./factory/factory.json` is missing, the command returns
`CURRENT_FACTORY_NOT_FOUND` before it accesses model assets. Create a valid
Current Factory, then retry.

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

### Generate an embedding

The built-in `embed` model converts one text input into one `embedding:JSON`
output. Its only operation is `EMBED`, so Models infers the operation when you
omit `--operation`.

Run this command from a directory containing a valid Current Factory:

```bash
you models invoke embed --operation EMBED --input text="Find similar work"
```

The command resolves `embed` and acquires missing managed assets on first use.
It loads the runtime, invokes the model, writes one JSON numeric array to
stdout, and releases the runtime. Diagnostics remain on stderr, so stdout is
safe to pipe.

Use `@<path>` for a text file. Models reads the bounded file content before
asset download or backend activation:

```bash
you models invoke embed --input text=@./query.txt
```

Use `json:` for the optional `parameters` input. The JSON value must be an
object. Supported parameters are `dimensions`, `encoding_format`, and
`normalize`:

```bash
you models invoke embed \
  --input text="Find similar work" \
  --input 'parameters=json:{"normalize":true}'
```

Use the global `--json` flag when a script needs the output name and metadata:

```bash
you --json models invoke embed --operation EMBED --input text="Find similar work"
```

The response contains one named `embedding` output. Its modality and media
type are `JSON` and `application/json`. Its `content` is the JSON numeric
array:

```json
{
  "outputs": [
    {
      "name": "embedding",
      "modality": "JSON",
      "contentType": "application/json",
      "mediaType": "application/json",
      "content": "[0.1,0.2,0.3,0.4]"
    }
  ]
}
```

After a successful invocation, verified cache entries are reused. The command
does not provide an `--offline` flag.

Malformed input syntax, missing `text`, unknown slots, duplicate slots,
malformed JSON, and unsupported parameters fail before download or backend
activation. Backend protocol or response failures return the typed
`MODEL_BACKEND_FAILURE` diagnostic and no partial embedding. Diagnostics do
not include backend addresses, cache paths, signed URLs, or tokens.

The generic HTTP endpoint provides the same operation inference and named
output:

```http
POST /models/invocations
Content-Type: application/json

{
  "scope": "factory-session:embed",
  "holder": "example",
  "model": {"nameOrUri": "embed"},
  "inputs": [
    {"name": "text", "modality": "TEXT", "content": "Find similar work"}
  ]
}
```

The HTTP response uses the same `embedding` slot, `JSON` modality,
`application/json` media type, and canonical JSON vector content.

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
you --json models invoke tts --operation TTS --input text="Read the release summary."
```

With no output mapping, JSON mode validates the request and reports that
inference was not executed. To execute and receive output metadata, provide an
explicit output mapping:

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
requested without an output path, input mapping, or explicit output mappings, for example
`{"modelName":"tts","operation":"TTS","mode":"VALIDATION_ONLY","validationOnly":true,"inferenceExecuted":false}`.
It validates the request but does not execute inference or return model output.
An output path or explicit output mapping selects execution behavior.

Invocation is readiness-gated. For `MISSING`, pull the model. For `LOADING`,
wait and inspect again. For `FAILED`, use the inspect diagnostics and service
logs to correct runtime startup or health failures. `MODEL_NOT_AVAILABLE` and
the managed-runtime failure details identify the model and readiness state;
successful output is not emitted for a failed invocation.

## Invoke The Built-In Omni Model

The built-in `llm` model exposes the pinned `OMNI` operation. It accepts the
following named input slots:

| Slot | Value | Required | Repeatable |
| --- | --- | --- | --- |
| `prompt` | Text | Yes | No |
| `image` | `@` file detected as an image | No | Yes |
| `audio` | `@` file detected as audio | No | No |
| `video` | `@` file detected as video | No | No |
| `parameters` | JSON text prefixed with `json:` | No | No |

The pinned-protocol conformance fixture records the `Audios` and `Videos`
request fields as accepted at the pinned llama.cpp protocol revision. This
slice therefore supports text, images, audio, and video. The output is text.

Use a repeatable `--input` flag for each named binding. Set
`--operation OMNI` to select the built-in operation.

```bash
you models invoke llm --operation OMNI --input prompt="Write a haiku"
```

The command writes the generated UTF-8 text to stdout. Diagnostics remain on
stderr. Use global `--json` when a structured output object is required.

### Add Images In Command Order

Repeat the `image` binding to preserve the supplied image order:

```bash
you models invoke llm \
  --input prompt="Compare these two designs" \
  --input image=@a.png \
  --input image=@b.png
```

The protocol receives `a.png` before `b.png`. A second value for any
non-repeatable slot fails before generation.

### Bind Media Files

Prefix a file path with `@` to read its bytes and detect its media type. Common
extensions map to their concrete types, including `.txt`, `.png`, `.wav`, and
`.mp4`. Unknown extensions use content detection.

```bash
you models invoke llm \
  --input prompt="What happens at 0:30?" \
  --input video=@clip.mp4

you models invoke llm \
  --input prompt="Describe this recording" \
  --input audio=@speech.wav
```

The detected type must match the named slot. For example,
`--input audio=@clip.mp4` is rejected before generation. The Models service
classifies this as `MEDIA_CAPABILITY`. The CLI reports the safe
`CLI_COMMAND_FAILED` diagnostic.
Unsupported modalities are never silently omitted or converted.

Cancel a running invocation with `Ctrl-C`. Cancellation releases model capacity
and leaves no partial stdout or output file.

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
