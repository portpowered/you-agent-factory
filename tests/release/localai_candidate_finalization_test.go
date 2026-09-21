package release_test

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalAICandidateFinalizationRequiresCompleteV2Evidence(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	tempDir := t.TempDir()
	caseDirectories := map[string]string{
		"valid":                       filepath.Join(tempDir, "valid"),
		"sourceModified":              filepath.Join(tempDir, "source-modified"),
		"sourceIdentityMismatch":      filepath.Join(tempDir, "source-identity-mismatch"),
		"nonLoopback":                 filepath.Join(tempDir, "non-loopback"),
		"port7437":                    filepath.Join(tempDir, "port-7437"),
		"backendProcess":              filepath.Join(tempDir, "backend-process"),
		"missingProcessAttribution":   filepath.Join(tempDir, "missing-process-attribution"),
		"missingDistributionEvidence": filepath.Join(tempDir, "missing-distribution-evidence"),
		"cleanupFailure":              filepath.Join(tempDir, "cleanup-failure"),
		"changedCommandEvidence":      filepath.Join(tempDir, "changed-command-evidence"),
		"manifestDrift":               filepath.Join(tempDir, "manifest-drift"),
	}
	resultPath := filepath.Join(tempDir, "v2-finalization.json")
	harnessPath := filepath.Join(tempDir, "v2-finalization.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
$cases = [ordered]@{
    valid = [ordered]@{ output = %s; kind = 'valid' }
    sourceModified = [ordered]@{ output = %s; kind = 'sourceModified' }
    sourceIdentityMismatch = [ordered]@{ output = %s; kind = 'sourceIdentityMismatch' }
    nonLoopback = [ordered]@{ output = %s; kind = 'nonLoopback' }
    port7437 = [ordered]@{ output = %s; kind = 'port7437' }
    backendProcess = [ordered]@{ output = %s; kind = 'backendProcess' }
    missingProcessAttribution = [ordered]@{ output = %s; kind = 'missingProcessAttribution' }
    missingDistributionEvidence = [ordered]@{ output = %s; kind = 'missingDistributionEvidence' }
    cleanupFailure = [ordered]@{ output = %s; kind = 'cleanupFailure' }
    changedCommandEvidence = [ordered]@{ output = %s; kind = 'changedCommandEvidence' }
    manifestDrift = [ordered]@{ output = %s; kind = 'manifestDrift' }
}
$artifactSpecs = @(
    [ordered]@{ role = 'windows-amd64-archive'; file = 'you_fixture_windows_amd64.zip'; contents = 'archive bytes' },
    [ordered]@{ role = 'windows-amd64-executable'; file = 'you.exe'; contents = 'executable bytes' },
    [ordered]@{ role = 'windows-installer'; file = 'install.ps1'; contents = 'installer bytes' },
    [ordered]@{ role = 'detached-checksums'; file = 'SHA256SUMS.txt'; contents = 'checksum bytes' },
    [ordered]@{ role = 'public-doc-snapshot'; file = 'models.md'; contents = 'public docs bytes' }
)
$results = [ordered]@{}
foreach ($entry in $cases.GetEnumerator()) {
    $case = $entry.Value
    $output = $case.output
    [void][System.IO.Directory]::CreateDirectory($output)
    $artifactEvidence = New-Object 'System.Collections.Generic.List[object]'
    foreach ($spec in $artifactSpecs) {
        $artifactPath = Join-Path $output $spec.file
        [System.IO.File]::WriteAllText($artifactPath, $spec.contents, [System.Text.UTF8Encoding]::new($false))
        [void]$artifactEvidence.Add((Get-SmokeFileEvidence $spec.role $artifactPath))
    }
    $report = [ordered]@{
        schemaVersion = 'local-windows-candidate/v2'
        driverRevision = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
        status = 'PASS'
        source = [ordered]@{
            commit = 'b8300f24bf8b2a333a3c2743c1814d264478aedb'
            tree = '1bb0badc9faa244005d81e9255cb44050b4b720f'
            vcsModified = $false
        }
        target = [ordered]@{ os = 'windows'; arch = 'amd64' }
        build = [ordered]@{
            goVersion = 'go version go1.26.8 windows/amd64'
            goReleaserVersion = 'v2.12.7'
            maximumWorkBytes = [int64]4294967296
            maximumChildren = 4
            workBytes = [int64]1024
        }
        artifacts = @($artifactEvidence | ForEach-Object { $_ })
        commandEvidence = @()
        install = [ordered]@{
            status = 'PASS'
            preinstallAbsent = $true
            stateRootsInitiallyEmpty = $true
            pathResolution = (Join-Path $output 'install\you.exe')
            executableBuildInfo = [ordered]@{ sourceRevision = 'b8300f24bf8b2a333a3c2743c1814d264478aedb'; vcsModified = $false }
            modelCalls = 0
            modelBackendDownloadBytes = [int64]0
        }
        observer = [ordered]@{
            observed = $true
            sampleCount = 2
            attributedProcessCount = 1
            nonLoopbackConnections = 0
            port7437Accesses = 0
            backendProcessStarts = 0
            survivingTaskProcesses = 0
            loopbackDistributionRequests = 4
            distributionListenerPort = 45678
            distributionListenerStopped = $true
            distributionObserverObserved = $true
            distributionObserverError = ''
            error = ''
        }
        cleanup = [ordered]@{
            status = 'PASS'
            listenerStopped = $true
            workDirectoryRemoved = $true
            installDirectoryRemoved = $true
            partialFilesRemaining = 0
            retainedEvidenceHashesStable = $true
        }
        error = ''
    }
    switch ($case.kind) {
        'sourceModified' { $report.source.vcsModified = $true }
        'sourceIdentityMismatch' { $report.source.commit = 'cccccccccccccccccccccccccccccccccccccccc' }
        'nonLoopback' { $report.observer.nonLoopbackConnections = 1 }
        'port7437' { $report.observer.port7437Accesses = 1 }
        'backendProcess' { $report.observer.backendProcessStarts = 1 }
        'missingProcessAttribution' { $report.observer.attributedProcessCount = 0 }
        'missingDistributionEvidence' { $report.observer.distributionObserverObserved = $false }
        'cleanupFailure' { $report.cleanup.listenerStopped = $false }
        'changedCommandEvidence' {
            $commandPath = Join-Path $output 'release.stderr.log'
            [System.IO.File]::WriteAllText($commandPath, 'retained command evidence')
            $report.commandEvidence = @(Get-SmokeFileEvidence 'command-output' $commandPath)
            [System.IO.File]::WriteAllText($commandPath, 'changed command evidence')
        }
        'manifestDrift' {
            [System.IO.File]::WriteAllText((Join-Path $output 'candidate-report.json'), 'stale manifest')
            [System.IO.File]::WriteAllText((Join-Path $output 'candidate-report.sha256'), ('0' * 64) + '  candidate-report.json' + [char]10)
        }
    }
    $finalized = Finalize-SmokeCandidateReport -OutputDirectory $output -ReportPath (Join-Path $output 'candidate-report.json') -Report $report -Failure $null
    $reportEvidence = Get-SmokeFileEvidence 'candidate-report' (Join-Path $output 'candidate-report.json')
    $expectedDigest = $reportEvidence.sha256 + '  candidate-report.json' + [char]10
    $actualDigest = [System.IO.File]::ReadAllText((Join-Path $output 'candidate-report.sha256'))
    $retainedEvidenceError = ''
    if ($finalized.report.cleanup.Contains('retainedEvidenceError')) {
        $retainedEvidenceError = [string]$finalized.report.cleanup.retainedEvidenceError
    }
    $results[$entry.Key] = [ordered]@{
        status = $finalized.report.status
        failure = if ($null -eq $finalized.failure) { '' } else { $finalized.failure.Message }
        reportError = [string]$finalized.report.error
        cleanupStatus = [string]$finalized.report.cleanup.status
        stable = [bool]$finalized.report.cleanup.retainedEvidenceHashesStable
        detail = $retainedEvidenceError
        digestMatches = $actualDigest -ceq $expectedDigest
    }
}
$results | ConvertTo-Json -Depth 16 | Set-Content -LiteralPath %s -Encoding UTF8
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(caseDirectories["valid"]),
		localAICandidatePowerShellLiteral(caseDirectories["sourceModified"]),
		localAICandidatePowerShellLiteral(caseDirectories["sourceIdentityMismatch"]),
		localAICandidatePowerShellLiteral(caseDirectories["nonLoopback"]),
		localAICandidatePowerShellLiteral(caseDirectories["port7437"]),
		localAICandidatePowerShellLiteral(caseDirectories["backendProcess"]),
		localAICandidatePowerShellLiteral(caseDirectories["missingProcessAttribution"]),
		localAICandidatePowerShellLiteral(caseDirectories["missingDistributionEvidence"]),
		localAICandidatePowerShellLiteral(caseDirectories["cleanupFailure"]),
		localAICandidatePowerShellLiteral(caseDirectories["changedCommandEvidence"]),
		localAICandidatePowerShellLiteral(caseDirectories["manifestDrift"]),
		localAICandidatePowerShellLiteral(resultPath),
	)
	runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
	var results map[string]struct {
		Status        string `json:"status"`
		Failure       string `json:"failure"`
		ReportError   string `json:"reportError"`
		CleanupStatus string `json:"cleanupStatus"`
		Stable        bool   `json:"stable"`
		Detail        string `json:"detail"`
		DigestMatches bool   `json:"digestMatches"`
	}
	readJSONFile(t, resultPath, &results)
	if valid := results["valid"]; valid.Status != "PASS" || valid.Failure != "" || !valid.DigestMatches {
		t.Fatalf("complete v2 evidence finalization = %#v, want a stable PASS report", valid)
	}
	wantFailures := map[string]string{
		"sourceModified":              "source.vcsModified must be false",
		"sourceIdentityMismatch":      "install executable build info must match source.commit",
		"nonLoopback":                 "observer.nonLoopbackConnections must be zero",
		"port7437":                    "observer.port7437Accesses must be zero",
		"backendProcess":              "observer backendProcessStarts and survivingTaskProcesses must be zero",
		"missingProcessAttribution":   "observer must have attributed process/network samples",
		"missingDistributionEvidence": "observer must attribute four loopback distribution requests",
		"cleanupFailure":              "cleanup must prove stopped listener",
		"changedCommandEvidence":      "command-output changed after retention",
		"manifestDrift":               "candidate-report.sha256 does not match the retained manifest bytes",
	}
	for name, expectedError := range wantFailures {
		result, ok := results[name]
		if !ok || result.Status != "FAIL" || !result.DigestMatches ||
			!strings.Contains(result.Failure, expectedError) || !strings.Contains(result.ReportError, expectedError) {
			t.Fatalf("%s finalization = %#v, want FAIL with %q and a valid detached digest", name, result, expectedError)
		}
		if name == "changedCommandEvidence" &&
			(result.CleanupStatus != "FAIL" || result.Stable || !strings.Contains(result.Detail, "command-output")) {
			t.Fatalf("changed command evidence finalization = %#v, want named unstable-evidence cleanup failure", result)
		}
	}
}
