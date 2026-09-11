package release_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalAICandidateProcessPolicy(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	tempDir := t.TempDir()
	childPath := filepath.Join(tempDir, "record-process-environment.ps1")
	if err := os.WriteFile(childPath, []byte(`[ordered]@{ GOFLAGS = $env:GOFLAGS; GOMAXPROCS = $env:GOMAXPROCS } | ConvertTo-Json -Compress`), 0o600); err != nil {
		t.Fatalf("write process environment child: %v", err)
	}
	resultPath := filepath.Join(tempDir, "process-environment.json")
	harnessPath := filepath.Join(tempDir, "process-policy.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
$requested = Set-CandidateGoProcessEnvironment -GoProcessLimit 2
$child = Invoke-CandidateCommand -FilePath %s -ArgumentList @('-NoProfile', '-NonInteractive', '-File', %s) -WorkingDirectory %s -StdoutPath %s -StderrPath %s
if ($child.exitCode -ne 0) { throw "process policy child failed: $($child | ConvertTo-Json -Compress)" }
$default = Set-CandidateGoProcessEnvironment
$invalid = @{}
foreach ($value in @(0, 5)) {
    $key = [string]$value
    try { Set-CandidateGoProcessEnvironment -GoProcessLimit $value | Out-Null; $invalid[$key] = $false } catch { $invalid[$key] = $true }
}
[ordered]@{ requested = $requested; child = ($child.stdout | ConvertFrom-Json); default = $default; invalid = $invalid } |
    ConvertTo-Json -Depth 8 | Set-Content -LiteralPath %s -Encoding UTF8
if ($requested.GOFLAGS -cne '-p=2' -or $requested.GOMAXPROCS -cne '2' -or
    $child.stdout.Trim() -notmatch '"GOFLAGS":"-p=2"' -or $child.stdout.Trim() -notmatch '"GOMAXPROCS":"2"' -or
    $default.GOFLAGS -cne '-p=4' -or $default.GOMAXPROCS -cne '4' -or -not $invalid['0'] -or -not $invalid['5']) {
    throw ([ordered]@{ requested = $requested; child = $child.stdout.Trim(); default = $default; invalid = $invalid } | ConvertTo-Json -Compress)
}
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(localAICandidatePowerShell(t)),
		localAICandidatePowerShellLiteral(childPath),
		localAICandidatePowerShellLiteral(tempDir),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "child.stdout")),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "child.stderr")),
		localAICandidatePowerShellLiteral(resultPath),
	)
	runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
	var result struct {
		Requested struct {
			GOFLAGS    string `json:"GOFLAGS"`
			GOMAXPROCS string `json:"GOMAXPROCS"`
		} `json:"requested"`
		Child struct {
			GOFLAGS    string `json:"GOFLAGS"`
			GOMAXPROCS string `json:"GOMAXPROCS"`
		} `json:"child"`
		Default struct {
			GOFLAGS    string `json:"GOFLAGS"`
			GOMAXPROCS string `json:"GOMAXPROCS"`
		} `json:"default"`
		Invalid map[string]bool `json:"invalid"`
	}
	readJSONFile(t, resultPath, &result)
	if result.Requested.GOFLAGS != "-p=2" || result.Requested.GOMAXPROCS != "2" || result.Child.GOFLAGS != "-p=2" || result.Child.GOMAXPROCS != "2" || result.Default.GOFLAGS != "-p=4" || result.Default.GOMAXPROCS != "4" || !result.Invalid["0"] || !result.Invalid["5"] {
		t.Fatalf("candidate process policy = %#v", result)
	}
	for _, value := range []string{"0", "5"} {
		invalidCommand := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-File", localAICandidateScriptPath(t), "-InstallDir", filepath.Join(tempDir, "invalid-"+value), "-CandidateGoProcessLimit", value)
		invalidCommand.Env = localAICandidatePowerShellEnvironment(t, tempDir, nil)
		if output, err := invalidCommand.CombinedOutput(); err == nil {
			t.Fatalf("invalid candidate process limit %s unexpectedly succeeded: %s", value, output)
		}
	}
}

