---
author: Agent Factory Team
last-modified: 2026-09-24
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

The built-in `asr` selects Qwen3-ASR 0.6B Q8_0 with a separate Qwen3 forced
aligner for speech timestamps, including Chinese, Japanese, Korean, and English.
Its immutable recognition source is
`hf://OpenVoiceOS/qwen3-asr-0.6b-q8-0/qwen3-asr-0.6b-q8_0.gguf@47fe022389f25564002e574e5d82aa32a268893b`.
The bundled aligner is independently pinned to
`OpenVoiceOS/qwen3-forced-aligner-0.6b-q8-0@efe4002aa7d567851883c05f9950c869debe8847`.
The native Qwen ASR backend is currently distributed for Windows AMD64 with
CUDA; this manual testing release does not provide CPU, Linux, or Metal builds.
Recognition quality varies by language and recording. Use the ASR `language`
parameter when the source language is known; translation into a target language
is a separate operation.

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
| `asr` | `ASR` with aligned timestamps | 2.348 GB |
| `tts` | `TTS` | 1.714 GB |
| `embed` | `EMBED` | 639 MB |
| `qwen3-tts-base` | `TTS` with a reference WAV | 884 MB |

Run `you --json models inspect <name>` to confirm the pinned source before a
pull. After installation, `cacheBytes` reports the exact managed cache size.

Warning: Pulling `tts` downloads an approximately 1.714 GB three-file model
bundle. Backend and runtime files need additional disk space. Inspect `tts`
before pulling or invoking it.

### Built-in Windows ASR backend provenance

The default Qwen3-ASR CUDA backend uses `qwen3-asr.cpp` source
`6dcc586e5073fd6e85ee5728e75f0903d6c70c6c`, the LocalAI SDK at
`b224c96db6f4b87306a33a808650bfce63b12588`, and the repository bridge at
`7e640360a555b8b8a7460deee87fced4e1aef14f`. Its immutable publication is
`localai-backends-v1-8d2cd1a1c4337e1c8efb60bf64ffdfb9a559a91e6a6d06ec3eea7df2996e5e8a`.
The Windows AMD64 archive is 441,891,463 bytes, with SHA-256
`e20cafd64c87a5a0293f6a7a19ef20be3566aa1a0cc1af54f70c383b7fdb531c`.
The recognition and forced-alignment model files are separate downloads.

### Earlier Windows Whisper backend provenance

This earlier backend identity is pinned to merged build
`8f945b0eadcf9863821585c7f5edeb84d855f471`. It describes the Windows Whisper
CPU backend ZIP, separate from the ASR model payload listed above.

| Field | Pinned value |
| --- | --- |
| LocalAI source | `b224c96db6f4b87306a33a808650bfce63b12588` (`backend/go/whisper`) |
| Whisper.cpp source | `080bbbe85230f624f0b52127f1ae1218247989f9` |
| Protocol source | `backend/backend.proto` at `ad62c6df07ae1169eb14411a565a689cd996b19c` |
| Release tag | `localai-backends-v1-17273d7dbb61dba3f7bfdfd6e05bd90231cf1c1224cf5556668abea1bac91106` |
| Asset | `localai-backend-localai-whisper-windows-amd64-080bbbe85230f624f0b52127f1ae1218247989f9.zip` |
| Size | `11,935,463` bytes |
| Manifest SHA-256 | `6956415b4b47b14346e0eacc1c5fa34b15a0c8cf9e7c4e7a3436b8b9e96b63c3` |
| Immutable asset URL | `https://github.com/portpowered/you-agent-factory/releases/download/localai-backends-v1-17273d7dbb61dba3f7bfdfd6e05bd90231cf1c1224cf5556668abea1bac91106/localai-backend-localai-whisper-windows-amd64-080bbbe85230f624f0b52127f1ae1218247989f9.zip` |

A no-body HTTPS `HEAD` trace on 2026-09-24 observed this redirect chain:

| Time (UTC) | HTTPS host | Status | Declared next host | Peer |
| --- | --- | ---: | --- | --- |
| `08:23:03.612`–`08:23:04.130` | `github.com` | `302` | `release-assets.githubusercontent.com` | `140.82.114.4` |
| `08:23:04.184`–`08:23:04.534` | `release-assets.githubusercontent.com` | `200` | — | `185.199.108.133` |

