package omni_media_probe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	prebuiltArtifactEnv         = "INFINITE_YOU_PREBUILT_ARTIFACT"
	prebuiltArtifactRequiredEnv = "INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT"
)

// TestProbeRunnerPreflightConsumesInvokingLaneArtifact is the only integration
// case in this package. The invoking lane owns the build; this test supplies
// that immutable artifact to the production-shaped preflight and verifies the
// compiled-boundary handoff without running a model or backend.
func TestProbeRunnerPreflightConsumesInvokingLaneArtifact(t *testing.T) {
	artifactPath := strings.TrimSpace(os.Getenv(prebuiltArtifactEnv))
	if artifactPath == "" {
		if strings.TrimSpace(os.Getenv(prebuiltArtifactRequiredEnv)) == "" {
			t.Skipf("compiled-artifact preflight is optional outside the integration lane; set %s=1 to require it", prebuiltArtifactRequiredEnv)
		}
		t.Fatalf("compiled-artifact preflight requires %s from the invoking integration lane; no in-test build fallback", prebuiltArtifactEnv)
	}
	if !filepath.IsAbs(artifactPath) {
		t.Fatalf("compiled artifact path = %q, want absolute path", artifactPath)
	}

	input, inputPath, reportPath := validProbeInput(t, "prebuilt-artifact")
	input.Build.Path = artifactPath
	input.Build.Identity = "factory-cli@invoking-lane-build"
	input.Build.SHA256 = fileSHA256(t, artifactPath)
	if err := WriteProbeInputAtomic(inputPath, input); err != nil {
		t.Fatalf("write compiled-artifact probe input: %v", err)
	}

	report, err := NewRunner(nil).Run(context.Background(), inputPath, reportPath)
	if err != nil {
		t.Fatalf("run compiled-artifact preflight: %v", err)
	}
	if report.Status != "READY" || report.Failure != nil {
		t.Fatalf("compiled-artifact preflight report = %#v, want READY without failure", report)
	}
	artifactInfo, err := os.Stat(artifactPath)
	if err != nil {
		t.Fatalf("stat compiled artifact after preflight: %v", err)
	}
	if report.Build.Identity != input.Build.Identity || report.Build.SHA256 != input.Build.SHA256 || report.Build.Bytes != artifactInfo.Size() || report.Build.PathIdentity == artifactPath {
		t.Fatalf("compiled-artifact identity = %#v, want supplied digest, size, label and redacted path", report.Build)
	}
	if report.Dependencies.Model.Identity != input.Dependencies.Model.Identity || report.Dependencies.Projector.Identity != input.Dependencies.Projector.Identity || report.Dependencies.Backend.Identity != input.Dependencies.Backend.Identity || len(report.Fixtures) != 2 || report.Fixtures[0].SHA256 != wantImageSHA256 || report.Fixtures[1].SHA256 != wantVideoSHA256 {
		t.Fatalf("compiled-artifact dependency/fixture identities = %#v/%#v, want supplied controlled dependencies and pinned media", report.Dependencies, report.Fixtures)
	}
	if report.Policy.Port == ProbeForbiddenPort || len(report.Policy.RootIdentities) < 6 || !uniqueStrings(report.Policy.RootIdentities) {
		t.Fatalf("compiled-artifact policy = %#v, want isolated roots and non-7437 port", report.Policy)
	}
	if report.Policy.MaxCompilerTestProcesses != ProbeMaxCompilerTestProcesses || report.Policy.MaxDiskBytes != ProbeMaxDiskBytes {
		t.Fatalf("compiled-artifact declared limits = compiler=%d disk=%d, want compiler=%d disk=%d", report.Policy.MaxCompilerTestProcesses, report.Policy.MaxDiskBytes, ProbeMaxCompilerTestProcesses, ProbeMaxDiskBytes)
	}
	if report.Policy.DownloadBytes != 0 || report.Policy.PaidUSD != 0 || len(report.Processes) != 0 || len(report.Outputs) != 0 || report.Cleanup != (CleanupEvidence{Checked: true}) {
		t.Fatalf("compiled-artifact effects = policy=%#v processes=%#v outputs=%#v cleanup=%#v, want zero external activity and survivors", report.Policy, report.Processes, report.Outputs, report.Cleanup)
	}
	if _, err := ReadReport(reportPath); err != nil {
		t.Fatalf("read compiled-artifact report: %v", err)
	}
	if _, err := os.Stat(input.ProbeRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("compiled-artifact probe root = %v, want removed after preflight", err)
	}
	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read compiled-artifact report bytes: %v", err)
	}
	for _, leaked := range []string{artifactPath, input.ProbeRoot, input.Dependencies.Model.Path, input.Dependencies.Projector.Path, input.Dependencies.Backend.Path, input.FixtureManifest.Path} {
		if strings.Contains(string(body), leaked) {
			t.Fatalf("compiled-artifact report leaked path %q: %s", leaked, body)
		}
	}
	t.Logf("compiled-artifact preflight READY identity=%s sha256=%s bytes=%d port=%d roots=%d", report.Build.Identity, report.Build.SHA256, report.Build.Bytes, report.Policy.Port, len(report.Policy.RootIdentities))
}

