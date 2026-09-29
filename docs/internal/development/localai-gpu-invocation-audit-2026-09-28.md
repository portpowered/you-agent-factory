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

## TTS — built-in VibeVoice GGUF (`cuda12-vibevoice-cpp`)

- `you models pull tts` installed the model, tokenizer, and voice files on WSL
  Ubuntu. The initial gallery mapping selected `cuda12-vibevoice`, a Python
  Hugging Face backend incompatible with this GGUF bundle; model loading
  failed after interpreting the literal `tts` as a remote model ID.
- The resolver now selects `cuda12-vibevoice-cpp` (and `cpu-vibevoice-cpp` for
  CPU). A rebuilt binary invoked TTS with `--offline` and wrote a valid WAV:
  mono, 24 kHz, 64,000 frames, 2.67 seconds. This verifies managed first use
  with the Linux CUDA gallery archive and reuse of cached model artifacts.

## OMNI codec

- The repository OMNI codec initially failed with empty text.
- A raw gRPC request using pinned `PredictOptions` `Tokens=16`,
  `UseTokenizerTemplate=true`, and one user `Message` returned `READY`.
- The checked-in protobuf subset/mapping were extended to send these fields
  with 256 default tokens; live repository `OmniCodec` then returned `READY`
  plus usage.

## Direct Models HTTP multipart audio/video — 2026-09-29 UTC

- MCP session `63268445-1f69-4970-abaa-1d7cf850a282` ran the WSL
  `/home/andre/you-localai-probe/models-http-probe.sh` against
  `POST /models/invocations` with `offline:true` and multipart WAV/MP4 files.
  The installed `llm` model detail reports cached Gemma 4 E4B
  `Q4_K_M` and projector assets as ready; the probe used the
  `cuda12-llama-cpp` backend.
- The known WAV saying “Zero” returned text `zero` with HTTP 200. The MP4
  displaying `BLUE` returned `BLUE` with HTTP 200. These are live
  direct-endpoint semantic results, beyond the transport byte-path tests.
- The `nvidia-smi` sampling logs recorded GPU compute process PID 4838 during
  both requests (process name shown as `[Not Found]` under WSL). They do not
  measure GPU utilization. No independent checksum of the bytes received by
  the backend was taken.
- Evidence: `models-http-{audio,video}-response.json`, matching `curl.txt`
  and `gpu.log` files, `models-http-probe.sh`, and
  `models-http-model-detail.json` under `/home/andre/you-localai-probe/`.

## Related

TTS Qwen3 and IndexTTS results are in
[localai-tts-reference-audio-audit-2026-09-28.md](localai-tts-reference-audio-audit-2026-09-28.md).

## Native CUDA archive availability — 2026-09-29

The managed published resolver selects a matching CUDA archive before a CPU
archive on CUDA-capable Windows and Linux amd64 hosts. The checked-in manifest
has no CUDA entry. The latest public LocalAI backend release
(`localai-backends-v1-17273d7dbb61dba3f7bfdfd6e05bd90231cf1c1224cf5556668abea1bac91106`)
lists CPU/Metal archives for llama.cpp, Whisper, and VibeVoice, with no CUDA
archive. The local release matrix defines Linux and Windows CUDA llama.cpp
builds, but both require named self-hosted runners; the repository Actions
runner API reported `total_count: 0` during this audit. Consequently this
worktree can select a valid published CUDA release but cannot obtain a native
Windows CUDA backend from the current public release. A verified build and
publication are still needed.

## Limitation

The WSL-built `you` binary proves managed CUDA backend first-use and offline
reuse for ASR, embeddings, LLM, and TTS. Native Windows GPU execution
remains unverified, while managed WSL Qwen3/IndexTTS reference-audio
synthesis is documented in the linked audit; speech content, voice
similarity, and actual `ref_text` use are unverified. The host is
Windows; the CUDA gallery backends in this audit ran under WSL Ubuntu.
