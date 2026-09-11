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
    [ValidateRange(1, 4)]
    [int]$CandidateGoProcessLimit = 4,
    [int64]$CandidateMaximumWorkBytes = 4294967296,
    [string]$ReportPath
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
$SmokeMaximumTaskRootPathLength = 220
$script:SmokeInstallScriptPath = $PSCommandPath

if ($null -eq ([System.Management.Automation.PSTypeName]::new("InfiniteYou.ReleaseSmoke.ProcessRunner").Type)) {
    Add-Type -TypeDefinition @'
using System;
using System.Diagnostics;
using System.IO;
using System.Text;
using System.Threading;
using System.Threading.Tasks;

namespace InfiniteYou.ReleaseSmoke
{
    public sealed class CommandResult
    {
        public int ExitCode { get; set; }
        public bool TimedOut { get; set; }
        public bool OutputLimitExceeded { get; set; }
    }

    internal sealed class CaptureState
    {
        public long TotalBytes;
        public int LimitExceeded;
    }

    public static class ProcessRunner
    {
        public static CommandResult Run(
            string filePath,
            string[] arguments,
            string workingDirectory,
            string stdoutPath,
            string stderrPath,
            int timeoutSeconds,
            int outputMaximumBytes)
        {
            if (timeoutSeconds <= 0) throw new ArgumentOutOfRangeException("timeoutSeconds");
            if (outputMaximumBytes <= 0) throw new ArgumentOutOfRangeException("outputMaximumBytes");

            var startInfo = new ProcessStartInfo
            {
                FileName = filePath,
                Arguments = BuildArguments(arguments),
                WorkingDirectory = workingDirectory,
                UseShellExecute = false,
                CreateNoWindow = true,
                RedirectStandardOutput = true,
                RedirectStandardError = true
            };
            var process = new Process { StartInfo = startInfo };
            var state = new CaptureState();
            var terminationRequested = 0;
            try
            {
                if (!process.Start()) throw new InvalidOperationException("process did not start");
                Action stop = delegate
                {
                    if (Interlocked.Exchange(ref terminationRequested, 1) == 0)
                    {
                        KillProcessTree(process);
                    }
                };
                Task stdoutTask = CaptureAsync(process.StandardOutput.BaseStream, stdoutPath, outputMaximumBytes, state, stop);
                Task stderrTask = CaptureAsync(process.StandardError.BaseStream, stderrPath, outputMaximumBytes, state, stop);
                Task processTask = Task.Run(delegate { process.WaitForExit(); });
                Task all = Task.WhenAll(stdoutTask, stderrTask, processTask);
                long timeoutMilliseconds = (long)timeoutSeconds * 1000L;
                int waitMilliseconds = timeoutMilliseconds >= Int32.MaxValue ? Int32.MaxValue : (int)timeoutMilliseconds;
                bool completed = all.Wait(waitMilliseconds);
                bool timedOut = !completed;
                if (timedOut)
                {
                    stop();
                    try { all.Wait(10000); } catch (AggregateException) { }
                }
                if (!all.IsCompleted)
                {
                    throw new TimeoutException("process did not terminate after the bounded kill wait");
                }
                all.GetAwaiter().GetResult();
                int exitCode = process.ExitCode;
                bool outputLimitExceeded = Volatile.Read(ref state.LimitExceeded) != 0;
                if (outputLimitExceeded) exitCode = 125;
                else if (timedOut) exitCode = 124;
                return new CommandResult
                {
                    ExitCode = exitCode,
                    TimedOut = timedOut,
                    OutputLimitExceeded = outputLimitExceeded
                };
            }
            finally
            {
                try
                {
                    if (!process.HasExited) KillProcessTree(process);
                }
                catch (InvalidOperationException) { }
                process.Dispose();
            }
        }

        private static async Task CaptureAsync(
            Stream input,
            string path,
            int maximumBytes,
            CaptureState state,
            Action limit)
        {
            using (var output = new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.ReadWrite, 8192, true))
            {
                var buffer = new byte[8192];
                long total = 0;
                while (true)
                {
                    int read;
                    try
                    {
                        read = await input.ReadAsync(buffer, 0, buffer.Length).ConfigureAwait(false);
                    }
                    catch (IOException)
                    {
                        if (Volatile.Read(ref state.LimitExceeded) != 0) return;
                        throw;
                    }
                    if (read == 0) return;
                    long previous = total;
                    total += read;
                    Interlocked.Exchange(ref state.TotalBytes, total);
                    long retain = Math.Min((long)read, Math.Max(0L, (long)maximumBytes - previous + 1L));
                    if (retain > 0) await output.WriteAsync(buffer, 0, (int)retain).ConfigureAwait(false);
                    if (total > maximumBytes)
                    {
                        Interlocked.Exchange(ref state.LimitExceeded, 1);
                        limit();
                        return;
                    }
                }
            }
        }

        private static void KillProcessTree(Process process)
        {
            try
            {
                string systemRoot = Environment.GetEnvironmentVariable("SystemRoot");
                if (String.IsNullOrEmpty(systemRoot)) systemRoot = "C:\\Windows";
                var killer = Process.Start(new ProcessStartInfo
                {
                    FileName = Path.Combine(systemRoot, "System32", "taskkill.exe"),
                    Arguments = "/PID " + process.Id.ToString() + " /T /F",
                    UseShellExecute = false,
                    CreateNoWindow = true
                });
                if (killer != null)
                {
                    try { killer.WaitForExit(5000); } finally { killer.Dispose(); }
                }
            }
            catch (Exception) { }
            try
            {
                if (!process.HasExited) process.Kill();
            }
            catch (Exception) { }
        }

        private static string BuildArguments(string[] arguments)
        {
            var values = new string[arguments == null ? 0 : arguments.Length];
            for (int index = 0; index < values.Length; index++) values[index] = QuoteArgument(arguments[index]);
            return String.Join(" ", values);
        }

        private static string QuoteArgument(string argument)
        {
            if (argument == null) argument = String.Empty;
            bool needsQuotes = argument.Length == 0;
            for (int index = 0; index < argument.Length && !needsQuotes; index++)
            {
                needsQuotes = Char.IsWhiteSpace(argument[index]) || argument[index] == '"';
            }
            if (!needsQuotes) return argument;
            var result = new StringBuilder();
            result.Append('"');
            int slashes = 0;
            foreach (char value in argument)
            {
                if (value == '\\')
                {
                    slashes++;
                    continue;
                }
                if (value == '"')
                {
                    result.Append('\\', slashes * 2 + 1);
                    result.Append('"');
                    slashes = 0;
                    continue;
                }
                result.Append('\\', slashes);
                result.Append(value);
                slashes = 0;
            }
            result.Append('\\', slashes * 2);
            result.Append('"');
            return result.ToString();
        }
    }
}
'@
}

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

