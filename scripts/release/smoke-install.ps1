param(
    [string]$InstallScriptUrl,
    [string]$InstallVersion,
    [Parameter(Mandatory = $true)]
    [string]$InstallDir,
    [string]$BinaryName = "you.exe",
    [string]$CandidateManifestPath,
    [string]$ReportPath,
    [switch]$ObserverFixture,
    [string]$ObserverReportPath
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Fail-Smoke {
    param([string]$Message)
    throw "install smoke: $Message"
}

function Ensure-SmokeFileHashCommand {
    if ($null -ne (Get-Command Get-FileHash -ErrorAction SilentlyContinue)) {
        return
    }

    function global:Get-FileHash {
        param(
            [string[]]$Path,
            [string[]]$LiteralPath,
            [string]$Algorithm = "SHA256"
        )

        $paths = if ($null -ne $LiteralPath -and $LiteralPath.Count -ne 0) {
            $LiteralPath
        } else {
            @(Resolve-Path -Path $Path | ForEach-Object { $_.ProviderPath })
        }
        foreach ($filePath in $paths) {
            $hasher = [System.Security.Cryptography.HashAlgorithm]::Create($Algorithm)
            $stream = [System.IO.File]::OpenRead($filePath)
            try {
                $digest = $hasher.ComputeHash($stream)
            } finally {
                $stream.Dispose()
                $hasher.Dispose()
            }
            [pscustomobject]@{
                Algorithm = $Algorithm.ToUpperInvariant()
                Hash = [System.BitConverter]::ToString($digest).Replace("-", "")
                Path = $filePath
            }
        }
    }

    if ($null -eq (Get-Command Get-FileHash -ErrorAction SilentlyContinue)) {
        Fail-Smoke "candidate mode could not establish the SHA-256 file-hash observer"
    }
}

function Resolve-SmokeAbsolutePath {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value)) {
        Fail-Smoke "path is required"
    }
    if ([System.IO.Path]::IsPathRooted($Value)) {
        return [System.IO.Path]::GetFullPath($Value)
    }
    return [System.IO.Path]::GetFullPath((Join-Path (Get-Location).Path $Value))
}

function Assert-SmokeDisposableRoot {
    param(
        [string]$Name,
        [string]$Path
    )

    if (-not [System.IO.Path]::IsPathRooted($Path)) {
        Fail-Smoke "declared root '$Name' is not absolute: $Path"
    }

    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -eq $item) {
        return
    }
    if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        Fail-Smoke "declared root '$Name' is a reparse point; it must be absent or empty"
    }
    if (-not $item.PSIsContainer) {
        Fail-Smoke "declared root '$Name' is a file; it must be absent or an empty directory"
    }

    $entries = @(Get-ChildItem -LiteralPath $Path -Force -ErrorAction Stop)
    if ($entries.Count -ne 0) {
        Fail-Smoke "declared root '$Name' is not empty: $((($entries | ForEach-Object Name) -join ', '))"
    }
}

function Test-SmokePathResolvesYou {
    param([string]$PathValue)

    if ([string]::IsNullOrWhiteSpace($PathValue)) {
        return $false
    }
    foreach ($directory in $PathValue.Split([System.IO.Path]::PathSeparator, [System.StringSplitOptions]::RemoveEmptyEntries)) {
        $trimmedDirectory = $directory.Trim()
        if ([string]::IsNullOrWhiteSpace($trimmedDirectory)) {
            continue
        }
        foreach ($executable in @("you.exe", "you.cmd", "you.bat", "you.com", "you")) {
            $candidate = Join-Path $trimmedDirectory $executable
            $item = Get-Item -LiteralPath $candidate -Force -ErrorAction SilentlyContinue
            if ($null -ne $item -and -not $item.PSIsContainer -and (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -eq 0)) {
                return $true
            }
        }
    }
    return $false
}

function Get-SmokeForbiddenProcesses {
    $patterns = @("localai", "local-ai", "llama-server", "llama-cli", "vibevoice", "whisper", "piper")
    $matches = @()
    foreach ($process in @(Get-Process -ErrorAction SilentlyContinue)) {
        $name = $process.ProcessName.ToLowerInvariant()
        if ($patterns | Where-Object { $name -like "*$_*" }) {
            $matches += "{0}:{1}" -f $process.Id, $process.ProcessName
        }
    }
    return $matches
}

function Get-SmokeNetworkConnections {
    param([int]$ProcessId)

    if ($null -eq (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue)) {
        Fail-Smoke "candidate mode requires the Get-NetTCPConnection network observer"
    }
    try {
        $connections = @(Get-NetTCPConnection -OwningProcess $ProcessId -ErrorAction Stop)
    } catch {
        if ($_.Exception.Message -like "*No MSFT_NetTCPConnection objects found*" -or $_.Exception.Message -like "*No matching MSFT_NetTCPConnection*") {
            return @()
        }
        throw
    }
    return @($connections | ForEach-Object {
        [ordered]@{
            processId = $ProcessId
            localAddress = [string]$_.LocalAddress
            localPort = [int]$_.LocalPort
            remoteAddress = [string]$_.RemoteAddress
            remotePort = [int]$_.RemotePort
            state = [string]$_.State
        }
    })
}

function Get-SmokeDirectoryBytes {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Container)) {
        return [int64]0
    }
    $total = [int64]0
    foreach ($item in @(Get-ChildItem -LiteralPath $Path -File -Force -Recurse -ErrorAction Stop)) {
        if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            Fail-Smoke "activity root contains a reparse point: $($item.FullName)"
        }
        $total += [int64]$item.Length
    }
    return $total
}

function Test-SmokeLoopbackAddress {
    param([string]$Address)

    return $Address -eq "127.0.0.1" -or $Address -eq "::1" -or $Address -eq "0:0:0:0:0:0:0:1"
}

function Get-SmokeUnexpectedConnections {
    param(
        [object[]]$Connections,
        [int]$AllowedRemotePort = 0
    )

    return @($Connections | Where-Object {
        $loopback = Test-SmokeLoopbackAddress ([string]$_.remoteAddress)
        $allowed = $loopback -and $AllowedRemotePort -gt 0 -and [int]$_.remotePort -eq $AllowedRemotePort
        -not $allowed
    })
}

function Assert-SmokeCommandNetwork {
    param(
        [object]$Result,
        [string]$CommandName
    )

    if ($null -eq $Result.network -or $Result.network.observerStatus -ne "PASS" -or [int]$Result.network.samples -le 0) {
        Fail-Smoke "network observer did not produce a live sample for $CommandName"
    }
    if (@($Result.network.unexpectedConnections).Count -ne 0) {
        Fail-Smoke "unexpected network connection observed for $($CommandName): $($Result.network.unexpectedConnections | ConvertTo-Json -Compress)"
    }
}

function Record-SmokeCommandObservation {
    param(
        [object]$Result,
        [System.Collections.Generic.List[object]]$NetworkObservations,
        [System.Collections.Generic.List[string]]$ForbiddenProcessObservations
    )

    [void]$NetworkObservations.Add($Result.network)
    foreach ($processName in @($Result.forbiddenProcesses)) {
        [void]$ForbiddenProcessObservations.Add([string]$processName)
    }
}

function Get-SmokeArtifactEvidence {
    param(
        [string]$Role,
        [string]$Path
    )

    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if ($item.PSIsContainer -or (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "$Role is not a regular file: $Path"
    }
    $hasher = [System.Security.Cryptography.SHA256]::Create()
    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $digest = $hasher.ComputeHash($stream)
    } finally {
        $stream.Dispose()
        $hasher.Dispose()
    }
    $hash = [System.BitConverter]::ToString($digest).Replace("-", "").ToLowerInvariant()
    return [ordered]@{
        role = $Role
        file = $item.Name
        bytes = [int64]$item.Length
        sha256 = $hash
    }
}

function Get-SmokeZipEntryEvidence {
    param([string]$ArchivePath)

    try {
        Add-Type -AssemblyName System.IO.Compression.FileSystem -ErrorAction SilentlyContinue
    } catch {
    }

    $archive = [System.IO.Compression.ZipFile]::OpenRead($ArchivePath)
    try {
        $entries = @($archive.Entries | Where-Object { $_.FullName -eq "you.exe" })
        if ($entries.Count -ne 1 -or $entries[0].Name -ne "you.exe") {
            Fail-Smoke "candidate archive must contain exactly one regular you.exe entry"
        }

        $entry = $entries[0]
        $hasher = [System.Security.Cryptography.SHA256]::Create()
        $stream = $entry.Open()
        try {
            $digest = $hasher.ComputeHash($stream)
        } finally {
            $stream.Dispose()
            $hasher.Dispose()
        }
        return [ordered]@{
            file = $entry.FullName
            bytes = [int64]$entry.Length
            sha256 = ([System.BitConverter]::ToString($digest).Replace("-", "").ToLowerInvariant())
        }
    } finally {
        $archive.Dispose()
    }
}

