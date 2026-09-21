package models_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	managedRecoveryHelperPathEnv = "INFINITE_YOU_MANAGED_RUNTIME_RECOVERY_HELPER"
	managedRecoveryHelperSHAEnv  = "INFINITE_YOU_MANAGED_RUNTIME_RECOVERY_HELPER_SHA256"
	managedRecoveryHeadSHAEnv    = "INFINITE_YOU_MANAGED_RUNTIME_RECOVERY_HEAD_SHA"
	managedRecoveryRequiredEnv   = "INFINITE_YOU_REQUIRE_MANAGED_RUNTIME_RECOVERY_HELPER"
	managedRecoveryModelName     = "managed-stage-recovery-fixture"
	managedRecoverySignalPhase   = "MANAGED_METADATA_STAGE_WRITTEN"
	managedRecoveryTimeout       = 14 * time.Minute
	managedRecoveryArtifactA     = "weights.gguf"
	managedRecoveryArtifactB     = "projector.bin"
)

var managedRecoveryHeadPattern = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

type managedRecoveryArtifact struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	Body   []byte `json:"-"`
}

type managedRecoveryConfiguration struct {
	Root            string                    `json:"-"`
	CacheDirectory  string                    `json:"cacheDirectory"`
	SourceDirectory string                    `json:"sourceDirectory"`
	SignalPath      string                    `json:"signalPath"`
	ModelName       string                    `json:"modelName"`
	Artifacts       []managedRecoveryArtifact `json:"artifacts"`
}

type managedRecoveryStageSignal struct {
	Phase             string `json:"phase"`
	PID               int    `json:"pid"`
	RevisionStagePath string `json:"revisionStagePath"`
	MetadataStagePath string `json:"metadataStagePath"`
}

type managedRecoveryObservation struct {
	Outcome         string                          `json:"outcome"`
	AssetReadiness  string                          `json:"assetReadiness"`
	AssetIntegrity  string                          `json:"assetIntegrity"`
	Artifacts       []managedRecoveryArtifactResult `json:"artifacts"`
	CatalogStatus   string                          `json:"catalogStatus"`
	Readiness       string                          `json:"readiness"`
	Lifecycle       string                          `json:"lifecycle"`
	CachePath       string                          `json:"cachePath"`
	NetworkRequests int64                           `json:"networkRequests"`
}

type managedRecoveryArtifactResult struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type managedRecoveryProcess struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	done    chan error
	reaped  bool
	stdout  bytes.Buffer
	stderr  bytes.Buffer
}

func TestManagedRuntimeGenericAbruptProcessRecovery(t *testing.T) {
	helper, helperSHA, headSHA := requireManagedRecoveryHelper(t)
	fixture := newManagedRecoveryFixture(t)
	configPath, configSHA := writeManagedRecoveryConfiguration(t, fixture)
	logManagedRecoveryBudget(t, helperSHA, headSHA, configSHA, fixture)

	owner := startManagedRecoveryProcess(t, helper, configPath, fixture.Root, "owner", true)
	t.Cleanup(func() { owner.killAndReap(t) })
	signal := waitForManagedRecoveryStage(t, fixture.SignalPath, owner)
	assertManagedRecoveryOwnerStillStaged(t, owner)
	assertObservedManagedStages(t, signal, fixture)
	owner.killAndReap(t)
	if owner.command.ProcessState == nil || owner.command.ProcessState.Success() {
		t.Fatalf("staged owner process state = %#v, want killed and reaped", owner.command.ProcessState)
	}
	if signal.PID != owner.command.Process.Pid {
		t.Fatalf("managed-stage signal PID = %d, want prebuilt owner PID %d", signal.PID, owner.command.Process.Pid)
	}
	assertObservedManagedStages(t, signal, fixture)
	if err := os.RemoveAll(fixture.SourceDirectory); err != nil {
		t.Fatalf("remove local source before fresh retry: %v", err)
	}
	if _, err := os.Stat(fixture.SourceDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local source after removal has stat error %v, want not-exist", err)
	}

	retry := startManagedRecoveryProcess(t, helper, configPath, fixture.Root, "retry", false)
	t.Cleanup(func() { retry.killAndReap(t) })
	if err := retry.wait(t); err == nil {
		if !retry.command.ProcessState.Success() {
			t.Fatalf("retry process exited unsuccessfully without an error: %s", retry.stderr.String())
		}
	} else {
		t.Fatalf("fresh process retry failed: %v; stdout=%s stderr=%s", err, retry.stdout.String(), retry.stderr.String())
	}
	observation := decodeManagedRecoveryObservation(t, retry.stdout.Bytes())
	assertManagedRecoveryReady(t, observation, fixture, signal)
	assertNoManagedRecoveryResidue(t, fixture.CacheDirectory)
	assertPrebuiltHelperUnchanged(t, helper, helperSHA)
	assertManagedRecoveryProcessesReaped(t, owner, retry)
	t.Logf("INT-RECOVERY-01 head=%s helper_sha256=%s platform=%s/%s roots={cache:%q,source:%q,signal:%q} timeout=15m process_limit=1 network=disabled downloads=0 model_calls=0 paid_calls=0 config_sha256=%s artifacts=%s owner_pid=%d owner_exit_code=%d retry_pid=%d retry_exit=0 survivors=0",
		headSHA, helperSHA, runtime.GOOS, runtime.GOARCH,
		fixture.CacheDirectory, fixture.SourceDirectory, fixture.SignalPath,
		configSHA, managedRecoveryArtifactSummary(fixture.Artifacts),
		owner.command.Process.Pid, owner.command.ProcessState.ExitCode(), retry.command.Process.Pid)
}

