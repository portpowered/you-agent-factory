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
