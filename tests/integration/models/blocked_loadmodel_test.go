//go:build managed_process_integration

package models_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/platform/process/managedchild"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	appwire "github.com/portpowered/infinite-you/pkg/wire"
)

const (
	blockedReadinessHelperEnv    = "LOCALAI_READINESS_HELPER"
	blockedReadinessHelperSHAEnv = "LOCALAI_READINESS_HELPER_SHA256"
	blockedReadinessHeadEnv      = "LOCALAI_READINESS_SOURCE_HEAD"
	blockedRuntimeEvidenceEnv    = "INFINITE_YOU_INTEGRATION_MODEL_RUNTIME_EVIDENCE"
	blockedReadinessModel        = "OMNIVOICE_Q4_K_M"
	blockedReadinessBudget       = 30 * time.Second
	blockedReadinessMaximum      = 35 * time.Second
	blockedReadinessWait         = 5 * time.Second
)

type blockedReadinessEvent struct {
	Kind         string `json:"kind"`
	Method       string `json:"method,omitempty"`
	PID          int    `json:"pid"`
	Endpoint     string `json:"endpoint"`
	AtUTC        string `json:"at_utc"`
	RequestBytes int    `json:"request_bytes,omitempty"`
	HasDeadline  bool   `json:"has_deadline"`
	DeadlineUTC  string `json:"deadline_utc,omitempty"`
}

type blockedReadinessPreflight struct {
	SourceHead            string                 `json:"source_head"`
	ASRMerge              string                 `json:"asr_merge"`
	Platform              string                 `json:"platform"`
	GoVersion             string                 `json:"go_version"`
	Fixture               string                 `json:"fixture"`
	HelperPath            string                 `json:"helper_path"`
	HelperSHA256          string                 `json:"helper_sha256"`
	Endpoint              string                 `json:"endpoint"`
	Port                  string                 `json:"port"`
	ParentPID             int                    `json:"parent_pid"`
	ProcessTree           []blockedReadinessNode `json:"process_tree"`
	ReadinessBudgetSecond int                    `json:"readiness_budget_seconds"`
	MaximumSeconds        int                    `json:"maximum_seconds"`
	DownloadBudget        int                    `json:"model_backend_downloads"`
	PaidCalls             int                    `json:"paid_calls"`
	AttemptBudget         int                    `json:"attempts"`
	RetryBudget           int                    `json:"retries"`
	FixtureWaitSeconds    int                    `json:"fixture_wait_seconds"`
	ReadinessCleanupSecs  int                    `json:"readiness_cleanup_seconds"`
	TotalWitnessMaxSecs   int                    `json:"ensure_start_to_return_maximum_seconds"`
	NetworkPolicy         string                 `json:"network_policy"`
	StartedAtUTC          string                 `json:"started_at_utc"`
	PeerReadyAtUTC        string                 `json:"peer_ready_at_utc,omitempty"`
}

type blockedReadinessNode struct {
	Role      string `json:"role"`
	PID       int    `json:"pid,omitempty"`
	ParentPID int    `json:"parent_pid"`
	Image     string `json:"image"`
}

type blockedReadinessRuntimeEvidence struct {
	Sequence       uint64 `json:"sequence"`
	Kind           string `json:"kind"`
	Stage          string `json:"stage,omitempty"`
	Outcome        string `json:"outcome"`
	FailureClass   string `json:"failure_class,omitempty"`
	DurationMillis int64  `json:"duration_millis"`
}

type blockedReadinessOutcome struct {
	result models.EnsureModelHostResult
	err    error
	at     time.Time
}

type blockedReadinessHarness struct {
	helper          string
	helperSHA       string
	sourceHead      string
	root            string
	eventsPath      string
	runtimeEvidence string
	service         models.Service
	scope           models.RuntimeScopeRef
	launcher        *blockedReadinessProcessLauncher
	watcher         *fsnotify.Watcher
	ensureCh        chan blockedReadinessOutcome
	ensureStartedAt time.Time
	loadEvent       blockedReadinessEvent
	loadObservedAt  time.Time
}