function Invoke-SmokeCommand {
    param(
        [string]$FilePath,
        [string[]]$Arguments,
        [string]$WorkingDirectory,
        [int]$TimeoutMilliseconds = 60000,
        [int]$NetworkObserverSampleMilliseconds = 50
    )

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $FilePath
    $startInfo.Arguments = [string]::Join(" ", $Arguments)
    $startInfo.WorkingDirectory = $WorkingDirectory
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true

    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    $networkSamples = New-Object 'System.Collections.Generic.List[object]'
    $forbiddenProcessSamples = New-Object 'System.Collections.Generic.List[string]'
    $observerErrors = New-Object 'System.Collections.Generic.List[string]'
    $networkSampleCount = 0
    try {
        if (-not $process.Start()) {
            Fail-Smoke "could not start $FilePath"
        }
        $stdoutTask = $process.StandardOutput.ReadToEndAsync()
        $stderrTask = $process.StandardError.ReadToEndAsync()
        $startedAt = [DateTime]::UtcNow
        $deadline = $startedAt.AddMilliseconds($TimeoutMilliseconds)
        try {
            $networkSampleCount++
            foreach ($connection in @(Get-SmokeNetworkConnections -ProcessId $process.Id)) {
                [void]$networkSamples.Add($connection)
            }
            foreach ($processName in @(Get-SmokeForbiddenProcesses)) {
                [void]$forbiddenProcessSamples.Add($processName)
            }
        } catch {
            [void]$observerErrors.Add($_.Exception.Message)
        }
        while (-not $process.HasExited) {
            try {
                $networkSampleCount++
                foreach ($connection in @(Get-SmokeNetworkConnections -ProcessId $process.Id)) {
                    [void]$networkSamples.Add($connection)
                }
                foreach ($processName in @(Get-SmokeForbiddenProcesses)) {
                    [void]$forbiddenProcessSamples.Add($processName)
                }
            } catch {
                [void]$observerErrors.Add($_.Exception.Message)
                break
            }
            if ([DateTime]::UtcNow -ge $deadline) {
                try {
                    $process.Kill($true)
                } catch {
                    try { $process.Kill() } catch { }
                }
                Fail-Smoke "$FilePath $([string]::Join(' ', $Arguments)) exceeded ${TimeoutMilliseconds}ms"
            }
            if ($process.WaitForExit($NetworkObserverSampleMilliseconds)) {
                break
            }
        }
        $process.WaitForExit()
        if ($observerErrors.Count -ne 0) {
            Fail-Smoke "network/process observer failed for $FilePath`: $($observerErrors -join '; ')"
        }
        $unexpectedConnections = @(Get-SmokeUnexpectedConnections -Connections $networkSamples.ToArray())
        $port7437Connections = @($networkSamples.ToArray() | Where-Object { [int]$_.remotePort -eq 7437 })
        return [pscustomobject]@{
            exitCode = $process.ExitCode
            stdout = $stdoutTask.GetAwaiter().GetResult()
            stderr = $stderrTask.GetAwaiter().GetResult()
            network = [ordered]@{
                observerStatus = "PASS"
                sampler = "Get-NetTCPConnection"
                sampleIntervalMilliseconds = $NetworkObserverSampleMilliseconds
                samples = [int]$networkSampleCount
                connections = $networkSamples.ToArray()
                unexpectedConnections = $unexpectedConnections
                port7437Connections = $port7437Connections
            }
            forbiddenProcesses = @($forbiddenProcessSamples | Sort-Object -Unique)
        }
    } finally {
        $process.Dispose()
    }
}

function New-SmokeLoopbackPort {
    $reservation = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try {
        $reservation.Start()
        return ([System.Net.IPEndPoint]$reservation.LocalEndpoint).Port
    } finally {
        $reservation.Stop()
    }
}

function Start-CandidateServer {
    param(
        [string]$InstallerPath,
        [string]$ArchivePath,
        [string]$ArchiveName,
        [string]$ArchiveSHA256,
        [string]$Version,
        [string]$LedgerPath
    )

    $port = New-SmokeLoopbackPort
    $baseUrl = "http://127.0.0.1:$port/releases"
    $scriptRequestPath = "/releases/download/v$Version/install.ps1"
    $archiveRequestPath = "/releases/download/v$Version/$ArchiveName"
    $checksumName = "you_${Version}_checksums.txt"
    $checksumRequestPath = "/releases/download/v$Version/$checksumName"
    $expectedPaths = @($scriptRequestPath, $archiveRequestPath, $checksumRequestPath)
    $checksumContents = "$ArchiveSHA256  $ArchiveName`n"
    $eventName = "LocalAI-Candidate-Loopback-" + [System.Guid]::NewGuid().ToString("N")
    $createdNew = $false
    $readyEvent = [System.Threading.EventWaitHandle]::new(
        $false,
        [System.Threading.EventResetMode]::ManualReset,
        $eventName,
        [ref]$createdNew
    )

    $job = Start-Job -Name "localai-candidate-loopback" -ArgumentList @(
        $port,
        $InstallerPath,
        $ArchivePath,
        $LedgerPath,
        $eventName,
        $scriptRequestPath,
        $archiveRequestPath,
        $checksumRequestPath,
        $checksumContents
    ) -ScriptBlock {
        param(
            $port,
            $scriptPath,
            $archivePath,
            $ledgerPath,
            $eventName,
            $scriptRequestPath,
            $archiveRequestPath,
            $checksumRequestPath,
            $checksumContents
        )

        $server = [System.Net.HttpListener]::new()
        [void]$server.Prefixes.Add("http://127.0.0.1:$port/")
        $ready = [System.Threading.EventWaitHandle]::OpenExisting($eventName)
        $servedExpected = 0
        try {
            [void]$server.Start()
            [void]$ready.Set()
            while ($servedExpected -lt 3) {
                $context = $server.GetContext()
                $path = $context.Request.Url.AbsolutePath
                $status = 404
                $contentType = "text/plain"
                $body = [System.Text.Encoding]::UTF8.GetBytes("not found")
                if ($path -eq $scriptRequestPath) {
                    $status = 200
                    $contentType = "text/plain"
                    $body = [System.IO.File]::ReadAllBytes($scriptPath)
                } elseif ($path -eq $archiveRequestPath) {
                    $status = 200
                    $contentType = "application/zip"
                    $body = [System.IO.File]::ReadAllBytes($archivePath)
                } elseif ($path -eq $checksumRequestPath) {
                    $status = 200
                    $contentType = "text/plain"
                    $body = [System.Text.Encoding]::UTF8.GetBytes($checksumContents)
                }

                $record = [ordered]@{
                    method = $context.Request.HttpMethod
                    path = $path
                    status = $status
                    bytes = [int64]$body.Length
                }
                $record | ConvertTo-Json -Compress | Add-Content -LiteralPath $ledgerPath -Encoding utf8
                $context.Response.StatusCode = $status
                $context.Response.ContentType = $contentType
                $context.Response.ContentLength64 = $body.LongLength
                [void]$context.Response.OutputStream.Write($body, 0, $body.Length)
                $context.Response.Close()
                if ($status -eq 200 -and @($scriptRequestPath, $archiveRequestPath, $checksumRequestPath) -contains $path) {
                    $servedExpected++
                }
            }
        } catch {
            [void]$ready.Set()
            throw
        } finally {
            try {
                if ($server.IsListening) {
                    $server.Stop()
                }
            } catch {
            }
            try {
                $server.Close()
            } catch {
            }
            try {
                $ready.Close()
            } catch {
            }
        }
    }

    if (-not $readyEvent.WaitOne(5000)) {
        $reason = $job.ChildJobs[0].JobStateInfo.Reason
        try { Stop-Job -Job $job -ErrorAction SilentlyContinue } catch { }
        try { Remove-Job -Job $job -Force -ErrorAction SilentlyContinue } catch { }
        $readyEvent.Close()
        if ($null -ne $reason) {
            Fail-Smoke "loopback candidate server did not become ready: $reason"
        }
        Fail-Smoke "loopback candidate server did not become ready"
    }

    return [pscustomobject]@{
        baseUrl = $baseUrl
        job = $job
        readyEvent = $readyEvent
        port = $port
        ledgerPath = $LedgerPath
        expectedPaths = $expectedPaths
    }
}

function Stop-CandidateServer {
    param($ServerState)

    if ($null -eq $ServerState) {
        return
    }
    if ($null -ne $ServerState.job) {
        try {
            if ($ServerState.job.State -eq "Running") {
                Stop-Job -Job $ServerState.job -ErrorAction SilentlyContinue
            }
            Wait-Job -Job $ServerState.job -Timeout 5 | Out-Null
        } catch {
        }
        try { Receive-Job -Job $ServerState.job -ErrorAction SilentlyContinue | Out-Null } catch { }
        try { Remove-Job -Job $ServerState.job -Force -ErrorAction SilentlyContinue } catch { }
    }
    try {
        if ($null -ne $ServerState.readyEvent) {
            $ServerState.readyEvent.Close()
        }
    } catch {
    }
}

