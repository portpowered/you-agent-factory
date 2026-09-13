package wire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	managedchild "github.com/portpowered/infinite-you/pkg/platform/process/managedchild"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	managedbackend "github.com/portpowered/infinite-you/pkg/wire/internal/managedbackend"
)

func TestSystemInitializationInspectPathPreservesOverrideAndSelectsProcessDefault(t *testing.T) {
	t.Parallel()

	path := t.TempDir()
	info, err := provideSystemInitializationInspectPath(serviceedges.Edges{})(path)
	if err != nil {
		t.Fatalf("default inspect path: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("default inspect path IsDir() = false for %q", path)
	}

	inspected := ""
	override := func(path string) (fs.FileInfo, error) {
		inspected = path
		return nil, fs.ErrPermission
	}
	_, err = provideSystemInitializationInspectPath(serviceedges.Edges{
		SystemInitializationInspectPath: override,
	})("customer-path")
	if !errors.Is(err, fs.ErrPermission) || inspected != "customer-path" {
		t.Fatalf("override inspect path = (%q, %v), want customer-path and permission error", inspected, err)
	}
}

func TestModelsManagedProcessRetainsCleanupErrorOnce(t *testing.T) {
	t.Parallel()
	cleanupErr := errors.New("bounded cleanup failure")
	cleanupCalls := 0
	process := &modelsManagedProcess{
		cleanup: func() error {
			cleanupCalls++
			return cleanupErr
		},
		finished: make(chan struct{}),
	}
	close(process.finished)

	process.cleanupResources()
	process.cleanupResources()
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want once", cleanupCalls)
	}
	if err := process.Wait(); !errors.Is(err, cleanupErr) {
		t.Fatalf("process wait error = %v, want retained cleanup error", err)
	}
}

func TestModelsProcessLauncherUsesIndependentChildLifetimeContext(t *testing.T) {
	t.Parallel()
	parentContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	startErr := errors.New("controlled child start failure")
	var childContext context.Context
	launcher := modelsProcessLauncher{
		resolveLaunch: func(context.Context, serviceedges.HostProcessStartSpec) (managedbackend.ManagedBackendLaunch, error) {
			return managedbackend.ManagedBackendLaunch{Command: "controlled-managed-backend"}, nil
		},
		startProcess: func(ctx context.Context, _ managedchild.Spec) (*managedchild.Process, error) {
			childContext = ctx
			return nil, startErr
		},
	}
	if _, err := launcher.Start(parentContext, serviceedges.HostProcessStartSpec{}); !errors.Is(err, startErr) {
		t.Fatalf("modelsProcessLauncher.Start() error = %v, want controlled start error", err)
	}
	cancel()
	if childContext == nil {
		t.Fatal("modelsProcessLauncher did not supply a child context")
	}
	if childContext.Done() != nil || childContext.Err() != nil {
		t.Fatalf("child lifetime context = (done=%v, err=%v), want cancellation-independent context", childContext.Done(), childContext.Err())
	}
}

// TestModelsManagedProcessProductionBoundaryLifecycle is the dedicated local
// real-process I1-I4 lane. It is skipped by ordinary package tests because the
// Make-owned helper identity is intentionally supplied only by the integration
// target.
func TestModelsManagedProcessProductionBoundaryLifecycle(t *testing.T) {
	helper, helperSHA := requireProductionManagedHelper(t)
	t.Run("I1 ready survives context release and reuses", func(t *testing.T) {
		runProductionManagedI1(t, helper, helperSHA)
	})
	t.Run("I2 pending cancellation reaps tree", func(t *testing.T) {
		runProductionManagedI2(t, helper, helperSHA)
	})
	t.Run("I3 explicit stop and application shutdown reap once", func(t *testing.T) {
		runProductionManagedI3(t, helper, helperSHA)
	})
	t.Run("I4 crash preserves identity and retries", func(t *testing.T) {
		runProductionManagedI4(t, helper, helperSHA)
	})
}