func TestLocalAICandidateDriverProvenancePreservesTarget(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	tempDir := t.TempDir()
	targetDir := filepath.Join(tempDir, "target")
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		t.Fatalf("create target repository: %v", err)
	}
	localAICandidateRunGit(t, targetDir, "init", "--quiet")
	localAICandidateRunGit(t, targetDir, "config", "user.email", "candidate@example.invalid")
	localAICandidateRunGit(t, targetDir, "config", "user.name", "candidate")
	if err := os.WriteFile(filepath.Join(targetDir, "target.txt"), []byte("immutable target"), 0o600); err != nil {
		t.Fatalf("write target repository: %v", err)
	}
	localAICandidateRunGit(t, targetDir, "add", "target.txt")
	localAICandidateRunGit(t, targetDir, "commit", "--quiet", "-m", "target fixture")
	localAICandidateRunGit(t, targetDir, "remote", "add", "origin", "https://example.invalid/target.git")
	targetCommit := localAICandidateGit(t, targetDir, "rev-parse", "HEAD")
	targetTree := localAICandidateGit(t, targetDir, "rev-parse", "HEAD^{tree}")
	targetStatus := localAICandidateGit(t, targetDir, "status", "--porcelain=v1", "--untracked-files=all")
	resultPath := filepath.Join(tempDir, "provenance.json")
	harnessPath := filepath.Join(tempDir, "provenance.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
$driverRevision = Get-CandidateDriverRevision
$sourceIdentity = Get-CandidateSourceIdentity -SourcePath %s -Commit %s -Repository 'https://example.invalid/target.git'
[ordered]@{ driverRevision = $driverRevision; source = [ordered]@{ repository = $sourceIdentity.repository; commit = $sourceIdentity.commit; tree = $sourceIdentity.tree } } |
    ConvertTo-Json -Depth 8 | Set-Content -LiteralPath %s -Encoding UTF8
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(targetDir),
		localAICandidatePowerShellLiteral(targetCommit),
		localAICandidatePowerShellLiteral(resultPath),
	)
	runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
	var provenance struct {
		DriverRevision string `json:"driverRevision"`
		Source         struct {
			Repository string `json:"repository"`
			Commit     string `json:"commit"`
			Tree       string `json:"tree"`
		} `json:"source"`
	}
	readJSONFile(t, resultPath, &provenance)
	if len(provenance.DriverRevision) != 40 || provenance.DriverRevision == targetCommit {
		t.Fatalf("driver revision = %q, want a distinct full revision from target %s", provenance.DriverRevision, targetCommit)
	}
	if provenance.Source.Repository != "https://example.invalid/target.git" || provenance.Source.Commit != targetCommit || provenance.Source.Tree != targetTree {
		t.Fatalf("target provenance = %#v, want repository %s commit %s tree %s", provenance.Source, "https://example.invalid/target.git", targetCommit, targetTree)
	}
	if got := localAICandidateGit(t, targetDir, "rev-parse", "HEAD"); got != targetCommit {
		t.Fatalf("target commit changed from %s to %s", targetCommit, got)
	}
	if got := localAICandidateGit(t, targetDir, "rev-parse", "HEAD^{tree}"); got != targetTree {
		t.Fatalf("target tree changed from %s to %s", targetTree, got)
	}
	if got := localAICandidateGit(t, targetDir, "status", "--porcelain=v1", "--untracked-files=all"); got != targetStatus {
		t.Fatalf("target status changed from %q to %q", targetStatus, got)
	}
}

