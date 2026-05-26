//go:build functionallong

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

const (
	cliHTTPStabilityPollInterval = 100 * time.Millisecond
	cliHTTPStabilitySleepMS      = 5000
	cliHTTPStabilityMinPolls     = 3
)

func TestMockWorkers_CLIServiceModeStartupWorkFileSupportsRepeatedLiveHTTPPollingBeforeCompletion(t *testing.T) {
	support.SkipLongFunctional(t, "slow mock-worker CLI HTTP stability smoke")

	dir := support.ScaffoldFactory(t, cliHTTPStabilitySmokeConfig())
	support.WriteAgentConfig(t, dir, "script-worker", `---
type: SCRIPT_WORKER
command: echo
args:
  - "mock-worker-cli-http-stability"
---
`)

	workFile := filepath.Join(dir, "startup-work.json")
	const (
		requestID = "request-cli-http-stability"
		workID    = "work-cli-http-stability"
		traceID   = "trace-cli-http-stability"
	)
	support.WriteWorkRequestFile(t, workFile, interfaces.SubmitRequest{
		RequestID:  requestID,
		Name:       "cli-http-stability",
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte(`{"title":"cli http stability startup work"}`),
	})

	sideEffectPath := filepath.Join(t.TempDir(), "mock-worker-cli-http-stability.txt")
	mockWorkersPath := writeCLIHTTPStabilityMockWorkersConfig(t, sideEffectPath, cliHTTPStabilitySleepMS)
	port := unusedTCPPortForCLIHTTPStability(t)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runcli.Run(ctx, runcli.RunConfig{
			Dir:                        dir,
			Continuously:               true,
			Port:                       port,
			WorkFile:                   workFile,
			MockWorkersEnabled:         true,
			MockWorkersConfigPath:      mockWorkersPath,
			SuppressDashboardRendering: true,
			Logger:                     zap.NewNop(),
		})
	}()

	waitForCLIHTTPStabilityStatusReadiness(t, baseURL, errCh, 10*time.Second)
	assertCLIHTTPStabilityInFlightStatusPollingWindow(t, baseURL, errCh, 15*time.Second)
	work := waitForCLIHTTPStabilityWorkAtPlace(t, baseURL, traceID, "task:done", errCh, 20*time.Second)
	item := requireCLIHTTPStabilityWorkByTrace(t, work, traceID)
	if stringPointerValue(item.WorkId) != workID {
		t.Fatalf("GET /work work_id = %q, want %q", stringPointerValue(item.WorkId), workID)
	}
	if cliHTTPStabilityWorkStateName(item.State) != "done" {
		t.Fatalf("GET /work state name = %q, want done", cliHTTPStabilityWorkStateName(item.State))
	}
	if cliHTTPStabilityWorkStateType(item.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work state type = %q, want TERMINAL", cliHTTPStabilityWorkStateType(item.State))
	}

	rawSideEffect, err := os.ReadFile(sideEffectPath)
	if err != nil {
		t.Fatalf("read mock-worker side effect: %v", err)
	}
	if string(rawSideEffect) != "mock worker cli http stability" {
		t.Fatalf("mock-worker side effect = %q, want %q", rawSideEffect, "mock worker cli http stability")
	}

	cancel()
	waitForCLIHTTPStabilityRunShutdown(t, errCh, 5*time.Second)
}

func cliHTTPStabilitySmokeConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "done", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "script-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "run-script",
				"worker":    "script-worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "done"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func writeCLIHTTPStabilityMockWorkersConfig(t *testing.T, sideEffectPath string, sleepMS int) string {
	t.Helper()

	cfg := factoryconfig.MockWorkersConfig{
		MockWorkers: []factoryconfig.MockWorkerConfig{{
			ID:              "delayed-cli-http-stability-script-worker",
			WorkerName:      "script-worker",
			WorkstationName: "run-script",
			RunType:         factoryconfig.MockWorkerRunTypeScript,
			ScriptConfig: &factoryconfig.MockWorkerScriptConfig{
				Command: os.Args[0],
				Args: []string{
					"-test.run=TestMockWorkers_ScriptHelper",
					"--",
					"sleep-write-file",
					strconv.Itoa(sleepMS),
					sideEffectPath,
					"mock worker cli http stability",
				},
			},
		}},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal CLI HTTP stability mock-workers config: %v", err)
	}

	path := filepath.Join(t.TempDir(), "mock-workers-cli-http-stability.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write CLI HTTP stability mock-workers config: %v", err)
	}
	return path
}

