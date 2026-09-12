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

type localAICandidatePromotionFailureFixture struct {
	tempDir       string
	sourceDir     string
	sourceCommit  string
	dependencyDir string
	releaseTool   string
	outputDir     string
	workDir       string
	installDir    string
	reportPath    string
	resultPath    string
}

func TestLocalAICandidatePromotionFailureStillCleansOwnedRoots(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell candidate delivery is Windows-only")
	}
	fixture := newLocalAICandidatePromotionFailureFixture(t)
	harnessPath := filepath.Join(fixture.tempDir, "promotion-failure.ps1")
	runLocalAICandidateHarness(t, harnessPath,
		localAICandidatePromotionFailureHarness(t, fixture), nil, "")
	var result struct {
		Caught                       string `json:"caught"`
		Status                       string `json:"status"`
		CleanupStatus                string `json:"cleanupStatus"`
		InstallDirectoryRemoved      bool   `json:"installDirectoryRemoved"`
		WorkDirectoryRemoved         bool   `json:"workDirectoryRemoved"`
		RetainedEvidenceHashesStable bool   `json:"retainedEvidenceHashesStable"`
		OutputExists                 bool   `json:"outputExists"`
		ReportExists                 bool   `json:"reportExists"`
		DigestExists                 bool   `json:"digestExists"`
		CommandEvidenceCount         int    `json:"commandEvidenceCount"`
		VersionCommandsRecorded      bool   `json:"versionCommandsRecorded"`
	}
	readJSONFile(t, fixture.resultPath, &result)
	if !strings.Contains(result.Caught, "controlled command-evidence promotion failure") ||
		result.Status != "FAIL" || result.CleanupStatus != "FAIL" ||
		!result.InstallDirectoryRemoved || !result.WorkDirectoryRemoved ||
		!result.RetainedEvidenceHashesStable || !result.OutputExists ||
		!result.ReportExists || !result.DigestExists ||
		result.CommandEvidenceCount != 0 || !result.VersionCommandsRecorded {
		t.Fatalf("promotion failure cleanup = %#v", result)
	}
	for name, path := range map[string]string{
		"install": fixture.installDir,
		"work":    fixture.workDir,
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s directory remains after promotion failure: %v", name, err)
		}
	}
}

func newLocalAICandidatePromotionFailureFixture(t *testing.T) localAICandidatePromotionFailureFixture {
	t.Helper()
	tempDir, err := os.MkdirTemp(os.TempDir(), "lmx-")
	if err != nil {
		t.Fatalf("create short candidate fixture root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	fixture := localAICandidatePromotionFailureFixture{
		tempDir:       tempDir,
		sourceDir:     filepath.Join(tempDir, "source"),
		dependencyDir: filepath.Join(tempDir, "dependencies"),
		releaseTool:   filepath.Join(tempDir, "goreleaser.exe"),
		outputDir:     filepath.Join(tempDir, "output"),
		workDir:       filepath.Join(tempDir, "work"),
		installDir:    filepath.Join(tempDir, "install"),
	}
	fixture.reportPath = filepath.Join(fixture.outputDir, "candidate-report.json")
	fixture.resultPath = filepath.Join(tempDir, "promotion-failure.json")
	prepareLocalAICandidatePromotionSource(t, fixture)
	localAICandidateRunGit(t, fixture.sourceDir, "init", "--quiet")
	localAICandidateRunGit(t, fixture.sourceDir, "config", "user.email", "candidate@example.invalid")
	localAICandidateRunGit(t, fixture.sourceDir, "config", "user.name", "candidate")
	localAICandidateRunGit(t, fixture.sourceDir, "add", ".")
	localAICandidateRunGit(t, fixture.sourceDir, "commit", "--quiet", "-m", "candidate fixture")
	localAICandidateRunGit(t, fixture.sourceDir, "remote", "add", "origin", "https://example.invalid/candidate.git")
	fixture.sourceCommit = localAICandidateGit(t, fixture.sourceDir, "rev-parse", "HEAD")
	if err := os.WriteFile(fixture.releaseTool, []byte("controlled release tool"), 0o600); err != nil {
		t.Fatalf("write release tool fixture: %v", err)
	}
	return fixture
}

func prepareLocalAICandidatePromotionSource(t *testing.T, fixture localAICandidatePromotionFailureFixture) {
	t.Helper()
	for _, directory := range []string{
		fixture.sourceDir,
		filepath.Join(fixture.dependencyDir, "ui", "node_modules", "esbuild", "lib"),
		filepath.Join(fixture.sourceDir, "ui"),
		filepath.Join(fixture.sourceDir, "scripts"),
	} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create candidate fixture directory: %v", err)
		}
	}
	lockContents := []byte("candidate lock\n")
	for _, path := range []string{
		filepath.Join(fixture.sourceDir, "ui", "bun.lock"),
		filepath.Join(fixture.dependencyDir, "ui", "bun.lock"),
	} {
		if err := os.WriteFile(path, lockContents, 0o600); err != nil {
			t.Fatalf("write lock file %s: %v", path, err)
		}
	}
	for path, contents := range map[string][]byte{
		filepath.Join(fixture.sourceDir, ".gitignore"):             []byte("ui/node_modules/\n"),
		filepath.Join(fixture.sourceDir, ".goreleaser.yml"):        []byte("fixture release config\n"),
		filepath.Join(fixture.sourceDir, "scripts", "install.ps1"): []byte("fixture installer\n"),
	} {
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatalf("write source fixture %s: %v", path, err)
		}
	}
	nativePath := filepath.Join(fixture.dependencyDir, "ui", "node_modules", "esbuild", "lib", "downloaded-@esbuild-win32-x64-esbuild.exe")
	if err := os.WriteFile(nativePath, []byte("esbuild fixture"), 0o600); err != nil {
		t.Fatalf("write native fixture: %v", err)
	}
}

