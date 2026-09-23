package release_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalAICandidateFinalizationAcceptsCompleteV2Evidence(t *testing.T) {
	t.Parallel()
	requireLocalAICandidateWindows(t)

	results := runLocalAICandidateFinalizationCases(t, []localAICandidateFinalizationCase{
		{Name: "valid", Kind: "valid"},
		{Name: "packetOnly", Kind: "packetOnlyValid"},
	})
	valid, ok := results["valid"]
	if !ok || valid.Status != "PASS" || valid.Failure != "" || !valid.DigestMatches {
		t.Fatalf("complete v2 evidence finalization = %#v, want a stable PASS report", valid)
	}
	packetOnly, ok := results["packetOnly"]
	if !ok || packetOnly.Status != "PASS" || packetOnly.Failure != "" || !packetOnly.DigestMatches {
		t.Fatalf("packet-only evidence finalization = %#v, want stable PASS without installer execution", packetOnly)
	}
}

func TestLocalAICandidateFinalizationRejectsSourceAndObserverDrift(t *testing.T) {
	t.Parallel()
	requireLocalAICandidateWindows(t)

	cases := []localAICandidateFinalizationCase{
		{Name: "sourceModified", Kind: "sourceModified"},
		{Name: "sourceIdentityMismatch", Kind: "sourceIdentityMismatch"},
		{Name: "packetBuildInfoMismatch", Kind: "packetBuildInfoMismatch"},
		{Name: "packetExecutableDigestMismatch", Kind: "packetExecutableDigestMismatch"},
		{Name: "nonLoopback", Kind: "nonLoopback"},
		{Name: "port7437", Kind: "port7437"},
		{Name: "backendProcess", Kind: "backendProcess"},
		{Name: "missingProcessAttribution", Kind: "missingProcessAttribution"},
		{Name: "missingDistributionEvidence", Kind: "missingDistributionEvidence"},
	}
	results := runLocalAICandidateFinalizationCases(t, cases)
	wantFailures := map[string]string{
		"sourceModified":                 "source.vcsModified must be false",
		"sourceIdentityMismatch":         "install executable build info must match source.commit",
		"packetBuildInfoMismatch":        "packet executable build info must match source.commit",
		"packetExecutableDigestMismatch": "packet executable path and SHA-256 must match the retained Windows executable",
		"nonLoopback":                    "observer.nonLoopbackConnections must be zero",
		"port7437":                       "observer.port7437Accesses must be zero",
		"backendProcess":                 "observer backendProcessStarts and survivingTaskProcesses must be zero",
		"missingProcessAttribution":      "observer must have attributed process/network samples",
		"missingDistributionEvidence":    "observer must attribute four loopback distribution requests",
	}
	for name, expectedError := range wantFailures {
		requireLocalAICandidateFinalizationFailure(t, results, name, expectedError)
	}
}

func TestLocalAICandidateFinalizationRejectsCleanupAndArtifactDrift(t *testing.T) {
	t.Parallel()
	requireLocalAICandidateWindows(t)

	cases := []localAICandidateFinalizationCase{
		{Name: "cleanupFailure", Kind: "cleanupFailure"},
		{Name: "changedCommandEvidence", Kind: "changedCommandEvidence"},
		{Name: "manifestDrift", Kind: "manifestDrift"},
	}
	results := runLocalAICandidateFinalizationCases(t, cases)
	for name, expectedError := range map[string]string{
		"cleanupFailure":         "cleanup must prove stopped listener",
		"changedCommandEvidence": "command-output changed after retention",
		"manifestDrift":          "candidate-report.sha256 does not match the retained manifest bytes",
	} {
		requireLocalAICandidateFinalizationFailure(t, results, name, expectedError)
	}
}

func requireLocalAICandidateWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
}

func runLocalAICandidateFinalizationCases(t *testing.T, cases []localAICandidateFinalizationCase) map[string]localAICandidateCompleteFinalizationResult {
	t.Helper()
	tempDir := t.TempDir()
	casesPath := filepath.Join(tempDir, "finalization-cases.json")
	resultPath := filepath.Join(tempDir, "v2-finalization.json")
	harnessPath := filepath.Join(tempDir, "v2-finalization.ps1")
	caseBytes, err := json.Marshal(struct {
		Cases []localAICandidateFinalizationCase `json:"cases"`
	}{Cases: cases})
	if err != nil {
		t.Fatalf("encode finalization cases: %v", err)
	}
	if err := os.WriteFile(casesPath, caseBytes, 0o600); err != nil {
		t.Fatalf("write finalization cases: %v", err)
	}
	harness := strings.NewReplacer(
		"__SCRIPT_PATH__", localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		"__INSTALL_DIR__", localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		"__ROOT_DIRECTORY__", localAICandidatePowerShellLiteral(tempDir),
		"__CASES_PATH__", localAICandidatePowerShellLiteral(casesPath),
		"__RESULT_PATH__", localAICandidatePowerShellLiteral(resultPath),
	).Replace(localAICandidateFinalizationHarness)
	runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
	results := make(map[string]localAICandidateCompleteFinalizationResult)
	readJSONFile(t, resultPath, &results)
	return results
}

