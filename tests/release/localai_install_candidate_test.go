package release_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
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

	"github.com/portpowered/infinite-you/internal/testutil"
)

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
	if result.ExitCode != 23 || len(result.Arguments) != len(wantArguments)+4 {
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
$result = Invoke-InstalledCandidateSmoke -CandidateDirectory %s -ArchivePath %s -ChecksumPath %s -InstallerPath %s -ArchiveExecutablePath %s -Version %s -RequestedInstallDir %s -WorkDirectory %s
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
	}
	readJSONFile(t, resultPath, &result)
	if result.Status != "PASS" || result.ModelCalls != 0 || result.ModelBackendDownloadBytes != 0 {
		t.Fatalf("public candidate install result = %#v", result)
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
