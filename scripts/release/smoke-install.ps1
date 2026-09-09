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

function Protect-SmokeOutput {
    param([string]$Path, [int]$MaximumBytes = 1048576)
    $bytes = [System.IO.File]::ReadAllBytes($Path)
    $count = [Math]::Min($bytes.Length, $MaximumBytes)
    if ($count -ge 2 -and $bytes[0] -eq 0xff -and $bytes[1] -eq 0xfe) {
        $text = [System.Text.Encoding]::Unicode.GetString($bytes, 2, $count - 2)
    } elseif ($count -ge 3 -and $bytes[0] -eq 0xef -and $bytes[1] -eq 0xbb -and $bytes[2] -eq 0xbf) {
        $text = [System.Text.Encoding]::UTF8.GetString($bytes, 3, $count - 3)
    } else {
        $text = [System.Text.Encoding]::UTF8.GetString($bytes, 0, $count)
    }
    $pattern = '(?i)(token|password|secret|api[_-]?key|authorization)\s*([:=])\s*[^\s,;]+'
    $text = [System.Text.RegularExpressions.Regex]::Replace($text, $pattern, '$1$2<redacted>')
    [System.IO.File]::WriteAllText($Path, $text, [System.Text.UTF8Encoding]::new($false))
    $fileEvidence = Get-SmokeFileEvidence "command-output" $Path
    $evidence = [ordered]@{
        role = $fileEvidence.role
        file = $fileEvidence.file
        bytes = $fileEvidence.bytes
        sha256 = $fileEvidence.sha256
        totalBytes = [int64]$bytes.Length
        truncated = $bytes.Length -gt $MaximumBytes
    }
    return [pscustomobject]@{ text = $text; evidence = $evidence }
}

function Invoke-CandidateCommand {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$ArgumentList,
        [Parameter(Mandatory = $true)][string]$WorkingDirectory,
        [Parameter(Mandatory = $true)][string]$StdoutPath,
        [Parameter(Mandatory = $true)][string]$StderrPath
    )
    $workingDirectory = Resolve-SmokePath $WorkingDirectory
    $stdoutPath = Resolve-SmokePath $StdoutPath
    $stderrPath = Resolve-SmokePath $StderrPath
    foreach ($parent in @((Split-Path -Parent $stdoutPath), (Split-Path -Parent $stderrPath))) {
        [void][System.IO.Directory]::CreateDirectory($parent)
    }
    $previousLocation = Get-Location
    $previousErrorActionPreference = $ErrorActionPreference
    $exitCode = $null
    try {
        Set-Location -LiteralPath $workingDirectory
        # Windows PowerShell promotes native stderr to ErrorRecord when the
        # caller uses Stop. Capture it as process output and decide by exit.
        $ErrorActionPreference = "Continue"
        if (Test-Path Variable:\PSNativeCommandUseErrorActionPreference) { $PSNativeCommandUseErrorActionPreference = $false }
        & $FilePath @ArgumentList 1> $stdoutPath 2> $stderrPath
        $exitCode = [int]$LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
        Set-Location -LiteralPath $previousLocation.Path
    }
    $stdout = Protect-SmokeOutput $stdoutPath
    $stderr = Protect-SmokeOutput $stderrPath
    return [pscustomobject][ordered]@{
        file = $FilePath
        arguments = @($ArgumentList)
        exitCode = $exitCode
        stdout = $stdout.text
        stderr = $stderr.text
        stdoutEvidence = $stdout.evidence
        stderrEvidence = $stderr.evidence
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
        [string]$RequestedInstallDir,
        [string]$WorkDirectory
    )
    $installDir = Resolve-SmokePath $RequestedInstallDir
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
        $versionCommand = Invoke-CandidateCommand -FilePath $installedBinary `
            -ArgumentList @("--version") -WorkingDirectory $smokeRoot `
            -StdoutPath (Join-Path $smokeRoot "version.stdout") `
            -StderrPath (Join-Path $smokeRoot "version.stderr")
        [void]$commands.Add($versionCommand)
        if ($versionCommand.exitCode -ne 0 -or $versionCommand.stdout.Trim() -cne $Version) {
            Fail-Smoke "installed candidate version is '$($versionCommand.stdout.Trim())', want '$Version'"
        }
        $helpCommand = Invoke-CandidateCommand -FilePath $installedBinary `
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
            $inspect = Invoke-CandidateCommand -FilePath $installedBinary `
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
            commands = @($commands | ForEach-Object { $_ })
            modelCalls = 0
            modelBackendDownloadBytes = 0
        }
    } finally {
        Stop-CandidateFileServer $server
        foreach ($name in $environmentNames) {
            [System.Environment]::SetEnvironmentVariable($name, $originalEnvironment[$name], "Process")
        }
        if (Test-Path -LiteralPath $installDir) {
            Remove-Item -LiteralPath $installDir -Recurse -Force
        }
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
    $sourcePath = Resolve-SmokePath $SourcePath
    $dependencySourcePath = Resolve-SmokePath $DependencySourcePath
    $outputDirectory = Resolve-SmokePath $OutputDirectory
    $workDirectory = Resolve-SmokePath $WorkDirectory
    $installDirectory = Resolve-SmokePath $RequestedInstallDir
    $reportPath = Resolve-SmokePath $RequestedReportPath
    $releaseToolPath = Resolve-SmokePath $ReleaseToolPath
    Assert-SmokeEmptyRoot "candidate output directory" $outputDirectory
    Assert-SmokeEmptyRoot "candidate work directory" $workDirectory
    Assert-SmokeEmptyRoot "install directory" $installDirectory
    [void](Assert-SmokeRegularFile "GoReleaser" $releaseToolPath)
    if ($workDirectory.Length -gt 80) {
        Fail-Smoke "candidate work directory must be at most 80 characters so Windows native tools remain below path limits"
    }
    foreach ($pair in @(@($outputDirectory, $workDirectory), @($outputDirectory, $installDirectory), @($workDirectory, $installDirectory))) {
        if ($pair[0].TrimEnd('\').Equals($pair[1].TrimEnd('\'), [System.StringComparison]::OrdinalIgnoreCase)) {
            Fail-Smoke "candidate output, work, and install directories must be distinct"
        }
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
        if ($releaseToolResult.exitCode -ne 0 -or -not $releaseToolResult.stdout.Contains($ReleaseToolVersion)) {
            Fail-Smoke "GoReleaser version check failed: exit=$($releaseToolResult.exitCode) output='$($releaseToolResult.stdout.Trim())'"
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
            stdout = $releaseResult.stdoutEvidence
            stderr = $releaseResult.stderrEvidence
            goReleaser = $releaseToolEvidence
            goReleaserVersion = $ReleaseToolVersion
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
            RequestedInstallDir = $installDirectory
            WorkDirectory = $workDirectory
        }
        $report.install = Invoke-InstalledCandidateSmoke @installArguments
        $report.status = "PASS"
    } catch {
        $failure = $_.Exception
        $report.error = $failure.Message
    } finally {
        foreach ($name in $environmentNames) {
            [System.Environment]::SetEnvironmentVariable($name, $originalEnvironment[$name], "Process")
        }
        try {
            if (Test-Path -LiteralPath $installDirectory) {
                Remove-Item -LiteralPath $installDirectory -Recurse -Force
            }
            if (Test-Path -LiteralPath $workDirectory) {
                Remove-Item -LiteralPath $workDirectory -Recurse -Force
            }
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
