//go:build managed_process_integration

package managed_process_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	appwire "github.com/portpowered/infinite-you/pkg/wire"
)

const (
	modelRuntimeEvidenceEnvironment = "INFINITE_YOU_INTEGRATION_MODEL_RUNTIME_EVIDENCE"
	managedChildEvidenceKind        = "MANAGED_CHILD"
	managedChildPhaseStarted        = "PROCESS_STARTED"
	managedChildPhaseExited         = "PROCESS_EXITED"
	productionManagedModelName      = "OMNIVOICE_Q4_K_M"
	productionManagedWaitTime       = 15 * time.Second
)

// TestModelsManagedProcessProductionBoundaryLifecycle is the I1-I4
// compiled-artifact witness. The Make-owned helper is built once, hashed, and
// then launched through the canonical production Models/Wire construction
// seam. The test uses only loopback endpoints and deterministic cache files.
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
	waitProductionManagedReady(t, harness, http.StatusOK)

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
	waitProductionManagedReady(t, harness, http.StatusOK)
	assertProductionManagedEvidence(t, harness.evidencePath, 1, 0, 1)

	stopProductionManagedHost(t, harness)
	assertProductionManagedEvidence(t, harness.evidencePath, 1, 1, 2)
	assertProductionManagedProcessExited(t, rootPID)
	assertProductionManagedProcessExited(t, descendantPID)
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
	waitProductionManagedReady(t, harness, http.StatusServiceUnavailable)
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
	assertProductionManagedEvidence(t, harness.evidencePath, 1, 1, 2)
	assertProductionManagedProcessExited(t, rootPID)
	assertProductionManagedProcessExited(t, descendantPID)

	t.Logf("MANAGED-MODELS-WIRE-EVIDENCE cell=I2 helper=%s helperSHA256=%s rootPID=%d descendantPID=%d outcome=HOST_CANCELLED launches=1 exits=1 survivors=0 network=loopback-only", helper, helperSHA, rootPID, descendantPID)
}

