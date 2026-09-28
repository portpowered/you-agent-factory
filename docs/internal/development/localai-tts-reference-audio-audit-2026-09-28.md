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

## Unverified

- Qwen3-TTS and IndexTTS support (no code path, backend wiring, or tests).
- End-to-end voice cloning with a reference WAV and `ref_text`. LocalAI's [release notes](https://github.com/mudler/LocalAI/releases) specify the cross-backend `params.ref_text` contract and request `voice` precedence. The adapter mapping is tested, but synthesis against a running cloning backend is not.
- GPU real execution.

The Windows host reports an NVIDIA GeForce RTX 4090, and WSL exposes
`nvidia-smi`. No `local-ai` binary is installed. Docker Desktop's Linux
engine reported `stopped` and did not create its engine pipe after startup
and restart attempts, so no LocalAI process or GPU inference was run.

## Upstream configuration evidence

LocalAI's [model configuration reference](https://github.com/mudler/LocalAI/blob/master/docs/content/advanced/model-configuration.md) describes `tts.voice` and `tts.audio_path` defaults, overridden by a request voice. Its [TTS feature guide](https://localai.io/features/text-to-audio/index.print.html) identifies Qwen3-TTS cloning backends and the `index_tts2` family through audio.cpp. These sources describe LocalAI's current interface; they do not prove compatibility with this repository's pinned VibeVoice artifact or local GPU execution.

## Verification Next Steps

1. Run `go test ./pkg/services/models/internal/backends/localai/...` to confirm existing TTS codec and protocol tests pass.
2. Run `go test ./pkg/services/models/internal/backendregistry/...` to confirm registry tests pass.
3. If adding a new TTS backend, add unit tests for `EncodeRequest`/`DecodeResponse` and `ttsProtocolRequest` mapping before wiring.
4. For real-execution validation, run the Windows-only integration harness (`tests/integration/models/tts_real_windows_test.go`) against a running LocalAI binary.
