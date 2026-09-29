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
$patchedNeedle = 'reply->set_message(arr.dump());'
$needleCount = ($source.Split(@($needle), [StringSplitOptions]::None).Length - 1)
$patchedCount = ($source.Split(@($patchedNeedle), [StringSplitOptions]::None).Length - 1)
if ($needleCount -eq 1 -and $patchedCount -eq 0) {
    Set-Content -LiteralPath $serverSource -Value $source.Replace($needle, $patchedNeedle) -NoNewline
} elseif ($needleCount -eq 0 -and $patchedCount -eq 1) {
    Write-Output 'llama gRPC string conversion is already patched for this retry'
} else {
    throw 'expected one pinned llama gRPC string conversion in the original or already-patched shape'
}

# The pinned llama grpc-server.cpp includes POSIX <getopt.h> and parses
# --addr/-a with getopt_long, which MSVC does not provide
# (grpc-server.cpp(56): fatal error C1083: 'getopt.h'). Patch the pinned
# working tree idempotently instead of changing the pinned source commit
# (verify-source above pins LOCALAI_ROOT/BACKEND_SOURCE_COMMIT): guard the
# include to POSIX and keep the getopt_long path on Unix while using a small
# argv loop on Windows with identical CLI semantics (--addr=<address>,
# --addr value, -a value; anything else prints usage and returns 1).
# Keyed by the marker LOCALAI_WINDOWS_CUDA_MSVC_GETOPT.
$getoptMarker = 'LOCALAI_WINDOWS_CUDA_MSVC_GETOPT'
$serverSourceText = Get-Content -LiteralPath $serverSource -Raw
if ($serverSourceText.Contains($getoptMarker)) {
    Write-Output 'llama gRPC getopt parse is already patched for this retry'
} else {
    $getoptInclude = '#include <getopt.h>'
    if (-not $serverSourceText.Contains($getoptInclude)) { throw 'expected pinned llama gRPC getopt include in the original or already-patched shape' }
    $getoptParse = @'
  // Define long and short options
  struct option long_options[] = {
      {"addr", required_argument, nullptr, 'a'},
      {nullptr, 0, nullptr, 0}
  };

  // Parse command-line arguments
  int option;
  int option_index = 0;
  while ((option = getopt_long(argc, argv, "a:", long_options, &option_index)) != -1) {
    switch (option) {
      case 'a':
        server_address = optarg;
        break;
      default:
        std::cerr << "Usage: " << argv[0] << " [--addr=<address>] or [-a <address>]" << std::endl;
        return 1;
    }
  }
'@
    if (-not $serverSourceText.Contains($getoptParse)) { throw 'expected pinned llama gRPC addr parse in the original or already-patched shape' }
    $getoptIncludeReplacement = @'
#if !defined(_WIN32)
#include <getopt.h>
#endif
'@
    $getoptParseReplacement = @'
  // LOCALAI_WINDOWS_CUDA_MSVC_GETOPT: working-tree retry patch applied by
  // scripts/build-localai-backend-cuda.ps1; the pinned llama source commit is unchanged
  // (see BACKEND_SOURCE_COMMIT / verify-source). MSVC has no POSIX <getopt.h>, so the
  // --addr/-a parse keeps the getopt_long path on POSIX and uses a small argv loop on
  // Windows with identical CLI semantics (--addr=<address>, --addr value, -a value).
#if defined(_WIN32)
  for (int i = 1; i < argc; ++i) {
    std::string arg = argv[i];
    if (arg.rfind("--addr=", 0) == 0) {
      server_address = arg.substr(sizeof("--addr=") - 1);
    } else if (arg == "--addr" || arg == "-a") {
      if (i + 1 >= argc) {
        std::cerr << "Usage: " << argv[0] << " [--addr=<address>] or [-a <address>]" << std::endl;
        return 1;
      }
      server_address = argv[++i];
    } else {
      std::cerr << "Usage: " << argv[0] << " [--addr=<address>] or [-a <address>]" << std::endl;
      return 1;
    }
  }
#else
  // Define long and short options
  struct option long_options[] = {
      {"addr", required_argument, nullptr, 'a'},
      {nullptr, 0, nullptr, 0}
  };

  // Parse command-line arguments
  int option;
  int option_index = 0;
  while ((option = getopt_long(argc, argv, "a:", long_options, &option_index)) != -1) {
    switch (option) {
      case 'a':
        server_address = optarg;
        break;
      default:
        std::cerr << "Usage: " << argv[0] << " [--addr=<address>] or [-a <address>]" << std::endl;
        return 1;
    }
  }
#endif
'@
    $serverSourceText = $serverSourceText.Replace($getoptInclude, $getoptIncludeReplacement.TrimEnd() + [Environment]::NewLine)
    $serverSourceText = $serverSourceText.Replace($getoptParse, $getoptParseReplacement.TrimEnd() + [Environment]::NewLine)
    Set-Content -LiteralPath $serverSource -Value $serverSourceText -NoNewline
}