type blockedReadinessFixture struct {
	root                string
	cacheDirectory      string
	port                string
	endpoint            string
	eventsPath          string
	readyPath           string
	preflightPath       string
	runtimeEvidencePath string
	preflight           blockedReadinessPreflight
}

func TestBlockedLoadModelStopsOwnedProcess(t *testing.T) {
	witness := newBlockedReadinessHarness(t)
	startBlockedReadinessCall(t, witness)
	assertBlockedReadinessCompletion(t, witness)
}

func newBlockedReadinessHarness(t *testing.T) *blockedReadinessHarness {
	helper, helperSHA := requireBlockedReadinessHelper(t)
	sourceHead := strings.TrimSpace(os.Getenv(blockedReadinessHeadEnv))
	if len(sourceHead) != 40 {
		t.Fatalf("%s must contain the exact 40-character source HEAD", blockedReadinessHeadEnv)
	}
	if _, err := hex.DecodeString(sourceHead); err != nil {
		t.Fatalf("%s is not a hexadecimal source HEAD: %v", blockedReadinessHeadEnv, err)
	}
	fixture := newBlockedReadinessFixture(t, helper, helperSHA, sourceHead)
	t.Setenv(blockedRuntimeEvidenceEnv, fixture.runtimeEvidencePath)
	t.Setenv("HF_HUB_OFFLINE", "1")
	t.Setenv("LOCALAI_OFFLINE", "1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	launcher := &blockedReadinessProcessLauncher{
		preflight: fixture.preflight, preflightPath: fixture.preflightPath,
		readyPath: fixture.readyPath,
	}
	service, err := appwire.NewModelsServiceForManagedProcessIntegration(serviceedges.Edges{
		ModelInvocationGRPCDialer: platformgrpc.NetworkDialer{},
		ModelHostProcessLauncher:  launcher,
	})
	if err != nil {
		t.Fatalf("construct production Models service: %v", err)
	}
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: blockedReadinessRuntimeConfig(fixture.cacheDirectory, helper, fixture.endpoint, fixture.port, fixture.eventsPath, fixture.readyPath),
	})
	if err != nil {
		t.Fatalf("open isolated Models runtime scope: %v", err)
	}
	witness := &blockedReadinessHarness{
		helper: helper, helperSHA: helperSHA, sourceHead: sourceHead,
		root: fixture.root, eventsPath: fixture.eventsPath,
		runtimeEvidence: fixture.runtimeEvidencePath, service: service,
		scope: opened.Scope, launcher: launcher,
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), blockedReadinessWait)
		defer cancel()
		_, _ = service.StopModelHost(cleanupCtx, models.StopModelHostRequest{Scope: opened.Scope, Name: blockedReadinessModel})
		_, _ = service.CloseRuntimeScope(cleanupCtx, models.CloseRuntimeScopeRequest{Scope: opened.Scope})
		if closer, ok := service.(interface{ Close(context.Context) error }); ok {
			_ = closer.Close(cleanupCtx)
		}
		if child := launcher.processSnapshot(); child != nil {
			_ = child.Stop(cleanupCtx)
		}
	})
	return witness
}

