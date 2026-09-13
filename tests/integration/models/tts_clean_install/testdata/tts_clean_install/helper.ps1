[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('burst', 'tree', 'descendant', 'listener', 'sentinel', 'journey')]
    [string]$Mode,
    [string]$ReadyPath,
    [string]$DescendantReadyPath,
    [string]$OutputRoot,
    [string]$ArtifactPath,
    [int]$OutputBytes = 0,
    [switch]$Offline,
    [string]$FailPhase
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:utf8NoBom = [System.Text.UTF8Encoding]::new($false)

function Write-HelperEvent {
    param([System.Collections.IDictionary]$Event)

    [Console]::Out.WriteLine(($Event | ConvertTo-Json -Compress -Depth 8))
}

function Write-HelperText {
    param([string]$Path, [string]$Value)

    $parent = [System.IO.Path]::GetDirectoryName([System.IO.Path]::GetFullPath($Path))
    if (-not (Test-Path -LiteralPath $parent -PathType Container)) {
        [void](New-Item -ItemType Directory -Path $parent -Force)
    }
    [System.IO.File]::WriteAllText($Path, $Value, $script:utf8NoBom)
}

function Wait-HelperFile {
    param([string]$Path)

    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $parent = [System.IO.Path]::GetDirectoryName($fullPath)
    $name = [System.IO.Path]::GetFileName($fullPath)
    if (Test-Path -LiteralPath $fullPath -PathType Leaf) {
        return
    }
    $watcher = [System.IO.FileSystemWatcher]::new($parent, $name)
    $watcher.NotifyFilter = [System.IO.NotifyFilters]::FileName -bor [System.IO.NotifyFilters]::LastWrite
    $watcher.EnableRaisingEvents = $true
    try {
        if (Test-Path -LiteralPath $fullPath -PathType Leaf) {
            return
        }
        $change = $watcher.WaitForChanged(
            [System.IO.WatcherChangeTypes]::Created -bor [System.IO.WatcherChangeTypes]::Changed,
            10000
        )
        if ($change.TimedOut -or -not (Test-Path -LiteralPath $fullPath -PathType Leaf)) {
            throw "helper readiness file was not observed: $fullPath"
        }
    }
    finally {
        $watcher.Dispose()
    }
}

function Hold-HelperProcess {
    # This is an explicitly held process fixture. Readiness is emitted before
    # this loop; the loop is never used as a synchronization mechanism.
    while ($true) {
        Start-Sleep -Seconds 1
    }
}

function Start-HelperChild {
    param([string[]]$Arguments)

    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = Join-Path $PSHOME 'pwsh.exe'
    $startInfo.UseShellExecute = $false
    foreach ($argument in $Arguments) {
        [void]$startInfo.ArgumentList.Add($argument)
    }
    $child = [System.Diagnostics.Process]::Start($startInfo)
    if ($null -eq $child) {
        throw 'helper child did not start'
    }
    return $child
}

function New-Pcm16Wav {
    param([int[]]$Samples, [int]$SampleRate = 8000)

    $stream = [System.IO.MemoryStream]::new()
    $writer = [System.IO.BinaryWriter]::new($stream, [System.Text.Encoding]::ASCII, $true)
    try {
        $writer.Write([System.Text.Encoding]::ASCII.GetBytes('RIFF'))
        $writer.Write([int32]0)
        $writer.Write([System.Text.Encoding]::ASCII.GetBytes('WAVE'))
        $writer.Write([System.Text.Encoding]::ASCII.GetBytes('fmt '))
        $writer.Write([int32]16)
        $writer.Write([int16]1)
        $writer.Write([int16]1)
        $writer.Write([int32]$SampleRate)
        $writer.Write([int32]($SampleRate * 2))
        $writer.Write([int16]2)
        $writer.Write([int16]16)
        $writer.Write([System.Text.Encoding]::ASCII.GetBytes('data'))
        $writer.Write([int32]($Samples.Count * 2))
        foreach ($sample in $Samples) {
            $writer.Write([int16]$sample)
        }
        $writer.Flush()
        $bytes = $stream.ToArray()
        $riffLength = [BitConverter]::GetBytes([int32]($bytes.Length - 8))
        [Array]::Copy($riffLength, 0, $bytes, 4, 4)
        return ,$bytes
    }
    finally {
        $writer.Dispose()
        $stream.Dispose()
    }
}

