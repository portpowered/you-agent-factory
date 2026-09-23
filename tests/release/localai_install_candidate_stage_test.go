package release_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const localAICandidateStageRepository = "https://example.invalid/localai-stage.git"

func TestLocalAICandidateStageCrossVolumeIdentityCapAndCleanup(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("candidate staging uses Windows PowerShell")
	}
	for _, testCase := range []struct {
		name         string
		maximumBytes int64
		driftCommit  bool
		expectBuild  bool
	}{
		{name: "within-cap-before-build", maximumBytes: 4_294_967_296, expectBuild: true},
		{name: "over-cap", maximumBytes: 1},
		{name: "identity-drift", maximumBytes: 4_294_967_296, driftCommit: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runLocalAICandidateStage(t, testCase.maximumBytes, testCase.driftCommit, testCase.expectBuild)
		})
	}
}

type localAICandidateStageFixture struct {
	sourceDir     string
	dependencyDir string
	outputDir     string
	installDir    string
	workDir       string
	releaseTool   string
	goToolDir     string
	buildMarker   string
	reportPath    string
	commit        string
	tree          string
	esbuildHash   string
}

func runLocalAICandidateStage(t *testing.T, maximumBytes int64, driftCommit, expectBuild bool) {
	t.Helper()
	fixture := newLocalAICandidateStageFixture(t)
	harnessPath := filepath.Join(t.TempDir(), "candidate-stage.ps1")
	harness := localAICandidateStageHarness(t, fixture, maximumBytes, driftCommit, expectBuild)
	runLocalAICandidateHarness(t, harnessPath, harness, nil, "")
	assertLocalAICandidateStage(t, fixture, maximumBytes, driftCommit, expectBuild)
}