func waitForCLIHTTPStabilityStatusReadiness(t *testing.T, baseURL string, errCh <-chan error, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		assertCLIHTTPStabilityRunStillActive(t, errCh)
		if _, err := getCLIHTTPStabilityJSON[factoryapi.StatusResponse](baseURL + "/status"); err == nil {
			return
		}
		time.Sleep(cliHTTPStabilityPollInterval)
	}

	t.Fatalf("timed out waiting %s for CLI HTTP stability status readiness at %s/status", timeout, baseURL)
}

func assertCLIHTTPStabilityInFlightStatusPollingWindow(
	t *testing.T,
	baseURL string,
	errCh <-chan error,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	successfulPolls := 0
	sawActiveRuntime := false
	var lastStatus factoryapi.StatusResponse
	for time.Now().Before(deadline) {
		assertCLIHTTPStabilityRunStillActive(t, errCh)

		status := mustGetCLIHTTPStabilityJSON[factoryapi.StatusResponse](t, baseURL+"/status")
		lastStatus = status
		if status.Categories.Terminal == 0 {
			successfulPolls++
			if status.RuntimeStatus == string(interfaces.RuntimeStatusActive) {
				sawActiveRuntime = true
			}
			if successfulPolls >= cliHTTPStabilityMinPolls && sawActiveRuntime {
				return
			}
		}

		time.Sleep(cliHTTPStabilityPollInterval)
	}

	t.Fatalf(
		"timed out waiting %s for repeated in-flight status polling window; successful_polls=%d saw_active_runtime=%t last_status=%#v",
		timeout,
		successfulPolls,
		sawActiveRuntime,
		lastStatus,
	)
}

func waitForCLIHTTPStabilityWorkAtPlace(
	t *testing.T,
	baseURL string,
	traceID string,
	placeID string,
	errCh <-chan error,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		assertCLIHTTPStabilityRunStillActive(t, errCh)
		work := mustGetCLIHTTPStabilityJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		if item, found := findCLIHTTPStabilityWorkByTrace(work, traceID); found && cliHTTPStabilityWorkPlaceID(item) == placeID {
			return work
		}
		time.Sleep(cliHTTPStabilityPollInterval)
	}

	work := mustGetCLIHTTPStabilityJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
	t.Fatalf("timed out waiting %s for trace %q at %s; last work response: %#v", timeout, traceID, placeID, work)
	return factoryapi.ListWorkResponse{}
}

func requireCLIHTTPStabilityWorkByTrace(t *testing.T, work factoryapi.ListWorkResponse, traceID string) factoryapi.Work {
	t.Helper()

	item, found := findCLIHTTPStabilityWorkByTrace(work, traceID)
	if !found {
		t.Fatalf("trace %q missing from GET /work response: %#v", traceID, work)
	}
	return item
}

func findCLIHTTPStabilityWorkByTrace(work factoryapi.ListWorkResponse, traceID string) (factoryapi.Work, bool) {
	for _, item := range work.Results {
		if stringPointerValue(item.TraceId) == traceID {
			return item, true
		}
	}
	return factoryapi.Work{}, false
}

func cliHTTPStabilityWorkPlaceID(work factoryapi.Work) string {
	return stringPointerValue(work.WorkTypeName) + ":" + cliHTTPStabilityWorkStateName(work.State)
}

func cliHTTPStabilityWorkStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func cliHTTPStabilityWorkStateType(state *factoryapi.WorkState) factoryapi.WorkStateType {
	if state == nil {
		return ""
	}
	return state.Type
}

func assertCLIHTTPStabilityRunStillActive(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		t.Fatalf("runcli.Run returned before test finished: %v", err)
	default:
	}
}

func waitForCLIHTTPStabilityRunShutdown(t *testing.T, errCh <-chan error, timeout time.Duration) {
	t.Helper()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runcli.Run after cancellation: %v", err)
		}
	case <-time.After(timeout):
		t.Fatalf("timed out waiting %s for runcli.Run shutdown", timeout)
	}
}

func mustGetCLIHTTPStabilityJSON[T any](t *testing.T, endpoint string) T {
	t.Helper()

	out, err := getCLIHTTPStabilityJSON[T](endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	return out
}

func getCLIHTTPStabilityJSON[T any](endpoint string) (T, error) {
	var out T

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func unusedTCPPortForCLIHTTPStability(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on unused TCP port: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address has type %T, want *net.TCPAddr", listener.Addr())
	}
	return addr.Port
}
