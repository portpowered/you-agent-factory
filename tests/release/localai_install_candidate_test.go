package release_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	localAICandidateManifestEnv, localAICandidateSchemaVersion, localAICandidateProject                                                                                                                                                                                                                                                  = "INFINITE_YOU_LOCALAI_CANDIDATE_MANIFEST", "localai-windows-install-candidate/v1", "localai"
	localAICandidateCycle, localAICandidateGoVersion, localAICandidateGOFLAGS, localAICandidateGOMAXPROCS, localAICandidateObserverQueryMode                                                                                                                                                                                             = "076", "go1.26.8", "-p=4", "4", "one-full-TCP-table-query-per-interval-filtered-to-owned-process-identities"
	localAICandidateRepository, localAICandidateSourceCommit, localAICandidateSourceTree                                                                                                                                                                                                                                                 = "https://github.com/portpowered/you-agent-factory", "059474b2c00915865306a33ca5e3d02b618bb6f0", "ae5d9c87fbc99a898ad2c11e1409105d78e4a094"
	localAICandidateGoReleaser, localAICandidateGOOS, localAICandidateGOARCH                                                                                                                                                                                                                                                             = "v2.12.7", "windows", "amd64"
	localAICandidateNativeToolVersion, localAICandidateNativeToolSHA256                                                                                                                                                                                                                                                                  = "0.27.7", "44ce6728d54c891b1c5a6d7dbfb1a0f13419884cca0b090f1fbcf0dcd8bee0e9"
	localAICandidateTemporaryDiskMaximum, localAICandidateToolDownloadMaximum, localAICandidateModelDownloadMaximum, localAICandidateModelCallsMaximum, localAICandidatePaidUSDMaximum, localAICandidateDescendantMaximum, localAICandidateRerunsMaximum, localAICandidateProcessNetworkGapMaximum, localAICandidateDiskGapMaximum int64 = 4294967296, 0, 0, 0, 0, 36, 1, 2000, 2000
	localAICandidateNativeToolBytes                                                                                                                                                                                                                                                                                                int64 = 11386368
)

var localAICandidateSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type localAICandidateManifest struct {
	SchemaVersion string                     `json:"schemaVersion"`
	Project       string                     `json:"project"`
	Cycle         string                     `json:"cycle"`
	Source        localAICandidateSource     `json:"source"`
	Build         localAICandidateBuild      `json:"build"`
	Artifacts     []localAICandidateArtifact `json:"artifacts"`
	Limits        localAICandidateLimits     `json:"limits"`
	Attempts      localAICandidateAttempts   `json:"attempts"`
}
type localAICandidateSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Tree       string `json:"tree"`
}
type localAICandidateBuild struct {
	CandidateVersion       string                 `json:"candidateVersion"`
	CLIVersion             string                 `json:"cliVersion"`
	GoVersion              string                 `json:"goVersion"`
	GoReleaserVersion      string                 `json:"goreleaserVersion"`
	GoReleaserConfigSHA256 string                 `json:"goreleaserConfigSha256"`
	Target                 localAICandidateTarget `json:"target"`
	Environment            map[string]string      `json:"environment"`
}
type localAICandidateTarget struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	CgoEnabled *bool  `json:"cgoEnabled"`
}
type localAICandidateArtifact struct {
	Role   string `json:"role"`
	File   string `json:"file"`
	Bytes  *int64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type localAICandidateLimits struct {
	TemporaryDiskBytesMaximum     *int64 `json:"temporaryDiskBytesMaximum"`
	OrdinaryToolDownloadMaximum   *int64 `json:"ordinaryToolDownloadBytesMaximum"`
	ModelBackendDownloadMaximum   *int64 `json:"modelBackendDownloadBytesMaximum"`
	ModelCallsMaximum             *int64 `json:"modelCallsMaximum"`
	PaidUSDMaximum                *int64 `json:"paidUSDMaximum"`
	DescendantMaximum             *int64 `json:"descendantMaximum"`
	PackagingOrSmokeRerunsMaximum *int64 `json:"packagingOrSmokeRerunsMaximum"`
	GOFLAGS                       string `json:"goflags"`
	GOMAXPROCS                    string `json:"gomaxprocs"`
}
type localAICandidateAttempts struct {
	PriorCycles          []string `json:"priorCycles"`
	Cycle063BuildMaximum *int64   `json:"cycle063BuildMaximum"`
	Cycle063BuildUsed    *int64   `json:"cycle063BuildUsed"`
	Cycle067BuildMaximum *int64   `json:"cycle067BuildMaximum"`
	Cycle067BuildUsed    *int64   `json:"cycle067BuildUsed"`
	Cycle068BuildMaximum *int64   `json:"cycle068BuildMaximum"`
	Cycle068BuildUsed    *int64   `json:"cycle068BuildUsed"`
	Cycle070BuildMaximum *int64   `json:"cycle070BuildMaximum"`
	Cycle070BuildUsed    *int64   `json:"cycle070BuildUsed"`
	Cycle076BuildMaximum *int64   `json:"cycle076BuildMaximum"`
	Cycle076BuildUsed    *int64   `json:"cycle076BuildUsed"`
}
type localAICandidateArtifactEvidence struct {
	Role   string `json:"role"`
	File   string `json:"file"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type localAICandidateValidationEvidence struct {
	Status                 string                           `json:"status"`
	Property               string                           `json:"property"`
	SchemaVersion          string                           `json:"schemaVersion"`
	SourceCommit           string                           `json:"sourceCommit"`
	SourceTree             string                           `json:"sourceTree"`
	EmbeddedSourceRevision string                           `json:"embeddedSourceRevision"`
	EmbeddedVCSModified    bool                             `json:"embeddedVCSModified"`
	CandidateVersion       string                           `json:"candidateVersion"`
	CLIVersion             string                           `json:"cliVersion"`
	Target                 string                           `json:"target"`
	Archive                localAICandidateArtifactEvidence `json:"archive"`
	Installer              localAICandidateArtifactEvidence `json:"installer"`
	Manifest               localAICandidateArtifactEvidence `json:"manifest"`
	Preconditions          string                           `json:"preconditions"`
}
type localAICandidateRoot struct {
	Name string
	Path string
}
type localAICandidateProcessNetworkObserver struct {
	StartedBeforeRoot              bool     `json:"startedBeforeRoot"`
	ContinuedThroughDescendantExit bool     `json:"continuedThroughDescendantExit"`
	MaximumGapMilliseconds         int64    `json:"maximumGapMilliseconds"`
	Status                         string   `json:"status"`
	NonLoopbackConnections         int      `json:"nonLoopbackConnections"`
	ExternalTransferBytes          int64    `json:"externalTransferBytes"`
	SampleCount                    int      `json:"sampleCount"`
	TCPTableQueries                int      `json:"tcpTableQueries"`
	ZeroConnectionSamples          int      `json:"zeroConnectionSamples"`
	OwnedConnectionMatches         int      `json:"ownedConnectionMatches"`
	OwnedProcessIdentityCount      int      `json:"ownedProcessIdentityCount"`
	QueryMode                      string   `json:"queryMode"`
	ForbiddenProcesses             []string `json:"forbiddenProcesses"`
	Error                          string   `json:"error"`
	ReleaseStatusCleanBeforeRoot   bool     `json:"releaseStatusCleanBeforeRoot"`
	ReleaseStatusCleanDuringRoot   bool     `json:"releaseStatusCleanDuringRoot"`
	ReleaseStatusCleanAfterOutput  bool     `json:"releaseStatusCleanAfterOutput"`
	ReleaseStatusChecks            int      `json:"releaseStatusChecks"`
}
type localAICandidateDiskObserver struct {
	Independent            bool   `json:"independent"`
	StartPeriodicFinal     bool   `json:"startPeriodicFinal"`
	MaximumGapMilliseconds int64  `json:"maximumGapMilliseconds"`
	Status                 string `json:"status"`
	PeakDeltaBytes         int64  `json:"peakDeltaBytes"`
	Error                  string `json:"error"`
}
type localAICandidateRootOutputStream struct {
	Status        string `json:"status"`
	Path          string `json:"path"`
	Present       bool   `json:"present"`
	TotalBytes    int64  `json:"totalBytes"`
	CapturedBytes int64  `json:"capturedBytes"`
	Truncated     bool   `json:"truncated"`
	RedactedBytes int64  `json:"redactedBytes"`
	SHA256        string `json:"sha256"`
}
type localAICandidateRootOutput struct {
	Status       string                           `json:"status"`
	MaximumBytes int64                            `json:"maximumBytes"`
	Stdout       localAICandidateRootOutputStream `json:"stdout"`
	Stderr       localAICandidateRootOutputStream `json:"stderr"`
}
type localAICandidateObserverEvidence struct {
	Environment            map[string]string                      `json:"environment"`
	ProcessNetworkObserver localAICandidateProcessNetworkObserver `json:"processNetworkObserver"`
	DiskObserver           localAICandidateDiskObserver           `json:"diskObserver"`
	DescendantMaximum      int64                                  `json:"descendantMaximum"`
	DescendantHighWater    int64                                  `json:"descendantHighWater"`
	RootExitCode           *int64                                 `json:"rootExitCode"`
	RootOutput             localAICandidateRootOutput             `json:"rootOutput"`
	FailurePropagation     string                                 `json:"failurePropagation"`
	ModelBackendBytes      int64                                  `json:"modelBackendBytes"`
	ModelBackendCalls      int64                                  `json:"modelBackendCalls"`
}
type localAICandidateObserverCleanup struct {
	Status             string   `json:"status"`
	RemainingTaskPaths []string `json:"remainingTaskPaths"`
	Errors             []string `json:"errors"`
}
type localAICandidateReleaseCheckoutEvidence struct {
	Status                      string   `json:"status"`
	GitDirIsDirectory           bool     `json:"gitDirIsDirectory"`
	CheckoutPath                string   `json:"checkoutPath"`
	DetachedHead                bool     `json:"detachedHead"`
	Head                        string   `json:"head"`
	Tree                        string   `json:"tree"`
	Origin                      string   `json:"origin"`
	PrivateExcludePath          string   `json:"privateExcludePath"`
	PrivateExcludeBefore        []string `json:"privateExcludeBefore"`
	PrivateExcludeAfter         []string `json:"privateExcludeAfter"`
	PrivateExcludeAdded         []string `json:"privateExcludeAdded"`
	StatusCleanBefore           bool     `json:"statusCleanBefore"`
	StatusCleanAfterPreparation bool     `json:"statusCleanAfterPreparation"`
	StatusCleanDuringOutput     bool     `json:"statusCleanDuringOutput"`
	StatusCleanAfterOutput      bool     `json:"statusCleanAfterOutput"`
	StatusChecks                int      `json:"statusChecks"`
}
type localAICandidateDependencyManifestEvidence struct {
	Status            string `json:"status"`
	ManifestPath      string `json:"manifestPath"`
	ManifestSHA256    string `json:"manifestSHA256"`
	EntryCount        int64  `json:"entryCount"`
	FileCount         int64  `json:"fileCount"`
	DirectoryCount    int64  `json:"directoryCount"`
	JunctionCount     int64  `json:"junctionCount"`
	SymbolicLinkCount int64  `json:"symbolicLinkCount"`
	TotalBytes        int64  `json:"totalBytes"`
}
type localAICandidateDependencyReport struct {
	SchemaVersion  string                                     `json:"schemaVersion"`
	Status         string                                     `json:"status"`
	Phase          string                                     `json:"phase"`
	SourceRoot     string                                     `json:"sourceRoot"`
	StageRoot      string                                     `json:"stageRoot"`
	PackageInstall string                                     `json:"packageInstallation"`
	SourceManifest localAICandidateDependencyManifestEvidence `json:"sourceManifest"`
	StageManifest  localAICandidateDependencyManifestEvidence `json:"stageManifest"`
	SourceLock     any                                        `json:"sourceLockClosure"`
	StageLock      any                                        `json:"stageLockClosure"`
	Copy           any                                        `json:"copy"`
	Equality       struct {
		SourceStageManifestEqual  bool  `json:"sourceStageManifestEqual"`
		SourceStagePostBuildEqual bool  `json:"sourceStagePostBuildEqual"`
		SourceEntryCount          int64 `json:"sourceEntryCount"`
		StageEntryCount           int64 `json:"stageEntryCount"`
		SourceTotalBytes          int64 `json:"sourceTotalBytes"`
		StageTotalBytes           int64 `json:"stageTotalBytes"`
	} `json:"equality"`
	Error string `json:"error"`
}
type localAICandidateObserverReport struct {
	SchemaVersion   string                                   `json:"schemaVersion"`
	Status          string                                   `json:"status"`
	Cycle           string                                   `json:"cycle"`
	ReleaseStatus   string                                   `json:"releaseStatus"`
	Observation     localAICandidateObserverEvidence         `json:"observation"`
	Cleanup         localAICandidateObserverCleanup          `json:"cleanup"`
	ReleaseCheckout *localAICandidateReleaseCheckoutEvidence `json:"releaseCheckout"`
}
type localAICandidateNativeToolLaunch struct {
	Status   string `json:"status"`
	Path     string `json:"path"`
	ExitCode *int64 `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error"`
}
type localAICandidateNativeToolFile struct {
	Path       string                           `json:"path"`
	PathLength int                              `json:"pathLength"`
	Bytes      int64                            `json:"bytes"`
	SHA256     string                           `json:"sha256"`
	Launch     localAICandidateNativeToolLaunch `json:"launch"`
}
type localAICandidateNativeToolReport struct {
	SchemaVersion string `json:"schemaVersion"`
	Status        string `json:"status"`
	Cycle         string `json:"cycle"`
	Property      string `json:"property"`
	ShortRoot     struct {
		Status     string `json:"status"`
		Path       string `json:"path"`
		Parent     string `json:"parent"`
		PathLength int    `json:"pathLength"`
		Present    bool   `json:"present"`
		Empty      bool   `json:"empty"`
	} `json:"shortRoot"`
	LongPath  localAICandidateNativeToolFile  `json:"longPath"`
	ShortPath localAICandidateNativeToolFile  `json:"shortPath"`
	Cleanup   localAICandidateObserverCleanup `json:"cleanup"`
}

func localAICandidateManifestPath(t *testing.T, purpose string) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Windows " + purpose + " is only available on Windows")
	}
	manifestPath := strings.TrimSpace(os.Getenv(localAICandidateManifestEnv))
	if manifestPath == "" {
		t.Skip("set " + localAICandidateManifestEnv + " to run the " + purpose)
	}
	if !filepath.IsAbs(manifestPath) {
		t.Fatalf("candidate manifest path %q is not absolute; %s must name an absolute path", manifestPath, localAICandidateManifestEnv)
	}
	return manifestPath
}
func TestLocalAICandidate_ValidatesOptInPrebuiltWindowsCandidate(t *testing.T) {
	manifestPath := localAICandidateManifestPath(t, "candidate release cell")
	evidence, err := validateLocalAICandidate(manifestPath)
	if err != nil {
		t.Logf("LOCALAI-CANDIDATE status=FAIL property=prebuilt-candidate-schema-source-target-artifact-and-detached-digest-identity reason=%q", err)
		t.Fatalf("validate prebuilt Windows candidate: %v", err)
	}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		t.Fatalf("marshal candidate evidence: %v", err)
	}
	t.Logf("LOCALAI-CANDIDATE status=PASS property=%s evidence=%s", evidence.Property, encoded)
}
func TestLocalAICandidate_PublicInstallDiscoveryAndCleanup(t *testing.T) {
	manifestPath := localAICandidateManifestPath(t, "candidate release smoke")
	if _, err := validateLocalAICandidate(manifestPath); err != nil {
		t.Fatalf("validate candidate before public install smoke: %v", err)
	}
	installDir := filepath.Join(t.TempDir(), "installed-bin")
	reportPath := filepath.Join(t.TempDir(), "install-validation-report.json")
	output, err := localAICandidateSmokeCommand(t, manifestPath, installDir, reportPath).CombinedOutput()
	if err != nil {
		t.Fatalf("public Windows candidate install smoke: %v\n%s", err, output)
	}
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read candidate install smoke report: %v\n%s", err, output)
	}
	var report map[string]any
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatalf("decode candidate install smoke report: %v\n%s", err, reportBytes)
	}
	run, _ := report["run"].(map[string]any)
	cleanup, _ := report["cleanup"].(map[string]any)
	postCleanup, _ := report["postCleanup"].(map[string]any)
	installer, _ := report["installer"].(map[string]any)
	requests, _ := installer["requests"].([]any)
	if report["status"] != "PASS" || report["property"] != "public-install-identity-discovery-zero-model-backend-activity-cleanup" ||
		run["status"] != "PASS" || cleanup["status"] != "PASS" || len(requests) != 3 ||
		postCleanup["candidateHashesStable"] != true {
		t.Fatalf("candidate install smoke report = %#v, want PASS with complete run/install/cleanup evidence", report)
	}
	reportBytes = regexp.MustCompile(`\s+`).ReplaceAll(reportBytes, nil)
	for _, want := range []string{
		`"powershell"`, `"commandMilliseconds"`, `"loopbackServerReadyMilliseconds"`,
		`"loopbackCompletionMilliseconds"`, `"networkObserverSampleMilliseconds"`, `"observer"`,
		`"networkSamples"`, `"unexpectedConnections": 0`, `"port7437Connections": 0`,
		`"modelBackendDownloadBytes": 0`, `"modelCalls": 0`, `"backendRequestsObserved": 0`,
		`"remainingTaskPaths": []`,
	} {
		if !bytes.Contains(reportBytes, []byte(strings.ReplaceAll(want, " ", ""))) {
			t.Fatalf("candidate install report missing %q: %s", want, reportBytes)
		}
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("install directory stat error = %v, want task-owned install directory removed", err)
	}
}
func TestLocalAICandidate_PublicInstallPreconditionsRemainFailClosed(t *testing.T) {
	manifestPath := localAICandidateManifestPath(t, "candidate release smoke")
	installDir := filepath.Join(t.TempDir(), "installed-bin")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatalf("create nonempty install directory: %v", err)
	}
	markerPath := filepath.Join(installDir, "ambient-marker.txt")
	if err := os.WriteFile(markerPath, []byte("must remain untouched"), 0o600); err != nil {
		t.Fatalf("write install precondition marker: %v", err)
	}
	reportPath := filepath.Join(t.TempDir(), "install-validation-report.json")
	output, err := localAICandidateSmokeCommand(t, manifestPath, installDir, reportPath).CombinedOutput()
	if err == nil {
		t.Fatalf("nonempty install precondition unexpectedly passed\n%s", output)
	}
	if !strings.Contains(string(output), "declared root 'install' is not empty") {
		t.Fatalf("precondition output = %q, want fail-closed install-root evidence", output)
	}
	if _, statErr := os.Stat(markerPath); statErr != nil {
		t.Fatalf("precondition marker stat error = %v, want marker preserved", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(installDir, "you.exe")); !os.IsNotExist(statErr) {
		t.Fatalf("installed binary stat error = %v, want no installation after precondition failure", statErr)
	}
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read failed candidate install smoke report: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatalf("decode failed candidate install smoke report: %v", err)
	}
	cleanup, _ := report["cleanup"].(map[string]any)
	remaining, _ := cleanup["remainingTaskPaths"].([]any)
	if report["status"] != "FAIL" || cleanup["status"] != "PASS" || len(remaining) != 0 {
		t.Fatalf("failed candidate install smoke report = %#v, want failed operation with clean task roots", report)
	}
}
func localAICandidateSmokeCommand(t *testing.T, manifestPath, installDir, reportPath string) *exec.Cmd {
	t.Helper()
	pwsh, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatalf("candidate release smoke requires Windows PowerShell (powershell.exe): %v", err)
	}
	root := testutil.MustRepoRoot(t)
	command := exec.Command(pwsh, "-NoProfile", "-File", filepath.Join(root, "scripts", "release", "smoke-install.ps1"), "-CandidateManifestPath", manifestPath, "-InstallDir", installDir, "-ReportPath", reportPath)
	command.Dir = root
	return command
}
func localAICandidatePowerShellCommand(t *testing.T, arguments ...string) *exec.Cmd {
	t.Helper()
	pwsh, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatalf("candidate release smoke requires Windows PowerShell (powershell.exe): %v", err)
	}
	root := testutil.MustRepoRoot(t)
	commandArguments := append([]string{"-NoProfile", "-NonInteractive", "-File", filepath.Join(root, "scripts", "release", "smoke-install.ps1")}, arguments...)
	command := exec.Command(pwsh, commandArguments...)
	command.Dir = root
	return command
}
func localAICandidatePowerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
func utf16LE(value string) []byte {
	encoded := utf16.Encode([]rune(value))
	result := make([]byte, len(encoded)*2)
	for index, character := range encoded {
		binary.LittleEndian.PutUint16(result[index*2:], character)
	}
	return result
}
func localAICandidateRunGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	commandArguments := append([]string{"-C", directory}, arguments...)
	output, err := exec.Command("git", commandArguments...).CombinedOutput()
	if err != nil {
		t.Fatalf("git -C %s %s: %v\n%s", directory, strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
func TestLocalAICandidateManifestValidationRejectsDriftAndAmbiguity(t *testing.T) {
	tests := []struct {
		name string
		edit func(*localAICandidateFixture)
		want string
	}{
		{
			name: "archive hash drift",
			edit: func(fixture *localAICandidateFixture) {
				fixture.manifest.Artifacts[0].SHA256 = strings.Repeat("d", sha256.Size*2)
				fixture.writeManifest(t)
			},
			want: "sha256 mismatch",
		},
		{
			name: "installer hash drift",
			edit: func(fixture *localAICandidateFixture) {
				fixture.manifest.Artifacts[1].SHA256 = strings.Repeat("d", sha256.Size*2)
				fixture.writeManifest(t)
			},
			want: "sha256 mismatch",
		},
		{
			name: "ambiguous archive",
			edit: func(fixture *localAICandidateFixture) {
				second := filepath.Join(fixture.root, "you_9.9.9_windows_amd64.zip")
				if err := os.WriteFile(second, fixture.archiveBytes, 0o600); err != nil {
					t.Fatalf("write ambiguous archive: %v", err)
				}
			},
			want: "exactly one windows-amd64 ZIP",
		},
		{
			name: "old descendant ceiling",
			edit: func(fixture *localAICandidateFixture) {
				fixture.manifest.Limits.DescendantMaximum = candidateInt64Pointer(12)
				fixture.writeManifest(t)
			},
			want: "candidate limits.descendantMaximum = 12, want 36",
		},
		{
			name: "ordinary download allowance",
			edit: func(fixture *localAICandidateFixture) {
				fixture.manifest.Limits.OrdinaryToolDownloadMaximum = candidateInt64Pointer(536870912)
				fixture.writeManifest(t)
			},
			want: "candidate limits.ordinaryToolDownloadBytesMaximum = 536870912, want 0",
		},
		{name: "changed source commit", edit: func(fixture *localAICandidateFixture) {
			fixture.manifest.Source.Commit = strings.Repeat("d", 40)
			fixture.writeManifest(t)
		}, want: "candidate source identity"},
		{name: "changed source tree", edit: func(fixture *localAICandidateFixture) {
			fixture.manifest.Source.Tree = strings.Repeat("e", 40)
			fixture.writeManifest(t)
		}, want: "candidate source identity"},
		{name: "changed Go controls", edit: func(fixture *localAICandidateFixture) {
			fixture.manifest.Limits.GOFLAGS = "-p=8"
			fixture.manifest.Limits.GOMAXPROCS = "8"
			fixture.writeManifest(t)
		}, want: "candidate Go controls"},
		{name: "changed Go version", edit: func(fixture *localAICandidateFixture) {
			fixture.manifest.Build.GoVersion = "go1.25.0"
			fixture.writeManifest(t)
		}, want: "candidate Go version"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newLocalAICandidateFixture(t)
			test.edit(fixture)
			if _, err := validateLocalAICandidate(fixture.manifestPath); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate candidate error = %v, want substring %q", err, test.want)
			}
		})
	}
	for _, cycle := range []string{"030", "040", "042", "045", "048", "050", "063", "067", "068", "070", "074"} {
		cycle := cycle
		t.Run("historical cycle "+cycle, func(t *testing.T) {
			fixture := newLocalAICandidateFixture(t)
			fixture.manifest.Cycle = cycle
			fixture.writeManifest(t)
			if _, err := validateLocalAICandidate(fixture.manifestPath); err == nil || !strings.Contains(err.Error(), `candidate cycle = "`+cycle+`", want "`+localAICandidateCycle+`"`) {
				t.Fatalf("validate historical cycle error = %v, want cycle %s rejection", err, cycle)
			}
		})
	}
}
func TestLocalAICandidateManifestValidationRejectsReparseCandidateDirectory(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows junction fixture is only available on Windows")
	}
	fixture := newLocalAICandidateFixture(t)
	linkRoot := filepath.Join(t.TempDir(), "candidate-link")
	command := exec.Command("cmd.exe", "/c", "mklink", "/J", linkRoot, fixture.root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Skipf("junction/reparse fixture unavailable: %v (%s)", err, output)
	}
	t.Cleanup(func() { _ = os.Remove(linkRoot) })
	linkedManifest := filepath.Join(linkRoot, filepath.Base(fixture.manifestPath))
	if _, err := validateLocalAICandidate(linkedManifest); err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink/reparse") {
		t.Fatalf("validate reparse candidate directory error = %v, want symlink/reparse rejection", err)
	}
	externalArchive := filepath.Join(t.TempDir(), fixture.manifest.Artifacts[0].File)
	if err := os.WriteFile(externalArchive, fixture.archiveBytes, 0o600); err != nil {
		t.Fatalf("write external archive: %v", err)
	}
	retainedArchive := filepath.Join(fixture.root, fixture.manifest.Artifacts[0].File)
	if err := os.Remove(retainedArchive); err != nil {
		t.Fatalf("remove retained archive: %v", err)
	}
	if err := os.Symlink(externalArchive, retainedArchive); err != nil {
		t.Logf("file symlink fixture unavailable: %v", err)
		return
	}
	if _, err := validateLocalAICandidate(fixture.manifestPath); err == nil || !strings.Contains(strings.ToLower(err.Error()), "symlink/reparse") {
		t.Fatalf("validate reparse artifact error = %v, want symlink/reparse rejection", err)
	}
}
func TestLocalAICandidateManifestValidationAcceptsMatchingArtifacts(t *testing.T) {
	fixture := newLocalAICandidateFixture(t)
	evidence, err := validateLocalAICandidateWithBuildInfoReader(fixture.manifestPath, func(string) (*debug.BuildInfo, error) {
		return localAICandidateCleanBuildInfo(), nil
	})
	if err != nil {
		t.Fatalf("validate matching candidate: %v", err)
	}
	if evidence.Status != "PASS" || evidence.Property == "" || evidence.SourceCommit != localAICandidateSourceCommit || evidence.SourceTree != localAICandidateSourceTree || evidence.EmbeddedSourceRevision != localAICandidateSourceCommit || evidence.EmbeddedVCSModified || evidence.Archive.File != "you_1.2.3-snapshot-test_windows_amd64.zip" || evidence.Archive.Bytes != int64(len(fixture.archiveBytes)) || evidence.Installer.File != "install.ps1" || evidence.Installer.Bytes <= 0 {
		t.Fatalf("candidate evidence = %#v, want PASS with matching archive, executable, and clean embedded revision", evidence)
	}
}
func TestLocalAICandidateBuildInfoValidationRejectsDriftAndAmbiguity(t *testing.T) {
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		readErr  error
		want     string
	}{
		{
			name: "clean revision and VCS state",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: localAICandidateSourceCommit},
				{Key: "vcs.modified", Value: "false"},
			},
		},
		{
			name:     "missing revision",
			settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "false"}},
			want:     "vcs.revision is missing",
		},
		{
			name: "mismatched revision",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: strings.Repeat("d", 40)},
				{Key: "vcs.modified", Value: "false"},
			},
			want: "vcs.revision =",
		},
		{
			name:     "missing modified state",
			settings: []debug.BuildSetting{{Key: "vcs.revision", Value: localAICandidateSourceCommit}},
			want:     "vcs.modified is missing",
		},
		{
			name: "dirty modified state",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: localAICandidateSourceCommit},
				{Key: "vcs.modified", Value: "true"},
			},
			want: "vcs.modified =",
		},
		{
			name: "duplicate revision settings",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: localAICandidateSourceCommit},
				{Key: "vcs.revision", Value: localAICandidateSourceCommit},
				{Key: "vcs.modified", Value: "false"},
			},
			want: "duplicate \"vcs.revision\" settings",
		},
		{
			name: "duplicate modified settings",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: localAICandidateSourceCommit},
				{Key: "vcs.modified", Value: "false"},
				{Key: "vcs.modified", Value: "true"},
			},
			want: "duplicate \"vcs.modified\" settings",
		},
		{
			name:    "unreadable executable metadata",
			readErr: errors.New("not a Go executable"),
			want:    "read executable build info",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			build := &debug.BuildInfo{Settings: test.settings}
			revision, modified, err := readAndValidateLocalAICandidateBuildInfo("candidate.exe", func(string) (*debug.BuildInfo, error) {
				return build, test.readErr
			})
			if test.want == "" {
				if err != nil || revision != localAICandidateSourceCommit || modified {
					t.Fatalf("build-info validation = revision=%q modified=%v error=%v, want clean protected revision", revision, modified, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("build-info validation error = %v, want substring %q", err, test.want)
			}
		})
	}
}
func TestLocalAICandidateBuildInfoReadFailureFailsClosed(t *testing.T) {
	fixture := newLocalAICandidateFixture(t)
	if _, err := validateLocalAICandidate(fixture.manifestPath); err == nil || !strings.Contains(err.Error(), "read executable build info") {
		t.Fatalf("candidate build-info read error = %v, want fail-closed metadata evidence", err)
	}
}
func TestLocalAICandidatePreconditionsFailClosedBeforeInstallation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*localAICandidateFixture)
		want string
	}{
		{
			name: "ambient you on task PATH",
			edit: func(fixture *localAICandidateFixture) {
				ambientDir := filepath.Join(fixture.root, "ambient-bin")
				if err := os.MkdirAll(ambientDir, 0o700); err != nil {
					t.Fatalf("create ambient bin: %v", err)
				}
				if err := os.WriteFile(filepath.Join(ambientDir, "you.exe"), []byte("ambient"), 0o600); err != nil {
					t.Fatalf("write ambient you: %v", err)
				}
				fixture.taskPath = ambientDir
			},
			want: "task PATH already resolves you",
		},
		{
			name: "nonempty declared state",
			edit: func(fixture *localAICandidateFixture) {
				stateDir := fixture.roots[0].Path
				if err := os.MkdirAll(stateDir, 0o700); err != nil {
					t.Fatalf("create state root: %v", err)
				}
				if err := os.WriteFile(filepath.Join(stateDir, "existing.json"), []byte("ambient state"), 0o600); err != nil {
					t.Fatalf("write state: %v", err)
				}
			},
			want: "declared root \"HOME\" is not empty",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newLocalAICandidateFixture(t)
			test.edit(fixture)
			if err := validateLocalAICandidatePreconditions(fixture.taskPath, fixture.roots); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("precondition error = %v, want substring %q", err, test.want)
			}
		})
	}
	fixture := newLocalAICandidateFixture(t)
	if err := validateLocalAICandidatePreconditions(fixture.taskPath, fixture.roots); err != nil {
		t.Fatalf("validate absent roots: %v", err)
	}
	if err := os.Mkdir(fixture.roots[0].Path, 0o700); err != nil {
		t.Fatalf("create empty root: %v", err)
	}
	if err := validateLocalAICandidatePreconditions(fixture.taskPath, fixture.roots); err != nil {
		t.Fatalf("validate empty roots: %v", err)
	}
}
func TestLocalAICandidateShortRootValidation(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows short-root validation is only available on Windows")
	}
	parent := t.TempDir()
	run := func(name, rootPath, wantError string) {
		t.Run(name, func(t *testing.T) {
			reportPath := filepath.Join(t.TempDir(), "short-root-report.json")
			command := localAICandidatePowerShellCommand(t,
				"-InstallDir", t.TempDir(),
				"-ValidateShortRoot",
				"-ShortRootParent", parent,
				"-ShortRootPath", rootPath,
				"-ShortRootReportPath", reportPath,
				"-ShortRootMaximumPathLength", "220",
			)
			output, err := command.CombinedOutput()
			raw, readErr := os.ReadFile(reportPath)
			if readErr != nil {
				t.Fatalf("read short-root report: %v\n%s", readErr, output)
			}
			var report struct {
				SchemaVersion string `json:"schemaVersion"`
				Status        string `json:"status"`
				Cycle         string `json:"cycle"`
				Property      string `json:"property"`
				Root          struct {
					Status       string `json:"status"`
					Path         string `json:"path"`
					Parent       string `json:"parent"`
					RelativePath string `json:"relativePath"`
					PathLength   int    `json:"pathLength"`
					Present      bool   `json:"present"`
					Empty        bool   `json:"empty"`
				} `json:"root"`
				Cleanup localAICandidateObserverCleanup `json:"cleanup"`
				Error   string                          `json:"error"`
			}
			if decodeErr := decodeLocalAICandidateJSON(raw, &report); decodeErr != nil {
				t.Fatalf("decode short-root report: %v", decodeErr)
			}
			if wantError == "" {
				if err != nil || report.Status != "PASS" || report.Cycle != localAICandidateCycle || report.Root.Status != "PASS" || report.Root.PathLength > 220 || !report.Root.Empty {
					t.Fatalf("short-root success = err=%v report=%#v output=%s, want PASS contained empty root", err, report, output)
				}
				return
			}
			if err == nil || report.Status != "FAIL" || !strings.Contains(report.Error, wantError) {
				t.Fatalf("short-root failure = err=%v report=%#v output=%s, want %q", err, report, output, wantError)
			}
		})
	}

	run("absent root", filepath.Join(parent, "localai-c076-absent"), "")
	emptyRoot := filepath.Join(parent, "localai-c076-empty")
	if err := os.Mkdir(emptyRoot, 0o700); err != nil {
		t.Fatalf("create empty short root: %v", err)
	}
	run("empty root", emptyRoot, "")
	nonemptyRoot := filepath.Join(parent, "localai-c076-nonempty")
	if err := os.Mkdir(nonemptyRoot, 0o700); err != nil {
		t.Fatalf("create nonempty short root: %v", err)
	}
	markerPath := filepath.Join(nonemptyRoot, "ambient-marker.txt")
	if err := os.WriteFile(markerPath, []byte("must remain"), 0o600); err != nil {
		t.Fatalf("write short-root marker: %v", err)
	}
	run("nonempty collision", nonemptyRoot, "not empty")
	longRoot := filepath.Join(parent, strings.Repeat("r", 230))
	run("path length", longRoot, "path length")
	run("outside parent", filepath.Join(t.TempDir(), "outside"), "outside the contained root")

	reparseTarget := filepath.Join(t.TempDir(), "reparse-target")
	if err := os.Mkdir(reparseTarget, 0o700); err != nil {
		t.Fatalf("create reparse target: %v", err)
	}
	reparseRoot := filepath.Join(parent, "localai-c076-reparse")
	linkCommand := exec.Command("cmd.exe", "/c", "mklink", "/J", reparseRoot, reparseTarget)
	if output, err := linkCommand.CombinedOutput(); err != nil {
		t.Skipf("short-root reparse fixture unavailable: %v (%s)", err, output)
	}
	t.Cleanup(func() { _ = os.Remove(reparseRoot) })
	run("reparse collision", reparseRoot, "reparse point")
}
func TestLocalAICandidateNativeToolPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows native-tool readiness is only available on Windows")
	}
	longPath := strings.TrimSpace(os.Getenv("INFINITE_YOU_LOCALAI_NATIVE_LONG_PATH"))
	shortPath := strings.TrimSpace(os.Getenv("INFINITE_YOU_LOCALAI_NATIVE_SHORT_PATH"))
	shortRoot := strings.TrimSpace(os.Getenv("INFINITE_YOU_LOCALAI_NATIVE_SHORT_ROOT"))
	shortParent := strings.TrimSpace(os.Getenv("INFINITE_YOU_LOCALAI_NATIVE_SHORT_PARENT"))
	if longPath == "" || shortPath == "" || shortRoot == "" || shortParent == "" {
		t.Skip("set INFINITE_YOU_LOCALAI_NATIVE_LONG_PATH, INFINITE_YOU_LOCALAI_NATIVE_SHORT_PATH, INFINITE_YOU_LOCALAI_NATIVE_SHORT_ROOT, and INFINITE_YOU_LOCALAI_NATIVE_SHORT_PARENT for the local-real native-tool witness")
	}
	reportPath := strings.TrimSpace(os.Getenv("INFINITE_YOU_LOCALAI_NATIVE_REPORT_PATH"))
	if reportPath == "" {
		reportPath = filepath.Join(t.TempDir(), "native-tool-report.json")
	}
	command := localAICandidatePowerShellCommand(t,
		"-InstallDir", t.TempDir(),
		"-NativeToolFixture",
		"-NativeToolLongPath", longPath,
		"-NativeToolShortPath", shortPath,
		"-NativeToolRoot", shortRoot,
		"-NativeToolParent", shortParent,
		"-NativeToolReportPath", reportPath,
		"-NativeToolTimeoutMilliseconds", "10000",
	)
	output, err := command.CombinedOutput()
	raw, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatalf("read native-tool report: %v\n%s", readErr, output)
	}
	var report localAICandidateNativeToolReport
	if decodeErr := decodeLocalAICandidateJSON(raw, &report); decodeErr != nil {
		t.Fatalf("decode native-tool report: %v", decodeErr)
	}
	if err != nil || report.SchemaVersion != "localai-windows-install-candidate-native-tool/v1" || report.Status != "PASS" || report.Cycle != localAICandidateCycle || report.Property == "" || report.LongPath.PathLength != 280 || report.LongPath.Bytes != localAICandidateNativeToolBytes || report.LongPath.SHA256 != localAICandidateNativeToolSHA256 || report.LongPath.Launch.Status == "PASS" || report.ShortRoot.PathLength > 220 || report.ShortPath.PathLength > 220 || report.ShortPath.Bytes != localAICandidateNativeToolBytes || report.ShortPath.SHA256 != localAICandidateNativeToolSHA256 || report.ShortPath.Launch.Status != "PASS" || report.ShortPath.Launch.ExitCode == nil || *report.ShortPath.Launch.ExitCode != 0 || strings.TrimSpace(report.ShortPath.Launch.Stdout) != localAICandidateNativeToolVersion || report.Cleanup.Status != "PASS" {
		t.Fatalf("native-tool witness = err=%v report=%#v output=%s, want exact long failure and short 0.27.7/exit0", err, report, output)
	}
	t.Logf("LOCALAI-NATIVE status=PASS evidence=%s", raw)
}
func TestLocalAICandidateObserverEnvelopeFixture(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows observer fixture is only available on Windows")
	}
	pwsh, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatalf("observer fixture requires Windows PowerShell: %v", err)
	}
	reportPath := filepath.Join(t.TempDir(), "observer-report.json")
	command := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", filepath.Join(testutil.MustRepoRoot(t), "scripts", "release", "smoke-install.ps1"), "-InstallDir", t.TempDir(), "-ObserverFixture", "-ObserverReportPath", reportPath)
	command.Env = append(os.Environ(), "GOFLAGS="+localAICandidateGOFLAGS, "GOMAXPROCS="+localAICandidateGOMAXPROCS)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("observer fixture: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read observer report: %v", err)
	}
	var report localAICandidateObserverReport
	if err := decodeLocalAICandidateJSON(raw, &report); err != nil {
		t.Fatalf("decode observer report: %v", err)
	}
	if report.SchemaVersion != "localai-windows-install-candidate-observer/v1" || report.Status != "PASS" || report.Cycle != localAICandidateCycle || report.ReleaseStatus != "observer-fixture" {
		t.Fatalf("observer report identity/status = %#v, want cycle-%s PASS observer-fixture", report, localAICandidateCycle)
	}
	if err := validateLocalAICandidateObserverEvidence(report.Observation); err != nil {
		t.Fatalf("observer report evidence: %v", err)
	}
	if report.Cleanup.Status != "PASS" || len(report.Cleanup.RemainingTaskPaths) != 0 || len(report.Cleanup.Errors) != 0 {
		t.Fatalf("observer cleanup evidence = %#v, want complete cleanup", report.Cleanup)
	}
	for _, stream := range []localAICandidateRootOutputStream{report.Observation.RootOutput.Stdout, report.Observation.RootOutput.Stderr} {
		contents, err := os.ReadFile(stream.Path)
		if err != nil {
			t.Fatalf("read retained root output %q: %v", stream.Path, err)
		}
		if strings.Contains(string(contents), "observer-secret") || !strings.Contains(string(contents), "<redacted>") {
			t.Fatalf("retained root output %q was not redacted: %q", stream.Path, contents)
		}
	}
	t.Logf("LOCALAI-OBSERVER status=PASS evidence=%s", raw)
}
func TestLocalAICandidateObserverRootFailureRetainsOutput(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows observer fixture is only available on Windows")
	}
	reportPath := filepath.Join(t.TempDir(), "observer-failure-report.json")
	command := localAICandidatePowerShellCommand(t, "-InstallDir", t.TempDir(), "-ObserverFixture", "-ObserverRootExitCode", "7", "-ObserverReportPath", reportPath)
	command.Env = append(os.Environ(), "GOFLAGS="+localAICandidateGOFLAGS, "GOMAXPROCS="+localAICandidateGOMAXPROCS)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("observer failure fixture unexpectedly succeeded: %s", output)
	}
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read observer failure report: %v\n%s", err, output)
	}
	var report localAICandidateObserverReport
	if err := decodeLocalAICandidateJSON(raw, &report); err != nil {
		t.Fatalf("decode observer failure report: %v", err)
	}
	if report.Status != "FAIL" || report.Observation.RootExitCode == nil || *report.Observation.RootExitCode != 7 || report.Observation.FailurePropagation != "FAIL" {
		t.Fatalf("observer failure evidence = %#v, want exit 7 and failure propagation", report.Observation)
	}
	if report.Observation.RootOutput.Status != "PASS" || report.Cleanup.Status != "PASS" {
		t.Fatalf("observer failure output/cleanup evidence = %#v / %#v, want retained output and cleanup", report.Observation.RootOutput, report.Cleanup)
	}
	for _, stream := range []localAICandidateRootOutputStream{report.Observation.RootOutput.Stdout, report.Observation.RootOutput.Stderr} {
		contents, err := os.ReadFile(stream.Path)
		if err != nil {
			t.Fatalf("read retained failed-root output %q: %v", stream.Path, err)
		}
		if strings.Contains(string(contents), "observer-secret") || !strings.Contains(string(contents), "<redacted>") {
			t.Fatalf("failed-root output %q was not redacted: %q", stream.Path, contents)
		}
	}
	t.Logf("LOCALAI-OBSERVER-FAILURE status=PASS evidence=%s", raw)
}
func TestLocalAICandidateObserverArgumentVector(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows argument-vector fixture is only available on Windows")
	}
	repo := testutil.MustRepoRoot(t)
	shell, err := exec.LookPath("pwsh.exe")
	if err != nil {
		shell, err = exec.LookPath("powershell.exe")
		if err != nil {
			t.Fatalf("argument-vector fixture requires PowerShell: %v", err)
		}
	}
	recorderPath := filepath.Join(t.TempDir(), "argv-recorder.ps1")
	recorder := `$observed = [string[]]$args
Write-Output (ConvertTo-Json -InputObject $observed -Compress)
[Console]::Error.WriteLine("argv-recorder stderr")
$sleepScript = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes("Start-Sleep -Milliseconds 1500"))
$childPath = Join-Path $PSHOME "pwsh.exe"
if (-not (Test-Path -LiteralPath $childPath)) { $childPath = Join-Path $PSHOME "powershell.exe" }
$child = Start-Process -FilePath $childPath -WindowStyle Hidden -ArgumentList @("-NoProfile", "-NonInteractive", "-EncodedCommand", $sleepScript) -PassThru
Wait-Process -Id $child.Id
exit 0
`
	if err := os.WriteFile(recorderPath, []byte(recorder), 0o600); err != nil {
		t.Fatalf("write argv recorder: %v", err)
	}
	spacedArgument := filepath.Join(t.TempDir(), "config path with spaces", ".goreleaser.yml")
	wantArguments := []string{"release", "--snapshot", "--clean", "-f", spacedArgument}
	collapsedArguments := "'" + strings.Join([]string{"release", "--snapshot", "--clean", "-f", spacedArgument}, "','") + "'"
	readArguments := func(t *testing.T, reportPath string) []string {
		t.Helper()
		raw, err := os.ReadFile(reportPath)
		if err != nil {
			t.Fatalf("read argument observer report: %v", err)
		}
		var report localAICandidateObserverReport
		if err := decodeLocalAICandidateJSON(raw, &report); err != nil {
			t.Fatalf("decode argument observer report: %v", err)
		}
		if report.Status != "PASS" || report.Observation.RootExitCode == nil || *report.Observation.RootExitCode != 0 {
			t.Fatalf("argument observer report = %#v, want successful observed root", report)
		}
		if err := validateLocalAICandidateObserverEvidence(report.Observation); err != nil {
			t.Fatalf("argument observer evidence: %v", err)
		}
		stdout, err := os.ReadFile(report.Observation.RootOutput.Stdout.Path)
		if err != nil {
			t.Fatalf("read argument recorder stdout: %v", err)
		}
		stderr, err := os.ReadFile(report.Observation.RootOutput.Stderr.Path)
		if err != nil {
			t.Fatalf("read argument recorder stderr: %v", err)
		}
		if !strings.Contains(string(stderr), "argv-recorder stderr") {
			t.Fatalf("argument recorder stderr = %q, want retained stderr", stderr)
		}
		var observed []string
		if err := json.Unmarshal([]byte(strings.TrimSpace(string(stdout))), &observed); err != nil {
			t.Fatalf("decode recorded argv %q: %v", stdout, err)
		}
		return observed
	}
	newInstallDir := filepath.Join(t.TempDir(), "install-root")
	if err := os.MkdirAll(newInstallDir, 0o700); err != nil {
		t.Fatalf("create direct-array install root: %v", err)
	}
	newReportPath := filepath.Join(t.TempDir(), "direct-array-report.json")
	newStdoutPath := filepath.Join(filepath.Dir(newReportPath), "direct-array.stdout.txt")
	newStderrPath := filepath.Join(filepath.Dir(newReportPath), "direct-array.stderr.txt")
	argumentLiterals := make([]string, 0, len(wantArguments)+4)
	for _, argument := range append([]string{"-NoProfile", "-NonInteractive", "-File", recorderPath}, wantArguments...) {
		argumentLiterals = append(argumentLiterals, localAICandidatePowerShellLiteral(argument))
	}
	invocation := fmt.Sprintf(`$parameters = @{
    InstallDir = %s
    ObserverFixture = $true
    ObserverReportPath = %s
    ObserverRootCommand = %s
    ObserverRootArgumentList = @(%s)
    ObserverRootWorkingDirectory = %s
    ObserverRootStdoutPath = %s
    ObserverRootStderrPath = %s
    ObserverRootOutputMaximumBytes = 65536
    ObserverTimeoutSeconds = 10
}
& %s @parameters`, localAICandidatePowerShellLiteral(newInstallDir), localAICandidatePowerShellLiteral(newReportPath), localAICandidatePowerShellLiteral(shell), strings.Join(argumentLiterals, ", "), localAICandidatePowerShellLiteral(repo), localAICandidatePowerShellLiteral(newStdoutPath), localAICandidatePowerShellLiteral(newStderrPath), localAICandidatePowerShellLiteral(filepath.Join(repo, "scripts", "release", "smoke-install.ps1")))
	command := exec.Command(shell, "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(utf16LE(invocation)))
	command.Dir = repo
	command.Env = append(os.Environ(), "GOFLAGS="+localAICandidateGOFLAGS, "GOMAXPROCS="+localAICandidateGOMAXPROCS)
	if output, err := command.CombinedOutput(); err != nil {
		report, readErr := os.ReadFile(newReportPath)
		t.Fatalf("direct argument-vector observer: %v\n%s\nreport=%s readError=%v", err, output, report, readErr)
	}
	observed := readArguments(t, newReportPath)
	if !slices.Equal(observed, wantArguments) {
		t.Fatalf("direct argument-vector recorder = %#v, want %#v", observed, wantArguments)
	}
	t.Logf("LOCALAI-ARGV status=PASS requested=%#v observed=%#v", wantArguments, observed)

	collapsedInstallDir := filepath.Join(t.TempDir(), "install-root")
	if err := os.MkdirAll(collapsedInstallDir, 0o700); err != nil {
		t.Fatalf("create collapsed-array install root: %v", err)
	}
	collapsedReportPath := filepath.Join(t.TempDir(), "collapsed-array-report.json")
	collapsedStdoutPath := filepath.Join(filepath.Dir(collapsedReportPath), "collapsed-array.stdout.txt")
	collapsedStderrPath := filepath.Join(filepath.Dir(collapsedReportPath), "collapsed-array.stderr.txt")
	collapsedCommand := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", filepath.Join(repo, "scripts", "release", "smoke-install.ps1"), "-InstallDir", collapsedInstallDir, "-ObserverFixture", "-ObserverReportPath", collapsedReportPath, "-ObserverRootCommand", shell, "-ObserverRootWorkingDirectory", repo, "-ObserverRootStdoutPath", collapsedStdoutPath, "-ObserverRootStderrPath", collapsedStderrPath, "-ObserverRootOutputMaximumBytes", "65536", "-ObserverTimeoutSeconds", "10", "-ObserverRootArgumentList", collapsedArguments)
	collapsedCommand.Dir = repo
	collapsedCommand.Env = append(os.Environ(), "GOFLAGS="+localAICandidateGOFLAGS, "GOMAXPROCS="+localAICandidateGOMAXPROCS)
	if output, err := collapsedCommand.CombinedOutput(); err == nil {
		t.Fatalf("retained collapsed argument-vector observer unexpectedly succeeded: %s", output)
	}
	collapsedRaw, err := os.ReadFile(collapsedReportPath)
	if err != nil {
		t.Fatalf("read collapsed argument observer report: %v", err)
	}
	var collapsedReport localAICandidateObserverReport
	if err := decodeLocalAICandidateJSON(collapsedRaw, &collapsedReport); err != nil {
		t.Fatalf("decode collapsed argument observer report: %v", err)
	}
	if collapsedReport.Status != "FAIL" || collapsedReport.Observation.RootExitCode == nil || *collapsedReport.Observation.RootExitCode == 0 || collapsedReport.Observation.RootOutput.Status != "PASS" {
		t.Fatalf("collapsed argument observer report = %#v, want retained failure with root output", collapsedReport)
	}
	t.Logf("LOCALAI-ARGV-REGRESSION status=PASS retainedCollapsed=%q rootExit=%d", collapsedArguments, *collapsedReport.Observation.RootExitCode)
}
func TestLocalAICandidateObserverIdentityFixture(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows observer identity fixture is only available on Windows")
	}
	reportPath := filepath.Join(t.TempDir(), "observer-identity-report.json")
	command := localAICandidatePowerShellCommand(t, "-InstallDir", t.TempDir(), "-ObserverIdentityFixture", "-ObserverReportPath", reportPath)
	command.Env = append(os.Environ(), "GOFLAGS="+localAICandidateGOFLAGS, "GOMAXPROCS="+localAICandidateGOMAXPROCS)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("observer identity fixture: %v\n%s", err, output)
	}
	var report struct {
		SchemaVersion string `json:"schemaVersion"`
		Status        string `json:"status"`
		Cycle         string `json:"cycle"`
		ReleaseStatus string `json:"releaseStatus"`
		CrossInterval struct {
			Status               string   `json:"status"`
			ActiveIdentity       string   `json:"activeIdentity"`
			HistoricalIdentities []string `json:"historicalIdentities"`
			Continued            bool     `json:"continued"`
		} `json:"crossInterval"`
		WithinSample struct {
			Status   string `json:"status"`
			Rejected bool   `json:"rejected"`
			Error    string `json:"error"`
		} `json:"withinSample"`
		EmptyTCP struct {
			Status   string `json:"status"`
			RowCount int    `json:"rowCount"`
		} `json:"emptyTCP"`
		QueryError struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"queryError"`
	}
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read observer identity report: %v", err)
	}
	if err := decodeLocalAICandidateJSON(raw, &report); err != nil {
		t.Fatalf("decode observer identity report: %v", err)
	}
	if report.SchemaVersion != "localai-windows-install-candidate-observer-identity/v1" || report.Status != "PASS" || report.Cycle != localAICandidateCycle || report.ReleaseStatus != "observer-identity-fixture" {
		t.Fatalf("observer identity report identity/status = %#v, want cycle-%s PASS observer-identity-fixture", report, localAICandidateCycle)
	}
	if report.CrossInterval.Status != "PASS" || report.CrossInterval.ActiveIdentity != "42/B" || !report.CrossInterval.Continued || !slices.Equal(report.CrossInterval.HistoricalIdentities, []string{"100/root", "42/A", "42/B"}) {
		t.Fatalf("cross-interval identity evidence = %#v, want retired 42/A with historical 42/A and 42/B", report.CrossInterval)
	}
	if report.WithinSample.Status != "PASS" || !report.WithinSample.Rejected || !strings.Contains(report.WithinSample.Error, "changed identity") {
		t.Fatalf("within-sample identity evidence = %#v, want fail-closed PID reuse", report.WithinSample)
	}
	if report.EmptyTCP.Status != "PASS" || report.EmptyTCP.RowCount != 0 {
		t.Fatalf("empty TCP evidence = %#v, want valid zero-row sample", report.EmptyTCP)
	}
	if report.QueryError.Status != "PASS" || !strings.Contains(report.QueryError.Error, "synthetic query error") {
		t.Fatalf("query-error evidence = %#v, want preserved query failure", report.QueryError)
	}
	t.Logf("LOCALAI-OBSERVER-IDENTITY status=PASS evidence=%s", raw)
}
func TestLocalAICandidateReleaseCheckoutPreparation(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows release checkout preparation is only available on Windows")
	}
	repoRoot := testutil.MustRepoRoot(t)
	clonePath := filepath.Join(t.TempDir(), "source-clone")
	cloneOutput, err := exec.Command("git", "clone", "--no-local", "--no-checkout", repoRoot, clonePath).CombinedOutput()
	if err != nil {
		t.Fatalf("clone exact-source fixture: %v\n%s", err, cloneOutput)
	}
	localAICandidateRunGit(t, clonePath, "checkout", "--detach", localAICandidateSourceCommit)
	reportPath := filepath.Join(t.TempDir(), "release-checkout-report.json")
	command := localAICandidatePowerShellCommand(t, "-InstallDir", t.TempDir(), "-PrepareReleaseCheckout", "-ReleaseCheckoutPath", clonePath, "-ReleaseCheckoutReportPath", reportPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare release checkout: %v\n%s", err, output)
	}
	var report localAICandidateReleaseCheckoutEvidence
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read release checkout report: %v", err)
	}
	if err := decodeLocalAICandidateJSON(raw, &report); err != nil {
		t.Fatalf("decode release checkout report: %v", err)
	}
	distEntryCount := 0
	for _, line := range report.PrivateExcludeAfter {
		if line == "/dist/" {
			distEntryCount++
		}
	}
	remoteIsNonLocal := strings.Contains(report.Origin, "://") && !strings.HasPrefix(report.Origin, "file://") || strings.HasPrefix(report.Origin, "git@")
	if report.Status != "PASS" || !report.GitDirIsDirectory || !report.DetachedHead || report.Head != localAICandidateSourceCommit || report.Tree != localAICandidateSourceTree || report.Origin == "" || remoteIsNonLocal || !report.StatusCleanBefore || !report.StatusCleanAfterPreparation || !slices.Equal(report.PrivateExcludeAdded, []string{"/dist/"}) || distEntryCount != 1 {
		t.Fatalf("release checkout preparation evidence = %#v, want exact detached clean local clone and /dist/ addition", report)
	}
	distPath := filepath.Join(clonePath, "dist")
	if err := os.MkdirAll(distPath, 0o700); err != nil {
		t.Fatalf("create representative dist root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distPath, "representative-output.txt"), []byte("ignored release output"), 0o600); err != nil {
		t.Fatalf("write representative dist output: %v", err)
	}
	if status := localAICandidateRunGit(t, clonePath, "status", "--porcelain=v1", "--untracked-files=all"); status != "" {
		t.Fatalf("status after ignored dist output = %q, want clean", status)
	}
	trackedPath := filepath.Join(clonePath, ".goreleaser.yml")
	trackedFile, err := os.OpenFile(trackedPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open tracked fixture: %v", err)
	}
	if _, err := trackedFile.WriteString("\n# controlled dirty fixture\n"); err != nil {
		trackedFile.Close()
		t.Fatalf("modify tracked fixture: %v", err)
	}
	if err := trackedFile.Close(); err != nil {
		t.Fatalf("close tracked fixture: %v", err)
	}
	if status := localAICandidateRunGit(t, clonePath, "status", "--porcelain=v1", "--untracked-files=all"); !strings.Contains(status, ".goreleaser.yml") {
		t.Fatalf("status after tracked source modification = %q, want tracked modification", status)
	}
	t.Logf("LOCALAI-CLONE status=PASS evidence=%s", raw)
}
func TestLocalAICandidateDependencyStageCopiesExactContainedClosure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows dependency staging is only available on Windows")
	}
	fixtureRoot := t.TempDir()
	sourceRepo := filepath.Join(fixtureRoot, "source-repo")
	stageRepo := filepath.Join(fixtureRoot, "stage-repo")
	if err := os.MkdirAll(sourceRepo, 0o700); err != nil {
		t.Fatalf("create source repository: %v", err)
	}
	if err := os.MkdirAll(stageRepo, 0o700); err != nil {
		t.Fatalf("create stage repository: %v", err)
	}
	localAICandidateWriteDependencyFixture(t, sourceRepo, true)
	localAICandidateWriteDependencyFixture(t, stageRepo, false)
	sourceUI := filepath.Join(sourceRepo, "ui")
	stageUI := filepath.Join(stageRepo, "ui")
	sourceNodeModules := filepath.Join(sourceUI, "node_modules")
	stageNodeModules := filepath.Join(stageUI, "node_modules")
	linkPath := filepath.Join(sourceNodeModules, "workspace-link")
	linkTarget := filepath.Join(sourceUI, "packages", "app")
	linkCommand := exec.Command("cmd.exe", "/c", "mklink", "/J", linkPath, linkTarget)
	if output, err := linkCommand.CombinedOutput(); err != nil {
		t.Skipf("contained junction fixture unavailable: %v (%s)", err, output)
	}
	t.Cleanup(func() { _ = os.Remove(linkPath) })
	stageEvidence := filepath.Join(fixtureRoot, "evidence")
	if err := os.MkdirAll(stageEvidence, 0o700); err != nil {
		t.Fatalf("create dependency evidence root: %v", err)
	}
	lockPath := filepath.Join(sourceUI, "bun.lock")
	lockSHA256, err := sha256File(lockPath)
	if err != nil {
		t.Fatalf("hash fixture lock: %v", err)
	}
	lockBlob := localAICandidateRunGit(t, sourceRepo, "rev-parse", "HEAD:ui/bun.lock")
	command := localAICandidatePowerShellCommand(t,
		"-InstallDir", stageEvidence,
		"-StageDependencyClosure",
		"-DependencySourceRoot", sourceNodeModules,
		"-DependencyStageRoot", stageNodeModules,
		"-DependencySourceManifestPath", filepath.Join(stageEvidence, "source-manifest.jsonl"),
		"-DependencyStageManifestPath", filepath.Join(stageEvidence, "stage-manifest.jsonl"),
		"-DependencyReportPath", filepath.Join(stageEvidence, "stage-report.json"),
		"-DependencyExpectedLockSHA256", lockSHA256,
		"-DependencyExpectedLockBlob", lockBlob,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		reportBytes, _ := os.ReadFile(filepath.Join(stageEvidence, "stage-report.json"))
		t.Fatalf("exact dependency staging: %v\n%s\nreport=%s", err, output, reportBytes)
	}
	var report localAICandidateDependencyReport
	raw, err := os.ReadFile(filepath.Join(stageEvidence, "stage-report.json"))
	if err != nil {
		t.Fatalf("read dependency stage report: %v", err)
	}
	if err := decodeLocalAICandidateJSON(raw, &report); err != nil {
		t.Fatalf("decode dependency stage report: %v", err)
	}
	if report.SchemaVersion != "localai-windows-install-candidate-dependency-stage/v1" || report.Status != "PASS" || report.Phase != "stage" || report.PackageInstall != "NOT_RUN" || !report.Equality.SourceStageManifestEqual || report.SourceManifest.EntryCount != 5 || report.StageManifest.EntryCount != 5 || report.SourceManifest.JunctionCount != 1 || report.StageManifest.JunctionCount != 1 || report.SourceManifest.ManifestSHA256 != report.StageManifest.ManifestSHA256 || report.Error != "" {
		t.Fatalf("dependency stage evidence = %#v, want exact no-install closure copy", report)
	}
	if copyEvidence, ok := report.Copy.(map[string]any); !ok || copyEvidence["rewrittenLinks"] != float64(1) {
		t.Fatalf("dependency stage copy evidence = %#v, want one rewritten contained junction", report.Copy)
	}
	if _, err := os.Stat(filepath.Join(stageNodeModules, "dep", "index.js")); err != nil {
		t.Fatalf("staged dependency content missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stageNodeModules, "workspace-link", "package.json")); err != nil {
		t.Fatalf("staged contained junction target missing: %v", err)
	}
	indexPath := filepath.Join(stageNodeModules, "dep", "index.js")
	originalIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read staged dependency fixture before mutation: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("module.exports = false;\n"), 0o600); err != nil {
		t.Fatalf("mutate staged dependency fixture: %v", err)
	}
	mutationReportPath := filepath.Join(stageEvidence, "post-build-mutation-report.json")
	mutationCommand := localAICandidatePowerShellCommand(t,
		"-InstallDir", stageEvidence,
		"-VerifyDependencyClosure",
		"-DependencySourceRoot", sourceNodeModules,
		"-DependencyStageRoot", stageNodeModules,
		"-DependencyExpectedManifestPath", filepath.Join(stageEvidence, "stage-manifest.jsonl"),
		"-DependencyPostBuildManifestPath", filepath.Join(stageEvidence, "post-build-mutation-manifest.jsonl"),
		"-DependencySourceAfterManifestPath", filepath.Join(stageEvidence, "source-after-mutation-manifest.jsonl"),
		"-DependencyReportPath", mutationReportPath,
		"-DependencyExpectedLockSHA256", lockSHA256,
		"-DependencyExpectedLockBlob", lockBlob,
	)
	mutationOutput, mutationErr := mutationCommand.CombinedOutput()
	if mutationErr == nil {
		t.Fatalf("post-build dependency mutation unexpectedly passed\n%s", mutationOutput)
	}
	mutationReportBytes, err := os.ReadFile(mutationReportPath)
	if err != nil {
		t.Fatalf("read post-build mutation report: %v", err)
	}
	var mutationReport struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(mutationReportBytes, &mutationReport); err != nil {
		t.Fatalf("decode post-build mutation report: %v", err)
	}
	if mutationReport.Status != "FAIL" || !strings.Contains(mutationReport.Error, "differs") {
		t.Fatalf("post-build mutation evidence = %#v, want fail-closed equality drift", mutationReport)
	}
	if err := os.WriteFile(indexPath, originalIndex, 0o600); err != nil {
		t.Fatalf("restore staged dependency fixture: %v", err)
	}
	verifyCommand := localAICandidatePowerShellCommand(t,
		"-InstallDir", stageEvidence,
		"-VerifyDependencyClosure",
		"-DependencySourceRoot", sourceNodeModules,
		"-DependencyStageRoot", stageNodeModules,
		"-DependencyExpectedManifestPath", filepath.Join(stageEvidence, "stage-manifest.jsonl"),
		"-DependencyPostBuildManifestPath", filepath.Join(stageEvidence, "post-build-manifest.jsonl"),
		"-DependencySourceAfterManifestPath", filepath.Join(stageEvidence, "source-after-manifest.jsonl"),
		"-DependencyReportPath", filepath.Join(stageEvidence, "post-build-report.json"),
		"-DependencyExpectedLockSHA256", lockSHA256,
		"-DependencyExpectedLockBlob", lockBlob,
	)
	verifyOutput, err := verifyCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("post-build dependency equality verification: %v\n%s", err, verifyOutput)
	}
	verifyReportBytes, err := os.ReadFile(filepath.Join(stageEvidence, "post-build-report.json"))
	if err != nil {
		t.Fatalf("read post-build report: %v", err)
	}
	var verifyReport struct {
		Status   string `json:"status"`
		Phase    string `json:"phase"`
		Equality struct {
			SourceStagePostBuildEqual bool `json:"sourceStagePostBuildEqual"`
		} `json:"equality"`
	}
	if err := json.Unmarshal(verifyReportBytes, &verifyReport); err != nil {
		t.Fatalf("decode post-build report: %v", err)
	}
	if verifyReport.Status != "PASS" || verifyReport.Phase != "post-build" || !verifyReport.Equality.SourceStagePostBuildEqual {
		t.Fatalf("post-build equality evidence = %#v, want source/stage/post-build equality", verifyReport)
	}
	t.Logf("LOCALAI-STAGE status=PASS evidence=%s", raw)
}
func TestLocalAICandidateDependencyStageRejectsExternalLink(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows dependency staging is only available on Windows")
	}
	fixtureRoot := t.TempDir()
	sourceRepo := filepath.Join(fixtureRoot, "source-repo")
	stageRepo := filepath.Join(fixtureRoot, "stage-repo")
	for _, repository := range []string{sourceRepo, stageRepo} {
		if err := os.MkdirAll(repository, 0o700); err != nil {
			t.Fatalf("create fixture repository: %v", err)
		}
	}
	localAICandidateWriteDependencyFixture(t, sourceRepo, true)
	localAICandidateWriteDependencyFixture(t, stageRepo, false)
	sourceUI := filepath.Join(sourceRepo, "ui")
	externalTarget := filepath.Join(fixtureRoot, "external-target")
	if err := os.MkdirAll(externalTarget, 0o700); err != nil {
		t.Fatalf("create external link target: %v", err)
	}
	externalLink := filepath.Join(sourceUI, "node_modules", "external-link")
	linkCommand := exec.Command("cmd.exe", "/c", "mklink", "/J", externalLink, externalTarget)
	if output, err := linkCommand.CombinedOutput(); err != nil {
		t.Skipf("external junction fixture unavailable: %v (%s)", err, output)
	}
	t.Cleanup(func() { _ = os.Remove(externalLink) })
	lockPath := filepath.Join(sourceUI, "bun.lock")
	lockSHA256, err := sha256File(lockPath)
	if err != nil {
		t.Fatalf("hash fixture lock: %v", err)
	}
	lockBlob := localAICandidateRunGit(t, sourceRepo, "rev-parse", "HEAD:ui/bun.lock")
	evidenceRoot := filepath.Join(fixtureRoot, "evidence")
	if err := os.MkdirAll(evidenceRoot, 0o700); err != nil {
		t.Fatalf("create dependency evidence root: %v", err)
	}
	reportPath := filepath.Join(evidenceRoot, "stage-report.json")
	command := localAICandidatePowerShellCommand(t,
		"-InstallDir", evidenceRoot,
		"-StageDependencyClosure",
		"-DependencySourceRoot", filepath.Join(sourceUI, "node_modules"),
		"-DependencyStageRoot", filepath.Join(stageRepo, "ui", "node_modules"),
		"-DependencySourceManifestPath", filepath.Join(evidenceRoot, "source-manifest.jsonl"),
		"-DependencyStageManifestPath", filepath.Join(evidenceRoot, "stage-manifest.jsonl"),
		"-DependencyReportPath", reportPath,
		"-DependencyExpectedLockSHA256", lockSHA256,
		"-DependencyExpectedLockBlob", lockBlob,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("external dependency junction unexpectedly passed\n%s", output)
	}
	var report localAICandidateDependencyReport
	raw, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatalf("read external-link failure report: %v\n%s", readErr, output)
	}
	if decodeErr := decodeLocalAICandidateJSON(raw, &report); decodeErr != nil {
		t.Fatalf("decode external-link failure report: %v", decodeErr)
	}
	if report.Status != "FAIL" || !strings.Contains(report.Error, "outside the contained root") {
		t.Fatalf("external-link failure evidence = %#v, want containment rejection", report)
	}
	t.Logf("LOCALAI-STAGE-EXTERNAL-LINK status=PASS evidence=%s", raw)
}
func TestLocalAICandidateDependencyStageRejectsMissingRequiredPackage(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows dependency staging is only available on Windows")
	}
	fixtureRoot := t.TempDir()
	sourceRepo := filepath.Join(fixtureRoot, "source-repo")
	stageRepo := filepath.Join(fixtureRoot, "stage-repo")
	for _, repository := range []string{sourceRepo, stageRepo} {
		if err := os.MkdirAll(repository, 0o700); err != nil {
			t.Fatalf("create fixture repository: %v", err)
		}
	}
	localAICandidateWriteDependencyFixture(t, sourceRepo, true)
	localAICandidateWriteDependencyFixture(t, stageRepo, false)
	sourceUI := filepath.Join(sourceRepo, "ui")
	missingManifest := filepath.Join(sourceUI, "node_modules", "dep", "package.json")
	if err := os.Remove(missingManifest); err != nil {
		t.Fatalf("remove required fixture package: %v", err)
	}
	localAICandidateRunGit(t, sourceRepo, "add", "-A")
	localAICandidateRunGit(t, sourceRepo, "commit", "-m", "remove required package")
	lockPath := filepath.Join(sourceUI, "bun.lock")
	lockSHA256, err := sha256File(lockPath)
	if err != nil {
		t.Fatalf("hash fixture lock: %v", err)
	}
	lockBlob := localAICandidateRunGit(t, sourceRepo, "rev-parse", "HEAD:ui/bun.lock")
	evidenceRoot := filepath.Join(fixtureRoot, "evidence")
	if err := os.MkdirAll(evidenceRoot, 0o700); err != nil {
		t.Fatalf("create failure evidence root: %v", err)
	}
	command := localAICandidatePowerShellCommand(t,
		"-InstallDir", evidenceRoot,
		"-StageDependencyClosure",
		"-DependencySourceRoot", filepath.Join(sourceUI, "node_modules"),
		"-DependencyStageRoot", filepath.Join(stageRepo, "ui", "node_modules"),
		"-DependencySourceManifestPath", filepath.Join(evidenceRoot, "source-manifest.jsonl"),
		"-DependencyStageManifestPath", filepath.Join(evidenceRoot, "stage-manifest.jsonl"),
		"-DependencyReportPath", filepath.Join(evidenceRoot, "stage-report.json"),
		"-DependencyExpectedLockSHA256", lockSHA256,
		"-DependencyExpectedLockBlob", lockBlob,
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("missing required dependency unexpectedly passed\n%s", output)
	}
	var report localAICandidateDependencyReport
	raw, readErr := os.ReadFile(filepath.Join(evidenceRoot, "stage-report.json"))
	if readErr != nil {
		t.Fatalf("read dependency failure report: %v\n%s", readErr, output)
	}
	if decodeErr := decodeLocalAICandidateJSON(raw, &report); decodeErr != nil {
		t.Fatalf("decode dependency failure report: %v", decodeErr)
	}
	if report.Status != "FAIL" || !strings.Contains(report.Error, "missing required package") {
		t.Fatalf("dependency failure evidence = %#v, want fail-closed missing package", report)
	}
	t.Logf("LOCALAI-STAGE-FAILURE status=PASS evidence=%s", raw)
}
func localAICandidateWriteDependencyFixture(t *testing.T, repository string, includeDependency bool) {
	t.Helper()
	uiRoot := filepath.Join(repository, "ui")
	workspaceRoot := filepath.Join(uiRoot, "packages", "app")
	if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
		t.Fatalf("create fixture workspace: %v", err)
	}
	dependencyName := "dep"
	rootPackage := fmt.Sprintf(`{"name":"fixture-root","private":true,"workspaces":["packages/*"],"dependencies":{"%s":"1.0.0"},"devDependencies":{},"optionalDependencies":{}}`, dependencyName)
	workspacePackage := fmt.Sprintf(`{"name":"fixture-app","dependencies":{"%s":"1.0.0"},"devDependencies":{},"optionalDependencies":{}}`, dependencyName)
	lock := fmt.Sprintf(`{"lockfileVersion":1,"configVersion":0,"workspaces":{"":{"name":"fixture-root","dependencies":{"%s":"1.0.0"},"devDependencies":{},"optionalDependencies":{}},"packages/app":{"name":"fixture-app","dependencies":{"%s":"1.0.0"},"devDependencies":{},"optionalDependencies":{}}},"packages":{"dep@1.0.0":["dep@1.0.0","",{},""]}}`, dependencyName, dependencyName)
	for path, contents := range map[string]string{
		filepath.Join(uiRoot, "package.json"):        rootPackage,
		filepath.Join(workspaceRoot, "package.json"): workspacePackage,
		filepath.Join(uiRoot, "bun.lock"):            lock,
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write fixture file %s: %v", path, err)
		}
	}
	if includeDependency {
		dependencyRoot := filepath.Join(uiRoot, "node_modules", dependencyName)
		if err := os.MkdirAll(dependencyRoot, 0o700); err != nil {
			t.Fatalf("create fixture dependency: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dependencyRoot, "package.json"), []byte(`{"name":"dep","version":"1.0.0","dependencies":{},"optionalDependencies":{},"peerDependencies":{}}`), 0o600); err != nil {
			t.Fatalf("write fixture dependency manifest: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dependencyRoot, "index.js"), []byte("module.exports = true;\n"), 0o600); err != nil {
			t.Fatalf("write fixture dependency file: %v", err)
		}
	}
	localAICandidateRunGit(t, repository, "init")
	localAICandidateRunGit(t, repository, "config", "user.email", "fixture@example.invalid")
	localAICandidateRunGit(t, repository, "config", "user.name", "dependency-fixture")
	localAICandidateRunGit(t, repository, "add", ".")
	localAICandidateRunGit(t, repository, "commit", "-m", "dependency fixture")
}
func validateLocalAICandidateObserverEvidence(observation localAICandidateObserverEvidence) error {
	process := observation.ProcessNetworkObserver
	if process.Status != "PASS" || !process.StartedBeforeRoot || !process.ContinuedThroughDescendantExit || process.MaximumGapMilliseconds < 0 || process.MaximumGapMilliseconds > localAICandidateProcessNetworkGapMaximum || process.NonLoopbackConnections != 0 || process.ExternalTransferBytes != 0 || process.SampleCount <= 0 || process.TCPTableQueries != process.SampleCount || process.ZeroConnectionSamples <= 0 || process.OwnedConnectionMatches < 0 || process.OwnedProcessIdentityCount <= 0 || process.QueryMode != localAICandidateObserverQueryMode || len(process.ForbiddenProcesses) != 0 || process.Error != "" {
		return fmt.Errorf("invalid process/network observer evidence: %#v", process)
	}
	disk := observation.DiskObserver
	if disk.Status != "PASS" || !disk.Independent || !disk.StartPeriodicFinal || disk.MaximumGapMilliseconds < 0 || disk.MaximumGapMilliseconds > localAICandidateDiskGapMaximum || disk.PeakDeltaBytes < 0 || disk.PeakDeltaBytes > localAICandidateTemporaryDiskMaximum || disk.Error != "" {
		return fmt.Errorf("invalid disk observer evidence: %#v", disk)
	}
	if observation.DescendantMaximum != localAICandidateDescendantMaximum || observation.DescendantHighWater < 1 || observation.DescendantHighWater > observation.DescendantMaximum {
		return fmt.Errorf("descendant observation = high-water %d maximum %d, want high-water 1..%d", observation.DescendantHighWater, observation.DescendantMaximum, localAICandidateDescendantMaximum)
	}
	if observation.RootExitCode == nil {
		return errors.New("root exit code is required before process disposal")
	}
	rootOutput := observation.RootOutput
	if rootOutput.Status != "PASS" || rootOutput.MaximumBytes <= 0 {
		return fmt.Errorf("invalid root output evidence: %#v", rootOutput)
	}
	for name, stream := range map[string]localAICandidateRootOutputStream{"stdout": rootOutput.Stdout, "stderr": rootOutput.Stderr} {
		if stream.Status != "PASS" || !stream.Present || !filepath.IsAbs(stream.Path) || stream.TotalBytes < 0 || stream.CapturedBytes < 0 || stream.CapturedBytes > stream.TotalBytes || stream.CapturedBytes > rootOutput.MaximumBytes || stream.RedactedBytes < 0 || !localAICandidateSHA256Pattern.MatchString(stream.SHA256) {
			return fmt.Errorf("invalid root %s output evidence: %#v", name, stream)
		}
		if stream.Truncated != (stream.TotalBytes > rootOutput.MaximumBytes) {
			return fmt.Errorf("root %s truncation metadata does not match byte counts: %#v", name, stream)
		}
	}
	if *observation.RootExitCode != 0 || observation.FailurePropagation != "PASS" || observation.ModelBackendBytes != 0 || observation.ModelBackendCalls != 0 {
		return fmt.Errorf("invalid observer completion evidence: %#v", observation)
	}
	return nil
}
func TestLocalAICandidateObserverEvidenceRejectsUnprovenSamples(t *testing.T) {
	rootExitCode := int64(0)
	rootOutputPath := filepath.Join(os.TempDir(), "localai-observer-output.txt")
	base := localAICandidateObserverEvidence{
		ProcessNetworkObserver: localAICandidateProcessNetworkObserver{
			StartedBeforeRoot: true, ContinuedThroughDescendantExit: true, MaximumGapMilliseconds: 1,
			Status: "PASS", SampleCount: 3, TCPTableQueries: 3, ZeroConnectionSamples: 3,
			OwnedProcessIdentityCount: 3, QueryMode: localAICandidateObserverQueryMode,
		},
		DiskObserver:        localAICandidateDiskObserver{Independent: true, StartPeriodicFinal: true, MaximumGapMilliseconds: 1, Status: "PASS"},
		DescendantMaximum:   localAICandidateDescendantMaximum,
		DescendantHighWater: 1,
		RootExitCode:        &rootExitCode,
		RootOutput: localAICandidateRootOutput{
			Status:       "PASS",
			MaximumBytes: 1024,
			Stdout:       localAICandidateRootOutputStream{Status: "PASS", Path: rootOutputPath, Present: true, TotalBytes: 10, CapturedBytes: 10, SHA256: strings.Repeat("a", sha256.Size*2)},
			Stderr:       localAICandidateRootOutputStream{Status: "PASS", Path: rootOutputPath, Present: true, TotalBytes: 10, CapturedBytes: 10, SHA256: strings.Repeat("b", sha256.Size*2)},
		},
		FailurePropagation: "PASS",
	}
	tests := []struct {
		name string
		edit func(*localAICandidateObserverEvidence)
	}{
		{name: "query count drift", edit: func(evidence *localAICandidateObserverEvidence) { evidence.ProcessNetworkObserver.TCPTableQueries = 2 }},
		{name: "zero row not recorded", edit: func(evidence *localAICandidateObserverEvidence) {
			evidence.ProcessNetworkObserver.ZeroConnectionSamples = 0
		}},
		{name: "identity count missing", edit: func(evidence *localAICandidateObserverEvidence) {
			evidence.ProcessNetworkObserver.OwnedProcessIdentityCount = 0
		}},
		{name: "query mode missing", edit: func(evidence *localAICandidateObserverEvidence) {
			evidence.ProcessNetworkObserver.QueryMode = "per-pid"
		}},
		{name: "query error", edit: func(evidence *localAICandidateObserverEvidence) {
			evidence.ProcessNetworkObserver.Error = "query failed"
		}},
		{name: "disk gap over limit", edit: func(evidence *localAICandidateObserverEvidence) {
			evidence.DiskObserver.MaximumGapMilliseconds = localAICandidateDiskGapMaximum + 1
		}},
		{name: "disk error", edit: func(evidence *localAICandidateObserverEvidence) { evidence.DiskObserver.Error = "disk sample failed" }},
		{name: "root output missing", edit: func(evidence *localAICandidateObserverEvidence) { evidence.RootOutput.Stdout.Present = false }},
		{name: "root output hash missing", edit: func(evidence *localAICandidateObserverEvidence) { evidence.RootOutput.Stderr.SHA256 = "" }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			evidence := base
			test.edit(&evidence)
			if err := validateLocalAICandidateObserverEvidence(evidence); err == nil {
				t.Fatal("observer evidence unexpectedly passed")
			}
		})
	}
}
func validateLocalAICandidate(manifestPath string) (localAICandidateValidationEvidence, error) {
	return validateLocalAICandidateWithBuildInfoReader(manifestPath, buildinfo.ReadFile)
}
func validateLocalAICandidateWithBuildInfoReader(manifestPath string, readBuildInfo func(string) (*debug.BuildInfo, error)) (localAICandidateValidationEvidence, error) {
	if !filepath.IsAbs(manifestPath) {
		return localAICandidateValidationEvidence{}, fmt.Errorf("candidate manifest path %q must be absolute", manifestPath)
	}
	candidateDir := filepath.Dir(manifestPath)
	if _, err := validateLocalAICandidatePath(candidateDir, "candidate directory"); err != nil {
		return localAICandidateValidationEvidence{}, err
	}
	manifestInfo, err := validateLocalAICandidatePath(manifestPath, "candidate manifest")
	if err != nil {
		return localAICandidateValidationEvidence{}, err
	}
	manifestBytes, manifest, err := readLocalAICandidateManifest(manifestPath)
	if err != nil {
		return localAICandidateValidationEvidence{}, err
	}
	if err := validateLocalAICandidateIdentity(manifest); err != nil {
		return localAICandidateValidationEvidence{}, err
	}
	manifestDigest, err := sha256File(manifestPath)
	if err != nil {
		return localAICandidateValidationEvidence{}, fmt.Errorf("hash candidate manifest: %w", err)
	}
	if int64(len(manifestBytes)) != manifestInfo.Size() {
		return localAICandidateValidationEvidence{}, fmt.Errorf("candidate manifest changed while reading: read %d bytes, stat reported %d", len(manifestBytes), manifestInfo.Size())
	}
	if err := validateLocalAICandidateDetachedDigest(filepath.Dir(manifestPath), manifestDigest); err != nil {
		return localAICandidateValidationEvidence{}, err
	}
	artifactByRole := make(map[string]localAICandidateArtifact, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		if _, exists := artifactByRole[artifact.Role]; exists {
			return localAICandidateValidationEvidence{}, fmt.Errorf("candidate manifest contains duplicate artifact role %q", artifact.Role)
		}
		artifactByRole[artifact.Role] = artifact
	}
	archive, ok := artifactByRole["windows-amd64-archive"]
	if !ok {
		return localAICandidateValidationEvidence{}, errors.New("candidate manifest is missing windows-amd64-archive artifact")
	}
	installer, ok := artifactByRole["windows-installer"]
	if !ok {
		return localAICandidateValidationEvidence{}, errors.New("candidate manifest is missing windows-installer artifact")
	}
	executable, ok := artifactByRole["windows-amd64-executable"]
	if !ok {
		return localAICandidateValidationEvidence{}, errors.New("candidate manifest is missing windows-amd64-executable artifact")
	}
	archiveCandidates, err := localAICandidateArchiveCandidates(candidateDir)
	if err != nil {
		return localAICandidateValidationEvidence{}, err
	}
	if len(archiveCandidates) != 1 {
		return localAICandidateValidationEvidence{}, fmt.Errorf("candidate archive selection: expected exactly one windows-amd64 ZIP, found %d (%s)", len(archiveCandidates), strings.Join(archiveCandidates, ", "))
	}
	if archiveCandidates[0] != archive.File {
		return localAICandidateValidationEvidence{}, fmt.Errorf("candidate archive selection: manifest names %q, retained archive is %q", archive.File, archiveCandidates[0])
	}
	archiveEvidence, err := validateLocalAICandidateArtifact(candidateDir, archive, true)
	if err != nil {
		return localAICandidateValidationEvidence{}, fmt.Errorf("validate windows archive: %w", err)
	}
	installerEvidence, err := validateLocalAICandidateArtifact(candidateDir, installer, false)
	if err != nil {
		return localAICandidateValidationEvidence{}, fmt.Errorf("validate windows installer: %w", err)
	}
	embeddedSourceRevision, embeddedVCSModified, err := readAndValidateLocalAICandidateBuildInfo(filepath.Join(candidateDir, filepath.FromSlash(executable.File)), readBuildInfo)
	if err != nil {
		return localAICandidateValidationEvidence{}, err
	}
	return localAICandidateValidationEvidence{
		Status:                 "PASS",
		Property:               "prebuilt-candidate-schema-source-target-artifact-and-detached-digest-identity",
		SchemaVersion:          manifest.SchemaVersion,
		SourceCommit:           manifest.Source.Commit,
		SourceTree:             manifest.Source.Tree,
		EmbeddedSourceRevision: embeddedSourceRevision,
		EmbeddedVCSModified:    embeddedVCSModified,
		CandidateVersion:       manifest.Build.CandidateVersion,
		CLIVersion:             manifest.Build.CLIVersion,
		Target:                 manifest.Build.Target.GOOS + "/" + manifest.Build.Target.GOARCH,
		Archive:                archiveEvidence,
		Installer:              installerEvidence,
		Manifest: localAICandidateArtifactEvidence{
			Role:   "candidate-manifest",
			File:   filepath.Base(manifestPath),
			Bytes:  manifestInfo.Size(),
			SHA256: manifestDigest,
		},
		Preconditions: "not-run-by-schema-cell; install cell must supply task-owned PATH and roots",
	}, nil
}
func readAndValidateLocalAICandidateBuildInfo(path string, readBuildInfo func(string) (*debug.BuildInfo, error)) (string, bool, error) {
	if readBuildInfo == nil {
		return "", false, errors.New("read executable build info: reader is nil")
	}
	build, err := readBuildInfo(path)
	if err != nil {
		return "", false, fmt.Errorf("read executable build info %q: %w", path, err)
	}
	if build == nil {
		return "", false, fmt.Errorf("read executable build info %q: returned no metadata", path)
	}
	revision, err := localAICandidateBuildSetting(build, "vcs.revision")
	if err != nil {
		return "", false, err
	}
	if revision != localAICandidateSourceCommit {
		return "", false, fmt.Errorf("candidate executable build info vcs.revision = %q, want %q", revision, localAICandidateSourceCommit)
	}
	modified, err := localAICandidateBuildSetting(build, "vcs.modified")
	if err != nil {
		return "", false, err
	}
	if modified != "false" {
		return "", false, fmt.Errorf("candidate executable build info vcs.modified = %q, want %q", modified, "false")
	}
	return revision, false, nil
}
func localAICandidateBuildSetting(build *debug.BuildInfo, key string) (string, error) {
	var value string
	found := false
	for _, setting := range build.Settings {
		if setting.Key != key {
			continue
		}
		if found {
			return "", fmt.Errorf("candidate executable build info contains duplicate %q settings", key)
		}
		found = true
		value = strings.TrimSpace(setting.Value)
	}
	if !found || value == "" {
		return "", fmt.Errorf("candidate executable build info %s is missing", key)
	}
	return value, nil
}
func readLocalAICandidateManifest(manifestPath string) ([]byte, localAICandidateManifest, error) {
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, localAICandidateManifest{}, fmt.Errorf("read candidate manifest %q: %w", manifestPath, err)
	}
	var manifest localAICandidateManifest
	if err := decodeLocalAICandidateJSON(manifestBytes, &manifest); err != nil {
		return nil, localAICandidateManifest{}, fmt.Errorf("decode candidate manifest %q: %w", manifestPath, err)
	}
	return manifestBytes, manifest, nil
}
func decodeLocalAICandidateJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("trailing data: %w", err)
	}
	return nil
}
func validateLocalAICandidateIdentity(manifest localAICandidateManifest) error {
	if manifest.SchemaVersion != localAICandidateSchemaVersion {
		return fmt.Errorf("candidate schemaVersion = %q, want %q", manifest.SchemaVersion, localAICandidateSchemaVersion)
	}
	if manifest.Project != localAICandidateProject {
		return fmt.Errorf("candidate project = %q, want %q", manifest.Project, localAICandidateProject)
	}
	if manifest.Cycle != localAICandidateCycle {
		return fmt.Errorf("candidate cycle = %q, want %q", manifest.Cycle, localAICandidateCycle)
	}
	if manifest.Source.Repository != localAICandidateRepository ||
		manifest.Source.Commit != localAICandidateSourceCommit ||
		manifest.Source.Tree != localAICandidateSourceTree {
		return fmt.Errorf("candidate source identity = repository=%q commit=%q tree=%q, want repository=%q commit=%q tree=%q", manifest.Source.Repository, manifest.Source.Commit, manifest.Source.Tree, localAICandidateRepository, localAICandidateSourceCommit, localAICandidateSourceTree)
	}
	if strings.TrimSpace(manifest.Build.CandidateVersion) == "" || strings.ContainsAny(manifest.Build.CandidateVersion, "/\\\\:*?\"<>|\t\r\n ") {
		return fmt.Errorf("candidateVersion %q is not a usable release version", manifest.Build.CandidateVersion)
	}
	for name, value := range map[string]string{
		"cliVersion":             manifest.Build.CLIVersion,
		"goVersion":              manifest.Build.GoVersion,
		"goreleaserVersion":      manifest.Build.GoReleaserVersion,
		"goreleaserConfigSha256": manifest.Build.GoReleaserConfigSHA256,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("candidate build.%s is required", name)
		}
	}
	if manifest.Build.GoVersion != localAICandidateGoVersion {
		return fmt.Errorf("candidate Go version = %q, want %q", manifest.Build.GoVersion, localAICandidateGoVersion)
	}
	if manifest.Build.GoReleaserVersion != localAICandidateGoReleaser {
		return fmt.Errorf("candidate goreleaserVersion = %q, want %q", manifest.Build.GoReleaserVersion, localAICandidateGoReleaser)
	}
	if !localAICandidateSHA256Pattern.MatchString(manifest.Build.GoReleaserConfigSHA256) {
		return fmt.Errorf("candidate goreleaserConfigSha256 = %q, want lowercase SHA-256", manifest.Build.GoReleaserConfigSHA256)
	}
	if manifest.Build.Target.GOOS != localAICandidateGOOS || manifest.Build.Target.GOARCH != localAICandidateGOARCH {
		return fmt.Errorf("candidate target = %s/%s, want %s/%s", manifest.Build.Target.GOOS, manifest.Build.Target.GOARCH, localAICandidateGOOS, localAICandidateGOARCH)
	}
	if manifest.Build.Target.CgoEnabled == nil || *manifest.Build.Target.CgoEnabled {
		return fmt.Errorf("candidate target.cgoEnabled = %v, want false", manifest.Build.Target.CgoEnabled != nil && *manifest.Build.Target.CgoEnabled)
	}
	if len(manifest.Artifacts) != 3 {
		return fmt.Errorf("candidate artifacts count = %d, want exactly 3", len(manifest.Artifacts))
	}
	for _, artifact := range manifest.Artifacts {
		if (artifact.Role != "windows-amd64-archive" && artifact.Role != "windows-installer" && artifact.Role != "windows-amd64-executable") || !isLocalAICandidateBasename(artifact.File) {
			return fmt.Errorf("candidate artifact %q has an unsupported role or unsafe file %q", artifact.Role, artifact.File)
		}
		if artifact.Bytes == nil || *artifact.Bytes <= 0 || !localAICandidateSHA256Pattern.MatchString(artifact.SHA256) {
			return fmt.Errorf("candidate artifact %q has invalid positive-byte/lowercase-SHA256 metadata", artifact.Role)
		}
	}
	for _, limit := range []struct {
		name   string
		actual *int64
		want   int64
	}{
		{"temporaryDiskBytesMaximum", manifest.Limits.TemporaryDiskBytesMaximum, localAICandidateTemporaryDiskMaximum},
		{"ordinaryToolDownloadBytesMaximum", manifest.Limits.OrdinaryToolDownloadMaximum, localAICandidateToolDownloadMaximum},
		{"modelBackendDownloadBytesMaximum", manifest.Limits.ModelBackendDownloadMaximum, localAICandidateModelDownloadMaximum},
		{"modelCallsMaximum", manifest.Limits.ModelCallsMaximum, localAICandidateModelCallsMaximum},
		{"paidUSDMaximum", manifest.Limits.PaidUSDMaximum, localAICandidatePaidUSDMaximum},
		{"descendantMaximum", manifest.Limits.DescendantMaximum, localAICandidateDescendantMaximum},
		{"packagingOrSmokeRerunsMaximum", manifest.Limits.PackagingOrSmokeRerunsMaximum, localAICandidateRerunsMaximum},
	} {
		if err := validateLocalAICandidateLimit(limit.name, limit.actual, limit.want); err != nil {
			return err
		}
	}
	if strings.TrimSpace(manifest.Limits.GOFLAGS) == "" || manifest.Limits.GOFLAGS != localAICandidateGOFLAGS || strings.TrimSpace(manifest.Limits.GOMAXPROCS) == "" || manifest.Limits.GOMAXPROCS != localAICandidateGOMAXPROCS {
		return fmt.Errorf("candidate Go controls = GOFLAGS=%q GOMAXPROCS=%q, want GOFLAGS=%q GOMAXPROCS=%q", manifest.Limits.GOFLAGS, manifest.Limits.GOMAXPROCS, localAICandidateGOFLAGS, localAICandidateGOMAXPROCS)
	}
	if err := validateLocalAICandidateAttempts(manifest.Attempts); err != nil {
		return err
	}
	return nil
}
func validateLocalAICandidateAttempts(attempts localAICandidateAttempts) error {
	wantPriorCycles := []string{"030", "040", "042", "045", "048", "050", "063", "067", "068"}
	if !slices.Equal(attempts.PriorCycles, wantPriorCycles) {
		return fmt.Errorf("candidate attempts priorCycles = %v, want %v", attempts.PriorCycles, wantPriorCycles)
	}
	for _, attempt := range []struct {
		name    string
		maximum *int64
		used    *int64
		wantMax int64
		wantUse int64
	}{
		{name: "cycle063", maximum: attempts.Cycle063BuildMaximum, used: attempts.Cycle063BuildUsed, wantMax: 1, wantUse: 1},
		{name: "cycle067", maximum: attempts.Cycle067BuildMaximum, used: attempts.Cycle067BuildUsed, wantMax: 2, wantUse: 2},
		{name: "cycle068", maximum: attempts.Cycle068BuildMaximum, used: attempts.Cycle068BuildUsed, wantMax: 1, wantUse: 1},
		{name: "cycle070", maximum: attempts.Cycle070BuildMaximum, used: attempts.Cycle070BuildUsed, wantMax: 1, wantUse: 1},
		{name: "cycle076", maximum: attempts.Cycle076BuildMaximum, used: attempts.Cycle076BuildUsed, wantMax: 1, wantUse: 1},
	} {
		if attempt.maximum == nil || attempt.used == nil || *attempt.maximum != attempt.wantMax || *attempt.used != attempt.wantUse {
			return fmt.Errorf("candidate attempts %sBuildMaximum/Used = %v/%v, want %d/%d", attempt.name, valueOrZero(attempt.maximum), valueOrZero(attempt.used), attempt.wantMax, attempt.wantUse)
		}
	}
	return nil
}
func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
func validateLocalAICandidateLimit(name string, actual *int64, want int64) error {
	if actual == nil {
		return fmt.Errorf("candidate limits.%s is required", name)
	}
	if *actual != want {
		return fmt.Errorf("candidate limits.%s = %d, want %d", name, *actual, want)
	}
	return nil
}
func localAICandidateArchiveCandidates(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read candidate directory: %w", err)
	}
	var candidates []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "you_") || !strings.HasSuffix(strings.ToLower(entry.Name()), "_windows_amd64.zip") {
			continue
		}
		if _, err := validateLocalAICandidatePath(filepath.Join(directory, entry.Name()), "candidate archive"); err != nil {
			return nil, err
		}
		candidates = append(candidates, entry.Name())
	}
	slices.Sort(candidates)
	return candidates, nil
}
func validateLocalAICandidateArtifact(directory string, artifact localAICandidateArtifact, requireWindowsArchive bool) (localAICandidateArtifactEvidence, error) {
	if !isLocalAICandidateBasename(artifact.File) {
		return localAICandidateArtifactEvidence{}, fmt.Errorf("artifact file %q must be a basename", artifact.File)
	}
	path := filepath.Join(directory, filepath.FromSlash(artifact.File))
	info, err := validateLocalAICandidatePath(path, fmt.Sprintf("artifact %s", artifact.Role))
	if err != nil {
		return localAICandidateArtifactEvidence{}, err
	}
	if artifact.Bytes == nil || info.Size() != *artifact.Bytes {
		wantBytes := int64(0)
		if artifact.Bytes != nil {
			wantBytes = *artifact.Bytes
		}
		return localAICandidateArtifactEvidence{}, fmt.Errorf("artifact %s byte count = %d, want %d", artifact.File, info.Size(), wantBytes)
	}
	actualSHA256, err := sha256File(path)
	if err != nil {
		return localAICandidateArtifactEvidence{}, fmt.Errorf("hash %s: %w", artifact.File, err)
	}
	if actualSHA256 != artifact.SHA256 {
		return localAICandidateArtifactEvidence{}, fmt.Errorf("artifact %s sha256 mismatch: manifest=%s actual=%s", artifact.File, artifact.SHA256, actualSHA256)
	}
	if requireWindowsArchive {
		if err := validateLocalAICandidateArchive(path); err != nil {
			return localAICandidateArtifactEvidence{}, err
		}
	}
	return localAICandidateArtifactEvidence{Role: artifact.Role, File: artifact.File, Bytes: info.Size(), SHA256: actualSHA256}, nil
}
func validateLocalAICandidateArchive(path string) error {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", filepath.Base(path), err)
	}
	defer archive.Close()
	entries := 0
	for _, entry := range archive.File {
		if entry.Name != "you.exe" || entry.FileInfo().IsDir() {
			continue
		}
		entries++
	}
	if entries != 1 {
		return fmt.Errorf("archive %s contains %d regular you.exe entries, want exactly 1", filepath.Base(path), entries)
	}
	return nil
}
func validateLocalAICandidateDetachedDigest(directory, expected string) error {
	digestPath := filepath.Join(directory, "candidate-manifest.sha256")
	if _, err := validateLocalAICandidatePath(digestPath, "detached candidate manifest digest"); err != nil {
		return err
	}
	contents, err := os.ReadFile(digestPath)
	if err != nil {
		return fmt.Errorf("read detached candidate manifest digest: %w", err)
	}
	fields := strings.Fields(string(contents))
	if len(fields) == 0 || len(fields) > 2 || !localAICandidateSHA256Pattern.MatchString(fields[0]) || fields[0] != expected {
		return fmt.Errorf("detached candidate manifest digest does not match manifest: expected %s", expected)
	}
	if len(fields) == 2 && filepath.Base(filepath.FromSlash(fields[1])) != "candidate-manifest.json" {
		return fmt.Errorf("detached candidate manifest digest names %q, want candidate-manifest.json", fields[1])
	}
	return nil
}
func validateLocalAICandidatePath(path, role string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %s %q: %w", role, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s %q is a symlink/reparse point; candidate paths must be regular in-tree entries", role, path)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve %s %q: %w", role, path, err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("resolve %s %q for reparse validation: %w", role, path, err)
	}
	absolutePath, resolvedPath = filepath.Clean(absolutePath), filepath.Clean(resolvedPath)
	pathsEqual := absolutePath == resolvedPath
	if runtime.GOOS == "windows" {
		pathsEqual = strings.EqualFold(absolutePath, resolvedPath)
	}
	if !pathsEqual {
		return nil, fmt.Errorf("%s %q resolves through a symlink/reparse point to %q; candidate paths must stay in place", role, path, resolvedPath)
	}
	if runtime.GOOS == "windows" && !info.IsDir() && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %q is a Windows symlink/reparse point or unsupported filesystem entry", role, path)
	}
	if info.IsDir() {
		return info, nil
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %q is not a regular file", role, path)
	}
	return info, nil
}
func validateLocalAICandidatePreconditions(taskPath string, roots []localAICandidateRoot) error {
	if localAICandidatePathResolvesYou(taskPath) {
		return errors.New("task PATH already resolves you; remove ambient you before installation")
	}
	seen := make(map[string]string, len(roots))
	for _, root := range roots {
		if strings.TrimSpace(root.Name) == "" {
			return errors.New("declared precondition root has an empty name")
		}
		if !filepath.IsAbs(root.Path) {
			return fmt.Errorf("declared root %q path %q is not absolute", root.Name, root.Path)
		}
		key := strings.ToLower(filepath.Clean(root.Path))
		if previous, exists := seen[key]; exists {
			return fmt.Errorf("declared roots %q and %q refer to the same path %q", previous, root.Name, root.Path)
		}
		seen[key] = root.Name
		info, err := os.Lstat(root.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect declared root %q: %w", root.Name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("declared root %q is a symlink; it must be absent or empty without following ambient state", root.Name)
		}
		if info.IsDir() {
			entries, readErr := os.ReadDir(root.Path)
			if readErr != nil {
				return fmt.Errorf("inspect declared root %q: %w", root.Name, readErr)
			}
			if len(entries) != 0 {
				return fmt.Errorf("declared root %q is not empty", root.Name)
			}
			continue
		}
		if info.Mode().IsRegular() && info.Size() == 0 {
			continue
		}
		return fmt.Errorf("declared root %q is not absent or empty", root.Name)
	}
	return nil
}
func localAICandidatePathResolvesYou(pathValue string) bool {
	for _, directory := range strings.Split(pathValue, string(filepath.ListSeparator)) {
		directory = strings.TrimSpace(directory)
		if directory == "" {
			continue
		}
		for _, executable := range []string{"you.exe", "you.cmd", "you.bat", "you.com", "you"} {
			if info, err := os.Stat(filepath.Join(directory, executable)); err == nil && info.Mode().IsRegular() {
				return true
			}
		}
	}
	return false
}
func isLocalAICandidateBasename(value string) bool {
	return value != "" && value == filepath.Base(filepath.FromSlash(value)) && !strings.ContainsAny(value, `/\\`)
}
func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type localAICandidateFixture struct {
	root         string
	manifestPath string
	archiveBytes []byte
	manifest     localAICandidateManifest
	roots        []localAICandidateRoot
	taskPath     string
}

func newLocalAICandidateFixture(t *testing.T) *localAICandidateFixture {
	t.Helper()
	root := t.TempDir()
	archiveBytes := localAICandidateZip(t)
	archiveName := "you_1.2.3-snapshot-test_windows_amd64.zip"
	archivePath := filepath.Join(root, archiveName)
	if err := os.WriteFile(archivePath, archiveBytes, 0o600); err != nil {
		t.Fatalf("write candidate archive: %v", err)
	}
	installerBytes := []byte("Write-Output 'candidate installer'\n")
	installerPath := filepath.Join(root, "install.ps1")
	if err := os.WriteFile(installerPath, installerBytes, 0o600); err != nil {
		t.Fatalf("write candidate installer: %v", err)
	}
	executableBytes := []byte("prebuilt-you-executable")
	executablePath := filepath.Join(root, "you.exe")
	if err := os.WriteFile(executablePath, executableBytes, 0o600); err != nil {
		t.Fatalf("write candidate executable: %v", err)
	}
	cgoDisabled := false
	archiveSize := int64(len(archiveBytes))
	installerSize := int64(len(installerBytes))
	executableSize := int64(len(executableBytes))
	manifest := localAICandidateManifest{
		SchemaVersion: localAICandidateSchemaVersion,
		Project:       localAICandidateProject,
		Cycle:         localAICandidateCycle,
		Source:        localAICandidateSource{Repository: localAICandidateRepository, Commit: localAICandidateSourceCommit, Tree: localAICandidateSourceTree},
		Build: localAICandidateBuild{
			CandidateVersion: "1.2.3-snapshot-test", CLIVersion: "1.2.3-snapshot-test",
			GoVersion: localAICandidateGoVersion, GoReleaserVersion: localAICandidateGoReleaser,
			GoReleaserConfigSHA256: strings.Repeat("c", sha256.Size*2),
			Target: localAICandidateTarget{
				GOOS: localAICandidateGOOS, GOARCH: localAICandidateGOARCH,
				CgoEnabled: &cgoDisabled,
			},
			Environment: map[string]string{"GOPROXY": "file:///C:/cache/download", "GOSUMDB": "off", "GOTOOLCHAIN": "go1.26.8", "npm_config_offline": "true", "GOFLAGS": localAICandidateGOFLAGS, "GOMAXPROCS": localAICandidateGOMAXPROCS},
		},
		Artifacts: []localAICandidateArtifact{
			{Role: "windows-amd64-archive", File: archiveName, Bytes: &archiveSize, SHA256: localAICandidateSHA256(t, archivePath)},
			{Role: "windows-installer", File: "install.ps1", Bytes: &installerSize, SHA256: localAICandidateSHA256(t, installerPath)},
			{Role: "windows-amd64-executable", File: "you.exe", Bytes: &executableSize, SHA256: localAICandidateSHA256(t, executablePath)},
		},
		Limits: localAICandidateLimits{
			TemporaryDiskBytesMaximum:     candidateInt64Pointer(localAICandidateTemporaryDiskMaximum),
			OrdinaryToolDownloadMaximum:   candidateInt64Pointer(localAICandidateToolDownloadMaximum),
			ModelBackendDownloadMaximum:   candidateInt64Pointer(localAICandidateModelDownloadMaximum),
			ModelCallsMaximum:             candidateInt64Pointer(localAICandidateModelCallsMaximum),
			PaidUSDMaximum:                candidateInt64Pointer(localAICandidatePaidUSDMaximum),
			DescendantMaximum:             candidateInt64Pointer(localAICandidateDescendantMaximum),
			PackagingOrSmokeRerunsMaximum: candidateInt64Pointer(localAICandidateRerunsMaximum),
			GOFLAGS:                       localAICandidateGOFLAGS, GOMAXPROCS: localAICandidateGOMAXPROCS,
		},
		Attempts: localAICandidateAttempts{
			PriorCycles:          []string{"030", "040", "042", "045", "048", "050", "063", "067", "068"},
			Cycle063BuildMaximum: candidateInt64Pointer(1), Cycle063BuildUsed: candidateInt64Pointer(1),
			Cycle067BuildMaximum: candidateInt64Pointer(2), Cycle067BuildUsed: candidateInt64Pointer(2),
			Cycle068BuildMaximum: candidateInt64Pointer(1), Cycle068BuildUsed: candidateInt64Pointer(1),
			Cycle070BuildMaximum: candidateInt64Pointer(1), Cycle070BuildUsed: candidateInt64Pointer(1),
			Cycle076BuildMaximum: candidateInt64Pointer(1), Cycle076BuildUsed: candidateInt64Pointer(1),
		},
	}
	roots := make([]localAICandidateRoot, 0, 8)
	for _, name := range []string{"HOME", "USERPROFILE", "install", "config", "state", "models", "backend", "HF"} {
		pathName := strings.Replace(strings.ToLower(name), "userprofile", "profile", 1)
		roots = append(roots, localAICandidateRoot{Name: name, Path: filepath.Join(root, pathName)})
	}
	fixture := &localAICandidateFixture{
		root:         root,
		manifestPath: filepath.Join(root, "candidate-manifest.json"),
		archiveBytes: archiveBytes,
		manifest:     manifest,
		roots:        roots,
		taskPath:     "",
	}
	fixture.writeManifest(t)
	return fixture
}
func (fixture *localAICandidateFixture) writeManifest(t *testing.T) {
	t.Helper()
	manifestBytes, err := json.MarshalIndent(fixture.manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal candidate fixture manifest: %v", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.WriteFile(fixture.manifestPath, manifestBytes, 0o600); err != nil {
		t.Fatalf("write candidate fixture manifest: %v", err)
	}
	digest := sha256.Sum256(manifestBytes)
	digestPath := filepath.Join(fixture.root, "candidate-manifest.sha256")
	if err := os.WriteFile(digestPath, []byte(hex.EncodeToString(digest[:])+"  candidate-manifest.json\n"), 0o600); err != nil {
		t.Fatalf("write candidate fixture manifest digest: %v", err)
	}
}
func localAICandidateZip(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, contents := range map[string][]byte{
		"you.exe":   []byte("prebuilt-you-executable"),
		"README.md": []byte("candidate documentation"),
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create candidate ZIP entry %q: %v", name, err)
		}
		if _, err := entry.Write(contents); err != nil {
			t.Fatalf("write candidate ZIP entry %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close candidate ZIP: %v", err)
	}
	return buffer.Bytes()
}
func localAICandidateSHA256(t *testing.T, path string) string {
	t.Helper()
	digest, err := sha256File(path)
	if err != nil {
		t.Fatalf("hash candidate fixture %q: %v", path, err)
	}
	return digest
}
func localAICandidateCleanBuildInfo() *debug.BuildInfo {
	return &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: localAICandidateSourceCommit},
		{Key: "vcs.modified", Value: "false"},
	}}
}
func candidateInt64Pointer(value int64) *int64 { return &value }