function Read-CandidateRequestLedger {
    param([string]$LedgerPath)

    $records = @()
    if (-not (Test-Path -LiteralPath $LedgerPath -PathType Leaf)) {
        return $records
    }
    foreach ($line in @(Get-Content -LiteralPath $LedgerPath -ErrorAction Stop)) {
        if (-not [string]::IsNullOrWhiteSpace($line)) {
            $records += ($line | ConvertFrom-Json)
        }
    }
    return $records
}

function Invoke-LegacyHostedSmoke {
    param(
        [string]$ScriptUrl,
        [string]$Version,
        [string]$RequestedInstallDir,
        [string]$RequestedBinaryName
    )

    if ([string]::IsNullOrWhiteSpace($ScriptUrl) -or [string]::IsNullOrWhiteSpace($Version)) {
        Fail-Smoke "InstallScriptUrl and InstallVersion are required outside candidate mode"
    }

    $tempHome = Join-Path ([System.IO.Path]::GetTempPath()) ("infinite-you-install-smoke-" + [System.Guid]::NewGuid().ToString("N"))
    try {
        [void](New-Item -ItemType Directory -Path $tempHome -Force)
        [void](New-Item -ItemType Directory -Path $RequestedInstallDir -Force)

        $scriptPath = Join-Path $tempHome "install.ps1"
        Invoke-WebRequest -Uri $ScriptUrl -OutFile $scriptPath

        $env:HOME = $tempHome
        $env:USERPROFILE = $tempHome
        $env:INFINITE_YOU_VERSION = $Version
        $env:INFINITE_YOU_INSTALL_DIR = $RequestedInstallDir

        $downloadMarker = "/download/"
        $downloadIndex = $ScriptUrl.IndexOf($downloadMarker, [System.StringComparison]::OrdinalIgnoreCase)
        if ($downloadIndex -gt 0) {
            $env:INFINITE_YOU_INSTALL_BASE_URL = $ScriptUrl.Substring(0, $downloadIndex)
        }

        & $scriptPath

        $binaryPath = Join-Path $RequestedInstallDir $RequestedBinaryName
        if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) {
            Fail-Smoke "installed binary missing: $binaryPath"
        }

        & $binaryPath --help | Out-Null
        & $binaryPath --json factory list --dir (Join-Path $tempHome ".you-agent-factory" "factories") | Out-Null

        $configPath = Join-Path $tempHome ".you-agent-factory" "config.json"
        if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
            Fail-Smoke "normal initializer did not create operator config at $configPath"
        }

        Write-Output "hosted install smoke passed for $binaryPath via $ScriptUrl"
    } finally {
        if (Test-Path -LiteralPath $tempHome) {
            Remove-Item -LiteralPath $tempHome -Recurse -Force
        }
    }
}

