<#
.SYNOPSIS
Smoke-tests a hosted release or builds and installs one local Windows candidate.

.DESCRIPTION
Candidate mode takes the source commit, repository, cached UI dependency tree,
and native tool hashes as arguments. A new candidate identity does not require
editing this script. The work directory must be short, absent or empty, and
separate from the retained output and clean install directories.
#>
param(
    [string]$InstallScriptUrl,
    [string]$InstallVersion,
    [Parameter(Mandatory = $true)]
    [string]$InstallDir,
    [string]$BinaryName = "you.exe",
    [string]$CandidateSourcePath,
    [string]$CandidateSourceCommit,
    [string]$CandidateSourceRepository,
    [string]$CandidateDependencySourcePath,
    [string]$CandidateOutputDir,
    [string]$CandidateWorkDir,
    [string]$GoReleaserPath,
    [string]$ExpectedGoReleaserVersion,
    [string]$ExpectedEsbuildVersion,
    [string]$ExpectedEsbuildSHA256,
    [int64]$CandidateMaximumWorkBytes = 4294967296,
    [string]$ReportPath
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
$SmokeMaximumTaskRootPathLength = 220

function Fail-Smoke {
    param([string]$Message)
    throw "install smoke: $Message"
}

function Resolve-SmokePath {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) { Fail-Smoke "path is required" }
    if ([System.IO.Path]::IsPathRooted($Path)) { return [System.IO.Path]::GetFullPath($Path) }
    return [System.IO.Path]::GetFullPath((Join-Path (Get-Location).Path $Path))
}

