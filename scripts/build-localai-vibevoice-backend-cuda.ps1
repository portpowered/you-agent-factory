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
if ($env:BACKEND_ID -ne 'localai-vibevoice' -or $env:TARGET_ID -ne 'windows-amd64-cuda' -or $env:BUILD_TYPE -ne 'cublas') {
    throw 'this MSVC build only supports localai-vibevoice/windows-amd64-cuda with BUILD_TYPE=cublas'
}
Assert-Tool cmake @('--version') $config.toolchain.cmakeVersion
Assert-Tool go @('version') "go$($config.toolchain.goVersion)"
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'verify-source', '--config', (Join-Path $repositoryRoot '.github\localai-backend-artifacts.json'), '--localai-root', $env:LOCALAI_ROOT, '--backend', $env:BACKEND_ID, '--target', $env:TARGET_ID)

$vibevoiceBackend = ($config.backends | Where-Object { $_.id -eq 'localai-vibevoice' })
if (-not $vibevoiceBackend) { throw 'pinned config is missing the localai-vibevoice backend' }
$vibevoiceRoot = Join-Path $env:LOCALAI_ROOT 'backend\go\vibevoice-cpp'
$vibevoiceMakefile = Join-Path $vibevoiceRoot 'Makefile'
if (-not (Test-Path -LiteralPath $vibevoiceMakefile)) { throw "missing $vibevoiceMakefile" }
$vibevoiceMakefileText = Get-Content -LiteralPath $vibevoiceMakefile -Raw
foreach ($pin in @(
    @('VIBEVOICE_REPO', $vibevoiceBackend.sourceRepository),
    @('VIBEVOICE_CPP_VERSION', $vibevoiceBackend.sourceCommit)
)) {
    $pattern = "(?m)^$([regex]::Escape($pin[0]))\?=\s*([^\s#]+)"
    $match = [regex]::Match($vibevoiceMakefileText, $pattern)
    if (-not $match.Success -or $match.Groups[1].Value -cne $pin[1]) {
        throw "vibevoice Makefile $($pin[0]) must be $($pin[1])"
    }
}

# Obtain the exact pinned vibevoice.cpp source when it is absent or stale.
# The pinned Makefile fetches the same commit with `git fetch origin` plus
# `git checkout $(VIBEVOICE_CPP_VERSION)` and
# `git submodule update --init --recursive --depth 1 --single-branch`, so the
# MSVC build keeps the shallow exact-commit fetch plus those recursive
# submodule flags. The pinned source commit is unchanged (verify-source
# above pins LOCALAI_ROOT); this only materializes the configured checkout
# under LOCALAI_ROOT. This build uses backend/go/vibevoice-cpp subtrees only,
# disjoint from the parallel Whisper build's backend/go/whisper subtrees.
$vibevoiceSource = Join-Path $vibevoiceRoot 'sources\vibevoice.cpp'
$vibevoiceOriginUrl = $vibevoiceBackend.sourceRepository
if (-not (Test-Path -LiteralPath $vibevoiceSource)) {
    New-Item -ItemType Directory -Path $vibevoiceSource -Force | Out-Null
    Invoke-Checked git @('-C', $vibevoiceSource, 'init')
}
if (-not (Test-Path -LiteralPath (Join-Path $vibevoiceSource '.git'))) {
    Invoke-Checked git @('-C', $vibevoiceSource, 'init')
}
$existingVibevoiceOrigin = (& git -C $vibevoiceSource remote get-url origin 2>$null | Out-String)
if ($LASTEXITCODE -ne 0) {
    Invoke-Checked git @('-C', $vibevoiceSource, 'remote', 'add', 'origin', $vibevoiceOriginUrl)
} elseif ($existingVibevoiceOrigin.Trim() -cne $vibevoiceOriginUrl) {
    throw "vibevoice.cpp origin remote must be $vibevoiceOriginUrl; refusing to reset a conflicting checkout"
}
$vibevoiceHead = (& git -C $vibevoiceSource rev-parse HEAD 2>$null | Out-String)
if ($LASTEXITCODE -ne 0 -or $vibevoiceHead.Trim() -cne $vibevoiceBackend.sourceCommit) {
    Invoke-Checked git @('-C', $vibevoiceSource, 'fetch', '--depth', '1', 'origin', $vibevoiceBackend.sourceCommit)
    Invoke-Checked git @('-C', $vibevoiceSource, 'checkout', '--detach', 'FETCH_HEAD')
}
Invoke-Checked git @('-C', $vibevoiceSource, 'submodule', 'update', '--init', '--recursive', '--depth', '1', '--single-branch')
if ((& git -C $vibevoiceSource rev-parse HEAD).Trim() -ne $vibevoiceBackend.sourceCommit) { throw 'vibevoice.cpp checkout does not match the pinned commit' }