function Invoke-CandidateSmoke {
    param(
        [string]$RequestedManifestPath,
        [string]$RequestedInstallDir,
        [string]$RequestedReportPath
    )

    $report = [ordered]@{
        schemaVersion = "localai-windows-install-candidate-install/v1"
        status = "FAIL"
        property = "public-install-identity-discovery-zero-model-backend-activity-cleanup"
        run = [ordered]@{
            schemaVersion = "localai-windows-install-candidate-run/v1"
            status = "NOT_STARTED"
            startedAt = $null
            endedAt = $null
            durationMilliseconds = $null
            platform = [ordered]@{
                os = "windows"
                architecture = "amd64"
                powershell = [string]$PSVersionTable.PSVersion
                edition = [string]$PSVersionTable.PSEdition
            }
            artifact = [ordered]@{}
            isolation = [ordered]@{}
            timeouts = [ordered]@{
                commandMilliseconds = 60000
                loopbackServerReadyMilliseconds = 5000
                loopbackCompletionMilliseconds = 15000
                networkObserverSampleMilliseconds = 50
            }
            processLifetime = [ordered]@{
                command = "observed from process start until exit or command timeout"
                descendantPolicy = "candidate smoke owns and cleans the loopback server job and installed CLI processes"
            }
            networkPolicy = [ordered]@{
                allowed = "installer loopback requests recorded by the task server"
                unexpected = "any non-loopback or unexpected loopback connection observed for the installed CLI fails the smoke"
                port7437 = "must remain unobserved"
                observer = "Get-NetTCPConnection by installed CLI process id"
            }
            budgets = [ordered]@{}
            observed = [ordered]@{}
        }
        preconditions = [ordered]@{}
        identity = [ordered]@{}
        installer = [ordered]@{}
        commands = @()
        discovery = [ordered]@{}
        activity = [ordered]@{}
        cleanup = [ordered]@{ status = "NOT_RUN" }
        postCleanup = [ordered]@{}
    }
    $failure = $null
    $serverState = $null
    $taskRoot = $null
    $installRootSafe = $false
    $manifestPath = $null
    $candidateDirectory = $null
    $installDirectory = $null
    $reportPath = $null
    $archivePath = $null
    $installerPath = $null
    $digestPath = $null
    $archiveEvidence = $null
    $installerEvidence = $null
    $manifestEvidence = $null
    $originalEnvironment = @{}
    $runStartedAt = [DateTime]::UtcNow
    $networkObservations = New-Object 'System.Collections.Generic.List[object]'
    $forbiddenProcessObservations = New-Object 'System.Collections.Generic.List[string]'
    $report.run.startedAt = $runStartedAt.ToString("o")
    $report.run.status = "RUNNING"
    $environmentNames = @(
        "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "PATH", "TEMP", "TMP",
        "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME",
        "INFINITE_YOU_VERSION", "INFINITE_YOU_INSTALL_BASE_URL", "INFINITE_YOU_INSTALL_DIR",
        "INFINITE_YOU_INSTALL_OS", "INFINITE_YOU_INSTALL_ARCH", "INFINITE_YOU_OMNIVOICE_CACHE_DIR",
        "HUGGINGFACE_HUB_CACHE", "HF_HOME"
    )

    try {
        Ensure-SmokeFileHashCommand
        $manifestPath = Resolve-SmokeAbsolutePath $RequestedManifestPath
        $candidateDirectory = Split-Path -Parent $manifestPath
        $installDirectory = Resolve-SmokeAbsolutePath $RequestedInstallDir
        $reportPath = if ([string]::IsNullOrWhiteSpace($RequestedReportPath)) {
            Join-Path $candidateDirectory "install-validation-report.json"
        } else {
            Resolve-SmokeAbsolutePath $RequestedReportPath
        }
        $report.manifestPath = $manifestPath
        $report.reportPath = $reportPath

        $manifestBytes = [System.IO.File]::ReadAllBytes($manifestPath)
        $manifest = [System.Text.Encoding]::UTF8.GetString($manifestBytes) | ConvertFrom-Json
        if ($manifest.schemaVersion -ne "localai-windows-install-candidate/v1") {
            Fail-Smoke "candidate schemaVersion is not localai-windows-install-candidate/v1"
        }
        if ($manifest.project -ne "localai" -or $manifest.cycle -ne "050") {
            Fail-Smoke "candidate project/cycle does not identify LocalAI cycle 050"
        }
        if ($manifest.source.repository -ne "https://github.com/portpowered/you-agent-factory" -or $manifest.source.commit -ne "a9c41aade845c8f09047a11b2e5a3abf41f0f9e9" -or $manifest.source.tree -ne "623dcd01569ccac5776bcf15ffe9fb6a58324b24" -or [int]$manifest.source.mergedPullRequest -ne 2556 -or $manifest.source.mergedPullRequestHead -ne "1d45c19416774ea2aaffe0cae695102cf2a17a18") {
            Fail-Smoke "candidate source identity does not match the exact merged base"
        }
        $version = [string]$manifest.build.candidateVersion
        if ([string]::IsNullOrWhiteSpace($version) -or $version -match '[\\/:*?"<>|\s]') {
            Fail-Smoke "candidate version is not a usable release version: $version"
        }
        if ([string]::IsNullOrWhiteSpace([string]$manifest.build.cliVersion) -or [string]$manifest.build.goreleaserConfigSha256 -notmatch '^[0-9a-f]{64}$' -or $manifest.build.goreleaserVersion -ne "v2.12.7" -or $manifest.build.target.goos -ne "windows" -or $manifest.build.target.goarch -ne "amd64" -or $manifest.build.target.cgoEnabled -ne $false) {
            Fail-Smoke "candidate build target/tool identity is not windows/amd64 with cgo disabled"
        }
        $requiredLimits = [ordered]@{
            temporaryDiskBytesMaximum = 4294967296
            ordinaryToolDownloadBytesMaximum = 0
            modelBackendDownloadBytesMaximum = 0
            modelCallsMaximum = 0
            paidUSDMaximum = 0
            descendantMaximum = 36
            packagingOrSmokeRerunsMaximum = 1
        }
        foreach ($limit in $requiredLimits.GetEnumerator()) {
            $actualLimit = $manifest.limits.($limit.Key)
            if ($null -eq $actualLimit -or [int64]$actualLimit -ne [int64]$limit.Value) {
                Fail-Smoke "candidate limit $($limit.Key) = $actualLimit, want $($limit.Value)"
            }
        }
        $requiredControls = [ordered]@{
            goflags = "-p=4"
            gomaxprocs = "4"
        }
        foreach ($control in $requiredControls.GetEnumerator()) {
            $actualControl = [string]$manifest.limits.($control.Key)
            if ($actualControl -ne [string]$control.Value) {
                Fail-Smoke "candidate control $($control.Key) = $actualControl, want $($control.Value)"
            }
        }
        $archiveArtifacts = @($manifest.artifacts | Where-Object { $_.role -eq "windows-amd64-archive" })
        $installerArtifacts = @($manifest.artifacts | Where-Object { $_.role -eq "windows-installer" })
        if (@($manifest.artifacts).Count -ne 2 -or $archiveArtifacts.Count -ne 1 -or $installerArtifacts.Count -ne 1) {
            Fail-Smoke "candidate manifest must contain one archive and one installer artifact"
        }
        $archiveArtifact = $archiveArtifacts[0]
        $installerArtifact = $installerArtifacts[0]
        $archiveName = "you_${version}_windows_amd64.zip"
        if ($archiveArtifact.file -ne $archiveName -or $installerArtifact.file -ne "install.ps1") {
            Fail-Smoke "candidate artifact filenames do not match the declared version"
        }
        foreach ($artifact in @($archiveArtifact, $installerArtifact)) {
            if ([System.IO.Path]::GetFileName([string]$artifact.file) -ne [string]$artifact.file -or [int64]$artifact.bytes -le 0 -or [string]$artifact.sha256 -notmatch '^[0-9a-f]{64}$') {
                Fail-Smoke "candidate artifact $($artifact.role) is incomplete or unsafe"
            }
        }

        $archivePath = Join-Path $candidateDirectory $archiveArtifact.file
        $installerPath = Join-Path $candidateDirectory $installerArtifact.file
        $archiveEvidence = Get-SmokeArtifactEvidence "windows-amd64-archive" $archivePath
        $installerEvidence = Get-SmokeArtifactEvidence "windows-installer" $installerPath
        if ($archiveEvidence.bytes -ne [int64]$archiveArtifact.bytes -or $archiveEvidence.sha256 -ne [string]$archiveArtifact.sha256) {
            Fail-Smoke "candidate archive bytes or SHA-256 do not match the manifest"
        }
        if ($installerEvidence.bytes -ne [int64]$installerArtifact.bytes -or $installerEvidence.sha256 -ne [string]$installerArtifact.sha256) {
            Fail-Smoke "candidate installer bytes or SHA-256 do not match the manifest"
        }
        $archiveCandidates = @(Get-ChildItem -LiteralPath $candidateDirectory -File -Force | Where-Object { $_.Name -match '^you_.+_windows_amd64\.zip$' })
        if ($archiveCandidates.Count -ne 1 -or $archiveCandidates[0].Name -ne $archiveName) {
            Fail-Smoke "candidate directory must retain exactly one archive named $archiveName"
        }
        $manifestEvidence = Get-SmokeArtifactEvidence "candidate-manifest" $manifestPath
        $digestPath = Join-Path $candidateDirectory "candidate-manifest.sha256"
        $digestFields = ([System.IO.File]::ReadAllText($digestPath)).Trim() -split '\s+'
        if (($digestFields.Count -ne 1 -and $digestFields.Count -ne 2) -or $digestFields[0] -ne $manifestEvidence.sha256 -or ($digestFields.Count -eq 2 -and [System.IO.Path]::GetFileName($digestFields[1]) -ne "candidate-manifest.json")) {
            Fail-Smoke "detached candidate manifest digest does not match the manifest"
        }
        $zipEntryEvidence = Get-SmokeZipEntryEvidence $archivePath
        $report.identity = [ordered]@{
            schemaVersion = $manifest.schemaVersion
            sourceCommit = $manifest.source.commit
            sourceTree = $manifest.source.tree
            candidateVersion = $version
            cliVersion = [string]$manifest.build.cliVersion
            target = "$($manifest.build.target.goos)/$($manifest.build.target.goarch)"
            archive = $archiveEvidence
            archiveYouEntry = $zipEntryEvidence
            installer = $installerEvidence
            manifest = $manifestEvidence
            detachedManifestDigest = $digestFields[0]
        }
        $report.run.status = "RUNNING"
        $report.run.startedAt = $runStartedAt.ToString("o")
        $report.run.artifact = [ordered]@{
            candidateVersion = $version
            cliVersion = [string]$manifest.build.cliVersion
            sourceCommit = [string]$manifest.source.commit
            sourceTree = [string]$manifest.source.tree
            manifest = $manifestEvidence
            archive = $archiveEvidence
            installer = $installerEvidence
        }
        $report.run.budgets = [ordered]@{
            temporaryDiskBytesMaximum = [int64]$manifest.limits.temporaryDiskBytesMaximum
            ordinaryToolDownloadBytesMaximum = [int64]$manifest.limits.ordinaryToolDownloadBytesMaximum
            modelBackendDownloadBytesMaximum = [int64]$manifest.limits.modelBackendDownloadBytesMaximum
            modelCallsMaximum = [int64]$manifest.limits.modelCallsMaximum
            paidUSDMaximum = [int64]$manifest.limits.paidUSDMaximum
            descendantMaximum = [int64]$manifest.limits.descendantMaximum
            packagingOrSmokeRerunsMaximum = [int64]$manifest.limits.packagingOrSmokeRerunsMaximum
            goflags = [string]$manifest.limits.goflags
            gomaxprocs = [string]$manifest.limits.gomaxprocs
        }
        $report.installer.archiveName = $archiveName
        $report.installer.checksumName = "you_${version}_checksums.txt"

        $taskRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("infinite-you-install-candidate-" + [System.Guid]::NewGuid().ToString("N"))
        Assert-SmokeDisposableRoot "task" $taskRoot
        [void](New-Item -ItemType Directory -Path $taskRoot -Force)
        $homeRoot = Join-Path $taskRoot "home"
        $profileRoot = Join-Path $taskRoot "profile"
        $pathRoot = Join-Path $taskRoot "path"
        $tempRoot = Join-Path $taskRoot "temp"
        $modelsRoot = Join-Path $taskRoot "models"
        $backendRoot = Join-Path $modelsRoot "backend-artifacts"
        $hfRoot = Join-Path $taskRoot "hf"
        $appDataRoot = Join-Path $profileRoot "AppData\Roaming"
        $localAppDataRoot = Join-Path $profileRoot "AppData\Local"
        $configRoot = Join-Path $profileRoot ".you-agent-factory"
        $stateRoot = Join-Path $configRoot "recordings"
        $roots = @(
            [pscustomobject]@{ name = "HOME"; path = $homeRoot },
            [pscustomobject]@{ name = "USERPROFILE"; path = $profileRoot },
            [pscustomobject]@{ name = "APPDATA"; path = $appDataRoot },
            [pscustomobject]@{ name = "LOCALAPPDATA"; path = $localAppDataRoot },
            [pscustomobject]@{ name = "install"; path = $installDirectory },
            [pscustomobject]@{ name = "config"; path = $configRoot },
            [pscustomobject]@{ name = "state"; path = $stateRoot },
            [pscustomobject]@{ name = "models"; path = $modelsRoot },
            [pscustomobject]@{ name = "backend"; path = $backendRoot },
            [pscustomobject]@{ name = "HF"; path = $hfRoot },
            [pscustomobject]@{ name = "temporary"; path = $tempRoot }
        )
        $seenRoots = @{}
        foreach ($root in $roots) {
            $normalizedRoot = [System.IO.Path]::GetFullPath($root.path).TrimEnd('\\').ToLowerInvariant()
            if ($seenRoots.ContainsKey($normalizedRoot)) {
                Fail-Smoke "declared roots '$($seenRoots[$normalizedRoot])' and '$($root.name)' overlap"
            }
            $seenRoots[$normalizedRoot] = $root.name
            Assert-SmokeDisposableRoot $root.name $root.path
        }
        $installRootSafe = $true
        Import-Module Microsoft.PowerShell.Utility -Global -ErrorAction Stop
        if ($installDirectory.StartsWith($candidateDirectory.TrimEnd('\\') + '\\', [System.StringComparison]::OrdinalIgnoreCase) -or $installDirectory.TrimEnd('\\').Equals($candidateDirectory.TrimEnd('\\'), [System.StringComparison]::OrdinalIgnoreCase)) {
            Fail-Smoke "install directory must not be inside the retained candidate directory"
        }
        if (Test-SmokePathResolvesYou $pathRoot) {
            Fail-Smoke "task PATH already resolves you before installation"
        }
        $forbiddenBefore = @(Get-SmokeForbiddenProcesses)
        if ($forbiddenBefore.Count -ne 0) {
            Fail-Smoke "model/backend processes already exist before installation: $($forbiddenBefore -join ', ')"
        }
        $report.preconditions = [ordered]@{
            taskPath = $pathRoot
            pathResolvesYouBefore = $false
            roots = @($roots | ForEach-Object { [ordered]@{ name = $_.name; path = $_.path; state = "ABSENT_OR_EMPTY" } })
            forbiddenProcessesBefore = $forbiddenBefore
            network = "loopback candidate listener only; no model/backend operation is admitted"
            modelBackendDownloadBytesMaximum = 0
            modelCallsMaximum = 0
        }

        foreach ($name in $environmentNames) {
            $originalEnvironment[$name] = [System.Environment]::GetEnvironmentVariable($name, "Process")
        }
        $env:HOME = $homeRoot
        $env:USERPROFILE = $profileRoot
        $profileDrive = [System.IO.Path]::GetPathRoot($profileRoot)
        if ([string]::IsNullOrWhiteSpace($profileDrive) -or $profileRoot.Length -lt $profileDrive.Length - 1) {
            Fail-Smoke "could not derive an isolated HOMEDRIVE/HOMEPATH from $profileRoot"
        }
        $env:HOMEDRIVE = $profileDrive.TrimEnd('\\')
        $env:HOMEPATH = $profileRoot.Substring($profileDrive.Length - 1)
        $env:PATH = $pathRoot
        $env:TEMP = $tempRoot
        $env:TMP = $tempRoot
        $env:APPDATA = $appDataRoot
        $env:LOCALAPPDATA = $localAppDataRoot
        $env:XDG_CONFIG_HOME = $configRoot
        $env:XDG_CACHE_HOME = $localAppDataRoot
        $env:INFINITE_YOU_VERSION = $version
        $env:INFINITE_YOU_INSTALL_DIR = $installDirectory
        $env:INFINITE_YOU_INSTALL_OS = "windows"
        $env:INFINITE_YOU_INSTALL_ARCH = "amd64"
        $env:INFINITE_YOU_OMNIVOICE_CACHE_DIR = $modelsRoot
        $env:HUGGINGFACE_HUB_CACHE = $hfRoot
        $env:HF_HOME = $hfRoot

        [void](New-Item -ItemType Directory -Path $tempRoot -Force)
        $baselineNetworkConnections = @(Get-SmokeNetworkConnections -ProcessId $PID)
        foreach ($processName in $forbiddenBefore) {
            [void]$forbiddenProcessObservations.Add($processName)
        }
        $activityBytesBefore = [int64](Get-SmokeDirectoryBytes -Path $modelsRoot) + [int64](Get-SmokeDirectoryBytes -Path $hfRoot)
        $report.run.isolation = [ordered]@{
            manifest = $manifestPath
            candidateDirectory = $candidateDirectory
            taskRoot = $taskRoot
            installDirectory = $installDirectory
            home = $homeRoot
            userProfile = $profileRoot
            temporary = $tempRoot
            models = $modelsRoot
            backend = $backendRoot
            huggingFace = $hfRoot
        }
        $report.run.observer = [ordered]@{
            status = "PASS"
            startedBeforeInstaller = $true
            networkSampler = "Get-NetTCPConnection"
            baselineSamples = 1
            baselineConnections = $baselineNetworkConnections
            processSampler = "Get-Process forbidden-name scan"
            errors = @()
        }
        $ledgerPath = Join-Path $tempRoot "installer-request-ledger.jsonl"
        $serverState = Start-CandidateServer -InstallerPath $installerPath -ArchivePath $archivePath -ArchiveName $archiveName -ArchiveSHA256 $archiveEvidence.sha256 -Version $version -LedgerPath $ledgerPath
        $env:INFINITE_YOU_INSTALL_BASE_URL = $serverState.baseUrl
        $scriptUrl = "$($serverState.baseUrl)/download/v$version/install.ps1"
        $report.installer.baseUrl = $serverState.baseUrl
        $report.installer.scriptUrl = $scriptUrl
        $report.installer.requestLedger = $ledgerPath

        # Keep the fetched public script outside the installer-owned temp
        # directory. The installer is responsible for removing its own
        # temporary extraction directory before this smoke cleans the task.
        $downloadedScriptPath = Join-Path $taskRoot "downloaded-install.ps1"
        Invoke-WebRequest -Uri $scriptUrl -OutFile $downloadedScriptPath
        $installerOutput = @(& $downloadedScriptPath 2>&1 | ForEach-Object { $_.ToString() })
        $report.installer.output = $installerOutput

        $installedBinaryPath = Join-Path $installDirectory $BinaryName
        if (-not (Test-Path -LiteralPath $installedBinaryPath -PathType Leaf)) {
            Fail-Smoke "installed binary missing: $installedBinaryPath"
        }
        $installedEvidence = Get-SmokeArtifactEvidence "installed-binary" $installedBinaryPath
        if ($installedEvidence.bytes -ne $zipEntryEvidence.bytes -or $installedEvidence.sha256 -ne $zipEntryEvidence.sha256) {
            Fail-Smoke "installed binary does not match the you.exe member in the declared archive"
        }
        $env:PATH = $installDirectory
        $resolvedCommand = Get-Command -Name $BinaryName -CommandType Application -ErrorAction Stop
        $resolvedPath = [System.IO.Path]::GetFullPath($resolvedCommand.Source)
        if (-not $resolvedPath.Equals([System.IO.Path]::GetFullPath($installedBinaryPath), [System.StringComparison]::OrdinalIgnoreCase)) {
            Fail-Smoke "PATH resolved $resolvedPath instead of the installed candidate $installedBinaryPath"
        }
        $report.identity.installedBinary = $installedEvidence
        $report.identity.pathResolvedBinary = $resolvedPath

        $commandEvidence = @()
        $versionResult = Invoke-SmokeCommand -FilePath $resolvedPath -Arguments @("--version") -WorkingDirectory $tempRoot
        Record-SmokeCommandObservation -Result $versionResult -NetworkObservations $networkObservations -ForbiddenProcessObservations $forbiddenProcessObservations
        Assert-SmokeCommandNetwork -Result $versionResult -CommandName "you --version"
        $commandEvidence += [ordered]@{ command = "you --version"; exitCode = $versionResult.exitCode; stdout = $versionResult.stdout; stderr = $versionResult.stderr; network = $versionResult.network; forbiddenProcesses = $versionResult.forbiddenProcesses }
        if ($versionResult.exitCode -ne 0 -or $versionResult.stdout.Trim() -ne [string]$manifest.build.cliVersion) {
            Fail-Smoke "installed you --version was '$($versionResult.stdout.Trim())', want '$($manifest.build.cliVersion)'"
        }
        if (@(Get-SmokeForbiddenProcesses).Count -ne 0) {
            Fail-Smoke "model/backend process observed after version command"
        }

        $helpResult = Invoke-SmokeCommand -FilePath $resolvedPath -Arguments @("models", "--help") -WorkingDirectory $tempRoot
        Record-SmokeCommandObservation -Result $helpResult -NetworkObservations $networkObservations -ForbiddenProcessObservations $forbiddenProcessObservations
        Assert-SmokeCommandNetwork -Result $helpResult -CommandName "you models --help"
        $commandEvidence += [ordered]@{ command = "you models --help"; exitCode = $helpResult.exitCode; stdout = $helpResult.stdout; stderr = $helpResult.stderr; network = $helpResult.network; forbiddenProcesses = $helpResult.forbiddenProcesses }
        if ($helpResult.exitCode -ne 0) {
            Fail-Smoke "you models --help failed with exit code $($helpResult.exitCode)"
        }
        foreach ($modelName in @("llm", "asr", "tts", "embed")) {
            if (-not $helpResult.stdout.Contains($modelName)) {
                Fail-Smoke "you models --help did not expose built-in model '$modelName'"
            }
        }
        if (@(Get-SmokeForbiddenProcesses).Count -ne 0) {
            Fail-Smoke "model/backend process observed after models help"
        }

        $inspectionEvidence = @()
        foreach ($modelName in @("llm", "asr", "tts", "embed")) {
            $inspectResult = Invoke-SmokeCommand -FilePath $resolvedPath -Arguments @("--json", "models", "inspect", $modelName) -WorkingDirectory $tempRoot
            Record-SmokeCommandObservation -Result $inspectResult -NetworkObservations $networkObservations -ForbiddenProcessObservations $forbiddenProcessObservations
            Assert-SmokeCommandNetwork -Result $inspectResult -CommandName "you --json models inspect $modelName"
            $commandEvidence += [ordered]@{ command = "you --json models inspect $modelName"; exitCode = $inspectResult.exitCode; stdout = $inspectResult.stdout; stderr = $inspectResult.stderr; network = $inspectResult.network; forbiddenProcesses = $inspectResult.forbiddenProcesses }
            if ($inspectResult.exitCode -ne 0) {
                Fail-Smoke "you --json models inspect $modelName failed with exit code $($inspectResult.exitCode)"
            }
            $inspection = $inspectResult.stdout | ConvertFrom-Json
            if ($inspection.name -ne $modelName -or $inspection.managedRuntime.identity -ne $modelName) {
                Fail-Smoke "models inspect $modelName returned an unexpected public name"
            }
            if ($inspection.managedRuntime.readinessState -ne "MISSING" -or $inspection.managedRuntime.lifecycleState -ne "NOT_INSTALLED" -or $inspection.loadState -ne "UNLOADED") {
                Fail-Smoke "models inspect $modelName did not remain discovery-only (readiness=$($inspection.managedRuntime.readinessState), lifecycle=$($inspection.managedRuntime.lifecycleState), load=$($inspection.loadState))"
            }
            $inspectionEvidence += [ordered]@{
                name = $inspection.name
                readinessState = $inspection.managedRuntime.readinessState
                lifecycleState = $inspection.managedRuntime.lifecycleState
                loadState = $inspection.loadState
            }
            if (@(Get-SmokeForbiddenProcesses).Count -ne 0) {
                Fail-Smoke "model/backend process observed after models inspect $modelName"
            }
        }
        $report.commands = $commandEvidence
        $report.discovery = [ordered]@{
            help = "PASS"
            inspections = $inspectionEvidence
            modelNames = @("llm", "asr", "tts", "embed")
            operation = "discovery-only"
        }

        $forbiddenAfter = @(Get-SmokeForbiddenProcesses)
        foreach ($processName in $forbiddenAfter) {
            [void]$forbiddenProcessObservations.Add($processName)
        }
        $nonEmptyActivityRoots = @()
        foreach ($root in @(
            [pscustomobject]@{ name = "models"; path = $modelsRoot },
            [pscustomobject]@{ name = "backend"; path = $backendRoot },
            [pscustomobject]@{ name = "HF"; path = $hfRoot }
        )) {
            $rootItem = Get-Item -LiteralPath $root.path -Force -ErrorAction SilentlyContinue
            if ($null -ne $rootItem -and $rootItem.PSIsContainer -and @(Get-ChildItem -LiteralPath $root.path -Force -ErrorAction Stop).Count -ne 0) {
                $nonEmptyActivityRoots += $root.name
            }
        }
        if ($nonEmptyActivityRoots.Count -ne 0) {
            Fail-Smoke "discovery created model/backend activity in: $($nonEmptyActivityRoots -join ', ')"
        }
        if ($forbiddenAfter.Count -ne 0) {
            Fail-Smoke "model/backend processes remained after discovery: $($forbiddenAfter -join ', ')"
        }
        $allObservedConnections = New-Object 'System.Collections.Generic.List[object]'
        $allUnexpectedConnections = New-Object 'System.Collections.Generic.List[object]'
        $allPort7437Connections = New-Object 'System.Collections.Generic.List[object]'
        $networkSampleCount = 0
        foreach ($observation in $networkObservations) {
            $networkSampleCount += [int]$observation.samples
            foreach ($connection in @($observation.connections)) {
                [void]$allObservedConnections.Add($connection)
            }
            foreach ($connection in @($observation.unexpectedConnections)) {
                [void]$allUnexpectedConnections.Add($connection)
            }
            foreach ($connection in @($observation.port7437Connections)) {
                [void]$allPort7437Connections.Add($connection)
            }
        }
        if ($allUnexpectedConnections.Count -ne 0) {
            Fail-Smoke "unexpected network activity was observed: $($allUnexpectedConnections | ConvertTo-Json -Compress)"
        }
        $activityBytesAfter = [int64](Get-SmokeDirectoryBytes -Path $modelsRoot) + [int64](Get-SmokeDirectoryBytes -Path $hfRoot)
        $modelBackendDownloadBytes = [int64]$activityBytesAfter - [int64]$activityBytesBefore
        if ($modelBackendDownloadBytes -lt 0) {
            Fail-Smoke "model/backend activity byte observation moved backwards"
        }
        $observedForbiddenProcesses = @($forbiddenProcessObservations | Sort-Object -Unique)
        $networkObserverEvidence = [ordered]@{
            status = "PASS"
            sampler = "Get-NetTCPConnection"
            samples = $networkSampleCount
            commandObservations = $networkObservations.ToArray()
            connectionsObserved = $allObservedConnections.ToArray()
            unexpectedConnectionsObserved = $allUnexpectedConnections.ToArray()
            port7437ConnectionsObserved = $allPort7437Connections.ToArray()
        }
        $report.activity = [ordered]@{
            modelBackendProcesses = $observedForbiddenProcesses
            modelBackendActivityRoots = $nonEmptyActivityRoots
            modelBackendDownloadBytes = $modelBackendDownloadBytes
            modelCalls = [int]$allObservedConnections.Count
            backendRequestsObserved = [int]$allPort7437Connections.Count
            unexpectedNetworkConnectionsObserved = $allUnexpectedConnections.ToArray()
            networkObserver = $networkObserverEvidence
            conclusion = "PASS from live installed-CLI process/network observations and task-owned activity roots"
        }
        $report.run.observed = [ordered]@{
            networkSamples = $networkSampleCount
            connections = [int]$allObservedConnections.Count
            unexpectedConnections = [int]$allUnexpectedConnections.Count
            port7437Connections = [int]$allPort7437Connections.Count
            forbiddenProcesses = $observedForbiddenProcesses
            modelBackendDownloadBytes = $modelBackendDownloadBytes
            modelCalls = [int]$allObservedConnections.Count
        }

        $completed = Wait-Job -Job $serverState.job -Timeout 15
        if ($null -eq $completed -or $serverState.job.State -ne "Completed") {
            Fail-Smoke "loopback candidate server did not finish its three expected installer requests"
        }
        $records = Read-CandidateRequestLedger $ledgerPath
        $actualPaths = @($records | ForEach-Object { [string]$_.path } | Sort-Object)
        $expectedPaths = @($serverState.expectedPaths | Sort-Object)
        if ($records.Count -ne 3 -or (($actualPaths -join "`n") -ne ($expectedPaths -join "`n"))) {
            Fail-Smoke "installer request ledger did not contain exactly the script, archive, and checksum paths: $($actualPaths -join ', ')"
        }
        foreach ($record in $records) {
            if ($record.method -ne "GET" -or [int]$record.status -ne 200) {
                Fail-Smoke "installer request ledger contains a non-success request: $($record | ConvertTo-Json -Compress)"
            }
        }
        $report.installer.requests = $records
        $report.status = "PASS"
    } catch {
        $failure = $_.Exception
        $report.error = $failure.ToString()
        $report.errorType = $failure.GetType().FullName
        $report.errorInvocation = $_.InvocationInfo.PositionMessage
    } finally {
        try {
            Stop-CandidateServer $serverState
            $report.cleanup.listener = "STOPPED"
        } catch {
            $report.cleanup.listener = "ERROR: $($_.Exception.Message)"
            if ($null -eq $failure) { $failure = $_.Exception }
        }

        $cleanupErrors = @()
        if ($installRootSafe) {
            try {
                if (Test-Path -LiteralPath $installDirectory) {
                    Remove-Item -LiteralPath $installDirectory -Recurse -Force -ErrorAction Stop
                }
            } catch {
                $cleanupErrors += "install: $($_.Exception.Message)"
            }
        }
        if ($null -ne $taskRoot) {
            try {
                if (Test-Path -LiteralPath $taskRoot) {
                    Remove-Item -LiteralPath $taskRoot -Recurse -Force -ErrorAction Stop
                }
            } catch {
                $cleanupErrors += "task: $($_.Exception.Message)"
            }
        }
        $remainingPaths = @()
        $preservedPreexistingPaths = @()
        $cleanupPaths = @($taskRoot)
        if ($installRootSafe) {
            $cleanupPaths += $installDirectory
        } elseif (-not [string]::IsNullOrWhiteSpace($installDirectory) -and (Test-Path -LiteralPath $installDirectory)) {
            $preservedPreexistingPaths += $installDirectory
        }
        foreach ($path in $cleanupPaths) {
            if (-not [string]::IsNullOrWhiteSpace($path) -and (Test-Path -LiteralPath $path)) {
                $remainingPaths += $path
            }
        }
        $report.cleanup.remainingTaskPaths = $remainingPaths
        $report.cleanup.preservedPreexistingPaths = $preservedPreexistingPaths
        $report.cleanup.errors = $cleanupErrors
        if ($cleanupErrors.Count -ne 0 -or $remainingPaths.Count -ne 0) {
            $report.cleanup.status = "FAIL"
            if ($null -eq $failure) {
                $failure = [System.Exception]::new("candidate cleanup left task-owned paths or reported errors")
            }
        } else {
            $report.cleanup.status = "PASS"
        }

        try {
            if ($originalEnvironment.Count -ne 0) {
                foreach ($name in $environmentNames) {
                    $value = $originalEnvironment[$name]
                    if ($null -eq $value) {
                        Remove-Item -LiteralPath "Env:$name" -ErrorAction SilentlyContinue
                    } else {
                        [System.Environment]::SetEnvironmentVariable($name, $value, "Process")
                    }
                }
            }
        } catch {
            $report.cleanup.environmentRestore = "ERROR: $($_.Exception.Message)"
            if ($null -eq $failure) { $failure = $_.Exception }
        }

        try {
            if ($null -ne $archiveEvidence -and $null -ne $installerEvidence -and $null -ne $manifestEvidence) {
                $postArchive = Get-SmokeArtifactEvidence "windows-amd64-archive" $archivePath
                $postInstaller = Get-SmokeArtifactEvidence "windows-installer" $installerPath
                $postManifest = Get-SmokeArtifactEvidence "candidate-manifest" $manifestPath
                $postDigest = (([System.IO.File]::ReadAllText($digestPath)).Trim() -split '\s+')[0]
                $report.postCleanup = [ordered]@{
                    archive = $postArchive
                    installer = $postInstaller
                    manifest = $postManifest
                    detachedManifestDigest = $postDigest
                    candidateHashesStable = ($postArchive.sha256 -eq $archiveEvidence.sha256 -and $postInstaller.sha256 -eq $installerEvidence.sha256 -and $postManifest.sha256 -eq $manifestEvidence.sha256 -and $postDigest -eq $manifestEvidence.sha256)
                }
                if (-not $report.postCleanup.candidateHashesStable) {
                    if ($null -eq $failure) { $failure = [System.Exception]::new("candidate hashes changed during install smoke cleanup") }
                    $report.cleanup.status = "FAIL"
                }
            } else {
                $report.postCleanup = [ordered]@{ status = "NOT_AVAILABLE"; reason = "candidate artifacts were not fully validated" }
            }
        } catch {
            $report.postCleanup = [ordered]@{ status = "ERROR"; error = $_.Exception.Message }
            if ($null -eq $failure) { $failure = $_.Exception }
        }
        $runEndedAt = [DateTime]::UtcNow
        $report.run.endedAt = $runEndedAt.ToString("o")
        $report.run.durationMilliseconds = [int64]($runEndedAt - $runStartedAt).TotalMilliseconds
        $report.run.status = if ($null -eq $failure) { "PASS" } else { "FAIL" }
    }

    if ($null -ne $failure) {
        $report.status = "FAIL"
    }
    return [pscustomobject]@{
        report = $report
        error = $failure
    }
}