The signed redirect path and query are omitted. The corrected chain completed
in `0.933` seconds, returned no body, and accounted for `7,700` request and
response bytes. The diagnosed initial capture plus the corrected chain
accounted for `13,356` application bytes total. The probe allowed at most two
redirects, a 60-second deadline per chain, and 16 MiB total transfer; it used
one corrected retry and no further retry.

Before pulling this pinned Windows backend, predeclare outbound DNS and HTTPS
TCP port `443` access to `github.com` and
`release-assets.githubusercontent.com`. Apply the policy to hostnames; do not
allowlist the observed peer IPs or infer additional hosts from reverse DNS.
The GitHub peer varied between observations while the declared redirect host
remained the same.

This observation identifies the manifest's expected size and checksum; it did
not download or hash the asset. It does not cover the ASR model payload, other
builds, backend startup, transcription, offline reuse, or cleanup. For another
build, inspect that build's selected manifest entry and verify its artifact
identity before reusing this host policy.

### Linux LocalAI backend installation

On Linux amd64, `you` installs managed LocalAI backends from LocalAI's current
backend gallery on first use. It selects a CUDA 12 backend when an NVIDIA GPU
device and `nvidia-smi` are available. Otherwise it selects the CPU backend.
The `localai-llamacpp`, `localai-whisper`, and `localai-vibevoice` identities
map to the corresponding gallery backends. VibeVoice uses the `vibevoice-cpp`
gallery variant for its GGUF model bundle. The installed backend stays in the
user cache for later use.

`you` uses `LOCALAI_BINARY` when it is set, then `local-ai` on `PATH`. If
neither is available, it downloads the latest official Linux amd64 LocalAI
binary and verifies its size and SHA-256 against that release's checksums.
The first install needs access to GitHub, LocalAI's backend gallery, and its
backend image registry. Run `you` inside WSL Ubuntu to use the current CUDA
gallery builds on a Windows host.

On native Windows, `you` checks for an NVIDIA GPU and, when online, reads the
latest compatible backend archive manifest from the project's published
releases. It selects a CUDA archive for each backend that has one, then uses
the CPU archive for backends without a CUDA build. The current publication
includes manual Windows CUDA test archives for llama.cpp, Whisper, and
VibeVoice. Offline first use uses the bundled archive manifest. A publication
loaded earlier in the same process remains available to offline requests.

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

The packaged backend publication used by native Windows and macOS includes
these target identities. Linux amd64 uses the LocalAI gallery described above.

| Target | Artifact | Size in bytes | SHA-256 | Accelerator |
| --- | --- | ---: | --- | --- |
| `darwin-arm64` | `localai-backend-localai-vibevoice-darwin-arm64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz` | `9200265` | `624385483a7c67804ff546ed8649e35c4e7122b833f318ff4d1cf2d44d9f2752` | `metal` |
| `windows-amd64` | `localai-backend-localai-vibevoice-windows-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.zip` | `10757902` | `8f3c14212948be34c930e9a790af7757460cb2f6bb6a0de80d5b9f95b71e8646` | `cpu` |
| `windows-amd64-cuda` | `localai-backend-localai-vibevoice-windows-amd64-cuda-000e37282bc5bb09edc20f7047a47924122ba3a0.zip` | `448293919` | `e3f9c42e1d4433044857b1e08422f43efc564c858a73474e993d17f9ffa83342` | `cuda` |

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

Remove one managed model cache revision when you no longer need its local files:

```bash
you models remove llm
you --json models remove embed
you --json models remove asr --reclaim-unused-cache
```

Without `--reclaim-unused-cache`, removal deletes only the selected revision and
reports its measured `bytesRemoved`. The default retains shared model and backend
cache assets.

Add `--reclaim-unused-cache` to reclaim proven unreferenced cache assets managed by
YOU. The flag defaults to off. JSON output reports `bytesRemoved`,
`reclaimedCacheBytes`, and `retainedSharedCacheBytes` separately.
`MODEL_CACHE_REFERENCE_UNCERTAIN` means Models could not prove that reclamation is
safe. In that case, removal leaves the revision and candidate shared assets intact.

`MODEL_CACHE_NOT_FOUND` means no installed cache exists. `MODEL_CACHE_IN_USE` means
an active host or invocation still holds the cache.

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

