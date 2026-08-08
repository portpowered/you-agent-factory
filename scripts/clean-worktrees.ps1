
#!/usr/bin/env pwsh

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Apply = $false
$BaseRef = 'HEAD'
$Force = $false

function Show-Usage {
    @'
Usage: pwsh ./scripts/clean-worktrees.ps1 [--base <git-ref>] [--apply] [--force]

Defaults to a dry run.
Use --apply to remove clean merged worktrees.
Use --apply --force to remove dirty merged worktrees too.
'@
}

for ($ArgumentIndex = 0; $ArgumentIndex -lt $args.Count; $ArgumentIndex++) {
    switch ($args[$ArgumentIndex]) {
        '--apply' {
            $Apply = $true
        }

        '--base' {
            if ($ArgumentIndex + 1 -ge $args.Count) {
                [Console]::Error.WriteLine('Missing value for --base.')
                [Console]::Error.WriteLine()
                [Console]::Error.WriteLine((Show-Usage))
                exit 1
            }

            $ArgumentIndex++
            $BaseRef = $args[$ArgumentIndex]
        }

        '--force' {
            $Force = $true
        }

        { $_ -in @('--help', '-h') } {
            Show-Usage
            exit 0
        }

        default {
            [Console]::Error.WriteLine(
                "Unknown argument: $($args[$ArgumentIndex])"
            )
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

    if ([string]::IsNullOrWhiteSpace($WorkingDirectory)) {
        $Output = @(& git @Arguments 2>&1)
    }
    else {
        $Output = @(& git -C $WorkingDirectory @Arguments 2>&1)
    }

    $ExitCode = $LASTEXITCODE

    if ($ExitCode -ne 0) {
        $Message = $Output -join [Environment]::NewLine

        throw @"
git $($Arguments -join ' ') failed with exit code $ExitCode.
$Message
"@
    }

    $Output
}

$TopLevelOutput = @(
    Invoke-Git -Arguments @('rev-parse', '--show-toplevel')
)

if ($TopLevelOutput.Count -eq 0) {
    throw 'Git did not return a repository root path.'
}

$CurrentWorktreePath = ([string]$TopLevelOutput[0]).Trim()

$CandidateBranches = [System.Collections.Generic.List[string]]::new()
$CandidatePaths = [System.Collections.Generic.List[string]]::new()
$CandidateStatuses = [System.Collections.Generic.List[string]]::new()

$CurrentPath = $null
$CurrentBranch = $null

function Clear-CurrentWorktree {
    $script:CurrentPath = $null
    $script:CurrentBranch = $null
}

function Add-CurrentWorktreeCandidate {
    if (
        [string]::IsNullOrWhiteSpace($script:CurrentPath) -or
        [string]::IsNullOrWhiteSpace($script:CurrentBranch)
    ) {
        Clear-CurrentWorktree
        return
    }

    $WorktreeFullPath = [System.IO.Path]::GetFullPath(
        $script:CurrentPath
    )

    $CurrentFullPath = [System.IO.Path]::GetFullPath(
        $script:CurrentWorktreePath
    )

    $PathComparison = if ($IsWindows) {
        [System.StringComparison]::OrdinalIgnoreCase
    }
    else {
        [System.StringComparison]::Ordinal
    }

    if (
        [string]::Equals(
            $WorktreeFullPath,
            $CurrentFullPath,
            $PathComparison
        )
    ) {
        Clear-CurrentWorktree
        return
    }

    & git merge-base --is-ancestor `
        $script:CurrentBranch `
        $script:BaseRef `
        2>$null

    $MergeBaseExitCode = $LASTEXITCODE

    if ($MergeBaseExitCode -eq 1) {
        Clear-CurrentWorktree
        return
    }

    if ($MergeBaseExitCode -ne 0) {
        throw @"
Unable to determine whether branch '$($script:CurrentBranch)' is merged into '$($script:BaseRef)'.
git merge-base exited with code $MergeBaseExitCode.
"@
    }

    $Status = 'clean'

    if (
        -not (
            Test-Path `
                -LiteralPath $script:CurrentPath `
                -PathType Container
        )
    ) {
        $Status = 'missing'
    }
    else {
        $GitStatus = @(
            Invoke-Git `
                -WorkingDirectory $script:CurrentPath `
                -Arguments @(
                    'status'
                    '--short'
                    '--untracked-files=no'
                )
        )

        if ($GitStatus.Count -gt 0) {
            $Status = 'dirty'
        }
    }

    $script:CandidateBranches.Add(
        $script:CurrentBranch
    )

    $script:CandidatePaths.Add(
        $script:CurrentPath
    )

    $script:CandidateStatuses.Add(
        $Status
    )

    Clear-CurrentWorktree
}

$WorktreeOutput = @(
    Invoke-Git -Arguments @(
        'worktree'
        'list'
        '--porcelain'
    )
)

foreach ($LineValue in @($WorktreeOutput) + @('')) {
    $Line = [string]$LineValue

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
        $CurrentBranch = $Line.Substring(
            'branch refs/heads/'.Length
        )
    }
}

Add-CurrentWorktreeCandidate

$CandidateCount = $CandidatePaths.Count

if ($CandidateCount -eq 0) {
    Write-Output "No merged worktrees found for base $BaseRef."
    exit 0
}

Write-Output (
    "Found $CandidateCount merged worktree(s) for base ${BaseRef}:"
)

for ($Index = 0; $Index -lt $CandidateCount; $Index++) {
    Write-Output (
        '- {0}: {1} ({2})' -f
        $CandidateBranches[$Index],
        $CandidatePaths[$Index],
        $CandidateStatuses[$Index]
    )
}

if (-not $Apply) {
    Write-Output ''
    Write-Output (
        'Dry run only. Re-run with --apply to remove clean ' +
        'worktrees, or --apply --force to remove dirty ones too.'
    )
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
        Write-Output (
            "Worktree path already missing for ${Branch}: $Path"
        )

        $MissingCount++
        continue
    }

    if ($Status -eq 'dirty' -and -not $Force) {
        Write-Output (
            "Skipping dirty worktree ${Branch}: $Path"
        )

        $SkippedDirtyCount++
        continue
    }

    Write-Output "Removing ${Branch}: $Path"

    $RemoveArguments = @(
        'worktree'
        'remove'
    )

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

    Invoke-Git -Arguments @(
        'worktree'
        'prune'
    ) | Out-Null
}

if ($RemovedCount -eq 0) {
    Write-Output ''
    Write-Output 'Nothing removed.'
}

if ($SkippedDirtyCount -gt 0) {
    Write-Output ''
    Write-Output (
        "Skipped $SkippedDirtyCount dirty merged worktree(s). " +
        'Re-run with --apply --force if that is intentional.'
    )
}

if ($MissingCount -gt 0) {
    Write-Output ''
    Write-Output (
        "Found $MissingCount merged worktree(s) that were " +
        'already missing on disk. Their metadata is cleaned ' +
        'up by prune during apply runs.'
    )
}