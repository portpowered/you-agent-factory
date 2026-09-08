package release_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
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
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	localAICandidateManifestEnv                 = "INFINITE_YOU_LOCALAI_CANDIDATE_MANIFEST"
	localAICandidateSchemaVersion               = "localai-windows-install-candidate/v1"
	localAICandidateProject                     = "localai"
	localAICandidateCycle                       = "045"
	localAICandidateRepository                  = "https://github.com/portpowered/you-agent-factory"
	localAICandidateCommit                      = "a9c41aade845c8f09047a11b2e5a3abf41f0f9e9"
	localAICandidateTree                        = "623dcd01569ccac5776bcf15ffe9fb6a58324b24"
	localAICandidatePullRequest                 = 2556
	localAICandidateMergedHead                  = "1d45c19416774ea2aaffe0cae695102cf2a17a18"
	localAICandidateGoReleaser                  = "v2.12.7"
	localAICandidateGOOS                        = "windows"
	localAICandidateGOARCH                      = "amd64"
	localAICandidateTemporaryDiskMaximum  int64 = 4294967296
	localAICandidateToolDownloadMaximum   int64 = 0
	localAICandidateModelDownloadMaximum  int64 = 0
	localAICandidateModelCallsMaximum     int64 = 0
	localAICandidatePaidUSDMaximum        int64 = 0
	localAICandidateChildProcessesMaximum int64 = 12
	localAICandidateRerunsMaximum         int64 = 1
)

var localAICandidateSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// localAICandidateManifest is the validation-only v1 input for the exact
// Windows release cell. It intentionally lives with the release test because
// it is not product configuration or a public API contract.
type localAICandidateManifest struct {
	SchemaVersion string                     `json:"schemaVersion"`
	Project       string                     `json:"project"`
	Cycle         string                     `json:"cycle"`
	Source        localAICandidateSource     `json:"source"`
	Build         localAICandidateBuild      `json:"build"`
	Artifacts     []localAICandidateArtifact `json:"artifacts"`
	Limits        localAICandidateLimits     `json:"limits"`
}
type localAICandidateSource struct {
	Repository            string `json:"repository"`
	Commit                string `json:"commit"`
	Tree                  string `json:"tree"`
	MergedPullRequest     int    `json:"mergedPullRequest"`
	MergedPullRequestHead string `json:"mergedPullRequestHead"`
}
type localAICandidateBuild struct {
	CandidateVersion       string                 `json:"candidateVersion"`
	CLIVersion             string                 `json:"cliVersion"`
	GoVersion              string                 `json:"goVersion"`
	GoReleaserVersion      string                 `json:"goreleaserVersion"`
	GoReleaserConfigSHA256 string                 `json:"goreleaserConfigSha256"`
	Target                 localAICandidateTarget `json:"target"`
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
	ChildProcessesMaximum         *int64 `json:"childProcessesMaximum"`
	PackagingOrSmokeRerunsMaximum *int64 `json:"packagingOrSmokeRerunsMaximum"`
}
type localAICandidateArtifactEvidence struct {
	Role   string `json:"role"`
	File   string `json:"file"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type localAICandidateValidationEvidence struct {
	Status           string                           `json:"status"`
	Property         string                           `json:"property"`
	SchemaVersion    string                           `json:"schemaVersion"`
	SourceCommit     string                           `json:"sourceCommit"`
	SourceTree       string                           `json:"sourceTree"`
	CandidateVersion string                           `json:"candidateVersion"`
	CLIVersion       string                           `json:"cliVersion"`
	Target           string                           `json:"target"`
	Archive          localAICandidateArtifactEvidence `json:"archive"`
	Installer        localAICandidateArtifactEvidence `json:"installer"`
	Manifest         localAICandidateArtifactEvidence `json:"manifest"`
	Preconditions    string                           `json:"preconditions"`
}
type localAICandidateRoot struct {
	Name string
	Path string
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
	pwsh, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatalf("candidate release smoke requires Windows PowerShell (powershell.exe): %v", err)
	}
	repoRoot := testutil.MustRepoRoot(t)
	installDir := filepath.Join(t.TempDir(), "installed-bin")
	reportPath := filepath.Join(t.TempDir(), "install-validation-report.json")
	command := exec.Command(
		pwsh,
		"-NoProfile",
		"-File",
		filepath.Join(repoRoot, "scripts", "release", "smoke-install.ps1"),
		"-CandidateManifestPath",
		manifestPath,
		"-InstallDir",
		installDir,
		"-ReportPath",
		reportPath,
	)
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
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
	pwsh, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatalf("candidate release smoke requires Windows PowerShell (powershell.exe): %v", err)
	}
	repoRoot := testutil.MustRepoRoot(t)
	installDir := filepath.Join(t.TempDir(), "installed-bin")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatalf("create nonempty install directory: %v", err)
	}
	markerPath := filepath.Join(installDir, "ambient-marker.txt")
	if err := os.WriteFile(markerPath, []byte("must remain untouched"), 0o600); err != nil {
		t.Fatalf("write install precondition marker: %v", err)
	}
	reportPath := filepath.Join(t.TempDir(), "install-validation-report.json")
	command := exec.Command(
		pwsh,
		"-NoProfile",
		"-File",
		filepath.Join(repoRoot, "scripts", "release", "smoke-install.ps1"),
		"-CandidateManifestPath",
		manifestPath,
		"-InstallDir",
		installDir,
		"-ReportPath",
		reportPath,
	)
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
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
func TestLocalAICandidateManifestValidationRejectsDriftAndAmbiguity(t *testing.T) {
	t.Parallel()
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
			name: "historical cycle",
			edit: func(fixture *localAICandidateFixture) {
				fixture.manifest.Cycle = "030"
				fixture.writeManifest(t)
			},
			want: `candidate cycle = "030", want "045"`,
		},
		{
			name: "failed cycle",
			edit: func(fixture *localAICandidateFixture) {
				fixture.manifest.Cycle = "040"
				fixture.writeManifest(t)
			},
			want: `candidate cycle = "040", want "045"`,
		},
		{
			name: "contaminated dependency cycle",
			edit: func(fixture *localAICandidateFixture) {
				fixture.manifest.Cycle = "042"
				fixture.writeManifest(t)
			},
			want: `candidate cycle = "042", want "045"`,
		},
		{
			name: "ordinary download allowance",
			edit: func(fixture *localAICandidateFixture) {
				fixture.manifest.Limits.OrdinaryToolDownloadMaximum = candidateInt64Pointer(536870912)
				fixture.writeManifest(t)
			},
			want: "candidate limits.ordinaryToolDownloadBytesMaximum = 536870912, want 0",
		},
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
	t.Parallel()
	fixture := newLocalAICandidateFixture(t)
	evidence, err := validateLocalAICandidate(fixture.manifestPath)
	if err != nil {
		t.Fatalf("validate matching candidate: %v", err)
	}
	if evidence.Status != "PASS" || evidence.Property == "" {
		t.Fatalf("candidate evidence = %#v, want PASS with named property", evidence)
	}
	if evidence.Archive.File != "you_1.2.3-snapshot-test_windows_amd64.zip" || evidence.Archive.Bytes != int64(len(fixture.archiveBytes)) {
		t.Fatalf("archive evidence = %#v, want fixture identity", evidence.Archive)
	}
	if evidence.Installer.File != "install.ps1" || evidence.Installer.Bytes <= 0 {
		t.Fatalf("installer evidence = %#v, want nonempty install.ps1", evidence.Installer)
	}
}
func TestLocalAICandidatePreconditionsFailClosedBeforeInstallation(t *testing.T) {
	t.Parallel()
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
}
func TestLocalAICandidatePreconditionsAcceptAbsentAndEmptyRoots(t *testing.T) {
	t.Parallel()
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
func validateLocalAICandidate(manifestPath string) (localAICandidateValidationEvidence, error) {
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
	archiveName := fmt.Sprintf("you_%s_windows_amd64.zip", manifest.Build.CandidateVersion)
	if archive.File != archiveName {
		return localAICandidateValidationEvidence{}, fmt.Errorf("windows archive filename %q does not match candidate version; want %q", archive.File, archiveName)
	}
	if installer.File != "install.ps1" {
		return localAICandidateValidationEvidence{}, fmt.Errorf("windows installer filename %q does not match required install.ps1", installer.File)
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
	return localAICandidateValidationEvidence{
		Status:           "PASS",
		Property:         "prebuilt-candidate-schema-source-target-artifact-and-detached-digest-identity",
		SchemaVersion:    manifest.SchemaVersion,
		SourceCommit:     manifest.Source.Commit,
		SourceTree:       manifest.Source.Tree,
		CandidateVersion: manifest.Build.CandidateVersion,
		CLIVersion:       manifest.Build.CLIVersion,
		Target:           manifest.Build.Target.GOOS + "/" + manifest.Build.Target.GOARCH,
		Archive:          archiveEvidence,
		Installer:        installerEvidence,
		Manifest: localAICandidateArtifactEvidence{
			Role:   "candidate-manifest",
			File:   filepath.Base(manifestPath),
			Bytes:  manifestInfo.Size(),
			SHA256: manifestDigest,
		},
		Preconditions: "not-run-by-schema-cell; install cell must supply task-owned PATH and roots",
	}, nil
}

func readLocalAICandidateManifest(manifestPath string) ([]byte, localAICandidateManifest, error) {
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, localAICandidateManifest{}, fmt.Errorf("read candidate manifest %q: %w", manifestPath, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	var manifest localAICandidateManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, localAICandidateManifest{}, fmt.Errorf("decode candidate manifest %q: %w", manifestPath, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, localAICandidateManifest{}, fmt.Errorf("decode candidate manifest %q: trailing JSON value", manifestPath)
		}
		return nil, localAICandidateManifest{}, fmt.Errorf("decode candidate manifest %q: trailing data: %w", manifestPath, err)
	}
	return manifestBytes, manifest, nil
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
		manifest.Source.Commit != localAICandidateCommit ||
		manifest.Source.Tree != localAICandidateTree ||
		manifest.Source.MergedPullRequest != localAICandidatePullRequest ||
		manifest.Source.MergedPullRequestHead != localAICandidateMergedHead {
		return fmt.Errorf("candidate source identity does not match exact merged LocalAI base: repository=%q commit=%q tree=%q mergedPullRequest=%d mergedPullRequestHead=%q", manifest.Source.Repository, manifest.Source.Commit, manifest.Source.Tree, manifest.Source.MergedPullRequest, manifest.Source.MergedPullRequestHead)
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
		return fmt.Errorf("candidate target.cgoEnabled = %v, want false", candidateBoolValue(manifest.Build.Target.CgoEnabled))
	}
	if len(manifest.Artifacts) != 2 {
		return fmt.Errorf("candidate artifacts count = %d, want exactly 2", len(manifest.Artifacts))
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.Role != "windows-amd64-archive" && artifact.Role != "windows-installer" {
			return fmt.Errorf("candidate artifact role %q is not one of windows-amd64-archive or windows-installer", artifact.Role)
		}
		if !isLocalAICandidateBasename(artifact.File) {
			return fmt.Errorf("candidate artifact %q has unsafe or non-basename file %q", artifact.Role, artifact.File)
		}
		if artifact.Bytes == nil || *artifact.Bytes <= 0 {
			return fmt.Errorf("candidate artifact %q bytes must be positive", artifact.Role)
		}
		if !localAICandidateSHA256Pattern.MatchString(artifact.SHA256) {
			return fmt.Errorf("candidate artifact %q sha256 = %q, want lowercase SHA-256", artifact.Role, artifact.SHA256)
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
		{"childProcessesMaximum", manifest.Limits.ChildProcessesMaximum, localAICandidateChildProcessesMaximum},
		{"packagingOrSmokeRerunsMaximum", manifest.Limits.PackagingOrSmokeRerunsMaximum, localAICandidateRerunsMaximum},
	} {
		if err := validateLocalAICandidateLimit(limit.name, limit.actual, limit.want); err != nil {
			return err
		}
	}
	return nil
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
		return localAICandidateArtifactEvidence{}, fmt.Errorf("artifact %s byte count = %d, want %d", artifact.File, info.Size(), candidateInt64Value(artifact.Bytes))
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
	for _, entry := range archive.File {
		if entry.Name != "you.exe" || entry.FileInfo().IsDir() {
			continue
		}
		return nil
	}
	return fmt.Errorf("archive %s does not contain you.exe", filepath.Base(path))
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
	if !localAICandidatePathsEqual(absolutePath, resolvedPath) {
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

func localAICandidatePathsEqual(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
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
				return fmt.Errorf("declared root %q is not empty: %s", root.Name, entryNames(entries))
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

func entryNames(entries []os.DirEntry) string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
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

func candidateBoolValue(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func candidateInt64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
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
	cgoDisabled := false
	archiveSize := int64(len(archiveBytes))
	installerSize := int64(len(installerBytes))
	manifest := localAICandidateManifest{
		SchemaVersion: localAICandidateSchemaVersion,
		Project:       localAICandidateProject,
		Cycle:         localAICandidateCycle,
		Source: localAICandidateSource{
			Repository:            localAICandidateRepository,
			Commit:                localAICandidateCommit,
			Tree:                  localAICandidateTree,
			MergedPullRequest:     localAICandidatePullRequest,
			MergedPullRequestHead: localAICandidateMergedHead,
		},
		Build: localAICandidateBuild{
			CandidateVersion:       "1.2.3-snapshot-test",
			CLIVersion:             "1.2.3-snapshot-test",
			GoVersion:              "go1.25.0",
			GoReleaserVersion:      localAICandidateGoReleaser,
			GoReleaserConfigSHA256: strings.Repeat("c", sha256.Size*2),
			Target: localAICandidateTarget{
				GOOS:       localAICandidateGOOS,
				GOARCH:     localAICandidateGOARCH,
				CgoEnabled: &cgoDisabled,
			},
		},
		Artifacts: []localAICandidateArtifact{
			{Role: "windows-amd64-archive", File: archiveName, Bytes: &archiveSize, SHA256: localAICandidateSHA256(t, archivePath)},
			{Role: "windows-installer", File: "install.ps1", Bytes: &installerSize, SHA256: localAICandidateSHA256(t, installerPath)},
		},
		Limits: localAICandidateLimits{
			TemporaryDiskBytesMaximum:     candidateInt64Pointer(localAICandidateTemporaryDiskMaximum),
			OrdinaryToolDownloadMaximum:   candidateInt64Pointer(localAICandidateToolDownloadMaximum),
			ModelBackendDownloadMaximum:   candidateInt64Pointer(localAICandidateModelDownloadMaximum),
			ModelCallsMaximum:             candidateInt64Pointer(localAICandidateModelCallsMaximum),
			PaidUSDMaximum:                candidateInt64Pointer(localAICandidatePaidUSDMaximum),
			ChildProcessesMaximum:         candidateInt64Pointer(localAICandidateChildProcessesMaximum),
			PackagingOrSmokeRerunsMaximum: candidateInt64Pointer(localAICandidateRerunsMaximum),
		},
	}
	fixture := &localAICandidateFixture{
		root:         root,
		manifestPath: filepath.Join(root, "candidate-manifest.json"),
		archiveBytes: archiveBytes,
		manifest:     manifest,
		roots: []localAICandidateRoot{
			{Name: "HOME", Path: filepath.Join(root, "home")},
			{Name: "USERPROFILE", Path: filepath.Join(root, "profile")},
			{Name: "install", Path: filepath.Join(root, "install")},
			{Name: "config", Path: filepath.Join(root, "config")},
			{Name: "state", Path: filepath.Join(root, "state")},
			{Name: "models", Path: filepath.Join(root, "models")},
			{Name: "backend", Path: filepath.Join(root, "backend")},
			{Name: "HF", Path: filepath.Join(root, "hf")},
		},
		taskPath: "",
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

func candidateInt64Pointer(value int64) *int64 {
	return &value
}