func localAICandidatePromotionFailureHarness(t *testing.T, fixture localAICandidatePromotionFailureFixture) string {
	t.Helper()
	esbuildDigest := fmt.Sprintf("%x", sha256.Sum256([]byte("esbuild fixture")))
	return fmt.Sprintf(`
. %s -InstallDir %s
$releaseTool = %s
$sourcePath = %s
$sourceCommit = %s
$dependencyPath = %s
$outputPath = %s
$workPath = %s
$installPath = %s
$reportPath = %s
$resultPath = %s
$version = '1.0.1-snapshot-fixture'
%s
function Expand-Archive {
    param([string]$LiteralPath, [string]$DestinationPath, [switch]$Force)
    [void][System.IO.Directory]::CreateDirectory($DestinationPath)
    [System.IO.File]::WriteAllText((Join-Path $DestinationPath 'you.exe'), 'candidate executable')
}
function Invoke-InstalledCandidateSmoke {
    param([string]$RequestedInstallDir)
    [void][System.IO.Directory]::CreateDirectory($RequestedInstallDir)
    [System.IO.File]::WriteAllText((Join-Path $RequestedInstallDir 'you.exe'), 'installed executable')
    return [pscustomobject][ordered]@{
        status = 'PASS'
        executableBuildInfo = [ordered]@{ sourceRevision = $sourceCommit; vcsModified = $false }
    }
}
function Promote-SmokeCommandEvidence {
    throw 'controlled command-evidence promotion failure'
}
$caught = ''
try {
    Invoke-LocalCandidateSmoke -SourcePath $sourcePath -SourceCommit $sourceCommit -SourceRepository 'https://example.invalid/candidate.git' -DependencySourcePath $dependencyPath -OutputDirectory $outputPath -WorkDirectory $workPath -GoProcessLimit 2 -ReleaseToolPath $releaseTool -ReleaseToolVersion 'v2.12.7' -EsbuildVersion '0.27.7' -EsbuildSHA256 %s -MaximumWorkBytes 1048576 -RequestedInstallDir $installPath -RequestedReportPath $reportPath
    $caught = 'UNEXPECTED_PASS'
} catch {
    $caught = $_.Exception.Message
}
$report = Get-Content -LiteralPath $reportPath -Raw | ConvertFrom-Json
$stable = $false
$cleanupPropertyNames = @($report.cleanup.PSObject.Properties | ForEach-Object { $_.Name })
if ($cleanupPropertyNames -contains 'retainedEvidenceHashesStable') { $stable = [bool]$report.cleanup.retainedEvidenceHashesStable }
$buildPropertyNames = @($report.build.PSObject.Properties | ForEach-Object { $_.Name })
$versionCommandsRecorded = ($buildPropertyNames -contains 'esbuildVersionCommand') -and
    ($buildPropertyNames -contains 'goReleaserVersionCommand')
[ordered]@{
    caught = $caught
    status = $report.status
    cleanupStatus = $report.cleanup.status
    installDirectoryRemoved = $report.cleanup.installDirectoryRemoved
    workDirectoryRemoved = $report.cleanup.workDirectoryRemoved
    retainedEvidenceHashesStable = $stable
    outputExists = Test-Path -LiteralPath $outputPath -PathType Container
    reportExists = Test-Path -LiteralPath $reportPath -PathType Leaf
    digestExists = Test-Path -LiteralPath (Join-Path $outputPath 'candidate-report.sha256') -PathType Leaf
    commandEvidenceCount = @($report.commandEvidence).Count
    versionCommandsRecorded = $versionCommandsRecorded
} | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $resultPath -Encoding UTF8
if (-not $caught.Contains('controlled command-evidence promotion failure') -or $report.status -ne 'FAIL' -or
    $report.cleanup.status -ne 'FAIL' -or -not $report.cleanup.installDirectoryRemoved -or
    -not $report.cleanup.workDirectoryRemoved -or -not $stable -or
    @($report.commandEvidence).Count -ne 0 -or -not $versionCommandsRecorded) { exit 3 }
`,
		localAICandidatePowerShellLiteral(localAICandidateScriptPath(t)),
		localAICandidatePowerShellLiteral(filepath.Join(fixture.tempDir, "unused-install")),
		localAICandidatePowerShellLiteral(fixture.releaseTool),
		localAICandidatePowerShellLiteral(fixture.sourceDir),
		localAICandidatePowerShellLiteral(fixture.sourceCommit),
		localAICandidatePowerShellLiteral(fixture.dependencyDir),
		localAICandidatePowerShellLiteral(fixture.outputDir),
		localAICandidatePowerShellLiteral(fixture.workDir),
		localAICandidatePowerShellLiteral(fixture.installDir),
		localAICandidatePowerShellLiteral(fixture.reportPath),
		localAICandidatePowerShellLiteral(fixture.resultPath),
		localAICandidatePromotionCommandFunction(),
		localAICandidatePowerShellLiteral(esbuildDigest),
	)
}