function New-ControlledRoots {
    param([string]$Root)

    foreach ($name in @('work', 'profile', 'state', 'cache', 'temp', 'streams', 'runtime')) {
        [void](New-Item -ItemType Directory -Path (Join-Path $Root $name) -Force)
    }
}

function Get-PublicArguments {
    param([string]$Phase)

    switch ($Phase) {
        'install' { return @('you', 'models', 'install') }
        'installed-identity' { return @('you', 'version') }
        'help' { return @('you', '--help') }
        'docs' { return @('you', 'docs', 'models') }
        'models-list' { return @('you', 'models', 'list') }
        'models-inspect' { return @('you', 'models', 'inspect') }
        'cold-invoke' { return @('you', 'models', 'invoke', '--text', '<redacted-text>') }
        'cold-readiness' { return @('you', 'models', 'status') }
        'warm-offline-invoke' { return @('you', 'models', 'invoke', '--offline', '--text', '<redacted-text>') }
        'warm-offline-readiness' { return @('you', 'models', 'status') }
        'remove' { return @('you', 'models', 'remove') }
        'post-remove' { return @('you', 'models', 'inspect') }
        'cleanup' { return @('probe', 'cleanup-owned-roots') }
        default { return @('you', 'models', $Phase) }
    }
}

function Write-JourneyPhase {
    param([string]$Phase, [string]$Status = 'PASS', [string]$Evidence = '')

    $event = [ordered]@{
        event = 'phase'
        phase = $Phase
        status = $Status
        evidence = @($Evidence)
        argv = @(Get-PublicArguments $Phase)
        networkAttempts = 0
    }
    if ($Phase -eq 'cold-readiness') {
        $event.readinessState = 'READY'
        $event.lifecycleState = 'LOADED'
        $event.cacheBytes = 0
        $event.cacheReused = $false
    }
    if ($Phase -eq 'warm-offline-readiness') {
        $event.readinessState = 'READY'
        $event.lifecycleState = 'LOADED'
        $event.cacheBytes = [int64](Get-Item -LiteralPath (Join-Path $OutputRoot 'cache/controlled-cache.marker')).Length
        $event.cacheReused = $true
    }
    Write-HelperEvent $event
}

function Run-Burst {
    if ($OutputBytes -lt 131072) {
        throw 'burst output must exceed both ordinary pipe buffers'
    }
    $payload = [byte[]]::new($OutputBytes)
    for ($index = 0; $index -lt $payload.Length; $index++) {
        $payload[$index] = [byte](($index * 31 + 7) % 251)
    }
    $stdout = [Console]::OpenStandardOutput()
    $stderr = [Console]::OpenStandardError()
    $stdout.Write($payload, 0, $payload.Length)
    $stdout.Flush()
    $stderr.Write($payload, 0, $payload.Length)
    $stderr.Flush()
}

function Run-Tree {
    if ([string]::IsNullOrWhiteSpace($ReadyPath) -or [string]::IsNullOrWhiteSpace($DescendantReadyPath)) {
        throw 'tree mode requires readiness paths'
    }
    $child = Start-HelperChild @(
        '-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass',
        '-File', $PSCommandPath, '-Mode', 'descendant', '-ReadyPath', $DescendantReadyPath
    )
    Wait-HelperFile $DescendantReadyPath
    Write-HelperEvent ([ordered]@{ event = 'ready'; pid = $PID; descendantPid = $child.Id })
    Hold-HelperProcess
}