function Get-SmokeCandidateArchiveSet {
    param([string]$DistDirectory)
    $dist = Resolve-SmokePath $DistDirectory
    if (-not (Test-Path -LiteralPath $dist -PathType Container)) {
        Fail-Smoke "candidate release dist directory is missing: $dist"
    }
    $files = @(Get-ChildItem -LiteralPath $dist -File -Force -ErrorAction Stop)
    $specifications = @(
        [ordered]@{ role = "darwin-amd64-archive"; os = "darwin"; arch = "amd64"; extension = "tar.gz" }
        [ordered]@{ role = "darwin-arm64-archive"; os = "darwin"; arch = "arm64"; extension = "tar.gz" }
        [ordered]@{ role = "linux-amd64-archive"; os = "linux"; arch = "amd64"; extension = "tar.gz" }
        [ordered]@{ role = "linux-arm64-archive"; os = "linux"; arch = "arm64"; extension = "tar.gz" }
        [ordered]@{ role = "windows-amd64-archive"; os = "windows"; arch = "amd64"; extension = "zip" }
        [ordered]@{ role = "windows-arm64-archive"; os = "windows"; arch = "arm64"; extension = "zip" }
    )
    $archives = New-Object 'System.Collections.Generic.List[object]'
    $version = ""
    foreach ($specification in $specifications) {
        $pattern = '^you_(?<version>.+)_' + [regex]::Escape($specification.os) + '_' +
            [regex]::Escape($specification.arch) + '\.' + [regex]::Escape($specification.extension) + '$'
        $matchingFiles = @($files | Where-Object { $_.Name -cmatch $pattern })
        if ($matchingFiles.Count -ne 1) {
            Fail-Smoke "candidate artifact role $($specification.role) matched $($matchingFiles.Count) files, want exactly one"
        }
        $nameMatch = [regex]::Match($matchingFiles[0].Name, $pattern)
        $archiveVersion = $nameMatch.Groups["version"].Value
        if ([string]::IsNullOrWhiteSpace($archiveVersion)) {
            Fail-Smoke "candidate artifact role $($specification.role) has an empty version"
        }
        if ([string]::IsNullOrWhiteSpace($version)) {
            $version = $archiveVersion
        } elseif ($version -cne $archiveVersion) {
            Fail-Smoke "candidate artifact role $($specification.role) has version $archiveVersion, want $version"
        }
        [void]$archives.Add([pscustomobject][ordered]@{
            role = $specification.role
            sourcePath = $matchingFiles[0].FullName
            file = $matchingFiles[0].Name
        })
    }
    $checksumPath = Join-Path $dist "you_${version}_checksums.txt"
    [void](Assert-SmokeRegularFile "checksums source" $checksumPath)
    return [pscustomobject][ordered]@{
        version = $version
        archives = @($archives | ForEach-Object { $_ })
        checksumPath = $checksumPath
    }
}

function Copy-SmokeCandidateArtifact {
    param(
        [string]$Role,
        [string]$SourcePath,
        [string]$DestinationPath
    )
    $sourceEvidence = Get-SmokeFileEvidence "$Role source" $SourcePath
    $destination = Resolve-SmokePath $DestinationPath
    [void][System.IO.Directory]::CreateDirectory((Split-Path -Parent $destination))
    [void](Copy-Item -LiteralPath (Resolve-SmokePath $SourcePath) -Destination $destination -Force)
    $retainedEvidence = Get-SmokeFileEvidence $Role $destination
    if ($sourceEvidence.bytes -ne $retainedEvidence.bytes -or
        $sourceEvidence.sha256 -cne $retainedEvidence.sha256) {
        Fail-Smoke "$Role promotion changed bytes or SHA-256"
    }
    return $retainedEvidence
}

function Promote-SmokeCandidateArtifacts {
    param(
        [string]$DistDirectory,
        [string]$InstallerSourcePath,
        [string]$OutputDirectory
    )
    $output = Resolve-SmokePath $OutputDirectory
    Assert-SmokeTaskRootLength "candidate output directory" $output
    Assert-SmokeEmptyRoot "candidate output directory" $output
    [void][System.IO.Directory]::CreateDirectory($output)
    $archiveSet = Get-SmokeCandidateArchiveSet -DistDirectory $DistDirectory
    $installerSource = Resolve-SmokePath $InstallerSourcePath
    [void](Assert-SmokeRegularFile "windows-installer source" $installerSource)
    foreach ($archive in @($archiveSet.archives)) {
        [void](Get-SmokeFileEvidence "$($archive.role) source" $archive.sourcePath)
    }
    [void](Get-SmokeFileEvidence "checksums source" $archiveSet.checksumPath)
    [void](Get-SmokeFileEvidence "windows-installer source" $installerSource)
    $retained = New-Object 'System.Collections.Generic.List[object]'
    foreach ($archive in @($archiveSet.archives)) {
        $destination = Join-Path $output $archive.file
        [void]$retained.Add((Copy-SmokeCandidateArtifact -Role $archive.role `
            -SourcePath $archive.sourcePath -DestinationPath $destination))
    }
    $checksumName = [System.IO.Path]::GetFileName($archiveSet.checksumPath)
    $checksumPath = Join-Path $output $checksumName
    [void]$retained.Add((Copy-SmokeCandidateArtifact -Role "checksums" `
        -SourcePath $archiveSet.checksumPath -DestinationPath $checksumPath))
    $installerPath = Join-Path $output "install.ps1"
    [void]$retained.Add((Copy-SmokeCandidateArtifact -Role "windows-installer" `
        -SourcePath $installerSource -DestinationPath $installerPath))
    $windowsArchive = @($archiveSet.archives | Where-Object { $_.role -eq "windows-amd64-archive" })
    if ($windowsArchive.Count -ne 1) {
        Fail-Smoke "candidate retained Windows amd64 archive selection is not unique"
    }
    return [pscustomobject][ordered]@{
        version = $archiveSet.version
        windowsAmd64Path = Join-Path $output $windowsArchive[0].file
        checksumPath = $checksumPath
        installerPath = $installerPath
        artifacts = @($retained | ForEach-Object { $_ })
    }
}

function Promote-SmokeCommandEvidence {
    param(
        [string]$SourceDirectory,
        [string]$OutputDirectory
    )
    $source = Resolve-SmokePath $SourceDirectory
    $output = Resolve-SmokePath $OutputDirectory
    if (-not (Test-Path -LiteralPath $source)) { return @() }
    $sourceItem = Get-Item -LiteralPath $source -Force -ErrorAction Stop
    if (-not $sourceItem.PSIsContainer -or (($sourceItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "candidate command evidence directory must be a regular directory: $source"
    }
    $entries = @(Get-ChildItem -LiteralPath $source -Force -ErrorAction Stop)
    foreach ($entry in $entries) {
        if (-not $entry.PSIsContainer -and (($entry.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -eq 0)) { continue }
        Fail-Smoke "candidate command evidence must contain only regular files: $($entry.FullName)"
    }
    [void][System.IO.Directory]::CreateDirectory($output)
    $retained = New-Object 'System.Collections.Generic.List[object]'
    foreach ($entry in $entries) {
        $destination = Join-Path $output $entry.Name
        if (Test-Path -LiteralPath $destination) {
            Fail-Smoke "candidate command evidence destination already exists: $destination"
        }
        [void]$retained.Add((Copy-SmokeCandidateArtifact -Role "command-output" `
            -SourcePath $entry.FullName -DestinationPath $destination))
    }
    return @($retained | ForEach-Object { $_ })
}

