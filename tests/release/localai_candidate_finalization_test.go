package release_test

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type localAICandidateFinalizationResult struct {
	Failure              string `json:"failure"`
	Status               string `json:"status"`
	CleanupStatus        string `json:"cleanupStatus"`
	Stable               bool   `json:"stable"`
	Detail               string `json:"detail"`
	CommandEvidenceCount int    `json:"commandEvidenceCount"`
	OutputExists         bool   `json:"outputExists"`
	ReportExists         bool   `json:"reportExists"`
	DigestExists         bool   `json:"digestExists"`
}

func TestLocalAICandidateFinalizationRetainsFailureReport(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	tempDir := t.TempDir()
	caseDirectories := map[string]string{
		"missingArtifact": filepath.Join(tempDir, "missing-artifact"),
		"changedArtifact": filepath.Join(tempDir, "changed-artifact"),
		"missingCommand":  filepath.Join(tempDir, "missing-command"),
		"changedCommand":  filepath.Join(tempDir, "changed-command"),
	}
	resultPath := filepath.Join(tempDir, "finalization.json")
	harnessPath := filepath.Join(tempDir, "finalization.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
$cases = [ordered]@{
    missingArtifact = [ordered]@{ output = %s; removeArtifact = $true; changeArtifact = $false; removeCommand = $false; changeCommand = $false }
    changedArtifact = [ordered]@{ output = %s; removeArtifact = $false; changeArtifact = $true; removeCommand = $false; changeCommand = $false }
    missingCommand = [ordered]@{ output = %s; removeArtifact = $false; changeArtifact = $false; removeCommand = $true; changeCommand = $false }
    changedCommand = [ordered]@{ output = %s; removeArtifact = $false; changeArtifact = $false; removeCommand = $false; changeCommand = $true }
}
$results = [ordered]@{}
foreach ($entry in $cases.GetEnumerator()) {
    $case = $entry.Value
    $output = $case.output
    [void][System.IO.Directory]::CreateDirectory($output)
    $artifactPath = Join-Path $output 'candidate.bin'
    $commandPath = Join-Path $output 'command.log'
    [System.IO.File]::WriteAllText($artifactPath, 'retained bytes')
    [System.IO.File]::WriteAllText($commandPath, 'command evidence')
    $report = [ordered]@{
        status = 'PASS'
        error = ''
        artifacts = @(Get-SmokeFileEvidence 'linux-amd64-archive' $artifactPath)
        commandEvidence = @(Get-SmokeFileEvidence 'command-output' $commandPath)
        cleanup = [ordered]@{ status = 'PASS' }
    }
    if ($case.removeArtifact) { Remove-Item -LiteralPath $artifactPath -Force }
    if ($case.changeArtifact) { [System.IO.File]::WriteAllText($artifactPath, 'changed artifact') }
    if ($case.removeCommand) { Remove-Item -LiteralPath $commandPath -Force }
    if ($case.changeCommand) { [System.IO.File]::WriteAllText($commandPath, 'changed command') }
    $result = Finalize-SmokeCandidateReport -OutputDirectory $output -ReportPath (Join-Path $output 'candidate-report.json') -Report $report -Failure $null
    $results[$entry.Key] = [ordered]@{
        failure = if ($null -eq $result.failure) { '' } else { $result.failure.Message }
        status = $result.report.status
        cleanupStatus = $result.report.cleanup.status
        stable = $result.report.cleanup.retainedEvidenceHashesStable
        detail = $result.report.cleanup.retainedEvidenceError
        commandEvidenceCount = @($result.report.commandEvidence).Count
        outputExists = Test-Path -LiteralPath $output -PathType Container
        reportExists = Test-Path -LiteralPath (Join-Path $output 'candidate-report.json') -PathType Leaf
        digestExists = Test-Path -LiteralPath (Join-Path $output 'candidate-report.sha256') -PathType Leaf
    }
}
$results | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath %s -Encoding UTF8
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(caseDirectories["missingArtifact"]),
		localAICandidatePowerShellLiteral(caseDirectories["changedArtifact"]),
		localAICandidatePowerShellLiteral(caseDirectories["missingCommand"]),
		localAICandidatePowerShellLiteral(caseDirectories["changedCommand"]),
		localAICandidatePowerShellLiteral(resultPath),
	)
	runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
	var results map[string]localAICandidateFinalizationResult
	readJSONFile(t, resultPath, &results)
	for name, result := range results {
		wantRole := "linux-amd64-archive"
		if strings.Contains(name, "Command") {
			wantRole = "command-output"
		}
		if result.Failure == "" || result.Status != "FAIL" ||
			result.CleanupStatus != "FAIL" || result.Stable ||
			result.CommandEvidenceCount != 1 ||
			!strings.Contains(result.Detail, wantRole) ||
			!result.OutputExists || !result.ReportExists || !result.DigestExists {
			t.Fatalf("%s finalization = %#v, want named retained-evidence failure", name, result)
		}
	}
}