func requireManagedRecoveryHelper(t *testing.T) (string, string, string) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(managedRecoveryRequiredEnv)) != "1" {
		t.Skip("managed-runtime process recovery requires the strict prebuilt helper handoff")
	}
	helper := strings.TrimSpace(os.Getenv(managedRecoveryHelperPathEnv))
	wantSHA := strings.TrimSpace(os.Getenv(managedRecoveryHelperSHAEnv))
	headSHA := strings.TrimSpace(os.Getenv(managedRecoveryHeadSHAEnv))
	if helper == "" || wantSHA == "" || !managedRecoveryHeadPattern.MatchString(headSHA) {
		t.Fatalf("strict helper handoff requires %s, %s, and a 40-character %s", managedRecoveryHelperPathEnv, managedRecoveryHelperSHAEnv, managedRecoveryHeadSHAEnv)
	}
	if !filepath.IsAbs(helper) {
		t.Fatalf("prebuilt managed-runtime helper path %q is not absolute", helper)
	}
	info, err := os.Stat(helper)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("prebuilt managed-runtime helper is not a regular file: stat=%v info=%#v", err, info)
	}
	body, err := os.ReadFile(helper)
	if err != nil {
		t.Fatalf("read prebuilt managed-runtime helper: %v", err)
	}
	actualSHA := sha256Hex(body)
	if actualSHA != strings.ToLower(wantSHA) {
		t.Fatalf("prebuilt managed-runtime helper SHA-256 = %s, want %s", actualSHA, wantSHA)
	}
	return helper, actualSHA, strings.ToLower(headSHA)
}