function Assert-SmokeNoReparseAncestry {
    param([string]$Name, [string]$Path)
    $current = Resolve-SmokePath $Path
    while (-not [string]::IsNullOrWhiteSpace($current)) {
        $item = Get-Item -LiteralPath $current -Force -ErrorAction SilentlyContinue
        if ($null -ne $item -and (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
            Fail-Smoke "$Name has a reparse point in its ancestry: $($item.FullName)"
        }
        $parent = [System.IO.Directory]::GetParent($current)
        if ($null -eq $parent -or $parent.FullName -eq $current) { break }
        $current = $parent.FullName
    }
}

function Test-SmokePathContains {
    param([string]$Parent, [string]$Child)
    $parentPath = (Resolve-SmokePath $Parent).TrimEnd('\') + '\'
    $childPath = Resolve-SmokePath $Child
    return $childPath.StartsWith($parentPath, [System.StringComparison]::OrdinalIgnoreCase)
}

function Assert-SmokePathsDisjoint {
    param([string]$FirstName, [string]$FirstPath, [string]$SecondName, [string]$SecondPath)
    $first = Resolve-SmokePath $FirstPath
    $second = Resolve-SmokePath $SecondPath
    $equal = $first.TrimEnd('\').Equals($second.TrimEnd('\'), [System.StringComparison]::OrdinalIgnoreCase)
    if ($equal -or (Test-SmokePathContains $first $second) -or (Test-SmokePathContains $second $first)) {
        Fail-Smoke "$FirstName and $SecondName must not overlap: $first ; $second"
    }
}

function Assert-SmokeTaskRootLength {
    param([string]$Name, [string]$Path)
    $resolved = Resolve-SmokePath $Path
    if ($resolved.Length -gt $SmokeMaximumTaskRootPathLength) {
        Fail-Smoke "$Name must be at most $SmokeMaximumTaskRootPathLength characters: $resolved"
    }
}

function Assert-SmokeCandidateRoots {
    param(
        [string]$SourcePath,
        [string]$DependencySourcePath,
        [string]$OutputDirectory,
        [string]$WorkDirectory,
        [string]$InstallDirectory,
        [string]$ReportPath
    )
    $paths = [ordered]@{
        source = Resolve-SmokePath $SourcePath
        dependency = Resolve-SmokePath $DependencySourcePath
        output = Resolve-SmokePath $OutputDirectory
        work = Resolve-SmokePath $WorkDirectory
        install = Resolve-SmokePath $InstallDirectory
        report = Resolve-SmokePath $ReportPath
    }
    foreach ($entry in $paths.GetEnumerator()) {
        Assert-SmokeNoReparseAncestry $entry.Key $entry.Value
    }
    foreach ($ownedName in @("output", "work", "install", "report")) {
        Assert-SmokeTaskRootLength $ownedName $paths[$ownedName]
    }
    foreach ($ownedName in @("output", "work", "install")) {
        foreach ($readName in @("source", "dependency")) {
            Assert-SmokePathsDisjoint $ownedName $paths[$ownedName] $readName $paths[$readName]
        }
    }
    foreach ($pair in @(@("output", "work"), @("output", "install"), @("work", "install"))) {
        Assert-SmokePathsDisjoint $pair[0] $paths[$pair[0]] $pair[1] $paths[$pair[1]]
    }
    if (-not (Test-SmokePathContains $paths.output $paths.report)) {
        Fail-Smoke "candidate report must be inside the candidate output directory: $($paths.report)"
    }
    return [pscustomobject]$paths
}

function Assert-SmokeEmptyRoot {
    param([string]$Name, [string]$Path)
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -eq $item) { return }
    if (-not $item.PSIsContainer -or (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "$Name must be absent or an empty regular directory: $Path"
    }
    if (@(Get-ChildItem -LiteralPath $Path -Force -ErrorAction Stop).Count -ne 0) { Fail-Smoke "$Name must be empty: $Path" }
}

function Assert-SmokeRegularFile {
    param([string]$Name, [string]$Path)
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -eq $item -or $item.PSIsContainer -or (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "$Name is missing or is not a regular file: $Path"
    }
    return $item
}

function Get-SmokeFileEvidence {
    param([string]$Role, [string]$Path)
    $item = Assert-SmokeRegularFile $Role $Path
    $hasher = [System.Security.Cryptography.SHA256]::Create()
    $stream = [System.IO.File]::OpenRead($item.FullName)
    try {
        $digest = $hasher.ComputeHash($stream)
    } finally {
        $stream.Dispose()
        $hasher.Dispose()
    }
    return [ordered]@{
        role = $Role
        file = $item.Name
        bytes = [int64]$item.Length
        sha256 = [System.BitConverter]::ToString($digest).Replace("-", "").ToLowerInvariant()
    }
}

function Get-SmokeDirectoryBytes {
    param([string]$Path)
    $total = [int64]0
    $pending = New-Object 'System.Collections.Generic.Stack[string]'
    $pending.Push((Resolve-SmokePath $Path))
    while ($pending.Count -gt 0) {
        foreach ($item in @(Get-ChildItem -LiteralPath $pending.Pop() -Force -ErrorAction Stop)) {
            if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { continue }
            if ($item.PSIsContainer) { $pending.Push($item.FullName) } else { $total += [int64]$item.Length }
        }
    }
    return $total
}

function Remove-SmokeOwnedTree {
    param([string]$Name, [string]$Path)
    $resolved = Resolve-SmokePath $Path
    $root = Get-Item -LiteralPath $resolved -Force -ErrorAction SilentlyContinue
    if ($null -eq $root) { return }
    if (-not $root.PSIsContainer -or (($root.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "$Name cleanup root is not a regular directory: $resolved"
    }
    $pending = New-Object 'System.Collections.Generic.Stack[string]'
    $pending.Push($resolved)
    while ($pending.Count -gt 0) {
        foreach ($item in @(Get-ChildItem -LiteralPath $pending.Pop() -Force -ErrorAction Stop)) {
            if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
                Fail-Smoke "$Name cleanup refused unexpected reparse point: $($item.FullName)"
            }
            if ($item.PSIsContainer) { $pending.Push($item.FullName) }
        }
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}

function Remove-SmokeCandidateWork {
    param([string]$WorkDirectory, [string]$DependencyJunctionPath)
    $work = Resolve-SmokePath $WorkDirectory
    if (-not [string]::IsNullOrWhiteSpace($DependencyJunctionPath)) {
        $junction = Resolve-SmokePath $DependencyJunctionPath
        if (-not (Test-SmokePathContains $work $junction)) {
            Fail-Smoke "dependency junction is outside the candidate work directory: $junction"
        }
        $item = Get-Item -LiteralPath $junction -Force -ErrorAction SilentlyContinue
        if ($null -ne $item) {
            if (-not $item.PSIsContainer -or (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -eq 0)) {
                Fail-Smoke "candidate dependency junction is not a directory reparse point: $junction"
            }
            # Directory.Delete removes the junction entry itself and never walks
            # into the shared dependency cache it targets.
            [System.IO.Directory]::Delete($junction)
        }
    }
    Remove-SmokeOwnedTree "candidate work directory" $work
}

function Protect-SmokeOutput {
    param([string]$Path, [int]$MaximumBytes = 1048576)
    if ($MaximumBytes -le 0) { Fail-Smoke "command output limit must be positive" }
    $stream = [System.IO.File]::Open($Path, [System.IO.FileMode]::OpenOrCreate, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
    try {
        $totalBytes = [int64]$stream.Length
        $count = [int][Math]::Min($totalBytes, [int64]$MaximumBytes)
        $bytes = New-Object byte[] $count
        $offset = 0
        while ($offset -lt $count) {
            $read = $stream.Read($bytes, $offset, $count - $offset)
            if ($read -eq 0) { break }
            $offset += $read
        }
        if ($offset -ne $count) { $count = $offset }
    } finally {
        $stream.Dispose()
    }
    if ($count -ge 2 -and $bytes[0] -eq 0xff -and $bytes[1] -eq 0xfe) {
        $text = [System.Text.Encoding]::Unicode.GetString($bytes, 2, $count - 2)
    } elseif ($count -ge 3 -and $bytes[0] -eq 0xef -and $bytes[1] -eq 0xbb -and $bytes[2] -eq 0xbf) {
        $text = [System.Text.Encoding]::UTF8.GetString($bytes, 3, $count - 3)
    } else {
        $text = [System.Text.Encoding]::UTF8.GetString($bytes, 0, $count)
    }
    $pattern = '(?i)(token|password|secret|api[_-]?key|authorization)\s*([:=])\s*[^\s,;]+'
    $text = [System.Text.RegularExpressions.Regex]::Replace($text, $pattern, '$1$2<redacted>')
    $encoding = [System.Text.UTF8Encoding]::new($false)
    $redactedBytes = $encoding.GetBytes($text)
    if ($redactedBytes.Length -gt $MaximumBytes) {
        $text = $encoding.GetString($redactedBytes, 0, $MaximumBytes)
        while ($encoding.GetByteCount($text) -gt $MaximumBytes) {
            $text = $text.Substring(0, $text.Length - 1)
        }
    }
    [System.IO.File]::WriteAllText($Path, $text, $encoding)
    $fileEvidence = Get-SmokeFileEvidence "command-output" $Path
    $evidence = [ordered]@{
        role = $fileEvidence.role
        file = $fileEvidence.file
        bytes = $fileEvidence.bytes
        sha256 = $fileEvidence.sha256
        totalBytes = $totalBytes
        truncated = $totalBytes -gt $MaximumBytes
    }
    return [pscustomobject]@{ text = $text; evidence = $evidence }
}

function Invoke-CandidateCommand {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$ArgumentList,
        [Parameter(Mandatory = $true)][string]$WorkingDirectory,
        [Parameter(Mandatory = $true)][string]$StdoutPath,
        [Parameter(Mandatory = $true)][string]$StderrPath,
        [int]$TimeoutSeconds = 7200,
        [int]$OutputMaximumBytes = 1048576
    )
    if ($TimeoutSeconds -le 0) { Fail-Smoke "command timeout must be positive" }
    if ($OutputMaximumBytes -le 0) { Fail-Smoke "command output limit must be positive" }
    $workingDirectory = Resolve-SmokePath $WorkingDirectory
    $stdoutPath = Resolve-SmokePath $StdoutPath
    $stderrPath = Resolve-SmokePath $StderrPath
    foreach ($parent in @((Split-Path -Parent $stdoutPath), (Split-Path -Parent $stderrPath))) {
        [void][System.IO.Directory]::CreateDirectory($parent)
    }
    [System.IO.File]::WriteAllBytes($stdoutPath, [byte[]]@())
    [System.IO.File]::WriteAllBytes($stderrPath, [byte[]]@())
    $pidPath = Join-Path ([System.IO.Path]::GetDirectoryName($stdoutPath)) ("command-" + [System.Guid]::NewGuid().ToString("N") + ".pid")
    $payload = [pscustomobject]@{
        FilePath = $FilePath
        Arguments = @($ArgumentList)
        WorkingDirectory = $workingDirectory
        StdoutPath = $stdoutPath
        StderrPath = $stderrPath
        PidPath = $pidPath
    }
    $job = $null
    $exitCode = 125
    $timedOut = $false
    $outputLimitExceeded = $false
    $terminationReason = ""
    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $job = Start-Job -ScriptBlock {
            param($Command)
            $ErrorActionPreference = "Continue"
            Set-Location -LiteralPath $Command.WorkingDirectory
            [System.IO.File]::WriteAllText($Command.PidPath, [string]$PID, [System.Text.Encoding]::ASCII)
            $arguments = @($Command.Arguments)
            $commandPath = [string]$Command.FilePath
            $commandStdout = [string]$Command.StdoutPath
            $commandStderr = [string]$Command.StderrPath
            $commandExitCode = 125
            & $commandPath @arguments 1> $commandStdout 2> $commandStderr
            if ($null -ne $LASTEXITCODE) { $commandExitCode = [int]$LASTEXITCODE }
            return $commandExitCode
        } -ArgumentList $payload
        $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
        while ($job.State -notin @("Completed", "Failed", "Stopped")) {
            Wait-Job -Job $job -Timeout 1 | Out-Null
            foreach ($path in @($stdoutPath, $stderrPath)) {
                $item = Get-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
                if ($null -ne $item -and [int64]$item.Length -gt $OutputMaximumBytes) {
                    $outputLimitExceeded = $true
                    $terminationReason = "output exceeded $OutputMaximumBytes bytes"
                    break
                }
            }
            if ($outputLimitExceeded) { break }
            if ([DateTime]::UtcNow -ge $deadline) {
                $timedOut = $true
                $terminationReason = "deadline of $TimeoutSeconds seconds exceeded"
                break
            }
        }
        foreach ($path in @($stdoutPath, $stderrPath)) {
            $item = Get-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
            if ($null -ne $item -and [int64]$item.Length -gt $OutputMaximumBytes) {
                $outputLimitExceeded = $true
                $terminationReason = "output exceeded $OutputMaximumBytes bytes"
                break
            }
        }
        if ($timedOut -or $outputLimitExceeded) {
            $jobProcessID = 0
            if (Test-Path -LiteralPath $pidPath -PathType Leaf) {
                [void][int]::TryParse(([System.IO.File]::ReadAllText($pidPath).Trim()), [ref]$jobProcessID)
            }
            if ($jobProcessID -gt 0 -and $job.State -notin @("Completed", "Failed", "Stopped")) {
                $taskkill = Join-Path $env:SystemRoot "System32\taskkill.exe"
                $previousErrorActionPreference = $ErrorActionPreference
                try {
                    $ErrorActionPreference = "Continue"
                    & $taskkill /PID $jobProcessID /T /F 1>$null 2>$null
                } finally {
                    $ErrorActionPreference = $previousErrorActionPreference
                }
            }
            Wait-Job -Job $job -Timeout 10 | Out-Null
            if ($job.State -notin @("Completed", "Failed", "Stopped")) {
                Stop-Job -Job $job -ErrorAction SilentlyContinue
            }
            $exitCode = if ($timedOut) { 124 } else { 125 }
        } elseif ($job.State -eq "Completed") {
            $jobOutput = @(Receive-Job -Job $job -ErrorAction SilentlyContinue)
            if ($jobOutput.Count -gt 0) { $exitCode = [int]$jobOutput[-1] }
        } else {
            $terminationReason = "command job ended in state $($job.State)"
        }
    } finally {
        if ($null -ne $job) {
            if ($job.State -notin @("Completed", "Failed", "Stopped")) { Stop-Job -Job $job -ErrorAction SilentlyContinue }
            Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
        }
        Remove-Item -LiteralPath $pidPath -Force -ErrorAction SilentlyContinue
    }
    $stopwatch.Stop()
    $stdout = Protect-SmokeOutput $stdoutPath $OutputMaximumBytes
    $stderr = Protect-SmokeOutput $stderrPath $OutputMaximumBytes
    return [pscustomobject][ordered]@{
        file = $FilePath
        arguments = @($ArgumentList)
        exitCode = $exitCode
        timedOut = $timedOut
        outputLimitExceeded = $outputLimitExceeded
        terminationReason = $terminationReason
        elapsedMilliseconds = [int64][Math]::Max(0, $stopwatch.ElapsedMilliseconds)
        stdout = $stdout.text
        stderr = $stderr.text
        stdoutEvidence = $stdout.evidence
        stderrEvidence = $stderr.evidence
    }
}

function Get-SmokeExecutableBuildInfo {
    param(
        [string]$ExecutablePath,
        [string]$ExpectedRevision,
        [string]$GoExecutablePath,
        [string]$WorkingDirectory,
        [string]$OutputDirectory
    )
    $goPath = $GoExecutablePath
    if ([string]::IsNullOrWhiteSpace($goPath)) {
        $go = Get-Command go.exe -CommandType Application -ErrorAction SilentlyContinue
        if ($null -eq $go) { Fail-Smoke "Go is required to inspect executable build info" }
        $goPath = $go.Source
    }
    $result = Invoke-CandidateCommand -FilePath $goPath `
        -ArgumentList @("version", "-m", (Resolve-SmokePath $ExecutablePath)) `
        -WorkingDirectory $WorkingDirectory `
        -StdoutPath (Join-Path $OutputDirectory "executable-build-info.stdout.log") `
        -StderrPath (Join-Path $OutputDirectory "executable-build-info.stderr.log")
    if ($result.exitCode -ne 0) {
        Fail-Smoke "go version -m failed with exit $($result.exitCode): $($result.stderr)"
    }
    $settings = @{}
    foreach ($line in ($result.stdout -split "`r?`n")) {
        if ($line -match '^\s*build\s+(vcs\.(revision|modified))=(\S+)\s*$') {
            $key = $Matches[1]
            if ($settings.ContainsKey($key)) { Fail-Smoke "executable build info contains duplicate $key" }
            $settings[$key] = $Matches[3]
        }
    }
    foreach ($key in @("vcs.revision", "vcs.modified")) {
        if (-not $settings.ContainsKey($key)) { Fail-Smoke "executable build info $key is missing" }
    }
    if ([string]$settings["vcs.revision"] -cne $ExpectedRevision) {
        Fail-Smoke "executable build info vcs.revision = $($settings['vcs.revision']), want $ExpectedRevision"
    }
    if ([string]$settings["vcs.modified"] -cne "false") {
        Fail-Smoke "executable build info vcs.modified = $($settings['vcs.modified']), want false"
    }
    return [ordered]@{
        command = $result
        sourceRevision = [string]$settings["vcs.revision"]
        vcsModified = $false
    }
}

function Invoke-SmokeGit {
    param([string[]]$ArgumentList)
    $output = @(& git.exe @ArgumentList 2>&1 | ForEach-Object { [string]$_ })
    if ($LASTEXITCODE -ne 0) { Fail-Smoke "git $($ArgumentList -join ' ') failed: $($output -join ' ')" }
    return ($output -join "`n").Trim()
}

function Normalize-CandidateRepository {
    param([string]$Repository)
    return $Repository.Trim().TrimEnd('/').Replace('\', '/').ToLowerInvariant() -replace '\.git$', ''
}

function Get-CandidateSourceIdentity {
    param([string]$SourcePath, [string]$Commit, [string]$Repository)
    $sourcePath = Resolve-SmokePath $SourcePath
    if (-not (Test-Path -LiteralPath $sourcePath -PathType Container)) { Fail-Smoke "candidate source repository is missing: $sourcePath" }
    if ($Commit -notmatch '^[0-9a-fA-F]{40}$') { Fail-Smoke "candidate source commit must be a full Git object ID" }
    $resolvedCommit = Invoke-SmokeGit @("-C", $sourcePath, "rev-parse", "$Commit^{commit}")
    if ($resolvedCommit -cne $Commit.ToLowerInvariant()) { Fail-Smoke "candidate source resolved to $resolvedCommit, want $Commit" }
    $tree = Invoke-SmokeGit @("-C", $sourcePath, "rev-parse", "$resolvedCommit^{tree}")
    $origin = Invoke-SmokeGit @("-C", $sourcePath, "remote", "get-url", "origin")
    if (-not [string]::IsNullOrWhiteSpace($Repository) -and (Normalize-CandidateRepository $origin) -cne (Normalize-CandidateRepository $Repository)) {
        Fail-Smoke "candidate source origin is $origin, want $Repository"
    }
    return [pscustomobject][ordered]@{ repository = $origin; commit = $resolvedCommit; tree = $tree }
}

function Get-SmokeAvailablePort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try { $listener.Start(); return [int]$listener.LocalEndpoint.Port } finally { $listener.Stop() }
}

function Start-CandidateFileServer {
    param([string]$InstallerPath, [string]$ArchivePath, [string]$ChecksumPath, [string]$Version, [string]$ReadyPath)
    $port = Get-SmokeAvailablePort
    $job = Start-Job -ScriptBlock {
        param($Port, $InstallerPath, $ArchivePath, $ChecksumPath, $Version, $ReadyPath)
        $ErrorActionPreference = "Stop"
        $listener = [System.Net.HttpListener]::new()
        $listener.Prefixes.Add("http://127.0.0.1:$Port/")
        try {
            $listener.Start()
            [System.IO.File]::WriteAllText($ReadyPath, "ready")
            $routes = @{
                "/download/v$Version/install.ps1" = $InstallerPath
                "/releases/download/v$Version/$([System.IO.Path]::GetFileName($ArchivePath))" = $ArchivePath
                "/releases/download/v$Version/$([System.IO.Path]::GetFileName($ChecksumPath))" = $ChecksumPath
            }
            while ($true) {
                $context = $listener.GetContext()
                $requestPath = [System.Uri]::UnescapeDataString($context.Request.Url.AbsolutePath)
                if ($requestPath -eq "/stop") { $context.Response.StatusCode = 204; $context.Response.Close(); break }
                if (-not $routes.ContainsKey($requestPath)) { $context.Response.StatusCode = 404; $context.Response.Close(); continue }
                $bytes = [System.IO.File]::ReadAllBytes([string]$routes[$requestPath])
                $context.Response.StatusCode = 200
                $context.Response.ContentLength64 = $bytes.Length
                $context.Response.OutputStream.Write($bytes, 0, $bytes.Length)
                $context.Response.Close()
            }
        } finally {
            if ($listener.IsListening) { $listener.Stop() }
            $listener.Close()
        }
    } -ArgumentList $port, $InstallerPath, $ArchivePath, $ChecksumPath, $Version, $ReadyPath
    $deadline = [DateTime]::UtcNow.AddSeconds(10)
    while (-not (Test-Path -LiteralPath $ReadyPath -PathType Leaf) -and [DateTime]::UtcNow -lt $deadline) {
        if ($job.State -in @("Failed", "Completed", "Stopped")) {
            $details = Receive-Job -Job $job -Keep -ErrorAction SilentlyContinue | Out-String
            Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
            Fail-Smoke "candidate file server stopped before readiness: $details"
        }
        Start-Sleep -Milliseconds 25
    }
    if (-not (Test-Path -LiteralPath $ReadyPath -PathType Leaf)) {
        Stop-Job -Job $job -ErrorAction SilentlyContinue
        Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
        Fail-Smoke "candidate file server did not become ready"
    }
    return [pscustomobject]@{ job = $job; port = $port; baseUrl = "http://127.0.0.1:$port" }
}

function Stop-CandidateFileServer {
    param([object]$Server)
    if ($null -eq $Server) { return }
    try {
        Invoke-WebRequest -Uri "$($Server.baseUrl)/stop" -UseBasicParsing | Out-Null
        Wait-Job -Job $Server.job -Timeout 10 | Out-Null
    } finally {
        if ($Server.job.State -notin @("Completed", "Failed", "Stopped")) { Stop-Job -Job $Server.job -ErrorAction SilentlyContinue }
        Remove-Job -Job $Server.job -Force -ErrorAction SilentlyContinue
    }
}

function Invoke-InstalledCandidateSmoke {
    param(
        [string]$CandidateDirectory,
        [string]$ArchivePath,
        [string]$ChecksumPath,
        [string]$InstallerPath,
        [string]$ArchiveExecutablePath,
        [string]$Version,
        [string]$ExpectedSourceCommit,
        [string]$GoExecutablePath,
        [string]$RequestedInstallDir,
        [string]$WorkDirectory
    )
    $installDir = Resolve-SmokePath $RequestedInstallDir
    Assert-SmokeTaskRootLength "install directory" $installDir
    Assert-SmokeTaskRootLength "candidate output directory" $CandidateDirectory
    Assert-SmokeTaskRootLength "candidate work directory" $WorkDirectory
    Assert-SmokeEmptyRoot "install directory" $installDir
    $smokeRoot = Join-Path $WorkDirectory "install-smoke"
    [void][System.IO.Directory]::CreateDirectory($smokeRoot)
    $homeRoot = Join-Path $smokeRoot "home"
    $profileRoot = Join-Path $smokeRoot "profile"
    $tempRoot = Join-Path $smokeRoot "temp"
    $pathRoot = Join-Path $smokeRoot "path"
    $modelsRoot = Join-Path $smokeRoot "models"
    $hfRoot = Join-Path $smokeRoot "hf"
    foreach ($path in @($homeRoot, $profileRoot, $tempRoot, $pathRoot)) {
        [void][System.IO.Directory]::CreateDirectory($path)
    }
    $readyPath = Join-Path $smokeRoot "server.ready"
    $server = $null
    $environmentNames = @(
        "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA",
        "TEMP", "TMP", "PATH", "XDG_CONFIG_HOME", "XDG_CACHE_HOME",
        "INFINITE_YOU_VERSION", "INFINITE_YOU_INSTALL_DIR", "INFINITE_YOU_INSTALL_OS",
        "INFINITE_YOU_INSTALL_ARCH", "INFINITE_YOU_INSTALL_BASE_URL",
        "INFINITE_YOU_OMNIVOICE_CACHE_DIR", "HUGGINGFACE_HUB_CACHE", "HF_HOME"
    )
    $originalEnvironment = @{}
    foreach ($name in $environmentNames) {
        $originalEnvironment[$name] = [System.Environment]::GetEnvironmentVariable($name, "Process")
    }
    $shell = (Get-Command powershell.exe -ErrorAction Stop).Source
    $commands = New-Object 'System.Collections.Generic.List[object]'
    try {
        $serverArguments = @{
            InstallerPath = $InstallerPath
            ArchivePath = $ArchivePath
            ChecksumPath = $ChecksumPath
            Version = $Version
            ReadyPath = $readyPath
        }
        $server = Start-CandidateFileServer @serverArguments
        $env:HOME = $homeRoot
        $env:USERPROFILE = $profileRoot
        $profileDrive = [System.IO.Path]::GetPathRoot($profileRoot)
        $env:HOMEDRIVE = $profileDrive.TrimEnd('\')
        $env:HOMEPATH = $profileRoot.Substring($profileDrive.Length - 1)
        $env:APPDATA = Join-Path $profileRoot "AppData\Roaming"
        $env:LOCALAPPDATA = Join-Path $profileRoot "AppData\Local"
        $env:TEMP = $tempRoot
        $env:TMP = $tempRoot
        $env:PATH = $pathRoot
        $env:XDG_CONFIG_HOME = Join-Path $profileRoot ".you-agent-factory"
        $env:XDG_CACHE_HOME = $env:LOCALAPPDATA
        $env:INFINITE_YOU_VERSION = $Version
        $env:INFINITE_YOU_INSTALL_DIR = $installDir
        $env:INFINITE_YOU_INSTALL_OS = "windows"
        $env:INFINITE_YOU_INSTALL_ARCH = "amd64"
        $env:INFINITE_YOU_INSTALL_BASE_URL = "$($server.baseUrl)/releases"
        $env:INFINITE_YOU_OMNIVOICE_CACHE_DIR = $modelsRoot
        $env:HUGGINGFACE_HUB_CACHE = $hfRoot
        $env:HF_HOME = $hfRoot
        $downloadedInstaller = Join-Path $smokeRoot "install.ps1"
        Invoke-WebRequest -Uri "$($server.baseUrl)/download/v$Version/install.ps1" -OutFile $downloadedInstaller -UseBasicParsing
        $installerLiteral = "'" + $downloadedInstaller.Replace("'", "''") + "'"
        $installerWrapper = Join-Path $smokeRoot "run-install.ps1"
        $wrapperSource = @(
            "Import-Module Microsoft.PowerShell.Utility -ErrorAction Stop"
            "Import-Module Microsoft.PowerShell.Archive -ErrorAction Stop"
            "& $installerLiteral"
        ) -join "`n"
        [System.IO.File]::WriteAllText($installerWrapper, $wrapperSource, [System.Text.UTF8Encoding]::new($false))
        $installArguments = @{
            FilePath = $shell
            ArgumentList = @("-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", $installerWrapper)
            WorkingDirectory = $smokeRoot
            StdoutPath = Join-Path $CandidateDirectory "install.stdout.log"
            StderrPath = Join-Path $CandidateDirectory "install.stderr.log"
        }
        $install = Invoke-CandidateCommand @installArguments
        [void]$commands.Add($install)
        if ($install.exitCode -ne 0) {
            Fail-Smoke "public installer exited $($install.exitCode): $($install.stderr)"
        }
        $installedBinary = Join-Path $installDir "you.exe"
        $installedEvidence = Get-SmokeFileEvidence "installed-executable" $installedBinary
        $archiveEvidence = Get-SmokeFileEvidence "archive-executable" $ArchiveExecutablePath
        if ($installedEvidence.bytes -ne $archiveEvidence.bytes -or $installedEvidence.sha256 -ne $archiveEvidence.sha256) {
            Fail-Smoke "installed executable does not match the archive member"
        }
        $env:PATH = "$installDir$([System.IO.Path]::PathSeparator)$pathRoot"
        $resolvedCommand = Get-Command you.exe -CommandType Application -ErrorAction SilentlyContinue
        if ($null -eq $resolvedCommand) { Fail-Smoke "you.exe was not resolvable from PATH after installation" }
        $resolvedPath = Resolve-SmokePath $resolvedCommand.Source
        if (-not $resolvedPath.Equals((Resolve-SmokePath $installedBinary), [System.StringComparison]::OrdinalIgnoreCase)) {
            Fail-Smoke "PATH resolved $resolvedPath instead of the installed executable $installedBinary"
        }
        $pathResolution = $resolvedPath
        $buildInfo = $null
        if (-not [string]::IsNullOrWhiteSpace($ExpectedSourceCommit)) {
            $buildInfo = Get-SmokeExecutableBuildInfo -ExecutablePath $resolvedPath `
                -ExpectedRevision $ExpectedSourceCommit -GoExecutablePath $GoExecutablePath `
                -WorkingDirectory $smokeRoot `
                -OutputDirectory $CandidateDirectory
            [void]$commands.Add($buildInfo.command)
        }
        $versionCommand = Invoke-CandidateCommand -FilePath $resolvedPath `
            -ArgumentList @("--version") -WorkingDirectory $smokeRoot `
            -StdoutPath (Join-Path $smokeRoot "version.stdout") `
            -StderrPath (Join-Path $smokeRoot "version.stderr")
        [void]$commands.Add($versionCommand)
        if ($versionCommand.exitCode -ne 0 -or $versionCommand.stdout.Trim() -cne $Version) {
            Fail-Smoke "installed candidate version is '$($versionCommand.stdout.Trim())', want '$Version'"
        }
        $rootHelpCommand = Invoke-CandidateCommand -FilePath $resolvedPath `
            -ArgumentList @("--help") -WorkingDirectory $smokeRoot `
            -StdoutPath (Join-Path $smokeRoot "root-help.stdout") `
            -StderrPath (Join-Path $smokeRoot "root-help.stderr")
        [void]$commands.Add($rootHelpCommand)
        if ($rootHelpCommand.exitCode -ne 0 -or [string]::IsNullOrWhiteSpace($rootHelpCommand.stdout)) {
            Fail-Smoke "installed candidate root help failed"
        }
        $docsCommand = Invoke-CandidateCommand -FilePath $resolvedPath `
            -ArgumentList @("docs", "models") -WorkingDirectory $smokeRoot `
            -StdoutPath (Join-Path $smokeRoot "models-docs.stdout") `
            -StderrPath (Join-Path $smokeRoot "models-docs.stderr")
        [void]$commands.Add($docsCommand)
        if ($docsCommand.exitCode -ne 0 -or [string]::IsNullOrWhiteSpace($docsCommand.stdout)) {
            Fail-Smoke "installed candidate models docs failed"
        }
        $listCommand = Invoke-CandidateCommand -FilePath $resolvedPath `
            -ArgumentList @("models", "list") -WorkingDirectory $smokeRoot `
            -StdoutPath (Join-Path $smokeRoot "models-list.stdout") `
            -StderrPath (Join-Path $smokeRoot "models-list.stderr")
        [void]$commands.Add($listCommand)
        if ($listCommand.exitCode -ne 0) {
            Fail-Smoke "installed candidate models list exited $($listCommand.exitCode)"
        }
        $helpCommand = Invoke-CandidateCommand -FilePath $resolvedPath `
            -ArgumentList @("models", "--help") -WorkingDirectory $smokeRoot `
            -StdoutPath (Join-Path $smokeRoot "models-help.stdout") `
            -StderrPath (Join-Path $smokeRoot "models-help.stderr")
        [void]$commands.Add($helpCommand)
        if ($helpCommand.exitCode -ne 0) {
            Fail-Smoke "installed candidate models help exited $($helpCommand.exitCode)"
        }
        foreach ($name in @("llm", "asr", "tts", "embed")) {
            if (-not $helpCommand.stdout.Contains($name)) {
                Fail-Smoke "installed candidate models help does not expose $name"
            }
            $inspect = Invoke-CandidateCommand -FilePath $resolvedPath `
                -ArgumentList @("--json", "models", "inspect", $name) `
                -WorkingDirectory $smokeRoot `
                -StdoutPath (Join-Path $smokeRoot "$name.stdout") `
                -StderrPath (Join-Path $smokeRoot "$name.stderr")
            [void]$commands.Add($inspect)
            if ($inspect.exitCode -ne 0) {
                Fail-Smoke "installed candidate models inspect $name exited $($inspect.exitCode)"
            }
            $model = $inspect.stdout | ConvertFrom-Json
            $unexpectedState = $model.name -ne $name -or
                $model.managedRuntime.identity -ne $name -or
                $model.managedRuntime.readinessState -ne "MISSING" -or
                $model.managedRuntime.lifecycleState -ne "NOT_INSTALLED" -or
                $model.loadState -ne "UNLOADED"
            if ($unexpectedState) {
                Fail-Smoke "installed candidate models inspect $name performed work or returned an unexpected identity/state"
            }
        }
        foreach ($path in @($modelsRoot, $hfRoot)) {
            if ((Test-Path -LiteralPath $path) -and @(Get-ChildItem -LiteralPath $path -Force -ErrorAction Stop).Count -ne 0) {
                Fail-Smoke "discovery wrote model or backend content under $path"
            }
        }
        return [pscustomobject][ordered]@{
            status = "PASS"
            installedExecutable = $installedEvidence
            pathResolution = $pathResolution
            executableBuildInfo = $buildInfo
            commands = @($commands | ForEach-Object { $_ })
            modelCalls = 0
            modelBackendDownloadBytes = 0
        }
    } finally {
        Stop-CandidateFileServer $server
        foreach ($name in $environmentNames) {
            [System.Environment]::SetEnvironmentVariable($name, $originalEnvironment[$name], "Process")
        }
        Remove-SmokeOwnedTree "install directory" $installDir
    }
}

function Invoke-LocalCandidateSmoke {
    param(
        [string]$SourcePath,
        [string]$SourceCommit,
        [string]$SourceRepository,
        [string]$DependencySourcePath,
        [string]$OutputDirectory,
        [string]$WorkDirectory,
        [string]$ReleaseToolPath,
        [string]$ReleaseToolVersion,
        [string]$EsbuildVersion,
        [string]$EsbuildSHA256,
        [int64]$MaximumWorkBytes,
        [string]$RequestedInstallDir,
        [string]$RequestedReportPath
    )
    $requiredValues = [ordered]@{
        CandidateSourcePath = $SourcePath
        CandidateSourceCommit = $SourceCommit
        CandidateSourceRepository = $SourceRepository
        CandidateDependencySourcePath = $DependencySourcePath
        CandidateOutputDir = $OutputDirectory
        CandidateWorkDir = $WorkDirectory
        GoReleaserPath = $ReleaseToolPath
        ExpectedGoReleaserVersion = $ReleaseToolVersion
        ExpectedEsbuildVersion = $EsbuildVersion
        ExpectedEsbuildSHA256 = $EsbuildSHA256
        ReportPath = $RequestedReportPath
    }
    foreach ($required in $requiredValues.GetEnumerator()) {
        if ([string]::IsNullOrWhiteSpace([string]$required.Value)) {
            Fail-Smoke "$($required.Key) is required in candidate mode"
        }
    }
    if ($EsbuildSHA256 -notmatch '^[0-9a-fA-F]{64}$') {
        Fail-Smoke "ExpectedEsbuildSHA256 must be a SHA-256 digest"
    }
    $roots = Assert-SmokeCandidateRoots -SourcePath $SourcePath `
        -DependencySourcePath $DependencySourcePath -OutputDirectory $OutputDirectory `
        -WorkDirectory $WorkDirectory -InstallDirectory $RequestedInstallDir `
        -ReportPath $RequestedReportPath
    $sourcePath = $roots.source
    $dependencySourcePath = $roots.dependency
    $outputDirectory = $roots.output
    $workDirectory = $roots.work
    $installDirectory = $roots.install
    $reportPath = $roots.report
    $releaseToolPath = Resolve-SmokePath $ReleaseToolPath
    Assert-SmokeEmptyRoot "candidate output directory" $outputDirectory
    Assert-SmokeEmptyRoot "candidate work directory" $workDirectory
    Assert-SmokeEmptyRoot "install directory" $installDirectory
    [void](Assert-SmokeRegularFile "GoReleaser" $releaseToolPath)
    if ($workDirectory.Length -gt 80) {
        Fail-Smoke "candidate work directory must be at most 80 characters so Windows native tools remain below path limits"
    }
    $sourceIdentity = Get-CandidateSourceIdentity -SourcePath $sourcePath `
        -Commit $SourceCommit -Repository $SourceRepository
    [void][System.IO.Directory]::CreateDirectory($outputDirectory)
    [void][System.IO.Directory]::CreateDirectory($workDirectory)
    $checkoutPath = Join-Path $workDirectory "src"
    $extractPath = Join-Path $workDirectory "archive"
    $environmentNames = @("GOFLAGS", "GOMAXPROCS", "GOPROXY", "GOSUMDB", "GOTOOLCHAIN", "npm_config_offline")
    $originalEnvironment = @{}
    foreach ($name in $environmentNames) {
        $originalEnvironment[$name] = [System.Environment]::GetEnvironmentVariable($name, "Process")
    }
    $report = [ordered]@{
        schemaVersion = "local-windows-candidate/v1"
        status = "FAIL"
        source = [ordered]@{
            repository = $SourceRepository
            commit = $sourceIdentity.commit
            tree = $sourceIdentity.tree
        }
        build = [ordered]@{}
        artifacts = @()
        install = [ordered]@{ status = "NOT_RUN" }
        cleanup = [ordered]@{ status = "NOT_RUN" }
        error = ""
    }
    $failure = $null
    $dependencyJunctionPath = $null
    try {
        $env:GOFLAGS = "-p=4"
        $env:GOMAXPROCS = "4"
        $env:GOPROXY = "off"
        $env:GOSUMDB = "off"
        $env:GOTOOLCHAIN = "local"
        $env:npm_config_offline = "true"
        [void](Invoke-SmokeGit @("clone", "--local", "--no-checkout", "--", $sourcePath, $checkoutPath))
        [void](Invoke-SmokeGit @("-C", $checkoutPath, "checkout", "--detach", $sourceIdentity.commit))
        $gitDirectory = Get-Item -LiteralPath (Join-Path $checkoutPath ".git") -Force -ErrorAction Stop
        if (-not $gitDirectory.PSIsContainer) {
            Fail-Smoke "candidate clone must have a real .git directory"
        }
        $checkoutCommit = Invoke-SmokeGit @("-C", $checkoutPath, "rev-parse", "HEAD")
        $checkoutTree = Invoke-SmokeGit @("-C", $checkoutPath, "rev-parse", "HEAD^{tree}")
        if ($checkoutCommit -cne $sourceIdentity.commit -or $checkoutTree -cne $sourceIdentity.tree) {
            Fail-Smoke "candidate clone source identity changed"
        }
        [System.IO.File]::AppendAllText((Join-Path $checkoutPath ".git\info\exclude"), "`n/dist/`n", [System.Text.UTF8Encoding]::new($false))
        $sourceLock = Get-SmokeFileEvidence "source-bun-lock" (Join-Path $checkoutPath "ui\bun.lock")
        $dependencyLock = Get-SmokeFileEvidence "dependency-bun-lock" (Join-Path $dependencySourcePath "ui\bun.lock")
        if ($sourceLock.sha256 -ne $dependencyLock.sha256) {
            Fail-Smoke "candidate and dependency source bun.lock hashes differ"
        }
        $dependencyNodeModules = Join-Path $dependencySourcePath "ui\node_modules"
        if (-not (Test-Path -LiteralPath $dependencyNodeModules -PathType Container)) {
            Fail-Smoke "verified dependency source ui/node_modules is missing"
        }
        $checkoutNodeModules = Join-Path $checkoutPath "ui\node_modules"
        if (Test-Path -LiteralPath $checkoutNodeModules) {
            Fail-Smoke "fresh candidate clone unexpectedly contains ui/node_modules"
        }
        [void](New-Item -ItemType Junction -Path $checkoutNodeModules -Target $dependencyNodeModules)
        $dependencyJunctionPath = $checkoutNodeModules
        $esbuildPath = Join-Path $checkoutNodeModules "esbuild\lib\downloaded-@esbuild-win32-x64-esbuild.exe"
        $esbuildEvidence = Get-SmokeFileEvidence "esbuild" $esbuildPath
        if ($esbuildEvidence.sha256 -cne $EsbuildSHA256.ToLowerInvariant()) {
            Fail-Smoke "esbuild SHA-256 is $($esbuildEvidence.sha256), want $($EsbuildSHA256.ToLowerInvariant())"
        }
        if ($esbuildPath.Length -gt 220) {
            Fail-Smoke "staged esbuild path is $($esbuildPath.Length) characters, want at most 220"
        }
        $esbuildResult = Invoke-CandidateCommand -FilePath $esbuildPath `
            -ArgumentList @("--version") -WorkingDirectory $checkoutPath `
            -StdoutPath (Join-Path $outputDirectory "esbuild.stdout.log") `
            -StderrPath (Join-Path $outputDirectory "esbuild.stderr.log")
        if ($esbuildResult.exitCode -ne 0 -or $esbuildResult.stdout.Trim() -cne $EsbuildVersion) {
            Fail-Smoke "esbuild sanity check failed: exit=$($esbuildResult.exitCode) version='$($esbuildResult.stdout.Trim())'"
        }
        $releaseToolEvidence = Get-SmokeFileEvidence "goreleaser" $releaseToolPath
        $releaseToolResult = Invoke-CandidateCommand -FilePath $releaseToolPath `
            -ArgumentList @("--version") -WorkingDirectory $checkoutPath `
            -StdoutPath (Join-Path $outputDirectory "goreleaser-version.stdout.log") `
            -StderrPath (Join-Path $outputDirectory "goreleaser-version.stderr.log")
        if ($releaseToolResult.exitCode -ne 0 -or $releaseToolResult.stdout -notmatch ("(?m)\b" + [regex]::Escape($ReleaseToolVersion) + "\b")) {
            Fail-Smoke "GoReleaser version check failed: exit=$($releaseToolResult.exitCode) output='$($releaseToolResult.stdout.Trim())'"
        }
        $goCommand = Get-Command go.exe -CommandType Application -ErrorAction SilentlyContinue
        if ($null -eq $goCommand) { Fail-Smoke "Go 1.26.8 is required for candidate release" }
        $goVersionResult = Invoke-CandidateCommand -FilePath $goCommand.Source `
            -ArgumentList @("version") -WorkingDirectory $checkoutPath `
            -StdoutPath (Join-Path $outputDirectory "go-version.stdout.log") `
            -StderrPath (Join-Path $outputDirectory "go-version.stderr.log")
        if ($goVersionResult.exitCode -ne 0 -or $goVersionResult.stdout -notmatch '(?m)\bgo1\.26\.8\b') {
            Fail-Smoke "Go version check failed: exit=$($goVersionResult.exitCode) output='$($goVersionResult.stdout.Trim())', want go1.26.8"
        }
        $statusBefore = Invoke-SmokeGit @("-C", $checkoutPath, "status", "--porcelain=v1", "--untracked-files=all")
        if ($statusBefore -ne "") {
            Fail-Smoke "candidate clone is dirty before release"
        }
        $releaseArguments = @("release", "--snapshot", "--clean", "-f", ".goreleaser.yml")
        $releaseResult = Invoke-CandidateCommand -FilePath $releaseToolPath `
            -ArgumentList $releaseArguments -WorkingDirectory $checkoutPath `
            -StdoutPath (Join-Path $outputDirectory "release.stdout.log") `
            -StderrPath (Join-Path $outputDirectory "release.stderr.log")
        $workBytes = Get-SmokeDirectoryBytes $workDirectory
        $report.build = [ordered]@{
            command = $releaseToolPath
            arguments = $releaseArguments
            exitCode = $releaseResult.exitCode
            elapsedMilliseconds = $releaseResult.elapsedMilliseconds
            timedOut = $releaseResult.timedOut
            outputLimitExceeded = $releaseResult.outputLimitExceeded
            terminationReason = $releaseResult.terminationReason
            stdout = $releaseResult.stdoutEvidence
            stderr = $releaseResult.stderrEvidence
            goReleaser = $releaseToolEvidence
            goReleaserVersion = $ReleaseToolVersion
            goVersion = $goVersionResult
            config = Get-SmokeFileEvidence "goreleaser-config" (Join-Path $checkoutPath ".goreleaser.yml")
            bunLock = $sourceLock
            esbuild = $esbuildEvidence
            esbuildVersion = $EsbuildVersion
            esbuildPathLength = $esbuildPath.Length
            workBytes = $workBytes
            maximumWorkBytes = $MaximumWorkBytes
            environment = [ordered]@{
                GOFLAGS = $env:GOFLAGS
                GOMAXPROCS = $env:GOMAXPROCS
                GOPROXY = $env:GOPROXY
                GOSUMDB = $env:GOSUMDB
                GOTOOLCHAIN = $env:GOTOOLCHAIN
                npm_config_offline = $env:npm_config_offline
            }
        }
        if ($releaseResult.exitCode -ne 0) {
            Fail-Smoke "GoReleaser exited $($releaseResult.exitCode): $($releaseResult.stderr)"
        }
        if ($MaximumWorkBytes -le 0 -or $workBytes -gt $MaximumWorkBytes) {
            Fail-Smoke "candidate work directory used $workBytes bytes, limit $MaximumWorkBytes"
        }
        $statusAfter = Invoke-SmokeGit @("-C", $checkoutPath, "status", "--porcelain=v1", "--untracked-files=all")
        if ($statusAfter -ne "") {
            Fail-Smoke "candidate clone is dirty after release"
        }
        $archives = @(Get-ChildItem -LiteralPath (Join-Path $checkoutPath "dist") -Filter "you_*_windows_amd64.zip" -File -ErrorAction Stop)
        if ($archives.Count -ne 1) {
            Fail-Smoke "release produced $($archives.Count) Windows amd64 archives, want exactly one"
        }
        $archiveMatch = [regex]::Match($archives[0].Name, '^you_(.+)_windows_amd64\.zip$')
        if (-not $archiveMatch.Success) {
            Fail-Smoke "release archive name does not expose its version: $($archives[0].Name)"
        }
        $version = $archiveMatch.Groups[1].Value
        $checksumSource = Join-Path $checkoutPath "dist\you_${version}_checksums.txt"
        $installerSource = Join-Path $checkoutPath "scripts\install.ps1"
        foreach ($source in @($archives[0].FullName, $checksumSource, $installerSource)) {
            [void](Assert-SmokeRegularFile "candidate artifact" $source)
            Copy-Item -LiteralPath $source -Destination $outputDirectory
        }
        $archivePath = Join-Path $outputDirectory $archives[0].Name
        $checksumPath = Join-Path $outputDirectory ([System.IO.Path]::GetFileName($checksumSource))
        $installerPath = Join-Path $outputDirectory "install.ps1"
        [void][System.IO.Directory]::CreateDirectory($extractPath)
        Expand-Archive -LiteralPath $archivePath -DestinationPath $extractPath -Force
        $archiveExecutablePath = Join-Path $extractPath "you.exe"
        [void](Assert-SmokeRegularFile "archive executable" $archiveExecutablePath)
        $report.artifacts = @(
            Get-SmokeFileEvidence "windows-amd64-archive" $archivePath
            Get-SmokeFileEvidence "checksums" $checksumPath
            Get-SmokeFileEvidence "windows-installer" $installerPath
            Get-SmokeFileEvidence "windows-amd64-executable" $archiveExecutablePath
        )
        $installArguments = @{
            CandidateDirectory = $outputDirectory
            ArchivePath = $archivePath
            ChecksumPath = $checksumPath
            InstallerPath = $installerPath
            ArchiveExecutablePath = $archiveExecutablePath
            Version = $version
            ExpectedSourceCommit = $sourceIdentity.commit
            GoExecutablePath = $goCommand.Source
            RequestedInstallDir = $installDirectory
            WorkDirectory = $workDirectory
        }
        $report.install = Invoke-InstalledCandidateSmoke @installArguments
        $report.build.executableBuildInfo = $report.install.executableBuildInfo
        $report.status = "PASS"
    } catch {
        $failure = $_.Exception
        $report.error = $failure.Message
    } finally {
        foreach ($name in $environmentNames) {
            [System.Environment]::SetEnvironmentVariable($name, $originalEnvironment[$name], "Process")
        }
        try {
            Remove-SmokeOwnedTree "install directory" $installDirectory
            Remove-SmokeCandidateWork -WorkDirectory $workDirectory -DependencyJunctionPath $dependencyJunctionPath
            $report.cleanup = [ordered]@{
                status = "PASS"
                workDirectoryRemoved = -not (Test-Path -LiteralPath $workDirectory)
                installDirectoryRemoved = -not (Test-Path -LiteralPath $installDirectory)
            }
        } catch {
            $report.cleanup = [ordered]@{ status = "FAIL"; error = $_.Exception.Message }
            if ($null -eq $failure) { $failure = $_.Exception }
            $report.status = "FAIL"
        }
        if ($report.cleanup.status -eq "PASS" -and @($report.artifacts).Count -gt 0) {
            $retainedEvidenceStable = $true
            foreach ($artifact in @($report.artifacts)) {
                $artifactPath = Join-Path $outputDirectory ([string]$artifact.file)
                $currentEvidence = Get-SmokeFileEvidence ([string]$artifact.role) $artifactPath
                if ($currentEvidence.bytes -ne [int64]$artifact.bytes -or $currentEvidence.sha256 -cne [string]$artifact.sha256) {
                    $retainedEvidenceStable = $false
                    break
                }
            }
            $report.cleanup.retainedEvidenceHashesStable = $retainedEvidenceStable
            if (-not $retainedEvidenceStable) {
                $report.cleanup.status = "FAIL"
                if ($null -eq $failure) { $failure = [System.Exception]::new("retained candidate evidence changed during install smoke cleanup") }
                $report.status = "FAIL"
            }
        }
        [void][System.IO.Directory]::CreateDirectory((Split-Path -Parent $reportPath))
        $report | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $reportPath -Encoding UTF8
        $reportEvidence = Get-SmokeFileEvidence "candidate-report" $reportPath
        $reportDigestPath = Join-Path $outputDirectory "candidate-report.sha256"
        $reportDigest = "$($reportEvidence.sha256)  $([System.IO.Path]::GetFileName($reportPath))`n"
        [System.IO.File]::WriteAllText($reportDigestPath, $reportDigest, [System.Text.UTF8Encoding]::new($false))
    }
    if ($null -ne $failure) {
        throw $failure
    }
    Write-Output "local candidate build and install smoke passed for $($sourceIdentity.commit); report: $reportPath"
}

function Invoke-HostedInstallSmoke {
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
        New-Item -ItemType Directory -Path $tempHome -Force | Out-Null
        New-Item -ItemType Directory -Path $RequestedInstallDir -Force | Out-Null
        $scriptPath = Join-Path $tempHome "install.ps1"
        Invoke-WebRequest -Uri $ScriptUrl -OutFile $scriptPath
        $env:HOME = $tempHome
        $env:INFINITE_YOU_VERSION = $Version
        $env:INFINITE_YOU_INSTALL_DIR = $RequestedInstallDir
        & $scriptPath
        $binaryPath = Join-Path $RequestedInstallDir $RequestedBinaryName
        if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) {
            Fail-Smoke "installed binary missing: $binaryPath"
        }
        & $binaryPath --help | Out-Null
        & $binaryPath --json factory list --dir (Join-Path $tempHome ".you-agent-factory" "factories") | Out-Null
        $configPath = Join-Path $tempHome ".you-agent-factory\config.json"
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

if ($MyInvocation.InvocationName -eq ".") { return }
if (-not [string]::IsNullOrWhiteSpace($CandidateSourcePath)) {
    $candidateArguments = @{
        SourcePath = $CandidateSourcePath
        SourceCommit = $CandidateSourceCommit
        SourceRepository = $CandidateSourceRepository
        DependencySourcePath = $CandidateDependencySourcePath
        OutputDirectory = $CandidateOutputDir
        WorkDirectory = $CandidateWorkDir
        ReleaseToolPath = $GoReleaserPath
        ReleaseToolVersion = $ExpectedGoReleaserVersion
        EsbuildVersion = $ExpectedEsbuildVersion
        EsbuildSHA256 = $ExpectedEsbuildSHA256
        MaximumWorkBytes = $CandidateMaximumWorkBytes
        RequestedInstallDir = $InstallDir
        RequestedReportPath = $ReportPath
    }
    Invoke-LocalCandidateSmoke @candidateArguments
    exit 0
}
$hostedArguments = @{
    ScriptUrl = $InstallScriptUrl
    Version = $InstallVersion
    RequestedInstallDir = $InstallDir
    RequestedBinaryName = $BinaryName
}
Invoke-HostedInstallSmoke @hostedArguments