func newBlockedReadinessFixture(
	t *testing.T,
	helper string,
	helperSHA string,
	sourceHead string,
) blockedReadinessFixture {
	t.Helper()
	root := t.TempDir()
	cacheDirectory := filepath.Join(root, "cache")
	writeBlockedReadinessCache(t, cacheDirectory)
	port, err := reserveBlockedReadinessAddress()
	if err != nil {
		t.Fatalf("reserve OS-assigned loopback port: %v", err)
	}
	fixture := blockedReadinessFixture{
		root: root, cacheDirectory: cacheDirectory,
		port: port, endpoint: "grpc://" + port,
		eventsPath:          filepath.Join(root, "peer-events.jsonl"),
		readyPath:           filepath.Join(root, "peer-ready.json"),
		preflightPath:       filepath.Join(root, "preflight.json"),
		runtimeEvidencePath: filepath.Join(root, "runtime-evidence.jsonl"),
	}
	fixture.preflight = blockedReadinessPreflight{
		SourceHead: sourceHead, ASRMerge: "8f945b0eadcf9863821585c7f5edeb84d855f471",
		Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(),
		Fixture: "blocked-host-helper-v1", HelperPath: helper, HelperSHA256: helperSHA,
		Endpoint: fixture.endpoint, Port: port, ParentPID: os.Getpid(),
		ProcessTree: []blockedReadinessNode{
			{Role: "models-integration-test", PID: os.Getpid(), ParentPID: os.Getppid(), Image: os.Args[0]},
			{Role: "owned-blocked-host-helper", ParentPID: os.Getpid(), Image: helper},
		},
		ReadinessBudgetSecond: int(blockedReadinessBudget / time.Second),
		MaximumSeconds:        int(blockedReadinessMaximum / time.Second),
		DownloadBudget:        0, PaidCalls: 0, AttemptBudget: 1, RetryBudget: 0,
		FixtureWaitSeconds:   int(blockedReadinessWait / time.Second),
		ReadinessCleanupSecs: int(blockedReadinessWait / time.Second),
		TotalWitnessMaxSecs:  int((blockedReadinessMaximum + blockedReadinessWait) / time.Second),
		NetworkPolicy:        "loopback only; Go module proxy disabled by invoking command",
		StartedAtUTC:         time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := writeBlockedReadinessPreflight(fixture.preflightPath, fixture.preflight); err != nil {
		t.Fatalf("write controlled witness preflight: %v", err)
	}
	return fixture
}

func blockedReadinessRuntimeConfig(
	cacheDirectory string,
	helper string,
	endpoint string,
	port string,
	eventsPath string,
	readyPath string,
) models.RuntimeScopeConfig {
	return models.RuntimeScopeConfig{
		CacheDirectory: cacheDirectory,
		Runtime: models.RuntimeConfig{
			Workers: []models.RuntimeWorker{{
				Name: "blocked-readiness-worker", Type: models.RuntimeWorkerTypeInference,
				Model: blockedReadinessModel, ModelLocality: models.RuntimeModelLocalityLocal,
				Command: helper,
				Args: []string{"--grpc-endpoint", endpoint, "--fixture-endpoint", port,
					"--events-file", eventsPath, "--ready-file", readyPath},
			}},
			Resources: []models.RuntimeResource{{
				Name: "blocked-readiness-cache", Type: models.RuntimeResourceTypeModel,
				Capacity: 1, Model: blockedReadinessModel, Backend: "localai-llamacpp",
				LoadPolicy: string(models.LoadPolicyOnDemand), Provider: "MODELSCOPE",
			}},
		},
	}
}

func startBlockedReadinessCall(t *testing.T, witness *blockedReadinessHarness) {
	t.Helper()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("create peer evidence watcher: %v", err)
	}
	if err := watcher.Add(witness.root); err != nil {
		_ = watcher.Close()
		t.Fatalf("watch peer evidence directory: %v", err)
	}
	witness.watcher = watcher
	t.Cleanup(func() { _ = watcher.Close() })
	ensureCtx, cancelEnsure := context.WithCancel(context.Background())
	t.Cleanup(cancelEnsure)
	witness.ensureCh = make(chan blockedReadinessOutcome, 1)
	witness.ensureStartedAt = time.Now()
	go func() {
		result, ensureErr := witness.service.EnsureModelHost(ensureCtx, models.EnsureModelHostRequest{
			Scope: witness.scope, Name: blockedReadinessModel,
		})
		witness.ensureCh <- blockedReadinessOutcome{result: result, err: ensureErr, at: time.Now()}
	}()
	witness.loadEvent, witness.loadObservedAt = waitForBlockedReadinessEvent(
		t, watcher, witness.eventsPath, witness.ensureCh, "LOADMODEL_BLOCKED", blockedReadinessWait,
	)
	assertBlockedReadinessPeerDeadline(t, witness)
}