function Copy-SmokeNativeToolToShortPath {
    param(
        [string]$SourcePath,
        [string]$DestinationPath,
        [string]$ExpectedSHA256
    )
    if ($ExpectedSHA256 -notmatch '^[0-9a-fA-F]{64}$') {
        Fail-Smoke "native tool SHA-256 must be a 64-character digest"
    }
    $sourceEvidence = Get-SmokeFileEvidence "native tool" $SourcePath
    if ($sourceEvidence.sha256 -cne $ExpectedSHA256.ToLowerInvariant()) {
        Fail-Smoke "native tool SHA-256 is $($sourceEvidence.sha256), want $($ExpectedSHA256.ToLowerInvariant())"
    }
    $destination = Resolve-SmokePath $DestinationPath
    if (Test-Path -LiteralPath $destination) {
        Fail-Smoke "short native tool destination already exists: $destination"
    }
    if ($destination.Length -gt $SmokeMaximumTaskRootPathLength) {
        Fail-Smoke "short native tool path is $($destination.Length) characters, want at most $SmokeMaximumTaskRootPathLength"
    }
    [void][System.IO.Directory]::CreateDirectory((Split-Path -Parent $destination))
    Copy-Item -LiteralPath (Resolve-SmokePath $SourcePath) -Destination $destination -Force
    $destinationEvidence = Get-SmokeFileEvidence "short native tool" $destination
    if ($destinationEvidence.bytes -ne $sourceEvidence.bytes -or
        $destinationEvidence.sha256 -cne $ExpectedSHA256.ToLowerInvariant()) {
        Fail-Smoke "short native tool bytes or SHA-256 differ from the verified source"
    }
    return [ordered]@{
        source = $sourceEvidence
        destination = $destinationEvidence
        path = $destination
        pathLength = $destination.Length
    }
}

function Get-SmokeDirectoryBytes {
    param([string]$Path, [switch]$RejectReparsePoint)
    if (-not (Test-Path -LiteralPath $Path -PathType Container)) { return [int64]0 }
    $total = [int64]0
    $pending = New-Object 'System.Collections.Generic.Stack[string]'
    $pending.Push((Resolve-SmokePath $Path))
    while ($pending.Count -gt 0) {
        foreach ($item in @(Get-ChildItem -LiteralPath $pending.Pop() -Force -ErrorAction Stop)) {
            if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
                if ($RejectReparsePoint) { Fail-Smoke "directory byte root contains a reparse point: $($item.FullName)" }
                continue
            }
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
    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $commandResult = [InfiniteYou.ReleaseSmoke.ProcessRunner]::Run(
            $FilePath, @($ArgumentList), $workingDirectory, $stdoutPath, $stderrPath,
            $TimeoutSeconds, $OutputMaximumBytes)
    }
    finally { $stopwatch.Stop() }
    $stdout = Protect-SmokeOutput $stdoutPath $OutputMaximumBytes
    $stderr = Protect-SmokeOutput $stderrPath $OutputMaximumBytes
    $terminationReason = if ($commandResult.TimedOut) {
        "deadline of $TimeoutSeconds seconds exceeded"
    } elseif ($commandResult.OutputLimitExceeded) {
        "output exceeded $OutputMaximumBytes bytes"
    } else {
        ""
    }
    return [pscustomobject][ordered]@{
        file = $FilePath
        arguments = @($ArgumentList)
        exitCode = [int]$commandResult.ExitCode
        timedOut = [bool]$commandResult.TimedOut
        outputLimitExceeded = [bool]$commandResult.OutputLimitExceeded
        terminationReason = $terminationReason
        elapsedMilliseconds = [int64][Math]::Max(0, $stopwatch.ElapsedMilliseconds)
        stdout = $stdout.text
        stderr = $stderr.text
        stdoutEvidence = $stdout.evidence
        stderrEvidence = $stderr.evidence
    }
}