func newManagedRecoveryFixture(t *testing.T) managedRecoveryConfiguration {
	t.Helper()
	root := t.TempDir()
	fixture := managedRecoveryConfiguration{
		Root:            root,
		CacheDirectory:  filepath.Join(root, "cache"),
		SourceDirectory: filepath.Join(root, "source"),
		SignalPath:      filepath.Join(root, "signals", "managed-stage.json"),
		ModelName:       managedRecoveryModelName,
		Artifacts:       managedRecoveryFixtureArtifacts(),
	}
	if err := os.MkdirAll(fixture.SourceDirectory, 0o700); err != nil {
		t.Fatalf("create isolated local model source: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(fixture.SignalPath), 0o700); err != nil {
		t.Fatalf("create isolated signal directory: %v", err)
	}
	for _, artifact := range fixture.Artifacts {
		if err := os.WriteFile(filepath.Join(fixture.SourceDirectory, artifact.Name), artifact.Body, 0o600); err != nil {
			t.Fatalf("write controlled source artifact %q: %v", artifact.Name, err)
		}
	}
	return fixture
}

func managedRecoveryFixtureArtifacts() []managedRecoveryArtifact {
	return []managedRecoveryArtifact{
		managedRecoveryArtifactFor(managedRecoveryArtifactA, []byte("managed recovery weights fixture v1\n")),
		managedRecoveryArtifactFor(managedRecoveryArtifactB, []byte("managed recovery projector fixture v1\n")),
	}
}

func managedRecoveryArtifactFor(name string, body []byte) managedRecoveryArtifact {
	return managedRecoveryArtifact{
		Name: name, Bytes: int64(len(body)), SHA256: sha256Hex(body), Body: body,
	}
}

func writeManagedRecoveryConfiguration(
	t *testing.T,
	fixture managedRecoveryConfiguration,
) (string, string) {
	t.Helper()
	fixtureBody := fixture
	fixtureBody.Artifacts = make([]managedRecoveryArtifact, len(fixture.Artifacts))
	for index, artifact := range fixture.Artifacts {
		fixtureBody.Artifacts[index] = managedRecoveryArtifact{
			Name: artifact.Name, Bytes: artifact.Bytes, SHA256: artifact.SHA256,
		}
	}
	body, err := json.Marshal(fixtureBody)
	if err != nil {
		t.Fatalf("encode controlled recovery configuration: %v", err)
	}
	path := filepath.Join(filepath.Dir(fixture.SourceDirectory), "recovery.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write controlled recovery configuration: %v", err)
	}
	return path, sha256Hex(body)
}

func logManagedRecoveryBudget(
	t *testing.T,
	helperSHA, headSHA, configSHA string,
	fixture managedRecoveryConfiguration,
) {
	t.Helper()
	t.Logf("INT-RECOVERY-01 preflight head=%s helper_sha256=%s platform=%s/%s roots={cache:%q,source:%q,signal:%q} timeout=15m max_helpers=1 network=disabled download_budget=0 model_calls=0 paid_calls=0 retries=1 config_sha256=%s artifacts=%s",
		headSHA, helperSHA, runtime.GOOS, runtime.GOARCH,
		fixture.CacheDirectory, fixture.SourceDirectory, fixture.SignalPath,
		configSHA, managedRecoveryArtifactSummary(fixture.Artifacts))
}

func managedRecoveryArtifactSummary(artifacts []managedRecoveryArtifact) string {
	parts := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		parts = append(parts, fmt.Sprintf("%s:%d:%s", artifact.Name, artifact.Bytes, artifact.SHA256))
	}
	return strings.Join(parts, ",")
}

func startManagedRecoveryProcess(
	t *testing.T,
	helper, configPath, workDir, mode string,
	withStdin bool,
) *managedRecoveryProcess {
	t.Helper()
	command := exec.Command(helper, "--mode", mode, "--config", configPath)
	command.Dir = workDir
	process := &managedRecoveryProcess{command: command, done: make(chan error, 1)}
	command.Stdout = &process.stdout
	command.Stderr = &process.stderr
	if withStdin {
		stdin, err := command.StdinPipe()
		if err != nil {
			t.Fatalf("open staged owner control pipe: %v", err)
		}
		process.stdin = stdin
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start prebuilt recovery helper: %v", err)
	}
	go func() { process.done <- command.Wait() }()
	return process
}

func (process *managedRecoveryProcess) wait(t *testing.T) error {
	t.Helper()
	timer := time.NewTimer(managedRecoveryTimeout)
	defer timer.Stop()
	select {
	case err := <-process.done:
		process.reaped = true
		return err
	case <-timer.C:
		_ = process.killAndReap(t)
		return errors.New("prebuilt recovery helper exceeded the 15-minute safety ceiling")
	}
}

func (process *managedRecoveryProcess) killAndReap(t *testing.T) error {
	t.Helper()
	if process.command.Process == nil || process.reaped {
		return nil
	}
	if process.command.ProcessState == nil {
		if err := process.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill staged recovery helper: %w", err)
		}
	}
	if process.stdin != nil {
		_ = process.stdin.Close()
		process.stdin = nil
	}
	select {
	case err := <-process.done:
		process.reaped = true
		return err
	case <-time.After(15 * time.Second):
		return errors.New("prebuilt recovery helper was not reaped within 15 seconds")
	}
}

