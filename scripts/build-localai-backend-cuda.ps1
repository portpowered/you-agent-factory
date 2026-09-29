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
# Ship native code for the CUDA 13.3 GPU generations plus compute_75 PTX for
# forward JIT compatibility. Keep this identical to the pinned artifact config.
$cudaArchitectures = '75-real;80-real;86-real;89-real;90-real;100-real;110-real;120-real;75-virtual'
if ($config.hostToolchain.windows.cudaArchitecture -cne $cudaArchitectures) { throw 'unexpected CUDA architecture set' }
if ($ProbeOnly) { Invoke-CudaProbe; exit 0 }

foreach ($name in @('LOCALAI_ROOT', 'BACKEND_ID', 'TARGET_ID', 'BUILD_TYPE', 'GRPC_COMMIT', 'BACKEND_SOURCE_COMMIT')) {
    if (-not [Environment]::GetEnvironmentVariable($name)) { throw "$name is required" }
}
if ($env:BACKEND_ID -ne 'localai-llamacpp' -or $env:TARGET_ID -ne 'windows-amd64-cuda' -or $env:BUILD_TYPE -ne 'cublas') {
    throw 'this MSVC build only supports localai-llamacpp/windows-amd64-cuda with BUILD_TYPE=cublas'
}
Assert-Tool cmake @('--version') $config.toolchain.cmakeVersion
Assert-Tool go @('version') "go$($config.toolchain.goVersion)"
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'verify-source', '--config', (Join-Path $repositoryRoot '.github\localai-backend-artifacts.json'), '--localai-root', $env:LOCALAI_ROOT, '--backend', $env:BACKEND_ID, '--target', $env:TARGET_ID)

$grpcRoot = Join-Path $env:LOCALAI_ROOT 'backend\cpp\grpc'
$grpcSource = Join-Path $grpcRoot 'grpc_repo\grpc'
$grpcBuild = Join-Path $grpcRoot 'grpc_build'
$grpcInstall = Join-Path $grpcRoot 'installed_packages'
$llamaRoot = Join-Path $env:LOCALAI_ROOT 'backend\cpp\llama-cpp'
$llamaSource = Join-Path $llamaRoot 'llama.cpp'
$serverSource = Join-Path $llamaSource 'tools\grpc-server\grpc-server.cpp'
if (-not (Test-Path -LiteralPath $serverSource)) { throw 'pinned llama gRPC source must be prepared before the MSVC build' }
$source = Get-Content -LiteralPath $serverSource -Raw
$needle = 'reply->set_message(arr);'
if (($source.Split(@($needle), [StringSplitOptions]::None).Length - 1) -ne 1) { throw 'expected one pinned llama gRPC string conversion' }
Set-Content -LiteralPath $serverSource -Value $source.Replace($needle, 'reply->set_message(arr.dump());') -NoNewline