function Invoke-InstalledCandidateCommand {
    param(
        [string]$ExecutablePath,
        [string[]]$Arguments,
        [string]$WorkingDirectory,
        [string]$OutputDirectory,
        [string]$OutputName,
        [System.Collections.Generic.List[object]]$Commands,
        [string[]]$CommandPrefix = @()
    )
    $processArguments = @($CommandPrefix) + @($Arguments)
    $command = Invoke-CandidateCommand -FilePath $ExecutablePath `
        -ArgumentList $processArguments -WorkingDirectory $WorkingDirectory `
        -StdoutPath (Join-Path $OutputDirectory "$OutputName.stdout") `
        -StderrPath (Join-Path $OutputDirectory "$OutputName.stderr")
    $evidence = [ordered]@{}
    foreach ($property in $command.PSObject.Properties) {
        $evidence[$property.Name] = $property.Value
    }
    # Keep report arguments tied to the installed CLI contract when a controlled
    # launcher is used by the release-harness tests.
    $evidence.arguments = @($Arguments)
    if (@($CommandPrefix).Count -gt 0) {
        $evidence.processArguments = @($processArguments)
    }
    $normalized = [pscustomobject]$evidence
    [void]$Commands.Add($normalized)
    return $normalized
}

function Assert-InstalledCandidateCommand {
    param(
        [object]$Command,
        [string]$Identity,
        [switch]$RequireOutput
    )
    if ([int]$Command.exitCode -ne 0) {
        Fail-Smoke "installed candidate $Identity exited $($Command.exitCode)"
    }
    if ($RequireOutput -and [string]::IsNullOrWhiteSpace([string]$Command.stdout)) {
        Fail-Smoke "installed candidate $Identity produced no output"
    }
}

function Invoke-InstalledCandidateDiscovery {
    param(
        [string]$ExecutablePath,
        [string]$WorkingDirectory,
        [string]$OutputDirectory,
        [string]$ExpectedVersion,
        [System.Collections.Generic.List[object]]$Commands,
        [string[]]$CommandPrefix = @()
    )
    $versionCommand = Invoke-InstalledCandidateCommand -ExecutablePath $ExecutablePath `
        -Arguments @("--version") -WorkingDirectory $WorkingDirectory `
        -OutputDirectory $OutputDirectory -OutputName "version" -Commands $Commands `
        -CommandPrefix $CommandPrefix
    Assert-InstalledCandidateCommand $versionCommand "--version" -RequireOutput
    $reportedVersion = $versionCommand.stdout.Trim()
    if ($reportedVersion -match '[\r\n]' -or $reportedVersion -cne $ExpectedVersion) {
        Fail-Smoke "installed candidate --version returned '$reportedVersion', want '$ExpectedVersion'"
    }

    $rootHelpCommand = Invoke-InstalledCandidateCommand -ExecutablePath $ExecutablePath `
        -Arguments @("--help") -WorkingDirectory $WorkingDirectory `
        -OutputDirectory $OutputDirectory -OutputName "root-help" -Commands $Commands `
        -CommandPrefix $CommandPrefix
    Assert-InstalledCandidateCommand $rootHelpCommand "--help" -RequireOutput
    $docsModelsCommand = Invoke-InstalledCandidateCommand -ExecutablePath $ExecutablePath `
        -Arguments @("docs", "models") -WorkingDirectory $WorkingDirectory `
        -OutputDirectory $OutputDirectory -OutputName "models-docs" -Commands $Commands `
        -CommandPrefix $CommandPrefix
    Assert-InstalledCandidateCommand $docsModelsCommand "docs models" -RequireOutput
    $docsAgentsCommand = Invoke-InstalledCandidateCommand -ExecutablePath $ExecutablePath `
        -Arguments @("docs", "agents") -WorkingDirectory $WorkingDirectory `
        -OutputDirectory $OutputDirectory -OutputName "agents-docs" -Commands $Commands `
        -CommandPrefix $CommandPrefix
    Assert-InstalledCandidateCommand $docsAgentsCommand "docs agents" -RequireOutput
    $listCommand = Invoke-InstalledCandidateCommand -ExecutablePath $ExecutablePath `
        -Arguments @("models", "list") -WorkingDirectory $WorkingDirectory `
        -OutputDirectory $OutputDirectory -OutputName "models-list" -Commands $Commands `
        -CommandPrefix $CommandPrefix
    Assert-InstalledCandidateCommand $listCommand "models list" -RequireOutput
    $helpCommand = Invoke-InstalledCandidateCommand -ExecutablePath $ExecutablePath `
        -Arguments @("models", "--help") -WorkingDirectory $WorkingDirectory `
        -OutputDirectory $OutputDirectory -OutputName "models-help" -Commands $Commands `
        -CommandPrefix $CommandPrefix
    Assert-InstalledCandidateCommand $helpCommand "models --help" -RequireOutput
    foreach ($name in @("llm", "asr", "tts", "embed")) {
        if (-not $helpCommand.stdout.Contains($name)) {
            Fail-Smoke "installed candidate models --help did not expose $name"
        }
        $inspectArguments = @("--json", "models", "inspect", $name)
        $inspect = Invoke-InstalledCandidateCommand -ExecutablePath $ExecutablePath `
            -Arguments $inspectArguments -WorkingDirectory $WorkingDirectory `
            -OutputDirectory $OutputDirectory -OutputName "model-$name" -Commands $Commands `
            -CommandPrefix $CommandPrefix
        Assert-InstalledCandidateCommand $inspect ($inspectArguments -join " ") -RequireOutput
        try {
            $model = $inspect.stdout | ConvertFrom-Json -ErrorAction Stop
        } catch {
            Fail-Smoke "installed candidate $($inspectArguments -join ' ') returned invalid JSON"
        }
        $unexpectedState = $null -eq $model -or $model.name -ne $name -or
            $model.managedRuntime.identity -ne $name -or
            $model.managedRuntime.readinessState -ne "MISSING" -or
            $model.managedRuntime.lifecycleState -ne "NOT_INSTALLED" -or
            $model.loadState -ne "UNLOADED"
        if ($unexpectedState) {
            Fail-Smoke "installed candidate models inspect $name performed work or returned an unexpected identity/state"
        }
    }
    return [pscustomobject][ordered]@{
        status = "PASS"
        version = $reportedVersion
        commands = @($Commands | ForEach-Object { $_ })
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
        $go = Get-Command go.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -eq $go) { Fail-Smoke "Go is required to inspect executable build info" }
        $goPath = [string]$go.Source
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
    $previousErrorActionPreference = $ErrorActionPreference
    try {
        # Windows PowerShell promotes native stderr to a terminating error when
        # the caller uses Stop. Git writes normal clone progress there, so
        # capture it as command evidence and classify failure by exit status.
        $ErrorActionPreference = "Continue"
        $output = @(& git.exe @ArgumentList 2>&1 | ForEach-Object { [string]$_ })
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    if ($exitCode -ne 0) { Fail-Smoke "git $($ArgumentList -join ' ') failed: $($output -join ' ')" }
    return ($output -join "`n").Trim()
}

