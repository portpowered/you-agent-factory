param(
    [string]$InstallScriptUrl,
    [string]$InstallVersion,
    [Parameter(Mandatory = $true)]
    [string]$InstallDir,
    [string]$BinaryName = "you.exe",
    [string]$CandidateManifestPath,
    [string]$ReportPath,
    [switch]$ObserverFixture,
    [switch]$ObserverIdentityFixture,
    [string]$ObserverReportPath,
    [string]$ObserverRootCommand,
    [string[]]$ObserverRootArgumentList,
    [string]$ObserverRootWorkingDirectory,
    [string]$ObserverRootStdoutPath,
    [string]$ObserverRootStderrPath,
    [int]$ObserverRootOutputMaximumBytes = 1048576,
    [int]$ObserverRootExitCode = 0,
    [switch]$StageDependencyClosure,
    [switch]$VerifyDependencyClosure,
    [string]$DependencySourceRoot,
    [string]$DependencyStageRoot,
    [string]$DependencySourceManifestPath,
    [string]$DependencyStageManifestPath,
    [string]$DependencyExpectedManifestPath,
    [string]$DependencyPostBuildManifestPath,
    [string]$DependencySourceAfterManifestPath,
    [string]$DependencyReportPath,
    [string]$DependencyExpectedLockSHA256 = "45ca702f71dbd8fbf47855dcf8cdb86b13785a009b2c107153ba52e0b20157ad",
    [string]$DependencyExpectedLockBlob = "d6e4b715db635497bfcac0db085163ea719d21ab",
    [string]$ObserverReleaseCheckoutPath,
    [switch]$PrepareReleaseCheckout,
    [string]$ReleaseCheckoutPath,
    [string]$ReleaseCheckoutReportPath,
    [int]$ObserverTimeoutSeconds = 20
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$expectedCandidateCycle = "070"
$expectedCandidateGoVersion = "go1.26.8"
$expectedObserverQueryMode = "one-full-TCP-table-query-per-interval-filtered-to-owned-process-identities"
$expectedProcessNetworkGapMilliseconds = 2000
$expectedDiskGapMilliseconds = 2000
$expectedDescendantMaximum = 36
$expectedTemporaryDiskBytesMaximum = [int64]4294967296
$expectedCandidateSourceRepository = "https://github.com/portpowered/you-agent-factory"
$expectedCandidateSourceCommit = "059474b2c00915865306a33ca5e3d02b618bb6f0"
$expectedCandidateSourceTree = "ae5d9c87fbc99a898ad2c11e1409105d78e4a094"
$expectedDependencyLockSHA256 = "45ca702f71dbd8fbf47855dcf8cdb86b13785a009b2c107153ba52e0b20157ad"
$expectedDependencyLockBlob = "d6e4b715db635497bfcac0db085163ea719d21ab"

$observerIdentityFunctionSource = @'
function Register-ObserverIdentities {
    param(
        [object]$Snapshot,
        [hashtable]$IdentityByPid,
        [hashtable]$OwnedIdentityToPid,
        [hashtable]$State
    )

    foreach ($record in @($Snapshot.ownedRecords)) {
        if ($null -eq $record) { continue }
        $processId = [int]$record.processId
        $identity = [string]$record.identity
        if ($IdentityByPid.ContainsKey($processId) -and [string]$IdentityByPid[$processId] -cne $identity) {
            throw "owned process PID $processId changed identity from $($IdentityByPid[$processId]) to $identity"
        }
        if ($OwnedIdentityToPid.ContainsKey($identity) -and [int]$OwnedIdentityToPid[$identity] -ne $processId) {
            throw "owned process identity $identity is ambiguous across PIDs"
        }
        $IdentityByPid[$processId] = $identity
        $OwnedIdentityToPid[$identity] = $processId
    }
    if ($Snapshot.rootPresent) {
        $identity = [string]$Snapshot.root.identity
        if ($null -eq $State["rootIdentity"]) {
            $State["rootIdentity"] = $identity
        } elseif ([string]$State["rootIdentity"] -cne $identity) {
            throw "owned root identity changed from $($State['rootIdentity']) to $identity"
        }
    }
}

function Remove-ObserverInactivePidIdentities {
    param(
        [object]$Snapshot,
        [hashtable]$IdentityByPid
    )

    # A PID may be reused after its prior owned process has disappeared
    # between intervals. Retire both an absent PID and a PID whose current
    # creation identity differs, but retain every identity in the historical
    # OwnedIdentityToPid ledger. Register-ObserverIdentities still compares
    # the before/after pair without retirement, so reuse during one TCP query
    # remains fail closed.
    $activeIdentities = @{}
    foreach ($record in @($Snapshot.ownedRecords)) {
        if ($null -ne $record) {
            $activeIdentities[[int]$record.processId] = [string]$record.identity
        }
    }
    foreach ($processId in @($IdentityByPid.Keys)) {
        $processId = [int]$processId
        if (-not $activeIdentities.ContainsKey($processId) -or [string]$IdentityByPid[$processId] -cne [string]$activeIdentities[$processId]) {
            [void]$IdentityByPid.Remove($processId)
        }
    }
}

function Get-ObserverConnectionTable {
    param([scriptblock]$Query)

    if ($null -eq $Query) {
        throw "TCP-table query is missing"
    }
    try {
        return @(& $Query)
    } catch {
        throw "TCP-table query failed: $($_.Exception.Message)"
    }
}

function Get-ObserverGitStatus {
    param([string]$CheckoutPath)

    if ([string]::IsNullOrWhiteSpace($CheckoutPath)) {
        return ""
    }
    $output = @(& git.exe -C $CheckoutPath status --porcelain=v1 --untracked-files=all 2>&1)
    if ($LASTEXITCODE -ne 0) {
        throw "release checkout Git status failed: $($output -join ' ')"
    }
    return (($output | ForEach-Object { [string]$_ }) -join "`n").Trim()
}
'@

function Get-ObserverGitStatus {
    param([string]$CheckoutPath)

    if ([string]::IsNullOrWhiteSpace($CheckoutPath)) {
        return ""
    }
    $output = @(& git.exe -C $CheckoutPath status --porcelain=v1 --untracked-files=all 2>&1)
    if ($LASTEXITCODE -ne 0) {
        throw "release checkout Git status failed: $($output -join ' ')"
    }
    return (($output | ForEach-Object { [string]$_ }) -join "`n").Trim()
}

function Fail-Smoke {
    param([string]$Message)
    throw "install smoke: $Message"
}

function Get-SmokeRootOutputEvidence {
    param(
        [string]$RawPath,
        [string]$RetainedPath,
        [int]$MaximumBytes
    )

    if ($MaximumBytes -le 0) {
        Fail-Smoke "root output maximum must be positive"
    }
    Ensure-SmokeFileHashCommand
    $rawItem = Get-Item -LiteralPath $RawPath -Force -ErrorAction SilentlyContinue
    if ($null -eq $rawItem -or $rawItem.PSIsContainer -or (($rawItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "root output evidence is missing or not a regular file: $RawPath"
    }
    $retainedPath = Resolve-SmokeAbsolutePath $RetainedPath
    $retainedParent = Split-Path -Parent $retainedPath
    if (-not (Test-Path -LiteralPath $retainedParent -PathType Container)) {
        Fail-Smoke "root output evidence parent does not exist: $retainedParent"
    }
    $rawBytes = [System.IO.File]::ReadAllBytes($RawPath)
    $totalBytes = [int64]$rawBytes.Length
    $capturedByteCount = [int][Math]::Min([int64]$MaximumBytes, $totalBytes)
    $capturedBytes = New-Object byte[] $capturedByteCount
    if ($capturedByteCount -gt 0) {
        [System.Array]::Copy($rawBytes, $capturedBytes, $capturedByteCount)
    }
    $text = [System.Text.Encoding]::UTF8.GetString($capturedBytes)
    $redactionPattern = '(?i)(token|password|secret|api[_-]?key|authorization)\s*([:=])\s*[^\s,;]+'
    $redactedBytes = [int64]0
    foreach ($match in [System.Text.RegularExpressions.Regex]::Matches($text, $redactionPattern)) {
        $value = [string]$match.Value
        $separatorIndex = $value.IndexOfAny([char[]]@(':', '='))
        if ($separatorIndex -ge 0 -and $separatorIndex + 1 -lt $value.Length) {
            $redactedValue = $value.Substring($separatorIndex + 1).Trim()
            $redactedBytes += [int64][System.Text.Encoding]::UTF8.GetByteCount($redactedValue)
        }
    }
    $redactedText = [System.Text.RegularExpressions.Regex]::Replace($text, $redactionPattern, '$1$2<redacted>')
    [System.IO.File]::WriteAllText($retainedPath, $redactedText, [System.Text.UTF8Encoding]::new($false))
    $retainedItem = Get-Item -LiteralPath $retainedPath -Force -ErrorAction Stop
    if ($retainedItem.PSIsContainer -or (($retainedItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "retained root output evidence is not a regular file: $retainedPath"
    }
    try {
        Remove-Item -LiteralPath $RawPath -Force -ErrorAction Stop
    } catch {
        Fail-Smoke "unredacted root output evidence could not be removed: $RawPath; $($_.Exception.Message)"
    }
    return [ordered]@{
        status = "PASS"
        path = $retainedPath
        present = $true
        totalBytes = $totalBytes
        capturedBytes = [int64]$capturedByteCount
        truncated = ($totalBytes -gt $MaximumBytes)
        redactedBytes = $redactedBytes
        sha256 = ((Get-FileHash -LiteralPath $retainedPath -Algorithm SHA256).Hash.ToLowerInvariant())
    }
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

function Invoke-SmokeGit {
    param(
        [string]$CheckoutPath,
        [string[]]$ArgumentList
    )

    $output = @(& git.exe -C $CheckoutPath @ArgumentList 2>&1)
    if ($LASTEXITCODE -ne 0) {
        Fail-Smoke "git -C $CheckoutPath $($ArgumentList -join ' ') failed: $($output -join ' ')"
    }
    return @($output | ForEach-Object { [string]$_ })
}

function Assert-SmokeReleaseCheckout {
    param(
        [string]$RequestedCheckoutPath,
        [switch]$PrepareDistExclude,
        [switch]$RequireDistExclude
    )

    $checkoutPath = Resolve-SmokeAbsolutePath $RequestedCheckoutPath
    $gitPath = Join-Path $checkoutPath ".git"
    $gitItem = Get-Item -LiteralPath $gitPath -Force -ErrorAction SilentlyContinue
    if ($null -eq $gitItem -or -not $gitItem.PSIsContainer) {
        Fail-Smoke "release checkout .git is not a real directory: $gitPath"
    }
    $topLevel = (Invoke-SmokeGit $checkoutPath @("rev-parse", "--show-toplevel") | Select-Object -Last 1).Trim()
    if ([System.IO.Path]::GetFullPath($topLevel) -ne $checkoutPath) {
        Fail-Smoke "release checkout top-level = $topLevel, want $checkoutPath"
    }
    $head = (Invoke-SmokeGit $checkoutPath @("rev-parse", "HEAD") | Select-Object -Last 1).Trim()
    if ($head -cne $expectedCandidateSourceCommit) {
        Fail-Smoke "release checkout HEAD = $head, want $expectedCandidateSourceCommit"
    }
    $tree = (Invoke-SmokeGit $checkoutPath @("rev-parse", "HEAD^{tree}") | Select-Object -Last 1).Trim()
    if ($tree -cne $expectedCandidateSourceTree) {
        Fail-Smoke "release checkout tree = $tree, want $expectedCandidateSourceTree"
    }
    $branch = (Invoke-SmokeGit $checkoutPath @("rev-parse", "--abbrev-ref", "HEAD") | Select-Object -Last 1).Trim()
    if ($branch -cne "HEAD") {
        Fail-Smoke "release checkout is not detached: branch=$branch"
    }
    $origin = (Invoke-SmokeGit $checkoutPath @("remote", "get-url", "origin") | Select-Object -Last 1).Trim()
    if ($origin -match "^(https?|ssh)://" -or $origin -match "^git@") {
        Fail-Smoke "release checkout origin is not local-filesystem-only: $origin"
    }
    $statusBefore = Get-ObserverGitStatus $checkoutPath
    if ($statusBefore -ne "") {
        Fail-Smoke "release checkout is dirty before output production: $statusBefore"
    }
    $excludePath = Join-Path $gitPath "info\exclude"
    if (-not (Test-Path -LiteralPath $excludePath -PathType Leaf)) {
        Fail-Smoke "release checkout private exclude is missing: $excludePath"
    }
    $beforeLines = @([System.IO.File]::ReadAllLines($excludePath))
    $beforeDistCount = @($beforeLines | Where-Object { $_ -ceq "/dist/" }).Count
    if ($PrepareDistExclude) {
        if ($beforeDistCount -ne 0) {
            Fail-Smoke "release checkout private exclude already contains /dist/"
        }
        $existingText = [System.IO.File]::ReadAllText($excludePath)
        $separator = if ($existingText.EndsWith("`n") -or $existingText.EndsWith("`r") -or $existingText.Length -eq 0) { "" } else { [Environment]::NewLine }
        [System.IO.File]::AppendAllText($excludePath, $separator + "/dist/" + [Environment]::NewLine, [System.Text.UTF8Encoding]::new($false))
    }
    $afterLines = @([System.IO.File]::ReadAllLines($excludePath))
    $afterDistCount = @($afterLines | Where-Object { $_ -ceq "/dist/" }).Count
    if ($RequireDistExclude -and $afterDistCount -ne 1) {
        Fail-Smoke "release checkout private exclude must contain exactly one /dist/ entry, found $afterDistCount"
    }
    if ($PrepareDistExclude) {
        if ($afterLines.Count -ne $beforeLines.Count + 1 -or $afterLines[$afterLines.Count - 1] -cne "/dist/") {
            Fail-Smoke "release checkout private exclude changed by more than the exact /dist/ addition"
        }
        for ($index = 0; $index -lt $beforeLines.Count; $index++) {
            if ($afterLines[$index] -cne $beforeLines[$index]) {
                Fail-Smoke "release checkout private exclude changed before the exact /dist/ addition"
            }
        }
    }
    $statusAfter = Get-ObserverGitStatus $checkoutPath
    if ($statusAfter -ne "") {
        Fail-Smoke "release checkout is dirty after private exclude preparation: $statusAfter"
    }
    $privateExcludeAdded = New-Object 'System.Collections.Generic.List[string]'
    if ($PrepareDistExclude) {
        [void]$privateExcludeAdded.Add("/dist/")
    }
    return [ordered]@{
        status = "PASS"
        gitDirIsDirectory = $true
        checkoutPath = $checkoutPath
        detachedHead = $true
        head = $head
        tree = $tree
        origin = $origin
        privateExcludePath = $excludePath
        privateExcludeBefore = $beforeLines
        privateExcludeAfter = $afterLines
        privateExcludeAdded = $privateExcludeAdded
        statusCleanBefore = ($statusBefore -eq "")
        statusCleanAfterPreparation = ($statusAfter -eq "")
    }
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

function Assert-SmokeNewEvidenceFile {
    param(
        [string]$Name,
        [string]$RequestedPath
    )

    $path = Resolve-SmokeAbsolutePath $RequestedPath
    $parent = Split-Path -Parent $path
    if (-not (Test-Path -LiteralPath $parent -PathType Container)) {
        Fail-Smoke "$Name parent does not exist: $parent"
    }
    $item = Get-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
    if ($null -ne $item) {
        Fail-Smoke "$Name already exists; prior evidence cannot be reused: $path"
    }
    return $path
}

function Get-SmokeSHA256 {
    param([string]$Path)

    $hasher = [System.Security.Cryptography.SHA256]::Create()
    $stream = [System.IO.File]::Open($Path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
    try {
        $digest = $hasher.ComputeHash($stream)
    } finally {
        $stream.Dispose()
        $hasher.Dispose()
    }
    return ([System.BitConverter]::ToString($digest).Replace("-", "").ToLowerInvariant())
}

function Get-SmokeRelativePath {
    param(
        [string]$RootPath,
        [string]$CandidatePath,
        [string]$Role = "path"
    )

    $root = [System.IO.Path]::GetFullPath($RootPath).TrimEnd([char[]]"\/")
    $candidate = [System.IO.Path]::GetFullPath($CandidatePath)
    if ($candidate.Equals($root, [System.StringComparison]::OrdinalIgnoreCase)) {
        return "."
    }
    $prefix = $root + [System.IO.Path]::DirectorySeparatorChar
    if (-not $candidate.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        Fail-Smoke "$Role resolves outside the contained root: $candidate"
    }
    return $candidate.Substring($prefix.Length).Replace([System.IO.Path]::DirectorySeparatorChar, '/')
}

function Get-SmokeDependencyLinkTarget {
    param(
        [object]$Item,
        [string]$ContainmentRoot
    )

    $targetProperty = $Item.PSObject.Properties["Target"]
    $targets = @(
        if ($null -eq $targetProperty) { @() } else { @($targetProperty.Value) }
    )
    if ($targets.Count -ne 1 -or [string]::IsNullOrWhiteSpace([string]$targets[0])) {
        Fail-Smoke "dependency link has ambiguous or missing target: $($Item.FullName)"
    }
    $target = [string]$targets[0]
    if (-not [System.IO.Path]::IsPathRooted($target)) {
        $target = Join-Path (Split-Path -Parent $Item.FullName) $target
    }
    $target = [System.IO.Path]::GetFullPath($target)
    $targetItem = Get-Item -LiteralPath $target -Force -ErrorAction SilentlyContinue
    if ($null -eq $targetItem) {
        Fail-Smoke "dependency link target is missing: $($Item.FullName) -> $target"
    }
    $targetRelative = Get-SmokeRelativePath -RootPath $ContainmentRoot -CandidatePath $target -Role "dependency link target"
    return [pscustomobject]@{
        absolute = $target
        relative = $targetRelative
    }
}

function Get-SmokeDependencyLinkItems {
    param([string]$RootPath)

    $rootItem = Get-Item -LiteralPath $RootPath -Force -ErrorAction Stop
    $pending = New-Object 'System.Collections.Generic.Queue[string]'
    $links = New-Object 'System.Collections.Generic.List[object]'
    $pending.Enqueue($rootItem.FullName)
    while ($pending.Count -gt 0) {
        $directory = $pending.Dequeue()
        foreach ($item in @(Get-ChildItem -LiteralPath $directory -Force -ErrorAction Stop)) {
            $isReparse = (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)
            if ($isReparse) {
                [void]$links.Add($item)
                continue
            }
            if ($item.PSIsContainer) {
                $pending.Enqueue($item.FullName)
            }
        }
    }
    return @($links.ToArray())
}

function Get-SmokeDependencyEntries {
    param(
        [string]$RootPath,
        [string]$ContainmentRoot
    )

    $rootItem = Get-Item -LiteralPath $RootPath -Force -ErrorAction Stop
    if (-not $rootItem.PSIsContainer -or (($rootItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "dependency root is not a regular directory: $RootPath"
    }
    $rootFullPath = [System.IO.Path]::GetFullPath($rootItem.FullName)
    $containmentFullPath = [System.IO.Path]::GetFullPath($ContainmentRoot)
    $entries = New-Object 'System.Collections.Generic.List[object]'
    [void]$entries.Add([ordered]@{
            path = "."
            type = "directory"
            bytes = [int64]0
            sha256 = ""
            target = $null
        })
    $pending = New-Object 'System.Collections.Generic.Queue[string]'
    $pending.Enqueue($rootFullPath)
    while ($pending.Count -gt 0) {
        $directory = $pending.Dequeue()
        foreach ($item in @(Get-ChildItem -LiteralPath $directory -Force -ErrorAction Stop)) {
            $relative = Get-SmokeRelativePath -RootPath $rootFullPath -CandidatePath $item.FullName -Role "dependency entry"
            $isReparse = (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)
            if ($isReparse) {
                $linkTypeProperty = $item.PSObject.Properties["LinkType"]
                $linkType = if ($null -eq $linkTypeProperty) { "" } else { [string]$linkTypeProperty.Value }
                $link = Get-SmokeDependencyLinkTarget -Item $item -ContainmentRoot $containmentFullPath
                $type = if ($linkType -eq "Junction") { "junction" } elseif ($linkType -eq "SymbolicLink") { "symlink" } else { "" }
                if ([string]::IsNullOrWhiteSpace($type)) {
                    Fail-Smoke "dependency entry has unsupported link type '$linkType': $($item.FullName)"
                }
                [void]$entries.Add([ordered]@{
                        path = $relative
                        type = $type
                        bytes = [int64]0
                        sha256 = ""
                        target = $link.relative
                    })
                continue
            }
            if ($item.PSIsContainer) {
                [void]$entries.Add([ordered]@{
                        path = $relative
                        type = "directory"
                        bytes = [int64]0
                        sha256 = ""
                        target = $null
                    })
                $pending.Enqueue($item.FullName)
                continue
            }
            $fileBytes = [int64]$item.Length
            [void]$entries.Add([ordered]@{
                    path = $relative
                    type = "file"
                    bytes = $fileBytes
                    sha256 = (Get-SmokeSHA256 -Path $item.FullName)
                    target = $null
                })
        }
    }
    return @($entries.ToArray())
}

function New-SmokeDependencyManifest {
    param(
        [string]$RootPath,
        [string]$ContainmentRoot,
        [string]$ManifestPath
    )

    $manifestPath = Assert-SmokeNewEvidenceFile -Name "dependency manifest" -RequestedPath $ManifestPath
    $entries = @(Get-SmokeDependencyEntries -RootPath $RootPath -ContainmentRoot $ContainmentRoot)
    $sortedEntries = @($entries | Sort-Object -Property path)
    $writer = [System.IO.StreamWriter]::new($manifestPath, $false, [System.Text.UTF8Encoding]::new($false))
    try {
        foreach ($entry in $sortedEntries) {
            $writer.WriteLine(($entry | ConvertTo-Json -Compress -Depth 5))
        }
    } finally {
        $writer.Dispose()
    }
    $fileCount = [int64]0
    $directoryCount = [int64]0
    $junctionCount = [int64]0
    $symbolicLinkCount = [int64]0
    $totalBytes = [int64]0
    foreach ($entry in $sortedEntries) {
        switch ([string]$entry.type) {
            "file" { $fileCount++; $totalBytes += [int64]$entry.bytes }
            "directory" { $directoryCount++ }
            "junction" { $junctionCount++ }
            "symlink" { $symbolicLinkCount++ }
        }
    }
    return [ordered]@{
        status = "PASS"
        manifestPath = $manifestPath
        manifestSHA256 = Get-SmokeSHA256 -Path $manifestPath
        entryCount = [int64]$sortedEntries.Count
        fileCount = $fileCount
        directoryCount = $directoryCount
        junctionCount = $junctionCount
        symbolicLinkCount = $symbolicLinkCount
        totalBytes = $totalBytes
    }
}

function Read-SmokeStructuredJSON {
    param([string]$Path)

    $raw = [System.IO.File]::ReadAllText($Path)
    $normalized = [System.Text.RegularExpressions.Regex]::Replace($raw, ',(?=\s*[}\]])', '')
    try {
        [void][System.Reflection.Assembly]::LoadWithPartialName("System.Web.Extensions")
        $serializer = [System.Web.Script.Serialization.JavaScriptSerializer]::new()
        return $serializer.DeserializeObject($normalized)
    } catch {
        Fail-Smoke "dependency JSON could not be parsed: $Path; $($_.Exception.Message)"
    }
}

function Get-SmokeMapValue {
    param(
        [object]$Map,
        [string]$Name
    )

    if ($null -eq $Map) { return $null }
    if ($Map -is [System.Collections.IDictionary]) {
        if ($Map.ContainsKey($Name)) { return $Map[$Name] }
        return $null
    }
    $property = $Map.PSObject.Properties[$Name]
    if ($null -eq $property) { return $null }
    return $property.Value
}

function Get-SmokeMapEntries {
    param([object]$Map)

    if ($null -eq $Map -or -not ($Map -is [System.Collections.IDictionary])) { return @() }
    return @($Map.GetEnumerator())
}

function Assert-SmokeDependencyMapsMatch {
    param(
        [object]$ManifestMap,
        [object]$LockMap,
        [string]$WorkspaceKey,
        [string]$GroupName
    )

    $manifestEntries = @(Get-SmokeMapEntries $ManifestMap)
    $lockEntries = @(Get-SmokeMapEntries $LockMap)
    foreach ($entry in $manifestEntries) {
        $lockValue = Get-SmokeMapValue -Map $LockMap -Name ([string]$entry.Key)
        if ($null -eq $lockValue -or [string]$lockValue -cne [string]$entry.Value) {
            Fail-Smoke "lock workspace '$WorkspaceKey' $GroupName drift for '$($entry.Key)'"
        }
    }
    foreach ($entry in $lockEntries) {
        $manifestValue = Get-SmokeMapValue -Map $ManifestMap -Name ([string]$entry.Key)
        if ($null -eq $manifestValue) {
            Fail-Smoke "lock workspace '$WorkspaceKey' has undeclared $GroupName dependency '$($entry.Key)'"
        }
    }
}

function Resolve-SmokeNodePackageManifest {
    param(
        [string]$DependencyName,
        [string]$RequesterDirectory,
        [string]$UIRoot
    )

    $current = [System.IO.Path]::GetFullPath($RequesterDirectory)
    $uiFullPath = [System.IO.Path]::GetFullPath($UIRoot)
    while ($true) {
        $candidateDirectory = Join-Path (Join-Path $current "node_modules") $DependencyName
        $candidateManifest = Join-Path $candidateDirectory "package.json"
        $manifestItem = Get-Item -LiteralPath $candidateManifest -Force -ErrorAction SilentlyContinue
        if ($null -ne $manifestItem -and -not $manifestItem.PSIsContainer -and (($manifestItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -eq 0)) {
            return $manifestItem.FullName
        }
        if ($current.Equals($uiFullPath, [System.StringComparison]::OrdinalIgnoreCase)) { break }
        if (-not ($current.StartsWith($uiFullPath + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase))) { break }
        $parent = [System.IO.DirectoryInfo]$current
        $parent = $parent.Parent
        if ($null -eq $parent) { break }
        $current = $parent.FullName
    }
    return $null
}

function Get-SmokeDependencyLockClosure {
    param(
        [string]$DependencyRoot,
        [string]$ExpectedLockSHA256,
        [string]$ExpectedLockBlob
    )

    $rootPath = Resolve-SmokeAbsolutePath $DependencyRoot
    $uiRoot = Split-Path -Parent $rootPath
    $lockPath = Join-Path $uiRoot "bun.lock"
    $lockItem = Get-Item -LiteralPath $lockPath -Force -ErrorAction SilentlyContinue
    if ($null -eq $lockItem -or $lockItem.PSIsContainer -or (($lockItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "dependency lock is missing or not a regular file: $lockPath"
    }
    $lockSHA256 = Get-SmokeSHA256 -Path $lockPath
    if ($lockSHA256 -cne $ExpectedLockSHA256.ToLowerInvariant()) {
        Fail-Smoke "dependency lock SHA-256 = $lockSHA256, want $ExpectedLockSHA256"
    }
    $lockBlob = (Invoke-SmokeGit $uiRoot @("rev-parse", "HEAD:ui/bun.lock") | Select-Object -Last 1).Trim()
    if ($lockBlob -cne $ExpectedLockBlob.ToLowerInvariant()) {
        Fail-Smoke "dependency lock Git blob = $lockBlob, want $ExpectedLockBlob"
    }
    $lock = Read-SmokeStructuredJSON $lockPath
    $lockfileVersion = Get-SmokeMapValue -Map $lock -Name "lockfileVersion"
    if ([int]$lockfileVersion -ne 1) {
        Fail-Smoke "dependency lockfileVersion = $lockfileVersion, want 1"
    }
    $workspaces = Get-SmokeMapValue -Map $lock -Name "workspaces"
    $packages = Get-SmokeMapValue -Map $lock -Name "packages"
    $workspaceEntries = @(Get-SmokeMapEntries $workspaces)
    $packageEntries = @(Get-SmokeMapEntries $packages)
    if ($workspaceEntries.Count -eq 0 -or $packageEntries.Count -eq 0) {
        Fail-Smoke "dependency lock has no workspace/package closure"
    }

    $workspaceManifestPaths = New-Object 'System.Collections.Generic.List[string]'
    $rootManifestPath = Join-Path $uiRoot "package.json"
    if (-not (Test-Path -LiteralPath $rootManifestPath -PathType Leaf)) {
        Fail-Smoke "workspace root package manifest is missing: $rootManifestPath"
    }
    [void]$workspaceManifestPaths.Add($rootManifestPath)
    $workspacePackagesRoot = Join-Path $uiRoot "packages"
    if (-not (Test-Path -LiteralPath $workspacePackagesRoot -PathType Container)) {
        Fail-Smoke "workspace packages root is missing: $workspacePackagesRoot"
    }
    foreach ($workspaceDirectory in @(Get-ChildItem -LiteralPath $workspacePackagesRoot -Directory -Force -ErrorAction Stop)) {
        $workspaceManifestPath = Join-Path $workspaceDirectory.FullName "package.json"
        if (Test-Path -LiteralPath $workspaceManifestPath -PathType Leaf) {
            [void]$workspaceManifestPaths.Add($workspaceManifestPath)
        }
    }
    $workspaceByName = @{}
    $workspaceByPath = @{}
    $workspaceRecords = New-Object 'System.Collections.Generic.List[object]'
    foreach ($workspaceManifestPath in @($workspaceManifestPaths.ToArray())) {
        $manifest = Read-SmokeStructuredJSON $workspaceManifestPath
        $workspaceName = [string](Get-SmokeMapValue -Map $manifest -Name "name")
        if ([string]::IsNullOrWhiteSpace($workspaceName)) {
            Fail-Smoke "workspace package name is missing: $workspaceManifestPath"
        }
        $workspaceDirectory = Split-Path -Parent $workspaceManifestPath
        $workspaceKey = Get-SmokeRelativePath -RootPath $uiRoot -CandidatePath $workspaceDirectory -Role "workspace"
        if ($workspaceKey -eq ".") { $workspaceKey = "" }
        $lockWorkspace = Get-SmokeMapValue -Map $workspaces -Name $workspaceKey
        if ($null -eq $lockWorkspace) {
            Fail-Smoke "lock workspace '$workspaceKey' is missing"
        }
        foreach ($groupName in @("dependencies", "devDependencies", "optionalDependencies")) {
            Assert-SmokeDependencyMapsMatch -ManifestMap (Get-SmokeMapValue -Map $manifest -Name $groupName) -LockMap (Get-SmokeMapValue -Map $lockWorkspace -Name $groupName) -WorkspaceKey $workspaceKey -GroupName $groupName
        }
        if ($workspaceByName.ContainsKey($workspaceName)) {
            Fail-Smoke "duplicate workspace package name '$workspaceName'"
        }
        $workspaceByName[$workspaceName] = $workspaceManifestPath
        $workspaceByPath[$workspaceKey] = $workspaceManifestPath
        [void]$workspaceRecords.Add([pscustomobject]@{ key = $workspaceKey; name = $workspaceName; manifestPath = $workspaceManifestPath })
    }
    $expectedWorkspaceKeys = @($workspaceByPath.Keys | Sort-Object)
    $actualWorkspaceKeys = @($workspaces.Keys | ForEach-Object { [string]$_ } | Sort-Object)
    if (-not ([System.Linq.Enumerable]::SequenceEqual([string[]]$expectedWorkspaceKeys, [string[]]$actualWorkspaceKeys))) {
        Fail-Smoke "lock workspace set differs from package manifests"
    }

    $queue = New-Object 'System.Collections.Generic.Queue[string]'
    foreach ($record in @($workspaceRecords.ToArray())) { $queue.Enqueue($record.manifestPath) }
    $visited = @{}
    $optionalMissing = New-Object 'System.Collections.Generic.List[string]'
    $reachablePackageCount = [int64]0
    while ($queue.Count -gt 0) {
        $manifestPath = $queue.Dequeue()
        $manifestKey = [System.IO.Path]::GetFullPath($manifestPath).ToLowerInvariant()
        if ($visited.ContainsKey($manifestKey)) { continue }
        $visited[$manifestKey] = $true
        $packageManifest = Read-SmokeStructuredJSON $manifestPath
        $packageName = [string](Get-SmokeMapValue -Map $packageManifest -Name "name")
        if ([string]::IsNullOrWhiteSpace($packageName)) {
            Fail-Smoke "reachable package name is missing: $manifestPath"
        }
        $reachablePackageCount++
        $requesterDirectory = Split-Path -Parent $manifestPath
        foreach ($groupName in @("dependencies", "optionalDependencies", "peerDependencies")) {
            $dependencyMap = Get-SmokeMapValue -Map $packageManifest -Name $groupName
            foreach ($dependency in @(Get-SmokeMapEntries $dependencyMap)) {
                $dependencyName = [string]$dependency.Key
                $dependencySpec = [string]$dependency.Value
                $resolvedManifest = $null
                if ($dependencySpec -like "workspace:*") {
                    if (-not $workspaceByName.ContainsKey($dependencyName)) {
                        Fail-Smoke "workspace dependency '$dependencyName' from '$packageName' is not declared"
                    }
                    $resolvedManifest = $workspaceByName[$dependencyName]
                } else {
                    $resolvedManifest = Resolve-SmokeNodePackageManifest -DependencyName $dependencyName -RequesterDirectory $requesterDirectory -UIRoot $uiRoot
                }
                if ([string]::IsNullOrWhiteSpace([string]$resolvedManifest)) {
                    if ($groupName -eq "optionalDependencies") {
                        [void]$optionalMissing.Add("$packageName->$dependencyName")
                        continue
                    }
                    Fail-Smoke "dependency closure is missing required package '$dependencyName' from '$packageName'"
                }
                $resolvedItem = Get-Item -LiteralPath $resolvedManifest -Force -ErrorAction Stop
                [void](Get-SmokeRelativePath -RootPath $uiRoot -CandidatePath $resolvedItem.FullName -Role "resolved dependency")
                $queue.Enqueue($resolvedItem.FullName)
            }
        }
    }
    return [ordered]@{
        status = "PASS"
        lockPath = $lockPath
        lockSHA256 = $lockSHA256
        lockBlob = $lockBlob
        lockfileVersion = [int]$lockfileVersion
        workspaceCount = [int64]$workspaceEntries.Count
        lockPackageCount = [int64]$packageEntries.Count
        reachablePackageCount = $reachablePackageCount
        optionalMissingCount = [int64]$optionalMissing.Count
        optionalMissing = @($optionalMissing.ToArray())
    }
}

function Copy-SmokeDependencyTree {
    param(
        [string]$SourceRoot,
        [string]$StageRoot,
        [string]$SourceContainmentRoot,
        [string]$StageContainmentRoot
    )

    $sourcePath = Resolve-SmokeAbsolutePath $SourceRoot
    $stagePath = Resolve-SmokeAbsolutePath $StageRoot
    $sourceContainmentPath = Resolve-SmokeAbsolutePath $SourceContainmentRoot
    $stageContainmentPath = Resolve-SmokeAbsolutePath $StageContainmentRoot
    $sourceItem = Get-Item -LiteralPath $sourcePath -Force -ErrorAction Stop
    if (-not $sourceItem.PSIsContainer -or (($sourceItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
        Fail-Smoke "dependency source root is not a regular directory: $sourcePath"
    }
    Assert-SmokeDisposableRoot -Name "dependency stage" -Path $stagePath
    if (-not (Test-Path -LiteralPath $stagePath -PathType Container)) {
        [void][System.IO.Directory]::CreateDirectory($stagePath)
    }
    $robocopyArguments = @(
        $sourcePath, $stagePath,
        "/E", "/XJ", "/COPY:DAT", "/DCOPY:DAT", "/R:0", "/W:0",
        "/NFL", "/NDL", "/NJH", "/NJS", "/NP"
    )
    $robocopyOutput = @(& robocopy.exe @robocopyArguments 2>&1)
    $robocopyExitCode = [int]$LASTEXITCODE
    if ($robocopyExitCode -gt 7) {
        $diagnostic = @($robocopyOutput | ForEach-Object { [string]$_ } | Select-Object -Last 20) -join " | "
        Fail-Smoke "exact dependency copy failed with robocopy exit code ${robocopyExitCode}: $diagnostic"
    }

    $rewrittenLinks = [int64]0
    foreach ($sourceLink in @(Get-SmokeDependencyLinkItems -RootPath $sourcePath)) {
        $relative = Get-SmokeRelativePath -RootPath $sourcePath -CandidatePath $sourceLink.FullName -Role "dependency link"
        $stageLinkPath = Join-Path $stagePath ($relative.Replace('/', [System.IO.Path]::DirectorySeparatorChar))
        $sourceLinkTarget = Get-SmokeDependencyLinkTarget -Item $sourceLink -ContainmentRoot $sourceContainmentPath
        $stageTargetPath = Join-Path $stageContainmentPath ($sourceLinkTarget.relative.Replace('/', [System.IO.Path]::DirectorySeparatorChar))
        if (-not (Test-Path -LiteralPath $stageTargetPath)) {
            Fail-Smoke "staged dependency link target is missing: $stageLinkPath -> $stageTargetPath"
        }
        $stageLinkItem = Get-Item -LiteralPath $stageLinkPath -Force -ErrorAction SilentlyContinue
        if ($null -ne $stageLinkItem -and (($stageLinkItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -eq 0)) {
            Fail-Smoke "dependency copier followed a source reparse point: $stageLinkPath"
        }
        if ($null -ne $stageLinkItem) {
            Remove-Item -LiteralPath $stageLinkPath -Force -ErrorAction Stop
        }
        $stageLinkParent = Split-Path -Parent $stageLinkPath
        if (-not (Test-Path -LiteralPath $stageLinkParent -PathType Container)) {
            [void][System.IO.Directory]::CreateDirectory($stageLinkParent)
        }
        $linkTypeProperty = $sourceLink.PSObject.Properties["LinkType"]
        $linkType = if ($null -eq $linkTypeProperty) { "" } else { [string]$linkTypeProperty.Value }
        if ($linkType -eq "Junction") {
            [void](New-Item -ItemType Junction -Path $stageLinkPath -Target $stageTargetPath -ErrorAction Stop)
        } elseif ($linkType -eq "SymbolicLink") {
            [void](New-Item -ItemType SymbolicLink -Path $stageLinkPath -Target $stageTargetPath -ErrorAction Stop)
        } else {
            Fail-Smoke "dependency copier cannot reproduce link type '$linkType': $($sourceLink.FullName)"
        }
        $rewrittenLinks++
    }
    return [ordered]@{
        status = "PASS"
        robocopyExitCode = $robocopyExitCode
        rewrittenLinks = $rewrittenLinks
        outputLineCount = [int64]$robocopyOutput.Count
    }
}

function Write-SmokeJSONEvidence {
    param(
        [string]$RequestedPath,
        [object]$Evidence
    )

    $path = Assert-SmokeNewEvidenceFile -Name "dependency report" -RequestedPath $RequestedPath
    [System.IO.File]::WriteAllText($path, ($Evidence | ConvertTo-Json -Depth 20), [System.Text.UTF8Encoding]::new($false))
    return $path
}

function Invoke-SmokeDependencyStage {
    param(
        [string]$RequestedSourceRoot,
        [string]$RequestedStageRoot,
        [string]$RequestedSourceManifestPath,
        [string]$RequestedStageManifestPath,
        [string]$RequestedReportPath,
        [string]$ExpectedLockSHA256,
        [string]$ExpectedLockBlob
    )

    $reportPath = Resolve-SmokeAbsolutePath $RequestedReportPath
    $report = [ordered]@{
        schemaVersion = "localai-windows-install-candidate-dependency-stage/v1"
        status = "FAIL"
        phase = "stage"
        sourceRoot = Resolve-SmokeAbsolutePath $RequestedSourceRoot
        stageRoot = Resolve-SmokeAbsolutePath $RequestedStageRoot
        packageInstallation = "NOT_RUN"
        sourceManifest = $null
        stageManifest = $null
        sourceLockClosure = $null
        stageLockClosure = $null
        copy = $null
        equality = $null
        error = ""
    }
    try {
        $sourceRoot = $report.sourceRoot
        $stageRoot = $report.stageRoot
        $sourceContainmentRoot = Split-Path -Parent $sourceRoot
        $stageContainmentRoot = Split-Path -Parent $stageRoot
        $sourceManifestPath = Assert-SmokeNewEvidenceFile -Name "source dependency manifest" -RequestedPath $RequestedSourceManifestPath
        $stageManifestPath = Assert-SmokeNewEvidenceFile -Name "staged dependency manifest" -RequestedPath $RequestedStageManifestPath
        $sourceClosure = Get-SmokeDependencyLockClosure -DependencyRoot $sourceRoot -ExpectedLockSHA256 $ExpectedLockSHA256 -ExpectedLockBlob $ExpectedLockBlob
        $sourceManifest = New-SmokeDependencyManifest -RootPath $sourceRoot -ContainmentRoot $sourceContainmentRoot -ManifestPath $sourceManifestPath
        $copy = Copy-SmokeDependencyTree -SourceRoot $sourceRoot -StageRoot $stageRoot -SourceContainmentRoot $sourceContainmentRoot -StageContainmentRoot $stageContainmentRoot
        $stageClosure = Get-SmokeDependencyLockClosure -DependencyRoot $stageRoot -ExpectedLockSHA256 $ExpectedLockSHA256 -ExpectedLockBlob $ExpectedLockBlob
        $stageManifest = New-SmokeDependencyManifest -RootPath $stageRoot -ContainmentRoot $stageContainmentRoot -ManifestPath $stageManifestPath
        $manifestEqual = $sourceManifest.manifestSHA256 -ceq $stageManifest.manifestSHA256
        if (-not $manifestEqual) {
            Fail-Smoke "source and staged dependency manifests differ"
        }
        $report.sourceLockClosure = $sourceClosure
        $report.stageLockClosure = $stageClosure
        $report.sourceManifest = $sourceManifest
        $report.stageManifest = $stageManifest
        $report.copy = $copy
        $report.equality = [ordered]@{
            sourceStageManifestEqual = $manifestEqual
            sourceEntryCount = $sourceManifest.entryCount
            stageEntryCount = $stageManifest.entryCount
            sourceTotalBytes = $sourceManifest.totalBytes
            stageTotalBytes = $stageManifest.totalBytes
        }
        $report.status = "PASS"
    } catch {
        $report.error = "line $($_.InvocationInfo.ScriptLineNumber): $($_.Exception.Message)"
    }
    Write-SmokeJSONEvidence -RequestedPath $reportPath -Evidence $report | Out-Null
    if ($report.status -ne "PASS") {
        throw "dependency staging failed; report=$reportPath"
    }
    Write-Output "dependency staging passed; report=$reportPath"
}

function Invoke-SmokeDependencyVerification {
    param(
        [string]$RequestedSourceRoot,
        [string]$RequestedStageRoot,
        [string]$RequestedExpectedManifestPath,
        [string]$RequestedPostBuildManifestPath,
        [string]$RequestedSourceAfterManifestPath,
        [string]$RequestedReportPath,
        [string]$ExpectedLockSHA256,
        [string]$ExpectedLockBlob
    )

    $reportPath = Resolve-SmokeAbsolutePath $RequestedReportPath
    $report = [ordered]@{
        schemaVersion = "localai-windows-install-candidate-dependency-stage/v1"
        status = "FAIL"
        phase = "post-build"
        sourceRoot = Resolve-SmokeAbsolutePath $RequestedSourceRoot
        stageRoot = Resolve-SmokeAbsolutePath $RequestedStageRoot
        packageInstallation = "NOT_RUN"
        expectedManifest = $null
        postBuildManifest = $null
        sourceAfterBuildManifest = $null
        sourceLockClosure = $null
        stageLockClosure = $null
        equality = $null
        error = ""
    }
    try {
        $sourceRoot = $report.sourceRoot
        $stageRoot = $report.stageRoot
        $sourceContainmentRoot = Split-Path -Parent $sourceRoot
        $stageContainmentRoot = Split-Path -Parent $stageRoot
        $expectedManifestPath = Resolve-SmokeAbsolutePath $RequestedExpectedManifestPath
        if (-not (Test-Path -LiteralPath $expectedManifestPath -PathType Leaf)) {
            Fail-Smoke "expected dependency manifest is missing: $expectedManifestPath"
        }
        $postBuildManifestPath = Assert-SmokeNewEvidenceFile -Name "post-build dependency manifest" -RequestedPath $RequestedPostBuildManifestPath
        $sourceAfterManifestPath = Assert-SmokeNewEvidenceFile -Name "source-after-build dependency manifest" -RequestedPath $RequestedSourceAfterManifestPath
        $sourceClosure = Get-SmokeDependencyLockClosure -DependencyRoot $sourceRoot -ExpectedLockSHA256 $ExpectedLockSHA256 -ExpectedLockBlob $ExpectedLockBlob
        $stageClosure = Get-SmokeDependencyLockClosure -DependencyRoot $stageRoot -ExpectedLockSHA256 $ExpectedLockSHA256 -ExpectedLockBlob $ExpectedLockBlob
        $postBuildManifest = New-SmokeDependencyManifest -RootPath $stageRoot -ContainmentRoot $stageContainmentRoot -ManifestPath $postBuildManifestPath
        $sourceAfterManifest = New-SmokeDependencyManifest -RootPath $sourceRoot -ContainmentRoot $sourceContainmentRoot -ManifestPath $sourceAfterManifestPath
        $expectedSHA256 = Get-SmokeSHA256 -Path $expectedManifestPath
        $postBuildEqual = $expectedSHA256 -ceq $postBuildManifest.manifestSHA256
        $sourceUnchanged = $expectedSHA256 -ceq $sourceAfterManifest.manifestSHA256
        if (-not $postBuildEqual) { Fail-Smoke "post-build dependency manifest differs from staged manifest" }
        if (-not $sourceUnchanged) { Fail-Smoke "source dependency manifest changed during build" }
        $report.expectedManifest = [ordered]@{ path = $expectedManifestPath; manifestSHA256 = $expectedSHA256 }
        $report.postBuildManifest = $postBuildManifest
        $report.sourceAfterBuildManifest = $sourceAfterManifest
        $report.sourceLockClosure = $sourceClosure
        $report.stageLockClosure = $stageClosure
        $report.equality = [ordered]@{
            sourceStagePostBuildEqual = ($expectedSHA256 -ceq $postBuildManifest.manifestSHA256 -and $expectedSHA256 -ceq $sourceAfterManifest.manifestSHA256)
            expectedManifestSHA256 = $expectedSHA256
            postBuildManifestSHA256 = $postBuildManifest.manifestSHA256
            sourceAfterBuildManifestSHA256 = $sourceAfterManifest.manifestSHA256
        }
        $report.status = "PASS"
    } catch {
        $report.error = "line $($_.InvocationInfo.ScriptLineNumber): $($_.Exception.Message)"
    }
    Write-SmokeJSONEvidence -RequestedPath $reportPath -Evidence $report | Out-Null
    if ($report.status -ne "PASS") {
        throw "dependency post-build verification failed; report=$reportPath"
    }
    Write-Output "dependency post-build verification passed; report=$reportPath"
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

function Get-SmokeExecutableBuildInfo {
    param(
        [string]$ExecutablePath,
        [string]$GoExecutablePath,
        [string]$WorkingDirectory
    )

    if ([string]::IsNullOrWhiteSpace($GoExecutablePath)) {
        Fail-Smoke "candidate mode requires the Go tool to inspect executable build info"
    }
    $result = Invoke-SmokeCommand -FilePath $GoExecutablePath -Arguments @("version", "-m", $ExecutablePath) -WorkingDirectory $WorkingDirectory
    if ($result.exitCode -ne 0) {
        Fail-Smoke "go version -m failed for candidate executable with exit code $($result.exitCode): $($result.stderr.Trim())"
    }

    $settings = @{}
    foreach ($line in @($result.stdout -split "`r?`n")) {
        $columns = $line -split "`t", 2
        if ($columns.Count -ne 2 -or $columns[0] -ne "build") {
            continue
        }
        $setting = $columns[1] -split "=", 2
        if ($setting.Count -ne 2 -or $setting[0] -notin @("vcs.revision", "vcs.modified")) {
            continue
        }
        $key = [string]$setting[0]
        if ($settings.ContainsKey($key)) {
            Fail-Smoke "candidate executable build info contains duplicate '$key' settings"
        }
        $settings[$key] = [string]$setting[1].Trim()
    }

    foreach ($key in @("vcs.revision", "vcs.modified")) {
        if (-not $settings.ContainsKey($key) -or [string]::IsNullOrWhiteSpace([string]$settings[$key])) {
            Fail-Smoke "candidate executable build info $key is missing"
        }
    }
    if ([string]$settings["vcs.revision"] -cne $expectedCandidateSourceCommit) {
        Fail-Smoke "candidate executable build info vcs.revision = $($settings['vcs.revision']), want $expectedCandidateSourceCommit"
    }
    if ([string]$settings["vcs.modified"] -cne "false") {
        Fail-Smoke "candidate executable build info vcs.modified = $($settings['vcs.modified']), want false"
    }

    return [pscustomobject]@{
        sourceRevision = [string]$settings["vcs.revision"]
        vcsModified = $false
        result = $result
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

    $startInfo = New-SmokeProcessStartInfo -FilePath $FilePath -Arguments $Arguments -WorkingDirectory $WorkingDirectory

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

function Convert-SmokeProcessArgumentListToCommandLine {
    param([string[]]$Arguments)

    $quotedArguments = New-Object 'System.Collections.Generic.List[string]'
    foreach ($argument in @($Arguments)) {
        $value = if ($null -eq $argument) { "" } else { [string]$argument }
        $builder = [System.Text.StringBuilder]::new()
        [void]$builder.Append('"')
        $backslashCount = 0
        foreach ($character in $value.ToCharArray()) {
            if ($character -eq '\') {
                $backslashCount++
                continue
            }
            if ($character -eq '"') {
                if ($backslashCount -gt 0) { [void]$builder.Append(('\' * ($backslashCount * 2))) }
                [void]$builder.Append('\"')
                $backslashCount = 0
                continue
            }
            if ($backslashCount -gt 0) { [void]$builder.Append(('\' * $backslashCount)); $backslashCount = 0 }
            [void]$builder.Append($character)
        }
        if ($backslashCount -gt 0) { [void]$builder.Append(('\' * ($backslashCount * 2))) }
        [void]$builder.Append('"')
        [void]$quotedArguments.Add($builder.ToString())
    }
    return [string]::Join(" ", $quotedArguments.ToArray())
}

function New-SmokeProcessStartInfo {
    param(
        [string]$FilePath,
        [string[]]$Arguments,
        [string]$WorkingDirectory
    )

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $FilePath
    $argumentListProperty = $startInfo.PSObject.Properties["ArgumentList"]
    if ($null -ne $argumentListProperty) {
        foreach ($argument in @($Arguments)) {
            $value = if ($null -eq $argument) { "" } else { [string]$argument }
            [void]$startInfo.ArgumentList.Add($value)
        }
    } else {
        # Windows PowerShell 5.1 uses .NET Framework, whose ProcessStartInfo
        # has no ArgumentList. The quoting routine preserves the same token
        # boundary without invoking a shell or reparsing a serialized list.
        $startInfo.Arguments = Convert-SmokeProcessArgumentListToCommandLine $Arguments
    }
    $startInfo.WorkingDirectory = $WorkingDirectory
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    return $startInfo
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
    $executablePath = $null
    $digestPath = $null
    $archiveEvidence = $null
    $installerEvidence = $null
    $executableEvidence = $null
    $manifestEvidence = $null
    $goExecutablePath = $null
    $commandEvidence = @()
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
        $goCommand = Get-Command go.exe -CommandType Application -ErrorAction SilentlyContinue
        if ($null -eq $goCommand) {
            Fail-Smoke "candidate mode requires Go to inspect executable build info"
        }
        $goExecutablePath = [System.IO.Path]::GetFullPath($goCommand.Source)
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
        if ($manifest.project -ne "localai" -or $manifest.cycle -ne $expectedCandidateCycle) {
            Fail-Smoke "candidate project/cycle does not identify LocalAI cycle $expectedCandidateCycle"
        }
        if ([string]$manifest.source.repository -cne $expectedCandidateSourceRepository -or [string]$manifest.source.commit -cne $expectedCandidateSourceCommit -or [string]$manifest.source.tree -cne $expectedCandidateSourceTree) {
            Fail-Smoke "candidate source identity = repository=$($manifest.source.repository) commit=$($manifest.source.commit) tree=$($manifest.source.tree), want repository=$expectedCandidateSourceRepository commit=$expectedCandidateSourceCommit tree=$expectedCandidateSourceTree"
        }
        $version = [string]$manifest.build.candidateVersion
        if ([string]::IsNullOrWhiteSpace($version) -or $version -match '[\\/:*?"<>|\s]') {
            Fail-Smoke "candidate version is not a usable release version: $version"
        }
        if ([string]::IsNullOrWhiteSpace([string]$manifest.build.cliVersion) -or $manifest.build.goVersion -ne $expectedCandidateGoVersion -or [string]$manifest.build.goreleaserConfigSha256 -notmatch '^[0-9a-f]{64}$' -or $manifest.build.goreleaserVersion -ne "v2.12.7" -or $manifest.build.target.goos -ne "windows" -or $manifest.build.target.goarch -ne "amd64" -or $manifest.build.target.cgoEnabled -ne $false) {
            Fail-Smoke "candidate build identity is not Go $expectedCandidateGoVersion with GoReleaser v2.12.7, windows/amd64 and cgo disabled"
        }
        $requiredEnvironment = [ordered]@{
            GOSUMDB = "off"
            GOTOOLCHAIN = "go1.26.8"
            npm_config_offline = "true"
            GOFLAGS = "-p=4"
            GOMAXPROCS = "4"
        }
        foreach ($environment in $requiredEnvironment.GetEnumerator()) {
            if ([string]$manifest.build.environment.($environment.Key) -ne [string]$environment.Value) {
                Fail-Smoke "candidate build environment $($environment.Key) = $($manifest.build.environment.($environment.Key)), want $($environment.Value)"
            }
        }
        if ([string]$manifest.build.environment.GOPROXY -notmatch '^file://') {
            Fail-Smoke "candidate build environment GOPROXY must be a file URL"
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
        $expectedPriorCycles = @("030", "040", "042", "045", "048", "050", "063", "067", "068")
        $actualPriorCycles = @($manifest.attempts.priorCycles | ForEach-Object { [string]$_ })
        if ($actualPriorCycles.Count -ne $expectedPriorCycles.Count -or (Compare-Object -ReferenceObject $expectedPriorCycles -DifferenceObject $actualPriorCycles)) {
            Fail-Smoke "candidate attempts priorCycles = $($actualPriorCycles -join ', '), want $($expectedPriorCycles -join ', ')"
        }
        if ([int]$manifest.attempts.cycle063BuildMaximum -ne 1 -or [int]$manifest.attempts.cycle063BuildUsed -ne 1) {
            Fail-Smoke "candidate attempts must record cycle063BuildMaximum=1 and cycle063BuildUsed=1"
        }
        if ([int]$manifest.attempts.cycle067BuildMaximum -ne 2 -or [int]$manifest.attempts.cycle067BuildUsed -ne 2) {
            Fail-Smoke "candidate attempts must record cycle067BuildMaximum=2 and cycle067BuildUsed=2"
        }
        if ([int]$manifest.attempts.cycle068BuildMaximum -ne 1 -or [int]$manifest.attempts.cycle068BuildUsed -ne 1) {
            Fail-Smoke "candidate attempts must record cycle068BuildMaximum=1 and cycle068BuildUsed=1"
        }
        if ([int]$manifest.attempts.cycle070BuildMaximum -ne 1 -or [int]$manifest.attempts.cycle070BuildUsed -ne 1) {
            Fail-Smoke "candidate attempts must record cycle070BuildMaximum=1 and cycle070BuildUsed=1"
        }
        $archiveArtifacts = @($manifest.artifacts | Where-Object { $_.role -eq "windows-amd64-archive" })
        $installerArtifacts = @($manifest.artifacts | Where-Object { $_.role -eq "windows-installer" })
        $executableArtifacts = @($manifest.artifacts | Where-Object { $_.role -eq "windows-amd64-executable" })
        if (@($manifest.artifacts).Count -ne 3 -or $archiveArtifacts.Count -ne 1 -or $installerArtifacts.Count -ne 1 -or $executableArtifacts.Count -ne 1) {
            Fail-Smoke "candidate manifest must contain one archive, one executable, and one installer artifact"
        }
        $archiveArtifact = $archiveArtifacts[0]
        $installerArtifact = $installerArtifacts[0]
        $executableArtifact = $executableArtifacts[0]
        $archiveName = "you_${version}_windows_amd64.zip"
        if ($archiveArtifact.file -ne $archiveName -or $installerArtifact.file -ne "install.ps1" -or $executableArtifact.file -ne "you.exe") {
            Fail-Smoke "candidate artifact filenames do not match the declared version and executable identity"
        }
        foreach ($artifact in @($archiveArtifact, $installerArtifact, $executableArtifact)) {
            if ([System.IO.Path]::GetFileName([string]$artifact.file) -ne [string]$artifact.file -or [int64]$artifact.bytes -le 0 -or [string]$artifact.sha256 -notmatch '^[0-9a-f]{64}$') {
                Fail-Smoke "candidate artifact $($artifact.role) is incomplete or unsafe"
            }
        }

        $archivePath = Join-Path $candidateDirectory $archiveArtifact.file
        $installerPath = Join-Path $candidateDirectory $installerArtifact.file
        $executablePath = Join-Path $candidateDirectory $executableArtifact.file
        $archiveEvidence = Get-SmokeArtifactEvidence "windows-amd64-archive" $archivePath
        $installerEvidence = Get-SmokeArtifactEvidence "windows-installer" $installerPath
        $executableEvidence = Get-SmokeArtifactEvidence "windows-amd64-executable" $executablePath
        if ($archiveEvidence.bytes -ne [int64]$archiveArtifact.bytes -or $archiveEvidence.sha256 -ne [string]$archiveArtifact.sha256) {
            Fail-Smoke "candidate archive bytes or SHA-256 do not match the manifest"
        }
        if ($installerEvidence.bytes -ne [int64]$installerArtifact.bytes -or $installerEvidence.sha256 -ne [string]$installerArtifact.sha256) {
            Fail-Smoke "candidate installer bytes or SHA-256 do not match the manifest"
        }
        if ($executableEvidence.bytes -ne [int64]$executableArtifact.bytes -or $executableEvidence.sha256 -ne [string]$executableArtifact.sha256) {
            Fail-Smoke "candidate executable bytes or SHA-256 do not match the manifest"
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
        if ($executableEvidence.bytes -ne $zipEntryEvidence.bytes -or $executableEvidence.sha256 -ne $zipEntryEvidence.sha256) {
            Fail-Smoke "candidate executable does not match the you.exe member in the declared archive"
        }
        $buildInfoEvidence = Get-SmokeExecutableBuildInfo -ExecutablePath $executablePath -GoExecutablePath $goExecutablePath -WorkingDirectory $candidateDirectory
        Record-SmokeCommandObservation -Result $buildInfoEvidence.result -NetworkObservations $networkObservations -ForbiddenProcessObservations $forbiddenProcessObservations
        Assert-SmokeCommandNetwork -Result $buildInfoEvidence.result -CommandName "go version -m candidate executable"
        $commandEvidence += [ordered]@{
            command = "go version -m $executablePath"
            exitCode = $buildInfoEvidence.result.exitCode
            stdout = $buildInfoEvidence.result.stdout
            stderr = $buildInfoEvidence.result.stderr
            network = $buildInfoEvidence.result.network
            forbiddenProcesses = $buildInfoEvidence.result.forbiddenProcesses
        }
        $report.identity = [ordered]@{
            schemaVersion = $manifest.schemaVersion
            sourceCommit = $manifest.source.commit
            sourceTree = $manifest.source.tree
            embeddedSourceRevision = $buildInfoEvidence.sourceRevision
            embeddedVCSModified = $buildInfoEvidence.vcsModified
            candidateVersion = $version
            cliVersion = [string]$manifest.build.cliVersion
            target = "$($manifest.build.target.goos)/$($manifest.build.target.goarch)"
            archive = $archiveEvidence
            archiveYouEntry = $zipEntryEvidence
            executable = $executableEvidence
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
            embeddedSourceRevision = $buildInfoEvidence.sourceRevision
            embeddedVCSModified = $buildInfoEvidence.vcsModified
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
            if ($null -ne $archiveEvidence -and $null -ne $installerEvidence -and $null -ne $executableEvidence -and $null -ne $manifestEvidence) {
                $postArchive = Get-SmokeArtifactEvidence "windows-amd64-archive" $archivePath
                $postInstaller = Get-SmokeArtifactEvidence "windows-installer" $installerPath
                $postExecutable = Get-SmokeArtifactEvidence "windows-amd64-executable" $executablePath
                $postManifest = Get-SmokeArtifactEvidence "candidate-manifest" $manifestPath
                $postDigest = (([System.IO.File]::ReadAllText($digestPath)).Trim() -split '\s+')[0]
                $report.postCleanup = [ordered]@{
                    archive = $postArchive
                    executable = $postExecutable
                    installer = $postInstaller
                    manifest = $postManifest
                    detachedManifestDigest = $postDigest
                    candidateHashesStable = ($postArchive.sha256 -eq $archiveEvidence.sha256 -and $postExecutable.sha256 -eq $executableEvidence.sha256 -and $postInstaller.sha256 -eq $installerEvidence.sha256 -and $postManifest.sha256 -eq $manifestEvidence.sha256 -and $postDigest -eq $manifestEvidence.sha256)
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

function Invoke-ObserverIdentityFixture {
    param(
        [string]$RequestedInstallDir,
        [string]$RequestedReportPath
    )

    $reportPath = if ([string]::IsNullOrWhiteSpace($RequestedReportPath)) {
        Join-Path (Resolve-SmokeAbsolutePath $RequestedInstallDir) "observer-identity-report.json"
    } else {
        Resolve-SmokeAbsolutePath $RequestedReportPath
    }
    $job = Start-Job -ScriptBlock {
        param($IdentityFunctionSource, $CandidateCycle)
        $ErrorActionPreference = "Stop"
        Invoke-Expression $IdentityFunctionSource

        function New-ObserverSyntheticSnapshot {
            param([string]$Identity)

            $root = [pscustomobject]@{ processId = 100; identity = "100/root"; name = "powershell" }
            $descendant = [pscustomobject]@{ processId = 42; identity = $Identity; name = "powershell" }
            return [pscustomobject]@{
                rootPresent = $true
                root = $root
                descendants = @($descendant)
                ownedRecords = @($root, $descendant)
            }
        }

        $crossActive = @{}
        $crossHistory = @{}
        $crossState = @{ rootIdentity = $null }
        Register-ObserverIdentities (New-ObserverSyntheticSnapshot "42/A") $crossActive $crossHistory $crossState
        $emptyRows = @(Get-ObserverConnectionTable { @() })
        Remove-ObserverInactivePidIdentities (New-ObserverSyntheticSnapshot "42/B") $crossActive
        Register-ObserverIdentities (New-ObserverSyntheticSnapshot "42/B") $crossActive $crossHistory $crossState
        Register-ObserverIdentities (New-ObserverSyntheticSnapshot "42/B") $crossActive $crossHistory $crossState
        $crossPass = [string]$crossActive[42] -ceq "42/B" -and $crossHistory.ContainsKey("42/A") -and $crossHistory.ContainsKey("42/B") -and $emptyRows.Count -eq 0

        $withinActive = @{}
        $withinHistory = @{}
        $withinState = @{ rootIdentity = $null }
        Register-ObserverIdentities (New-ObserverSyntheticSnapshot "42/A") $withinActive $withinHistory $withinState
        $withinRejected = $false
        $withinError = ""
        try {
            [void](Get-ObserverConnectionTable { @() })
            Register-ObserverIdentities (New-ObserverSyntheticSnapshot "42/B") $withinActive $withinHistory $withinState
        } catch {
            $withinRejected = $true
            $withinError = $_.Exception.Message
        }

        $queryErrorPreserved = $false
        $queryError = ""
        try {
            [void](Get-ObserverConnectionTable { throw "synthetic query error" })
        } catch {
            $queryError = $_.Exception.Message
            $queryErrorPreserved = $queryError -like "*synthetic query error*"
        }
        [ordered]@{
            schemaVersion = "localai-windows-install-candidate-observer-identity/v1"
            status = if ($crossPass -and $withinRejected -and $queryErrorPreserved) { "PASS" } else { "FAIL" }
            cycle = $CandidateCycle
            releaseStatus = "observer-identity-fixture"
            crossInterval = [ordered]@{
                status = if ($crossPass) { "PASS" } else { "FAIL" }
                activeIdentity = [string]$crossActive[42]
                historicalIdentities = @($crossHistory.Keys | Sort-Object)
                continued = $crossPass
            }
            withinSample = [ordered]@{
                status = if ($withinRejected) { "PASS" } else { "FAIL" }
                rejected = $withinRejected
                error = $withinError
            }
            emptyTCP = [ordered]@{
                status = if ($emptyRows.Count -eq 0) { "PASS" } else { "FAIL" }
                rowCount = $emptyRows.Count
            }
            queryError = [ordered]@{
                status = if ($queryErrorPreserved) { "PASS" } else { "FAIL" }
                error = $queryError
            }
        } | ConvertTo-Json -Depth 12
    } -ArgumentList $observerIdentityFunctionSource, $expectedCandidateCycle
    $report = $null
    $failure = $null
    try {
        $completed = Wait-Job -Job $job -Timeout 10
        if ($null -eq $completed -or $job.State -ne "Completed") {
            throw "observer identity fixture did not complete"
        }
        $results = @(Receive-Job -Job $job -ErrorAction Stop)
        if ($results.Count -eq 0) {
            throw "observer identity fixture returned no report"
        }
        $report = [string]$results[$results.Count - 1] | ConvertFrom-Json
    } catch {
        $failure = $_.Exception
    } finally {
        try { Remove-Job -Job $job -Force -ErrorAction Stop } catch { }
    }
    if ($null -eq $report) {
        $report = [ordered]@{
            schemaVersion = "localai-windows-install-candidate-observer-identity/v1"
            status = "FAIL"
            cycle = $expectedCandidateCycle
            releaseStatus = "observer-identity-fixture"
            error = if ($null -eq $failure) { "observer identity fixture returned no report" } else { $failure.Message }
        }
    }
    $reportParent = Split-Path -Parent $reportPath
    if (-not (Test-Path -LiteralPath $reportParent -PathType Container)) {
        Fail-Smoke "observer identity report parent does not exist: $reportParent"
    }
    [System.IO.File]::WriteAllText($reportPath, ($report | ConvertTo-Json -Depth 12))
    if ($report.status -ne "PASS") {
        throw "observer identity fixture failed; report=$reportPath"
    }
    Write-Output "observer identity fixture passed; report=$reportPath"
}

function Invoke-ObserverFixture {
    param(
        [string]$RequestedInstallDir,
        [string]$RequestedReportPath,
        [string]$RootCommand,
        [string[]]$RootArgumentList,
        [string]$RootWorkingDirectory,
        [string]$RootStdoutPath,
        [string]$RootStderrPath,
        [int]$RootOutputMaximumBytes = 1048576,
        [int]$RequestedRootExitCode = 0,
        [string]$ObserverReleaseCheckoutPath,
        [int]$TimeoutSeconds = 20
    )

    $externalRoot = -not [string]::IsNullOrWhiteSpace($RootCommand)
    if ($TimeoutSeconds -le 0) {
        Fail-Smoke "observer timeout must be positive"
    }
    $resolvedRootWorkingDirectory = $null
    if ($externalRoot) {
        $resolvedRootWorkingDirectory = if ([string]::IsNullOrWhiteSpace($RootWorkingDirectory)) {
            (Get-Location).Path
        } else {
            Resolve-SmokeAbsolutePath $RootWorkingDirectory
        }
        if (-not (Test-Path -LiteralPath $resolvedRootWorkingDirectory -PathType Container)) {
            Fail-Smoke "observer root working directory does not exist: $resolvedRootWorkingDirectory"
        }
    }
    $reportPath = if ([string]::IsNullOrWhiteSpace($RequestedReportPath)) {
        Join-Path (Resolve-SmokeAbsolutePath $RequestedInstallDir) "observer-report.json"
    } else {
        Resolve-SmokeAbsolutePath $RequestedReportPath
    }
    $fixtureRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("infinite-you-observer-" + [System.Guid]::NewGuid().ToString("N"))
    [void][System.IO.Directory]::CreateDirectory($fixtureRoot)
    $observationRoot = if ($externalRoot) {
        Resolve-SmokeAbsolutePath $RequestedInstallDir
    } else {
        $fixtureRoot
    }
    if (-not (Test-Path -LiteralPath $observationRoot -PathType Container)) {
        Fail-Smoke "observer observation root does not exist: $observationRoot"
    }
    $releaseCheckoutEvidence = $null
    if (-not [string]::IsNullOrWhiteSpace($ObserverReleaseCheckoutPath)) {
        $releaseCheckoutEvidence = Assert-SmokeReleaseCheckout -RequestedCheckoutPath $ObserverReleaseCheckoutPath -RequireDistExclude
        $releaseCheckoutEvidence.statusCleanDuringOutput = $false
        $releaseCheckoutEvidence.statusCleanAfterOutput = $false
        $releaseCheckoutEvidence.statusChecks = 0
    }
    $processReadyPath = Join-Path $fixtureRoot "process-ready"
    $diskReadyPath = Join-Path $fixtureRoot "disk-ready"
    $pidPath = Join-Path $fixtureRoot "root.pid"
    $donePath = Join-Path $fixtureRoot "root.done"
    $rootStdoutPath = if ([string]::IsNullOrWhiteSpace($RootStdoutPath)) {
        Join-Path (Split-Path -Parent $reportPath) "observer-root.stdout.txt"
    } else {
        Resolve-SmokeAbsolutePath $RootStdoutPath
    }
    $rootStderrPath = if ([string]::IsNullOrWhiteSpace($RootStderrPath)) {
        Join-Path (Split-Path -Parent $reportPath) "observer-root.stderr.txt"
    } else {
        Resolve-SmokeAbsolutePath $RootStderrPath
    }
    if ($RootOutputMaximumBytes -le 0) {
        Fail-Smoke "observer root output maximum must be positive"
    }
    foreach ($retainedPath in @($rootStdoutPath, $rootStderrPath)) {
        $retainedParent = Split-Path -Parent $retainedPath
        if (-not (Test-Path -LiteralPath $retainedParent -PathType Container)) {
            Fail-Smoke "observer root output parent does not exist: $retainedParent"
        }
        if (Test-Path -LiteralPath $retainedPath -PathType Leaf) {
            Fail-Smoke "observer root output evidence already exists: $retainedPath"
        }
        if (Test-Path -LiteralPath $retainedPath -PathType Container) {
            Fail-Smoke "observer root output evidence path is a directory: $retainedPath"
        }
    }
    $rootStdoutCapturePath = $rootStdoutPath + ".capture"
    $rootStderrCapturePath = $rootStderrPath + ".capture"
    foreach ($capturePath in @($rootStdoutCapturePath, $rootStderrCapturePath)) {
        if (Test-Path -LiteralPath $capturePath) {
            Fail-Smoke "observer root output capture already exists: $capturePath"
        }
    }
    $processJob = $null
    $diskJob = $null
    $rootProcess = $null
    $rootExitCode = $null
    $rootOutput = [ordered]@{
        status = "FAIL"
        maximumBytes = [int64]$RootOutputMaximumBytes
        stdout = [ordered]@{ status = "FAIL"; path = $rootStdoutPath; present = $false; totalBytes = [int64]0; capturedBytes = [int64]0; truncated = $false; redactedBytes = [int64]0; sha256 = "" }
        stderr = [ordered]@{ status = "FAIL"; path = $rootStderrPath; present = $false; totalBytes = [int64]0; capturedBytes = [int64]0; truncated = $false; redactedBytes = [int64]0; sha256 = "" }
    }
    $failure = $null
    $cleanupErrors = New-Object 'System.Collections.Generic.List[string]'
    $remainingTaskPaths = @()
    $processObservation = [pscustomobject]@{
        status = "FAIL"; startedBeforeRoot = $false; continuedThroughDescendantExit = $false
        maximumGapMilliseconds = [int64]0; nonLoopbackConnections = 0; externalTransferBytes = [int64]0; descendantHighWater = [int64]0
        sampleCount = 0; tcpTableQueries = 0; zeroConnectionSamples = 0; ownedConnectionMatches = 0
        ownedProcessIdentityCount = 0; queryMode = $expectedObserverQueryMode; forbiddenProcesses = @(); error = ""
        releaseStatusCleanBeforeRoot = $false; releaseStatusCleanDuringRoot = $false; releaseStatusCleanAfterOutput = $false; releaseStatusChecks = 0
    }
    $diskObservation = [pscustomobject]@{
        status = "FAIL"; independent = $false; startPeriodicFinal = $false
        maximumGapMilliseconds = [int64]0; peakDeltaBytes = [int64]0; error = ""
    }
    $goEnvironment = [ordered]@{
        GOFLAGS = [string]$env:GOFLAGS
        GOMAXPROCS = [string]$env:GOMAXPROCS
    }

    try {
        $processJob = Start-Job -ScriptBlock {
            param($ReadyPath, $PidPath, $DonePath, $TimeoutSeconds, $IdentityFunctionSource, $ReleaseCheckoutPath)
            $ErrorActionPreference = "Stop"
            Invoke-Expression $IdentityFunctionSource
            $processNetworkGapMaximumMilliseconds = [int64]2000
            $descendantMaximum = [int64]36
            $queryMode = "one-full-TCP-table-query-per-interval-filtered-to-owned-process-identities"
            $forbiddenNamePatterns = @("localai", "local-ai", "llama-server", "llama-cli", "vibevoice", "whisper", "piper")
            $releaseStatusCleanBeforeRoot = $false
            $releaseStatusCleanDuringRoot = $false
            $releaseStatusCleanAfterOutput = $false
            $releaseStatusChecks = 0
            if (-not [string]::IsNullOrWhiteSpace($ReleaseCheckoutPath)) {
                if ((Get-ObserverGitStatus $ReleaseCheckoutPath) -ne "") {
                    throw "release checkout is dirty before root start"
                }
                $releaseStatusChecks++
                $releaseStatusCleanBeforeRoot = $true
            }
            [System.IO.File]::WriteAllText($ReadyPath, "ready")
            function Convert-ObserverCreationTimeTicks {
                param($Value)

                if ($Value -is [DateTime]) {
                    return [int64]$Value.ToUniversalTime().Ticks
                }
                try {
                    $date = [System.Management.ManagementDateTimeConverter]::ToDateTime([string]$Value)
                } catch {
                    throw "process creation time is not readable: $Value"
                }
                return [int64]$date.ToUniversalTime().Ticks
            }
            function Get-ObserverProcessSnapshot {
                param([int]$RootId)
                $all = @(Get-CimInstance -ClassName Win32_Process -ErrorAction Stop)
                $byId = @{}
                $children = @{}
                foreach ($item in $all) {
                    $processId = [int]$item.ProcessId
                    if ($byId.ContainsKey($processId)) {
                        throw "process identity is ambiguous for PID $processId"
                    }
                    $creationTicks = Convert-ObserverCreationTimeTicks $item.CreationDate
                    $record = [pscustomobject]@{
                        processId = $processId
                        parentProcessId = [int]$item.ParentProcessId
                        identity = "$processId/$creationTicks"
                        name = [string]$item.Name
                    }
                    $byId[$processId] = $record
                    $parent = [int]$item.ParentProcessId
                    if (-not $children.ContainsKey($parent)) { $children[$parent] = @() }
                    $children[$parent] = @($children[$parent]) + $processId
                }
                if (-not $byId.ContainsKey($RootId)) {
                    return [pscustomobject]@{
                        rootPresent = $false
                        root = $null
                        descendants = @()
                        ownedRecords = @()
                    }
                }
                $pending = [System.Collections.Generic.Queue[int]]::new()
                $visited = @{$RootId = $true}
                $descendants = New-Object 'System.Collections.Generic.List[object]'
                $ownedRecords = New-Object 'System.Collections.Generic.List[object]'
                $root = $byId[$RootId]
                [void]$ownedRecords.Add($root)
                $pending.Enqueue($RootId)
                while ($pending.Count -gt 0) {
                    $parent = $pending.Dequeue()
                    if (-not $children.ContainsKey($parent)) { continue }
                    foreach ($childId in @($children[$parent])) {
                        $childId = [int]$childId
                        if ($visited.ContainsKey($childId)) {
                            throw "owned process tree is ambiguous at PID $childId"
                        }
                        if (-not $byId.ContainsKey($childId)) {
                            throw "owned process tree lost PID $childId during enumeration"
                        }
                        $visited[$childId] = $true
                        $child = $byId[$childId]
                        [void]$descendants.Add($child)
                        [void]$ownedRecords.Add($child)
                        $pending.Enqueue($childId)
                    }
                }
                if ($descendants.Count -gt $descendantMaximum) {
                    throw "owned descendant count $($descendants.Count) exceeds $descendantMaximum"
                }
                [pscustomobject]@{
                    rootPresent = $true
                    root = $root
                    descendants = $descendants.ToArray()
                    ownedRecords = $ownedRecords.ToArray()
                }
            }
            function Test-ObserverLoopbackAddress {
                param([string]$Address)

                return $Address.Trim().ToLowerInvariant() -in @("127.0.0.1", "::1", "0:0:0:0:0:0:0:1", "::ffff:127.0.0.1")
            }
            function Get-ObserverSample {
                param(
                    [int]$RootId,
                    [hashtable]$IdentityByPid,
                    [hashtable]$OwnedIdentityToPid,
                    [hashtable]$State
                )
                if ($null -eq (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue)) {
                    throw "Get-NetTCPConnection is unavailable"
                }
                $before = Get-ObserverProcessSnapshot $RootId
                Remove-ObserverInactivePidIdentities $before $IdentityByPid
                Register-ObserverIdentities $before $IdentityByPid $OwnedIdentityToPid $State
                # One complete TCP-table query is deliberately shared by every
                # owned process in this interval; an empty table is a valid sample.
                $connections = @(Get-ObserverConnectionTable { Get-NetTCPConnection -ErrorAction Stop })
                $after = Get-ObserverProcessSnapshot $RootId
                Register-ObserverIdentities $after $IdentityByPid $OwnedIdentityToPid $State
                $ownedPids = @{}
                foreach ($record in @($before.ownedRecords) + @($after.ownedRecords)) {
                    $ownedPids[[int]$record.processId] = [string]$record.identity
                }
                $nonLoopback = 0
                $ownedMatches = 0
                foreach ($connection in @($connections)) {
                    if ($null -eq $connection.OwningProcess) {
                        throw "TCP-table row has no owning process identity"
                    }
                    $processId = [int]$connection.OwningProcess
                    if (-not $ownedPids.ContainsKey($processId)) {
                        continue
                    }
                    $ownedMatches++
                    if ([string]$connection.State -ne "Listen" -and -not (Test-ObserverLoopbackAddress ([string]$connection.RemoteAddress))) {
                        $nonLoopback++
                    }
                }
                $forbiddenProcesses = @($before.ownedRecords + $after.ownedRecords | Where-Object {
                    $name = ([string]$_.name).ToLowerInvariant()
                    $forbiddenNamePatterns | Where-Object { $name -like "*$_*" }
                } | ForEach-Object { "{0}:{1}" -f $_.processId, $_.name } | Sort-Object -Unique)
                if ($forbiddenProcesses.Count -ne 0) {
                    throw "forbidden owned process observed: $($forbiddenProcesses -join ', ')"
                }
                if ($nonLoopback -ne 0) {
                    throw "non-loopback connection observed in owned TCP sample"
                }
                [pscustomobject]@{
                    rootPresent = [bool]$after.rootPresent
                    descendantCount = [int]$after.descendants.Count
                    nonLoopbackConnections = [int]$nonLoopback
                    ownedConnectionMatches = [int]$ownedMatches
                    tcpRows = [int]$connections.Count
                    tcpTableQueries = 1
                    ownedProcessIdentityCount = [int]$OwnedIdentityToPid.Count
                    forbiddenProcesses = $forbiddenProcesses
                }
            }
            $identityByPid = @{}
            $ownedIdentityToPid = @{}
            $observerState = @{rootIdentity = $null}
            $maximumGap = [int64]0
            $samples = 0
            $descendantHighWater = [int64]0
            $nonLoopbackConnections = 0
            $tcpTableQueries = 0
            $zeroConnectionSamples = 0
            $ownedConnectionMatches = 0
            try {
                $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
                while (-not (Test-Path -LiteralPath $PidPath) -and [DateTime]::UtcNow -lt $deadline) { Start-Sleep -Milliseconds 25 }
                if (-not (Test-Path -LiteralPath $PidPath)) { throw "root PID was not published" }
                $rootId = [int]([System.IO.File]::ReadAllText($PidPath)).Trim()
                $previous = $null
                $rootExited = $false
                $continuedThroughExit = $false
                while ([DateTime]::UtcNow -lt $deadline) {
                    $sample = Get-ObserverSample $rootId $identityByPid $ownedIdentityToPid $observerState
                    $now = [DateTime]::UtcNow
                    if ($null -ne $previous) {
                        $gap = [int64]($now - $previous).TotalMilliseconds
                        if ($gap -gt $maximumGap) { $maximumGap = $gap }
                        if ($gap -gt $processNetworkGapMaximumMilliseconds) {
                            throw "process/network observer gap $gap ms exceeds $processNetworkGapMaximumMilliseconds ms"
                        }
                    }
                    $previous = $now
                    $samples++
                    if ($sample.descendantCount -gt $descendantHighWater) { $descendantHighWater = $sample.descendantCount }
                    if ($sample.descendantCount -gt $descendantMaximum) { throw "owned descendant count $($sample.descendantCount) exceeds $descendantMaximum" }
                    $nonLoopbackConnections += [int]$sample.nonLoopbackConnections
                    $tcpTableQueries += [int]$sample.tcpTableQueries
                    $ownedConnectionMatches += [int]$sample.ownedConnectionMatches
                    if ([int]$sample.ownedConnectionMatches -eq 0) { $zeroConnectionSamples++ }
                    if ([int]$sample.nonLoopbackConnections -ne 0) { throw "non-loopback connection observed in owned TCP sample" }
                    if (Test-Path -LiteralPath $DonePath) { $rootExited = $true }
                    if (-not [string]::IsNullOrWhiteSpace($ReleaseCheckoutPath)) {
                        if ((Get-ObserverGitStatus $ReleaseCheckoutPath) -ne "") {
                            throw "release checkout became dirty during output production"
                        }
                        $releaseStatusChecks++
                        $releaseStatusCleanDuringRoot = $true
                        if ($rootExited) { $releaseStatusCleanAfterOutput = $true }
                    }
                    if ($rootExited -and -not $sample.rootPresent -and $sample.descendantCount -eq 0) {
                        if ($null -eq $observerState["rootIdentity"]) { throw "root exited before its owned process identity was captured" }
                        $continuedThroughExit = $true
                        break
                    }
                    # This is the measurement cadence, not a completion wait;
                    # completion is decided from the next process/TCP sample.
                    Start-Sleep -Milliseconds 100
                }
                if (-not $continuedThroughExit) { throw "observer did not reach root and descendant exit" }
                [pscustomobject]@{
                    status = "PASS"; startedBeforeRoot = $true; continuedThroughDescendantExit = $continuedThroughExit
                    maximumGapMilliseconds = $maximumGap; nonLoopbackConnections = $nonLoopbackConnections
                    externalTransferBytes = [int64]0; descendantHighWater = $descendantHighWater
                    sampleCount = $samples; tcpTableQueries = $tcpTableQueries
                    zeroConnectionSamples = $zeroConnectionSamples; ownedConnectionMatches = $ownedConnectionMatches
                    ownedProcessIdentityCount = [int64]$ownedIdentityToPid.Count; queryMode = $queryMode
                    forbiddenProcesses = @(); error = ""
                    releaseStatusCleanBeforeRoot = $releaseStatusCleanBeforeRoot; releaseStatusCleanDuringRoot = $releaseStatusCleanDuringRoot; releaseStatusCleanAfterOutput = $releaseStatusCleanAfterOutput; releaseStatusChecks = $releaseStatusChecks
                }
            } catch {
                [pscustomobject]@{
                    status = "FAIL"; startedBeforeRoot = $true; continuedThroughDescendantExit = $false
                    maximumGapMilliseconds = $maximumGap; nonLoopbackConnections = $nonLoopbackConnections; externalTransferBytes = [int64]0
                    descendantHighWater = $descendantHighWater; sampleCount = $samples; tcpTableQueries = $tcpTableQueries
                    zeroConnectionSamples = $zeroConnectionSamples; ownedConnectionMatches = $ownedConnectionMatches
                    ownedProcessIdentityCount = [int64]$ownedIdentityToPid.Count; queryMode = $queryMode
                    forbiddenProcesses = @(); error = $_.Exception.Message
                    releaseStatusCleanBeforeRoot = $releaseStatusCleanBeforeRoot; releaseStatusCleanDuringRoot = $releaseStatusCleanDuringRoot; releaseStatusCleanAfterOutput = $releaseStatusCleanAfterOutput; releaseStatusChecks = $releaseStatusChecks
                }
            }
        } -ArgumentList $processReadyPath, $pidPath, $donePath, $TimeoutSeconds, $observerIdentityFunctionSource, $ObserverReleaseCheckoutPath

        $diskJob = Start-Job -ScriptBlock {
            param($RootPath, $ReadyPath, $DonePath, $TimeoutSeconds)
            $ErrorActionPreference = "Stop"
            $diskGapMaximumMilliseconds = [int64]2000
            $temporaryDiskMaximum = [int64]4294967296
            function Get-ObserverDirectoryBytes {
                $total = [int64]0
                if (-not (Test-Path -LiteralPath $RootPath -PathType Container)) { return $total }
                foreach ($item in @(Get-ChildItem -LiteralPath $RootPath -File -Force -Recurse -ErrorAction Stop)) {
                    if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { throw "observer fixture saw a reparse point" }
                    $total += [int64]$item.Length
                }
                return $total
            }
            $maximumGap = [int64]0
            $peakBytes = [int64]0
            $samples = 0
            try {
                $startedAt = [DateTime]::UtcNow
                [System.IO.File]::WriteAllText($ReadyPath, "ready")
                $previous = $null
                $initialBytes = $null
                $finalSample = $false
                $deadline = $startedAt.AddSeconds($TimeoutSeconds)
                while ([DateTime]::UtcNow -lt $deadline) {
                    $bytes = [int64](Get-ObserverDirectoryBytes)
                    $now = [DateTime]::UtcNow
                    if ($null -ne $previous) {
                        $gap = [int64]($now - $previous).TotalMilliseconds
                        if ($gap -gt $maximumGap) { $maximumGap = $gap }
                        if ($gap -gt $diskGapMaximumMilliseconds) {
                            throw "disk observer gap $gap ms exceeds $diskGapMaximumMilliseconds ms"
                        }
                    }
                    $previous = $now
                    if ($null -eq $initialBytes) { $initialBytes = $bytes }
                    if ($bytes -gt $peakBytes) { $peakBytes = $bytes }
                    if ($peakBytes - $initialBytes -gt $temporaryDiskMaximum) {
                        throw "disk observer delta $($peakBytes - $initialBytes) exceeds $temporaryDiskMaximum bytes"
                    }
                    $samples++
                    if (Test-Path -LiteralPath $DonePath) {
                        $bytes = [int64](Get-ObserverDirectoryBytes)
                        $now = [DateTime]::UtcNow
                        $gap = [int64]($now - $previous).TotalMilliseconds
                        if ($gap -gt $maximumGap) { $maximumGap = $gap }
                        if ($gap -gt $diskGapMaximumMilliseconds) {
                            throw "disk observer final gap $gap ms exceeds $diskGapMaximumMilliseconds ms"
                        }
                        if ($bytes -gt $peakBytes) { $peakBytes = $bytes }
                        if ($peakBytes - $initialBytes -gt $temporaryDiskMaximum) {
                            throw "disk observer delta $($peakBytes - $initialBytes) exceeds $temporaryDiskMaximum bytes"
                        }
                        $samples++
                        $finalSample = $true
                        break
                    }
                    # This is the disk measurement cadence; completion is
                    # decided from the observed done marker and final sample.
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
                    maximumGapMilliseconds = $maximumGap; peakDeltaBytes = [int64]$peakBytes; error = $_.Exception.Message
                }
            }
        } -ArgumentList $observationRoot, $diskReadyPath, $donePath, $TimeoutSeconds

        $readyDeadline = [DateTime]::UtcNow.AddSeconds(10)
        # Readiness is polled only until both independent observer jobs publish
        # their ready markers; sampling cadence remains inside each observer.
        while ((-not (Test-Path -LiteralPath $processReadyPath) -or -not (Test-Path -LiteralPath $diskReadyPath)) -and [DateTime]::UtcNow -lt $readyDeadline) { Start-Sleep -Milliseconds 25 }
        if (-not (Test-Path -LiteralPath $processReadyPath) -or -not (Test-Path -LiteralPath $diskReadyPath)) { throw "observers did not start before root" }

        if ($externalRoot) {
            $rootFilePath = $RootCommand
            $rootArguments = @($RootArgumentList)
        } else {
        $childOne = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes("Start-Sleep -Milliseconds 2500"))
        $childTwo = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes("Start-Sleep -Milliseconds 3000"))
        $fixtureFile = (Join-Path $fixtureRoot "fixture.txt").Replace("'", "''")
        if ($RequestedRootExitCode -lt 0 -or $RequestedRootExitCode -gt 255) { throw "observer root exit code must be between 0 and 255" }
        $rootExitCommand = "exit " + ([int]$RequestedRootExitCode)
        $rootScript = "`$ErrorActionPreference = 'Stop'; Set-Content -LiteralPath '$fixtureFile' -Value ('fixture' * 64); Write-Output 'root stdout token=observer-secret'; [Console]::Error.WriteLine('root stderr api_key=observer-secret'); `$one = Start-Process -FilePath 'powershell.exe' -WindowStyle Hidden -ArgumentList @('-NoProfile','-NonInteractive','-EncodedCommand','$childOne') -PassThru; `$two = Start-Process -FilePath 'powershell.exe' -WindowStyle Hidden -ArgumentList @('-NoProfile','-NonInteractive','-EncodedCommand','$childTwo') -PassThru; Wait-Process -Id @(`$one.Id, `$two.Id); $rootExitCommand"
        $rootEncoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($rootScript))
        $rootFilePath = "powershell.exe"
        $rootArguments = @("-NoProfile", "-NonInteractive", "-EncodedCommand", $rootEncoded)
        }
        $rootWorkingDirectory = if ($externalRoot) { $resolvedRootWorkingDirectory } else { $fixtureRoot }
        $rootStartInfo = New-SmokeProcessStartInfo -FilePath $rootFilePath -Arguments $rootArguments -WorkingDirectory $rootWorkingDirectory
        $rootProcess = [System.Diagnostics.Process]::new()
        $rootProcess.StartInfo = $rootStartInfo
        if (-not $rootProcess.Start()) { throw "observer root could not start: $rootFilePath" }
        $rootStdoutTask = $rootProcess.StandardOutput.ReadToEndAsync()
        $rootStderrTask = $rootProcess.StandardError.ReadToEndAsync()
        [System.IO.File]::WriteAllText($pidPath, [string]$rootProcess.Id)
        $waitMilliseconds = [int][Math]::Min([int64]2147483647, [int64]$TimeoutSeconds * 1000)
        if (-not $rootProcess.WaitForExit($waitMilliseconds)) { throw "observer root timed out" }
        $rootProcess.Refresh()
        $rootExitCode = [int64]$rootProcess.ExitCode
        [System.IO.File]::WriteAllText($rootStdoutCapturePath, $rootStdoutTask.GetAwaiter().GetResult(), [System.Text.UTF8Encoding]::new($false))
        [System.IO.File]::WriteAllText($rootStderrCapturePath, $rootStderrTask.GetAwaiter().GetResult(), [System.Text.UTF8Encoding]::new($false))
        $rootOutput.stdout = Get-SmokeRootOutputEvidence -RawPath $rootStdoutCapturePath -RetainedPath $rootStdoutPath -MaximumBytes $RootOutputMaximumBytes
        $rootOutput.stderr = Get-SmokeRootOutputEvidence -RawPath $rootStderrCapturePath -RetainedPath $rootStderrPath -MaximumBytes $RootOutputMaximumBytes
        $rootOutput.status = if ($rootOutput.stdout.status -eq "PASS" -and $rootOutput.stderr.status -eq "PASS") { "PASS" } else { "FAIL" }
        [System.IO.File]::WriteAllText($donePath, "done")
        $completedJobs = @(Wait-Job -Job @($processJob, $diskJob) -Timeout 25)
        if ($completedJobs.Count -ne 2) { throw "observers did not complete after root exit" }
        $processResults = @(Receive-Job -Job $processJob -ErrorAction Stop)
        $diskResults = @(Receive-Job -Job $diskJob -ErrorAction Stop)
        if ($processResults.Count -eq 0 -or $diskResults.Count -eq 0) { throw "observers returned no result" }
        $processObservation = $processResults[$processResults.Count - 1]
        $diskObservation = $diskResults[$diskResults.Count - 1]
        if (-not [string]::IsNullOrWhiteSpace($ObserverReleaseCheckoutPath)) {
            $finalCheckoutEvidence = Assert-SmokeReleaseCheckout -RequestedCheckoutPath $ObserverReleaseCheckoutPath -RequireDistExclude
            $releaseCheckoutEvidence.statusCleanDuringOutput = [bool]$processObservation.releaseStatusCleanDuringRoot
            $releaseCheckoutEvidence.statusCleanAfterOutput = [bool]$finalCheckoutEvidence.statusCleanAfterPreparation
            $releaseCheckoutEvidence.statusChecks = [int]$processObservation.releaseStatusChecks
        }
    } catch {
        $failure = $_.Exception
        if ($processObservation.error -eq "") { $processObservation.error = $failure.Message }
        $jobStates = @($processJob, $diskJob) | Where-Object { $null -ne $_ } | ForEach-Object { "$($_.Name)=$($_.State)" }
        if ($jobStates.Count -ne 0) { $failure = [System.Exception]::new("$($failure.Message); observer jobs: $($jobStates -join ', ')") }
    } finally {
        if ($null -ne $rootProcess) {
            try {
                if (-not $rootProcess.HasExited) { & taskkill.exe /PID $rootProcess.Id /T /F | Out-Null }
                if (-not $rootProcess.HasExited) { [void]$cleanupErrors.Add("root process $($rootProcess.Id) remained running") }
            } catch {
                [void]$cleanupErrors.Add("root process cleanup: $($_.Exception.Message)")
            }
            try { $rootProcess.Dispose() } catch { }
        }
        foreach ($job in @($processJob, $diskJob)) {
            if ($null -ne $job) {
                try { if ($job.State -eq "Running") { Stop-Job -Job $job -ErrorAction Stop } } catch { [void]$cleanupErrors.Add("observer job stop: $($_.Exception.Message)") }
                try {
                    Wait-Job -Job $job -Timeout 5 -ErrorAction Stop | Out-Null
                    if ($job.State -in @("Running", "NotStarted")) { [void]$cleanupErrors.Add("observer job $($job.Name) remained $($job.State)") }
                } catch { [void]$cleanupErrors.Add("observer job wait: $($_.Exception.Message)") }
                try { Remove-Job -Job $job -Force -ErrorAction Stop } catch { [void]$cleanupErrors.Add("observer job removal: $($_.Exception.Message)") }
            }
        }
        try {
            if (Test-Path -LiteralPath $fixtureRoot) { Remove-Item -LiteralPath $fixtureRoot -Recurse -Force -ErrorAction Stop }
        } catch { [void]$cleanupErrors.Add("fixture root cleanup: $($_.Exception.Message)") }
        foreach ($capturePath in @($rootStdoutCapturePath, $rootStderrCapturePath)) {
            try {
                if (Test-Path -LiteralPath $capturePath) { Remove-Item -LiteralPath $capturePath -Force -ErrorAction Stop }
            } catch { [void]$cleanupErrors.Add("root output capture cleanup: $($_.Exception.Message)") }
        }
        if (Test-Path -LiteralPath $fixtureRoot) { $remainingTaskPaths += $fixtureRoot }
    }

    if ($cleanupErrors.Count -ne 0 -and $processObservation.error -eq "") {
        $processObservation.error = $cleanupErrors -join "; "
    }
    $checkoutPass = [string]::IsNullOrWhiteSpace($ObserverReleaseCheckoutPath) -or ($null -ne $releaseCheckoutEvidence -and [bool]$processObservation.releaseStatusCleanBeforeRoot -and [bool]$processObservation.releaseStatusCleanDuringRoot -and [bool]$processObservation.releaseStatusCleanAfterOutput -and [bool]$releaseCheckoutEvidence.statusCleanAfterOutput)
    $rootOutputPass = $rootOutput.status -eq "PASS" -and $rootOutput.stdout.present -and $rootOutput.stderr.present -and $rootOutput.stdout.capturedBytes -le $RootOutputMaximumBytes -and $rootOutput.stderr.capturedBytes -le $RootOutputMaximumBytes
    $observerPass = $goEnvironment.GOFLAGS -eq "-p=4" -and $goEnvironment.GOMAXPROCS -eq "4" -and $processObservation.status -eq "PASS" -and $processObservation.sampleCount -gt 0 -and $processObservation.tcpTableQueries -eq $processObservation.sampleCount -and $processObservation.zeroConnectionSamples -gt 0 -and $processObservation.ownedProcessIdentityCount -gt 0 -and $processObservation.queryMode -eq $expectedObserverQueryMode -and $diskObservation.status -eq "PASS" -and [bool]$diskObservation.startPeriodicFinal -and $processObservation.maximumGapMilliseconds -le $expectedProcessNetworkGapMilliseconds -and $diskObservation.maximumGapMilliseconds -le $expectedDiskGapMilliseconds -and $diskObservation.peakDeltaBytes -le $expectedTemporaryDiskBytesMaximum -and $rootOutputPass -and $checkoutPass -and $cleanupErrors.Count -eq 0 -and $remainingTaskPaths.Count -eq 0
    $report = [ordered]@{
        schemaVersion = "localai-windows-install-candidate-observer/v1"
        status = if ($null -eq $failure -and $rootExitCode -eq 0 -and $observerPass) { "PASS" } else { "FAIL" }
        cycle = $expectedCandidateCycle
        releaseStatus = if ($externalRoot) { "observed-command" } else { "observer-fixture" }
        observation = [ordered]@{
            environment = $goEnvironment
            processNetworkObserver = [ordered]@{
                startedBeforeRoot = [bool]$processObservation.startedBeforeRoot
                continuedThroughDescendantExit = [bool]$processObservation.continuedThroughDescendantExit
                maximumGapMilliseconds = [int64]$processObservation.maximumGapMilliseconds
                status = [string]$processObservation.status
                nonLoopbackConnections = [int]$processObservation.nonLoopbackConnections
                externalTransferBytes = [int64]$processObservation.externalTransferBytes
                sampleCount = [int]$processObservation.sampleCount
                tcpTableQueries = [int]$processObservation.tcpTableQueries
                zeroConnectionSamples = [int]$processObservation.zeroConnectionSamples
                ownedConnectionMatches = [int]$processObservation.ownedConnectionMatches
                ownedProcessIdentityCount = [int]$processObservation.ownedProcessIdentityCount
                queryMode = [string]$processObservation.queryMode
                forbiddenProcesses = @($processObservation.forbiddenProcesses)
                error = [string]$processObservation.error
                releaseStatusCleanBeforeRoot = [bool]$processObservation.releaseStatusCleanBeforeRoot
                releaseStatusCleanDuringRoot = [bool]$processObservation.releaseStatusCleanDuringRoot
                releaseStatusCleanAfterOutput = [bool]$processObservation.releaseStatusCleanAfterOutput
                releaseStatusChecks = [int]$processObservation.releaseStatusChecks
            }
            diskObserver = [ordered]@{
                independent = [bool]$diskObservation.independent
                startPeriodicFinal = [bool]$diskObservation.startPeriodicFinal
                maximumGapMilliseconds = [int64]$diskObservation.maximumGapMilliseconds
                status = [string]$diskObservation.status
                peakDeltaBytes = [int64]$diskObservation.peakDeltaBytes
                error = [string]$diskObservation.error
            }
            descendantMaximum = $expectedDescendantMaximum
            descendantHighWater = [int64]$processObservation.descendantHighWater
            rootExitCode = $rootExitCode
            rootOutput = $rootOutput
            failurePropagation = if ($null -eq $failure -and $rootExitCode -eq 0 -and $observerPass) { "PASS" } else { "FAIL" }
            modelBackendBytes = [int64]0
            modelBackendCalls = [int64]0
        }
        cleanup = [ordered]@{
            status = if ($cleanupErrors.Count -eq 0 -and $remainingTaskPaths.Count -eq 0) { "PASS" } else { "FAIL" }
            remainingTaskPaths = $remainingTaskPaths
            errors = $cleanupErrors.ToArray()
        }
        releaseCheckout = $releaseCheckoutEvidence
    }
    $reportParent = Split-Path -Parent $reportPath
    if (-not (Test-Path -LiteralPath $reportParent -PathType Container)) { Fail-Smoke "observer report parent does not exist: $reportParent" }
    [System.IO.File]::WriteAllText($reportPath, ($report | ConvertTo-Json -Depth 12))
    if ($report.status -ne "PASS") { throw "observer fixture failed; report=$reportPath" }
    Write-Output "observer fixture passed; report=$reportPath"
}

if ($StageDependencyClosure) {
    Invoke-SmokeDependencyStage -RequestedSourceRoot $DependencySourceRoot -RequestedStageRoot $DependencyStageRoot -RequestedSourceManifestPath $DependencySourceManifestPath -RequestedStageManifestPath $DependencyStageManifestPath -RequestedReportPath $DependencyReportPath -ExpectedLockSHA256 $DependencyExpectedLockSHA256 -ExpectedLockBlob $DependencyExpectedLockBlob
    return
}

if ($VerifyDependencyClosure) {
    Invoke-SmokeDependencyVerification -RequestedSourceRoot $DependencySourceRoot -RequestedStageRoot $DependencyStageRoot -RequestedExpectedManifestPath $DependencyExpectedManifestPath -RequestedPostBuildManifestPath $DependencyPostBuildManifestPath -RequestedSourceAfterManifestPath $DependencySourceAfterManifestPath -RequestedReportPath $DependencyReportPath -ExpectedLockSHA256 $DependencyExpectedLockSHA256 -ExpectedLockBlob $DependencyExpectedLockBlob
    return
}

if ($PrepareReleaseCheckout) {
    if ([string]::IsNullOrWhiteSpace($ReleaseCheckoutPath)) {
        Fail-Smoke "release checkout path is required for private exclude preparation"
    }
    $releaseCheckoutEvidence = Assert-SmokeReleaseCheckout -RequestedCheckoutPath $ReleaseCheckoutPath -PrepareDistExclude -RequireDistExclude
    $releaseCheckoutReportPath = if ([string]::IsNullOrWhiteSpace($ReleaseCheckoutReportPath)) {
        Join-Path (Resolve-SmokeAbsolutePath $InstallDir) "release-checkout-report.json"
    } else {
        Resolve-SmokeAbsolutePath $ReleaseCheckoutReportPath
    }
    $releaseCheckoutReportParent = Split-Path -Parent $releaseCheckoutReportPath
    if (-not (Test-Path -LiteralPath $releaseCheckoutReportParent -PathType Container)) {
        Fail-Smoke "release checkout report parent does not exist: $releaseCheckoutReportParent"
    }
    [System.IO.File]::WriteAllText($releaseCheckoutReportPath, ($releaseCheckoutEvidence | ConvertTo-Json -Depth 12))
    Write-Output "release checkout prepared; report=$releaseCheckoutReportPath"
    return
}

if ($ObserverIdentityFixture) {
    Invoke-ObserverIdentityFixture -RequestedInstallDir $InstallDir -RequestedReportPath $ObserverReportPath
    return
}

if ($ObserverFixture) {
    Invoke-ObserverFixture -RequestedInstallDir $InstallDir -RequestedReportPath $ObserverReportPath -RootCommand $ObserverRootCommand -RootArgumentList $ObserverRootArgumentList -RootWorkingDirectory $ObserverRootWorkingDirectory -RootStdoutPath $ObserverRootStdoutPath -RootStderrPath $ObserverRootStderrPath -RootOutputMaximumBytes $ObserverRootOutputMaximumBytes -RequestedRootExitCode $ObserverRootExitCode -ObserverReleaseCheckoutPath $ObserverReleaseCheckoutPath -TimeoutSeconds $ObserverTimeoutSeconds
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
