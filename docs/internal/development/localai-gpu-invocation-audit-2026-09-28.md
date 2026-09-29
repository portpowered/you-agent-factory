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

## Manual Windows CUDA release — user-requested, IN PROGRESS (2026-09-29)

Host Windows RTX 4090, CUDA 13.3, VS2022;
`scripts/build-localai-backend-cuda.ps1 -ProbeOnly` passed a real
CUDA+MSVC compile probe. A 10-entry manual manifest path is accepted by the
runtime (MCP `463a65d1-91b9-4c48-a47f-7e54481d197c`); DLL lookup fixed to
toolkit `bin/x64` (commit `7d14f7fe27`); 9 baseline archives
(116,246,859 bytes) downloaded/verified outside the repo; pinned LocalAI
source prepared/verified; portable Go 1.25.4/CMake 3.31.10/Node 22.14.0
installed (Codex `0e98837b-6010-45fc-b819-980b8e051c17`). First native
build failed in recursive gRPC nested bloaty submodule
(`fatal: '$GIT_DIR' too big`); fixed nonrecursive checkout (commit
`050552eef8`) and retry idempotency (commit `8a1e770682`). A native-build
retry is RUNNING (PID 42004, checked live) past CMake gRPC configuration.
Archive, publish, and native GPU inference are NOT complete; do not treat
this section as native Windows GPU evidence.

Manual build update (2026-09-29): the retry gRPC step completed. The
pinned llama CMake step then reported `backend.pb.h` missing
`port_def.inc` while that header exists, caused by empty
`Protobuf_INCLUDE_DIRS`. OpenCode MCP session
`1ce72c0f-86c3-4a93-a63e-987b74b1d82e` fixed the build script by
exporting the pinned gRPC include via MSVC `INCLUDE` and correcting
`Protobuf_DIR`; workflow tests report 17 pass, 1 skip. The original
native build is still compiling CUDA objects; final success is NOT
claimed. The manual 10-entry assembler was committed as `2fc69146ec`;
no release published.

Native Windows CUDA full build succeeded 2026-09-29 (local): three OpenCode
MCP sessions completed bounded build-script fixes — `a74faae5-9467-4918-a370-e0151408db28`
(CMake proto target fix), `5ca45a17-74e3-40a0-b783-df216e30786a` (Windows
getopt fix), `1a94e9ff-f430-4f24-954b-1aca704cd102` (explicit pinned CUDA
runtime staging). `grpc-server` and `ggml-cuda` built; health/startup and
`verify-payload` passed. ZIP 584,983,117 bytes, SHA-256
`c9322c65b36eea345c32f6e29c3b2dcd4187c288fbf145b8f1b400d1541649dc`.
Manual 10-entry bundle tag
`localai-backends-v1-b2637a4aed57fb1312f436e56a6baec5f23656de8b5ae245b112398f01cbeaa7`
staged locally; Go manifest decode/resolver test passed; full `make lint`
24/24. No GitHub release published yet; no managed GPU inference claimed.

## Manual Windows CUDA release — PUBLISHED + native EMBED validation (2026-09-29, final)

Prior "No GitHub release published yet" notes above are preserved and now
superseded by this entry. Normal (non-draft) release published:
https://github.com/portpowered/you-agent-factory/releases/tag/localai-backends-v1-b2637a4aed57fb1312f436e56a6baec5f23656de8b5ae245b112398f01cbeaa7
Tag targets pushed branch commit `48412fc1c6e3f5e23ac906ae2cf0361baabf7e7c`.
All 11 hosted assets matched staged manifest sizes/digests before publish.
Windows CUDA ZIP SHA-256
`c9322c65b36eea345c32f6e29c3b2dcd4187c288fbf145b8f1b400d1541649dc`.
Fresh isolated native Windows CLI managed EMBED probe PASS: 1024 values;
EMBED cache selected the new `windows-amd64-cuda` ZIP and the actual cached
SHA matched the published manifest. `nvidia-smi` compute-app samples include
`llama-cpp-cpu-all.exe` PID 10592; GPU usage sampled max 53%; WDDM
per-process memory N/A. Evidence dir
`C:\Users\andre\AppData\Local\Temp\you-native-cuda-aca54b5bf4f44e75b147760509c2517c`.
LLM not separately invoked; ASR/TTS still CPU; no Linux CUDA in this release.

## Native Windows managed LLM invocation — 2026-09-29