function Get-CandidateDriverRevision {
    $scriptPath = $script:SmokeInstallScriptPath
    if ([string]::IsNullOrWhiteSpace($scriptPath)) {
        Fail-Smoke "candidate driver script path is unavailable"
    }
    $scriptDirectory = Split-Path -Parent (Resolve-SmokePath $scriptPath)
    $driverRoot = Invoke-SmokeGit @("-C", $scriptDirectory, "rev-parse", "--show-toplevel")
    $revision = Invoke-SmokeGit @("-C", $driverRoot, "rev-parse", "HEAD")
    if ($revision -notmatch '^[0-9a-fA-F]{40}$') {
        Fail-Smoke "candidate driver revision is not a full Git object ID: $revision"
    }
    return $revision.ToLowerInvariant()
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

function Set-CandidateGoProcessEnvironment {
    param(
        [ValidateRange(1, 4)]
        [int]$GoProcessLimit = 4
    )
    $env:GOFLAGS = "-p=$GoProcessLimit"
    $env:GOMAXPROCS = [string]$GoProcessLimit
    return [ordered]@{
        GOFLAGS = $env:GOFLAGS
        GOMAXPROCS = $env:GOMAXPROCS
    }
}

function Get-SmokeAvailablePort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    try { $listener.Start(); return [int]$listener.LocalEndpoint.Port } finally { $listener.Stop() }
}

function Get-SmokeActivitySnapshot {
    param([string]$Name, [string]$Path)
    $resolved = Resolve-SmokePath $Path
    $item = Get-Item -LiteralPath $resolved -Force -ErrorAction SilentlyContinue
    if ($null -eq $item) {
        return [pscustomobject][ordered]@{
            name = $Name
            path = $resolved
            exists = $false
            bytes = [int64]0
        }
    }
    if (-not $item.PSIsContainer -or (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "$Name activity root is not a regular directory: $resolved"
    }
    return [pscustomobject][ordered]@{
        name = $Name
        path = $resolved
        exists = $true
        bytes = [int64](Get-SmokeDirectoryBytes -Path $resolved -RejectReparsePoint)
    }
}

function Test-SmokeModelInvokeArguments {
    param([string[]]$Arguments)
    $values = @($Arguments)
    for ($index = 0; $index -lt ($values.Count - 1); $index++) {
        if ([string]$values[$index] -ieq "models" -and [string]$values[$index + 1] -ieq "invoke") {
            return $true
        }
    }
    return $false
}

function Get-SmokeRuntimeActivity {
    param([string]$Path)
    $resolved = Resolve-SmokePath $Path
    $item = Get-Item -LiteralPath $resolved -Force -ErrorAction SilentlyContinue
    if ($null -eq $item) {
        return [pscustomobject][ordered]@{
            path = $resolved
            exists = $false
            bytes = [int64]0
            records = 0
            backendProcessStarts = 0
        }
    }
    [void](Assert-SmokeRegularFile "model runtime evidence" $resolved)
    $records = 0
    $backendProcessStarts = 0
    foreach ($line in [System.IO.File]::ReadAllLines($resolved)) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        try {
            $record = $line | ConvertFrom-Json -ErrorAction Stop
        } catch {
            Fail-Smoke "model runtime evidence contains invalid JSON: $($_.Exception.Message)"
        }
        if ($null -eq $record) { Fail-Smoke "model runtime evidence contains an empty record" }
        $records++
        $kindProperty = $record.PSObject.Properties["kind"]
        $phaseProperty = $record.PSObject.Properties["phase"]
        $kind = if ($null -eq $kindProperty) { "" } else { [string]$kindProperty.Value }
        $phase = if ($null -eq $phaseProperty) { "" } else { [string]$phaseProperty.Value }
        if ($kind -eq "MANAGED_CHILD" -and $phase -eq "PROCESS_STARTED") { $backendProcessStarts++ }
    }
    return [pscustomobject][ordered]@{
        path = $resolved
        exists = $true
        bytes = [int64]$item.Length
        records = [int]$records
        backendProcessStarts = [int]$backendProcessStarts
    }
}

function Get-SmokeModelActivity {
    param(
        [object[]]$Before,
        [object[]]$After,
        [object[]]$Commands,
        [string]$RuntimeEvidencePath
    )
    $beforeSnapshots = @($Before)
    $afterSnapshots = @($After)
    if ($beforeSnapshots.Count -eq 0 -or $beforeSnapshots.Count -ne $afterSnapshots.Count) {
        Fail-Smoke "model activity observation has incomplete cache boundaries"
    }
    $beforeBytes = [int64]0
    $afterBytes = [int64]0
    $cacheRoots = New-Object 'System.Collections.Generic.List[string]'
    foreach ($snapshot in $beforeSnapshots) { $beforeBytes += [int64]$snapshot.bytes }
    foreach ($snapshot in $afterSnapshots) {
        $afterBytes += [int64]$snapshot.bytes
        [void]$cacheRoots.Add([string]$snapshot.path)
    }
    if ($afterBytes -lt $beforeBytes) {
        Fail-Smoke "model activity cache bytes decreased during observation"
    }
    $modelInvokeCommands = New-Object 'System.Collections.Generic.List[string]'
    foreach ($command in @($Commands)) {
        if (Test-SmokeModelInvokeArguments -Arguments @($command.arguments)) {
            [void]$modelInvokeCommands.Add(([string[]]@($command.arguments) -join " "))
        }
    }
    $runtime = Get-SmokeRuntimeActivity $RuntimeEvidencePath
    $cacheDelta = [int64]($afterBytes - $beforeBytes)
    return [pscustomobject][ordered]@{
        observed = $true
        cacheRoots = @($cacheRoots)
        cacheBytesBefore = $beforeBytes
        cacheBytesAfter = $afterBytes
        cacheBytesDelta = $cacheDelta
        modelCalls = [int]$modelInvokeCommands.Count
        modelInvokeCommands = @($modelInvokeCommands)
        modelBackendDownloadBytes = $cacheDelta
        runtimeEvidencePath = $runtime.path
        runtimeEvidenceObserved = [bool]$runtime.exists
        runtimeEvidenceRecords = [int]$runtime.records
        backendProcessStarts = [int]$runtime.backendProcessStarts
    }
}