func waitForManagedRecoveryStage(
	t *testing.T,
	signalPath string,
	owner *managedRecoveryProcess,
) managedRecoveryStageSignal {
	t.Helper()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("watch managed-stage signal: %v", err)
	}
	defer watcher.Close()
	if err := watcher.Add(filepath.Dir(signalPath)); err != nil {
		t.Fatalf("watch managed-stage signal directory: %v", err)
	}
	timer := time.NewTimer(managedRecoveryTimeout)
	defer timer.Stop()
	for {
		body, readErr := os.ReadFile(signalPath)
		if readErr == nil {
			var signal managedRecoveryStageSignal
			if err := json.Unmarshal(body, &signal); err != nil {
				t.Fatalf("decode managed-stage signal: %v", err)
			}
			return signal
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatalf("read managed-stage signal: %v", readErr)
		}
		select {
		case err := <-owner.done:
			owner.reaped = true
			t.Fatalf("owner exited before managed staging was observed: %v; stderr=%s", err, owner.stderr.String())
		case _, ok := <-watcher.Events:
			if !ok {
				t.Fatal("managed-stage watcher closed before the signal")
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				t.Fatal("managed-stage watcher error stream closed")
			}
			t.Fatalf("managed-stage watcher failed: %v", err)
		case <-timer.C:
			owner.killAndReap(t)
			t.Fatalf("managed-stage signal %q was not observed within the safety ceiling", signalPath)
		}
	}
}

func assertObservedManagedStages(
	t *testing.T,
	signal managedRecoveryStageSignal,
	fixture managedRecoveryConfiguration,
) {
	t.Helper()
	if signal.Phase != managedRecoverySignalPhase || signal.PID <= 0 {
		t.Fatalf("managed-stage signal = %#v, want phase %s and a process PID", signal, managedRecoverySignalPhase)
	}
	if !filepath.IsAbs(signal.RevisionStagePath) || !filepath.IsAbs(signal.MetadataStagePath) {
		t.Fatalf("managed-stage paths are not absolute: %#v", signal)
	}
	if filepath.Base(signal.MetadataStagePath) != ".managed-cache.json.partial" ||
		filepath.Dir(signal.MetadataStagePath) != filepath.Dir(signal.RevisionStagePath) ||
		!strings.HasSuffix(filepath.Base(signal.RevisionStagePath), ".partial") {
		t.Fatalf("observed stale paths are not the exact sibling managed stages: %#v", signal)
	}
	expectedModelRoot := filepath.Join(fixture.CacheDirectory, strings.ToUpper(fixture.ModelName))
	if filepath.Clean(filepath.Dir(signal.MetadataStagePath)) != filepath.Clean(expectedModelRoot) {
		t.Fatalf("observed stages are outside the selected model root %q: %#v", expectedModelRoot, signal)
	}
	if !pathWithin(fixture.CacheDirectory, filepath.Dir(signal.RevisionStagePath)) {
		t.Fatalf("observed managed stage escaped the isolated cache root: %#v", signal)
	}
	if _, err := os.Stat(strings.TrimSuffix(signal.RevisionStagePath, ".partial")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed revision beside staged path has stat error %v, want not-exist", err)
	}
	if _, err := os.Stat(strings.TrimSuffix(signal.MetadataStagePath, ".partial")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed metadata beside staged path has stat error %v, want not-exist", err)
	}
	for _, artifact := range fixture.Artifacts {
		assertFileIdentity(t, filepath.Join(signal.RevisionStagePath, artifact.Name), artifact.Bytes, artifact.SHA256)
	}
	assertFileIdentity(t, signal.MetadataStagePath, -1, "")
}

func decodeManagedRecoveryObservation(t *testing.T, body []byte) managedRecoveryObservation {
	t.Helper()
	var observation managedRecoveryObservation
	if err := json.Unmarshal(bytes.TrimSpace(body), &observation); err != nil {
		t.Fatalf("decode fresh-process recovery result: %v; stdout=%s", err, body)
	}
	return observation
}