Invoke a built-in model directly from any directory. A Current Factory is not
required. Use verified cached model and backend artifacts for an offline
text-only OMNI call:

```bash
you models invoke llm --offline --operation OMNI --input prompt="Explain how a SHA-256 checksum differs from encryption."
```

If a valid Current Factory exists, its Models scope remains available for
compatible Factory-backed model configuration.

Without an explicit `--server`, direct invocation stays in the local process.
An explicit `--server` must identify a reachable service and uses that service
instead of local Models composition.

Use an uppercase operation and bind inputs with repeatable
`--input slot=value` flags. Legacy `--text` and unqualified `--output <path>`
spellings remain supported for direct text and audio operations.

A missing implicit Current Factory is not an error for a built-in invocation.
An explicitly supplied or malformed Factory still returns its typed Factory
error; the command never creates or initializes a Factory as a workaround.

Built-in model names are `llm`, `asr`, `tts`, `embed`, and `qwen3-tts-base`. Use an uppercase
operation and bind inputs with repeatable `--input slot=value` flags.

Use `@path` to bind file bytes. Models detects the media type from the path and
file content, then validates it against the input slot before model execution.
Use inline values for text, and valid JSON values for JSON slots.

Use `--offline` to require verified cached model and backend artifacts. It never
accesses the network. If an artifact is missing, pull the model with
`you models pull <name>`, then retry the invocation.

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

Run this command from any directory; a Current Factory is not required:

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

After a successful invocation, verified cache entries are reused. Add
`--offline` when a run must remain cache-only:

```bash
you models invoke embed --offline --operation EMBED \
  --input text="Use only verified local assets"
```

If required model or backend artifacts are missing, the command returns
`MODEL_OFFLINE_CACHE_UNAVAILABLE` with the complete missing set. Pull the model
while online, then retry with `--offline`.

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

For direct `POST /models/invocations` responses, `TEXT` and `JSON` outputs use
`content`. `AUDIO`, `IMAGE`, `VIDEO`, and `BINARY` outputs use `contentBase64`
instead. Decode that field with standard Base64 to recover the original bytes.

For a direct HTTP invocation with local media files, send a multipart request.
The `request` part contains the generic invocation JSON. Each `files` part
fills the next image, audio, video, or binary input that has no inline content
or artifact reference, in input order:

```bash
curl -X POST http://localhost:7437/models/invocations \
  -F 'request={"scope":"factory-session:example","holder":"example","model":{"nameOrUri":"llm"},"operation":"OMNI","inputs":[{"name":"prompt","modality":"TEXT","content":"Describe the audio and video."},{"name":"audio","modality":"AUDIO","mediaType":"audio/wav"},{"name":"video","modality":"VIDEO","mediaType":"video/mp4"}]};type=application/json' \
  -F 'files=@speech.wav;type=audio/wav' \
  -F 'files=@clip.mp4;type=video/mp4'
```

Uploaded files must be nonempty. Direct Models requests have no fixed input
size limit. JSON callers can continue to use
`contentBase64` with the file bytes and `mediaType` for each media input.

`ASR` preserves backend segment timestamps in the `segments` JSON output. Each
segment contains `id`, `start`, `end`, and `text` fields.

### Transcribe audio or video

Map every ASR output explicitly:

```bash
you models invoke asr --operation ASR \
  --input audio=@meeting.wav \
  --output transcript=meeting.txt \
  --output segments=meeting.json
```

ASR also accepts a video file in the `audio` slot and transcribes its first
audio stream. Video extraction requires `ffmpeg` and `ffprobe` on `PATH`. A
video without audio fails with a diagnostic.

```bash
you models invoke asr --operation ASR --input audio=@demo.mp4 \
  --output transcript=video.txt --output segments=video-asr.json
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

The built-in `qwen3-tts-base` model uses Qwen3-TTS 0.6B Base with a
reference WAV. It downloads one talker and one tokenizer GGUF at immutable
revision `b7ee2e8c7459c3bea99da23e3d178125a7d1713c` from
`Serveurperso/Qwen3-TTS-GGUF`. Use the `voice:AUDIO` slot for the reference
clip, `ref_text` for its transcript, and `language` for the output language.
The reference must be a mono PCM WAV at 24 kHz for this backend.

```bash
you models invoke qwen3-tts-base --operation TTS \
  --input text="你现在能帮我吗？" \
  --input voice=@reference.wav \
  --parameter '{"name":"ref_text","value":"Can you assist me right now?"}' \
  --parameter '{"name":"language","value":"Chinese"}' \
  --output audio=speech.wav
