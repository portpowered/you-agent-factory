function Find-CudaRuntimeFile {
    param(
        [Parameter(Mandatory)][string]$CudaRoot,
        [Parameter(Mandatory)][string]$Name
    )

    foreach ($relative in @("bin\$Name", "bin\x64\$Name")) {
        $candidate = Join-Path $CudaRoot $relative
        if (Test-Path -LiteralPath $candidate) { return $candidate }
    }
    return $null
}

function Remove-GeneratedDirectory {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Parent,
        [Parameter(Mandatory)][string]$ExpectedName
    )

    if ($ExpectedName -ne 'package' -and $ExpectedName -cnotmatch '^localai-cuda-probe-[0-9a-f]{32}$') {
        throw "unexpected generated directory name: $ExpectedName"
    }
    $resolvedParent = [IO.Path]::GetFullPath((Resolve-Path -LiteralPath $Parent -ErrorAction Stop).ProviderPath)
    $resolvedTarget = [IO.Path]::GetFullPath((Resolve-Path -LiteralPath $Path -ErrorAction Stop).ProviderPath)
    if ([IO.Path]::GetRelativePath($resolvedParent, $resolvedTarget) -cne $ExpectedName) {
        throw "refusing recursive removal outside the expected generated directory: $resolvedTarget"
    }
    foreach ($entryPath in @($resolvedParent, $resolvedTarget)) {
        $cursor = $entryPath
        while ($cursor) {
            $entry = Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
            if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "refusing recursive removal through a link: $cursor"
            }
            $next = [IO.Path]::GetDirectoryName($cursor.TrimEnd([IO.Path]::DirectorySeparatorChar))
            if (-not $next -or $next -eq $cursor) { break }
            $cursor = $next
        }
    }
    $nestedLink = Get-ChildItem -LiteralPath $resolvedTarget -Recurse -Force -ErrorAction Stop |
        Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 } |
        Select-Object -First 1
    if ($nestedLink) { throw "refusing recursive removal containing a link: $($nestedLink.FullName)" }
    Remove-Item -LiteralPath $resolvedTarget -Recurse -Force
}
