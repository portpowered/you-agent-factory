param(
    [Parameter(Mandatory)][string]$LocalAIRoot,
    [Parameter(Mandatory)][string]$SourceRoot,
    [Parameter(Mandatory)][string]$BuildRoot,
    [Parameter(Mandatory)][string]$PackageRoot,
    [string]$CudaRoot = 'C:\Program Files\NVIDIA GPU Computing Toolkit\CUDA\v13.3',
    [string]$CudaArchitectures = '89-real;75-virtual'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'build-localai-backend-cuda-paths.ps1')
function Invoke-Checked {
    param([string]$File, [string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$File failed with exit code $LASTEXITCODE" }
}
$localaiCommit = 'b224c96db6f4b87306a33a808650bfce63b12588'
$sourceCommit = '6dcc586e5073fd6e85ee5728e75f0903d6c70c6c'
$ggmlCommit = '9be313313c8ecb9488911bd64550190e3ed80f38'
if ((& git -C $LocalAIRoot rev-parse HEAD).Trim() -cne $localaiCommit) { throw "LocalAI checkout must be $localaiCommit" }
if (-not (Test-Path -LiteralPath $SourceRoot)) {
    Invoke-Checked git @('clone', 'https://github.com/predict-woo/qwen3-asr.cpp.git', $SourceRoot)
}
Invoke-Checked git @('-C', $SourceRoot, 'checkout', '--detach', $sourceCommit)
Invoke-Checked git @('-C', $SourceRoot, 'submodule', 'update', '--init', '--recursive')
if ((& git -C (Join-Path $SourceRoot 'ggml') rev-parse HEAD).Trim() -cne $ggmlCommit) { throw "GGML checkout must be $ggmlCommit" }
$nativeBuild = Join-Path $BuildRoot 'native'
$shimBuild = Join-Path $BuildRoot 'bridge'
$cudaToolset = $CudaRoot.Replace('\', '/')
Invoke-Checked cmake @('-S', $SourceRoot, '-B', $nativeBuild, '-G', 'Visual Studio 17 2022', '-A', 'x64', '-T', "cuda=$cudaToolset",
    '-DBUILD_SHARED_LIBS=ON', '-DGGML_BACKEND_DL=OFF', '-DGGML_NATIVE=OFF', '-DGGML_CUDA=ON', "-DCMAKE_CUDA_ARCHITECTURES=$CudaArchitectures", '-DQWEN3_ASR_TEST=OFF')
Invoke-Checked cmake @('--build', $nativeBuild, '--config', 'Release', '--target', 'qwen3-asr', '--parallel', '4')
$bridgeSource = Join-Path $PSScriptRoot 'localai-qwen3-asr-backend'
Invoke-Checked cmake @('-S', $bridgeSource, '-B', $shimBuild, '-G', 'Visual Studio 17 2022', '-A', 'x64', "-DQWEN3_ASR_SOURCE=$SourceRoot", "-DQWEN3_ASR_BUILD=$nativeBuild")
Invoke-Checked cmake @('--build', $shimBuild, '--config', 'Release', '--parallel', '4')
Invoke-Checked (Join-Path $shimBuild 'Release/qwen3-asr-seam-test.exe') @()
$backendRoot = Join-Path $LocalAIRoot 'backend\go\qwen3-asr-cpp'
New-Item -ItemType Directory -Path $backendRoot -Force | Out-Null
New-Item -ItemType Directory -Path $PackageRoot -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $bridgeSource 'main.go.in') -Destination (Join-Path $backendRoot 'main.go') -Force
Copy-Item -LiteralPath (Join-Path $bridgeSource 'main_test.go.in') -Destination (Join-Path $backendRoot 'main_test.go') -Force
# Generate the existing pinned LocalAI ABI; no new protobuf contract is added.
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
        New-Item -ItemType Directory -Path 'pkg/grpc/proto' -Force | Out-Null
        Invoke-Checked protoc @('--experimental_allow_proto3_optional', '-Ibackend/',
            "--plugin=protoc-gen-go=$toolsRoot\protoc-gen-go.exe", "--plugin=protoc-gen-go-grpc=$toolsRoot\protoc-gen-go-grpc.exe",
            '--go_out=pkg/grpc/proto/', '--go_opt=paths=source_relative', '--go-grpc_out=pkg/grpc/proto/', '--go-grpc_opt=paths=source_relative', 'backend/backend.proto')
    } finally { Pop-Location }
    $env:CGO_ENABLED = '0'
    Invoke-Checked go @('test', '-C', $backendRoot, './')
    Invoke-Checked go @('build', '-C', $backendRoot, '-o', (Join-Path $PackageRoot 'qwen3-asr-cpp.exe'), './')
} finally { $env:GOBIN = $previousGoBin; $env:CGO_ENABLED = $previousCgo }
Copy-Item -LiteralPath (Join-Path $shimBuild 'Release/goqwen3asrcpp.dll') -Destination (Join-Path $PackageRoot 'libgoqwen3asrcpp.dll') -Force
Get-ChildItem -LiteralPath (Join-Path $nativeBuild 'Release') -Filter '*.dll' | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $PackageRoot -Force }
New-Item -ItemType Directory -Path (Join-Path $PackageRoot 'assets') -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $SourceRoot 'assets/korean_dict_jieba.dict') -Destination (Join-Path $PackageRoot 'assets/korean_dict_jieba.dict') -Force
New-Item -ItemType Directory -Path (Join-Path $PackageRoot 'licenses') -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $SourceRoot 'LICENSE') -Destination (Join-Path $PackageRoot 'licenses/qwen3-asr-MIT.txt') -Force
Copy-Item -LiteralPath (Join-Path $SourceRoot 'ggml/LICENSE') -Destination (Join-Path $PackageRoot 'licenses/ggml-MIT.txt') -Force
Copy-Item -LiteralPath (Join-Path $LocalAIRoot 'LICENSE') -Destination (Join-Path $PackageRoot 'licenses/LocalAI-MIT.txt') -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot '../LICENSE.md') -Destination (Join-Path $PackageRoot 'licenses/you-agent-factory.txt') -Force
foreach ($name in @('cudart64_13.dll', 'cublas64_13.dll', 'cublasLt64_13.dll')) {
    $runtimePath = Find-CudaRuntimeFile -CudaRoot $CudaRoot -Name $name
    if (-not $runtimePath) { throw "missing CUDA runtime $name" }
    Copy-Item -LiteralPath $runtimePath -Destination $PackageRoot -Force
}
$vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
$vsRoot = (& $vswhere -latest -products '*' -version '[17.0,18.0)' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath | Select-Object -First 1)
$redist = Get-ChildItem -LiteralPath (Join-Path $vsRoot 'VC/Redist/MSVC') -Directory | Where-Object Name -Match '^14\.' | Sort-Object Name -Descending | Select-Object -First 1
if (-not $redist) { throw 'MSVC redistributables are required' }
Get-ChildItem -LiteralPath (Join-Path $redist.FullName 'x64/Microsoft.VC143.CRT') -Filter '*.dll' | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $PackageRoot -Force }