```

The native Windows CUDA backend is a manual test build for an NVIDIA RTX 4090.
Its release notes identify the compiler, CUDA version, and source patches.
Other GPU variants require a compatible backend build.

The built-in `tts` model uses VibeVoice Realtime 0.5B and a packaged voice.
It rejects reference WAV inputs because this model does not support runtime
voice cloning. Operator-configured reference-capable models, including
Qwen3-TTS and IndexTTS, can use the same input and parameter form shown above.

Direct `models invoke` accepts nonempty files without a fixed size limit.
A successful WAV output confirms synthesis. Voice similarity depends on the
selected backend and reference audio.

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

The `OMNI` request can carry text, image, audio, and video inputs to the pinned
LocalAI protocol. The protocol conformance fixture checks the request fields;
it does not confirm that a loaded model understands the media. The output is
text. The built-in `llm` requires a verified multimodal projector for media
inference. Check `you models inspect llm` before sending media. If
`mediaReadiness` reports that the projector is missing or invalid, video is
unavailable. The effective operation also omits `image` and `audio`. Requests
for any of these media inputs fail before backend execution. Pull the model to
repair an incomplete cache, then inspect it again.

Use a repeatable `--input` flag for each named binding. Set
`--operation OMNI` to select the built-in operation.

```bash
you models invoke llm --operation OMNI --input prompt="Write a haiku"
```

The command writes the generated UTF-8 text to stdout. Diagnostics remain on
stderr. Use global `--json` when a structured output object is required.

### Limit OMNI output tokens

Pass an explicit `max_tokens` direct parameter to cap generation length:

```bash
you models invoke llm --operation OMNI --input prompt="Count to ten" \
  --parameter '{"name":"max_tokens","value":128}'
```

`max_tokens` accepts a positive integer from `1` to `2147483647`. When
supplied, it sets the LocalAI generation token limit (`PredictOptions.Tokens`)
and is excluded from request metadata. When omitted, generation stays
unbounded (`Tokens=0`) and context size stays model-derived (`ContextSize=0`).

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
`.mp4`. Unknown extensions use content detection. Each file must be nonempty.
Direct Models invocation does not impose a fixed file-size limit.

```bash
you models invoke llm \
  --input prompt="What happens at 0:30?" \
  --input video=@clip.mp4

you models invoke llm \
  --input prompt="Describe this recording" \
  --input audio=@speech.wav
```

When a video contains an audio stream, `OMNI` reads the first audio stream
and the video frames. It returns separate `Embedded audio from video input 1`
and `Video` observations. A request with a separate `audio` input and a
`video` input returns labeled observations for both sources. The host needs
`ffmpeg` and `ffprobe` on `PATH` for video input. A video without an audio
stream still returns a visual answer.

Combined audio and video analysis does not include the optional `usage`
output. It rejects an explicit `max_tokens` parameter because the composed
response cannot apply that token limit exactly. Single-modality `OMNI`
invocations continue to support `max_tokens` and `usage`.

The detected type must match the named slot. For `OMNI`,
`--input audio=@clip.mp4` is rejected before generation; bind the clip to
`video` instead. ASR accepts video containers in its `audio` slot. The Models
service classifies unsupported media as `MEDIA_CAPABILITY`. The CLI reports the
safe `CLI_COMMAND_FAILED` diagnostic.
Unsupported modalities are never silently omitted or converted.
The built-in `OMNI` operation has no general document or binary input slot.
Convert a document to text or a supported media type before invocation.

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

For media understanding, stage an image, audio, or video file as Work and bind
its content to the matching model input slot. The inference worker reads the
staged file bytes before invoking Models, preserving the file's media type and
the order of repeated inputs. Each media input is limited to 32 MiB in this
path. Direct `you models invoke` file inputs have no fixed size limit.

Use `you docs providers` for agent provider/model selection and limits. Use
`you docs workers` for worker capabilities, `you docs workstations` for routing
and bindings, `you docs resources` for managed capacity, `you docs config` for
the minimum Factory contract, and `you docs run` for complete Factory
invocation shapes.
