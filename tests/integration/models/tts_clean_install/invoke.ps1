[CmdletBinding()]
param(
    [string]$ArtifactPath,
    [string]$ArtifactIdentity,
    [string]$ArtifactSha256,
    [string]$ManifestPath,
    [string]$ReportPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function New-NotRunPhase {
    return [ordered]@{
        status = 'NOT_RUN'
        evidence = @()
        unprovenEdge = 'later TTS journey story'
    }
}

function New-EmptyReport {
    $emptySha = (New-Object System.Security.Cryptography.SHA256Managed).ComputeHash([byte[]]@()) | ForEach-Object { $_.ToString('x2') } | Join-String
    return [ordered]@{
        schemaVersion = 1
        verdict = 'INCONCLUSIVE'
        environment = [ordered]@{
            platform = 'windows'
            architecture = 'amd64'
            roots = [ordered]@{
                output = 'not-established'; work = 'not-established'; profile = 'not-established'; state = 'not-established'
                cache = 'not-established'; temp = 'not-established'; streams = 'not-established'; runtime = 'not-established'
            }
            limits = [ordered]@{}
        }
        identities = @([ordered]@{ name = 'preflight'; identity = 'not-established'; path = 'not-established'; bytes = 0; sha256 = $emptySha })
        preflight = [ordered]@{ completedBeforeEffects = $true; childStarts = 0; listenerOpens = 0; networkAttempts = 0; checks = @() }
        commands = @()
        journeys = [ordered]@{ install = (New-NotRunPhase); discovery = (New-NotRunPhase); cold = (New-NotRunPhase); warmOffline = (New-NotRunPhase); removal = (New-NotRunPhase) }
        audio = @(
            [ordered]@{ name = 'cold-tts.wav'; mediaType = 'audio/wav'; bytes = 0; sha256 = $emptySha; riffDecoded = $false; durationMillis = 0; nonSilentSamples = 0 },
            [ordered]@{ name = 'warm-offline-tts.wav'; mediaType = 'audio/wav'; bytes = 0; sha256 = $emptySha; riffDecoded = $false; durationMillis = 0; nonSilentSamples = 0 }
        )
        readiness = @()
        network = [ordered]@{ policy = 'none'; attempts = 0; warmOfflineAttempts = 0 }
        cleanup = [ordered]@{ ownedProcesses = 0; ownedListeners = 0; ownedRoots = 0; survivingProcesses = 0; survivingListeners = 0; removedRuntimeRoots = 0 }
        criteria = @()
        findings = @()
    }
}

function Add-Finding {
    param([System.Collections.IDictionary]$Report, [string]$Check, [string]$Expected, [string]$Observed)
    $Report.verdict = 'FAIL'
    $Report.preflight.checks += [ordered]@{ id = $Check; status = 'FAIL'; expected = $Expected; observed = $Observed.Substring(0, [Math]::Min(256, $Observed.Length)) }
    $Report.criteria = @(
        [ordered]@{ id = 'TTS-PROBE-01'; verdict = 'FAIL'; evidence = @('preflight rejected the input before effects'); unprovenEdge = $null },
        [ordered]@{ id = 'TTS-PROBE-02'; verdict = 'INCONCLUSIVE'; evidence = @(); unprovenEdge = 'later TTS journey and OS execution stories' },
        [ordered]@{ id = 'TTS-PROBE-03'; verdict = 'INCONCLUSIVE'; evidence = @(); unprovenEdge = 'later TTS journey and OS execution stories' }
    )
    $Report.findings += [ordered]@{ id = 'TTS-PROBE-01'; severity = 'FAIL'; check = $Check; expected = $Expected; observed = $Observed.Substring(0, [Math]::Min(256, $Observed.Length)) }
}

function Is-AbsolutePath {
    param([string]$Path)
    return -not [string]::IsNullOrWhiteSpace($Path) -and [System.IO.Path]::IsPathFullyQualified($Path)
}

function Normalize-Path {
    param([string]$Path)
    if (-not (Is-AbsolutePath $Path)) { throw 'path is not absolute' }
    return [System.IO.Path]::GetFullPath($Path)
}

function Assert-Properties {
    param([object]$Value, [string[]]$Required, [string[]]$Allowed, [string]$Name)
    if ($null -eq $Value) { throw "$Name is missing" }
    $properties = @($Value.PSObject.Properties.Name)
    foreach ($requiredName in $Required) {
        if ($properties -notcontains $requiredName) { throw "$Name.$requiredName is missing" }
    }
    foreach ($property in $properties) {
        if ($Allowed -notcontains $property) { throw "$Name.$property is not allowed" }
    }
}

function Assert-FileRecord {
    param([object]$Value, [string]$Name, [bool]$Named)
    $required = if ($Named) { @('name', 'path', 'identity', 'sha256') } else { @('path', 'identity', 'sha256') }
    $allowed = $required
    Assert-Properties $Value $required $allowed $Name
    if ([string]::IsNullOrWhiteSpace([string]$Value.identity)) { throw "$Name.identity is empty" }
    if (-not (Is-AbsolutePath ([string]$Value.path))) { throw "$Name.path is not absolute" }
    if (-not ([string]$Value.sha256 -cmatch '^[0-9a-f]{64}$')) { throw "$Name.sha256 is not lowercase SHA-256" }
    if ($Named -and [string]::IsNullOrWhiteSpace([string]$Value.name)) { throw "$Name.name is empty" }
}

function Get-FileRecord {
    param([string]$Name, [object]$Value, [bool]$Text)
    Assert-FileRecord $Value $Name ($Value.PSObject.Properties.Name -contains 'name')
    $path = Normalize-Path ([string]$Value.path)
    $item = Get-Item -LiteralPath $path -Force -ErrorAction Stop
    if (-not $item.PSIsContainer -and $item.LinkType) { throw "$Name is a reparse-point file" }
    if ($item.PSIsContainer) { throw "$Name is not a regular file" }
    $bytes = [System.IO.File]::ReadAllBytes($path)
    if ($Text) {
        try { [System.Text.UTF8Encoding]::new($false, $true).GetString($bytes) | Out-Null }
        catch { throw "$Name is not valid UTF-8" }
    }
    $hash = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($hash -cne ([string]$Value.sha256)) { throw "$Name SHA-256 does not match its declared identity" }
    return [ordered]@{ name = $Name; identity = [string]$Value.identity; path = ('sha256:' + (([System.Security.Cryptography.SHA256]::Create()).ComputeHash([System.Text.Encoding]::UTF8.GetBytes($path)) | ForEach-Object { $_.ToString('x2') } | Join-String)); bytes = $bytes.Length; sha256 = $hash }
}

function Write-AtomicReport {
    param([string]$Path, [System.Collections.IDictionary]$Report)
    $fullPath = Normalize-Path $Path
    $parent = [System.IO.Path]::GetDirectoryName($fullPath)
    if (-not (Test-Path -LiteralPath $parent -PathType Container)) { throw 'report parent does not exist' }
    $json = $Report | ConvertTo-Json -Depth 20
    $temporary = Join-Path $parent ('.tts-report-' + [Guid]::NewGuid().ToString('N') + '.tmp')
    try {
        [System.IO.File]::WriteAllText($temporary, $json + [Environment]::NewLine, [System.Text.UTF8Encoding]::new($false))
        Move-Item -LiteralPath $temporary -Destination $fullPath -Force
    }
    finally {
        if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue }
    }
}