func requireProductionManagedHelper(t *testing.T) (string, string) {
	t.Helper()
	helper := strings.TrimSpace(os.Getenv("YOU_MODELS_MANAGED_PROCESS_HELPER"))
	helperSHA := strings.TrimSpace(os.Getenv("YOU_MODELS_MANAGED_PROCESS_HELPER_SHA256"))
	if helper == "" || helperSHA == "" {
		t.Skip("production managed-process lifecycle requires the Make-owned helper")
	}
	if !filepath.IsAbs(helper) {
		t.Fatalf("managed-process helper path %q is not absolute", helper)
	}
	helperBody, err := os.ReadFile(helper)
	if err != nil {
		t.Fatalf("read managed-process helper %q: %v", helper, err)
	}
	digest := sha256.Sum256(helperBody)
	actualSHA := hex.EncodeToString(digest[:])
	if actualSHA != helperSHA {
		t.Fatalf("managed-process helper SHA-256 = %s, want Make-owned digest %s", actualSHA, helperSHA)
	}
	return helper, actualSHA
}

func runProductionManagedI1(t *testing.T, helper, helperSHA string) {
	t.Helper()
	harness := newProductionManagedHostHarness(t, helper, "production-ready")
	result := ensureProductionManagedHost(t, harness)
	if result.Outcome != models.HostEnsureBecameReady || result.Host.ReadinessState != models.ReadinessStateReady || result.Host.LifecycleState != models.LifecycleStateLoaded {
		t.Fatalf("first ensure result = %#v, want became-ready/ready/loaded", result)
	}
	rootPID := waitProductionManagedPID(t, harness.rootPIDPath)
	descendantPID := waitProductionManagedPID(t, harness.descendantPIDPath)
	assertProductionManagedEndpoint(t, harness.rootEndpoint)
	assertProductionManagedEndpoint(t, harness.descendantEndpoint)

	inspected, err := harness.service.InspectModelHost(context.Background(), models.InspectModelHostRequest{Scope: harness.scope, Name: productionManagedModelName})
	if err != nil {
		t.Fatalf("InspectModelHost after first ensure: %v", err)
	}
	if inspected.Host.ReadinessState != models.ReadinessStateReady || inspected.Host.Diagnostics["failureClass"] != "" {
		t.Fatalf("host after first ensure = %#v, want ready without a failure class", inspected.Host)
	}

	second := ensureProductionManagedHost(t, harness)
	if second.Outcome != models.HostEnsureAlreadyReady || second.Host.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("second ensure result = %#v, want already-ready/ready", second)
	}
	assertProductionManagedEndpoint(t, harness.rootEndpoint)
	assertProductionManagedEndpoint(t, harness.descendantEndpoint)
	assertProductionManagedEvidence(t, harness.evidencePath, 1, 0, 1)

	stopProductionManagedHost(t, harness)
	waitProductionManagedEndpointDown(t, harness.rootEndpoint)
	waitProductionManagedEndpointDown(t, harness.descendantEndpoint)
	assertProductionManagedEvidence(t, harness.evidencePath, 1, 1, 2)
	stopProductionManagedHost(t, harness)

	t.Logf("MANAGED-MODELS-WIRE-EVIDENCE cell=I1 helper=%s helperSHA256=%s rootPID=%d descendantPID=%d launches=1 exits=1 survivors=0 network=loopback-only", helper, helperSHA, rootPID, descendantPID)
}

