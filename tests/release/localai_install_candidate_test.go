package release_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const localAICandidateSourceCommit = "059474b2c00915865306a33ca5e3d02b618bb6f0"

func TestLocalAICandidateScriptParses(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}

	scriptPath := localAICandidateScriptPath(t)
	command := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-Command", fmt.Sprintf(`
$errors = $null
$tokens = $null
[System.Management.Automation.Language.Parser]::ParseFile(%s, [ref]$tokens, [ref]$errors) | Out-Null
if ($errors.Count -ne 0) { $errors | ForEach-Object { [Console]::Error.WriteLine($_.Message) }; exit 1 }
`, localAICandidatePowerShellLiteral(scriptPath)))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("parse candidate delivery script: %v\n%s", err, output)
	}
}

func TestLocalAICandidateCommandPreservesArgumentsFailureAndRedaction(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}

	tempDir := filepath.Join(t.TempDir(), "path with spaces")
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	recordPath := filepath.Join(tempDir, "arguments.json")
	recorderPath := filepath.Join(tempDir, "record arguments.ps1")
	recorder := `param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Values)
[System.IO.File]::WriteAllText($env:LOCALAI_ARGUMENT_RECORD, ($Values | ConvertTo-Json -Compress))
[Console]::Out.WriteLine("token=fixture-secret")
[Console]::Out.Write("stdout retained")
[Console]::Error.WriteLine("api_key=fixture-secret")
[Console]::Error.Write("stderr retained")
exit 23
`
	if err := os.WriteFile(recorderPath, []byte(recorder), 0o600); err != nil {
		t.Fatalf("write argument recorder: %v", err)
	}

	stdoutPath := filepath.Join(tempDir, "stdout.log")
	stderrPath := filepath.Join(tempDir, "stderr.log")
	resultPath := filepath.Join(tempDir, "result.json")
	installDir := filepath.Join(tempDir, "unused install")
	wantArguments := []string{"release", "--snapshot", "--clean", "-f", ".goreleaser.yml", filepath.Join(tempDir, "value with spaces")}
	argumentLiterals := make([]string, 0, len(wantArguments)+5)
	for _, value := range append([]string{"-NoProfile", "-NonInteractive", "-File", recorderPath}, wantArguments...) {
		argumentLiterals = append(argumentLiterals, localAICandidatePowerShellLiteral(value))
	}
	harnessPath := filepath.Join(tempDir, "invoke.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
$result = Invoke-CandidateCommand -FilePath %s -ArgumentList @(%s) -WorkingDirectory %s -StdoutPath %s -StderrPath %s
$result | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath %s -Encoding UTF8
if ($result.exitCode -ne 23) { exit 2 }
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(installDir),
		localAICandidatePowerShellLiteral(localAICandidatePowerShell(t)),
		strings.Join(argumentLiterals, ", "),
		localAICandidatePowerShellLiteral(tempDir),
		localAICandidatePowerShellLiteral(stdoutPath),
		localAICandidatePowerShellLiteral(stderrPath),
		localAICandidatePowerShellLiteral(resultPath),
	)
	if err := os.WriteFile(harnessPath, []byte(harness), 0o600); err != nil {
		t.Fatalf("write command harness: %v", err)
	}
	command := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-File", harnessPath)
	command.Env = append(os.Environ(), "LOCALAI_ARGUMENT_RECORD="+recordPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run candidate command helper: %v\n%s", err, output)
	}

	var gotArguments []string
	readJSONFile(t, recordPath, &gotArguments)
	if strings.Join(gotArguments, "\x00") != strings.Join(wantArguments, "\x00") {
		t.Fatalf("recorded arguments = %#v, want %#v", gotArguments, wantArguments)
	}
	var result struct {
		ExitCode       int      `json:"exitCode"`
		ElapsedMillis  int64    `json:"elapsedMilliseconds"`
		Arguments      []string `json:"arguments"`
		Stdout         string   `json:"stdout"`
		Stderr         string   `json:"stderr"`
		StdoutEvidence struct {
			SHA256 string `json:"sha256"`
		} `json:"stdoutEvidence"`
		StderrEvidence struct {
			SHA256 string `json:"sha256"`
		} `json:"stderrEvidence"`
	}
	readJSONFile(t, resultPath, &result)
	if result.ExitCode != 23 || result.ElapsedMillis < 0 || len(result.Arguments) != len(wantArguments)+4 {
		t.Fatalf("command result = %#v", result)
	}
	if !strings.Contains(result.Stdout, "token=<redacted>") || !strings.Contains(result.Stderr, "api_key=<redacted>") {
		t.Fatalf("redacted output = stdout %q stderr %q", result.Stdout, result.Stderr)
	}
	for _, path := range []string{stdoutPath, stderrPath} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read retained output %s: %v", path, err)
		}
		if strings.Contains(string(contents), "fixture-secret") || !strings.Contains(string(contents), "<redacted>") {
			t.Fatalf("retained output %s was not redacted: %q", path, contents)
		}
	}
	if len(result.StdoutEvidence.SHA256) != 64 || len(result.StderrEvidence.SHA256) != 64 {
		t.Fatalf("retained output hashes are missing: %#v", result)
	}
}

func TestLocalAICandidateCommandDeadlineKillsChildTree(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}

	tempDir := t.TempDir()
	childPIDPath := filepath.Join(tempDir, "child.pid")
	childPath := filepath.Join(tempDir, "child.ps1")
	parentPath := filepath.Join(tempDir, "parent.ps1")
	if err := os.WriteFile(childPath, []byte(`while ($true) { Start-Sleep -Milliseconds 100 }`), 0o600); err != nil {
		t.Fatalf("write hanging child: %v", err)
	}
	parent := fmt.Sprintf(`