func validProbeInput(t *testing.T, runID string) (ProbeInput, string, string) {
	t.Helper()
	root := t.TempDir()
	inputPath := filepath.Join(root, "probe-input.json")
	reportPath := filepath.Join(root, "probe-report.json")
	probeRoot := filepath.Join(root, "probe-root")
	buildName := "controlled-you"
	if runtime.GOOS == "windows" {
		buildName += ".exe"
	}
	buildPath := filepath.Join(root, buildName)
	writeExecutableTestFile(t, buildPath, []byte("controlled prebuilt you artifact\n"))
	modelPath := writeProbeDependency(t, root, "model.bin", "controlled model identity\n")
	projectorPath := writeProbeDependency(t, root, "projector.bin", "controlled projector identity\n")
	backendPath := writeProbeDependency(t, root, "backend.bin", "controlled backend identity\n")
	manifestPath := checkedInManifestPath(t)
	return ProbeInput{
		SchemaVersion: ProbeInputSchemaV1, RunID: runID,
		Build: ProbeBuildIdentity{Path: buildPath, Identity: "controlled-you@ff194dc", SHA256: fileSHA256(t, buildPath)},
		Dependencies: ProbeDependencies{
			Model: writeFileIdentity(t, modelPath, "model@controlled"), Projector: writeFileIdentity(t, projectorPath, "projector@controlled"), Backend: writeFileIdentity(t, backendPath, "backend@controlled"),
		},
		FixtureManifest: writeFileIdentity(t, manifestPath, "omni-fixture-manifest@v1"),
		ProbeRoot:       probeRoot, Journeys: []JourneyName{probeJourneyImage, probeJourneyVideo},
		Limits: ProbeLimits{
			TimeoutSeconds: 5, MaxHeavyProcesses: ProbeMaxHeavyProcesses, MaxCompilerTestProcesses: ProbeMaxCompilerTestProcesses,
			MaxDiskBytes: ProbeMaxDiskBytes, MaxDownloadBytes: 0, MaxPaidUSD: 0, ForbiddenPort: ProbeForbiddenPort, NetworkPolicy: ProbeNetworkPolicy,
		},
	}, inputPath, reportPath
}

func checkedInManifestPath(t testing.TB) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test source")
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "..", "tests", "integration", "models", "testdata", "omni_media", "manifest.json")
}

func writeProbeDependency(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write controlled dependency %s: %v", name, err)
	}
	return path
}

func writeExecutableTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o700); err != nil {
		t.Fatalf("write controlled build: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatalf("chmod controlled build: %v", err)
		}
	}
}

func writeFileIdentity(t *testing.T, path, identity string) ProbeFileIdentity {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat identity file %s: %v", path, err)
	}
	return ProbeFileIdentity{Path: path, Identity: identity, Bytes: info.Size(), SHA256: fileSHA256(t, path)}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read identity file %s: %v", path, err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
