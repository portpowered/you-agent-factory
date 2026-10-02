param(
    [Parameter(Mandatory)][string]$LocalAIRoot,
    [Parameter(Mandatory)][string]$BuildRoot,
    [Parameter(Mandatory)][string]$PackageRoot,
    [string]$CudaRoot = 'C:\Program Files\NVIDIA GPU Computing Toolkit\CUDA\v13.3',
    [string]$CudaArchitectures = '89-real;75-virtual'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$CudaRoot = $CudaRoot.Replace('\', '/')
. (Join-Path $PSScriptRoot 'build-localai-backend-cuda-paths.ps1')

function Invoke-Checked {
    param([string]$File, [string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$File failed with exit code $LASTEXITCODE" }
}

$localaiCommit = 'b224c96db6f4b87306a33a808650bfce63b12588'
$qwenCommit = 'd17c33d4ee2f56d15f9ca8a1bb82f7389305f838'
if ((& git -C $LocalAIRoot rev-parse HEAD).Trim() -cne $localaiCommit) {
    throw "LocalAI checkout must be $localaiCommit"
}
$backendRoot = Join-Path $LocalAIRoot 'backend\go\qwen3-tts-cpp'
$sourceRoot = Join-Path $backendRoot 'sources\qwentts.cpp'
if (-not (Test-Path -LiteralPath $sourceRoot)) {
    Invoke-Checked git @('clone', '--recurse-submodules', 'https://github.com/ServeurpersoCom/qwentts.cpp.git', $sourceRoot)
}
Invoke-Checked git @('-C', $sourceRoot, 'checkout', '--detach', $qwenCommit)
Invoke-Checked git @('-C', $sourceRoot, 'submodule', 'update', '--init', '--recursive')

# The pinned direct CUDA decode graphs gather Q6_K embedding rows. Backport
# upstream's exact Q6_K gather kernel rather than silently changing model weights.
$ggmlRoot = Join-Path $sourceRoot 'ggml'
$patch = Join-Path $PSScriptRoot 'localai-qwen3-tts-cuda-getrows.patch'
& git -C $ggmlRoot apply --unidiff-zero --reverse --check $patch 2>$null
if ($LASTEXITCODE -ne 0) { Invoke-Checked git @('-C', $ggmlRoot, 'apply', '--unidiff-zero', $patch) }

# The native decoder must observe EOS; exhausting the caller/model budget is
# a failed generation, never a successfully published truncated waveform.
$eosPatch = Join-Path $PSScriptRoot 'localai-qwen3-tts-eos.patch'
& git -C $sourceRoot apply --unidiff-zero --reverse --check $eosPatch 2>$null
if ($LASTEXITCODE -ne 0) { Invoke-Checked git @('-C', $sourceRoot, 'apply', '--unidiff-zero', $eosPatch) }

$main = Join-Path $backendRoot 'main.go'
$loader = Join-Path $backendRoot 'localai-backend-library_windows.go'
if (-not (Test-Path -LiteralPath $loader)) {
    Invoke-Checked node @((Join-Path $PSScriptRoot 'localai-backend-windows-patch.mjs'), $main, $loader, 'libgoqwen3ttscpp.dll', 'localai-qwen3-tts-cpp')
}
$errorPatch = Join-Path $PSScriptRoot 'localai-qwen3-tts-errors.patch'
& git -C $LocalAIRoot apply --unidiff-zero --reverse --check $errorPatch 2>$null
if ($LASTEXITCODE -ne 0) { Invoke-Checked git @('-C', $LocalAIRoot, 'apply', '--unidiff-zero', $errorPatch) }
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'localai-qwen3-tts-errors_test.go.in') -Destination (Join-Path $backendRoot 'termination_test.go') -Force

$cmakePath = Join-Path $backendRoot 'CMakeLists.txt'
$cmakeText = [IO.File]::ReadAllText($cmakePath)
$cmakeText = $cmakeText.Replace('add_library(goqwen3ttscpp MODULE cpp/goqwen3ttscpp.cpp)', 'add_library(goqwen3ttscpp SHARED cpp/goqwen3ttscpp.cpp)')
[IO.File]::WriteAllText($cmakePath, $cmakeText, [Text.UTF8Encoding]::new($false))
Invoke-Checked cmake @('-S', $backendRoot, '-B', $BuildRoot, '-G', 'Visual Studio 17 2022', '-A', 'x64', '-T', "cuda=$CudaRoot",
    '-DBUILD_SHARED_LIBS=ON', '-DCMAKE_WINDOWS_EXPORT_ALL_SYMBOLS=ON', '-DGGML_BACKEND_DL=OFF', '-DGGML_NATIVE=OFF',
    '-DGGML_CUDA=ON', "-DCMAKE_CUDA_ARCHITECTURES=$CudaArchitectures", '-DCMAKE_CXX_STANDARD=17')
Invoke-Checked cmake @('--build', $BuildRoot, '--config', 'Release', '--target', 'goqwen3ttscpp', '--parallel', '4')

New-Item -ItemType Directory -Path $PackageRoot -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $LocalAIRoot 'pkg\grpc\proto') -Force | Out-Null
$toolsRoot = Join-Path $BuildRoot 'proto-tools'
New-Item -ItemType Directory -Path $toolsRoot -Force | Out-Null
$previousGoBin = $env:GOBIN
$previousCgo = $env:CGO_ENABLED
try {
    $env:GOBIN = $toolsRoot
    Invoke-Checked go @('install', 'google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2')
    Invoke-Checked go @('install', 'google.golang.org/grpc/cmd/protoc-gen-go-grpc@1958fcbe2ca8bd93af633f11e97d44e567e945af')
    Push-Location $LocalAIRoot
    try {
        Invoke-Checked protoc @('--experimental_allow_proto3_optional', '-Ibackend/',
            "--plugin=protoc-gen-go=$toolsRoot\protoc-gen-go.exe", "--plugin=protoc-gen-go-grpc=$toolsRoot\protoc-gen-go-grpc.exe",
            '--go_out=pkg/grpc/proto/', '--go_opt=paths=source_relative', '--go-grpc_out=pkg/grpc/proto/',
            '--go-grpc_opt=paths=source_relative', 'backend/backend.proto')
    } finally { Pop-Location }
    $env:CGO_ENABLED = '0'
    # Upstream e2e_test.go directly uses Unix-only Dlopen. This CPU regression
    # compiles the actual Windows backend without executing GPU inference.
    Invoke-Checked go @('test', '-C', $backendRoot, 'audio.go', 'options.go', 'goqwen3ttscpp.go',
        'main.go', 'localai-backend-library_windows.go', 'termination_test.go')
    Invoke-Checked go @('build', '-C', $backendRoot, '-o', (Join-Path $PackageRoot 'qwen3-tts-cpp.exe'), './')
} finally {
    $env:GOBIN = $previousGoBin
    $env:CGO_ENABLED = $previousCgo
}
Copy-Item -LiteralPath (Join-Path $BuildRoot 'Release\goqwen3ttscpp.dll') -Destination (Join-Path $PackageRoot 'libgoqwen3ttscpp.dll') -Force
Get-ChildItem -LiteralPath (Join-Path $BuildRoot 'Release') -Filter 'ggml*.dll' | ForEach-Object {
    Copy-Item -LiteralPath $_.FullName -Destination $PackageRoot -Force
}
foreach ($name in @('cudart64_13.dll', 'cublas64_13.dll', 'cublasLt64_13.dll')) {
    $runtimePath = Find-CudaRuntimeFile -CudaRoot $CudaRoot -Name $name
    if (-not $runtimePath) { throw "missing CUDA runtime $name" }
    Copy-Item -LiteralPath $runtimePath -Destination $PackageRoot -Force
}
$vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
$vsRoot = (& $vswhere -latest -products '*' -version '[17.0,18.0)' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath | Select-Object -First 1)
$redist = Get-ChildItem -LiteralPath (Join-Path $vsRoot 'VC\Redist\MSVC') -Directory | Where-Object Name -Match '^14\.' | Sort-Object Name -Descending | Select-Object -First 1
if (-not $redist) { throw 'MSVC redistributables are required' }
Get-ChildItem -LiteralPath (Join-Path $redist.FullName 'x64\Microsoft.VC143.CRT') -Filter '*.dll' | ForEach-Object {
    Copy-Item -LiteralPath $_.FullName -Destination $PackageRoot -Force
}

$licenses = Join-Path $PackageRoot 'licenses'
New-Item -ItemType Directory -Path $licenses -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $LocalAIRoot 'LICENSE') -Destination (Join-Path $licenses 'LocalAI.LICENSE') -Force
Copy-Item -LiteralPath (Join-Path $sourceRoot 'LICENSE') -Destination (Join-Path $licenses 'qwentts.LICENSE') -Force
Copy-Item -LiteralPath (Join-Path $ggmlRoot 'LICENSE') -Destination (Join-Path $licenses 'ggml.LICENSE') -Force
