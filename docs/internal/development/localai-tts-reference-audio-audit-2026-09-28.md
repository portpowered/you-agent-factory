# LocalAI TTS Reference Audio/Text Audit — 2026-09-28

## Scope
Current code evidence for voice input and generated audio through LocalAI. Synthesis `text` is not a reference transcript.

## Evidence

### `codecs/tts.go`

- `EncodeRequest` (`codecs/tts.go:60`) accepts synthesis `text` (modality `text/plain`), optional per-request `voice` (modality `audio/*`), and optional `parameters` (modality `application/json`). Voice input validation is via `ttsValidateVoiceInput` (line 197).
- `ttsValidateVoiceInput` (`codecs/tts.go:197-207`) checks per-request voice modality `audio/*`, optional `audio/*` MIME type, and nonempty `Content` string. It does **not** validate PCM WAV format, encoding, or reference transcript.
- `validPCMWAV` (`codecs/tts.go:310-316`) checks generated OUTPUT: WAV header (`RIFF/WAVE`), `fmt` chunk with PCM format, and `data` chunk with valid PCM data. Emits `ttsMalformedResponseFailure` on failure.
- `DecodeResponse` (`codecs/tts.go:116`) validates response media type is `audio/wav`/`audio/wave`/`audio/x-wav`, size ≤ `MaxTTSAudioBytes` (16 MiB), and invokes `validPCMWAV()`. Returns `models.InferenceContent{Name: "audio", Modality: ModalityAudio, MediaType: "audio/wav", Content: <bytes>}`.
- Supported parameters: only `language` and `instructions` (`codecs/tts.go:27`). Any other parameter name triggers `ttsUnsupportedParameterFailure`.
- `ttsValidateTextInput` (`codecs/tts.go:187-195`) ensures `text` is non-empty `text/plain`; does not check WAV or reference transcript.

### `vibevoice_layout.go`

- `confinedVibeVoiceRolePaths` (`vibevoice_layout.go:51-79`) validates a static built-in model role `voice` file (alongside `model` and `tokenizer`) for `BuiltInModelNameTTS`. This is distinct from the per-request `voice` input validated by `ttsValidateVoiceInput`.

### `tts_protocol.go`

- `ttsProtocolRequest` (`tts_protocol.go:199-221`) maps `text`, `model`, `voice`, `language`, and `instructions` to the LocalAI gRPC `TTSRequest` proto sent to `/backend.Backend/TTS`. Any other parameter name returns `ttsInvalidParameterFailure`.
- Backend invocation (`invokeTTSProtocol`, `tts_protocol.go:126-147`) sends the protobuf request; expects `Result{Success: true}`.
- Output staging (`reserveTTSOutput`, `tts_protocol.go:165-197`) creates temporary `*.you-model-tts-*.wav` file.
- Response reading (`readTTSResponse`, `tts_protocol.go:149-163`) reads the temporary WAV file and returns `codecs.TTSResponse{MediaType: "audio/wav"}`.

### `backendregistry/registry.go`

- `Records` lists `localai-vibevoice` as the published VibeVoice backend artifact. No Qwen3-TTS or IndexTTS backend ID is present.

## Unverified

- Qwen3-TTS and IndexTTS support: no code path, backend wiring, or tests in this repository.
- Reference transcript behavior: not demonstrated in any code path.
- GPU real execution: no code or test demonstrates GPU operation.

## Verification Next Steps

1. Run `go test ./pkg/services/models/internal/backends/localai/...` to confirm existing TTS codec and protocol tests pass.
2. Run `go test ./pkg/services/models/internal/backendregistry/...` to confirm registry tests pass.
3. If adding a new TTS backend, add unit tests for `EncodeRequest`/`DecodeResponse` and `ttsProtocolRequest` mapping before wiring.
4. For real-execution validation, run the Windows-only integration harness (`tests/integration/models/tts_real_windows_test.go`) against a running LocalAI binary.