Native Windows managed LLM pull succeeded: Gemma-4 E4B `Q4_K_M`
(4,977,171,584 bytes) plus mmproj (990,372,672 bytes). The cached Windows
CUDA llama backend archive SHA-256
`c9322c65b36eea345c32f6e29c3b2dcd4187c288fbf145b8f1b400d1541649dc` matched the
published manifest and the on-disk file. OMNI invocation exited 0 and returned
`READY.` with usage 3 tokens / 21 prompt tokens. During the invoke,
`nvidia-smi` observed `llama-cpp-cpu-all.exe` PID 16904 and GPU memory rose
from ~4,224 to ~9,817 MiB then fell; this supports GPU offload, but WDDM
per-process memory is N/A and no CUDA init trace was captured, so exact GPU
compute is unproven. Evidence under
`C:\Users\andre\AppData\Local\Temp\you-native-cuda-aca54b5bf4f44e75b147760509c2517c`
in `llm-pull.json`, `llm-invoke4.json`, and `llm-gpu-*samples4.txt`. ASR/TTS
native Windows remain CPU; no Linux CUDA archive exists in this release.

## Manual Windows Whisper CUDA build (2026-09-29)

The pinned Windows Whisper CUDA build was configured with the broad set of
eight native GPU architectures plus compute-75 PTX. MSVC compiled 122 of 139
CUDA sources, but its `argsort.cu` host compiler consumed more than an hour
without producing an object. The process was stopped after checking its exact
build-process tree; no archive was published from that attempt. For the
manual RTX 4090 test release, the pinned Windows architecture set is now
`89-real;75-virtual`, retaining native Ada code and PTX for supported newer
GPUs. The packaging revision remains 4; the changed host-toolchain pin is
included in the new archive metadata and release fingerprint. The narrowed
build compiled all 139 CUDA sources and linked `ggml-cuda.dll`, `whisper.dll`,
and `gowhisper.dll`. The pinned LocalAI checkout lacks generated
`pkg/grpc/proto` sources, so the build script now installs LocalAI's pinned Go
protobuf plugins in an isolated directory and generates those sources with
`protoc 31.1`. The original CMake `MODULE` library had no exported symbols on
Windows; the script changes the pinned declaration to `SHARED` and enables
CMake's Windows symbol export before building. The native backend startup
health check passed, all five expected `gowhisper` exports were present, and
the package was staged with CUDA and MSVC runtime DLLs. The Windows CUDA ZIP
is 448,033,531 bytes with SHA-256
`4d0eb5cc09e94ab820335af5bcfff1634e72c4206f6975848eb12f4b81ba2c9c`.
The first published archive was selected and downloaded by a fresh Windows
resolver, but `LoadModel` terminated the backend with `GGML_ASSERT(device)`.
The dynamically loaded CUDA backend was present; `ggml-cpu.dll` was missing.
The first release was marked prerelease after this finding. Staging the
pinned build's `ggml-cpu.dll` made a direct gRPC `LoadModel` call succeed, with
the backend reporting CUDA0 on the RTX 4090. The build script now stages both
dynamic backends. A repaired archive was rebuilt at 448,333,120 bytes with
SHA-256 `17541d98c8d00ee0e6dfd9699ad3a886e2a9ed7469d4260cf9ed7452efd6cc58`.
The repaired release was published as
`localai-backends-v1-a8cb28e829e7b41b5ea960bad46fd039658f97bf90fbfb126b83e6cc157c4d1b`.
Its hosted manifest SHA-256 is
`f01e4a5f0d5c803167535048a740c1b4c9a49f2eb047594c464af03096130a25`.
A fresh public `you models invoke asr` selected and downloaded the repaired
448,333,120-byte archive, then returned transcript `Zero.` and a JSON segment
for the known audio fixture. A direct native gRPC probe also reported
`whisper_backend_init_gpu: using CUDA0 backend` and returned the same
transcript. This validates the Windows CUDA model load and transcription path.

## Manual Windows VibeVoice CUDA build (2026-09-29)

The pinned VibeVoice source compiled with MSVC and CUDA 13.3 after disabling
`GGML_BACKEND_DL`; the source directly links `ggml-cuda`, which cannot be a
CMake `MODULE_LIBRARY`. The first built `govibevoicecpp.dll` had no exported
`vv_capi_*` functions. The build script now makes that target `SHARED` and
provides an explicit DEF file with the six functions used by the Go wrapper.
The native health and payload checks passed, and `dumpbin /exports` found all
six functions. A direct Windows gRPC test loaded the pinned 0.5B TTS bundle
on the RTX 4090 and generated an 83,244-byte WAV from “Hello from CUDA.”
`nvidia-smi` listed the VibeVoice backend process while the model was loaded.
The staged CUDA archive is 448,293,919 bytes, SHA-256
`e3f9c42e1d4433044857b1e08422f43efc564c858a73474e993d17f9ffa83342`.
The manual release and public `you models invoke tts` test are pending.