func assertManagedRecoveryReady(
	t *testing.T,
	observation managedRecoveryObservation,
	fixture managedRecoveryConfiguration,
	signal managedRecoveryStageSignal,
) {
	t.Helper()
	if observation.Outcome != "PREPARED" || observation.AssetReadiness != "AVAILABLE" ||
		observation.AssetIntegrity != "VERIFIED" || observation.CatalogStatus != "READY" ||
		observation.Readiness != "READY" || observation.Lifecycle != "INSTALLED" ||
		observation.NetworkRequests != 0 {
		t.Fatalf("fresh retry observation = %#v, want verified, offline, installed/ready managed runtime", observation)
	}
	if !filepath.IsAbs(observation.CachePath) ||
		!pathWithin(fixture.CacheDirectory, observation.CachePath) {
		t.Fatalf("fresh retry cache path %q is not inside the isolated cache root", observation.CachePath)
	}
	if filepath.Clean(observation.CachePath) != filepath.Clean(strings.TrimSuffix(signal.RevisionStagePath, ".partial")) {
		t.Fatalf("fresh retry published %q, want the revision from abandoned stage %q", observation.CachePath, signal.RevisionStagePath)
	}
	if len(observation.Artifacts) != len(fixture.Artifacts) {
		t.Fatalf("content-addressed artifacts = %#v, want %d", observation.Artifacts, len(fixture.Artifacts))
	}
	for _, artifact := range fixture.Artifacts {
		assertFileIdentity(t, filepath.Join(observation.CachePath, artifact.Name), artifact.Bytes, artifact.SHA256)
		if !hasManagedRecoveryArtifact(observation.Artifacts, artifact) {
			t.Fatalf("verified input identities %#v do not contain %#v", observation.Artifacts, artifact)
		}
	}
}

func hasManagedRecoveryArtifact(
	observed []managedRecoveryArtifactResult,
	want managedRecoveryArtifact,
) bool {
	for _, artifact := range observed {
		if artifact.Name == want.Name && artifact.Bytes == want.Bytes && artifact.SHA256 == want.SHA256 {
			return true
		}
	}
	return false
}

func assertFileIdentity(t *testing.T, path string, bytes int64, digest string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read expected artifact %q: %v", path, err)
	}
	if bytes >= 0 && int64(len(body)) != bytes {
		t.Fatalf("artifact %q bytes = %d, want %d", path, len(body), bytes)
	}
	if digest != "" && sha256Hex(body) != digest {
		t.Fatalf("artifact %q SHA-256 = %s, want %s", path, sha256Hex(body), digest)
	}
}

func assertNoManagedRecoveryResidue(t *testing.T, cacheDirectory string) {
	t.Helper()
	err := filepath.WalkDir(cacheDirectory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == cacheDirectory {
			return nil
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".partial") || strings.HasSuffix(name, ".previous") {
			return fmt.Errorf("managed recovery left transient path %q", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect recovered cache for transient residue: %v", err)
	}
}

func assertPrebuiltHelperUnchanged(t *testing.T, helper, expectedSHA string) {
	t.Helper()
	body, err := os.ReadFile(helper)
	if err != nil {
		t.Fatalf("reread prebuilt helper after process proof: %v", err)
	}
	if got := sha256Hex(body); got != expectedSHA {
		t.Fatalf("prebuilt helper changed during process proof: SHA-256=%s want=%s", got, expectedSHA)
	}
}

func assertManagedRecoveryProcessesReaped(
	t *testing.T,
	owner, retry *managedRecoveryProcess,
) {
	t.Helper()
	if !owner.reaped || !retry.reaped || owner.command.ProcessState == nil || retry.command.ProcessState == nil {
		t.Fatalf("owned helper process states = owner:%#v retry:%#v, want both reaped", owner.command.ProcessState, retry.command.ProcessState)
	}
}

func assertManagedRecoveryOwnerStillStaged(t *testing.T, owner *managedRecoveryProcess) {
	t.Helper()
	select {
	case err := <-owner.done:
		owner.reaped = true
		t.Fatalf("owner process ended after publishing the stage signal: %v; stderr=%s", err, owner.stderr.String())
	default:
	}
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
