param()

$ErrorActionPreference = "Stop"

$commandName = if ($env:OMNIVOICE_COMMAND_NAME) { $env:OMNIVOICE_COMMAND_NAME } else { "omnivoice-llamacpp.exe" }
$commandUrl = $env:OMNIVOICE_COMMAND_URL
$installDir = if ($env:OMNIVOICE_COMMAND_INSTALL_DIR) { $env:OMNIVOICE_COMMAND_INSTALL_DIR } else { Join-Path (Get-Location) ".cache/omnivoice-command/bin" }
$extractDir = if ($env:OMNIVOICE_COMMAND_EXTRACT_DIR) { $env:OMNIVOICE_COMMAND_EXTRACT_DIR } else { Join-Path (Get-Location) ".cache/omnivoice-command/extract" }

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

New-Item -ItemType Directory -Path $installDir -Force | Out-Null
New-Item -ItemType Directory -Path $extractDir -Force | Out-Null

$targetPath = Join-Path $installDir $commandName
if (-not (Test-Path $targetPath)) {
    if ([string]::IsNullOrWhiteSpace($commandUrl)) {
        $existingCommand = Get-Command $commandName -ErrorAction SilentlyContinue
        if ($existingCommand) {
            Write-Host "Using $commandName already available on PATH at $($existingCommand.Source)"
            Write-InstallerOutputs -CommandPath $existingCommand.Source -Skipped "false" -SkipReason ""
            exit 0
        }

        Write-Host "OMNIVOICE_COMMAND_URL is not configured for $env:RUNNER_OS/$env:PROCESSOR_ARCHITECTURE; building repo-owned $commandName companion."
        & go build "-o" $targetPath "./cmd/omnivoice-llamacpp"
        Write-InstallerOutputs -CommandPath $targetPath -Skipped "false" -SkipReason ""
        $installDir | Out-File -FilePath $env:GITHUB_PATH -Encoding utf8 -Append
        exit 0
    }

    $archivePath = Join-Path $extractDir ([System.IO.Path]::GetFileName($commandUrl))
    $payloadDir = Join-Path $extractDir "payload"
    if (Test-Path $payloadDir) {
        Remove-Item -Recurse -Force $payloadDir
    }
    New-Item -ItemType Directory -Path $payloadDir -Force | Out-Null

    Write-Host "Downloading $commandName from $commandUrl"
    Invoke-WebRequest -Uri $commandUrl -OutFile $archivePath

    if ($archivePath.EndsWith(".zip")) {
        Expand-Archive -LiteralPath $archivePath -DestinationPath $payloadDir -Force
    } else {
        Copy-Item -LiteralPath $archivePath -Destination $targetPath -Force
    }

    if (-not (Test-Path $targetPath)) {
        $candidate = Get-ChildItem -Path $payloadDir -Recurse -File | Where-Object { $_.Name -eq $commandName } | Select-Object -First 1
        if (-not $candidate) {
            Get-ChildItem -Path $payloadDir -Recurse -File | Select-Object -ExpandProperty FullName | Write-Host
            throw "Downloaded payload from $commandUrl did not contain $commandName"
        }
        Copy-Item -LiteralPath $candidate.FullName -Destination $targetPath -Force
    }
}

Write-InstallerOutputs -CommandPath $targetPath -Skipped "false" -SkipReason ""
$installDir | Out-File -FilePath $env:GITHUB_PATH -Encoding utf8 -Append