func runProductionManagedI2(t *testing.T, helper, helperSHA string) {
	t.Helper()
	harness := newProductionManagedHostHarness(t, helper, "production-pending")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, ensureErr := harness.service.EnsureModelHost(ctx, models.EnsureModelHostRequest{Scope: harness.scope, Name: productionManagedModelName})
		errCh <- ensureErr
	}()

	rootPID := waitProductionManagedPID(t, harness.rootPIDPath)
	descendantPID := waitProductionManagedPID(t, harness.descendantPIDPath)
	waitProductionManagedEndpointStatus(t, harness.rootEndpoint, http.StatusServiceUnavailable)
	assertProductionManagedEndpoint(t, harness.descendantEndpoint)
	cancel()
	ensureErr := receiveProductionManagedError(t, errCh)
	if !errors.Is(ensureErr, models.ErrHostCancelled) || !errors.Is(ensureErr, context.Canceled) {
		t.Fatalf("pending EnsureModelHost error = %v, want typed host cancellation wrapping context.Canceled", ensureErr)
	}
	inspected, err := harness.service.InspectModelHost(context.Background(), models.InspectModelHostRequest{Scope: harness.scope, Name: productionManagedModelName})
	if err != nil {
		t.Fatalf("InspectModelHost after cancellation: %v", err)
	}
	if inspected.Host.ReadinessState == models.ReadinessStateReady {
		t.Fatalf("host after pending cancellation = %#v, must not publish READY", inspected.Host)
	}
	waitProductionManagedEndpointDown(t, harness.rootEndpoint)
	waitProductionManagedEndpointDown(t, harness.descendantEndpoint)
	assertProductionManagedEvidence(t, harness.evidencePath, 1, 1, 2)

	t.Logf("MANAGED-MODELS-WIRE-EVIDENCE cell=I2 helper=%s helperSHA256=%s rootPID=%d descendantPID=%d outcome=HOST_CANCELLED launches=1 exits=1 survivors=0 network=loopback-only", helper, helperSHA, rootPID, descendantPID)
}

func runProductionManagedI3(t *testing.T, helper, helperSHA string) {
	t.Helper()
	t.Run("explicit stop", func(t *testing.T) {
		harness := newProductionManagedHostHarness(t, helper, "production-ready")
		ensureProductionManagedHost(t, harness)
		rootPID := waitProductionManagedPID(t, harness.rootPIDPath)
		descendantPID := waitProductionManagedPID(t, harness.descendantPIDPath)
		stopProductionManagedHost(t, harness)
		waitProductionManagedEndpointDown(t, harness.rootEndpoint)
		waitProductionManagedEndpointDown(t, harness.descendantEndpoint)
		assertProductionManagedEvidence(t, harness.evidencePath, 1, 1, 2)
		stopProductionManagedHost(t, harness)
		inspected, err := harness.service.InspectModelHost(context.Background(), models.InspectModelHostRequest{Scope: harness.scope, Name: productionManagedModelName})
		if err != nil {
			t.Fatalf("InspectModelHost after explicit stop: %v", err)
		}
		if inspected.Host.Diagnostics["failureClass"] != "" {
			t.Fatalf("host after explicit stop = %#v, must not project a crash", inspected.Host)
		}
		t.Logf("MANAGED-MODELS-WIRE-EVIDENCE cell=I3/stop helper=%s helperSHA256=%s rootPID=%d descendantPID=%d launches=1 exits=1 survivors=0 network=loopback-only", helper, helperSHA, rootPID, descendantPID)
	})

	t.Run("application shutdown", func(t *testing.T) {
		harness := newProductionManagedHostHarness(t, helper, "production-ready")
		ensureProductionManagedHost(t, harness)
		rootPID := waitProductionManagedPID(t, harness.rootPIDPath)
		descendantPID := waitProductionManagedPID(t, harness.descendantPIDPath)
		shutdown, ok := harness.service.(interface{ Close(context.Context) error })
		if !ok {
			t.Fatal("production Models root does not expose its process-lifecycle Close hook")
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), productionManagedWaitTime)
		if err := shutdown.Close(shutdownCtx); err != nil {
			cancel()
			t.Fatalf("Models root Close: %v", err)
		}
		cancel()
		waitProductionManagedEndpointDown(t, harness.rootEndpoint)
		waitProductionManagedEndpointDown(t, harness.descendantEndpoint)
		assertProductionManagedEvidence(t, harness.evidencePath, 1, 1, 2)
		if err := shutdown.Close(context.Background()); err != nil {
			t.Fatalf("repeated Models root Close: %v", err)
		}
		t.Logf("MANAGED-MODELS-WIRE-EVIDENCE cell=I3/shutdown helper=%s helperSHA256=%s rootPID=%d descendantPID=%d launches=1 exits=1 survivors=0 network=loopback-only", helper, helperSHA, rootPID, descendantPID)
	})
}

