# LocalAI TTS Voice Input Audit — 2026-09-28

## Scope

Current code evidence for voice input and generated audio through LocalAI. Synthesis `text` is not a reference transcript.

## Evidence

### `codecs/tts.go`

- `EncodeRequest` accepts synthesis `text` (modality `TEXT`, optional `text/plain` media type), optional per-request `voice` (modality `AUDIO`, optional `audio/*` media type), and optional `parameters` (modality `JSON`, optional `application/json` media type).
- `ttsValidateVoiceInput` requires audio modality, an optional WAV media type, and valid nonempty PCM WAV content.
- Supported parameters: `language`, `instructions`, and `ref_text`, each as a nonempty string. Other names trigger `ttsUnsupportedParameterFailure`.
- `DecodeResponse` calls `validPCMWAV` on generated audio output, not on voice input. `validPCMWAV` checks RIFF/WAVE header, `fmt` chunk with PCM format, and `data` chunk.

### `vibevoice_layout.go`

- `confinedVibeVoiceRolePaths` validates a static built-in model role `voice` file (alongside `model` and `tokenizer`) for `BuiltInModelNameTTS`. This is distinct from the per-request `voice` input.

### `tts_protocol.go`

- The private adapter stages per-request voice bytes in a temporary WAV file, sends its path as protobuf `Voice`, and removes the input and output files after invocation. Wire tests inspect the staged WAV during the fake gRPC call and verify cleanup.
- `ttsProtocolRequest` maps `text`, `model`, staged `voice` path, `language`, and `instructions` to the LocalAI gRPC `TTSRequest` proto sent to `/backend.Backend/TTS`, and maps `ref_text` to its `Params` field.

### `backendregistry/registry.go`

- `Records` lists `localai-vibevoice` as the published VibeVoice backend artifact. No Qwen3-TTS or IndexTTS backend ID is present.

## Live GPU Results — 2026-09-28

The Windows host reports an NVIDIA GeForce RTX 4090, and WSL exposes
`nvidia-smi`. Docker Desktop's Linux engine had remained stopped; this
test used WSL without Docker. WSL Ubuntu installed the official LocalAI
v4.10.0 Linux amd64 binary in `/home/andre/you-localai-probe`.

### Qwen3-TTS 0.6B Q4 (`cuda12-qwen3-tts-cpp`)

- Model `localai@qwen3-tts-cpp-0.6b-base-q4` and backend
  `cuda12-qwen3-tts-cpp` were installed via the LocalAI gallery. The
  server's `/v1/models` listed it.
- `POST /v1/audio/speech` with a server-local 24 kHz reference WAV path
  and `ref_text` returned HTTP 200 and a 130604-byte mono 24 kHz PCM WAV
  (2.72 seconds, non-silent).
- `nvidia-smi` showed the `qwen3-tts-cpp` backend PID 1886 using ~6302 MiB.

### IndexTTS 2.5 (`cuda12-audio-cpp`)

- Model `localai@audio-cpp-indextts-2.5` (original dtype GGUF) and backend
  `cuda12-audio-cpp` were installed via the LocalAI gallery. The server's
  `/v1/models` listed it.
- `POST /v1/audio/speech` with a server-local reference WAV path and
  `ref_text` returned HTTP 200 and a 126508-byte mono 22050 Hz PCM WAV
  (2.867664 seconds, non-silent).
- `nvidia-smi` showed the `audio.cpp` backend PID 5576 using ~13824 MiB.

### Repository gRPC probes

Temporary Go probes imported this repository's
`localai.NewPinnedTTSBackend` and `codecs.TTSRequest`, sent reference
audio bytes plus `ref_text` to the live backend gRPC endpoints, and
returned validated WAVs:

| Backend | Bytes | Sample rate | Duration |
|---------|-------|-------------|----------|
| Qwen3-TTS | 80684 | 24000 Hz | 1.68 s |
| IndexTTS 2.5 | 209452 | 22050 Hz | 4.748481 s |

No `.you-model-tts*` staging files remained. The temporary probe source
was removed.

These results validate upstream LocalAI GPU execution and this
repository's gRPC adapter against live backends. They do not validate
speech content or voice similarity, backend consumption of `ref_text`
beyond protocol delivery, this repository's pinned artifact publication,
or the Windows first-use lifecycle for Qwen/Index (the registry still
only publishes VibeVoice).

## Reproduce with standalone LocalAI

Install both gallery models and start the server:

```sh
local-ai models install localai@qwen3-tts-cpp-0.6b-base-q4
local-ai models install localai@audio-cpp-indextts-2.5
local-ai run --address=127.0.0.1:18080
```

Then POST to `/v1/audio/speech` with a server-local absolute WAV path for `voice` and a matching `ref_text`:

```sh
curl -X POST http://127.0.0.1:18080/v1/audio/speech \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-tts-cpp-0.6b-base-q4",
    "input": "Hello, this is a test.",
    "voice": "/absolute/path/to/reference.wav",
    "ref_text": "Reference transcript for voice cloning."
  }' --output speech.wav
```

Notes:

- The CUDA backend is chosen by LocalAI's gallery/hardware resolution on this host.
- To test IndexTTS, use `"model": "audio-cpp-indextts-2.5"` in the request.
- The `voice` path must be visible to the LocalAI process.
- These commands are for standalone LocalAI validation, not managed `you models pull` support.

## Unverified

- Speech content and voice similarity for either backend.
- Backend consumption of `ref_text` beyond protocol delivery.
- This repository's pinned artifact publication and Windows first-use
  lifecycle for Qwen/Index (registry still only publishes VibeVoice).
- Managed TTS first-use lifecycle and native Windows GPU remain unverified;
  managed ASR, embeddings, and LLM evidence is recorded in [the invocation
  audit](localai-gpu-invocation-audit-2026-09-28.md).

## Upstream configuration evidence

LocalAI's [model configuration reference](https://github.com/mudler/LocalAI/blob/master/docs/content/advanced/model-configuration.md) describes `tts.voice` and `tts.audio_path` defaults, overridden by a request voice. Its [TTS feature guide](https://localai.io/docs/features/text-to-audio/) identifies Qwen3-TTS cloning backends and the `index_tts2` family through audio.cpp. These sources describe LocalAI's current interface; they do not prove compatibility with this repository's pinned VibeVoice artifact.

## Verification Next Steps

1. Run `go test ./pkg/services/models/internal/backends/localai/...` to confirm existing TTS codec and protocol tests pass.
2. Run `go test ./pkg/services/models/internal/backendregistry/...` to confirm registry tests pass.
3. If adding a new TTS backend, add unit tests for `EncodeRequest`/`DecodeResponse` and `ttsProtocolRequest` mapping before wiring.
4. For real-execution validation, run the Windows-only integration harness (`tests/integration/models/tts_real_windows_test.go`) against a running LocalAI binary.
