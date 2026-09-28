# LocalAI TTS Voice Input Audit — 2026-09-28

## Scope

Current code evidence for voice input and generated audio through LocalAI. Synthesis `text` is not a reference transcript.

## Evidence

### `codecs/tts.go`

- `EncodeRequest` accepts synthesis `text` (modality `TEXT`, optional `text/plain` media type), optional per-request `voice` (modality `AUDIO`, optional `audio/*` media type), and optional `parameters` (modality `JSON`, optional `application/json` media type).
- `ttsValidateVoiceInput` requires audio modality, optional `audio/*` media type, and nonempty `Content` string. It does not validate PCM WAV format.
- Supported parameters: only `language` and `instructions`. Any other name triggers `ttsUnsupportedParameterFailure`.
- `DecodeResponse` calls `validPCMWAV` on generated audio output, not on voice input. `validPCMWAV` checks RIFF/WAVE header, `fmt` chunk with PCM format, and `data` chunk.

### `vibevoice_layout.go`

- `confinedVibeVoiceRolePaths` validates a static built-in model role `voice` file (alongside `model` and `tokenizer`) for `BuiltInModelNameTTS`. This is distinct from the per-request `voice` input.

### `tts_protocol.go`

- `ttsProtocolRequest` maps `text`, `model`, `voice`, `language`, and `instructions` to the LocalAI gRPC `TTSRequest` proto sent to `/backend.Backend/TTS`.

### `backendregistry/registry.go`

- `Records` lists `localai-vibevoice` as the published VibeVoice backend artifact. No Qwen3-TTS or IndexTTS backend ID is present.

## Unverified

- Qwen3-TTS and IndexTTS support (no code path, backend wiring, or tests).
- Reference transcript behavior.
- GPU real execution.

## Verification Next Steps

1. Run `go test ./pkg/services/models/internal/backends/localai/...` to confirm existing TTS codec and protocol tests pass.
2. Run `go test ./pkg/services/models/internal/backendregistry/...` to confirm registry tests pass.
3. If adding a new TTS backend, add unit tests for `EncodeRequest`/`DecodeResponse` and `ttsProtocolRequest` mapping before wiring.
4. For real-execution validation, run the Windows-only integration harness (`tests/integration/models/tts_real_windows_test.go`) against a running LocalAI binary.