$child = Start-Process -FilePath %s -ArgumentList @('-NoProfile', '-NonInteractive', '-File', %s) -PassThru
[System.IO.File]::WriteAllText(%s, [string]$child.Id)
while ($true) { Start-Sleep -Milliseconds 100 }
`,
		localAICandidatePowerShellLiteral(localAICandidatePowerShell(t)),
		localAICandidatePowerShellLiteral(childPath),
		localAICandidatePowerShellLiteral(childPIDPath),
	)
	if err := os.WriteFile(parentPath, []byte(parent), 0o600); err != nil {
		t.Fatalf("write hanging parent: %v", err)
	}

	resultPath := filepath.Join(tempDir, "result.json")
	harnessPath := filepath.Join(tempDir, "deadline.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
$result = Invoke-CandidateCommand -FilePath %s -ArgumentList @('-NoProfile', '-NonInteractive', '-File', %s) -WorkingDirectory %s -StdoutPath %s -StderrPath %s -TimeoutSeconds 3 -OutputMaximumBytes 4096
$result | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath %s -Encoding UTF8
if (-not $result.timedOut -or $result.exitCode -ne 124) { exit 2 }
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(localAICandidatePowerShell(t)),
		localAICandidatePowerShellLiteral(parentPath),
		localAICandidatePowerShellLiteral(tempDir),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "stdout.log")),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "stderr.log")),
		localAICandidatePowerShellLiteral(resultPath),
	)
	if err := os.WriteFile(harnessPath, []byte(harness), 0o600); err != nil {
		t.Fatalf("write deadline harness: %v", err)
	}
	started := time.Now()
	if output, err := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-File", harnessPath).CombinedOutput(); err != nil {
		t.Fatalf("run command deadline harness: %v\n%s", err, output)
	}
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Fatalf("deadline helper took %s, want at most 15s", elapsed)
	}
	pidBytes, err := os.ReadFile(childPIDPath)
	if err != nil {
		t.Fatalf("read hanging child PID: %v", err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		t.Fatalf("parse hanging child PID: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		check := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-Command",
			fmt.Sprintf("if (Get-Process -Id %d -ErrorAction SilentlyContinue) { exit 1 }", childPID))
		if err := check.Run(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("hanging child process %d survived command deadline", childPID)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestLocalAICandidateCommandBoundsRetainedOutput(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}

	tempDir := t.TempDir()
	writerPath := filepath.Join(tempDir, "writer.ps1")
	if err := os.WriteFile(writerPath, []byte(`[Console]::Out.Write(('token=x ' * 1024)); exit 0`), 0o600); err != nil {
		t.Fatalf("write output fixture: %v", err)
	}
	stdoutPath := filepath.Join(tempDir, "stdout.log")
	resultPath := filepath.Join(tempDir, "result.json")
	harnessPath := filepath.Join(tempDir, "output-limit.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
$result = Invoke-CandidateCommand -FilePath %s -ArgumentList @('-NoProfile', '-NonInteractive', '-File', %s) -WorkingDirectory %s -StdoutPath %s -StderrPath %s -TimeoutSeconds 10 -OutputMaximumBytes 1024
$result | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath %s -Encoding UTF8
if (-not $result.outputLimitExceeded -or $result.exitCode -ne 125) { exit 2 }
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(localAICandidatePowerShell(t)),
		localAICandidatePowerShellLiteral(writerPath),
		localAICandidatePowerShellLiteral(tempDir),
		localAICandidatePowerShellLiteral(stdoutPath),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "stderr.log")),
		localAICandidatePowerShellLiteral(resultPath),
	)
	if err := os.WriteFile(harnessPath, []byte(harness), 0o600); err != nil {
		t.Fatalf("write output-limit harness: %v", err)
	}
	if output, err := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-File", harnessPath).CombinedOutput(); err != nil {
		t.Fatalf("run output-limit harness: %v\n%s", err, output)
	}
	info, err := os.Stat(stdoutPath)
	if err != nil {
		t.Fatalf("stat retained output: %v", err)
	}
	if info.Size() > 1024 {
		t.Fatalf("retained output is %d bytes, want at most 1024", info.Size())
	}
	var result struct {
		OutputLimitExceeded bool `json:"outputLimitExceeded"`
		StdoutEvidence      struct {
			TotalBytes int64 `json:"totalBytes"`
			Truncated  bool  `json:"truncated"`
		} `json:"stdoutEvidence"`
	}
	readJSONFile(t, resultPath, &result)
	if !result.OutputLimitExceeded || !result.StdoutEvidence.Truncated || result.StdoutEvidence.TotalBytes <= 1024 {
		t.Fatalf("bounded output evidence = %#v", result)
	}
}

func TestLocalAICandidateRootsRejectOverlapAndReparseAncestry(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}

	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	dependencyDir := filepath.Join(tempDir, "dependencies")
	for _, directory := range []string{sourceDir, dependencyDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create path fixture: %v", err)
		}
	}
	outputDir := filepath.Join(sourceDir, "output")
	harnessPath := filepath.Join(tempDir, "paths.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
try {
    Assert-SmokeCandidateRoots -SourcePath %s -DependencySourcePath %s -OutputDirectory %s -WorkDirectory %s -InstallDirectory %s -ReportPath %s | Out-Null
    exit 2
} catch {
    if (-not $_.Exception.Message.Contains('must not overlap')) { throw }
}
$link = %s
New-Item -ItemType Junction -Path $link -Target %s | Out-Null
try {
    Assert-SmokeCandidateRoots -SourcePath $link -DependencySourcePath %s -OutputDirectory %s -WorkDirectory %s -InstallDirectory %s -ReportPath %s | Out-Null
    exit 3
} catch {
    if (-not $_.Exception.Message.Contains('reparse point in its ancestry')) { throw }
}
$longRoot = Join-Path %s ('x' * 230)
$rootCases = @(
    @{ Name = 'output'; OutputDirectory = $longRoot },
    @{ Name = 'work'; WorkDirectory = $longRoot },
    @{ Name = 'install'; InstallDirectory = $longRoot },
    @{ Name = 'report'; ReportPath = (Join-Path $longRoot 'report.json') }
)
foreach ($case in $rootCases) {
    $arguments = @{
        SourcePath = %s
        DependencySourcePath = %s
        OutputDirectory = %s
        WorkDirectory = %s
        InstallDirectory = %s
        ReportPath = %s
    }
    foreach ($key in $case.Keys) {
        if ($key -ne 'Name') { $arguments[$key] = $case[$key] }
    }
    try {
        Assert-SmokeCandidateRoots @arguments | Out-Null
        exit 4
    } catch {
        if (-not $_.Exception.Message.Contains('at most 220 characters')) { throw }
    }
}
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(sourceDir),
		localAICandidatePowerShellLiteral(dependencyDir),
		localAICandidatePowerShellLiteral(outputDir),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "work")),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "install")),
		localAICandidatePowerShellLiteral(filepath.Join(outputDir, "report.json")),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "source-link")),
		localAICandidatePowerShellLiteral(sourceDir),
		localAICandidatePowerShellLiteral(dependencyDir),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "output")),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "work")),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "install")),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "output", "report.json")),
		localAICandidatePowerShellLiteral(tempDir),
		localAICandidatePowerShellLiteral(sourceDir),
		localAICandidatePowerShellLiteral(dependencyDir),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "output")),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "work")),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "install")),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "output", "report.json")),
	)
	if err := os.WriteFile(harnessPath, []byte(harness), 0o600); err != nil {
		t.Fatalf("write path-safety harness: %v", err)
	}
	if output, err := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-File", harnessPath).CombinedOutput(); err != nil {
		t.Fatalf("validate candidate roots: %v\n%s", err, output)
	}
}

func TestLocalAICandidateCleanupPreservesSharedDependencyCache(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}

	tempDir := t.TempDir()
	workDir := filepath.Join(tempDir, "work")
	junctionDir := filepath.Join(workDir, "src", "ui", "node_modules")
	dependencyDir := filepath.Join(tempDir, "dependencies")
	if err := os.MkdirAll(filepath.Dir(junctionDir), 0o700); err != nil {
		t.Fatalf("create work fixture: %v", err)
	}
	if err := os.MkdirAll(dependencyDir, 0o700); err != nil {
		t.Fatalf("create dependency fixture: %v", err)
	}
	markerPath := filepath.Join(dependencyDir, "keep.txt")
	if err := os.WriteFile(markerPath, []byte("shared cache"), 0o600); err != nil {
		t.Fatalf("write shared cache marker: %v", err)
	}
	harnessPath := filepath.Join(tempDir, "cleanup.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
New-Item -ItemType Junction -Path %s -Target %s | Out-Null
Remove-SmokeCandidateWork -WorkDirectory %s -DependencyJunctionPath %s
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(junctionDir),
		localAICandidatePowerShellLiteral(dependencyDir),
		localAICandidatePowerShellLiteral(workDir),
		localAICandidatePowerShellLiteral(junctionDir),
	)
	if err := os.WriteFile(harnessPath, []byte(harness), 0o600); err != nil {
		t.Fatalf("write cleanup harness: %v", err)
	}
	if output, err := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-File", harnessPath).CombinedOutput(); err != nil {
		t.Fatalf("clean candidate work: %v\n%s", err, output)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("candidate work directory remains after cleanup: %v", err)
	}
	contents, err := os.ReadFile(markerPath)
	if err != nil || string(contents) != "shared cache" {
		t.Fatalf("shared dependency cache was modified: %v %q", err, contents)
	}
}

func TestLocalAICandidateSourceIdentityComesFromRuntimeInput(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}

	repoRoot := testutil.MustRepoRoot(t)
	commit := localAICandidateGit(t, repoRoot, "rev-parse", "HEAD")
	tree := localAICandidateGit(t, repoRoot, "rev-parse", "HEAD^{tree}")
	repository := localAICandidateGit(t, repoRoot, "remote", "get-url", "origin")
	tempDir := t.TempDir()
	resultPath := filepath.Join(tempDir, "identity.json")
	harnessPath := filepath.Join(tempDir, "identity.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
Get-CandidateSourceIdentity -SourcePath %s -Commit %s -Repository %s | ConvertTo-Json -Compress | Set-Content -LiteralPath %s -Encoding UTF8
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(repoRoot),
		localAICandidatePowerShellLiteral(commit),
		localAICandidatePowerShellLiteral(repository),
		localAICandidatePowerShellLiteral(resultPath),
	)
	if err := os.WriteFile(harnessPath, []byte(harness), 0o600); err != nil {
		t.Fatalf("write source identity harness: %v", err)
	}
	if output, err := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-File", harnessPath).CombinedOutput(); err != nil {
		t.Fatalf("resolve candidate source identity: %v\n%s", err, output)
	}
	var identity struct {
		Repository string `json:"repository"`
		Commit     string `json:"commit"`
		Tree       string `json:"tree"`
	}
	readJSONFile(t, resultPath, &identity)
	if identity.Repository != repository || identity.Commit != commit || identity.Tree != tree {
		t.Fatalf("source identity = %#v, want repository %s commit %s tree %s", identity, repository, commit, tree)
	}

	badHarness := fmt.Sprintf(`