func newLocalAICandidateStageFixture(t *testing.T) localAICandidateStageFixture {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "ui"), 0o700); err != nil {
		t.Fatal(err)
	}
	localAICandidateRunGit(t, source, "init", "--quiet")
	localAICandidateRunGit(t, source, "config", "user.email", "candidate@example.invalid")
	localAICandidateRunGit(t, source, "config", "user.name", "candidate")
	lock := []byte("fixture bun lock\n")
	writeLocalAICandidateStageFile(t, filepath.Join(source, ".gitignore"), []byte("ui/node_modules/\ndist/\n"))
	writeLocalAICandidateStageFile(t, filepath.Join(source, ".goreleaser.yml"), []byte("version: 2\n"))
	writeLocalAICandidateStageFile(t, filepath.Join(source, "ui", "bun.lock"), lock)
	localAICandidateRunGit(t, source, "add", ".gitignore", ".goreleaser.yml", "ui/bun.lock")
	localAICandidateRunGit(t, source, "commit", "--quiet", "-m", "stage fixture")
	localAICandidateRunGit(t, source, "remote", "add", "origin", localAICandidateStageRepository)
	workDir, err := os.MkdirTemp(`D:\`, "localai-stage-")
	if err != nil {
		t.Skipf("cross-volume candidate staging needs D: available: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })
	if strings.EqualFold(filepath.VolumeName(source), filepath.VolumeName(workDir)) {
		t.Skip("candidate source and work roots must be on different volumes")
	}
	dependency := filepath.Join(root, "dependencies")
	esbuild := []byte("test esbuild executable")
	writeLocalAICandidateStageFile(t, filepath.Join(dependency, "ui", "bun.lock"), lock)
	writeLocalAICandidateStageFile(t, filepath.Join(dependency, "ui", "node_modules", "esbuild", "lib", "downloaded-@esbuild-win32-x64-esbuild.exe"), esbuild)
	writeLocalAICandidateStageFile(t, filepath.Join(dependency, "ui", "node_modules", "retained.bin"), []byte("shared dependency"))
	releaseTool := filepath.Join(root, "goreleaser.exe")
	writeLocalAICandidateStageFile(t, releaseTool, []byte("test runner placeholder"))
	goToolDir := filepath.Join(root, "tools")
	writeLocalAICandidateStageFile(t, filepath.Join(goToolDir, "go.exe"), []byte("test runner placeholder"))
	commit := localAICandidateGit(t, source, "rev-parse", "HEAD")
	tree := localAICandidateGit(t, source, "rev-parse", "HEAD^{tree}")
	return localAICandidateStageFixture{
		sourceDir: source, dependencyDir: dependency, outputDir: filepath.Join(root, "output"),
		installDir: filepath.Join(root, "install"), workDir: workDir, releaseTool: releaseTool,
		goToolDir: goToolDir, buildMarker: filepath.Join(root, "build-started.marker"),
		reportPath: filepath.Join(root, "output", "candidate-report.json"), commit: commit, tree: tree,
		esbuildHash: fmt.Sprintf("%x", sha256.Sum256(esbuild)),
	}
}

func localAICandidateStageHarness(t *testing.T, fixture localAICandidateStageFixture, maximumBytes int64, driftCommit, expectBuild bool) string {
	t.Helper()
	corrupt := "false"
	if driftCommit {
		corrupt = "true"
	}
	build := "false"
	if expectBuild {
		build = "true"
	}
	return fmt.Sprintf(`
. %s -InstallDir %s
$env:PATH = %s + ";" + $env:PATH
$env:LOCALAI_STAGE_TEST_BUILD_MARKER = %s
$env:LOCALAI_STAGE_TEST_CORRUPT_COMMIT = "%s"
$env:LOCALAI_STAGE_TEST_EXPECT_BUILD = "%s"
function Invoke-CandidateCommand {
    param([string]$FilePath, [string[]]$ArgumentList, [string]$WorkingDirectory, [string]$StdoutPath, [string]$StderrPath)
    $exitCode = 0
    $stdout = if ($FilePath -ieq %s) {
        if ($ArgumentList -contains "--version") { "goreleaser version 2.0.0" }
        else {
            [System.IO.File]::WriteAllText($env:LOCALAI_STAGE_TEST_BUILD_MARKER, "build invoked")
            $exitCode = 23
            "release started"
        }
    } elseif ([System.IO.Path]::GetFileName($FilePath) -ieq "go.exe") {
        "go version go1.26.8 windows/amd64"
    } else { "0.27.7" }
    [System.IO.File]::WriteAllText($StdoutPath, $stdout)
    [System.IO.File]::WriteAllText($StderrPath, "")
    return [pscustomobject]@{
        exitCode = $exitCode; stdout = $stdout; stderr = ""; elapsedMilliseconds = 1
        timedOut = $false; outputLimitExceeded = $false; terminationReason = ""
        stdoutEvidence = Get-SmokeFileEvidence "fixture stdout" $StdoutPath
        stderrEvidence = Get-SmokeFileEvidence "fixture stderr" $StderrPath
    }
}
if ($env:LOCALAI_STAGE_TEST_CORRUPT_COMMIT -eq "true") {
    $script:originalCandidateStage = (Get-Command New-SmokeCandidateGitStage -CommandType Function).ScriptBlock
    function New-SmokeCandidateGitStage {
        param([string]$SourcePath, [string]$StagePath, [string]$Repository, [string]$Commit, [string]$WorkDirectory)
        $evidence = & $script:originalCandidateStage @PSBoundParameters
        $evidence.commit = "0000000000000000000000000000000000000000"
        return $evidence
    }
}
$expectedFailure = if ($env:LOCALAI_STAGE_TEST_EXPECT_BUILD -eq "true") {
    "GoReleaser exited 23"
} elseif ($env:LOCALAI_STAGE_TEST_CORRUPT_COMMIT -eq "true") {
    "candidate staged source identity mismatch"
} else { "candidate staged work directory used" }
try {
    Invoke-LocalCandidateSmoke -SourcePath %s -SourceCommit %s -SourceRepository %s -DependencySourcePath %s -OutputDirectory %s -WorkDirectory %s -ReleaseToolPath %s -ReleaseToolVersion "2.0.0" -GoVersion "1.26.8" -GoProcessLimit 1 -EsbuildVersion "0.27.7" -EsbuildSHA256 %s -MaximumWorkBytes %d -RequestedInstallDir %s -RequestedReportPath %s
    throw "candidate staging unexpectedly passed its fail-closed check"
} catch {
    if ($_.Exception.Message -notlike ("*" + $expectedFailure + "*")) { throw }
}
if ((Test-Path -LiteralPath $env:LOCALAI_STAGE_TEST_BUILD_MARKER) -ne ($env:LOCALAI_STAGE_TEST_EXPECT_BUILD -eq "true")) {
    throw "GoReleaser marker does not match the expected staging gate outcome"
}
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(fixture.installDir),
		localAICandidatePowerShellLiteral(fixture.goToolDir),
		localAICandidatePowerShellLiteral(fixture.buildMarker), corrupt, build,
		localAICandidatePowerShellLiteral(fixture.releaseTool),
		localAICandidatePowerShellLiteral(fixture.sourceDir),
		localAICandidatePowerShellLiteral(fixture.commit),
		localAICandidatePowerShellLiteral(localAICandidateStageRepository),
		localAICandidatePowerShellLiteral(fixture.dependencyDir),
		localAICandidatePowerShellLiteral(fixture.outputDir),
		localAICandidatePowerShellLiteral(fixture.workDir),
		localAICandidatePowerShellLiteral(fixture.releaseTool),
		localAICandidatePowerShellLiteral(fixture.esbuildHash), maximumBytes,
		localAICandidatePowerShellLiteral(fixture.installDir),
		localAICandidatePowerShellLiteral(fixture.reportPath),
	)
}

