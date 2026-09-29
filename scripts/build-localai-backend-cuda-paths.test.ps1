$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'build-localai-backend-cuda-paths.ps1')

$tempParent = [IO.Path]::GetTempPath()
$root = Join-Path $tempParent ("localai-cuda-probe-" + [Guid]::NewGuid().ToString('N'))
$package = Join-Path $root 'package'
New-Item -ItemType Directory -Path $package | Out-Null
try {
    Set-Content -LiteralPath (Join-Path $package 'payload.txt') -Value 'test'
    try {
        Remove-GeneratedDirectory -Path $root -Parent $root -ExpectedName 'package'
        throw 'parent directory was accepted as a package'
    } catch {
        if ($_.Exception.Message -eq 'parent directory was accepted as a package') { throw }
    }
    if (-not (Test-Path -LiteralPath $package)) { throw 'rejected removal changed the package' }
    Remove-GeneratedDirectory -Path $package -Parent $root -ExpectedName 'package'
    if (Test-Path -LiteralPath $package) { throw 'package was not removed' }

    $outside = Join-Path $root 'outside'
    New-Item -ItemType Directory -Path $outside | Out-Null
    Set-Content -LiteralPath (Join-Path $outside 'keep.txt') -Value 'keep'
    New-Item -ItemType Junction -Path $package -Target $outside | Out-Null
    try {
        Remove-GeneratedDirectory -Path $package -Parent $root -ExpectedName 'package'
        throw 'junction was accepted as a package'
    } catch {
        if ($_.Exception.Message -eq 'junction was accepted as a package') { throw }
    }
    if (-not (Test-Path -LiteralPath (Join-Path $outside 'keep.txt'))) { throw 'rejected junction removal changed its target' }
    Remove-Item -LiteralPath $package -Force

    New-Item -ItemType Directory -Path $package | Out-Null
    $nestedLink = Join-Path $package 'linked'
    New-Item -ItemType Junction -Path $nestedLink -Target $outside | Out-Null
    try {
        Remove-GeneratedDirectory -Path $package -Parent $root -ExpectedName 'package'
        throw 'nested junction was accepted in a package'
    } catch {
        if ($_.Exception.Message -eq 'nested junction was accepted in a package') { throw }
    }
    if (-not (Test-Path -LiteralPath (Join-Path $outside 'keep.txt'))) { throw 'nested junction changed its target' }
    Remove-Item -LiteralPath $nestedLink -Force
    Remove-GeneratedDirectory -Path $package -Parent $root -ExpectedName 'package'
} finally {
    if (Test-Path -LiteralPath $root) {
        Remove-GeneratedDirectory -Path $root -Parent $tempParent -ExpectedName ([IO.Path]::GetFileName($root))
    }
}
Write-Output 'LOCALAI_WINDOWS_CUDA_PATH_GUARDS_OK'

$lookupParent = [IO.Path]::GetTempPath()
$lookupRoot = Join-Path $lookupParent ("localai-cuda-paths-" + [Guid]::NewGuid().ToString('N'))
$bin = Join-Path $lookupRoot 'bin'
$binX64 = Join-Path $bin 'x64'
New-Item -ItemType Directory -Path $lookupRoot | Out-Null
try {
    New-Item -ItemType Directory -Path $binX64 -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $binX64 'cudart64_13.dll') -Value 'x64-fallback'

    $found = Find-CudaRuntimeFile -CudaRoot $lookupRoot -Name 'cudart64_13.dll'
    if ($found -cne (Join-Path $binX64 'cudart64_13.dll')) { throw "bin\\x64 fallback was not used: $found" }

    Set-Content -LiteralPath (Join-Path $bin 'cudart64_13.dll') -Value 'bin-direct'
    $preferred = Find-CudaRuntimeFile -CudaRoot $lookupRoot -Name 'cudart64_13.dll'
    if ($preferred -cne (Join-Path $bin 'cudart64_13.dll')) { throw "bin was not preferred: $preferred" }

    $missing = Find-CudaRuntimeFile -CudaRoot $lookupRoot -Name 'missing64_13.dll'
    if ($null -ne $missing) { throw "missing DLL did not return null: $missing" }
} finally {
    foreach ($file in @((Join-Path $binX64 'cudart64_13.dll'), (Join-Path $bin 'cudart64_13.dll'))) {
        if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file -Force }
    }
    foreach ($directory in @($binX64, $bin, $lookupRoot)) {
        if (Test-Path -LiteralPath $directory) { Remove-Item -LiteralPath $directory }
    }
}
Write-Output 'LOCALAI_WINDOWS_CUDA_PATH_LOOKUP_OK'
