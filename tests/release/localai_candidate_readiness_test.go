package release_test

import (
	"crypto/sha256"
	"encoding/hex"
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

func TestLocalAICandidateArtifactsRetained(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	tempDir := t.TempDir()
	distDir := filepath.Join(tempDir, "dist")
	outputDir := filepath.Join(tempDir, "output")
	if err := os.MkdirAll(distDir, 0o700); err != nil {
		t.Fatalf("create release dist: %v", err)
	}
	version := "1.0.1-snapshot-fixture"
	artifacts := localAICandidateArchiveFixtures(version)
	for _, artifact := range artifacts {
		if artifact.role == "windows-installer" {
			continue
		}
		if err := os.WriteFile(filepath.Join(distDir, artifact.file), artifact.contents, 0o600); err != nil {
			t.Fatalf("write %s: %v", artifact.file, err)
		}
	}
	if err := os.WriteFile(filepath.Join(distDir, "unrelated-dist-file.txt"), []byte("must not be copied"), 0o600); err != nil {
		t.Fatalf("write unrelated dist file: %v", err)
	}
	installerSource := filepath.Join(tempDir, "install.ps1")
	if err := os.WriteFile(installerSource, []byte("installer fixture\n"), 0o600); err != nil {
		t.Fatalf("write installer source: %v", err)
	}
	resultPath := filepath.Join(tempDir, "promotion.json")
	harnessPath := filepath.Join(tempDir, "promotion.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
$promotion = Promote-SmokeCandidateArtifacts -DistDirectory %s -InstallerSourcePath %s -OutputDirectory %s
[ordered]@{ version = $promotion.version; windowsAmd64Path = $promotion.windowsAmd64Path; artifacts = @($promotion.artifacts) } |
    ConvertTo-Json -Depth 10 | Set-Content -LiteralPath %s -Encoding UTF8
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(distDir),
		localAICandidatePowerShellLiteral(installerSource),
		localAICandidatePowerShellLiteral(outputDir),
		localAICandidatePowerShellLiteral(resultPath),
	)
	runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
	assertLocalAICandidatePromotedArtifacts(t, resultPath, outputDir, version, artifacts)
}

func assertLocalAICandidatePromotedArtifacts(t *testing.T, resultPath, outputDir, version string, artifacts []localAICandidateArchiveFixture) {
	t.Helper()
	var result struct {
		Version          string `json:"version"`
		WindowsAmd64Path string `json:"windowsAmd64Path"`
		Artifacts        []struct {
			Role   string `json:"role"`
			File   string `json:"file"`
			Bytes  int64  `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"artifacts"`
	}
	readJSONFile(t, resultPath, &result)
	if result.Version != version || len(result.Artifacts) != len(artifacts) {
		t.Fatalf("promotion summary = %#v, want version %s and %d artifacts", result, version, len(artifacts))
	}
	expected := make(map[string]localAICandidateArchiveFixture, len(artifacts))
	for _, artifact := range artifacts {
		expected[artifact.role] = artifact
	}
	seen := make(map[string]bool, len(result.Artifacts))
	for _, evidence := range result.Artifacts {
		artifact, ok := expected[evidence.Role]
		if !ok || seen[evidence.Role] {
			t.Fatalf("unexpected or duplicate promoted role %q", evidence.Role)
		}
		seen[evidence.Role] = true
		contents, err := os.ReadFile(filepath.Join(outputDir, evidence.File))
		if err != nil {
			t.Fatalf("read promoted %s: %v", evidence.Role, err)
		}
		digest := sha256.Sum256(contents)
		if string(contents) != string(artifact.contents) || evidence.Bytes != int64(len(contents)) || evidence.SHA256 != hex.EncodeToString(digest[:]) {
			t.Fatalf("promoted evidence for %s = %#v, bytes=%q", evidence.Role, evidence, contents)
		}
	}
	if len(seen) != len(artifacts) {
		t.Fatalf("promoted roles = %#v, want all supported artifacts", seen)
	}
	wantWindows := filepath.Join(outputDir, "you_"+version+"_windows_amd64.zip")
	if !strings.EqualFold(filepath.Clean(result.WindowsAmd64Path), filepath.Clean(wantWindows)) {
		t.Fatalf("selected Windows amd64 archive = %q, want %q", result.WindowsAmd64Path, wantWindows)
	}
	if entries, err := os.ReadDir(outputDir); err != nil || len(entries) != len(artifacts) {
		t.Fatalf("retained output entries = %v/%d, want exactly %d supported artifacts", err, len(entries), len(artifacts))
	}
	if _, err := os.Stat(filepath.Join(outputDir, "unrelated-dist-file.txt")); !os.IsNotExist(err) {
		t.Fatalf("unrelated dist file was copied: %v", err)
	}
}

func TestLocalAICandidateArtifactPromotionFailsClosed(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	for _, testCase := range []struct {
		name    string
		missing string
	}{
		{name: "missing-platform", missing: "linux-arm64-archive"},
		{name: "missing-checksums", missing: "checksums"},
		{name: "missing-installer", missing: "windows-installer"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tempDir := t.TempDir()
			distDir := filepath.Join(tempDir, "dist")
			outputDir := filepath.Join(tempDir, "output")
			if err := os.MkdirAll(distDir, 0o700); err != nil {
				t.Fatalf("create release dist: %v", err)
			}
			version := "1.0.1-snapshot-fixture"
			for _, artifact := range localAICandidateArchiveFixtures(version) {
				if artifact.role == testCase.missing || artifact.role == "windows-installer" {
					continue
				}
				if err := os.WriteFile(filepath.Join(distDir, artifact.file), artifact.contents, 0o600); err != nil {
					t.Fatalf("write %s: %v", artifact.file, err)
				}
			}
			installerSource := filepath.Join(tempDir, "install.ps1")
			if testCase.missing != "windows-installer" {
				if err := os.WriteFile(installerSource, []byte("installer fixture\n"), 0o600); err != nil {
					t.Fatalf("write installer source: %v", err)
				}
			}
			resultPath := filepath.Join(tempDir, "promotion-failure.json")
			harnessPath := filepath.Join(tempDir, "promotion-failure.ps1")
			harness := fmt.Sprintf(`
. %s -InstallDir %s
$caught = ''
try { Promote-SmokeCandidateArtifacts -DistDirectory %s -InstallerSourcePath %s -OutputDirectory %s | Out-Null; exit 2 } catch { $caught = $_.Exception.Message }
$outputExists = Test-Path -LiteralPath %s -PathType Container
$outputEntries = @(Get-ChildItem -LiteralPath %s -Force -ErrorAction SilentlyContinue).Count
[ordered]@{ caught = $caught; outputExists = $outputExists; outputEntries = $outputEntries } |
    ConvertTo-Json -Depth 8 | Set-Content -LiteralPath %s -Encoding UTF8
if ([string]::IsNullOrWhiteSpace($caught) -or -not $caught.Contains(%s) -or -not $outputExists -or $outputEntries -ne 0) { exit 3 }
`,
				localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
				localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
				localAICandidatePowerShellLiteral(distDir),
				localAICandidatePowerShellLiteral(installerSource),
				localAICandidatePowerShellLiteral(outputDir),
				localAICandidatePowerShellLiteral(outputDir),
				localAICandidatePowerShellLiteral(outputDir),
				localAICandidatePowerShellLiteral(resultPath),
				localAICandidatePowerShellLiteral(testCase.missing),
			)
			runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
			var result struct {
				Caught        string `json:"caught"`
				OutputExists  bool   `json:"outputExists"`
				OutputEntries int    `json:"outputEntries"`
			}
			readJSONFile(t, resultPath, &result)
			if result.Caught == "" || !strings.Contains(result.Caught, testCase.missing) || !result.OutputExists || result.OutputEntries != 0 {
				t.Fatalf("promotion failure for %s = %#v", testCase.missing, result)
			}
		})
	}
}

func TestLocalAICandidateFinalizationRetainsFailureEvidence(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	tempDir := t.TempDir()
	for _, name := range []string{"missing", "changed"} {
		if err := os.MkdirAll(filepath.Join(tempDir, name), 0o700); err != nil {
			t.Fatalf("create %s output: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(tempDir, name, "candidate.bin"), []byte("retained bytes\n"), 0o600); err != nil {
			t.Fatalf("write %s artifact: %v", name, err)
		}
	}
	resultPath := filepath.Join(tempDir, "finalization.json")
	harnessPath := filepath.Join(tempDir, "finalization.ps1")
	missingPath := filepath.Join(tempDir, "missing", "candidate.bin")
	changedPath := filepath.Join(tempDir, "changed", "candidate.bin")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
function Invoke-FinalizationCase {
    param([string]$Name, [string]$ArtifactPath, [bool]$RemoveArtifact)
    $output = Split-Path -Parent $ArtifactPath
    $report = [ordered]@{
        status = 'PASS'
        artifacts = @(Get-SmokeFileEvidence 'linux-amd64-archive' $ArtifactPath)
        cleanup = [ordered]@{ status = 'PASS' }
        error = ''
    }
    if ($RemoveArtifact) { Remove-Item -LiteralPath $ArtifactPath -Force } else { [System.IO.File]::WriteAllText($ArtifactPath, 'changed bytes\n') }
    $finalized = Finalize-SmokeCandidateReport -OutputDirectory $output -ReportPath (Join-Path $output 'candidate-report.json') -Report $report -Failure $null
    $failureMessage = if ($null -eq $finalized.failure) { '' } else { $finalized.failure.Message }
    return [ordered]@{
        failure = $failureMessage
        status = $finalized.report.status
        cleanupStatus = $finalized.report.cleanup.status
        stable = $finalized.report.cleanup.retainedEvidenceHashesStable
        detail = $finalized.report.cleanup.retainedEvidenceError
        outputExists = Test-Path -LiteralPath $output -PathType Container
        reportExists = Test-Path -LiteralPath (Join-Path $output 'candidate-report.json') -PathType Leaf
        digestExists = Test-Path -LiteralPath (Join-Path $output 'candidate-report.sha256') -PathType Leaf
    }
}
[ordered]@{
    missing = Invoke-FinalizationCase -Name 'missing' -ArtifactPath %s -RemoveArtifact $true
    changed = Invoke-FinalizationCase -Name 'changed' -ArtifactPath %s -RemoveArtifact $false
} | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath %s -Encoding UTF8
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(missingPath),
		localAICandidatePowerShellLiteral(changedPath),
		localAICandidatePowerShellLiteral(resultPath),
	)
	runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
	var result struct {
		Missing localAICandidateFinalizationResult `json:"missing"`
		Changed localAICandidateFinalizationResult `json:"changed"`
	}
	readJSONFile(t, resultPath, &result)
	for name, value := range map[string]localAICandidateFinalizationResult{"missing": result.Missing, "changed": result.Changed} {
		if value.Failure == "" || value.Status != "FAIL" || value.CleanupStatus != "FAIL" || value.Stable || !strings.Contains(value.Detail, "linux-amd64-archive") || !value.OutputExists || !value.ReportExists || !value.DigestExists {
			t.Fatalf("%s finalization = %#v, want named retained-evidence failure", name, value)
		}
	}
}

func TestLocalAICandidateCleanupPreservesCallerRoots(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	dependencyDir := filepath.Join(tempDir, "dependency")
	outputDir := filepath.Join(tempDir, "output")
	workDir := filepath.Join(tempDir, "work")
	installDir := filepath.Join(tempDir, "install")
	resultPath := filepath.Join(tempDir, "cleanup.json")
	harnessPath := filepath.Join(tempDir, "cleanup.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
foreach ($path in @(%s, %s, %s, %s, %s)) { [void][System.IO.Directory]::CreateDirectory($path) }
[System.IO.File]::WriteAllText((Join-Path %s 'source.txt'), 'caller source')
[System.IO.File]::WriteAllText((Join-Path %s 'dependency.txt'), 'caller dependency')
[System.IO.File]::WriteAllText((Join-Path %s 'output.txt'), 'retained output')
[System.IO.File]::WriteAllText((Join-Path %s 'install.txt'), 'owned install')
$junction = Join-Path %s 'dependency-link'
New-Item -ItemType Junction -Path $junction -Target %s | Out-Null
[System.IO.File]::WriteAllText((Join-Path %s 'work.txt'), 'owned work')
Remove-SmokeOwnedTree 'install directory' %s
Remove-SmokeCandidateWork -WorkDirectory %s -DependencyJunctionPath $junction
[ordered]@{
    source = (Test-Path -LiteralPath (Join-Path %s 'source.txt') -PathType Leaf)
    dependency = (Test-Path -LiteralPath (Join-Path %s 'dependency.txt') -PathType Leaf)
    output = (Test-Path -LiteralPath (Join-Path %s 'output.txt') -PathType Leaf)
    workRemoved = -not (Test-Path -LiteralPath %s)
    installRemoved = -not (Test-Path -LiteralPath %s)
    dependencyLinkRemoved = -not (Test-Path -LiteralPath $junction)
} | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath %s -Encoding UTF8
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(sourceDir),
		localAICandidatePowerShellLiteral(dependencyDir),
		localAICandidatePowerShellLiteral(outputDir),
		localAICandidatePowerShellLiteral(workDir),
		localAICandidatePowerShellLiteral(installDir),
		localAICandidatePowerShellLiteral(sourceDir),
		localAICandidatePowerShellLiteral(dependencyDir),
		localAICandidatePowerShellLiteral(outputDir),
		localAICandidatePowerShellLiteral(installDir),
		localAICandidatePowerShellLiteral(workDir),
		localAICandidatePowerShellLiteral(dependencyDir),
		localAICandidatePowerShellLiteral(workDir),
		localAICandidatePowerShellLiteral(installDir),
		localAICandidatePowerShellLiteral(workDir),
		localAICandidatePowerShellLiteral(sourceDir),
		localAICandidatePowerShellLiteral(dependencyDir),
		localAICandidatePowerShellLiteral(outputDir),
		localAICandidatePowerShellLiteral(workDir),
		localAICandidatePowerShellLiteral(installDir),
		localAICandidatePowerShellLiteral(resultPath),
	)
	runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
	var result struct {
		Source                bool `json:"source"`
		Dependency            bool `json:"dependency"`
		Output                bool `json:"output"`
		WorkRemoved           bool `json:"workRemoved"`
		InstallRemoved        bool `json:"installRemoved"`
		DependencyLinkRemoved bool `json:"dependencyLinkRemoved"`
	}
	readJSONFile(t, resultPath, &result)
	if !result.Source || !result.Dependency || !result.Output || !result.WorkRemoved || !result.InstallRemoved || !result.DependencyLinkRemoved {
		t.Fatalf("candidate cleanup result = %#v", result)
	}
}

type localAICandidateArchiveFixture struct {
	role     string
	file     string
	contents []byte
}

func localAICandidateArchiveFixtures(version string) []localAICandidateArchiveFixture {
	return []localAICandidateArchiveFixture{
		{role: "darwin-amd64-archive", file: "you_" + version + "_darwin_amd64.tar.gz", contents: []byte("darwin amd64 archive")},
		{role: "darwin-arm64-archive", file: "you_" + version + "_darwin_arm64.tar.gz", contents: []byte("darwin arm64 archive")},
		{role: "linux-amd64-archive", file: "you_" + version + "_linux_amd64.tar.gz", contents: []byte("linux amd64 archive")},
		{role: "linux-arm64-archive", file: "you_" + version + "_linux_arm64.tar.gz", contents: []byte("linux arm64 archive")},
		{role: "windows-amd64-archive", file: "you_" + version + "_windows_amd64.zip", contents: []byte("windows amd64 archive")},
		{role: "windows-arm64-archive", file: "you_" + version + "_windows_arm64.zip", contents: []byte("windows arm64 archive")},
		{role: "checksums", file: "you_" + version + "_checksums.txt", contents: []byte("checksums fixture\n")},
		{role: "windows-installer", file: "install.ps1", contents: []byte("installer fixture\n")},
	}
}

type localAICandidateFinalizationResult struct {
	Failure       string `json:"failure"`
	Status        string `json:"status"`
	CleanupStatus string `json:"cleanupStatus"`
	Stable        bool   `json:"stable"`
	Detail        string `json:"detail"`
	OutputExists  bool   `json:"outputExists"`
	ReportExists  bool   `json:"reportExists"`
	DigestExists  bool   `json:"digestExists"`
}
