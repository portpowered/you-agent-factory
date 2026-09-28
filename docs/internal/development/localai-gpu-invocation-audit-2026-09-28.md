# LocalAI GPU Invocation Audit — 2026-09-28

## Environment

WSL Ubuntu with the official LocalAI v4.10.0 Linux amd64 binary on an RTX
4090. Docker Desktop's Linux engine was unavailable, so WSL was used without
Docker. Gallery CUDA backends and models were installed under
`/home/andre/you-localai-probe`.

## ASR — `whisper-base-en` (`cuda12-whisper`)

- `POST /v1/audio/transcriptions` on repository
  `tests/fixtures/localai/asr/localai-asr-known.wav` returned HTTP 200 and
  text `Zero.`; the fixture duration is 0.6435 s.
- `nvidia-smi` showed the backend GPU process at PID 708.
- Repository `NewPinnedASRBackend` live gRPC returned `Zero.` and one segment;
  staging was cleaned.
- A WSL-built `you` binary invoked built-in `asr` through managed
  `cuda12-whisper` and returned transcript `Zero.`; the invocation repeated
  successfully with `--offline`.

## Embeddings — `qwen3-embedding-0.6b` (`cuda12-llama-cpp`)

- HTTP `/v1/embeddings` returned 1024 values.
- Repository `NewPinnedEmbeddingBackend` live gRPC also returned 1024 values.
- GPU process at PID 1335.
- Managed `you models invoke embed --operation EMBED --input text="Find similar work"`
  now returns 1024 values from the Linux CUDA gallery backend. The same call
  with `--offline` returns 1024 values from the installed backend and model.
- The first managed attempt failed at `LoadModel`: `run.sh` needed
  `LLAMACPP_GRPC_SERVERS=1` to choose gRPC, but the gRPC executable parsed
  that value as an invalid RPC endpoint. Direct launch then exposed
  `GGML_ASSERT(cplan->n_threads > 0)` because the protobuf subset omitted
  LocalAI's `ModelOptions.Threads` field 15. The direct executable, absent
  launcher switch, positive thread count, and CUDA GPU-layer request are now
  exercised by the opt-in live load test and full CLI invocation.

## LLM — `gemma-4-e4b-it-qat-q4_0` (`cuda12-llama-cpp`)

- HTTP `/v1/chat/completions` returned `READY`.
- GPU process at PID 1697.
- Managed `you models pull llm` installed the 5.97 GB model and projector from
  the built-in source. `you models invoke llm --operation OMNI --input
  prompt="Reply READY"` returned `READY` through the installed Linux
  `cuda12-llama-cpp` gallery backend. The same invocation with `--offline`
  returned `READY` using cached artifacts.

## OMNI codec

- The repository OMNI codec initially failed with empty text.
- A raw gRPC request using pinned `PredictOptions` `Tokens=16`,
  `UseTokenizerTemplate=true`, and one user `Message` returned `READY`.
- The checked-in protobuf subset/mapping were extended to send these fields
  with 256 default tokens; live repository `OmniCodec` then returned `READY`
  plus usage.

## Related

TTS Qwen3 and IndexTTS results are in
[localai-tts-reference-audio-audit-2026-09-28.md](localai-tts-reference-audio-audit-2026-09-28.md).

## Limitation

The WSL-built `you` binary proves managed CUDA backend first-use and offline
reuse for ASR, embeddings, and LLM. Managed end-to-end TTS and native Windows
GPU execution remain unverified. The host is Windows; the CUDA backend
processes in this audit ran under WSL Ubuntu.
