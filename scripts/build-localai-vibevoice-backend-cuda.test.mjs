import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// Focused static contract for the native Windows CUDA VibeVoice backend build.
// The script itself only runs on a CUDA MSVC host, so this file asserts the
// script's pinned markers without compiling anything. It mirrors
// build-localai-whisper-backend-cuda.test.mjs and stays disjoint from the
// Whisper build (separate backend/go/vibevoice-cpp subtrees) so both CUDA
// builds can run in parallel.

test("the Windows CUDA VibeVoice script owns the vibevoice/cuda/cublas identity", async () => {
	const script = await readFile("scripts/build-localai-vibevoice-backend-cuda.ps1", "utf8");
	assert.match(script, /localai-vibevoice\/windows-amd64-cuda/);
	assert.match(script, /BUILD_TYPE=cublas/);
	assert.match(script, /LOCALAI_ROOT/);
	assert.match(script, /PACKAGE_ROOT/);
	assert.match(script, /Visual Studio 17 2022/);
	assert.match(script, /-DGGML_CUDA=ON/);
	assert.match(script, /-DVIBEVOICE_GGML_CUDA=ON/);
	assert.match(script, /-DGGML_BACKEND_DL=OFF/);
	assert.match(script, /-DBUILD_SHARED_LIBS=ON/);
	assert.doesNotMatch(script, /-DBUILD_SHARED_LIBS=OFF/);
	assert.match(script, /--target', 'govibevoicecpp'/);
	assert.match(script, /libgovibevoicecpp\.dll/);
	assert.match(script, /ggml-cuda\.dll/);
	assert.match(script, /cudart64_13\.dll/);
});

test("the Windows CUDA VibeVoice script verifies pins, toolchain, and payload", async () => {
	const script = await readFile("scripts/build-localai-vibevoice-backend-cuda.ps1", "utf8");
	assert.match(script, /verify-source/);
	assert.match(script, /VIBEVOICE_CPP_VERSION/);
	assert.match(script, /VIBEVOICE_REPO/);
	assert.match(script, /submodule', 'update', '--init', '--recursive', '--depth', '1', '--single-branch/);
	assert.match(script, /vibevoice\.cpp checkout does not match the pinned commit/);
	assert.match(script, /localai-backend-windows-patch\.mjs/);
	assert.match(script, /localai-vibevoice/);
	assert.match(script, /verify-payload/);
	assert.match(script, /'metadata'/);
	assert.match(script, /localai-backend-startup-smoke\.go/);
	assert.match(script, /vibevoice-cpp\.exe/);
	assert.match(script, /Build shared ggml backends with CUDA enabled/);
	assert.match(script, /add_library\(govibevoicecpp SHARED cpp\/govibevoicecpp\.cpp \$\{CMAKE_CURRENT_BINARY_DIR\}\/vibevoice-exports\.def\)/);
	assert.match(script, /vv_capi_tts_stream/);
	assert.match(script, /-DCMAKE_WINDOWS_EXPORT_ALL_SYMBOLS=ON/);
	assert.match(script, /WHOLEARCHIVE:\$<TARGET_FILE:vibevoice>/);
	assert.match(script, /libprotoc 31\.1/);
	assert.match(script, /protoc-gen-go-grpc@1958fcbe2ca8bd93af633f11e97d44e567e945af/);
	assert.match(script, /protoc-gen-go@v1\.34\.2/);
	assert.ok(script.indexOf("'backend_grpc.pb.go'") < script.indexOf("Invoke-Checked go @('build'"));
});

test("the Windows CUDA VibeVoice script reuses shared CUDA guards without whisper/llama coupling", async () => {
	const script = await readFile("scripts/build-localai-vibevoice-backend-cuda.ps1", "utf8");
	assert.match(script, /build-localai-backend-cuda-paths\.ps1/);
	assert.match(script, /Remove-GeneratedDirectory -Path \$packageRoot -Parent \$vibevoiceRoot -ExpectedName 'package'/);
	assert.match(script, /MODULE_LIBRARY, which CMake cannot link/);
	assert.match(script, /Get-ChildItem -LiteralPath \$vibevoiceBuild -Recurse -File -Filter \$name/);
	assert.match(script, /Find-CudaRuntimeFile -CudaRoot \$cudaRoot -Name \$name/);
	assert.match(script, /Find-CudaRuntimeFile -CudaRoot \$cudaRoot -Name 'cudart64_13\.dll'/);
	assert.match(script, /dumpbin \/dependents/);
	assert.match(script, /CUDA runtime DLL was not staged/);
	assert.match(script, /LOCALAI_WINDOWS_CUDA_MSVC_PROBE_OK/);
	assert.doesNotMatch(script, /libgowhisper/);
	assert.doesNotMatch(script, /whisper\.cpp/);
	assert.doesNotMatch(script, /llama-cpp/);
	assert.doesNotMatch(script, /grpc-server/);
	assert.doesNotMatch(script, /hw_grpc_proto/);
	assert.doesNotMatch(script, /getopt/);
});

test("the Windows CUDA VibeVoice script guards the streaming callback ABI", async () => {
	const script = await readFile("scripts/build-localai-vibevoice-backend-cuda.ps1", "utf8");
	assert.match(script, /govibevoicecpp\.go/);
	assert.match(script, /purego\\.NewCallback\\\(func/);
	assert.match(script, /uintptr-returning purego callback/);
	assert.match(script, /Unary TTS does not exercise the/);
	assert.match(script, /VIBEVOICECPP_LIBRARY/);
	assert.match(script, /CGO_ENABLED/);
});