func TestLocalAICandidateSourceMismatchRetainsFailureReport(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	tempDir, err := os.MkdirTemp(os.TempDir(), "lmx-")
	if err != nil {
		t.Fatalf("create short candidate fixture root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	targetDir := filepath.Join(tempDir, "target")
	dependencyDir := filepath.Join(tempDir, "dependencies")
	for _, directory := range []string{targetDir, dependencyDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create source fixture directory: %v", err)
		}
	}
	localAICandidateRunGit(t, targetDir, "init", "--quiet")
	localAICandidateRunGit(t, targetDir, "config", "user.email", "candidate@example.invalid")
	localAICandidateRunGit(t, targetDir, "config", "user.name", "candidate")
	if err := os.WriteFile(filepath.Join(targetDir, "target.txt"), []byte("immutable target"), 0o600); err != nil {
		t.Fatalf("write target repository: %v", err)
	}
	localAICandidateRunGit(t, targetDir, "add", "target.txt")
	localAICandidateRunGit(t, targetDir, "commit", "--quiet", "-m", "target fixture")
	localAICandidateRunGit(t, targetDir, "remote", "add", "origin", "https://example.invalid/target.git")
	targetCommit := localAICandidateGit(t, targetDir, "rev-parse", "HEAD")
	releaseToolPath := filepath.Join(tempDir, "release-tool.exe")
	if err := os.WriteFile(releaseToolPath, []byte("controlled release child"), 0o600); err != nil {
		t.Fatalf("write controlled release tool: %v", err)
	}
	outputDir := filepath.Join(tempDir, "output")
	workDir := filepath.Join(tempDir, "work")
	installDir := filepath.Join(tempDir, "install")
	reportPath := filepath.Join(outputDir, "candidate-report.json")
	resultPath := filepath.Join(tempDir, "failure.json")
	harnessPath := filepath.Join(tempDir, "failure.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
$caught = ''
try { Invoke-LocalCandidateSmoke -SourcePath %s -SourceCommit %s -SourceRepository 'https://example.invalid/different.git' -DependencySourcePath %s -OutputDirectory %s -WorkDirectory %s -GoProcessLimit 2 -ReleaseToolPath %s -ReleaseToolVersion 'fixture' -EsbuildVersion 'fixture' -EsbuildSHA256 %s -MaximumWorkBytes 1048576 -RequestedInstallDir %s -RequestedReportPath %s; exit 2 } catch { $caught = $_.Exception.Message }
$report = Get-Content -LiteralPath %s -Raw | ConvertFrom-Json
$outputExists = Test-Path -LiteralPath %s -PathType Container
$workExists = Test-Path -LiteralPath %s
$digestExists = Test-Path -LiteralPath (Join-Path %s 'candidate-report.sha256') -PathType Leaf
[ordered]@{ caught = $caught; report = $report; outputExists = $outputExists; workExists = $workExists; digestExists = $digestExists } |
    ConvertTo-Json -Depth 12 | Set-Content -LiteralPath %s -Encoding UTF8
if ([string]::IsNullOrWhiteSpace($caught) -or $report.status -ne 'FAIL' -or $report.cleanup.status -ne 'PASS' -or $report.driverRevision.Length -ne 40 -or -not $report.error.Contains('candidate source origin') -or -not $outputExists -or $workExists -or -not $digestExists) { exit 3 }
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(installDir),
		localAICandidatePowerShellLiteral(targetDir),
		localAICandidatePowerShellLiteral(targetCommit),
		localAICandidatePowerShellLiteral(dependencyDir),
		localAICandidatePowerShellLiteral(outputDir),
		localAICandidatePowerShellLiteral(workDir),
		localAICandidatePowerShellLiteral(releaseToolPath),
		localAICandidatePowerShellLiteral(strings.Repeat("0", 64)),
		localAICandidatePowerShellLiteral(installDir),
		localAICandidatePowerShellLiteral(reportPath),
		localAICandidatePowerShellLiteral(reportPath),
		localAICandidatePowerShellLiteral(outputDir),
		localAICandidatePowerShellLiteral(workDir),
		localAICandidatePowerShellLiteral(outputDir),
		localAICandidatePowerShellLiteral(resultPath),
	)
	runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
	var result struct {
		Caught string `json:"caught"`
		Report struct {
			Status         string `json:"status"`
			DriverRevision string `json:"driverRevision"`
			Error          string `json:"error"`
			Cleanup        struct {
				Status string `json:"status"`
			} `json:"cleanup"`
		} `json:"report"`
		OutputExists bool `json:"outputExists"`
		WorkExists   bool `json:"workExists"`
		DigestExists bool `json:"digestExists"`
	}
	readJSONFile(t, resultPath, &result)
	if result.Caught == "" || result.Report.Status != "FAIL" || result.Report.Cleanup.Status != "PASS" || len(result.Report.DriverRevision) != 40 || !strings.Contains(result.Report.Error, "candidate source origin") || !result.OutputExists || result.WorkExists || !result.DigestExists {
		t.Fatalf("candidate source mismatch evidence = %#v", result)
	}
}