func assertBlockedReadinessPeerDeadline(t *testing.T, witness *blockedReadinessHarness) {
	t.Helper()
	if witness.loadEvent.Method != "/backend.Backend/LoadModel" {
		t.Fatalf("blocked peer event method = %q, want /backend.Backend/LoadModel", witness.loadEvent.Method)
	}
	if !witness.loadEvent.HasDeadline {
		t.Fatal("LoadModel peer context had no deadline; readiness budget did not reach the RPC")
	}
	loadDeadline, err := time.Parse(time.RFC3339Nano, witness.loadEvent.DeadlineUTC)
	if err != nil {
		t.Fatalf("parse LoadModel deadline %q: %v", witness.loadEvent.DeadlineUTC, err)
	}
	if remaining := time.Until(loadDeadline); remaining <= 0 || remaining > blockedReadinessBudget {
		t.Fatalf("LoadModel deadline has remaining budget %s, want (0, %s]", remaining, blockedReadinessBudget)
	}
	childPID := witness.launcher.pid()
	if childPID <= 0 || childPID != witness.loadEvent.PID {
		t.Fatalf("owned child PID = %d, peer LoadModel PID = %d; want same positive process", childPID, witness.loadEvent.PID)
	}
	if !processRunning(childPID) {
		t.Fatalf("owned helper PID %d exited before LoadModel boundary was measured", childPID)
	}
}

func assertBlockedReadinessCompletion(t *testing.T, witness *blockedReadinessHarness) {
	t.Helper()
	timer := time.NewTimer(blockedReadinessMaximum)
	defer timer.Stop()
	var outcome blockedReadinessOutcome
	select {
	case outcome = <-witness.ensureCh:
	case <-timer.C:
		t.Fatal("EnsureModelHost did not return within 35 seconds of blocked LoadModel")
	}
	if !errors.Is(outcome.err, models.ErrHostLoadingTimeout) || errors.Is(outcome.err, models.ErrHostCancelled) {
		t.Fatalf("EnsureModelHost outcome = %#v, error %v; want typed readiness timeout without caller cancellation", outcome.result, outcome.err)
	}
	if outcome.result.Host.ReadinessState == models.ReadinessStateReady {
		t.Fatalf("host became READY after blocked LoadModel: %#v", outcome.result.Host)
	}
	inspect, err := witness.service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: witness.scope, Name: blockedReadinessModel,
	})
	if err != nil {
		t.Fatalf("inspect host after readiness failure: %v", err)
	}
	assertBlockedReadinessStopped(t, witness, inspect.Host.ReadinessState)
	runtimeRecords := readBlockedRuntimeEvidence(t, witness.runtimeEvidence)
	runtimeDuration := blockedReadinessRuntimeDuration(runtimeRecords)
	if runtimeDuration < int64(blockedReadinessBudget/time.Millisecond) {
		t.Fatalf("Runtime Host terminal evidence duration = %d ms, want at least %d ms", runtimeDuration, blockedReadinessBudget/time.Millisecond)
	}
	logBlockedReadinessEvidence(t, witness, outcome, inspect.Host.ReadinessState, runtimeDuration)
}

func assertBlockedReadinessStopped(
	t *testing.T,
	witness *blockedReadinessHarness,
	readiness models.ReadinessState,
) {
	t.Helper()
	if readiness == models.ReadinessStateReady {
		t.Fatalf("Models service published READY after LoadModel failure")
	}
	childPID := witness.launcher.pid()
	childStoppedAt := time.Now()
	if processRunning(childPID) {
		t.Fatalf("Runtime Host returned while owned helper PID %d was still alive", childPID)
	}
	if elapsed := childStoppedAt.Sub(witness.loadObservedAt); elapsed > blockedReadinessMaximum {
		t.Fatalf("owned helper stop observed %s after LoadModel started, budget is %s", elapsed, blockedReadinessMaximum)
	}
	if elapsed := time.Since(witness.loadObservedAt); elapsed > blockedReadinessMaximum {
		t.Fatalf("witness exceeded 35 seconds from LoadModel observation: %s", elapsed)
	}
}