New-Item -ItemType Directory -Path $grpcSource -Force | Out-Null
Invoke-Checked git @('-C', $grpcSource, 'init')
Invoke-Checked git @('-C', $grpcSource, 'remote', 'add', 'origin', 'https://github.com/grpc/grpc.git')
Invoke-Checked git @('-C', $grpcSource, 'fetch', '--depth', '1', 'origin', $env:GRPC_COMMIT)
Invoke-Checked git @('-C', $grpcSource, 'checkout', '--detach', 'FETCH_HEAD')
Invoke-Checked git @('-C', $grpcSource, 'submodule', 'update', '--init', '--recursive', '--depth', '1')
if ((& git -C $grpcSource rev-parse HEAD).Trim() -ne $env:GRPC_COMMIT) { throw 'gRPC checkout does not match the pinned commit' }
$grpcCmake = Get-Content -LiteralPath (Join-Path $grpcSource 'CMakeLists.txt') -Raw
if ($grpcCmake -notmatch "(?m)set\(PACKAGE_VERSION\s+`"$([regex]::Escape($config.toolchain.grpcVersion))`"\)") { throw 'pinned gRPC version does not match the config' }

$grpcArgs = @('-S', $grpcSource, '-B', $grpcBuild, '-G', 'Visual Studio 17 2022', '-A', 'x64',
    "-DCMAKE_INSTALL_PREFIX=$grpcInstall", '-DCMAKE_CXX_STANDARD=17', '-DBUILD_SHARED_LIBS=OFF',
    '-DgRPC_INSTALL=ON', '-DgRPC_BUILD_TESTS=OFF', '-DgRPC_BUILD_GRPC_CPP_PLUGIN=ON',
    '-DgRPC_ZLIB_PROVIDER=module', '-DgRPC_CARES_PROVIDER=module', '-DgRPC_RE2_PROVIDER=module',
    '-DgRPC_SSL_PROVIDER=module', '-DgRPC_PROTOBUF_PROVIDER=module', '-DgRPC_ABSL_PROVIDER=module',
    '-DOPENSSL_NO_ASM=ON')
Invoke-Checked cmake $grpcArgs
Invoke-Checked cmake @('--build', $grpcBuild, '--config', 'Release', '--target', 'install', '--parallel', '4')
$protoc = Join-Path $grpcInstall 'bin\protoc.exe'
$plugin = Join-Path $grpcInstall 'bin\grpc_cpp_plugin.exe'
if (-not (Test-Path -LiteralPath $plugin)) { throw 'pinned gRPC build did not install grpc_cpp_plugin.exe' }
Assert-Tool $protoc @('--version') "libprotoc $($config.toolchain.protobufVersion)"

$llamaBuild = Join-Path $llamaRoot 'llama-cpp-cuda-build'
$cmakeArgs = @('-S', $llamaSource, '-B', $llamaBuild, '-G', 'Visual Studio 17 2022', '-A', 'x64', '-T', "cuda=$cudaRoot",
    '-DCMAKE_CXX_STANDARD=17', '-DBUILD_SHARED_LIBS=ON', '-DLLAMA_CURL=OFF', '-DLLAMA_OPENSSL=OFF',
    '-DGGML_NATIVE=OFF', '-DGGML_BACKEND_DL=ON', '-DGGML_CUDA=ON',
    "-DCMAKE_CUDA_ARCHITECTURES=$($config.hostToolchain.windows.cudaArchitecture)",
    "-DCMAKE_CUDA_COMPILER=$nvcc", "-DCMAKE_PREFIX_PATH=$grpcInstall",
    "-Dabsl_DIR=$(Join-Path $grpcInstall 'lib\cmake\absl')",
    "-DProtobuf_DIR=$(Join-Path $grpcInstall 'lib\cmake\protobuf')",
    "-Dutf8_range_DIR=$(Join-Path $grpcInstall 'lib\cmake\utf8_range')",
    "-DgRPC_DIR=$(Join-Path $grpcInstall 'lib\cmake\grpc')",
    "-DProtobuf_PROTOC_EXECUTABLE=$protoc", "-D_PROTOBUF_PROTOC=$protoc", "-D_GRPC_CPP_PLUGIN_EXECUTABLE=$plugin")
Invoke-Checked cmake $cmakeArgs
Invoke-Checked cmake @('--build', $llamaBuild, '--config', 'Release', '--target', 'grpc-server', 'ggml-cuda', '--parallel', '4')

$packageRoot = Join-Path $llamaRoot 'package'
if (Test-Path -LiteralPath $packageRoot) { Remove-GeneratedDirectory -Path $packageRoot -Parent $llamaRoot -ExpectedName 'package' }
New-Item -ItemType Directory -Path $packageRoot -Force | Out-Null
$server = Get-ChildItem -LiteralPath $llamaBuild -Recurse -File -Filter 'grpc-server.exe' | Where-Object Length -gt 0 | Select-Object -First 1
if (-not $server) { throw 'MSVC CUDA build did not produce grpc-server.exe' }
Copy-Item -LiteralPath $server.FullName -Destination (Join-Path $packageRoot 'llama-cpp-cpu-all.exe')
$builtDlls = Get-ChildItem -LiteralPath $llamaBuild -Recurse -File -Filter '*.dll' | Where-Object Length -gt 0
foreach ($dll in $builtDlls) { Copy-Item -LiteralPath $dll.FullName -Destination $packageRoot -Force }
if (-not (Test-Path -LiteralPath (Join-Path $packageRoot 'ggml-cuda.dll'))) { throw 'MSVC build did not produce ggml-cuda.dll' }

# Stage the recursively imported CUDA and MSVC redistributable DLLs. Windows
# system DLLs remain system dependencies. Refuse unknown non-system imports.
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
        $candidate = Find-CudaRuntimeFile -CudaRoot $cudaRoot -Name $name
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
if (-not (Get-ChildItem -LiteralPath $packageRoot -File | Where-Object Name -Match '^cudart64_.*\.dll$')) { throw 'CUDA runtime DLL was not staged' }
Invoke-Checked go @('run', (Join-Path $repositoryRoot 'scripts\localai-backend-startup-smoke.go'), '--binary', (Join-Path $packageRoot 'llama-cpp-cpu-all.exe'), '--workdir', $packageRoot)
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'verify-payload', '--package-root', $packageRoot, '--binary', 'llama-cpp-cpu-all', '--target', 'windows-amd64-cuda')
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'metadata', '--config', (Join-Path $repositoryRoot '.github\localai-backend-artifacts.json'), '--localai-root', $env:LOCALAI_ROOT, '--backend', $env:BACKEND_ID, '--target', $env:TARGET_ID, '--output', (Join-Path $packageRoot 'build-metadata.json'))
Write-Output "LOCALAI_BACKEND_PACKAGE_OK backend=$env:BACKEND_ID target=$env:TARGET_ID path=$packageRoot"
