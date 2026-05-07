#!/usr/bin/env pwsh
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$BinaryName = "infinite-you"
$ReleaseBaseUrl = if ($env:INFINITE_YOU_INSTALL_BASE_URL) { $env:INFINITE_YOU_INSTALL_BASE_URL } else { "https://github.com/portpowered/infinite-you/releases" }
$InstallDir = if ($env:INFINITE_YOU_INSTALL_DIR) { $env:INFINITE_YOU_INSTALL_DIR } else { Join-Path $HOME ".local/bin" }
$VersionOverride = $env:INFINITE_YOU_VERSION
$OsOverride = $env:INFINITE_YOU_INSTALL_OS
$ArchOverride = $env:INFINITE_YOU_INSTALL_ARCH

function Write-InstallMessage {
    param([string]$Message)
    Write-Host $Message
}

function Fail-Install {
    param([string]$Message)
    throw "infinite-you install: $Message"
}

function Normalize-Os {
    param([string]$Value)

    switch ($Value.ToLowerInvariant()) {
        "windows" { return "windows" }
        default { Fail-Install "unsupported operating system '$Value'; supported values are windows" }
    }
}

function Detect-Os {
    if ($OsOverride) {
        return Normalize-Os $OsOverride
    }

    return "windows"
}

function Normalize-Arch {
    param([string]$Value)

    switch ($Value.ToLowerInvariant()) {
        "x86_64" { return "amd64" }
        "x64" { return "amd64" }
        "amd64" { return "amd64" }
        "arm64" { return "arm64" }
        "aarch64" { return "arm64" }
        default { Fail-Install "unsupported architecture '$Value'; supported values are amd64 and arm64" }
    }
}

function Detect-Arch {
    if ($ArchOverride) {
        return Normalize-Arch $ArchOverride
    }

    return Normalize-Arch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString())
}

function Resolve-Tag {
    if ($VersionOverride) {
        if ($VersionOverride.StartsWith("v")) {
            return $VersionOverride
        }
        return "v$VersionOverride"
    }

    $handler = $null
    $client = $null
    try {
        $handler = [System.Net.Http.HttpClientHandler]::new()
        $handler.AllowAutoRedirect = $true
        $client = [System.Net.Http.HttpClient]::new($handler)
        $response = $client.GetAsync("$ReleaseBaseUrl/latest").GetAwaiter().GetResult()
        $response.EnsureSuccessStatusCode() | Out-Null
        $effectiveUrl = $response.RequestMessage.RequestUri.AbsoluteUri
    } catch {
        Fail-Install "failed to resolve the latest infinite-you release from $ReleaseBaseUrl/latest"
    } finally {
        if ($client) {
            $client.Dispose()
        }
        if ($handler) {
            $handler.Dispose()
        }
    }

    $tag = [System.IO.Path]::GetFileName($effectiveUrl.TrimEnd('/'))
    if (-not $tag.StartsWith("v")) {
        Fail-Install "could not determine the latest release tag from $effectiveUrl"
    }

    return $tag
}

function Download-To {
    param(
        [string]$Url,
        [string]$Destination
    )

    try {
        Invoke-WebRequest -Uri $Url -OutFile $Destination
    } catch {
        Fail-Install "failed to download $Url"
    }
}

function Verify-Checksum {
    param(
        [string]$ArchivePath,
        [string]$ChecksumPath,
        [string]$ArchiveName
    )

    $checksumLine = Get-Content -LiteralPath $ChecksumPath |
        Where-Object { $_ -match "\s$([regex]::Escape($ArchiveName))$" } |
        Select-Object -First 1

    if (-not $checksumLine) {
        Fail-Install "checksum entry for $ArchiveName was not found in $([System.IO.Path]::GetFileName($ChecksumPath))"
    }

    $expected = ($checksumLine -split '\s+')[0].Trim()
    $actual = (Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expected.ToLowerInvariant() -ne $actual) {
        Fail-Install "checksum mismatch for $ArchiveName"
    }
}

function Install-Binary {
    param(
        [string]$SourcePath,
        [string]$TargetPath
    )

    $targetDir = Split-Path -Parent $TargetPath
    try {
        New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
    } catch {
        Fail-Install "could not create install directory $targetDir; set INFINITE_YOU_INSTALL_DIR to a writable path"
    }

    try {
        Copy-Item -LiteralPath $SourcePath -Destination $TargetPath -Force
    } catch {
        Fail-Install "could not copy $BinaryName to $TargetPath; set INFINITE_YOU_INSTALL_DIR to a writable path"
    }
}

function Test-PathContainsDir {
    param([string]$TargetDir)

    $pathValue = [System.Environment]::GetEnvironmentVariable("PATH", "Process")
    if (-not $pathValue) {
        return $false
    }

    $comparison = [System.StringComparer]::OrdinalIgnoreCase
    foreach ($pathDir in $pathValue.Split([System.IO.Path]::PathSeparator, [System.StringSplitOptions]::RemoveEmptyEntries)) {
        if ($comparison.Equals($pathDir.TrimEnd('\'), $TargetDir.TrimEnd('\'))) {
            return $true
        }
    }

    return $false
}

function Main {
    $osName = Detect-Os
    $archName = Detect-Arch
    $tag = Resolve-Tag
    $version = $tag.TrimStart('v')
    $archiveName = "${BinaryName}_${version}_${osName}_${archName}.zip"
    $checksumName = "${BinaryName}_${version}_checksums.txt"
    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("infinite-you-install-" + [System.Guid]::NewGuid().ToString("N"))
    $archivePath = Join-Path $tmpDir $archiveName
    $checksumPath = Join-Path $tmpDir $checksumName
    $extractDir = Join-Path $tmpDir "extracted"
    $binaryPath = Join-Path $InstallDir "${BinaryName}.exe"

    try {
        New-Item -ItemType Directory -Path $extractDir -Force | Out-Null

        Write-InstallMessage "Downloading $archiveName from $ReleaseBaseUrl/download/$tag/."
        Download-To "$ReleaseBaseUrl/download/$tag/$archiveName" $archivePath
        Download-To "$ReleaseBaseUrl/download/$tag/$checksumName" $checksumPath
        Verify-Checksum $archivePath $checksumPath $archiveName

        try {
            Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDir -Force
        } catch {
            Fail-Install "failed to extract $archiveName"
        }

        $sourceBinaryPath = Join-Path $extractDir "${BinaryName}.exe"
        if (-not (Test-Path -LiteralPath $sourceBinaryPath -PathType Leaf)) {
            Fail-Install "archive $archiveName did not contain ${BinaryName}.exe"
        }

        Install-Binary $sourceBinaryPath $binaryPath
        Write-InstallMessage "Installed $BinaryName $tag to $binaryPath"

        if (Test-PathContainsDir $InstallDir) {
            Write-InstallMessage "Run '$BinaryName --help' to get started."
            return
        }

        Write-InstallMessage "Add it to your PATH with:"
        Write-InstallMessage "  `$env:PATH = `"$InstallDir;`$env:PATH`""
    } finally {
        if (Test-Path -LiteralPath $tmpDir) {
            Remove-Item -LiteralPath $tmpDir -Recurse -Force
        }
    }
}

Main