New-Item -ItemType Directory -Path $grpcSource -Force | Out-Null
Invoke-Checked git @('-C', $grpcSource, 'init')
$grpcOriginUrl = 'https://github.com/grpc/grpc.git'
$existingOriginUrl = (& git -C $grpcSource remote get-url origin 2>$null | Out-String)
if ($LASTEXITCODE -ne 0) {
    Invoke-Checked git @('-C', $grpcSource, 'remote', 'add', 'origin', $grpcOriginUrl)
} elseif ($existingOriginUrl.Trim() -cne $grpcOriginUrl) {
    throw "gRPC origin remote must be $grpcOriginUrl; refusing to reset a conflicting checkout"
}
Invoke-Checked git @('-C', $grpcSource, 'fetch', '--depth', '1', 'origin', $env:GRPC_COMMIT)
Invoke-Checked git @('-C', $grpcSource, 'checkout', '--detach', 'FETCH_HEAD')
# gRPC builds every dependency from its pinned third-party submodules
# (all _PROVIDER=module below), so only top-level submodules are required.
# Do not re-add --recursive: it enters third_party/bloaty/third_party/abseil-cpp
# and fails on Windows with `fatal: '$GIT_DIR' too big`.
Invoke-Checked git @('-C', $grpcSource, 'submodule', 'update', '--init', '--depth', '1')
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

