//go:build windows

package models_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	localAIOMNIIntegrationArtifactEnv = "INFINITE_YOU_PREBUILT_ARTIFACT"
	localAIOMNIIntegrationRequireEnv  = "INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT"
	localAIOMNIIntegrationSchema      = "localai.windows-omni-diagnostic-evidence.v1"
	localAIOMNIIntegrationNetwork     = "loopback-only-external-denied"
	localAIOMNIIntegrationSelector    = "prebuilt-help"
	localAIOMNIIntegrationHelpFact    = "Run and manage CPN-based workflow factories"
	localAIOMNIIntegrationTimeout     = 30 * time.Second
)

type localAIOMNIIntegrationEvidence struct {
	Schema         string                           `json:"schema"`
	EvidenceKind   string                           `json:"evidenceKind"`
	RunID          string                           `json:"runId"`
	Selector       string                           `json:"selector"`
	Status         string                           `json:"status"`
	Platform       string                           `json:"platform"`
	Architecture   string                           `json:"architecture"`
	Redacted       bool                             `json:"redacted"`
	ManifestSHA256 string                           `json:"manifestSha256"`
	Input          localAIOMNIIntegrationInput      `json:"inputIdentity"`
	Policy         localAIOMNIIntegrationPolicy     `json:"policy"`
	Command        localAIOMNIIntegrationCommand    `json:"command"`
	Logs           localAIOMNIIntegrationLogs       `json:"logs"`
	Artifacts      []localAIOMNIIntegrationArtifact `json:"artifacts"`
	Cache          localAIOMNIIntegrationCache      `json:"cache"`
	Semantic       localAIOMNIIntegrationSemantic   `json:"semantic"`
	Release        localAIOMNIIntegrationRelease    `json:"release"`
	Failure        *localAIOMNIIntegrationFailure   `json:"failure,omitempty"`
	Activity       localAIOMNIIntegrationActivity   `json:"activity"`
}

type localAIOMNIIntegrationInput struct {
	CLISHA256       string `json:"cliSha256"`
	CLIBytes        int64  `json:"cliBytes"`
	ModelIdentity   string `json:"modelIdentity"`
	BackendIdentity string `json:"backendIdentity"`
}

type localAIOMNIIntegrationPolicy struct {
	RootIdentities    []string `json:"rootIdentities"`
	Host              string   `json:"host"`
	Port              int      `json:"port"`
	TimeoutSeconds    int      `json:"timeoutSeconds"`
	ProcessLimit      int      `json:"processLimit"`
	DownloadByteLimit int64    `json:"downloadByteLimit"`
	ModelCallLimit    int64    `json:"modelCallLimit"`
	NetworkPolicy     string   `json:"networkPolicy"`
}

type localAIOMNIIntegrationCommand struct {
	Arguments       []string                     `json:"arguments"`
	SecretsRedacted bool                         `json:"secretsRedacted"`
	Controlled      bool                         `json:"controlled"`
	Stdout          localAIOMNIIntegrationStream `json:"stdout"`
	Stderr          localAIOMNIIntegrationStream `json:"stderr"`
}

type localAIOMNIIntegrationStream struct {
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
	Excerpt   string `json:"excerpt"`
	Truncated bool   `json:"truncated"`
}

type localAIOMNIIntegrationLogs struct {
	Backend []localAIOMNIIntegrationLog `json:"backend"`
	Runtime []localAIOMNIIntegrationLog `json:"runtime"`
}

type localAIOMNIIntegrationLog struct {
	localAIOMNIIntegrationStream
	PathIdentity string `json:"pathIdentity"`
}

type localAIOMNIIntegrationArtifact struct {
	Kind      string `json:"kind"`
	Path      string `json:"pathIdentity"`
	MediaType string `json:"mediaType"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}

type localAIOMNIIntegrationCache struct {
	BeforeSHA256          string `json:"beforeSha256"`
	AfterSHA256           string `json:"afterSha256"`
	BeforeEntries         int    `json:"beforeEntries"`
	AfterEntries          int    `json:"afterEntries"`
	BeforeFiles           int    `json:"beforeFiles"`
	AfterFiles            int    `json:"afterFiles"`
	BeforeBytes           int64  `json:"beforeBytes"`
	AfterBytes            int64  `json:"afterBytes"`
	PartialArtifacts      int    `json:"partialArtifacts"`
	AfterInspectionFailed bool   `json:"afterInspectionFailed"`
}

type localAIOMNIIntegrationSemantic struct {
	Assertion string `json:"assertion"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
	Passed    bool   `json:"passed"`
}

