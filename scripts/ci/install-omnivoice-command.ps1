param()

$ErrorActionPreference = "Stop"

$commandName = if ($env:OMNIVOICE_COMMAND_NAME) { $env:OMNIVOICE_COMMAND_NAME } else { "omnivoice-llamacpp.exe" }
$commandUrl = $env:OMNIVOICE_COMMAND_URL
$installDir = if ($env:OMNIVOICE_COMMAND_INSTALL_DIR) { $env:OMNIVOICE_COMMAND_INSTALL_DIR } else { Join-Path (Get-Location) ".cache/omnivoice-command/bin" }
$extractDir = if ($env:OMNIVOICE_COMMAND_EXTRACT_DIR) { $env:OMNIVOICE_COMMAND_EXTRACT_DIR } else { Join-Path (Get-Location) ".cache/omnivoice-command/extract" }
$sourceDir = if ($env:OMNIVOICE_COMMAND_SOURCE_DIR) { $env:OMNIVOICE_COMMAND_SOURCE_DIR } else { Join-Path (Get-Location) ".cache/omnivoice-command/src/omnivoice.cpp" }
$sourceRepo = if ($env:OMNIVOICE_CPP_SOURCE_REPO) { $env:OMNIVOICE_CPP_SOURCE_REPO } else { "https://github.com/ServeurpersoCom/omnivoice.cpp.git" }
$sourceRef = if ($env:OMNIVOICE_CPP_SOURCE_REF) { $env:OMNIVOICE_CPP_SOURCE_REF } else { "5dff3f17a3e0a73353d8bea35e0fa322fc6dcfdf" }
$backendName = if ($env:OMNIVOICE_TTS_COMMAND_NAME) { $env:OMNIVOICE_TTS_COMMAND_NAME } else { "omnivoice-tts.exe" }

function Write-InstallerOutputs {
    param(
        [string]$CommandPath,
        [string]$Skipped,
        [string]$SkipReason
    )

    "command_path=$CommandPath" | Out-File -FilePath $env:GITHUB_OUTPUT -Encoding utf8 -Append
    "skipped=$Skipped" | Out-File -FilePath $env:GITHUB_OUTPUT -Encoding utf8 -Append
    "skip_reason=$SkipReason" | Out-File -FilePath $env:GITHUB_OUTPUT -Encoding utf8 -Append
}

function Invoke-NativeCommand {
    param(
        [string]$Description,
        [scriptblock]$Command
    )

    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$Description failed with exit code $LASTEXITCODE"
    }
}

New-Item -ItemType Directory -Path $installDir -Force | Out-Null
New-Item -ItemType Directory -Path $extractDir -Force | Out-Null

$targetPath = Join-Path $installDir $commandName
$backendTargetPath = Join-Path $installDir $backendName

function Build-Adapter {
    Write-Host "Building $commandName adapter"
    Invoke-NativeCommand "go build adapter" { go build "-o" $targetPath "./cmd/omnivoice-llamacpp" }
}

function Get-ExistingBackendPath {
    if (Test-Path $backendTargetPath) {
        return $backendTargetPath
    }

    $existing = Get-Command $backendName -ErrorAction SilentlyContinue
    if ($existing) {
        return $existing.Source
    }

    return $null
}

function Copy-BackendCandidate {
    param([string]$CandidatePath)
    Copy-Item -LiteralPath $CandidatePath -Destination $backendTargetPath -Force
}

function Download-BackendFromUrl {
    $archivePath = Join-Path $extractDir ([System.IO.Path]::GetFileName($commandUrl))
    $payloadDir = Join-Path $extractDir "payload"
    if (Test-Path $payloadDir) {
        Remove-Item -Recurse -Force $payloadDir
    }
    New-Item -ItemType Directory -Path $payloadDir -Force | Out-Null

    Write-Host "Downloading $backendName payload from $commandUrl"
    Invoke-WebRequest -Uri $commandUrl -OutFile $archivePath

    if ($archivePath.EndsWith(".zip")) {
        Expand-Archive -LiteralPath $archivePath -DestinationPath $payloadDir -Force
    } else {
        Copy-Item -LiteralPath $archivePath -Destination $backendTargetPath -Force
        return
    }

    $candidate = Get-ChildItem -Path $payloadDir -Recurse -File | Where-Object { $_.Name -eq $backendName } | Select-Object -First 1
    if (-not $candidate) {
        Get-ChildItem -Path $payloadDir -Recurse -File | Select-Object -ExpandProperty FullName | Write-Host
        throw "Downloaded payload from $commandUrl did not contain $backendName"
    }
    Copy-BackendCandidate -CandidatePath $candidate.FullName
}

function Build-BackendFromSource {
    if (-not (Test-Path (Join-Path $sourceDir ".git"))) {
        if (Test-Path $sourceDir) {
            Remove-Item -Recurse -Force $sourceDir
        }
        New-Item -ItemType Directory -Path ([System.IO.Path]::GetDirectoryName($sourceDir)) -Force | Out-Null
        Invoke-NativeCommand "git clone omnivoice.cpp" {
            git clone "--branch" "master" "--depth" "1" "--recurse-submodules" "--shallow-submodules" $sourceRepo $sourceDir
        }
    }

    Write-Host "Building real $backendName from pinned $sourceRepo@$sourceRef"
    Invoke-NativeCommand "git fetch omnivoice.cpp ref" { git -C $sourceDir fetch "--depth" "1" "origin" $sourceRef }
    Invoke-NativeCommand "git checkout omnivoice.cpp ref" { git -C $sourceDir checkout "--force" $sourceRef }
    Invoke-NativeCommand "git submodule sync omnivoice.cpp" { git -C $sourceDir submodule sync "--recursive" }
    Invoke-NativeCommand "git submodule update omnivoice.cpp" {
        git -C $sourceDir submodule update "--init" "--recursive" "--depth" "1"
    }

    $buildDir = Join-Path $sourceDir "build"
    if (Test-Path $buildDir) {
        Remove-Item -Recurse -Force $buildDir
    }
    New-Item -ItemType Directory -Path $buildDir -Force | Out-Null

    Invoke-NativeCommand "cmake configure omnivoice.cpp" {
        cmake "-S" $sourceDir "-B" $buildDir "-G" "Visual Studio 17 2022" "-A" "x64"
    }
    Invoke-NativeCommand "cmake build omnivoice.cpp" {
        cmake "--build" $buildDir "--config" "Release" "--parallel" $env:NUMBER_OF_PROCESSORS
    }

    $candidate = Get-ChildItem -Path $buildDir -Recurse -File | Where-Object { $_.Name -eq $backendName } | Select-Object -First 1
    if (-not $candidate) {
        Get-ChildItem -Path $buildDir -Recurse -File | Select-Object -ExpandProperty FullName | Write-Host
        throw "Built source tree did not produce $backendName"
    }
    Copy-BackendCandidate -CandidatePath $candidate.FullName
}

if (-not (Test-Path $targetPath) -or -not (Test-Path $backendTargetPath)) {
    if (-not (Get-ExistingBackendPath)) {
        if ([string]::IsNullOrWhiteSpace($commandUrl)) {
            Build-BackendFromSource
        } else {
            Download-BackendFromUrl
        }
    } elseif (-not (Test-Path $backendTargetPath)) {
        Copy-BackendCandidate -CandidatePath (Get-ExistingBackendPath)
    }

    if (-not (Test-Path $targetPath)) {
        Build-Adapter
    }
}

Write-InstallerOutputs -CommandPath $targetPath -Skipped "false" -SkipReason ""
$installDir | Out-File -FilePath $env:GITHUB_PATH -Encoding utf8 -Append