# The pinned llama grpc-server CMakeLists resolves Protobuf in CONFIG mode but
# propagates headers via the module-mode ${Protobuf_INCLUDE_DIRS} variable,
# which is empty there (retry log: "Using protobuf version 24.3.0 |
# Protobuf_INCLUDE_DIRS:  | ..."), and hw_grpc_proto itself never links
# protobuf::libprotobuf (only the final exe does), so its backend.pb.cc
# compile has no protobuf include path even though port_def.inc exists under
# the pinned install. Exporting $env:INCLUDE does NOT reach the
# CMake-generated MSBuild hw_grpc_proto target (observed C1083 for
# google/protobuf/port_def.inc on hw_grpc_proto.vcxproj despite the export),
# so patch the pinned grpc-server CMakeLists idempotently instead: link the
# already-found imported targets into hw_grpc_proto so their
# INTERFACE_INCLUDE_DIRECTORIES (<install>/include) land in the MSBuild
# AdditionalIncludeDirectories. The pinned source commits are unchanged
# (verify-source above pins LOCALAI_ROOT/BACKEND_SOURCE_COMMIT); this is a
# working-tree retry patch like the grpc-server.cpp string fix below, keyed
# by the marker LOCALAI_WINDOWS_CUDA_HW_GRPC_PROTO_INCLUDES.
$grpcInclude = Join-Path $grpcInstall 'include'
if (-not (Test-Path -LiteralPath (Join-Path $grpcInclude 'google\protobuf\port_def.inc'))) { throw 'pinned gRPC install is missing google/protobuf/port_def.inc' }
if ($env:INCLUDE) { $env:INCLUDE = "$grpcInclude;$env:INCLUDE" } else { $env:INCLUDE = $grpcInclude }
$grpcServerCmakeLists = Join-Path $llamaSource 'tools\grpc-server\CMakeLists.txt'
$protoIncludeMarker = 'LOCALAI_WINDOWS_CUDA_HW_GRPC_PROTO_INCLUDES'
$grpcServerCmake = Get-Content -LiteralPath $grpcServerCmakeLists -Raw
if ($grpcServerCmake.Contains($protoIncludeMarker)) {
    Write-Output 'llama gRPC CMake hw_grpc_proto includes are already patched for this retry'
} else {
    if (-not $grpcServerCmake.Contains('add_library(hw_grpc_proto STATIC')) { throw 'expected pinned llama gRPC CMake hw_grpc_proto library in the original or already-patched shape' }
    $protoIncludeClose = '${hw_proto_hdrs} )'
    if (-not $grpcServerCmake.Contains($protoIncludeClose)) { throw 'expected pinned llama gRPC CMake hw_grpc_proto sources in the original or already-patched shape' }
    $protoIncludePatch = @'

# LOCALAI_WINDOWS_CUDA_HW_GRPC_PROTO_INCLUDES: working-tree retry patch applied by
# scripts/build-localai-backend-cuda.ps1; the pinned llama source commit is unchanged
# (see BACKEND_SOURCE_COMMIT / verify-source). Links the already-found imported targets
# into hw_grpc_proto so the pinned <install>/include (google/protobuf/port_def.inc)
# reaches the MSVC compile via INTERFACE_INCLUDE_DIRECTORIES. Kept PUBLIC so the
# usage requirements also propagate to the grpc-server executable link.
if(TARGET protobuf::libprotobuf)
  target_link_libraries(hw_grpc_proto PUBLIC protobuf::libprotobuf)
endif()
if(TARGET gRPC::grpc++)
  target_link_libraries(hw_grpc_proto PUBLIC gRPC::grpc++)
endif()
'@
    $grpcServerCmake = $grpcServerCmake.Replace($protoIncludeClose, $protoIncludeClose + [Environment]::NewLine + $protoIncludePatch)
    Set-Content -LiteralPath $grpcServerCmakeLists -Value $grpcServerCmake -NoNewline
}
$llamaBuild = Join-Path $llamaRoot 'llama-cpp-cuda-build'
$cmakeArgs = @('-S', $llamaSource, '-B', $llamaBuild, '-G', 'Visual Studio 17 2022', '-A', 'x64', '-T', "cuda=$cudaRoot",
    '-DCMAKE_CXX_STANDARD=17', '-DBUILD_SHARED_LIBS=ON', '-DLLAMA_CURL=OFF', '-DLLAMA_OPENSSL=OFF',
    '-DGGML_NATIVE=OFF', '-DGGML_BACKEND_DL=ON', '-DGGML_CUDA=ON',
    "-DCMAKE_CUDA_ARCHITECTURES=$($config.hostToolchain.windows.cudaArchitecture)",
    "-DCMAKE_CUDA_COMPILER=$nvcc", "-DCMAKE_PREFIX_PATH=$grpcInstall",
    "-Dabsl_DIR=$(Join-Path $grpcInstall 'lib\cmake\absl')",
    "-DProtobuf_DIR=$(Join-Path $grpcInstall 'cmake')",
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
Invoke-Checked go @('run', (Join-Path $repositoryRoot 'scripts\localai-backend-startup-smoke.go'), '--binary', (Join-Path $packageRoot 'llama-cpp-cpu-all.exe'), '--workdir', $packageRoot)
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'verify-payload', '--package-root', $packageRoot, '--binary', 'llama-cpp-cpu-all', '--target', 'windows-amd64-cuda')
Invoke-Checked node @((Join-Path $repositoryRoot 'scripts\localai-backend-artifact-workflow.mjs'), 'metadata', '--config', (Join-Path $repositoryRoot '.github\localai-backend-artifacts.json'), '--localai-root', $env:LOCALAI_ROOT, '--backend', $env:BACKEND_ID, '--target', $env:TARGET_ID, '--output', (Join-Path $packageRoot 'build-metadata.json'))
Write-Output "LOCALAI_BACKEND_PACKAGE_OK backend=$env:BACKEND_ID target=$env:TARGET_ID path=$packageRoot"
