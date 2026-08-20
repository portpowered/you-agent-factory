#!/usr/bin/env pwsh

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Apply = $false
$BaseRef = 'HEAD'
$Force = $false

function Show-Usage {
    @'
Usage: pwsh ./scripts/cleanup-merged-worktrees.ps1 [--base <git-ref>] [--apply] [--force]

Defaults to a dry run.
Use --apply to remove clean merged worktrees.
Use --apply --force to remove dirty merged worktrees too.
'@
}

for ($Index = 0; $Index -lt $args.Count; $Index++) {
    switch ($args[$Index]) {
        '--apply' {
            $Apply = $true
        }

        '--base' {
            if ($Index + 1 -ge $args.Count) {
                [Console]::Error.WriteLine('Missing value for --base.')
                [Console]::Error.WriteLine()
                [Console]::Error.WriteLine((Show-Usage))
                exit 1
            }

            $Index++
            $BaseRef = $args[$Index]
        }

        '--force' {
            $Force = $true
        }

        { $_ -in '--help', '-h' } {
            Show-Usage
            exit 0
        }

        default {
            [Console]::Error.WriteLine("Unknown argument: $($args[$Index])")
            [Console]::Error.WriteLine()
            [Console]::Error.WriteLine((Show-Usage))
            exit 1
        }
    }
}

function Invoke-Git {
    param(
        [Parameter(Mandatory)]
        [string[]] $Arguments,

        [string] $WorkingDirectory
    )

    if ($WorkingDirectory) {
        $Output = & git -C $WorkingDirectory @Arguments 2>&1
    }
    else {
        $Output = & git @Arguments 2>&1
    }

    if ($LASTEXITCODE -ne 0) {
        $Message = $Output -join [Environment]::NewLine
        throw "git $($Arguments -join ' ') failed:`n$Message"
    }

    return $Output
}

$CurrentWorktreePath = (
    Invoke-Git -Arguments @('rev-parse', '--show-toplevel')
).Trim()

$CandidateBranches = [System.Collections.Generic.List[string]]::new()
$CandidatePaths = [System.Collections.Generic.List[string]]::new()
$CandidateStatuses = [System.Collections.Generic.List[string]]::new()

$CurrentPath = $null
$CurrentBranch = $null

function Add-CurrentWorktreeCandidate {
    if (
        [string]::IsNullOrWhiteSpace($script:CurrentPath) -or
        [string]::IsNullOrWhiteSpace($script:CurrentBranch)
    ) {
        $script:CurrentPath = $null
        $script:CurrentBranch = $null
        return
    }

    $ResolvedCurrentPath = [System.IO.Path]::GetFullPath($script:CurrentPath)
    $ResolvedMainPath = [System.IO.Path]::GetFullPath($script:CurrentWorktreePath)

    if ($ResolvedCurrentPath -eq $ResolvedMainPath) {
        $script:CurrentPath = $null
        $script:CurrentBranch = $null
        return
    }

    & git merge-base --is-ancestor $script:CurrentBranch $script:BaseRef
    $IsMerged = $LASTEXITCODE -eq 0

    if (-not $IsMerged) {
        $script:CurrentPath = $null
        $script:CurrentBranch = $null
        return
    }

    $Status = 'clean'

    if (-not (Test-Path -LiteralPath $script:CurrentPath -PathType Container)) {
        $Status = 'missing'
    }
    else {
        $GitStatus = Invoke-Git `
            -WorkingDirectory $script:CurrentPath `
            -Arguments @('status', '--short', '--untracked-files=no')

        if ($GitStatus.Count -gt 0) {
            $Status = 'dirty'
        }
    }

    $script:CandidateBranches.Add($script:CurrentBranch)
    $script:CandidatePaths.Add($script:CurrentPath)
    $script:CandidateStatuses.Add($Status)

    $script:CurrentPath = $null
    $script:CurrentBranch = $null
}

$WorktreeOutput = Invoke-Git -Arguments @('worktree', 'list', '--porcelain')

foreach ($Line in @($WorktreeOutput) + '') {
    if ([string]::IsNullOrEmpty($Line)) {
        Add-CurrentWorktreeCandidate
        continue
    }

    if ($Line.StartsWith('worktree ')) {
        Add-CurrentWorktreeCandidate
        $CurrentPath = $Line.Substring('worktree '.Length)
        continue
    }

    if ($Line.StartsWith('branch refs/heads/')) {
        $CurrentBranch = $Line.Substring('branch refs/heads/'.Length)
    }
}

Add-CurrentWorktreeCandidate

$CandidateCount = $CandidatePaths.Count

if ($CandidateCount -eq 0) {
    Write-Output "No merged worktrees found for base $BaseRef."
    exit 0
}

Write-Output "Found $CandidateCount merged worktree(s) for base ${BaseRef}:"

for ($Index = 0; $Index -lt $CandidateCount; $Index++) {
    Write-Output (
        "- {0}: {1} ({2})" -f
        $CandidateBranches[$Index],
        $CandidatePaths[$Index],
        $CandidateStatuses[$Index]
    )
}

if (-not $Apply) {
    Write-Output ''
    Write-Output 'Dry run only. Re-run with --apply to remove clean worktrees, or --apply --force to remove dirty ones too.'
    exit 0
}

$RemovedCount = 0
$SkippedDirtyCount = 0
$MissingCount = 0

Write-Output ''

for ($Index = 0; $Index -lt $CandidateCount; $Index++) {
    $Branch = $CandidateBranches[$Index]
    $Path = $CandidatePaths[$Index]
    $Status = $CandidateStatuses[$Index]

    if ($Status -eq 'missing') {
        Write-Output "Worktree path already missing for ${Branch}: $Path"
        $MissingCount++
        continue
    }

    if ($Status -eq 'dirty' -and -not $Force) {
        Write-Output "Skipping dirty worktree ${Branch}: $Path"
        $SkippedDirtyCount++
        continue
    }

    Write-Output "Removing ${Branch}: $Path"

    $RemoveArguments = @('worktree', 'remove')

    if ($Force) {
        $RemoveArguments += '--force'
    }

    $RemoveArguments += $Path
    Invoke-Git -Arguments $RemoveArguments | Out-Null

    $RemovedCount++
}

if ($RemovedCount -gt 0 -or $MissingCount -gt 0) {
    Write-Output ''
    Write-Output 'Pruning stale worktree metadata.'
    Invoke-Git -Arguments @('worktree', 'prune') | Out-Null
}

if ($RemovedCount -eq 0) {
    Write-Output ''
    Write-Output 'Nothing removed.'
}

if ($SkippedDirtyCount -gt 0) {
    Write-Output ''
    Write-Output "Skipped $SkippedDirtyCount dirty merged worktree(s). Re-run with --apply --force if that is intentional."
}

if ($MissingCount -gt 0) {
    Write-Output ''
    Write-Output "Found $MissingCount merged worktree(s) that were already missing on disk. Their metadata is cleaned up by prune during apply runs."
}