func runProductionManagedI3(t *testing.T, helper, helperSHA string) {
	t.Helper()
	t.Run("explicit stop", func(t *testing.T) {
		harness := newProductionManagedHostHarness(t, helper, "production-ready")
		ensureProductionManagedHost(t, harness)
		rootPID := waitProductionManagedPID(t, harness.rootPIDPath)
		descendantPID := waitProductionManagedPID(t, harness.descendantPIDPath)
		waitProductionManagedReady(t, harness, http.StatusOK)
		stopProductionManagedHost(t, harness)
		assertProductionManagedEvidence(t, harness.evidencePath, 1, 1, 2)
		assertProductionManagedProcessExited(t, rootPID)
		assertProductionManagedProcessExited(t, descendantPID)
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
		waitProductionManagedReady(t, harness, http.StatusOK)
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
		assertProductionManagedEvidence(t, harness.evidencePath, 1, 1, 2)
		assertProductionManagedProcessExited(t, rootPID)
		assertProductionManagedProcessExited(t, descendantPID)
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
	waitProductionManagedReady(t, harness, http.StatusOK)
	if err := os.WriteFile(harness.crashPath, []byte("crash\n"), 0o600); err != nil {
		t.Fatalf("publish crash trigger: %v", err)
	}
	waitProductionManagedFile(t, harness.crashCompletePath, func(body []byte) (bool, error) {
		return len(bytes.TrimSpace(body)) > 0, nil
	})
	assertProductionManagedEvidence(t, harness.evidencePath, 1, 1, 2)
	assertProductionManagedProcessExited(t, firstRootPID)
	assertProductionManagedProcessExited(t, firstDescendantPID)
	waitProductionManagedCrashState(t, harness)
	if err := os.Remove(harness.crashPath); err != nil {
		t.Fatalf("remove one-shot crash trigger: %v", err)
	}
	clearProductionManagedSignals(t, harness)

	result := ensureProductionManagedHost(t, harness)
	if result.Outcome != models.HostEnsureBecameReady || result.Host.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("retry ensure result = %#v, want became-ready/ready", result)
	}
	secondRootPID := waitProductionManagedPID(t, harness.rootPIDPath)
	secondDescendantPID := waitProductionManagedPID(t, harness.descendantPIDPath)
	waitProductionManagedReady(t, harness, http.StatusOK)
	assertProductionManagedEvidence(t, harness.evidencePath, 2, 1, 3)
	if firstRootPID == secondRootPID && firstDescendantPID == secondDescendantPID {
		t.Fatalf("retry reused both original PIDs: root=%d descendant=%d", secondRootPID, secondDescendantPID)
	}
	stopProductionManagedHost(t, harness)
	assertProductionManagedEvidence(t, harness.evidencePath, 2, 2, 4)
	assertProductionManagedProcessExited(t, secondRootPID)
	assertProductionManagedProcessExited(t, secondDescendantPID)

	t.Logf("MANAGED-MODELS-WIRE-EVIDENCE cell=I4 helper=%s helperSHA256=%s firstRootPID=%d firstDescendantPID=%d retryRootPID=%d retryDescendantPID=%d launches=2 exits=2 survivors=0 network=loopback-only", helper, helperSHA, firstRootPID, firstDescendantPID, secondRootPID, secondDescendantPID)
}

type productionManagedHostHarness struct {
	service             models.Service
	scope               models.RuntimeScopeRef
	rootEndpoint        string
	descendantEndpoint  string
	rootPIDPath         string
	descendantPIDPath   string
	descendantReadyPath string
	rootReadyPath       string
	crashPath           string
	crashCompletePath   string
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

type productionManagedEvidenceRecord struct {
	Kind    string `json:"kind"`
	Phase   string `json:"phase"`
	Stage   string `json:"stage"`
	Outcome string `json:"outcome"`
	Class   string `json:"failure_class"`
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
		rootReadyPath:       filepath.Join(root, "root.ready"),
		crashPath:           filepath.Join(root, "crash.trigger"),
		crashCompletePath:   filepath.Join(root, "crash.complete"),
		evidencePath:        filepath.Join(root, "runtime-evidence.jsonl"),
	}
	t.Setenv(modelRuntimeEvidenceEnvironment, harness.evidencePath)

	service, err := appwire.NewModelsServiceForManagedProcessIntegration(serviceedges.Edges{})
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
					"--root-ready-file", harness.rootReadyPath,
					"--crash-file", harness.crashPath,
					"--crash-complete-file", harness.crashCompletePath,
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
	body := waitProductionManagedFile(t, path, func(body []byte) (bool, error) {
		pid, err := strconv.Atoi(strings.TrimSpace(string(body)))
		return err == nil && pid > 0, nil
	})
	pid, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil || pid <= 0 {
		t.Fatalf("managed process PID file %q became invalid: %q", path, body)
	}
	return pid
}

func receiveProductionManagedError(t *testing.T, results <-chan error) error {
	t.Helper()
	timer := time.NewTimer(productionManagedWaitTime)
	defer timer.Stop()
	select {
	case err := <-results:
		return err
	case <-timer.C:
		t.Fatal("managed process EnsureModelHost did not return before the safety deadline")
		return nil
	}
}

func waitProductionManagedReady(t *testing.T, harness *productionManagedHostHarness, wantRootStatus int) {
	t.Helper()
	waitProductionManagedFile(t, harness.rootReadyPath, func(body []byte) (bool, error) {
		return len(bytes.TrimSpace(body)) > 0, nil
	})
	assertProductionManagedEndpointStatus(t, harness.rootEndpoint, wantRootStatus)
	assertProductionManagedEndpointStatus(t, harness.descendantEndpoint, http.StatusOK)
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

func assertProductionManagedEndpointStatus(t *testing.T, endpoint string, want int) {
	t.Helper()
	status, err := productionManagedHTTPStatus(endpoint)
	if err != nil {
		t.Fatalf("managed endpoint %s request: %v", endpoint, err)
	}
	if status != want {
		t.Fatalf("managed endpoint %s returned HTTP %d, want %d", endpoint, status, want)
	}
}

func assertProductionManagedProcessExited(t *testing.T, pid int) {
	t.Helper()
	// Stop/Wait and the crash evidence are the lifecycle completion signals;
	// this one-shot platform-aware probe independently proves the OS root and
	// descendant are gone without turning liveness into a polling synchronizer.
	if helperProcessRunning(pid) {
		t.Fatalf("managed process PID %d remained alive after completion signal", pid)
	}
}

func waitProductionManagedCrashState(t *testing.T, harness *productionManagedHostHarness) {
	t.Helper()
	waitProductionManagedEvidenceRecords(t, harness.evidencePath, func(records []productionManagedEvidenceRecord) bool {
		for _, record := range records {
			if record.Kind == "STAGE" && record.Stage == "BACKEND_START" && record.Outcome == "FAILED" && record.Class == "PROCESS_EXITED" {
				return true
			}
		}
		return false
	})
	inspected, err := harness.service.InspectModelHost(context.Background(), models.InspectModelHostRequest{Scope: harness.scope, Name: productionManagedModelName})
	if err != nil {
		t.Fatalf("InspectModelHost after crash evidence: %v", err)
	}
	if inspected.Host.ReadinessState != models.ReadinessStateFailed || inspected.Host.Diagnostics["failureClass"] != "process_crash" || inspected.Host.Diagnostics["endpoint"] != "" {
		t.Fatalf("host after crash evidence = %#v, want failed/process_crash/empty endpoint", inspected.Host)
	}
}

func assertProductionManagedEvidence(t *testing.T, path string, wantStarts, wantExits, wantRecords int) {
	t.Helper()
	records := waitProductionManagedEvidenceRecords(t, path, func(records []productionManagedEvidenceRecord) bool {
		_, starts, exits := productionManagedEvidenceCounts(records)
		return starts >= wantStarts && exits >= wantExits
	})
	managedRecords, starts, exits := productionManagedEvidenceCounts(records)
	if starts != wantStarts || exits != wantExits || managedRecords != wantRecords {
		t.Fatalf("managed process evidence records = %#v, want starts=%d exits=%d managed-records=%d", records, wantStarts, wantExits, wantRecords)
	}
}

func productionManagedEvidenceCounts(records []productionManagedEvidenceRecord) (managedRecords, starts, exits int) {
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
	return managedRecords, starts, exits
}

func clearProductionManagedSignals(t *testing.T, harness *productionManagedHostHarness) {
	t.Helper()
	for _, path := range []string{
		harness.rootPIDPath,
		harness.descendantPIDPath,
		harness.descendantReadyPath,
		harness.rootReadyPath,
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("remove stale managed-process signal %q: %v", path, err)
		}
	}
}

func waitProductionManagedEvidenceRecords(t *testing.T, path string, predicate func([]productionManagedEvidenceRecord) bool) []productionManagedEvidenceRecord {
	t.Helper()
	var matched []productionManagedEvidenceRecord
	waitProductionManagedFile(t, path, func(body []byte) (bool, error) {
		records, err := decodeProductionManagedEvidence(body)
		if err != nil {
			return false, err
		}
		if !predicate(records) {
			return false, nil
		}
		matched = records
		return true, nil
	})
	return matched
}

func decodeProductionManagedEvidence(body []byte) ([]productionManagedEvidenceRecord, error) {
	var records []productionManagedEvidenceRecord
	decoder := json.NewDecoder(bytes.NewReader(body))
	for {
		var record productionManagedEvidenceRecord
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			return records, nil
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
}

func waitProductionManagedFile(t *testing.T, path string, predicate func([]byte) (bool, error)) []byte {
	t.Helper()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("create managed-process signal watcher: %v", err)
	}
	defer watcher.Close()
	directory := filepath.Dir(path)
	if err := watcher.Add(directory); err != nil {
		t.Fatalf("watch managed-process signal directory %q: %v", directory, err)
	}
	timer := time.NewTimer(productionManagedWaitTime)
	defer timer.Stop()
	var lastErr error
	for {
		body, readErr := os.ReadFile(path)
		if readErr == nil {
			ready, predicateErr := predicate(body)
			if predicateErr != nil {
				t.Fatalf("read managed-process signal %q: %v", path, predicateErr)
			}
			if ready {
				return body
			}
			lastErr = fmt.Errorf("signal contents did not satisfy predicate")
		} else {
			lastErr = readErr
		}
		select {
		case _, ok := <-watcher.Events:
			if !ok {
				t.Fatalf("managed-process signal watcher closed for %q", path)
			}
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				t.Fatalf("managed-process signal watcher errors closed for %q", path)
			}
			t.Fatalf("managed-process signal watcher for %q: %v", path, watchErr)
		case <-timer.C:
			t.Fatalf("timed out waiting for managed-process signal %q: %v", path, lastErr)
		}
	}
}