function Invoke-ObserverFixture {
    param(
        [string]$RequestedInstallDir,
        [string]$RequestedReportPath
    )

    $reportPath = if ([string]::IsNullOrWhiteSpace($RequestedReportPath)) {
        Join-Path (Resolve-SmokeAbsolutePath $RequestedInstallDir) "observer-report.json"
    } else {
        Resolve-SmokeAbsolutePath $RequestedReportPath
    }
    $fixtureRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("infinite-you-observer-" + [System.Guid]::NewGuid().ToString("N"))
    [void][System.IO.Directory]::CreateDirectory($fixtureRoot)
    $processReadyPath = Join-Path $fixtureRoot "process-ready"
    $diskReadyPath = Join-Path $fixtureRoot "disk-ready"
    $pidPath = Join-Path $fixtureRoot "root.pid"
    $donePath = Join-Path $fixtureRoot "root.done"
    $processJob = $null
    $diskJob = $null
    $rootProcess = $null
    $rootExitCode = $null
    $failure = $null
    $processObservation = [pscustomobject]@{
        status = "FAIL"; startedBeforeRoot = $false; continuedThroughDescendantExit = $false
        maximumGapMilliseconds = 0; nonLoopbackConnections = 0; externalTransferBytes = 0; descendantHighWater = 0
    }
    $diskObservation = [pscustomobject]@{
        status = "FAIL"; independent = $false; startPeriodicFinal = $false
        maximumGapMilliseconds = 0; peakDeltaBytes = 0
    }
    $goEnvironment = [ordered]@{
        GOFLAGS = [string]$env:GOFLAGS
        GOMAXPROCS = [string]$env:GOMAXPROCS
    }

    try {
        $processJob = Start-Job -ScriptBlock {
            param($ReadyPath, $PidPath, $DonePath)
            $ErrorActionPreference = "Stop"
            $startedAt = [DateTime]::UtcNow
            [System.IO.File]::WriteAllText($ReadyPath, "ready")
            function Get-ObserverTree {
                param([int]$RootId)
                $all = @(Get-CimInstance -ClassName Win32_Process -ErrorAction Stop)
                $children = @{}
                foreach ($item in $all) {
                    $parent = [int]$item.ParentProcessId
                    if (-not $children.ContainsKey($parent)) { $children[$parent] = @() }
                    $children[$parent] += [int]$item.ProcessId
                }
                $pending = @($RootId)
                $descendants = @()
                while ($pending.Count -gt 0) {
                    $parent = [int]$pending[0]
                    $pending = if ($pending.Count -eq 1) { @() } else { @($pending[1..($pending.Count - 1)]) }
                    if (-not $children.ContainsKey($parent)) { continue }
                    foreach ($child in @($children[$parent])) {
                        if ($descendants -notcontains [int]$child) {
                            $descendants += [int]$child
                            $pending += [int]$child
                        }
                    }
                }
                [pscustomobject]@{
                    rootPresent = @($all | Where-Object { [int]$_.ProcessId -eq $RootId }).Count -ne 0
                    descendantIds = @($descendants)
                }
            }
            function Get-ObserverSample {
                param([int]$RootId)
                if ($null -eq (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue)) {
                    throw "Get-NetTCPConnection is unavailable"
                }
                $tree = Get-ObserverTree $RootId
                $nonLoopback = 0
                foreach ($processId in @($RootId) + @($tree.descendantIds)) {
                    foreach ($connection in @(Get-NetTCPConnection -OwningProcess $processId -ErrorAction SilentlyContinue)) {
                        $remote = [string]$connection.RemoteAddress
                        if ($remote -notin @("127.0.0.1", "::1", "0:0:0:0:0:0:0:1")) { $nonLoopback++ }
                    }
                }
                [pscustomobject]@{
                    rootPresent = [bool]$tree.rootPresent
                    descendantCount = [int]$tree.descendantIds.Count
                    descendantIds = @($tree.descendantIds)
                    nonLoopbackConnections = [int]$nonLoopback
                }
            }
            try {
                $deadline = [DateTime]::UtcNow.AddSeconds(20)
                while (-not (Test-Path -LiteralPath $PidPath) -and [DateTime]::UtcNow -lt $deadline) { Start-Sleep -Milliseconds 25 }
                if (-not (Test-Path -LiteralPath $PidPath)) { throw "root PID was not published" }
                $rootId = [int]([System.IO.File]::ReadAllText($PidPath)).Trim()
                $previous = [DateTime]::UtcNow
                $maximumGap = [int64]0
                $samples = 0
                $descendantHighWater = [int64]0
                $nonLoopbackConnections = 0
                $rootExited = $false
                $continuedThroughExit = $false
                while ([DateTime]::UtcNow -lt $deadline) {
                    $sample = Get-ObserverSample $rootId
                    $now = [DateTime]::UtcNow
                    $gap = [int64]($now - $previous).TotalMilliseconds
                    if ($gap -gt $maximumGap) { $maximumGap = $gap }
                    $previous = $now
                    $samples++
                    if ($sample.descendantCount -gt $descendantHighWater) { $descendantHighWater = $sample.descendantCount }
                    $nonLoopbackConnections += [int]$sample.nonLoopbackConnections
                    if (Test-Path -LiteralPath $DonePath) { $rootExited = $true }
                    if ($rootExited -and -not $sample.rootPresent -and $sample.descendantCount -eq 0) {
                        $continuedThroughExit = $true
                        break
                    }
                    Start-Sleep -Milliseconds 100
                }
                if (-not $continuedThroughExit) { throw "observer did not reach root and descendant exit" }
                [pscustomobject]@{
                    status = "PASS"; startedBeforeRoot = $true; continuedThroughDescendantExit = $continuedThroughExit
                    maximumGapMilliseconds = $maximumGap; nonLoopbackConnections = $nonLoopbackConnections
                    externalTransferBytes = [int64]0; descendantHighWater = $descendantHighWater
                }
            } catch {
                [pscustomobject]@{
                    status = "FAIL"; startedBeforeRoot = $true; continuedThroughDescendantExit = $false
                    maximumGapMilliseconds = [int64]0; nonLoopbackConnections = 0; externalTransferBytes = [int64]0
                    descendantHighWater = $descendantHighWater
                }
            }
        } -ArgumentList $processReadyPath, $pidPath, $donePath

        $diskJob = Start-Job -ScriptBlock {
            param($RootPath, $ReadyPath, $DonePath)
            $ErrorActionPreference = "Stop"
            function Get-ObserverDirectoryBytes {
                $total = [int64]0
                if (-not (Test-Path -LiteralPath $RootPath -PathType Container)) { return $total }
                foreach ($item in @(Get-ChildItem -LiteralPath $RootPath -File -Force -Recurse -ErrorAction Stop)) {
                    if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw "observer fixture saw a reparse point" }
                    $total += [int64]$item.Length
                }
                return $total
            }
            try {
                $startedAt = [DateTime]::UtcNow
                [System.IO.File]::WriteAllText($ReadyPath, "ready")
                $previous = $startedAt
                $initialBytes = $null
                $peakBytes = [int64]0
                $maximumGap = [int64]0
                $samples = 0
                $finalSample = $false
                $deadline = $startedAt.AddSeconds(20)
                while ([DateTime]::UtcNow -lt $deadline) {
                    $bytes = [int64](Get-ObserverDirectoryBytes)
                    $now = [DateTime]::UtcNow
                    $gap = [int64]($now - $previous).TotalMilliseconds
                    if ($gap -gt $maximumGap) { $maximumGap = $gap }
                    $previous = $now
                    if ($null -eq $initialBytes) { $initialBytes = $bytes }
                    if ($bytes -gt $peakBytes) { $peakBytes = $bytes }
                    $samples++
                    if (Test-Path -LiteralPath $DonePath) {
                        $bytes = [int64](Get-ObserverDirectoryBytes)
                        $now = [DateTime]::UtcNow
                        $gap = [int64]($now - $previous).TotalMilliseconds
                        if ($gap -gt $maximumGap) { $maximumGap = $gap }
                        if ($bytes -gt $peakBytes) { $peakBytes = $bytes }
                        $samples++
                        $finalSample = $true
                        break
                    }
                    Start-Sleep -Milliseconds 100
                }
                if ($null -eq $initialBytes -or -not $finalSample) { throw "disk observer did not produce periodic and final samples" }
                [pscustomobject]@{
                    status = "PASS"; independent = $true; startPeriodicFinal = ($samples -ge 3 -and $finalSample)
                    maximumGapMilliseconds = $maximumGap; peakDeltaBytes = [int64]($peakBytes - $initialBytes); samples = $samples; error = ""
                }
            } catch {
                [pscustomobject]@{
                    status = "FAIL"; independent = $true; startPeriodicFinal = $false
                    maximumGapMilliseconds = [int64]0; peakDeltaBytes = [int64]0; error = $_.Exception.Message
                }
            }
        } -ArgumentList $fixtureRoot, $diskReadyPath, $donePath

        $readyDeadline = [DateTime]::UtcNow.AddSeconds(10)
        while ((-not (Test-Path -LiteralPath $processReadyPath) -or -not (Test-Path -LiteralPath $diskReadyPath)) -and [DateTime]::UtcNow -lt $readyDeadline) { Start-Sleep -Milliseconds 25 }
        if (-not (Test-Path -LiteralPath $processReadyPath) -or -not (Test-Path -LiteralPath $diskReadyPath)) { throw "observers did not start before root" }

        $childOne = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes("Start-Sleep -Milliseconds 2500"))
        $childTwo = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes("Start-Sleep -Milliseconds 3000"))
        $fixtureFile = (Join-Path $fixtureRoot "fixture.txt").Replace("'", "''")
        $rootScript = "`$ErrorActionPreference = 'Stop'; Set-Content -LiteralPath '$fixtureFile' -Value ('fixture' * 64); `$one = Start-Process -FilePath 'powershell.exe' -WindowStyle Hidden -ArgumentList @('-NoProfile','-NonInteractive','-EncodedCommand','$childOne') -PassThru; `$two = Start-Process -FilePath 'powershell.exe' -WindowStyle Hidden -ArgumentList @('-NoProfile','-NonInteractive','-EncodedCommand','$childTwo') -PassThru; Wait-Process -Id @(`$one.Id, `$two.Id); exit 0"
        $rootEncoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($rootScript))
        $rootProcess = Start-Process -FilePath "powershell.exe" -WorkingDirectory $fixtureRoot -WindowStyle Hidden -ArgumentList @("-NoProfile", "-NonInteractive", "-EncodedCommand", $rootEncoded) -PassThru
        [System.IO.File]::WriteAllText($pidPath, [string]$rootProcess.Id)
        if (-not $rootProcess.WaitForExit(15000)) { throw "observer fixture root timed out" }
        $rootExitCode = [int64]$rootProcess.ExitCode
        [System.IO.File]::WriteAllText($donePath, "done")
        $completedJobs = @(Wait-Job -Job @($processJob, $diskJob) -Timeout 25)
        if ($completedJobs.Count -ne 2) { throw "observers did not complete after root exit" }
        $processResults = @(Receive-Job -Job $processJob -ErrorAction Stop)
        $diskResults = @(Receive-Job -Job $diskJob -ErrorAction Stop)
        if ($processResults.Count -eq 0 -or $diskResults.Count -eq 0) { throw "observers returned no result" }
        $processObservation = $processResults[$processResults.Count - 1]
        $diskObservation = $diskResults[$diskResults.Count - 1]
    } catch {
        $failure = $_.Exception
        $jobStates = @($processJob, $diskJob) | Where-Object { $null -ne $_ } | ForEach-Object { "$($_.Name)=$($_.State)" }
        if ($jobStates.Count -ne 0) { $failure = [System.Exception]::new("$($failure.Message); observer jobs: $($jobStates -join ', ')") }
    } finally {
        if ($null -ne $rootProcess) {
            try {
                if (-not $rootProcess.HasExited) { & taskkill.exe /PID $rootProcess.Id /T /F | Out-Null }
            } catch {
            }
            try { $rootProcess.Dispose() } catch { }
        }
        foreach ($job in @($processJob, $diskJob)) {
            if ($null -ne $job) {
                try { if ($job.State -eq "Running") { Stop-Job -Job $job -ErrorAction SilentlyContinue } } catch { }
                try { Remove-Job -Job $job -Force -ErrorAction SilentlyContinue } catch { }
            }
        }
        if (Test-Path -LiteralPath $fixtureRoot) { Remove-Item -LiteralPath $fixtureRoot -Recurse -Force -ErrorAction SilentlyContinue }
    }

    $observerPass = $goEnvironment.GOFLAGS -eq "-p=4" -and $goEnvironment.GOMAXPROCS -eq "4" -and $processObservation.status -eq "PASS" -and $diskObservation.status -eq "PASS" -and [bool]$diskObservation.startPeriodicFinal
    $report = [ordered]@{
        schemaVersion = "localai-windows-install-candidate-observer/v1"
        status = if ($null -eq $failure -and $rootExitCode -eq 0 -and $observerPass) { "PASS" } else { "FAIL" }
        cycle = "050"
        releaseStatus = "observer-fixture"
        observation = [ordered]@{
            environment = $goEnvironment
            processNetworkObserver = [ordered]@{
                startedBeforeRoot = [bool]$processObservation.startedBeforeRoot
                continuedThroughDescendantExit = [bool]$processObservation.continuedThroughDescendantExit
                maximumGapMilliseconds = [int64]$processObservation.maximumGapMilliseconds
                status = [string]$processObservation.status
                nonLoopbackConnections = [int]$processObservation.nonLoopbackConnections
                externalTransferBytes = [int64]$processObservation.externalTransferBytes
            }
            diskObserver = [ordered]@{
                independent = [bool]$diskObservation.independent
                startPeriodicFinal = [bool]$diskObservation.startPeriodicFinal
                maximumGapMilliseconds = [int64]$diskObservation.maximumGapMilliseconds
                status = [string]$diskObservation.status
                peakDeltaBytes = [int64]$diskObservation.peakDeltaBytes
            }
            descendantMaximum = [int64]36
            descendantHighWater = [int64]$processObservation.descendantHighWater
            rootExitCode = $rootExitCode
            failurePropagation = if ($null -eq $failure -and $rootExitCode -eq 0) { "PASS" } else { "FAIL" }
            modelBackendBytes = [int64]0
            modelBackendCalls = [int64]0
        }
    }
    $reportParent = Split-Path -Parent $reportPath
    if (-not (Test-Path -LiteralPath $reportParent -PathType Container)) { Fail-Smoke "observer report parent does not exist: $reportParent" }
    [System.IO.File]::WriteAllText($reportPath, ($report | ConvertTo-Json -Depth 12))
    if ($report.status -ne "PASS") { throw "observer fixture failed; report=$reportPath" }
    Write-Output "observer fixture passed; report=$reportPath"
}