func blockedReadinessRuntimeDuration(records []blockedReadinessRuntimeEvidence) int64 {
	var duration int64
	for _, record := range records {
		if record.DurationMillis > duration {
			duration = record.DurationMillis
		}
	}
	return duration
}

func logBlockedReadinessEvidence(
	t *testing.T,
	witness *blockedReadinessHarness,
	outcome blockedReadinessOutcome,
	readiness models.ReadinessState,
	runtimeDuration int64,
) {
	t.Helper()
	preflight := witness.launcher.preflightSnapshot()
	preflightBytes, err := json.Marshal(preflight)
	if err != nil {
		t.Fatalf("marshal completed preflight: %v", err)
	}
	childStoppedAt := time.Now()
	t.Logf("LOCALAI-READINESS-PREFLIGHT %s", preflightBytes)
	t.Logf("LOCALAI-READINESS-WITNESS source_head=%s helper_sha256=%s health=success load_model=blocked_at:%s peer_deadline=%s ensure_typed_timeout_at:%s external_cancellation=false typed_error=%T ready_state=%s runtime_duration_ms=%d child_pid=%d child_stop_observed_at:%s load_to_stop=%s no_model_or_backend_downloads=true retries=0", witness.sourceHead, witness.helperSHA, witness.loadEvent.AtUTC, witness.loadEvent.DeadlineUTC, outcome.at.UTC().Format(time.RFC3339Nano), outcome.err, readiness, runtimeDuration, witness.launcher.pid(), childStoppedAt.UTC().Format(time.RFC3339Nano), childStoppedAt.Sub(witness.loadObservedAt))
	if elapsed := time.Since(witness.ensureStartedAt); elapsed > blockedReadinessMaximum+blockedReadinessWait {
		t.Fatalf("bounded witness took %s including fixture startup, unexpected", elapsed)
	}
}

func requireBlockedReadinessHelper(t *testing.T) (string, string) {
	t.Helper()
	helper := strings.TrimSpace(os.Getenv(blockedReadinessHelperEnv))
	if helper == "" || !filepath.IsAbs(helper) {
		t.Fatalf("%s must be the absolute path to the build-lane-prebuilt helper", blockedReadinessHelperEnv)
	}
	digest := strings.TrimSpace(os.Getenv(blockedReadinessHelperSHAEnv))
	if digest == "" {
		body, err := os.ReadFile(helper)
		if err != nil {
			t.Fatalf("read blocked-readiness helper %q: %v", helper, err)
		}
		computed := sha256.Sum256(body)
		digest = hex.EncodeToString(computed[:])
	}
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != sha256.Size {
		t.Fatalf("%s must contain the SHA-256 digest of the build-lane-prebuilt helper", blockedReadinessHelperSHAEnv)
	}
	return helper, strings.ToLower(digest)
}

func reserveBlockedReadinessAddress() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", err
	}
	return address, nil
}