func assertLocalAICandidateStage(t *testing.T, fixture localAICandidateStageFixture, maximumBytes int64, driftCommit, expectBuild bool) {
	t.Helper()
	var report struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Source struct {
			Commit      string `json:"commit"`
			Tree        string `json:"tree"`
			VCSModified bool   `json:"vcsModified"`
		} `json:"source"`
		Stage struct {
			Status           string `json:"status"`
			RequestedCommit  string `json:"requestedCommit"`
			RequestedTree    string `json:"requestedTree"`
			StagedCommit     string `json:"stagedCommit"`
			StagedTree       string `json:"stagedTree"`
			Porcelain        string `json:"porcelain"`
			Shallow          bool   `json:"shallow"`
			WorkBytes        int64  `json:"workBytes"`
			MaximumWorkBytes int64  `json:"maximumWorkBytes"`
		} `json:"stage"`
		Build struct {
			WorkBytes int64 `json:"workBytes"`
		} `json:"build"`
		Cleanup struct {
			Status                  string `json:"status"`
			WorkDirectoryRemoved    bool   `json:"workDirectoryRemoved"`
			InstallDirectoryRemoved bool   `json:"installDirectoryRemoved"`
		} `json:"cleanup"`
	}
	readJSONFile(t, fixture.reportPath, &report)
	t.Logf("stage report: requested=%s tree=%s; staged=%s tree=%s; clean=%t shallow=%t workBytes=%d cap=%d stage=%s buildBoundaryReached=%t cleanup=%s",
		report.Stage.RequestedCommit, report.Stage.RequestedTree, report.Stage.StagedCommit,
		report.Stage.StagedTree, report.Stage.Porcelain == "", report.Stage.Shallow,
		report.Stage.WorkBytes, report.Stage.MaximumWorkBytes, report.Stage.Status, expectBuild,
		report.Cleanup.Status)
	wantStageStatus := "FAIL"
	if expectBuild {
		wantStageStatus = "PASS"
	}
	if report.Status != "FAIL" || report.Stage.Status != wantStageStatus {
		t.Fatalf("candidate stage report did not fail closed: %#v", report)
	}
	if report.Source.Commit != fixture.commit || report.Source.Tree != fixture.tree || report.Source.VCSModified ||
		report.Stage.RequestedCommit != fixture.commit || report.Stage.RequestedTree != fixture.tree {
		t.Fatalf("candidate stage report lost requested identity: %#v", report)
	}
	if report.Stage.StagedTree != fixture.tree || report.Stage.WorkBytes <= 0 ||
		!report.Stage.Shallow || report.Stage.Porcelain != "" {
		t.Fatalf("candidate stage evidence = %#v, want exact shallow clean tree and measured bytes", report.Stage)
	}
	buildBytesMatch := report.Build.WorkBytes == report.Stage.WorkBytes
	if expectBuild {
		buildBytesMatch = report.Build.WorkBytes >= report.Stage.WorkBytes
	}
	if report.Stage.MaximumWorkBytes != maximumBytes || !buildBytesMatch {
		t.Fatalf("candidate stage byte report = %#v build=%#v", report.Stage, report.Build)
	}
	if !expectBuild && (!strings.Contains(report.Error, fixture.commit) || !strings.Contains(report.Error, fixture.tree) ||
		!strings.Contains(report.Error, report.Stage.StagedCommit) || !strings.Contains(report.Error, report.Stage.StagedTree)) {
		t.Fatalf("candidate stage failure omitted measured source identities: %#v", report)
	}
	if expectBuild && (report.Stage.StagedCommit != fixture.commit || report.Stage.WorkBytes > maximumBytes) {
		t.Fatalf("under-cap candidate stage did not preserve exact identity before build: %#v", report.Stage)
	}
	if driftCommit && report.Stage.StagedCommit == fixture.commit {
		t.Fatalf("identity drift was not reported: %#v", report)
	}
	if !expectBuild && !driftCommit && (report.Stage.WorkBytes <= maximumBytes || !strings.Contains(report.Error, fmt.Sprint(report.Stage.WorkBytes))) {
		t.Fatalf("over-cap failure omitted measured bytes: %#v", report)
	}
	if report.Cleanup.Status != "PASS" || !report.Cleanup.WorkDirectoryRemoved || !report.Cleanup.InstallDirectoryRemoved {
		t.Fatalf("candidate failure cleanup = %#v", report.Cleanup)
	}
	if _, err := os.Stat(fixture.workDir); !os.IsNotExist(err) {
		t.Fatalf("owned work root remains after candidate failure: %v", err)
	}
	if _, err := os.Stat(fixture.installDir); !os.IsNotExist(err) {
		t.Fatalf("owned install root remains after candidate failure: %v", err)
	}
	dependencySentinel := filepath.Join(fixture.dependencyDir, "ui", "node_modules", "retained.bin")
	if data, err := os.ReadFile(dependencySentinel); err != nil || string(data) != "shared dependency" {
		t.Fatalf("shared dependency tree changed during candidate cleanup: %v %q", err, data)
	}
}

func writeLocalAICandidateStageFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}