func localAICandidatePromotionCommandFunction() string {
	return `
function Invoke-CandidateCommand {
    param([string]$FilePath, [string[]]$ArgumentList, [string]$WorkingDirectory, [string]$StdoutPath, [string]$StderrPath)
    $stdout = ''
    if (@($ArgumentList) -contains '--version') {
        if ($FilePath -like '*esbuild-native.exe') { $stdout = '0.27.7' }
        elseif ($FilePath -eq $releaseTool) { $stdout = 'goreleaser version v2.12.7' }
        else { $stdout = $version }
    } elseif (@($ArgumentList) -contains 'version') {
        $stdout = 'go version go1.26.8 windows/amd64'
    } elseif (@($ArgumentList) -contains 'release') {
        $dist = Join-Path $WorkingDirectory 'dist'
        [void][System.IO.Directory]::CreateDirectory($dist)
        foreach ($name in @(
            ('you_' + $version + '_darwin_amd64.tar.gz'),
            ('you_' + $version + '_darwin_arm64.tar.gz'),
            ('you_' + $version + '_linux_amd64.tar.gz'),
            ('you_' + $version + '_linux_arm64.tar.gz'),
            ('you_' + $version + '_windows_amd64.zip'),
            ('you_' + $version + '_windows_arm64.zip'),
            ('you_' + $version + '_checksums.txt'))) {
            [System.IO.File]::WriteAllText((Join-Path $dist $name), 'candidate artifact')
        }
    }
    [System.IO.File]::WriteAllText($StdoutPath, $stdout, [System.Text.UTF8Encoding]::new($false))
    [System.IO.File]::WriteAllText($StderrPath, '', [System.Text.UTF8Encoding]::new($false))
    return [pscustomobject][ordered]@{
        file = $FilePath
        arguments = @($ArgumentList)
        exitCode = 0
        timedOut = $false
        outputLimitExceeded = $false
        terminationReason = ''
        elapsedMilliseconds = 1
        stdout = $stdout
        stderr = ''
        stdoutEvidence = Get-SmokeFileEvidence 'command-output' $StdoutPath
        stderrEvidence = Get-SmokeFileEvidence 'command-output' $StderrPath
    }
}`
}