func writeBlockedReadinessCache(t *testing.T, cacheDirectory string) {
	t.Helper()
	modelRoot := filepath.Join(cacheDirectory, blockedReadinessModel)
	revisionRoot := filepath.Join(modelRoot, "rev-blocked-readiness")
	if err := os.MkdirAll(revisionRoot, 0o700); err != nil {
		t.Fatalf("create no-download model cache fixture: %v", err)
	}
	files := []struct {
		path   string
		sha256 string
	}{
		{path: "omnivoice-base-Q4_K_M.gguf", sha256: "fixture-base"},
		{path: "omnivoice-tokenizer-Q4_K_M.gguf", sha256: "fixture-tokenizer"},
	}
	for index, file := range files {
		if err := os.WriteFile(filepath.Join(revisionRoot, file.path), []byte{byte(index + 1), byte(index + 2)}, 0o600); err != nil {
			t.Fatalf("write no-download model fixture %q: %v", file.path, err)
		}
	}
	metadata := struct {
		ModelName string `json:"modelName"`
		Revision  string `json:"revision"`
		Files     []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}{ModelName: blockedReadinessModel, Revision: "rev-blocked-readiness"}
	for _, file := range files {
		metadata.Files = append(metadata.Files, struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		}{Path: file.path, SHA256: file.sha256})
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal no-download model cache metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelRoot, ".managed-cache.json"), body, 0o600); err != nil {
		t.Fatalf("write no-download model cache metadata: %v", err)
	}
}

func writeBlockedReadinessPreflight(path string, preflight blockedReadinessPreflight) error {
	body, err := json.Marshal(preflight)
	if err != nil {
		return fmt.Errorf("marshal preflight: %w", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		return fmt.Errorf("write preflight: %w", err)
	}
	return nil
}

func waitForBlockedReadinessEvent(
	t *testing.T,
	watcher *fsnotify.Watcher,
	path string,
	ensure <-chan blockedReadinessOutcome,
	wantKind string,
	timeout time.Duration,
) (blockedReadinessEvent, time.Time) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		events, err := readBlockedReadinessEvents(path)
		if err == nil {
			for _, item := range events {
				if item.Kind == wantKind {
					return item, time.Now()
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read fake LocalAI event evidence: %v", err)
		}
		select {
		case outcome := <-ensure:
			t.Fatalf("EnsureModelHost returned before fake peer emitted %s: result=%#v error=%v; no product conclusion", wantKind, outcome.result, outcome.err)
		case _, ok := <-watcher.Events:
			if !ok {
				t.Fatal("fake LocalAI event watcher closed")
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				t.Fatal("fake LocalAI event watcher error channel closed")
			}
			t.Fatalf("fake LocalAI event watcher: %v", err)
		case <-timer.C:
			t.Fatalf("timed out waiting for fake peer event %s; fixture or process observation failed, no product conclusion", wantKind)
		}
	}
}

func readBlockedReadinessEvents(path string) ([]blockedReadinessEvent, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var events []blockedReadinessEvent
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		var item blockedReadinessEvent
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, fmt.Errorf("decode peer event: %w", err)
		}
		events = append(events, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func readBlockedRuntimeEvidence(t *testing.T, path string) []blockedReadinessRuntimeEvidence {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Models runtime evidence: %v", err)
	}
	var records []blockedReadinessRuntimeEvidence
	decoder := json.NewDecoder(bytes.NewReader(body))
	for {
		var record blockedReadinessRuntimeEvidence
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode Models runtime evidence: %v", err)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		t.Fatal("Models runtime evidence is empty")
	}
	return records
}

type blockedReadinessReadyRecord struct {
	Kind     string `json:"kind"`
	PID      int    `json:"pid"`
	Endpoint string `json:"endpoint"`
	AtUTC    string `json:"at_utc"`
}

type blockedReadinessProcessLauncher struct {
	mu            sync.Mutex
	process       *managedchild.Process
	preflight     blockedReadinessPreflight
	preflightPath string
	readyPath     string
}

func (launcher *blockedReadinessProcessLauncher) Start(
	ctx context.Context,
	spec serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	process, err := managedchild.Start(ctx, managedchild.Spec{
		Command: spec.Command, Args: append([]string(nil), spec.Args...),
		Env: append([]string(nil), spec.Env...), WorkDir: spec.WorkDir,
	})
	if err != nil {
		return nil, fmt.Errorf("start prebuilt blocked-host helper: %w", err)
	}
	if err := waitForBlockedReadinessHelper(ctx, process, launcher.readyPath, blockedReadinessWait); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), blockedReadinessWait)
		defer cancel()
		_ = process.Stop(stopCtx)
		return nil, fmt.Errorf("blocked-host fixture did not become observable: %w", err)
	}
	readyBody, err := os.ReadFile(launcher.readyPath)
	if err != nil {
		stopBlockedReadinessProcess(process)
		return nil, fmt.Errorf("read blocked-host ready evidence: %w", err)
	}
	var ready blockedReadinessReadyRecord
	if err := json.Unmarshal(readyBody, &ready); err != nil {
		stopBlockedReadinessProcess(process)
		return nil, fmt.Errorf("decode blocked-host ready evidence: %w", err)
	}
	wantEndpoint := strings.TrimPrefix(spec.HealthEndpoint, "grpc://")
	if ready.Kind != "LISTENING" || ready.PID != process.PID() || ready.Endpoint != wantEndpoint {
		stopBlockedReadinessProcess(process)
		return nil, fmt.Errorf("blocked-host ready evidence = %#v, want pid %d endpoint %q", ready, process.PID(), wantEndpoint)
	}

	launcher.mu.Lock()
	launcher.process = process
	launcher.preflight.ProcessTree[1].PID = process.PID()
	launcher.preflight.PeerReadyAtUTC = ready.AtUTC
	if err := launcher.preflightBytesLocked(); err != nil {
		launcher.mu.Unlock()
		stopCtx, cancel := context.WithTimeout(context.Background(), blockedReadinessWait)
		defer cancel()
		_ = process.Stop(stopCtx)
		return nil, fmt.Errorf("write peer-ready preflight evidence: %w", err)
	}
	launcher.mu.Unlock()
	return blockedReadinessManagedProcess{process: process, endpoint: spec.HealthEndpoint}, nil
}