# purego v0.10.0 intentionally exposes Dlopen only on Unix. The pinned
# LocalAI vibevoice entrypoint uses that API unconditionally, so the Windows
# build adds the small build-tagged loader shim from
# scripts/localai-backend-windows-patch.mjs (the same shim shape used by the
# windows-amd64 CPU vibevoice build), staging the CMake MODULE as
# libgovibevoicecpp.dll. The shim is retry-aware: an already-patched working
# tree is reused instead of patching twice.
$vibevoiceMain = Join-Path $vibevoiceRoot 'main.go'
$vibevoiceLoader = Join-Path $vibevoiceRoot 'localai-backend-library_windows.go'
if (-not (Test-Path -LiteralPath $vibevoiceMain)) { throw "missing $vibevoiceMain" }
$vibevoiceMainText = Get-Content -LiteralPath $vibevoiceMain -Raw
if ($vibevoiceMainText.Contains('loadBackendLibrary(libName)')) {
    Write-Output 'vibevoice Go loader shim is already patched for this retry'
} else {
    Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-windows-patch.mjs'), $vibevoiceMain, $vibevoiceLoader, 'libgovibevoicecpp.dll', 'localai-vibevoice')
}

# The vibevoice streaming callback (streamCB in govibevoicecpp.go) already
# returns uintptr, so the Windows syscall.NewCallback ABI is satisfied without
# a whisper-style void-callback wrapper. UNVERIFIED at runtime in this pass
# (no CUDA build was run); this assertion pins the audited shape so a future
# upstream callback change fails loudly here instead of corrupting the stack.
$vibevoiceCallbackSource = Join-Path $vibevoiceRoot 'govibevoicecpp.go'
if (-not (Test-Path -LiteralPath $vibevoiceCallbackSource)) { throw "missing $vibevoiceCallbackSource" }
$vibevoiceCallbackText = Get-Content -LiteralPath $vibevoiceCallbackSource -Raw
if (([regex]::Matches($vibevoiceCallbackText, 'purego\.NewCallback\(func\([^)]*\) uintptr')).Count -ne 1) {
    throw 'expected exactly one uintptr-returning purego callback in govibevoicecpp.go; reassess the Windows NewCallback ABI shim'
}

# Build the govibevoicecpp CMake MODULE with the CUDA backend enabled. GGML_NATIVE
# stays OFF so the DLL runs on any x64 host. UNVERIFIED CMake behavior in this
# pass (no CUDA build was run): BUILD_SHARED_LIBS=ON with GGML_BACKEND_DL=ON
# mirrors the Whisper CUDA MSVC build so the vendored ggml produces a separate
# loadable ggml-cuda.dll. BUILD_SHARED_LIBS stays ON because the pinned ggml
# configure requires it for GGML_BACKEND_DL
# ('GGML_BACKEND_DL requires BUILD_SHARED_LIBS'), but the Unix vibevoice
# BUILD_SHARED_LIBS=OFF and whether vibevoice.cpp@000e372 honors
# GGML_BACKEND_DL on MSVC is unaudited. Likewise the explicit
# CMAKE_CUDA_ARCHITECTURES override (the backend CMakeLists defaults to
# "75-virtual;80-virtual;86-real;89-real" when undefined), the
# /WHOLEARCHIVE:vibevoice MSVC link branch, and the MSVC compile of
# cpp/govibevoicecpp.cpp are compile-unverified here. By design this script
# fails at the ggml-cuda.dll check below if the configure does not emit a
# separate CUDA backend DLL; reassess the flags then instead of shipping a
# CPU-only payload under a CUDA target.
$vibevoiceBuild = Join-Path $vibevoiceRoot 'build-windows-cuda'
$vibevoiceCmakeArgs = @('-S', $vibevoiceRoot, '-B', $vibevoiceBuild, '-G', 'Visual Studio 17 2022', '-A', 'x64', '-T', "cuda=$cudaRoot",
    '-DCMAKE_CXX_STANDARD=17', '-DBUILD_SHARED_LIBS=ON',
    '-DGGML_NATIVE=OFF', '-DGGML_BACKEND_DL=ON', '-DGGML_CUDA=ON', '-DVIBEVOICE_GGML_CUDA=ON',
    '-DVIBEVOICE_BUILD_TESTS=OFF', '-DVIBEVOICE_BUILD_EXAMPLES=OFF',
    "-DCMAKE_CUDA_ARCHITECTURES=$cudaArchitectures",
    "-DCMAKE_CUDA_COMPILER=$nvcc")