. %s -InstallDir %s
Get-CandidateSourceIdentity -SourcePath %s -Commit %s -Repository 'https://example.invalid/different'
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(repoRoot),
		localAICandidatePowerShellLiteral(commit),
	)
	badHarnessPath := filepath.Join(tempDir, "bad-identity.ps1")
	if err := os.WriteFile(badHarnessPath, []byte(badHarness), 0o600); err != nil {
		t.Fatalf("write rejected source identity harness: %v", err)
	}
	output, err := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-File", badHarnessPath).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "candidate source origin") {
		t.Fatalf("mismatched repository result = %v\n%s", err, output)
	}
}

func TestLocalAICandidateWorkAccountingSkipsLinkedDependencyTree(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}

	tempDir := t.TempDir()
	workDir := filepath.Join(tempDir, "work")
	dependencyDir := filepath.Join(tempDir, "dependencies")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatalf("create work directory: %v", err)
	}
	if err := os.MkdirAll(dependencyDir, 0o700); err != nil {
		t.Fatalf("create dependency directory: %v", err)
	}
	localBytes := []byte("tracked")
	if err := os.WriteFile(filepath.Join(workDir, "local.txt"), localBytes, 0o600); err != nil {
		t.Fatalf("write local work file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dependencyDir, "large-cache.bin"), make([]byte, 1<<20), 0o600); err != nil {
		t.Fatalf("write linked dependency fixture: %v", err)
	}

	resultPath := filepath.Join(tempDir, "bytes.txt")
	harnessPath := filepath.Join(tempDir, "measure.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
New-Item -ItemType Junction -Path %s -Target %s | Out-Null
Get-SmokeDirectoryBytes %s | Set-Content -LiteralPath %s -Encoding ASCII
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(filepath.Join(workDir, "node_modules")),
		localAICandidatePowerShellLiteral(dependencyDir),
		localAICandidatePowerShellLiteral(workDir),
		localAICandidatePowerShellLiteral(resultPath),
	)
	if err := os.WriteFile(harnessPath, []byte(harness), 0o600); err != nil {
		t.Fatalf("write work accounting harness: %v", err)
	}
	if output, err := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-File", harnessPath).CombinedOutput(); err != nil {
		t.Fatalf("measure candidate work directory: %v\n%s", err, output)
	}
	contents, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read measured work bytes: %v", err)
	}
	got, err := strconv.ParseInt(strings.TrimSpace(string(contents)), 10, 64)
	if err != nil {
		t.Fatalf("parse measured work bytes: %v", err)
	}
	if got != int64(len(localBytes)) {
		t.Fatalf("measured work bytes = %d, want only local bytes %d", got, len(localBytes))
	}
}

