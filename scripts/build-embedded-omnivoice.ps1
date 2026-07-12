[CmdletBinding()]
param(
    [ValidateSet("cpu", "cuda")]
    [string]$Backend = "cpu",
    [switch]$Clean
)

$ErrorActionPreference = "Stop"
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$sourceDirectory = Join-Path $repositoryRoot "third_party/omnivoice-cpp"
$buildDirectory = Join-Path $repositoryRoot "native/omnivoice/build"

if (-not (Test-Path (Join-Path $sourceDirectory "CMakeLists.txt"))) {
    throw "OmniVoice source is missing. Run: git submodule update --init --recursive"
}

$cmake = Get-Command cmake -ErrorAction SilentlyContinue
if ($null -eq $cmake) {
    $userCMake = Get-ChildItem (Join-Path $env:APPDATA "Python") -Filter cmake.exe -Recurse -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($null -ne $userCMake) {
        $cmake = @{ Source = $userCMake.FullName }
    } else {
        throw "CMake is required to build embedded OmniVoice. Install CMake and rerun this script."
    }
}

if ($Clean -and (Test-Path $buildDirectory)) {
    Remove-Item -Recurse -Force $buildDirectory
}
New-Item -ItemType Directory -Force -Path $buildDirectory | Out-Null

$configureArgs = @(
    "-S", $sourceDirectory,
    "-B", $buildDirectory,
    "-G", "MinGW Makefiles",
    "-DOMNIVOICE_SHARED=ON",
    "-DGGML_NATIVE=OFF"
)
if ($Backend -eq "cuda") {
    $configureArgs += "-DGGML_CUDA=ON"
} else {
    # The upstream project probes CUDA only to select distributed build
    # architectures. Disable that optional probe for a deterministic CPU build.
    $configureArgs += "-DGGML_CUDA=OFF"
    $configureArgs += "-DCMAKE_DISABLE_FIND_PACKAGE_CUDAToolkit=TRUE"
}

& $cmake.Source @configureArgs
if ($LASTEXITCODE -ne 0) {
    throw "OmniVoice CMake configuration failed for $Backend."
}
& $cmake.Source --build $buildDirectory --config Release --target omnivoice --parallel
if ($LASTEXITCODE -ne 0) {
    throw "OmniVoice $Backend build failed."
}

# OmniVoice discovers GGML backends next to the application. Copy every DLL
# emitted by the build, not only the public ABI library.
Get-ChildItem -Path $buildDirectory -Filter "*.dll" -File | ForEach-Object {
    Copy-Item -Force $_.FullName (Join-Path $repositoryRoot "bin")
}
Write-Host "Built embedded OmniVoice ($Backend): $buildDirectory"