if ($ObserverFixture) {
    Invoke-ObserverFixture -RequestedInstallDir $InstallDir -RequestedReportPath $ObserverReportPath
    return
}

if ([string]::IsNullOrWhiteSpace($CandidateManifestPath)) {
    Invoke-LegacyHostedSmoke -ScriptUrl $InstallScriptUrl -Version $InstallVersion -RequestedInstallDir $InstallDir -RequestedBinaryName $BinaryName
    return
}

$candidateResult = Invoke-CandidateSmoke -RequestedManifestPath $CandidateManifestPath -RequestedInstallDir $InstallDir -RequestedReportPath $ReportPath
$candidateReportPath = if ([string]::IsNullOrWhiteSpace($ReportPath)) {
    Join-Path (Split-Path -Parent (Resolve-SmokeAbsolutePath $CandidateManifestPath)) "install-validation-report.json"
} else {
    Resolve-SmokeAbsolutePath $ReportPath
}
$reportParent = Split-Path -Parent $candidateReportPath
if (-not (Test-Path -LiteralPath $reportParent -PathType Container)) {
    Fail-Smoke "report parent does not exist: $reportParent"
}
[System.IO.File]::WriteAllText($candidateReportPath, ($candidateResult.report | ConvertTo-Json -Depth 12))
if ($null -ne $candidateResult.error) {
    throw $candidateResult.error
}
Write-Output "candidate install smoke passed for $InstallDir; report=$candidateReportPath"