func TestLocalAICandidatePublicInstallUsesPrebuiltArtifact(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	binaryPath := strings.TrimSpace(os.Getenv("INFINITE_YOU_LOCALAI_PREBUILT_BINARY"))
	if binaryPath == "" {
		t.Skip("set INFINITE_YOU_LOCALAI_PREBUILT_BINARY to an already compiled you.exe")
	}
	if !filepath.IsAbs(binaryPath) {
		t.Fatalf("INFINITE_YOU_LOCALAI_PREBUILT_BINARY must be absolute: %s", binaryPath)
	}
	assertLocalAICandidateBuildInfo(t, binaryPath)
	goPath, err := exec.LookPath("go.exe")
	if err != nil {
		t.Fatalf("locate Go for build-info inspection: %v", err)
	}
	versionOutput, err := exec.Command(binaryPath, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("read prebuilt candidate version: %v\n%s", err, versionOutput)
	}
	version := strings.TrimSpace(string(versionOutput))
	if version == "" {
		t.Fatal("prebuilt candidate returned an empty version")
	}

	tempDir := t.TempDir()
	candidateDir := filepath.Join(tempDir, "candidate")
	stageDir := filepath.Join(tempDir, "stage")
	workDir := filepath.Join(tempDir, "work")
	installDir := filepath.Join(tempDir, "install")
	for _, directory := range []string{candidateDir, stageDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
	}
	stagedBinary := filepath.Join(stageDir, "you.exe")
	copyLocalAICandidateFile(t, binaryPath, stagedBinary)
	archiveName := "you_" + version + "_windows_amd64.zip"
	archivePath := filepath.Join(candidateDir, archiveName)
	writeLocalAICandidateZip(t, archivePath, stagedBinary)
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read candidate archive: %v", err)
	}
	archiveDigest := sha256.Sum256(archiveBytes)
	checksumPath := filepath.Join(candidateDir, "you_"+version+"_checksums.txt")
	checksum := fmt.Sprintf("%x  %s\n", archiveDigest, archiveName)
	if err := os.WriteFile(checksumPath, []byte(checksum), 0o600); err != nil {
		t.Fatalf("write candidate checksums: %v", err)
	}
	installerPath := filepath.Join(candidateDir, "install.ps1")
	copyLocalAICandidateFile(t, filepath.Join(testutil.MustRepoRoot(t), "scripts", "install.ps1"), installerPath)

	resultPath := filepath.Join(tempDir, "install-result.json")
	harnessPath := filepath.Join(tempDir, "install.ps1")
	harness := fmt.Sprintf(`
. %s -InstallDir %s
$result = Invoke-InstalledCandidateSmoke -CandidateDirectory %s -ArchivePath %s -ChecksumPath %s -InstallerPath %s -ArchiveExecutablePath %s -Version %s -ExpectedSourceCommit %s -GoExecutablePath %s -RequestedInstallDir %s -WorkDirectory %s
$result | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath %s -Encoding UTF8
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(installDir),
		localAICandidatePowerShellLiteral(candidateDir),
		localAICandidatePowerShellLiteral(archivePath),
		localAICandidatePowerShellLiteral(checksumPath),
		localAICandidatePowerShellLiteral(installerPath),
		localAICandidatePowerShellLiteral(stagedBinary),
		localAICandidatePowerShellLiteral(version),
		localAICandidatePowerShellLiteral(localAICandidateSourceCommit),
		localAICandidatePowerShellLiteral(goPath),
		localAICandidatePowerShellLiteral(installDir),
		localAICandidatePowerShellLiteral(workDir),
		localAICandidatePowerShellLiteral(resultPath),
	)
	if err := os.WriteFile(harnessPath, []byte(harness), 0o600); err != nil {
		t.Fatalf("write install harness: %v", err)
	}
	if output, err := exec.Command(localAICandidatePowerShell(t), "-NoProfile", "-NonInteractive", "-File", harnessPath).CombinedOutput(); err != nil {
		t.Fatalf("run public candidate install smoke: %v\n%s", err, output)
	}
	var result struct {
		Status                    string `json:"status"`
		ModelCalls                int    `json:"modelCalls"`
		ModelBackendDownloadBytes int64  `json:"modelBackendDownloadBytes"`
		PathResolution            string `json:"pathResolution"`
		ExecutableBuildInfo       struct {
			SourceRevision string `json:"sourceRevision"`
			VCSModified    bool   `json:"vcsModified"`
		} `json:"executableBuildInfo"`
		Commands []struct {
			Arguments     []string `json:"arguments"`
			ElapsedMillis int64    `json:"elapsedMilliseconds"`
		} `json:"commands"`
	}
	readJSONFile(t, resultPath, &result)
	if result.Status != "PASS" || result.ModelCalls != 0 || result.ModelBackendDownloadBytes != 0 {
		t.Fatalf("public candidate install result = %#v", result)
	}
	if result.PathResolution == "" || !strings.EqualFold(filepath.Clean(result.PathResolution), filepath.Clean(filepath.Join(installDir, "you.exe"))) {
		t.Fatalf("public candidate PATH resolution = %q, want installed executable", result.PathResolution)
	}
	if result.ExecutableBuildInfo.SourceRevision != localAICandidateSourceCommit || result.ExecutableBuildInfo.VCSModified {
		t.Fatalf("public candidate executable build info = %#v", result.ExecutableBuildInfo)
	}
	wantCommands := []string{
		"--version",
		"--help",
		"docs models",
		"models list",
		"models --help",
		"--json models inspect llm",
		"--json models inspect asr",
		"--json models inspect tts",
		"--json models inspect embed",
	}
	seenCommands := make(map[string]bool, len(result.Commands))
	for _, command := range result.Commands {
		if command.ElapsedMillis < 0 {
			t.Fatalf("candidate command has negative elapsed time: %#v", command)
		}
		seenCommands[strings.Join(command.Arguments, " ")] = true
	}
	for _, command := range wantCommands {
		if !seenCommands[command] {
			t.Fatalf("candidate install did not record command %q: %#v", command, result.Commands)
		}
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("install directory remains after smoke: %v", err)
	}
}

func localAICandidateScriptPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(testutil.MustRepoRoot(t), "scripts", "release", "smoke-install.ps1")
}

func localAICandidatePowerShell(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("Windows PowerShell is unavailable")
	}
	return path
}

func localAICandidatePowerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func assertLocalAICandidateBuildInfo(t *testing.T, binaryPath string) {
	t.Helper()
	info, err := buildinfo.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read candidate executable build info: %v", err)
	}
	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		if _, exists := settings[setting.Key]; exists {
			t.Fatalf("candidate executable build info repeats %q", setting.Key)
		}
		settings[setting.Key] = setting.Value
	}
	if settings["vcs.revision"] != localAICandidateSourceCommit || settings["vcs.modified"] != "false" {
		t.Fatalf("candidate executable build info = %#v, want revision %s and vcs.modified=false", settings, localAICandidateSourceCommit)
	}
}

func localAICandidateGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	contents = bytes.TrimPrefix(contents, []byte{0xef, 0xbb, 0xbf})
	if err := json.Unmarshal(contents, target); err != nil {
		t.Fatalf("decode %s: %v\n%s", path, err, contents)
	}
}

func copyLocalAICandidateFile(t *testing.T, sourcePath, destinationPath string) {
	t.Helper()
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatalf("open %s: %v", sourcePath, err)
	}
	defer source.Close()
	destination, err := os.Create(destinationPath)
	if err != nil {
		t.Fatalf("create %s: %v", destinationPath, err)
	}
	if _, err := io.Copy(destination, source); err != nil {
		destination.Close()
		t.Fatalf("copy %s: %v", destinationPath, err)
	}
	if err := destination.Close(); err != nil {
		t.Fatalf("close %s: %v", destinationPath, err)
	}
}

func writeLocalAICandidateZip(t *testing.T, archivePath, binaryPath string) {
	t.Helper()
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create candidate archive: %v", err)
	}
	writer := zip.NewWriter(archive)
	entry, err := writer.Create("you.exe")
	if err != nil {
		t.Fatalf("create candidate archive entry: %v", err)
	}
	binary, err := os.Open(binaryPath)
	if err != nil {
		t.Fatalf("open staged candidate: %v", err)
	}
	if _, err := io.Copy(entry, binary); err != nil {
		binary.Close()
		t.Fatalf("write candidate archive entry: %v", err)
	}
	if err := binary.Close(); err != nil {
		t.Fatalf("close staged candidate: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close candidate archive: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close candidate archive file: %v", err)
	}
}