func runProductionManagedI4(t *testing.T, helper, helperSHA string) {
	t.Helper()
	harness := newProductionManagedHostHarness(t, helper, "production-crash")
	ensureProductionManagedHost(t, harness)
	firstRootPID := waitProductionManagedPID(t, harness.rootPIDPath)
	firstDescendantPID := waitProductionManagedPID(t, harness.descendantPIDPath)
	assertProductionManagedEndpoint(t, harness.rootEndpoint)
	assertProductionManagedEndpoint(t, harness.descendantEndpoint)
	if err := os.WriteFile(harness.crashPath, []byte("crash\n"), 0o600); err != nil {
		t.Fatalf("publish crash trigger: %v", err)
	}
	waitProductionManagedEndpointDown(t, harness.rootEndpoint)
	waitProductionManagedEndpointDown(t, harness.descendantEndpoint)
	waitProductionManagedCrashState(t, harness)
	if err := os.Remove(harness.crashPath); err != nil {
		t.Fatalf("remove one-shot crash trigger: %v", err)
	}

	result := ensureProductionManagedHost(t, harness)
	if result.Outcome != models.HostEnsureBecameReady || result.Host.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("retry ensure result = %#v, want became-ready/ready", result)
	}
	secondRootPID := waitProductionManagedPID(t, harness.rootPIDPath)
	secondDescendantPID := waitProductionManagedPID(t, harness.descendantPIDPath)
	assertProductionManagedEndpoint(t, harness.rootEndpoint)
	assertProductionManagedEndpoint(t, harness.descendantEndpoint)
	assertProductionManagedEvidence(t, harness.evidencePath, 2, 1, 3)
	if firstRootPID == secondRootPID && firstDescendantPID == secondDescendantPID {
		t.Fatalf("retry reused both original PIDs: root=%d descendant=%d", secondRootPID, secondDescendantPID)
	}
	stopProductionManagedHost(t, harness)
	waitProductionManagedEndpointDown(t, harness.rootEndpoint)
	waitProductionManagedEndpointDown(t, harness.descendantEndpoint)
	assertProductionManagedEvidence(t, harness.evidencePath, 2, 2, 4)

	t.Logf("MANAGED-MODELS-WIRE-EVIDENCE cell=I4 helper=%s helperSHA256=%s firstRootPID=%d firstDescendantPID=%d retryRootPID=%d retryDescendantPID=%d launches=2 exits=2 survivors=0 network=loopback-only", helper, helperSHA, firstRootPID, firstDescendantPID, secondRootPID, secondDescendantPID)
}

const (
	productionManagedModelName = "OMNIVOICE_Q4_K_M"
	productionManagedWaitTime  = 15 * time.Second
	productionManagedPollTime  = 10 * time.Millisecond
)

type productionManagedHostHarness struct {
	service             models.Service
	scope               models.RuntimeScopeRef
	rootEndpoint        string
	descendantEndpoint  string
	rootPIDPath         string
	descendantPIDPath   string
	descendantReadyPath string
	crashPath           string
	evidencePath        string
}

type productionManagedCacheMetadata struct {
	ModelName string                               `json:"modelName"`
	Revision  string                               `json:"revision"`
	Files     []productionManagedCacheMetadataFile `json:"files"`
}

type productionManagedCacheMetadataFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func newProductionManagedHostHarness(t *testing.T, helper, caseName string) *productionManagedHostHarness {
	t.Helper()
	root := t.TempDir()
	cacheDirectory := filepath.Join(root, "cache")
	writeProductionManagedCacheFixture(t, cacheDirectory)
	rootAddress := reserveProductionManagedAddress(t)
	descendantAddress := reserveProductionManagedAddress(t)
	harness := &productionManagedHostHarness{
		rootEndpoint:        "http://" + rootAddress,
		descendantEndpoint:  "http://" + descendantAddress,
		rootPIDPath:         filepath.Join(root, "root.pid"),
		descendantPIDPath:   filepath.Join(root, "descendant.pid"),
		descendantReadyPath: filepath.Join(root, "descendant.ready"),
		crashPath:           filepath.Join(root, "crash.trigger"),
		evidencePath:        filepath.Join(root, "runtime-evidence.jsonl"),
	}
	t.Setenv(modelRuntimeEvidenceEnvironment, harness.evidencePath)

	service, err := provideModelsService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideModelsService: %v", err)
	}
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{Config: models.RuntimeScopeConfig{
		CacheDirectory: cacheDirectory,
		Runtime: models.RuntimeConfig{
			Workers: []models.RuntimeWorker{{
				Name:          "production-managed-worker",
				Type:          models.RuntimeWorkerTypeInference,
				Model:         productionManagedModelName,
				ModelLocality: models.RuntimeModelLocalityLocal,
				Command:       helper,
				Args: []string{
					"--case", caseName,
					"--health-endpoint", harness.rootEndpoint,
					"--health-server", rootAddress,
					"--descendant-health-server", descendantAddress,
					"--root-pid-file", harness.rootPIDPath,
					"--descendant-pid-file", harness.descendantPIDPath,
					"--descendant-ready-file", harness.descendantReadyPath,
					"--crash-file", harness.crashPath,
				},
			}},
			Resources: []models.RuntimeResource{{
				Name:       "production-managed-cache",
				Type:       models.RuntimeResourceTypeModel,
				Capacity:   1,
				Model:      productionManagedModelName,
				Backend:    "LLAMACPP",
				LoadPolicy: string(models.LoadPolicyKeepWarm),
				Provider:   "MODELSCOPE",
			}},
		},
	}})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	harness.service = service
	harness.scope = opened.Scope
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), productionManagedWaitTime)
		defer cancel()
		_, _ = service.StopModelHost(cleanupCtx, models.StopModelHostRequest{Scope: opened.Scope, Name: productionManagedModelName})
		_, _ = service.CloseRuntimeScope(cleanupCtx, models.CloseRuntimeScopeRequest{Scope: opened.Scope})
		if closer, ok := service.(interface{ Close(context.Context) error }); ok {
			_ = closer.Close(cleanupCtx)
		}
	})
	return harness
}

func writeProductionManagedCacheFixture(t *testing.T, cacheDirectory string) {
	t.Helper()
	revisionDirectory := filepath.Join(cacheDirectory, productionManagedModelName, "rev-test")
	if err := os.MkdirAll(revisionDirectory, 0o700); err != nil {
		t.Fatalf("create managed cache fixture: %v", err)
	}
	files := []productionManagedCacheMetadataFile{
		{Path: "omnivoice-base-Q4_K_M.gguf", SHA256: "aaa"},
		{Path: "omnivoice-tokenizer-Q4_K_M.gguf", SHA256: "bbb"},
	}
	for index, file := range files {
		if err := os.WriteFile(filepath.Join(revisionDirectory, file.Path), []byte{byte(index + 1), byte(index + 2)}, 0o600); err != nil {
			t.Fatalf("write managed cache artifact: %v", err)
		}
	}
	body, err := json.Marshal(productionManagedCacheMetadata{ModelName: productionManagedModelName, Revision: "rev-test", Files: files})
	if err != nil {
		t.Fatalf("marshal managed cache metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDirectory, productionManagedModelName, ".managed-cache.json"), body, 0o600); err != nil {
		t.Fatalf("write managed cache metadata: %v", err)
	}
}

func reserveProductionManagedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release loopback address: %v", err)
	}
	return address
}

func ensureProductionManagedHost(t *testing.T, harness *productionManagedHostHarness) models.EnsureModelHostResult {
	t.Helper()
	result, err := harness.service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{Scope: harness.scope, Name: productionManagedModelName})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	return result
}