func requireLocalAICandidateFinalizationFailure(t *testing.T, results map[string]localAICandidateCompleteFinalizationResult, name, expectedError string) {
	t.Helper()
	result, ok := results[name]
	if !ok {
		t.Fatalf("%s finalization result is missing from %#v", name, results)
	}
	if result.Status != "FAIL" || !result.DigestMatches ||
		!strings.Contains(result.Failure, expectedError) || !strings.Contains(result.ReportError, expectedError) {
		t.Fatalf("%s finalization = %#v, want FAIL with %q and a valid detached digest", name, result, expectedError)
	}
	if name == "changedCommandEvidence" &&
		(result.CleanupStatus != "FAIL" || result.Stable || !strings.Contains(result.Detail, "command-output")) {
		t.Fatalf("changed command evidence finalization = %#v, want named unstable-evidence cleanup failure", result)
	}
}

type localAICandidateFinalizationCase struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type localAICandidateCompleteFinalizationResult struct {
	Status        string `json:"status"`
	Failure       string `json:"failure"`
	ReportError   string `json:"reportError"`
	CleanupStatus string `json:"cleanupStatus"`
	Stable        bool   `json:"stable"`
	Detail        string `json:"detail"`
	DigestMatches bool   `json:"digestMatches"`
}

const localAICandidateFinalizationHarness = `
. __SCRIPT_PATH__ -InstallDir __INSTALL_DIR__
$rootDirectory = __ROOT_DIRECTORY__
$casePath = __CASES_PATH__
$resultPath = __RESULT_PATH__
$caseDocument = Get-Content -Raw -LiteralPath $casePath | ConvertFrom-Json
$cases = [ordered]@{}
foreach ($definition in $caseDocument.cases) {
    $name = [string]$definition.name
    $cases[$name] = [ordered]@{
        output = Join-Path $rootDirectory $name
        kind = [string]$definition.kind
    }
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
		deliveryMode = 'PUBLIC_INSTALL'
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
			maximumCommandSeconds = 3900
			workBytes = [int64]1024
			executablePath = ''
			executableSHA256 = ''
			executableBuildInfo = [ordered]@{ sourceRevision = ''; vcsModified = $null }
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
        'packetOnlyValid' {
            $report.deliveryMode = 'PACKET_ONLY'
            $report.install.status = 'NOT_RUN'
            $report.build.executablePath = Join-Path $output 'you.exe'
            $executable = @($artifactEvidence | Where-Object { $_.role -ceq 'windows-amd64-executable' })[0]
            $report.build.executableSHA256 = $executable.sha256
            $report.build.executableBuildInfo = [ordered]@{ sourceRevision = $report.source.commit; vcsModified = $false }
        }
		'packetBuildInfoMismatch' {
			$report.deliveryMode = 'PACKET_ONLY'
			$report.install.status = 'NOT_RUN'
			$report.build.executablePath = Join-Path $output 'you.exe'
			$executable = @($artifactEvidence | Where-Object { $_.role -ceq 'windows-amd64-executable' })[0]
			$report.build.executableSHA256 = $executable.sha256
			$report.build.executableBuildInfo = [ordered]@{ sourceRevision = 'cccccccccccccccccccccccccccccccccccccccc'; vcsModified = $false }
		}
		'packetExecutableDigestMismatch' {
			$report.deliveryMode = 'PACKET_ONLY'
			$report.install.status = 'NOT_RUN'
			$report.build.executablePath = Join-Path $output 'you.exe'
			$report.build.executableSHA256 = '0' * 64
			$report.build.executableBuildInfo = [ordered]@{ sourceRevision = $report.source.commit; vcsModified = $false }
		}
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
$results | ConvertTo-Json -Depth 16 | Set-Content -LiteralPath $resultPath -Encoding UTF8
`