function Assert-SmokeNoModelActivity {
    param([object]$Activity)
    if ($null -eq $Activity -or -not [bool]$Activity.observed -or
        -not [bool]$Activity.runtimeEvidenceObserved) {
        Fail-Smoke "model activity observation was unavailable"
    }
    if ([int]$Activity.modelCalls -ne 0 -or
        [int64]$Activity.modelBackendDownloadBytes -ne 0 -or
        [int64]$Activity.cacheBytesAfter -ne 0 -or
        [int]$Activity.runtimeEvidenceRecords -ne 0 -or
        [int]$Activity.backendProcessStarts -ne 0) {
        $details = $Activity | ConvertTo-Json -Compress -Depth 8
        Fail-Smoke "unexpected model or backend activity was observed: $details"
    }
}

function Start-CandidateFileServer {
    param([string]$InstallerPath, [string]$ArchivePath, [string]$ChecksumPath, [string]$Version, [string]$ReadyPath)
    $port = Get-SmokeAvailablePort
    $eventName = "Infinite-You-Candidate-Ready-" + [System.Guid]::NewGuid().ToString("N")
    $createdNew = $false
    $readyEvent = [System.Threading.EventWaitHandle]::new(
        $false, [System.Threading.EventResetMode]::ManualReset, $eventName, [ref]$createdNew)
    $job = Start-Job -ScriptBlock {
        param($Port, $InstallerPath, $ArchivePath, $ChecksumPath, $Version, $ReadyPath, $EventName)
        $ErrorActionPreference = "Stop"
        $listener = [System.Net.HttpListener]::new()
        $ready = [System.Threading.EventWaitHandle]::OpenExisting($EventName)
        $listener.Prefixes.Add("http://127.0.0.1:$Port/")
        try {
            $listener.Start()
            [System.IO.File]::WriteAllText($ReadyPath, "ready")
            [void]$ready.Set()
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
        } catch {
            try { [void]$ready.Set() } catch { }
            throw
        } finally {
            try { $ready.Close() } catch { }
            if ($listener.IsListening) { $listener.Stop() }
            $listener.Close()
        }
    } -ArgumentList $port, $InstallerPath, $ArchivePath, $ChecksumPath, $Version, $ReadyPath, $eventName
    if (-not $readyEvent.WaitOne(10000)) {
        $details = Receive-Job -Job $job -Keep -ErrorAction SilentlyContinue | Out-String
        Stop-Job -Job $job -ErrorAction SilentlyContinue
        Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
        $readyEvent.Close()
        Fail-Smoke "candidate file server did not become ready: $details"
    }
    if (-not (Test-Path -LiteralPath $ReadyPath -PathType Leaf)) {
        Stop-Job -Job $job -ErrorAction SilentlyContinue
        Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
        $readyEvent.Close()
        Fail-Smoke "candidate file server signaled readiness without its marker"
    }
    return [pscustomobject]@{ job = $job; readyEvent = $readyEvent; port = $port; baseUrl = "http://127.0.0.1:$port" }
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
        if ($null -ne $Server.readyEvent) { $Server.readyEvent.Close() }
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
        [string]$ExpectedExecutableVersion,
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
    $cacheRoot = Join-Path $smokeRoot "cache"
    $moduleAnalysisCacheRoot = Join-Path $smokeRoot "module-analysis-cache"
    $runtimeEvidencePath = Join-Path $smokeRoot "model-runtime-evidence.jsonl"
    foreach ($path in @($homeRoot, $profileRoot, $tempRoot, $pathRoot, $modelsRoot, $hfRoot, $cacheRoot, $moduleAnalysisCacheRoot)) {
        [void][System.IO.Directory]::CreateDirectory($path)
    }
    $readyPath = Join-Path $smokeRoot "server.ready"
    $server = $null
    $environmentNames = @(
        "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA",
        "TEMP", "TMP", "PATH", "XDG_CONFIG_HOME", "XDG_CACHE_HOME",
        "INFINITE_YOU_VERSION", "INFINITE_YOU_INSTALL_DIR", "INFINITE_YOU_INSTALL_OS",
        "INFINITE_YOU_INSTALL_ARCH", "INFINITE_YOU_INSTALL_BASE_URL",
        "INFINITE_YOU_OMNIVOICE_CACHE_DIR", "HUGGINGFACE_HUB_CACHE", "HF_HOME",
        "INFINITE_YOU_INTEGRATION_MODEL_RUNTIME_EVIDENCE", "PSModuleAnalysisCachePath",
        "HF_HUB_DISABLE_TELEMETRY"
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
        $env:LOCALAPPDATA = $cacheRoot
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
        $env:INFINITE_YOU_INTEGRATION_MODEL_RUNTIME_EVIDENCE = $runtimeEvidencePath
        $env:PSModuleAnalysisCachePath = Join-Path $moduleAnalysisCacheRoot "ModuleAnalysisCache"
        $env:HF_HUB_DISABLE_TELEMETRY = "1"
        $cacheRoots = @(
            Get-SmokeActivitySnapshot "managed-model-cache" $modelsRoot
            Get-SmokeActivitySnapshot "huggingface-backend-cache" $hfRoot
            Get-SmokeActivitySnapshot "redirected-local-cache" $cacheRoot
        )
        $activityBefore = @($cacheRoots)
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
        $expectedVersion = if ([string]::IsNullOrWhiteSpace($ExpectedExecutableVersion)) { $Version } else { $ExpectedExecutableVersion }
        $discovery = Invoke-InstalledCandidateDiscovery -ExecutablePath $resolvedPath `
            -WorkingDirectory $smokeRoot -OutputDirectory $smokeRoot `
            -ExpectedVersion $expectedVersion -Commands $commands
        $reportedVersion = $discovery.version
        foreach ($path in @($modelsRoot, $hfRoot)) {
            if ((Test-Path -LiteralPath $path) -and @(Get-ChildItem -LiteralPath $path -Force -ErrorAction Stop).Count -ne 0) {
                Fail-Smoke "discovery wrote model or backend content under $path"
            }
        }
        $activityAfter = @(
            Get-SmokeActivitySnapshot "managed-model-cache" $modelsRoot
            Get-SmokeActivitySnapshot "huggingface-backend-cache" $hfRoot
            Get-SmokeActivitySnapshot "redirected-local-cache" $cacheRoot
        )
        $activity = Get-SmokeModelActivity -Before $activityBefore -After $activityAfter `
            -Commands @($commands | ForEach-Object { $_ }) -RuntimeEvidencePath $runtimeEvidencePath
        Assert-SmokeNoModelActivity $activity
        return [pscustomobject][ordered]@{
            status = "PASS"
            installedExecutable = $installedEvidence
            pathResolution = $pathResolution
            version = $reportedVersion
            expectedVersion = $expectedVersion
            executableBuildInfo = $buildInfo
            commands = @($commands | ForEach-Object { $_ })
            activity = $activity
            modelCalls = [int]$activity.modelCalls
            modelBackendDownloadBytes = [int64]$activity.modelBackendDownloadBytes
        }
    } finally {
        Stop-CandidateFileServer $server
        foreach ($name in $environmentNames) {
            [System.Environment]::SetEnvironmentVariable($name, $originalEnvironment[$name], "Process")
        }
        Remove-SmokeOwnedTree "install directory" $installDir
    }
}

function Finalize-SmokeCandidateReport {
    param(
        [string]$OutputDirectory,
        [string]$ReportPath,
        [System.Collections.IDictionary]$Report,
        [System.Exception]$Failure
    )
    if (@($Report.artifacts).Count -gt 0) {
        $retainedEvidenceStable = $true
        $retainedEvidenceErrors = New-Object 'System.Collections.Generic.List[string]'
        foreach ($artifact in @($Report.artifacts)) {
            try {
                $artifactPath = Join-Path $OutputDirectory ([string]$artifact.file)
                $currentEvidence = Get-SmokeFileEvidence ([string]$artifact.role) $artifactPath
                if ($currentEvidence.bytes -ne [int64]$artifact.bytes -or
                    $currentEvidence.sha256 -cne [string]$artifact.sha256) {
                    $retainedEvidenceStable = $false
                    [void]$retainedEvidenceErrors.Add("$($artifact.role) changed after retention")
                }
            } catch {
                $retainedEvidenceStable = $false
                [void]$retainedEvidenceErrors.Add("$($artifact.role): $($_.Exception.Message)")
            }
        }
        $Report.cleanup.retainedEvidenceHashesStable = $retainedEvidenceStable
        if (-not $retainedEvidenceStable) {
            $Report.cleanup.status = "FAIL"
            $Report.cleanup.retainedEvidenceError = $retainedEvidenceErrors -join "; "
            if ($null -eq $Failure) {
                $Failure = [System.Exception]::new(
                    "retained candidate evidence was missing or changed during install smoke cleanup: " +
                    ($retainedEvidenceErrors -join "; "))
            }
            $Report.status = "FAIL"
            if ([string]::IsNullOrWhiteSpace([string]$Report.error)) { $Report.error = $Failure.Message }
        }
    }
    [void][System.IO.Directory]::CreateDirectory((Split-Path -Parent $ReportPath))
    $Report | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $ReportPath -Encoding UTF8
    $reportEvidence = Get-SmokeFileEvidence "candidate-report" $ReportPath
    $reportDigestPath = Join-Path $OutputDirectory "candidate-report.sha256"
    $reportDigest = "$($reportEvidence.sha256)  $([System.IO.Path]::GetFileName($ReportPath))`n"
    [System.IO.File]::WriteAllText($reportDigestPath, $reportDigest, [System.Text.UTF8Encoding]::new($false))
    return [pscustomobject][ordered]@{ report = $Report; failure = $Failure }
}

function Invoke-LocalCandidateSmoke {
    param(
        [string]$SourcePath,
        [string]$SourceCommit,
        [string]$SourceRepository,
        [string]$DependencySourcePath,
        [string]$OutputDirectory,
        [string]$WorkDirectory,
        [ValidateRange(1, 4)]
        [int]$GoProcessLimit = 4,
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
    [void][System.IO.Directory]::CreateDirectory($outputDirectory)
    [void][System.IO.Directory]::CreateDirectory($workDirectory)
    $checkoutPath = Join-Path $workDirectory "src"
    $extractPath = Join-Path $workDirectory "archive"
    $commandEvidenceDirectory = Join-Path $workDirectory "command-evidence"
    [void][System.IO.Directory]::CreateDirectory($commandEvidenceDirectory)
    $environmentNames = @("GOFLAGS", "GOMAXPROCS", "GOPROXY", "GOSUMDB", "GOTOOLCHAIN", "npm_config_offline", "ESBUILD_BINARY_PATH")
    $originalEnvironment = @{}
    foreach ($name in $environmentNames) {
        $originalEnvironment[$name] = [System.Environment]::GetEnvironmentVariable($name, "Process")
    }
    $report = [ordered]@{
        schemaVersion = "local-windows-candidate/v1"
        driverRevision = ""
        status = "FAIL"
        source = [ordered]@{
            repository = $SourceRepository
            commit = $SourceCommit
            tree = ""
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
        $report.driverRevision = Get-CandidateDriverRevision
        $sourceIdentity = Get-CandidateSourceIdentity -SourcePath $sourcePath `
            -Commit $SourceCommit -Repository $SourceRepository
        $report.source.repository = $sourceIdentity.repository
        $report.source.commit = $sourceIdentity.commit
        $report.source.tree = $sourceIdentity.tree
        $processEnvironment = Set-CandidateGoProcessEnvironment -GoProcessLimit $GoProcessLimit
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
        $esbuildOverridePath = Join-Path $workDirectory "esbuild-native.exe"
        $esbuildOverride = Copy-SmokeNativeToolToShortPath -SourcePath $esbuildPath `
            -DestinationPath $esbuildOverridePath -ExpectedSHA256 $EsbuildSHA256
        $esbuildResult = Invoke-CandidateCommand -FilePath $esbuildOverridePath `
            -ArgumentList @("--version") -WorkingDirectory $checkoutPath `
            -StdoutPath (Join-Path $commandEvidenceDirectory "esbuild.stdout.log") `
            -StderrPath (Join-Path $commandEvidenceDirectory "esbuild.stderr.log")
        if ($esbuildResult.exitCode -ne 0 -or $esbuildResult.stdout.Trim() -cne $EsbuildVersion) {
            Fail-Smoke "esbuild sanity check failed: exit=$($esbuildResult.exitCode) version='$($esbuildResult.stdout.Trim())'"
        }
        $env:ESBUILD_BINARY_PATH = $esbuildOverridePath
        $releaseToolEvidence = Get-SmokeFileEvidence "goreleaser" $releaseToolPath
        $releaseToolResult = Invoke-CandidateCommand -FilePath $releaseToolPath `
            -ArgumentList @("--version") -WorkingDirectory $checkoutPath `
            -StdoutPath (Join-Path $commandEvidenceDirectory "goreleaser-version.stdout.log") `
            -StderrPath (Join-Path $commandEvidenceDirectory "goreleaser-version.stderr.log")
        if ($releaseToolResult.exitCode -ne 0 -or $releaseToolResult.stdout -notmatch ("(?m)\b" + [regex]::Escape($ReleaseToolVersion) + "\b")) {
            Fail-Smoke "GoReleaser version check failed: exit=$($releaseToolResult.exitCode) output='$($releaseToolResult.stdout.Trim())'"
        }
        $goCommand = Get-Command go.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -eq $goCommand) { Fail-Smoke "Go 1.26.8 is required for candidate release" }
        $goPath = [string]$goCommand.Source
        $goVersionResult = Invoke-CandidateCommand -FilePath $goPath `
            -ArgumentList @("version") -WorkingDirectory $checkoutPath `
            -StdoutPath (Join-Path $commandEvidenceDirectory "go-version.stdout.log") `
            -StderrPath (Join-Path $commandEvidenceDirectory "go-version.stderr.log")
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
            -StdoutPath (Join-Path $commandEvidenceDirectory "release.stdout.log") `
            -StderrPath (Join-Path $commandEvidenceDirectory "release.stderr.log")
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
            esbuildOverride = $esbuildOverride
            esbuildVersion = $EsbuildVersion
            esbuildPathLength = $esbuildPath.Length
            esbuildOverridePathLength = $esbuildOverride.pathLength
            workBytes = $workBytes
            maximumWorkBytes = $MaximumWorkBytes
            environment = [ordered]@{
                GOFLAGS = $processEnvironment.GOFLAGS
                GOMAXPROCS = $processEnvironment.GOMAXPROCS
                GOPROXY = $env:GOPROXY
                GOSUMDB = $env:GOSUMDB
                GOTOOLCHAIN = $env:GOTOOLCHAIN
                npm_config_offline = $env:npm_config_offline
                ESBUILD_BINARY_PATH = $env:ESBUILD_BINARY_PATH
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
        $promotedArtifacts = Promote-SmokeCandidateArtifacts `
            -DistDirectory (Join-Path $checkoutPath "dist") `
            -InstallerSourcePath (Join-Path $checkoutPath "scripts\install.ps1") `
            -OutputDirectory $outputDirectory
        $version = $promotedArtifacts.version
        $archivePath = $promotedArtifacts.windowsAmd64Path
        $checksumPath = $promotedArtifacts.checksumPath
        $installerPath = $promotedArtifacts.installerPath
        $report.artifacts = @($promotedArtifacts.artifacts)
        [void][System.IO.Directory]::CreateDirectory($extractPath)
        Expand-Archive -LiteralPath $archivePath -DestinationPath $extractPath -Force
        $archiveExecutablePath = Join-Path $extractPath "you.exe"
        [void](Assert-SmokeRegularFile "archive executable" $archiveExecutablePath)
        $archiveVersionCommand = Invoke-CandidateCommand -FilePath $archiveExecutablePath `
            -ArgumentList @("--version") -WorkingDirectory $extractPath `
            -StdoutPath (Join-Path $outputDirectory "archive-version.stdout.log") `
            -StderrPath (Join-Path $outputDirectory "archive-version.stderr.log")
        $expectedExecutableVersion = $archiveVersionCommand.stdout.Trim()
        if ($archiveVersionCommand.exitCode -ne 0 -or [string]::IsNullOrWhiteSpace($expectedExecutableVersion) -or
            $expectedExecutableVersion -match '[\r\n]') {
            Fail-Smoke "archive executable version probe failed: exit=$($archiveVersionCommand.exitCode) output='$expectedExecutableVersion'"
        }
        $retainedExecutablePath = Join-Path $outputDirectory "you.exe"
        Copy-Item -LiteralPath $archiveExecutablePath -Destination $retainedExecutablePath -Force
        $archiveExecutableEvidence = Get-SmokeFileEvidence "archive executable" $archiveExecutablePath
        $retainedExecutableEvidence = Get-SmokeFileEvidence "windows-amd64-executable" $retainedExecutablePath
        if ($archiveExecutableEvidence.bytes -ne $retainedExecutableEvidence.bytes -or
            $archiveExecutableEvidence.sha256 -cne $retainedExecutableEvidence.sha256) {
            Fail-Smoke "retained executable does not match the archive member"
        }
        $report.artifacts = @($report.artifacts + $retainedExecutableEvidence)
        $installArguments = @{
            CandidateDirectory = $outputDirectory
            ArchivePath = $archivePath
            ChecksumPath = $checksumPath
            InstallerPath = $installerPath
            ArchiveExecutablePath = $archiveExecutablePath
            Version = $version
            ExpectedExecutableVersion = $expectedExecutableVersion
            ExpectedSourceCommit = $sourceIdentity.commit
            GoExecutablePath = $goPath
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
            [void](Promote-SmokeCommandEvidence -SourceDirectory $commandEvidenceDirectory `
                -OutputDirectory $outputDirectory)
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
        $finalized = Finalize-SmokeCandidateReport -OutputDirectory $outputDirectory `
            -ReportPath $reportPath -Report $report -Failure $failure
        $failure = $finalized.failure
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
        GoProcessLimit = $CandidateGoProcessLimit
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