func stopProductionManagedHost(t *testing.T, harness *productionManagedHostHarness) {
	t.Helper()
	result, err := harness.service.StopModelHost(context.Background(), models.StopModelHostRequest{Scope: harness.scope, Name: productionManagedModelName})
	if err != nil {
		t.Fatalf("StopModelHost: %v", err)
	}
	if result.Outcome != models.HostStopStopped && result.Outcome != models.HostStopAlreadyStopped {
		t.Fatalf("StopModelHost outcome = %q, want stopped or already-stopped", result.Outcome)
	}
}

func waitProductionManagedPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.NewTimer(productionManagedWaitTime)
	defer deadline.Stop()
	ticker := time.NewTicker(productionManagedPollTime)
	defer ticker.Stop()
	for {
		body, err := os.ReadFile(path)
		if err == nil {
			pid, scanErr := strconv.Atoi(strings.TrimSpace(string(body)))
			if scanErr == nil && pid > 0 {
				return pid
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for managed process PID file %q", path)
			return 0
		}
	}
}

func receiveProductionManagedError(t *testing.T, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(productionManagedWaitTime):
		t.Fatal("managed process EnsureModelHost did not return before the safety deadline")
		return nil
	}
}

func productionManagedHTTPStatus(endpoint string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/health", nil)
	if err != nil {
		return 0, err
	}
	response, err := (&http.Client{}).Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

func assertProductionManagedEndpoint(t *testing.T, endpoint string) {
	t.Helper()
	waitProductionManagedEndpointStatus(t, endpoint, http.StatusOK)
}

func waitProductionManagedEndpointStatus(t *testing.T, endpoint string, want int) {
	t.Helper()
	deadline := time.NewTimer(productionManagedWaitTime)
	defer deadline.Stop()
	ticker := time.NewTicker(productionManagedPollTime)
	defer ticker.Stop()
	for {
		if status, err := productionManagedHTTPStatus(endpoint); err == nil && status == want {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for managed endpoint %s to return HTTP %d", endpoint, want)
		}
	}
}

func waitProductionManagedEndpointDown(t *testing.T, endpoint string) {
	t.Helper()
	deadline := time.NewTimer(productionManagedWaitTime)
	defer deadline.Stop()
	ticker := time.NewTicker(productionManagedPollTime)
	defer ticker.Stop()
	for {
		if _, err := productionManagedHTTPStatus(endpoint); err != nil {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("managed endpoint %s remained reachable after teardown", endpoint)
		}
	}
}

func waitProductionManagedCrashState(t *testing.T, harness *productionManagedHostHarness) {
	t.Helper()
	deadline := time.NewTimer(productionManagedWaitTime)
	defer deadline.Stop()
	ticker := time.NewTicker(productionManagedPollTime)
	defer ticker.Stop()
	for {
		inspected, err := harness.service.InspectModelHost(context.Background(), models.InspectModelHostRequest{Scope: harness.scope, Name: productionManagedModelName})
		if err == nil && inspected.Host.ReadinessState == models.ReadinessStateFailed && inspected.Host.Diagnostics["failureClass"] == "process_crash" && inspected.Host.Diagnostics["endpoint"] == "" {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for typed process-crash state")
		}
	}
}

func assertProductionManagedEvidence(t *testing.T, path string, wantStarts, wantExits, wantRecords int) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read managed process evidence %q: %v", path, err)
	}
	records := decodeManagedChildEvidence(t, body)
	managedRecords, starts, exits := 0, 0, 0
	for _, record := range records {
		if record.Kind != managedChildEvidenceKind {
			continue
		}
		managedRecords++
		switch record.Phase {
		case managedChildPhaseStarted:
			starts++
		case managedChildPhaseExited:
			exits++
		}
	}
	if starts != wantStarts || exits != wantExits || managedRecords != wantRecords {
		t.Fatalf("managed process evidence records = %#v, want starts=%d exits=%d managed-records=%d", records, wantStarts, wantExits, wantRecords)
	}
}