function Run-Descendant {
    if ([string]::IsNullOrWhiteSpace($ReadyPath)) {
        throw 'descendant mode requires a readiness path'
    }
    Write-HelperText $ReadyPath ([string]$PID)
    Write-HelperEvent ([ordered]@{ event = 'descendant-ready'; pid = $PID })
    Hold-HelperProcess
}

function Run-Listener {
    if ([string]::IsNullOrWhiteSpace($ReadyPath)) {
        throw 'listener mode requires a readiness path'
    }
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Parse('127.0.0.1'), 0)
    try {
        $listener.Start()
        $port = ([System.Net.IPEndPoint]$listener.LocalEndpoint).Port
        if ($port -eq 7437) {
            throw 'dynamic listener selected forbidden port 7437'
        }
        Write-HelperText $ReadyPath ([string]$port)
        Write-HelperEvent ([ordered]@{ event = 'listener-ready'; pid = $PID; address = '127.0.0.1'; port = $port })
        Hold-HelperProcess
    }
    finally {
        $listener.Stop()
    }
}

function Run-Sentinel {
    if ([string]::IsNullOrWhiteSpace($ReadyPath)) {
        throw 'sentinel mode requires a readiness path'
    }
    Write-HelperText $ReadyPath ([string]$PID)
    Write-HelperEvent ([ordered]@{ event = 'sentinel-ready'; pid = $PID })
    Hold-HelperProcess
}

function Run-Journey {
    if ([string]::IsNullOrWhiteSpace($OutputRoot) -or [string]::IsNullOrWhiteSpace($ArtifactPath)) {
        throw 'journey mode requires output and artifact paths'
    }
    New-ControlledRoots $OutputRoot
    $installedPath = Join-Path $OutputRoot 'work/installed-build.bin'
    Copy-Item -LiteralPath $ArtifactPath -Destination $installedPath -Force
    $audioRoot = Join-Path $OutputRoot 'runtime/audio'
    [void](New-Item -ItemType Directory -Path $audioRoot -Force)
    $phases = @(
        'install', 'installed-identity', 'help', 'docs', 'models-list', 'models-inspect',
        'cold-invoke', 'cold-readiness', 'warm-offline-invoke', 'warm-offline-readiness',
        'remove', 'post-remove', 'cleanup'
    )
    foreach ($phase in $phases) {
        if ($FailPhase -eq $phase) {
            Write-JourneyPhase $phase 'FAIL' 'controlled phase failure'
            exit 17
        }
        switch ($phase) {
            'cold-invoke' {
                $cold = New-Pcm16Wav @(1, 2, 3, 4, 5, 6, 7, 8)
                [System.IO.File]::WriteAllBytes((Join-Path $audioRoot 'cold-tts.wav'), $cold)
            }
            'warm-offline-invoke' {
                if (-not $Offline) {
                    throw 'warm-offline-invoke requires --offline'
                }
                Write-HelperText (Join-Path $OutputRoot 'cache/controlled-cache.marker') 'controlled cache reuse marker'
                $warm = New-Pcm16Wav @(8, 7, 6, 5, 4, 3, 2, 1)
                [System.IO.File]::WriteAllBytes((Join-Path $audioRoot 'warm-offline-tts.wav'), $warm)
            }
            'remove' {
                Remove-Item -LiteralPath $installedPath -Force
                Remove-Item -LiteralPath (Join-Path $OutputRoot 'cache/controlled-cache.marker') -Force
            }
            'post-remove' {
                if (Test-Path -LiteralPath $installedPath -PathType Leaf) {
                    throw 'installed artifact survived public remove'
                }
                if (Test-Path -LiteralPath (Join-Path $OutputRoot 'cache/controlled-cache.marker') -PathType Leaf) {
                    throw 'cache survived public remove'
                }
            }
        }
        Write-JourneyPhase $phase 'PASS' ($phase + ' observed')
    }
}

switch ($Mode) {
    'burst' { Run-Burst }
    'tree' { Run-Tree }
    'descendant' { Run-Descendant }
    'listener' { Run-Listener }
    'sentinel' { Run-Sentinel }
    'journey' { Run-Journey }
}
