param([switch]$ProbeOnly)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'build-localai-backend-cuda-paths.ps1')

function Invoke-Checked {
    param([string]$File, [string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$File failed with exit code $LASTEXITCODE" }
}

function Initialize-Msvc {
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
    if (-not (Test-Path -LiteralPath $vswhere)) { throw 'Visual Studio Installer vswhere.exe is required' }
    $installation = (& $vswhere -latest -products '*' -version '[17.0,18.0)' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath | Select-Object -First 1)
    if (-not $installation) { throw 'Visual Studio 2022 x64 C++ Build Tools are required' }
    $vcvars = Join-Path $installation 'VC\Auxiliary\Build\vcvars64.bat'
    if (-not (Test-Path -LiteralPath $vcvars)) { throw "missing $vcvars" }
    $variables = & $env:ComSpec /d /c "call `"$vcvars`" >nul && set"
    if ($LASTEXITCODE -ne 0) { throw 'vcvars64.bat failed' }
    foreach ($line in $variables) {
        if ($line -match '^([^=]+)=(.*)$') { [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process') }
    }
    if ($env:VSCMD_ARG_TGT_ARCH -ne 'x64') { throw 'MSVC developer environment is not x64' }
    return $installation
}

function Assert-Tool {
    param([string]$File, [string[]]$Arguments, [string]$Expected)
    $output = (& $File @Arguments 2>&1 | Out-String)
    if ($LASTEXITCODE -ne 0 -or -not $output.Contains($Expected)) { throw "$File must report $Expected; observed $output" }
}

function Invoke-CudaProbe {
    $root = Join-Path ([IO.Path]::GetTempPath()) ("localai-cuda-probe-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $root | Out-Null
    try {
        Set-Content -LiteralPath (Join-Path $root 'CMakeLists.txt') -Value @'
cmake_minimum_required(VERSION 3.25)
project(localai_cuda_probe LANGUAGES CXX CUDA)
set(CMAKE_CUDA_STANDARD 17)
add_library(localai_cuda_probe STATIC probe.cu)
'@
        Set-Content -LiteralPath (Join-Path $root 'probe.cu') -Value '__global__ void localai_cuda_probe() {}'
        Invoke-Checked cmake @('-S', $root, '-B', (Join-Path $root 'build'), '-G', 'Visual Studio 17 2022', '-A', 'x64', '-T', "cuda=$cudaRoot", "-DCMAKE_CUDA_ARCHITECTURES=$cudaArchitectures")
        Invoke-Checked cmake @('--build', (Join-Path $root 'build'), '--config', 'Release')
        Write-Output 'LOCALAI_WINDOWS_CUDA_MSVC_PROBE_OK'
    } finally {
        Remove-GeneratedDirectory -Path $root -Parent ([IO.Path]::GetTempPath()) -ExpectedName ([IO.Path]::GetFileName($root))
    }
}

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$config = Get-Content -LiteralPath (Join-Path $repositoryRoot '.github\localai-backend-artifacts.json') -Raw | ConvertFrom-Json
$null = Initialize-Msvc
$cudaRoot = $env:CUDA_PATH
if (-not $cudaRoot) {
    $nvccCommand = Get-Command nvcc.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $nvccCommand) { throw 'CUDA_PATH or nvcc.exe on PATH is required' }
    $cudaRoot = Split-Path -Parent (Split-Path -Parent $nvccCommand.Source)
    $env:CUDA_PATH = $cudaRoot
}
$nvcc = Join-Path $cudaRoot 'bin\nvcc.exe'
if (-not (Test-Path -LiteralPath $nvcc)) { throw "missing $nvcc" }
Assert-Tool $nvcc @('--version') "release $($config.hostToolchain.windows.cudaVersion),"
# Ship native code for the Windows test GPU (RTX 4090, sm_89) plus compute_75
# PTX for JIT compatibility. Keep this identical to the pinned artifact config.
$cudaArchitectures = '89-real;75-virtual'
if ($config.hostToolchain.windows.cudaArchitecture -cne $cudaArchitectures) { throw 'unexpected CUDA architecture set' }
if ($ProbeOnly) { Invoke-CudaProbe; exit 0 }

foreach ($name in @('LOCALAI_ROOT', 'BACKEND_ID', 'TARGET_ID', 'BUILD_TYPE')) {
    if (-not [Environment]::GetEnvironmentVariable($name)) { throw "$name is required" }
}
if ($env:BACKEND_ID -ne 'localai-whisper' -or $env:TARGET_ID -ne 'windows-amd64-cuda' -or $env:BUILD_TYPE -ne 'cublas') {
    throw 'this MSVC build only supports localai-whisper/windows-amd64-cuda with BUILD_TYPE=cublas'
}
Assert-Tool cmake @('--version') $config.toolchain.cmakeVersion
Assert-Tool go @('version') "go$($config.toolchain.goVersion)"
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'verify-source', '--config', (Join-Path $repositoryRoot '.github\localai-backend-artifacts.json'), '--localai-root', $env:LOCALAI_ROOT, '--backend', $env:BACKEND_ID, '--target', $env:TARGET_ID)

$whisperBackend = ($config.backends | Where-Object { $_.id -eq 'localai-whisper' })
if (-not $whisperBackend) { throw 'pinned config is missing the localai-whisper backend' }
$whisperRoot = Join-Path $env:LOCALAI_ROOT 'backend\go\whisper'
$whisperMakefile = Join-Path $whisperRoot 'Makefile'
if (-not (Test-Path -LiteralPath $whisperMakefile)) { throw "missing $whisperMakefile" }
$whisperMakefileText = Get-Content -LiteralPath $whisperMakefile -Raw
foreach ($pin in @(
    @('WHISPER_REPO', $whisperBackend.sourceRepository),
    @('WHISPER_CPP_VERSION', $whisperBackend.sourceCommit)
)) {
    $pattern = "(?m)^$([regex]::Escape($pin[0]))\?=\s*([^\s#]+)"
    $match = [regex]::Match($whisperMakefileText, $pattern)
    if (-not $match.Success -or $match.Groups[1].Value -cne $pin[1]) {
        throw "whisper Makefile $($pin[0]) must be $($pin[1])"
    }
}

# Obtain the exact pinned whisper.cpp source when it is absent or stale.
# The pinned Makefile fetches the same commit with
# `git submodule update --init --recursive --depth 1 --single-branch`, so the
# MSVC build keeps those flags; whisper.cpp vendors ggml (with the CUDA
# backend) as a recursive submodule. The pinned source commit is unchanged
# (verify-source above pins LOCALAI_ROOT); this only materializes the
# configured checkout under LOCALAI_ROOT.
$whisperSource = Join-Path $whisperRoot 'sources\whisper.cpp'
$whisperOriginUrl = $whisperBackend.sourceRepository
if (-not (Test-Path -LiteralPath $whisperSource)) {
    New-Item -ItemType Directory -Path $whisperSource -Force | Out-Null
    Invoke-Checked git @('-C', $whisperSource, 'init')
}
if (-not (Test-Path -LiteralPath (Join-Path $whisperSource '.git'))) {
    Invoke-Checked git @('-C', $whisperSource, 'init')
}
$existingWhisperOrigin = (& git -C $whisperSource remote get-url origin 2>$null | Out-String)
if ($LASTEXITCODE -ne 0) {
    Invoke-Checked git @('-C', $whisperSource, 'remote', 'add', 'origin', $whisperOriginUrl)
} elseif ($existingWhisperOrigin.Trim() -cne $whisperOriginUrl) {
    throw "whisper.cpp origin remote must be $whisperOriginUrl; refusing to reset a conflicting checkout"
}
$whisperHead = (& git -C $whisperSource rev-parse HEAD 2>$null | Out-String)
if ($LASTEXITCODE -ne 0 -or $whisperHead.Trim() -cne $whisperBackend.sourceCommit) {
    Invoke-Checked git @('-C', $whisperSource, 'fetch', '--depth', '1', 'origin', $whisperBackend.sourceCommit)
    Invoke-Checked git @('-C', $whisperSource, 'checkout', '--detach', 'FETCH_HEAD')
}
Invoke-Checked git @('-C', $whisperSource, 'submodule', 'update', '--init', '--recursive', '--depth', '1', '--single-branch')
if ((& git -C $whisperSource rev-parse HEAD).Trim() -ne $whisperBackend.sourceCommit) { throw 'whisper.cpp checkout does not match the pinned commit' }

# purego v0.10.0 intentionally exposes Dlopen only on Unix. The pinned
# LocalAI whisper entrypoint uses that API unconditionally, so the Windows
# build adds the small build-tagged loader shim from
# scripts/localai-backend-windows-patch.mjs (the same shim used by the
# windows-amd64 CPU whisper build), staging the CMake MODULE as
# libgowhisper.dll. The shim is retry-aware: an already-patched working tree
# is reused instead of patching twice.
$whisperMain = Join-Path $whisperRoot 'main.go'
$whisperLoader = Join-Path $whisperRoot 'localai-backend-library_windows.go'
if (-not (Test-Path -LiteralPath $whisperMain)) { throw "missing $whisperMain" }
$whisperMainText = Get-Content -LiteralPath $whisperMain -Raw
if ($whisperMainText.Contains('loadBackendLibrary(libName)')) {
    Write-Output 'whisper Go loader shim is already patched for this retry'
} else {
    Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-windows-patch.mjs'), $whisperMain, $whisperLoader, 'libgowhisper.dll', 'localai-whisper')
}

# The pinned LocalAI CMake target is a MODULE with no dllexport declarations.
# MSVC links that target with an empty export table, so purego cannot resolve
# load_model at startup. A Windows SHARED target plus CMake's automatic export
# definition exposes its C ABI. Patch only the exact pinned declaration and
# accept the same patched declaration on a retry.
$whisperCmakeFile = Join-Path $whisperRoot 'CMakeLists.txt'
$moduleDeclaration = 'add_library(gowhisper MODULE cpp/gowhisper.cpp)'
$sharedDeclaration = 'add_library(gowhisper SHARED cpp/gowhisper.cpp)'
$whisperCmakeText = Get-Content -LiteralPath $whisperCmakeFile -Raw
if ($whisperCmakeText.Contains($moduleDeclaration)) {
    if (([regex]::Matches($whisperCmakeText, [regex]::Escape($moduleDeclaration))).Count -ne 1) {
        throw 'expected exactly one pinned gowhisper MODULE declaration'
    }
    [IO.File]::WriteAllText($whisperCmakeFile, $whisperCmakeText.Replace($moduleDeclaration, $sharedDeclaration), [Text.UTF8Encoding]::new($false))
} elseif (-not $whisperCmakeText.Contains($sharedDeclaration)) {
    throw 'pinned gowhisper CMake declaration changed; reassess the Windows export patch'
}

# Build the gowhisper CMake SHARED library with the CUDA backend enabled. GGML_NATIVE
# stays OFF so the DLL runs on any x64 host; GGML_BACKEND_DL=ON keeps the
# backends (including ggml-cuda.dll, staged below) loadable at runtime instead
# of linking them statically into gowhisper. BUILD_SHARED_LIBS stays ON
# because the pinned ggml configure requires it for GGML_BACKEND_DL
# (ggml/src/CMakeLists.txt:189); configuring OFF fails with
# 'GGML_BACKEND_DL requires BUILD_SHARED_LIBS'.
$whisperBuild = Join-Path $whisperRoot 'build-windows-cuda'
$whisperCmakeArgs = @('-S', $whisperRoot, '-B', $whisperBuild, '-G', 'Visual Studio 17 2022', '-A', 'x64', '-T', "cuda=$cudaRoot",
    '-DCMAKE_CXX_STANDARD=17', '-DBUILD_SHARED_LIBS=ON', '-DCMAKE_WINDOWS_EXPORT_ALL_SYMBOLS=ON',
    '-DGGML_NATIVE=OFF', '-DGGML_BACKEND_DL=ON', '-DGGML_CUDA=ON',
    "-DCMAKE_CUDA_ARCHITECTURES=$cudaArchitectures",
    "-DCMAKE_CUDA_COMPILER=$nvcc")
Invoke-Checked cmake $whisperCmakeArgs
Invoke-Checked cmake @('--build', $whisperBuild, '--config', 'Release', '--target', 'gowhisper', '--parallel', '4')

# PACKAGE_ROOT overrides the default backend package directory when set
# (for example, when the caller stages the payload elsewhere). Otherwise the
# payload lands in the pinned backend package directory used by the package
# contract and verify-payload below.
$packageRoot = Join-Path $whisperRoot 'package'
if ($env:PACKAGE_ROOT) {
    $packageRoot = $env:PACKAGE_ROOT
    if ([IO.Path]::GetFileName($packageRoot.TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)) -cne 'package') {
        throw 'PACKAGE_ROOT must resolve to a directory named package'
    }
}
if (Test-Path -LiteralPath $packageRoot) { Remove-GeneratedDirectory -Path $packageRoot -Parent $whisperRoot -ExpectedName 'package' }
New-Item -ItemType Directory -Path $packageRoot -Force | Out-Null
$gowhisperDll = Get-ChildItem -LiteralPath $whisperBuild -Recurse -File | Where-Object { $_.Length -gt 0 -and ($_.Name -ieq 'libgowhisper.dll' -or $_.Name -ieq 'gowhisper.dll') } | Select-Object -First 1
if (-not $gowhisperDll) { throw 'MSVC CUDA build did not produce the gowhisper DLL' }
Copy-Item -LiteralPath $gowhisperDll.FullName -Destination (Join-Path $packageRoot 'libgowhisper.dll') -Force
$cudaBackendDll = Get-ChildItem -LiteralPath $whisperBuild -Recurse -File -Filter 'ggml-cuda.dll' | Where-Object Length -gt 0 | Select-Object -First 1
if (-not $cudaBackendDll) { throw 'MSVC CUDA build did not produce ggml-cuda.dll' }
Copy-Item -LiteralPath $cudaBackendDll.FullName -Destination $packageRoot -Force
# With GGML_BACKEND_DL=ON, CPU is also loaded dynamically. Health succeeds
# without this DLL, but LoadModel aborts when it asks for the CPU device.
$cpuBackendDll = Get-ChildItem -LiteralPath $whisperBuild -Recurse -File -Filter 'ggml-cpu.dll' | Where-Object Length -gt 0 | Select-Object -First 1
if (-not $cpuBackendDll) { throw 'MSVC CUDA build did not produce ggml-cpu.dll' }
Copy-Item -LiteralPath $cpuBackendDll.FullName -Destination $packageRoot -Force

# The pinned whisper Makefile builds the Go entrypoint with CGO_ENABLED=0
# (purego dynamic loading, no cgo); keep that here so whisper.exe loads the
# staged libgowhisper.dll at runtime through WHISPER_LIBRARY.
# LocalAI does not check in pkg/grpc/proto. Its pinned root Makefile generates
# the Go bindings with protoc 31.1 and the two exact Go plugin versions below.
# Put those plugins in this generated build tree, leaving the caller's GOBIN
# and any globally installed plugin versions untouched.
$protocCommand = Get-Command protoc.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $protocCommand) { throw 'protoc 31.1 is required to generate LocalAI Go gRPC bindings' }
$protoc = $protocCommand.Source
Assert-Tool $protoc @('--version') 'libprotoc 31.1'
$protoTools = Join-Path $whisperBuild 'proto-tools'
New-Item -ItemType Directory -Path $protoTools -Force | Out-Null
$previousGoBin = $env:GOBIN
try {
    $env:GOBIN = $protoTools
    Invoke-Checked go @('install', 'google.golang.org/grpc/cmd/protoc-gen-go-grpc@1958fcbe2ca8bd93af633f11e97d44e567e945af')
    Invoke-Checked go @('install', 'google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2')
} finally {
    $env:GOBIN = $previousGoBin
}
$goPlugin = Join-Path $protoTools 'protoc-gen-go.exe'
$grpcPlugin = Join-Path $protoTools 'protoc-gen-go-grpc.exe'
foreach ($pluginPath in @($goPlugin, $grpcPlugin)) {
    if (-not (Test-Path -LiteralPath $pluginPath)) { throw "pinned Go protobuf plugin is missing: $pluginPath" }
}
$protoOutput = Join-Path $env:LOCALAI_ROOT 'pkg\grpc\proto'
New-Item -ItemType Directory -Path $protoOutput -Force | Out-Null
Push-Location $env:LOCALAI_ROOT
try {
    Invoke-Checked $protoc @('--experimental_allow_proto3_optional', '-Ibackend/',
        "--plugin=protoc-gen-go=$goPlugin", "--plugin=protoc-gen-go-grpc=$grpcPlugin",
        '--go_out=pkg/grpc/proto/', '--go_opt=paths=source_relative',
        '--go-grpc_out=pkg/grpc/proto/', '--go-grpc_opt=paths=source_relative',
        'backend/backend.proto')
} finally {
    Pop-Location
}
foreach ($generated in @('backend.pb.go', 'backend_grpc.pb.go')) {
    $generatedPath = Join-Path $protoOutput $generated
    if (-not (Test-Path -LiteralPath $generatedPath) -or (Get-Item -LiteralPath $generatedPath).Length -eq 0) {
        throw "LocalAI Go protobuf generation did not produce $generatedPath"
    }
}
$previousCgo = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    Invoke-Checked go @('build', '-C', $whisperRoot, '-o', (Join-Path $packageRoot 'whisper.exe'), './')
} finally {
    $env:CGO_ENABLED = $previousCgo
}
if (-not (Test-Path -LiteralPath (Join-Path $packageRoot 'libgowhisper.dll'))) { throw 'whisper package is missing libgowhisper.dll' }
if (-not (Test-Path -LiteralPath (Join-Path $packageRoot 'ggml-cuda.dll'))) { throw 'whisper package is missing ggml-cuda.dll' }
if (-not (Test-Path -LiteralPath (Join-Path $packageRoot 'ggml-cpu.dll'))) { throw 'whisper package is missing ggml-cpu.dll' }

# Stage recursively imported DLLs from the Release build tree first (ggml and
# whisper shared imports produced by this build), then the CUDA toolkit and
# MSVC redistributable. Exact file-name matches only; Windows system DLLs
# remain system dependencies and are never copied. Refuse unknown non-system
# imports.
$redist = $env:VCToolsRedistDir
$queue = [Collections.Generic.Queue[string]]::new()
Get-ChildItem -LiteralPath $packageRoot -File | Where-Object Extension -in @('.exe', '.dll') | ForEach-Object { $queue.Enqueue($_.FullName) }
$inspected = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
while ($queue.Count -gt 0) {
    $file = $queue.Dequeue()
    if (-not $inspected.Add($file)) { continue }
    $imports = (& dumpbin /dependents $file 2>&1 | Out-String)
    if ($LASTEXITCODE -ne 0) { throw "dumpbin failed for $file" }
    foreach ($match in [regex]::Matches($imports, '(?im)^\s*([A-Za-z0-9_.-]+\.dll)\s*$')) {
        $name = $match.Groups[1].Value
        $destination = Join-Path $packageRoot $name
        if (Test-Path -LiteralPath $destination) { continue }
        if ($name -match '^(api-ms-win-|ext-ms-win-)') { continue }
        $buildProduced = Get-ChildItem -LiteralPath $whisperBuild -Recurse -File -Filter $name -ErrorAction SilentlyContinue | Where-Object Length -gt 0 | Select-Object -First 1
        $candidate = if ($buildProduced) { $buildProduced.FullName } else { $null }
        if (-not $candidate) {
            $candidate = Find-CudaRuntimeFile -CudaRoot $cudaRoot -Name $name
        }
        if ((-not $candidate) -and $redist) {
            $redistributable = Get-ChildItem -LiteralPath (Join-Path $redist 'x64') -Recurse -File -Filter $name -ErrorAction SilentlyContinue | Select-Object -First 1
            $candidate = if ($redistributable) { $redistributable.FullName } else { $null }
        }
        if ($candidate -and (Test-Path -LiteralPath $candidate)) {
            Copy-Item -LiteralPath $candidate -Destination $destination
            $queue.Enqueue($destination)
        } elseif (-not (Test-Path -LiteralPath (Join-Path $env:WINDIR "System32\$name"))) {
            throw "unresolved Windows runtime dependency $name imported by $file"
        }
    }
}
# nvcc links -cudart static, so no staged PE imports cudart64_13.dll and the
# import scan above never stages it. Copy the pinned runtime explicitly when
# the scan did not already pull it; the generic cudart check below still
# validates the payload contract.
if (-not (Get-ChildItem -LiteralPath $packageRoot -File | Where-Object Name -Match '^cudart64_.*\.dll$')) {
    $pinnedCudartSource = Find-CudaRuntimeFile -CudaRoot $cudaRoot -Name 'cudart64_13.dll'
    if (-not $pinnedCudartSource) { throw 'pinned CUDA runtime cudart64_13.dll was not found under the CUDA toolkit' }
    Copy-Item -LiteralPath $pinnedCudartSource -Destination (Join-Path $packageRoot 'cudart64_13.dll')
}
if (-not (Get-ChildItem -LiteralPath $packageRoot -File | Where-Object Name -Match '^cudart64_.*\.dll$')) { throw 'CUDA runtime DLL was not staged' }
Invoke-Checked go @('run', (Join-Path $repositoryRoot 'scripts\localai-backend-startup-smoke.go'), '--binary', (Join-Path $packageRoot 'whisper.exe'), '--workdir', $packageRoot)
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'verify-payload', '--package-root', $packageRoot, '--binary', 'whisper', '--target', 'windows-amd64-cuda')
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'metadata', '--config', (Join-Path $repositoryRoot '.github\localai-backend-artifacts.json'), '--localai-root', $env:LOCALAI_ROOT, '--backend', $env:BACKEND_ID, '--target', $env:TARGET_ID, '--output', (Join-Path $packageRoot 'build-metadata.json'))
Write-Output "LOCALAI_BACKEND_PACKAGE_OK backend=$env:BACKEND_ID target=$env:TARGET_ID path=$packageRoot"