func stopBlockedReadinessProcess(process *managedchild.Process) {
	ctx, cancel := context.WithTimeout(context.Background(), blockedReadinessWait)
	defer cancel()
	_ = process.Stop(ctx)
}

func (launcher *blockedReadinessProcessLauncher) preflightBytesLocked() error {
	return writeBlockedReadinessPreflight(launcher.preflightPath, launcher.preflight)
}

func (launcher *blockedReadinessProcessLauncher) processSnapshot() *managedchild.Process {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.process
}

func (launcher *blockedReadinessProcessLauncher) preflightSnapshot() blockedReadinessPreflight {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.preflight
}

func (launcher *blockedReadinessProcessLauncher) pid() int {
	if process := launcher.processSnapshot(); process != nil {
		return process.PID()
	}
	return 0
}

type blockedReadinessManagedProcess struct {
	process  *managedchild.Process
	endpoint string
}

func (process blockedReadinessManagedProcess) HealthEndpoint() string { return process.endpoint }
func (process blockedReadinessManagedProcess) Wait() error            { return process.process.Wait() }
func (process blockedReadinessManagedProcess) Stop(ctx context.Context) error {
	return process.process.Stop(ctx)
}

func waitForBlockedReadinessHelper(
	ctx context.Context,
	process *managedchild.Process,
	readyPath string,
	timeout time.Duration,
) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create helper-ready watcher: %w", err)
	}
	defer watcher.Close()
	if err := watcher.Add(filepath.Dir(readyPath)); err != nil {
		return fmt.Errorf("watch helper-ready directory: %w", err)
	}
	exitCh := make(chan error, 1)
	go func() { exitCh <- process.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if _, err := os.Stat(readyPath); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect helper-ready file: %w", err)
		}
		select {
		case waitErr := <-exitCh:
			return fmt.Errorf("helper exited before serving: %w", waitErr)
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-watcher.Events:
			if !ok {
				return errors.New("helper-ready watcher closed")
			}
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return errors.New("helper-ready watcher error channel closed")
			}
			return fmt.Errorf("helper-ready watcher: %w", watchErr)
		case <-timer.C:
			return errors.New("helper-ready signal deadline expired")
		}
	}
}

func argumentValue(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}
