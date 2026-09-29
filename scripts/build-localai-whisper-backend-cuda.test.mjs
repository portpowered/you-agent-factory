import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// Focused static contract for the native Windows CUDA Whisper backend build.
// The script itself only runs on a CUDA MSVC host, so this file asserts the
// script's pinned markers without compiling anything.

test("the Windows CUDA Whisper script owns the whisper/cuda/cublas identity", async () => {
	const script = await readFile("scripts/build-localai-whisper-backend-cuda.ps1", "utf8");
	assert.match(script, /localai-whisper\/windows-amd64-cuda/);
	assert.match(script, /BUILD_TYPE=cublas/);
	assert.match(script, /LOCALAI_ROOT/);
	assert.match(script, /PACKAGE_ROOT/);
	assert.match(script, /Visual Studio 17 2022/);
	assert.match(script, /-DGGML_CUDA=ON/);
	assert.match(script, /-DGGML_BACKEND_DL=ON/);
	assert.match(script, /-DBUILD_SHARED_LIBS=ON/);
	assert.match(script, /-DCMAKE_WINDOWS_EXPORT_ALL_SYMBOLS=ON/);
	assert.match(script, /add_library\(gowhisper SHARED cpp\/gowhisper\.cpp\)/);
	assert.doesNotMatch(script, /-DBUILD_SHARED_LIBS=OFF/);
	assert.match(script, /--target', 'gowhisper'/);
	assert.match(script, /libgowhisper\.dll/);
	assert.match(script, /ggml-cuda\.dll/);
	assert.match(script, /cudart64_13\.dll/);
});

test("the Windows CUDA Whisper script verifies pins, toolchain, and payload", async () => {
	const script = await readFile("scripts/build-localai-whisper-backend-cuda.ps1", "utf8");
	assert.match(script, /verify-source/);
	assert.match(script, /WHISPER_CPP_VERSION/);
	assert.match(script, /WHISPER_REPO/);
	assert.match(script, /submodule', 'update', '--init', '--recursive', '--depth', '1', '--single-branch/);
	assert.match(script, /whisper\.cpp checkout does not match the pinned commit/);
	assert.match(script, /localai-backend-windows-patch\.mjs/);
	assert.match(script, /localai-whisper/);
	assert.match(script, /verify-payload/);
	assert.match(script, /'metadata'/);
	assert.match(script, /localai-backend-startup-smoke\.go/);
	assert.match(script, /whisper\.exe/);
	assert.match(script, /Build the gowhisper CMake SHARED library with the CUDA backend enabled/);
});

test("the Windows CUDA Whisper script reuses shared CUDA guards without llama coupling", async () => {
	const script = await readFile("scripts/build-localai-whisper-backend-cuda.ps1", "utf8");
	assert.match(script, /build-localai-backend-cuda-paths\.ps1/);
	assert.match(script, /Remove-GeneratedDirectory -Path \$packageRoot -Parent \$whisperRoot -ExpectedName 'package'/);
	assert.match(script, /GGML_BACKEND_DL requires BUILD_SHARED_LIBS/);
	assert.match(script, /Get-ChildItem -LiteralPath \$whisperBuild -Recurse -File -Filter \$name/);
	assert.match(script, /Find-CudaRuntimeFile -CudaRoot \$cudaRoot -Name \$name/);
	assert.match(script, /Find-CudaRuntimeFile -CudaRoot \$cudaRoot -Name 'cudart64_13\.dll'/);
	assert.match(script, /dumpbin \/dependents/);
	assert.match(script, /CUDA runtime DLL was not staged/);
	assert.match(script, /LOCALAI_WINDOWS_CUDA_MSVC_PROBE_OK/);
	assert.doesNotMatch(script, /llama-cpp/);
	assert.doesNotMatch(script, /grpc-server/);
	assert.doesNotMatch(script, /hw_grpc_proto/);
	assert.doesNotMatch(script, /getopt/);
});

test("the Windows CUDA Whisper script generates pinned Go protobuf bindings before building the wrapper", async () => {
	const script = await readFile("scripts/build-localai-whisper-backend-cuda.ps1", "utf8");
	assert.match(script, /libprotoc 31\.1/);
	assert.match(script, /protoc-gen-go-grpc@1958fcbe2ca8bd93af633f11e97d44e567e945af/);
	assert.match(script, /protoc-gen-go@v1\.34\.2/);
	assert.match(script, /--go_out=pkg\/grpc\/proto\//);
	assert.match(script, /--go-grpc_out=pkg\/grpc\/proto\//);
	assert.match(script, /backend\/backend\.proto/);
	assert.ok(script.indexOf("'backend_grpc.pb.go'") < script.indexOf("Invoke-Checked go @('build'"));
});