$report = New-EmptyReport
try {
    if (-not (Is-AbsolutePath $ArtifactPath) -or -not (Is-AbsolutePath $ManifestPath) -or -not (Is-AbsolutePath $ReportPath)) { throw 'all paths must be absolute' }
    if ([string]::IsNullOrWhiteSpace($ArtifactIdentity)) { throw 'artifact identity is required' }
    if ($ArtifactSha256 -cnotmatch '^[0-9a-f]{64}$') { throw 'artifact SHA-256 must be lowercase hexadecimal' }

    $artifact = Normalize-Path $ArtifactPath
    $manifestPath = Normalize-Path $ManifestPath
    $reportPath = Normalize-Path $ReportPath
    $outputRoot = [System.IO.Path]::GetDirectoryName($reportPath)
    if (-not (Test-Path -LiteralPath $outputRoot -PathType Container)) { throw 'report parent must be an existing empty output root' }
    if (@(Get-ChildItem -LiteralPath $outputRoot -Force).Count -ne 0) { throw 'report parent must be empty' }
    if ($artifact -ieq $manifestPath -or $artifact -ieq $reportPath -or $manifestPath -ieq $reportPath) { throw 'manifest, artifact, and report paths must be distinct' }

    $manifestItem = Get-Item -LiteralPath $manifestPath -Force -ErrorAction Stop
    if ($manifestItem.PSIsContainer -or $manifestItem.LinkType) { throw 'manifest is not a regular file' }
    $manifestBytes = [System.IO.File]::ReadAllBytes($manifestPath)
    if ($manifestBytes.Length -eq 0 -or $manifestBytes.Length -gt 1048576) { throw 'manifest size is outside the bounded range' }
    $manifestJson = [System.Text.UTF8Encoding]::new($false, $true).GetString($manifestBytes) | ConvertFrom-Json -ErrorAction Stop
    Assert-Properties $manifestJson @('schemaVersion', 'build', 'distribution', 'acceptance', 'publicDocs', 'fixtures', 'limits') @('schemaVersion', 'build', 'distribution', 'acceptance', 'publicDocs', 'fixtures', 'limits') 'manifest'
    if ([int]$manifestJson.schemaVersion -ne 1) { throw 'manifest schemaVersion must be 1' }
    Assert-Properties $manifestJson.build @('identity', 'platform', 'architecture', 'artifactPath', 'artifactKind', 'sha256') @('identity', 'platform', 'architecture', 'artifactPath', 'artifactKind', 'sha256') 'manifest.build'
    if ([string]$manifestJson.build.platform -cne 'windows' -or [string]$manifestJson.build.architecture -cne 'amd64') { throw 'manifest build target must be windows/amd64' }
    if ([string]$manifestJson.build.artifactKind -notin @('installer', 'archive', 'executable')) { throw 'manifest artifact kind is unsupported' }
    Assert-Properties $manifestJson.distribution @('installer', 'archive', 'checksums') @('installer', 'archive', 'checksums') 'manifest.distribution'
    Assert-FileRecord $manifestJson.distribution.installer 'manifest.distribution.installer' $false
    Assert-FileRecord $manifestJson.distribution.archive 'manifest.distribution.archive' $false
    Assert-FileRecord $manifestJson.distribution.checksums 'manifest.distribution.checksums' $false
    Assert-FileRecord $manifestJson.acceptance 'manifest.acceptance' $false
    if (@($manifestJson.publicDocs).Count -lt 1) { throw 'manifest.publicDocs must not be empty' }
    foreach ($doc in @($manifestJson.publicDocs)) { Assert-FileRecord $doc 'manifest.publicDocs' $true }
    Assert-Properties $manifestJson.fixtures @('text') @('text', 'voice') 'manifest.fixtures'
    Assert-FileRecord $manifestJson.fixtures.text 'manifest.fixtures.text' $true
    if ($null -ne $manifestJson.fixtures.voice) { Assert-FileRecord $manifestJson.fixtures.voice 'manifest.fixtures.voice' $true }
    Assert-Properties $manifestJson.limits @('timeoutSeconds', 'diskBytes', 'downloadBytes', 'paidUsd', 'maxOwnedProcesses', 'networkPolicy') @('timeoutSeconds', 'diskBytes', 'downloadBytes', 'paidUsd', 'maxOwnedProcesses', 'networkPolicy') 'manifest.limits'
    if ([int]$manifestJson.limits.timeoutSeconds -lt 1 -or [int64]$manifestJson.limits.diskBytes -lt 1 -or [int64]$manifestJson.limits.downloadBytes -lt 0 -or [double]$manifestJson.limits.paidUsd -ne 0 -or [int]$manifestJson.limits.maxOwnedProcesses -lt 1 -or [int]$manifestJson.limits.maxOwnedProcesses -gt 4) { throw 'manifest limits are outside the declared range' }
    if ([string]$manifestJson.limits.networkPolicy -notin @('none', 'staged-loopback-and-declared-public-origins')) { throw 'manifest network policy is unsupported' }
    if ((Normalize-Path ([string]$manifestJson.build.artifactPath)) -ine $artifact) { throw 'invocation artifact path does not match manifest build path' }
    if ([string]$manifestJson.build.identity -cne $ArtifactIdentity) { throw 'invocation artifact identity does not match manifest build identity' }
    if ([string]$manifestJson.build.sha256 -cne $ArtifactSha256) { throw 'invocation artifact hash does not match manifest build hash' }

    $paths = @{}
    $paths[$artifact.ToLowerInvariant()] = 'artifact'
    $totalBytes = [int64]0
    $records = @()
    $artifactRecord = Get-FileRecord 'artifact' ([pscustomobject]@{ path = $manifestJson.build.artifactPath; identity = $manifestJson.build.identity; sha256 = $manifestJson.build.sha256 }) $false
    $records += $artifactRecord
    $totalBytes += [int64]$artifactRecord.bytes
    $fileInputs = @(
        @{ name = 'distribution.installer'; value = $manifestJson.distribution.installer; text = $false },
        @{ name = 'distribution.archive'; value = $manifestJson.distribution.archive; text = $false },
        @{ name = 'distribution.checksums'; value = $manifestJson.distribution.checksums; text = $true },
        @{ name = 'acceptance'; value = $manifestJson.acceptance; text = $true },
        @{ name = 'fixtures.text'; value = $manifestJson.fixtures.text; text = $true }
    )
    foreach ($doc in @($manifestJson.publicDocs)) { $fileInputs += @{ name = 'publicDocs.' + [string]$doc.name; value = $doc; text = $true } }
    if ($null -ne $manifestJson.fixtures.voice) { $fileInputs += @{ name = 'fixtures.voice.' + [string]$manifestJson.fixtures.voice.name; value = $manifestJson.fixtures.voice; text = $false } }
    foreach ($input in $fileInputs) {
        $path = Normalize-Path ([string]$input.value.path)
        $key = $path.ToLowerInvariant()
        if ($paths.ContainsKey($key)) { throw "$($input.name) duplicates $($paths[$key])" }
        if ($path -ieq $manifestPath -or $path -ieq $reportPath) { throw "$($input.name) aliases a control path" }
        $paths[$key] = [string]$input.name
        $record = Get-FileRecord ([string]$input.name) $input.value ([bool]$input.text)
        $records += $record
        $totalBytes += [int64]$record.bytes
    }
    if ($totalBytes -gt [int64]$manifestJson.limits.diskBytes) { throw 'immutable inputs exceed limits.diskBytes' }
    $manifestHash = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $manifestRecord = [ordered]@{ name = 'manifest'; identity = 'schema-v1'; path = 'sha256:' + $manifestHash; bytes = $manifestBytes.Length; sha256 = $manifestHash }
    $records = @($manifestRecord) + @($records)
    $childRoots = [ordered]@{
        output = $outputRoot
        work = Join-Path $outputRoot 'work'
        profile = Join-Path $outputRoot 'profile'
        state = Join-Path $outputRoot 'state'
        cache = Join-Path $outputRoot 'cache'
        temp = Join-Path $outputRoot 'temp'
        streams = Join-Path $outputRoot 'streams'
        runtime = Join-Path $outputRoot 'runtime'
    }
    foreach ($rootName in @('work', 'profile', 'state', 'cache', 'temp', 'streams', 'runtime')) {
        if (-not ((Normalize-Path $childRoots[$rootName]).StartsWith($outputRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase))) { throw "$rootName root escapes output root" }
    }
    $pathHasher = [System.Security.Cryptography.SHA256]::Create()
    try {
        foreach ($rootName in @($childRoots.Keys)) {
            $rootBytes = [Text.Encoding]::UTF8.GetBytes((Normalize-Path $childRoots[$rootName]))
            $childRoots[$rootName] = 'sha256:' + (($pathHasher.ComputeHash($rootBytes) | ForEach-Object { $_.ToString('x2') }) -join '')
        }
    }
    finally { $pathHasher.Dispose() }
    $report.environment.platform = 'windows'
    $report.environment.architecture = 'amd64'
    $report.environment.roots = $childRoots
    $report.environment.limits = $manifestJson.limits
    $report.identities = $records
    $report.criteria = @(
        [ordered]@{ id = 'TTS-PROBE-01'; verdict = 'PASS'; evidence = @('sealed immutable inputs and empty output root'); unprovenEdge = '' },
        [ordered]@{ id = 'TTS-PROBE-02'; verdict = 'INCONCLUSIVE'; evidence = @(); unprovenEdge = 'later TTS journey and OS execution stories' },
        [ordered]@{ id = 'TTS-PROBE-03'; verdict = 'INCONCLUSIVE'; evidence = @(); unprovenEdge = 'later TTS journey and OS execution stories' }
    )
    Write-AtomicReport $reportPath $report
    exit 3
}
catch {
    try {
        Add-Finding $report 'preflight' 'complete immutable inputs and isolated output root' $_.Exception.Message
        if (-not [string]::IsNullOrWhiteSpace($ReportPath) -and (Is-AbsolutePath $ReportPath)) {
            $candidate = Normalize-Path $ReportPath
            $parent = [System.IO.Path]::GetDirectoryName($candidate)
            if ((Test-Path -LiteralPath $parent -PathType Container) -and (@(Get-ChildItem -LiteralPath $parent -Force).Count -eq 0)) { Write-AtomicReport $candidate $report }
        }
    }
    catch { }
    exit 2
}