type localAIOMNIIntegrationRelease struct {
	Checked           bool `json:"checked"`
	InspectionFailed  bool `json:"inspectionFailed"`
	ProcessTreeClosed bool `json:"processTreeClosed"`
	OwnedProcesses    int  `json:"ownedProcesses"`
	OwnedListeners    int  `json:"ownedListeners"`
	OwnedLeases       int  `json:"ownedLeases"`
	PartialArtifacts  int  `json:"partialArtifacts"`
}

type localAIOMNIIntegrationFailure struct {
	Owner     string `json:"owner"`
	Assertion string `json:"assertion"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
}

type localAIOMNIIntegrationActivity struct {
	BackendProcesses int   `json:"backendProcesses"`
	ExternalRequests int   `json:"externalRequests"`
	Downloads        int64 `json:"downloads"`
	ModelCalls       int64 `json:"modelCalls"`
}

type localAIOMNIIntegrationRun struct {
	Evidence               localAIOMNIIntegrationEvidence
	Roots                  localAIRealRoots
	Artifact               localAIFileIdentity
	ArtifactObservation    localAICommandObservation
	EnvironmentObservation localAICommandObservation
}

// TestLocalAIOMNIRealDiagnosticRunnerControlled is deliberately small. The
// exhaustive OMNI admission, semantic, ledger, redaction, cancellation and
// final-inspection matrix belongs to internal/omni_diag. This integration
// proof owns only the irreducible compiled-artifact and Windows process edges.
func TestLocalAIOMNIRealDiagnosticRunnerControlled(t *testing.T) {
	artifactPath := localAIOMNIIntegrationRequiredArtifact(t)
	t.Run("invalid-artifact-fails-before-fixtures-or-process", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		missing := filepath.Join(root, "missing", "you.exe")
		executor := &localAIOMNIIntegrationCountingExecutor{}
		_, err := runLocalAIOMNIIntegration(t.Context(), missing, root, filepath.Join(root, "report.json"), executor)
		if err == nil {
			t.Fatal("missing prebuilt artifact was accepted")
		}
		if executor.calls != 0 {
			t.Fatalf("invalid artifact launched %d child process(es)", executor.calls)
		}
		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			t.Fatalf("read invalid-artifact root: %v", readErr)
		}
		if len(entries) != 0 {
			t.Fatalf("invalid artifact created fixture/process state: %#v", entries)
		}
	})
	t.Run("valid-prebuilt-artifact-isolated-and-clean", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		reportPath := filepath.Join(root, "evidence", "report.json")
		ctx, cancel := context.WithTimeout(t.Context(), localAIOMNIIntegrationTimeout)
		defer cancel()
		result, err := runLocalAIOMNIIntegration(ctx, artifactPath, root, reportPath, localAIProcessExecutor{})
		if err != nil {
			t.Fatalf("prebuilt artifact integration: %v", err)
		}
		if result.Evidence.Status != "PASS" || !result.Evidence.Semantic.Passed {
			t.Fatalf("integration evidence = %#v, want PASS", result.Evidence)
		}
		if result.Evidence.Input.CLISHA256 != result.Artifact.SHA256 || result.Evidence.Input.CLIBytes != result.Artifact.Bytes {
			t.Fatalf("artifact identity = %#v, want %#v", result.Evidence.Input, result.Artifact)
		}
		assertLocalAIOMNIIntegrationObservation(t, result.ArtifactObservation, "prebuilt artifact")
		assertLocalAIOMNIIntegrationObservation(t, result.EnvironmentObservation, "environment probe")
		for _, value := range []string{result.Evidence.Policy.RootIdentities[0], result.Evidence.Policy.RootIdentities[1], result.Evidence.Policy.RootIdentities[2]} {
			if !strings.HasPrefix(value, "sha256=") {
				t.Fatalf("root identity = %q, want redacted identity", value)
			}
		}
		if !strings.Contains(string(result.EnvironmentObservation.Stdout), result.Roots.Profile) || !strings.Contains(string(result.EnvironmentObservation.Stdout), result.Roots.Temp) || !strings.Contains(string(result.EnvironmentObservation.Stdout), result.Roots.Work) {
			t.Fatalf("environment probe = %q, want isolated HOME, TEMP and working directory", result.EnvironmentObservation.Stdout)
		}
		body, err := os.ReadFile(reportPath)
		if err != nil {
			t.Fatalf("read canonical evidence: %v", err)
		}
		var decoded localAIOMNIIntegrationEvidence
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode canonical evidence: %v", err)
		}
		if decoded.Status != "PASS" || decoded.Command.Stdout.Truncated || decoded.Command.Stderr.Truncated || decoded.Cache.AfterInspectionFailed || !decoded.Release.Checked || !decoded.Release.ProcessTreeClosed || decoded.Release.OwnedProcesses != 0 || decoded.Release.OwnedListeners != 0 || decoded.Release.OwnedLeases != 0 || decoded.Activity != (localAIOMNIIntegrationActivity{}) {
			t.Fatalf("canonical evidence omitted bounded zero-real cleanup: %#v", decoded)
		}
		if len(decoded.Logs.Backend) != 1 || len(decoded.Logs.Runtime) != 1 || len(decoded.Artifacts) < 2 {
			t.Fatalf("canonical evidence omitted bounded logs/artifacts: %#v", decoded)
		}
		if bytes.Contains(body, []byte(artifactPath)) || bytes.Contains(body, []byte(root)) {
			t.Fatalf("canonical evidence leaked an owned path: %s", body)
		}
	})
}

func localAIOMNIIntegrationRequiredArtifact(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(localAIOMNIIntegrationArtifactEnv))
	if path == "" {
		if os.Getenv(localAIOMNIIntegrationRequireEnv) == "1" {
			t.Fatalf("required compiled-artifact integration evidence did not receive %s", localAIOMNIIntegrationArtifactEnv)
		}
		t.Skipf("OMNI integration requires a pinned prebuilt artifact; set %s to its path", localAIOMNIIntegrationArtifactEnv)
	}
	if _, err := localAIOMNIIntegrationArtifactIdentity(path); err != nil {
		t.Fatalf("invalid pinned OMNI artifact %q: %v", path, err)
	}
	return path
}

func localAIOMNIIntegrationArtifactIdentity(path string) (localAIFileIdentity, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return localAIFileIdentity{}, errors.New("prebuilt artifact path is not absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return localAIFileIdentity{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 {
		return localAIFileIdentity{}, errors.New("prebuilt artifact is not a nonempty regular file")
	}
	identity, ok := localAIReadFileIdentity(path)
	if !ok || identity.Bytes != info.Size() || identity.SHA256 == "" {
		return localAIFileIdentity{}, errors.New("prebuilt artifact identity could not be read")
	}
	return identity, nil
}

func runLocalAIOMNIIntegration(ctx context.Context, artifactPath, root, reportPath string, executor localAIRealCommandExecutor) (localAIOMNIIntegrationRun, error) {
	identity, err := localAIOMNIIntegrationArtifactIdentity(artifactPath)
	if err != nil {
		return localAIOMNIIntegrationRun{}, fmt.Errorf("artifact admission: %w", err)
	}
	if strings.TrimSpace(root) == "" || !filepath.IsAbs(root) || strings.TrimSpace(reportPath) == "" || !filepath.IsAbs(reportPath) {
		return localAIOMNIIntegrationRun{}, errors.New("integration roots and report path must be absolute")
	}
	if executor == nil {
		return localAIOMNIIntegrationRun{}, errors.New("integration executor is required")
	}
	roots := localAIRealRootsFor(root)
	if err := prepareLocalAIRoots(roots); err != nil {
		return localAIOMNIIntegrationRun{}, fmt.Errorf("prepare isolated roots: %w", err)
	}
	before, err := localAIRealCacheSnapshotForRoots(roots)
	if err != nil {
		return localAIOMNIIntegrationRun{}, fmt.Errorf("inspect initial cache: %w", err)
	}
	manifestBody, err := json.Marshal(struct {
		Schema, Selector, CLI, NetworkPolicy string
		Roots                                []string
		Arguments                            []string
	}{localAIOMNIIntegrationSchema, localAIOMNIIntegrationSelector, identity.SHA256, localAIOMNIIntegrationNetwork, []string{pathIdentityHash(roots.Work), pathIdentityHash(roots.Profile), pathIdentityHash(roots.Cache), pathIdentityHash(roots.HFHome), pathIdentityHash(roots.HFCache), pathIdentityHash(roots.Temp), pathIdentityHash(roots.Output), pathIdentityHash(roots.Streams)}, []string{"--help"}})
	if err != nil {
		return localAIOMNIIntegrationRun{}, fmt.Errorf("encode immutable manifest: %w", err)
	}
	manifestSHA := sha256Hex(manifestBody)
	command := localAICommandSpec{BinaryPath: artifactPath, Arguments: []string{"--help"}, Environment: localAIProcessEnvironment(roots, []string{"HTTP_PROXY=127.0.0.1:9", "HTTPS_PROXY=127.0.0.1:9", "NO_PROXY=127.0.0.1"})}
	artifactObservation := executor.Execute(ctx, command, roots)
	environmentObservation := localAIOMNIIntegrationEnvironmentProbe(ctx, roots, executor)
	after, afterErr := localAIRealCacheSnapshotForRoots(roots)
	evidence := localAIOMNIIntegrationEvidence{Schema: localAIOMNIIntegrationSchema, EvidenceKind: "controlled", RunID: "prebuilt-" + manifestSHA[:12], Selector: localAIOMNIIntegrationSelector, Status: "PASS", Platform: runtime.GOOS, Architecture: runtime.GOARCH, Redacted: true, ManifestSHA256: manifestSHA, Input: localAIOMNIIntegrationInput{CLISHA256: identity.SHA256, CLIBytes: identity.Bytes, ModelIdentity: "not-started", BackendIdentity: "not-started"}, Policy: localAIOMNIIntegrationPolicy{RootIdentities: []string{pathIdentityHash(roots.Work), pathIdentityHash(roots.Profile), pathIdentityHash(roots.Cache), pathIdentityHash(roots.HFHome), pathIdentityHash(roots.HFCache), pathIdentityHash(roots.Temp), pathIdentityHash(roots.Output), pathIdentityHash(roots.Streams)}, Host: "127.0.0.1", Port: 0, TimeoutSeconds: int(localAIOMNIIntegrationTimeout / time.Second), ProcessLimit: 2, DownloadByteLimit: 0, ModelCallLimit: 0, NetworkPolicy: localAIOMNIIntegrationNetwork}, Command: localAIOMNIIntegrationCommand{Arguments: []string{"--help"}, SecretsRedacted: true, Controlled: true, Stdout: localAIOMNIIntegrationCapture(artifactObservation.Stdout, artifactObservation.StdoutTruncated), Stderr: localAIOMNIIntegrationCapture(artifactObservation.Stderr, artifactObservation.StderrTruncated)}, Logs: localAIOMNIIntegrationLogs{Backend: localAIOMNIIntegrationLogsFor("backend-observer"), Runtime: localAIOMNIIntegrationLogsFor("runtime-observer")}, Artifacts: []localAIOMNIIntegrationArtifact{{Kind: "cli-stdout", Path: pathIdentityHash("cli-stdout"), MediaType: "text/plain", Bytes: int64(len(artifactObservation.Stdout)), SHA256: sha256Hex(artifactObservation.Stdout)}, {Kind: "environment-probe", Path: pathIdentityHash("environment-probe"), MediaType: "text/plain", Bytes: int64(len(environmentObservation.Stdout)), SHA256: sha256Hex(environmentObservation.Stdout)}}, Cache: localAIOMNIIntegrationCache{BeforeSHA256: before.IdentitySHA256, BeforeEntries: before.Entries, BeforeFiles: before.Files, BeforeBytes: before.Bytes}, Semantic: localAIOMNIIntegrationSemantic{Assertion: "compiled CLI help boundary", Expected: localAIOMNIIntegrationHelpFact, Observed: "stdout=" + sha256Hex(artifactObservation.Stdout)}, Release: localAIOMNIIntegrationRelease{Checked: true, ProcessTreeClosed: artifactObservation.ProcessTreeClosed && environmentObservation.ProcessTreeClosed}}
	evidence.Cache.AfterSHA256, evidence.Cache.AfterEntries, evidence.Cache.AfterFiles, evidence.Cache.AfterBytes, evidence.Cache.PartialArtifacts = after.IdentitySHA256, after.Entries, after.Files, after.Bytes, after.PartialArtifacts
	evidence.Release.OwnedProcesses = artifactObservation.OwnedProcesses + environmentObservation.OwnedProcesses
	evidence.Release.OwnedListeners = artifactObservation.OwnedListeners + environmentObservation.OwnedListeners
	evidence.Release.OwnedLeases = artifactObservation.OwnedLeases + environmentObservation.OwnedLeases
	evidence.Release.PartialArtifacts = after.PartialArtifacts
	if failure := localAIOMNIIntegrationFailureFor(artifactObservation, environmentObservation, afterErr, roots); failure != nil {
		evidence.Status, evidence.Semantic.Passed, evidence.Failure = failure.Status, false, &failure.Failure
	} else {
		evidence.Semantic.Passed = true
	}
	evidence.Release.InspectionFailed = afterErr != nil
	evidence.Cache.AfterInspectionFailed = afterErr != nil
	if evidence.Status == "PASS" && (evidence.Release.OwnedProcesses != 0 || evidence.Release.OwnedListeners != 0 || evidence.Release.OwnedLeases != 0 || evidence.Release.PartialArtifacts != 0 || !evidence.Release.ProcessTreeClosed) {
		evidence.Status, evidence.Semantic.Passed, evidence.Failure = "FAIL", false, &localAIOMNIIntegrationFailure{Owner: "harness", Assertion: "owned-resource cleanup", Expected: "all process-tree and partial-artifact counts are zero", Observed: "owned resource cleanup was not proven"}
	}
	body, err := json.Marshal(evidence)
	if err != nil {
		return localAIOMNIIntegrationRun{}, fmt.Errorf("encode integration evidence: %w", err)
	}
	if bytes.Contains(body, []byte(artifactPath)) || bytes.Contains(body, []byte(root)) {
		return localAIOMNIIntegrationRun{}, errors.New("integration evidence contains an unredacted owned path")
	}
	if err := writeLocalAIJSONAtomic(reportPath, evidence, nil); err != nil {
		return localAIOMNIIntegrationRun{}, fmt.Errorf("write integration evidence: %w", err)
	}
	return localAIOMNIIntegrationRun{Evidence: evidence, Roots: roots, Artifact: identity, ArtifactObservation: artifactObservation, EnvironmentObservation: environmentObservation}, nil
}

type localAIOMNIIntegrationFailureResult struct {
	Status  string
	Failure localAIOMNIIntegrationFailure
}

func localAIOMNIIntegrationFailureFor(artifact, environment localAICommandObservation, afterErr error, roots localAIRealRoots) *localAIOMNIIntegrationFailureResult {
	if afterErr != nil {
		return &localAIOMNIIntegrationFailureResult{"INCONCLUSIVE", localAIOMNIIntegrationFailure{"harness", "final cache and release inspection", "owned roots are observable", "cache inspection failed"}}
	}
	checks := []struct {
		bad                                          bool
		status, owner, assertion, expected, observed string
	}{
		{!artifact.Started, "FAIL", "environment", "artifact process start", "selected artifact starts", "prebuilt artifact did not start"},
		{artifact.TimedOut, "FAIL", "harness", "artifact timeout", "artifact exits within timeout", "prebuilt artifact timed out"},
		{!artifact.ProcessTreeAttached || !artifact.ProcessExited || !artifact.ProcessTreeClosed, "FAIL", "harness", "artifact process cleanup", "artifact process attaches, exits and its tree closes", "artifact process cleanup was not proven"},
		{artifact.StdoutTruncated || artifact.StderrTruncated, "FAIL", "harness", "bounded artifact streams", "streams fit the bound", "artifact command output exceeded the bound"},
		{localAIOMNIIntegrationStreamViolation(artifact.Stdout, artifact.Stderr) != "", "FAIL", "harness", "redacted artifact streams", "sensitive output is absent", "sensitive activity markers were observed"},
		{artifact.ExitCode != 0, "FAIL", "product", "compiled artifact exit status", "compiled artifact help exits with code zero", fmt.Sprintf("artifact exit code=%d", artifact.ExitCode)},
		{!strings.Contains(string(artifact.Stdout), localAIOMNIIntegrationHelpFact), "FAIL", "product", "compiled artifact help output", localAIOMNIIntegrationHelpFact, "compiled artifact help fact was not observed"},
		{!environment.Started || !environment.ProcessTreeAttached || !environment.ProcessExited || !environment.ProcessTreeClosed || environment.ExitCode != 0, "FAIL", "harness", "environment probe cleanup", "isolated environment probe attaches, exits with code zero and closes", "environment probe cleanup was not proven"},
		{environment.StdoutTruncated || environment.StderrTruncated, "FAIL", "harness", "bounded environment probe streams", "streams fit the bound", "environment probe output exceeded the bound"},
		{!strings.Contains(string(environment.Stdout), roots.Profile) || !strings.Contains(string(environment.Stdout), roots.Temp) || !strings.Contains(string(environment.Stdout), roots.Work), "FAIL", "harness", "isolated environment observation", "child observes isolated HOME, TEMP and working directory", "environment probe observed an unexpected root"},
	}
	for _, check := range checks {
		if check.bad {
			return &localAIOMNIIntegrationFailureResult{check.status, localAIOMNIIntegrationFailure{check.owner, check.assertion, check.expected, check.observed}}
		}
	}
	return nil
}

func localAIOMNIIntegrationEnvironmentProbe(ctx context.Context, roots localAIRealRoots, executor localAIRealCommandExecutor) localAICommandObservation {
	shell, err := exec.LookPath("cmd.exe")
	if err != nil {
		return localAICommandObservation{ExitCode: -1}
	}
	command := localAICommandSpec{BinaryPath: shell, Arguments: []string{"/D", "/C", "echo HOME=%HOME%&echo TEMP=%TEMP%&echo WORK=%CD%"}, Environment: localAIProcessEnvironment(roots, []string{"HTTP_PROXY=127.0.0.1:9", "HTTPS_PROXY=127.0.0.1:9", "NO_PROXY=127.0.0.1"})}
	return executor.Execute(ctx, command, roots)
}

func localAIOMNIIntegrationCapture(body []byte, truncated bool) localAIOMNIIntegrationStream {
	if len(body) > localAIRealMaxStreamBytes {
		body = body[:localAIRealMaxStreamBytes]
		truncated = true
	}
	return localAIOMNIIntegrationStream{Bytes: int64(len(body)), SHA256: sha256Hex(body), Excerpt: "sha256=" + sha256Hex(body), Truncated: truncated}
}

func localAIOMNIIntegrationStreamViolation(stdout, stderr []byte) string {
	value := strings.ToLower(string(append(append([]byte(nil), stdout...), stderr...)))
	for _, marker := range []string{
		"hf_token=", "authorization:", "bearer ", "password=", "api_key=", "access_token=",
		"x-amz-signature=", "signed_url=",
	} {
		if strings.Contains(value, marker) {
			return "forbidden sensitive stream marker"
		}
	}
	return ""
}

func localAIOMNIIntegrationLogsFor(label string) []localAIOMNIIntegrationLog {
	body := []byte("controlled prebuilt artifact " + label + ": no LocalAI backend or model started")
	return []localAIOMNIIntegrationLog{{localAIOMNIIntegrationStream: localAIOMNIIntegrationCapture(body, false), PathIdentity: pathIdentityHash(label)}}
}

func assertLocalAIOMNIIntegrationObservation(t *testing.T, observation localAICommandObservation, label string) {
	t.Helper()
	if !observation.Started || !observation.ProcessExited || observation.ExitCode != 0 || !observation.ProcessTreeAttached || !observation.ProcessTreeClosed || observation.OwnedProcesses != 0 || observation.OwnedListeners != 0 || observation.OwnedLeases != 0 || observation.StdoutTruncated || observation.StderrTruncated {
		t.Fatalf("%s observation = %#v, want bounded exited clean process", label, observation)
	}
}

type localAIOMNIIntegrationCountingExecutor struct{ calls int }

func (executor *localAIOMNIIntegrationCountingExecutor) Execute(context.Context, localAICommandSpec, localAIRealRoots) localAICommandObservation {
	executor.calls++
	return localAICommandObservation{}
}
