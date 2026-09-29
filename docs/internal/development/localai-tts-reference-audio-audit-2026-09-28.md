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

## Managed WSL verification

- Managed WSL `you models pull index-tts2.5` installed `cuda12-audio-cpp`
  and the 7,885,093,568-byte GGUF, reporting `READY`. A subsequent offline
  managed reference-audio invoke completed through the CUDA backend after
  passing audio.cpp's `backend:best` load option. The output was a non-silent
  56,364-byte mono WAV at 22,050 Hz (1.277 seconds); runtime evidence
  reported `INVOKE` and terminal `COMPLETED`. `nvidia-smi` showed GPU memory
  rise during the request. Speech content and voice similarity were not
  evaluated.
- Managed WSL root operator model `qwen3-tts-0.6b` used source directory
  `file:///home/andre/you-localai-probe/models/qwen3-tts-cpp-0.6b-base-q4`
  (two GGUF files), backend `localai-qwen3-tts-cpp`, and a managed pull
  returned `ALREADY_READY`/`READY` with gallery `cuda12-qwen3-tts-cpp`
  installed. A managed `models invoke --offline --operation TTS` with the
  `reference-zero-24k.wav` input and `ref_text` `Zero.` returned exit 0,
  producing a non-silent 103,724-byte mono 24,000 Hz WAV (51,840 frames,
  2.160 seconds, 50,657 nonzero samples). A concurrent 180-sample
  `nvidia-smi` probe during a repeat successful invocation saw GPU memory
  rise from 1,518 MiB to 6,458 MiB and utilization peak at 97%; this is
  GPU execution evidence. Speech content, voice similarity, and actual use
  of `ref_text` beyond protocol delivery were not evaluated.

## Direct Windows Models CLI probe — 2026-09-29

The current `you-ffce01616e.exe` binary accepted the repository WAV fixture
through `--input voice=@tests/fixtures/localai/asr/localai-asr-known.wav`
alongside `--input text=Read this line.` in a direct `models invoke tts
--operation TTS` request. It pulled the built-in VibeVoice model from its
effective definition, returned exit code 0, and produced an `audio/wav`
artifact of 32,044 bytes. A subsequent `models inspect tts` reported
`READY`/`INSTALLED` with 1,714,226,944 cached bytes. This proves file-backed
voice input and generated audio for the built-in Windows path; GPU use, voice
similarity, and `ref_text` behavior were not measured. The JSON CLI response
included the raw WAV content as a long text field, so future probes should
use an output mapping and inspect the file rather than print the full response.

## Direct Windows Models HTTP probe — 2026-09-29

A headless server from the installed `you-fe24e4b392.exe` binary accepted a
direct Windows HTTP multipart `POST /models/invocations` request for built-in
`tts` (VibeVoice, offline) with a voice WAV file. It returned HTTP 200; the
output carried `contentBase64` with no `content` text field. The decoded WAV
was 166,444 bytes, equal to `artifact.sizeBytes`, with RIFF/WAVE markers and
a 166,400-byte `data` chunk containing 154,144 nonzero data bytes. Its SHA-256
`0088c84b5e4c38bfa79cd35f5efcbe64c8f976d012574632c989ac11ce05d96f` matched
the artifact digest. `ref_text` was omitted because it is undocumented for
built-in VibeVoice. This supersedes the earlier probe note that JSON bytes
were not preserved. GPU use and voice similarity were not measured.

MCP live probe session `93d3ba76-1dfc-483d-acba-395a2cc90a6b`.

## Unverified

- Speech content and voice similarity for either backend.
- Backend consumption of `ref_text` beyond protocol delivery.
- This repository's pinned artifact publication and Windows first-use
  lifecycle for Qwen/Index (registry still only publishes VibeVoice).
- Native Windows GPU execution remains unverified. Managed built-in VibeVoice
  TTS on WSL is verified separately in [the invocation
  audit](localai-gpu-invocation-audit-2026-09-28.md); managed Qwen3
  reference-audio synthesis is verified for protocol delivery and GPU
  execution, but speech content, voice similarity, and actual use of
  `ref_text` beyond protocol delivery remain unverified.

## Upstream configuration evidence

LocalAI's [model configuration reference](https://github.com/mudler/LocalAI/blob/master/docs/content/advanced/model-configuration.md) describes `tts.voice` and `tts.audio_path` defaults, overridden by a request voice. Its [TTS feature guide](https://localai.io/docs/features/text-to-audio/) identifies Qwen3-TTS cloning backends and the `index_tts2` family through audio.cpp. These sources describe LocalAI's current interface; they do not prove compatibility with this repository's pinned VibeVoice artifact.

## Verification Next Steps

1. Run `go test ./pkg/services/models/internal/backends/localai/...` to confirm existing TTS codec and protocol tests pass.
2. Run `go test ./pkg/services/models/internal/backendregistry/...` to confirm registry tests pass.
3. If adding a new TTS backend, add unit tests for `EncodeRequest`/`DecodeResponse` and `ttsProtocolRequest` mapping before wiring.
4. For real-execution validation, run the Windows-only integration harness (`tests/integration/models/tts_real_windows_test.go`) against a running LocalAI binary.