Invoke-Checked cmake $vibevoiceCmakeArgs
Invoke-Checked cmake @('--build', $vibevoiceBuild, '--config', 'Release', '--target', 'govibevoicecpp', '--parallel', '4')

# PACKAGE_ROOT overrides the default backend package directory when set
# (for example, when the caller stages the payload elsewhere). Otherwise the
# payload lands in the pinned backend package directory used by the package
# contract and verify-payload below.
$packageRoot = Join-Path $vibevoiceRoot 'package'
if ($env:PACKAGE_ROOT) {
    $packageRoot = $env:PACKAGE_ROOT
    if ([IO.Path]::GetFileName($packageRoot.TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)) -cne 'package') {
        throw 'PACKAGE_ROOT must resolve to a directory named package'
    }
}
if (Test-Path -LiteralPath $packageRoot) { Remove-GeneratedDirectory -Path $packageRoot -Parent $vibevoiceRoot -ExpectedName 'package' }
New-Item -ItemType Directory -Path $packageRoot -Force | Out-Null
# UNVERIFIED: which MODULE file name the MSVC generator emits
# (libgovibevoicecpp.dll under MinGW versus govibevoicecpp.dll under MSVC);
# accept either build-produced name and stage the canonical package name.
$govibevoiceDll = Get-ChildItem -LiteralPath $vibevoiceBuild -Recurse -File | Where-Object { $_.Length -gt 0 -and ($_.Name -ieq 'libgovibevoicecpp.dll' -or $_.Name -ieq 'govibevoicecpp.dll') } | Select-Object -First 1
if (-not $govibevoiceDll) { throw 'MSVC CUDA build did not produce the govibevoicecpp DLL' }
Copy-Item -LiteralPath $govibevoiceDll.FullName -Destination (Join-Path $packageRoot 'libgovibevoicecpp.dll') -Force
$cudaBackendDll = Get-ChildItem -LiteralPath $vibevoiceBuild -Recurse -File -Filter 'ggml-cuda.dll' | Where-Object Length -gt 0 | Select-Object -First 1
if (-not $cudaBackendDll) { throw 'MSVC CUDA build did not produce ggml-cuda.dll' }
Copy-Item -LiteralPath $cudaBackendDll.FullName -Destination $packageRoot -Force

# The pinned vibevoice Makefile builds the Go entrypoint with CGO_ENABLED=0
# (purego dynamic loading, no cgo); keep that here so vibevoice-cpp.exe loads
# the staged libgovibevoicecpp.dll at runtime through VIBEVOICECPP_LIBRARY.
$previousCgo = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    Invoke-Checked go @('build', '-C', $vibevoiceRoot, '-o', (Join-Path $packageRoot 'vibevoice-cpp.exe'), './')
} finally {
    $env:CGO_ENABLED = $previousCgo
}
if (-not (Test-Path -LiteralPath (Join-Path $packageRoot 'libgovibevoicecpp.dll'))) { throw 'vibevoice package is missing libgovibevoicecpp.dll' }
if (-not (Test-Path -LiteralPath (Join-Path $packageRoot 'ggml-cuda.dll'))) { throw 'vibevoice package is missing ggml-cuda.dll' }

# Stage recursively imported DLLs from the Release build tree first (ggml and
# vibevoice shared imports produced by this build), then the CUDA toolkit and
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
        $buildProduced = Get-ChildItem -LiteralPath $vibevoiceBuild -Recurse -File -Filter $name -ErrorAction SilentlyContinue | Where-Object Length -gt 0 | Select-Object -First 1
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
Invoke-Checked go @('run', (Join-Path $repositoryRoot 'scripts\localai-backend-startup-smoke.go'), '--binary', (Join-Path $packageRoot 'vibevoice-cpp.exe'), '--workdir', $packageRoot)
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'verify-payload', '--package-root', $packageRoot, '--binary', 'vibevoice-cpp', '--target', 'windows-amd64-cuda')
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'metadata', '--config', (Join-Path $repositoryRoot '.github\localai-backend-artifacts.json'), '--localai-root', $env:LOCALAI_ROOT, '--backend', $env:BACKEND_ID, '--target', $env:TARGET_ID, '--output', (Join-Path $packageRoot 'build-metadata.json'))
Write-Output "LOCALAI_BACKEND_PACKAGE_OK backend=$env:BACKEND_ID target=$env:TARGET_ID path=$packageRoot"
