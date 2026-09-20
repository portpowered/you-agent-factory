package workscope_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	prebuiltWorkscopeLiveRequestID      = "workscope-live-request"
	prebuiltWorkscopeLiveWorkerSession  = "workscope-live-worker-session"
	prebuiltWorkscopeLiveDispatchID     = "workscope-live-dispatch"
	prebuiltWorkscopeLiveProviderState  = "FACTORY_RELIABILITY_WORKSCOPE_PROVIDER_STATE"
	prebuiltWorkscopeLiveProviderOutput = "{\"type\":\"turn.started\"}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"workscope-live-message\",\"type\":\"agent_message\",\"text\":\"Workscope live fixture COMPLETE\"}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n"
)

func TestPrebuiltWorkscopeRunningAndFinishedIdentity(t *testing.T) {
	binaryPath := resolvePrebuiltWorkscopeBinary(t)
	ctx, cancel := context.WithTimeout(t.Context(), prebuiltWorkscopeTimeout)
	defer cancel()
	root := t.TempDir()
	factoryDir := filepath.Join(root, "factory")
	workspace := filepath.Join(root, "workspace")
	home := filepath.Join(root, "home")
	providerDir := filepath.Join(root, "provider")
	providerState := filepath.Join(root, "provider-state")
	for _, directory := range []string{factoryDir, workspace, home, providerState} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create Work Session integration directory %s: %v", directory, err)
		}
	}
	writePrebuiltWorkscopeLiveFactory(t, factoryDir)
	writePrebuiltWorkscopeGatedCodex(t, providerDir)
	environment := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	environment = replacePrebuiltWorkscopeEnv(environment, "PATH", providerDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	environment = replacePrebuiltWorkscopeEnv(environment, prebuiltWorkscopeLiveProviderState, providerState)
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve isolated live Worker Session listener: %v", err)
	}
	if port == 7437 {
		t.Fatalf("reserved live Worker Session listener uses forbidden default port %d", port)
	}
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	daemon := startPrebuiltWorkscopeDaemon(t, ctx, binaryPath, factoryDir, environment,
		"run", "--dir", factoryDir, "--continuously", "--with-server", "--listen",
		net.JoinHostPort("127.0.0.1", fmt.Sprint(port)), "--no-record", "--quiet",
	)
	waitForPrebuiltWorkscopeLiveServer(t, ctx, daemon, serverURL)
	factorySessionID := readPrebuiltWorkscopeLiveFactorySessionID(t, ctx, serverURL)
	releasePath := filepath.Join(providerState, "release")
	t.Cleanup(func() { _ = os.WriteFile(releasePath, []byte("release"), 0o600) })
	workID := submitPrebuiltWorkscopeLiveWork(t, ctx, serverURL)
	invokeOutput := runPrebuiltWorkscopeCLI(t, ctx, binaryPath, factoryDir, environment,
		"--remote", "--server", serverURL, "--json", "worker-sessions", "invoke",
		"--request-id", prebuiltWorkscopeLiveRequestID,
		"--worker-session-id", prebuiltWorkscopeLiveWorkerSession,
		"--dispatch-id", prebuiltWorkscopeLiveDispatchID,
		"--execution", prebuiltWorkscopeLiveExecutionJSON(t, workspace, workID, factorySessionID),
		"--retry-max-attempts", "1", "--async",
	)
	assertPrebuiltWorkscopeLiveInvokeAccepted(t, invokeOutput)
	waitForPrebuiltWorkscopeProviderMarker(t, ctx, daemon, filepath.Join(providerState, "started"))
	assertPrebuiltWorkscopeLiveDirectAssociation(t, ctx, serverURL, workID, factorySessionID)
	active := waitForPrebuiltWorkscopeLiveObservation(t, ctx, daemon, serverURL, binaryPath, factoryDir, environment, workID,
		func(state string) bool { return state == "STARTING" || state == "RUNNING" },
	)
	if active.WorkerSessionId != prebuiltWorkscopeLiveWorkerSession {
		t.Fatalf("running Worker Session identity = %q, want %q", active.WorkerSessionId, prebuiltWorkscopeLiveWorkerSession)
	}
	if err := os.WriteFile(releasePath, []byte("release"), 0o600); err != nil {
		t.Fatalf("release controlled Codex provider: %v", err)
	}
	waitForPrebuiltWorkscopeProviderMarker(t, ctx, daemon, filepath.Join(providerState, "completed"))
	finished := waitForPrebuiltWorkscopeLiveObservation(t, ctx, daemon, serverURL, binaryPath, factoryDir, environment, workID,
		func(state string) bool { return state == "COMPLETED" },
	)
	if finished.WorkerSessionId != active.WorkerSessionId || finished.AttemptId != active.AttemptId ||
		finished.WorkId == nil || active.WorkId == nil || *finished.WorkId != *active.WorkId || *active.WorkId != workID {
		t.Fatalf("Worker Session identity changed from running to finished: running=%#v finished=%#v", active, finished)
	}
	if _, _, err := daemon.stopMemoryMonitor(); err != nil {
		t.Fatalf("measure live Work Session process memory: %v", err)
	}
	stopPrebuiltWorkscopeDaemon(t, ctx, daemon, binaryPath, factoryDir, environment, serverURL)
	t.Logf("PREBUILT-WS-LIVE artifact_sha256=%s work_id=%s worker_session_id=%s state=STARTING_OR_RUNNING->COMPLETED provider_processes=1",
		sha256FileDigest(t, binaryPath), workID, finished.WorkerSessionId)
}

func readPrebuiltWorkscopeLiveFactorySessionID(t *testing.T, ctx context.Context, serverURL string) string {
	t.Helper()
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions?scope=live"
	response := doPrebuiltWorkscopeGET(t, ctx, &http.Client{Timeout: 5 * time.Second}, endpoint)
	body, status := readPrebuiltWorkscopeResponse(t, response)
	if status != http.StatusOK {
		t.Fatalf("compiled live Factory Session list status=%d, want 200; body=%s", status, strings.TrimSpace(string(body)))
	}
	var listed factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("decode compiled live Factory Session list: %v; body=%s", err, strings.TrimSpace(string(body)))
	}
	for _, session := range listed.Sessions {
		if session.IsDefault && strings.TrimSpace(session.Id) != "" {
			return session.Id
		}
	}
	t.Fatalf("compiled live Factory Session list omitted the default runtime identity: %#v", listed.Sessions)
	return ""
}

func assertPrebuiltWorkscopeLiveDirectAssociation(t *testing.T, ctx context.Context, serverURL, workID, factorySessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(serverURL, "/") + "/worker-sessions/" + url.PathEscape(prebuiltWorkscopeLiveWorkerSession)
	response := doPrebuiltWorkscopeGET(t, ctx, &http.Client{Timeout: 5 * time.Second}, endpoint)
	body, status := readPrebuiltWorkscopeResponse(t, response)
	if status != http.StatusOK {
		t.Fatalf("compiled direct Worker Session status=%d, want 200; body=%s", status, strings.TrimSpace(string(body)))
	}
	var observation factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(body, &observation); err != nil {
		t.Fatalf("decode compiled direct Worker Session: %v; body=%s", err, strings.TrimSpace(string(body)))
	}
	if observation.FactorySessionId == nil || *observation.FactorySessionId != factorySessionID {
		t.Fatalf("compiled direct Worker Session Factory Session = %v, want resolved runtime ID %q", observation.FactorySessionId, factorySessionID)
	}
	for _, observedWorkID := range observation.WorkIds {
		if observedWorkID == workID {
			return
		}
	}
	t.Fatalf("compiled direct Worker Session omitted submitted Work %q: %#v", workID, observation)
}

func writePrebuiltWorkscopeLiveFactory(t *testing.T, factoryDir string) {
	t.Helper()
	definition := `{"name":"workscope-live","workTypes":[{"name":"task","states":[{"name":"ready","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"name":"processor"}],"workstations":[]}`
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(definition), 0o600); err != nil {
		t.Fatalf("write live Work Session factory: %v", err)
	}
}

func writePrebuiltWorkscopeGatedCodex(t *testing.T, providerDir string) {
	t.Helper()
	if err := os.MkdirAll(providerDir, 0o700); err != nil {
		t.Fatalf("create controlled Codex provider directory: %v", err)
	}
	if runtime.GOOS == "windows" {
		files := map[string]string{
			"codex.cmd": "@echo off\r\npowershell.exe -NoProfile -ExecutionPolicy Bypass -File \"%~dp0codex.ps1\"\r\nexit /b %errorlevel%\r\n",
			"codex.ps1": prebuiltWorkscopePowerShellCodex,
		}
		for name, contents := range files {
			writePrebuiltWorkscopeProviderFile(t, filepath.Join(providerDir, name), contents)
		}
		return
	}
	path := filepath.Join(providerDir, "codex")
	writePrebuiltWorkscopeProviderFile(t, path, prebuiltWorkscopeShellCodex)
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatalf("make controlled Codex provider executable: %v", err)
	}
}

func writePrebuiltWorkscopeProviderFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatalf("write controlled Codex provider %s: %v", path, err)
	}
}

func replacePrebuiltWorkscopeEnv(environment []string, key, value string) []string {
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, key) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, key+"="+value)
}

func waitForPrebuiltWorkscopeLiveServer(t *testing.T, ctx context.Context, daemon *prebuiltWorkscopeDaemon, serverURL string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(serverURL, "/")+"/status", nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				var status factoryapi.StatusResponse
				decodeErr := json.NewDecoder(response.Body).Decode(&status)
				response.Body.Close()
				if response.StatusCode == http.StatusOK && decodeErr == nil && status.FactoryState == "RUNNING" {
					return
				}
			}
		}
		select {
		case err := <-daemon.done:
			t.Fatalf("prebuilt live Work Session daemon exited before readiness: %v\nstdout=%s\nstderr=%s", err, daemon.stdout.String(), daemon.stderr.String())
		case <-ctx.Done():
			t.Fatalf("prebuilt live Work Session server was not ready: %v\nstdout=%s\nstderr=%s", ctx.Err(), daemon.stdout.String(), daemon.stderr.String())
		case <-deadline.C:
			t.Fatalf("prebuilt live Work Session server did not become ready\nstdout=%s\nstderr=%s", daemon.stdout.String(), daemon.stderr.String())
		case <-ticker.C:
		}
	}
}

func submitPrebuiltWorkscopeLiveWork(t *testing.T, ctx context.Context, serverURL string) string {
	t.Helper()
	name := "compiled Workscope live witness"
	payload, err := json.Marshal(factoryapi.SubmitWorkRequest{
		Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": "observe a live Worker Session"},
	})
	if err != nil {
		t.Fatalf("marshal compiled live Work request: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(serverURL, "/")+
		"/factory-sessions/"+url.PathEscape(prebuiltWorkscopeFactoryID)+"/work", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build compiled live Work request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("submit compiled live Work: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read compiled live Work response: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("compiled live Work status=%d, want 201; body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.Unmarshal(body, &submitted); err != nil {
		t.Fatalf("decode compiled live Work response: %v; body=%s", err, strings.TrimSpace(string(body)))
	}
	if submitted.WorkId == nil || strings.TrimSpace(*submitted.WorkId) == "" {
		t.Fatalf("compiled live Work response omitted Work ID: %#v", submitted)
	}
	return *submitted.WorkId
}

func prebuiltWorkscopeLiveExecutionJSON(t *testing.T, workspace, workID, factorySessionID string) string {
	t.Helper()
	request := map[string]any{
		"requestId":       prebuiltWorkscopeLiveRequestID,
		"workerSessionId": prebuiltWorkscopeLiveWorkerSession,
		"execution": map[string]any{
			"factorySessionId": factorySessionID,
			"workstationName":  workers.ProviderInvocationRoute,
			"dispatch": map[string]any{
				"dispatchId":      prebuiltWorkscopeLiveDispatchID,
				"workstationName": workers.ProviderInvocationRoute,
				"workerType":      "processor",
				"execution":       map[string]any{"workIds": []string{workID}},
			},
			"executorProvider":         "codex",
			"model":                    "workscope-fixture-model",
			"modelProvider":            "codex",
			"runnerId":                 "codex",
			"userMessage":              "Observe this compiled Work-scoped Worker Session.",
			"workerType":               "processor",
			"workingDirectory":         workspace,
			"workingDirectoryAuthored": true,
		},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode controlled Work Session execution: %v", err)
	}
	return string(encoded)
}

func assertPrebuiltWorkscopeLiveInvokeAccepted(t *testing.T, output []byte) {
	t.Helper()
	var response struct {
		Accepted        bool   `json:"accepted"`
		WorkerSessionID string `json:"workerSessionId"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatalf("decode compiled live Worker Session admission: %v; output=%s", err, strings.TrimSpace(string(output)))
	}
	if !response.Accepted || response.WorkerSessionID != prebuiltWorkscopeLiveWorkerSession {
		t.Fatalf("compiled live Worker Session admission = %#v, want accepted stable identity %q", response, prebuiltWorkscopeLiveWorkerSession)
	}
}

func waitForPrebuiltWorkscopeProviderMarker(
	t *testing.T,
	ctx context.Context,
	daemon *prebuiltWorkscopeDaemon,
	markerPath string,
) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for {
		if _, err := os.Stat(markerPath); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read controlled Codex provider marker %s: %v", markerPath, err)
		}
		select {
		case err := <-daemon.done:
			t.Fatalf("prebuilt live Work Session daemon exited before provider marker: %v\nstdout=%s\nstderr=%s", err, daemon.stdout.String(), daemon.stderr.String())
		case <-ctx.Done():
			t.Fatalf("wait for controlled Codex provider marker %s: %v", markerPath, ctx.Err())
		case <-deadline.C:
			t.Fatalf("controlled Codex provider did not write marker %s\nstdout=%s\nstderr=%s", markerPath, daemon.stdout.String(), daemon.stderr.String())
		case <-ticker.C:
		}
	}
}

func waitForPrebuiltWorkscopeLiveObservation(
	t *testing.T,
	ctx context.Context,
	daemon *prebuiltWorkscopeDaemon,
	serverURL, binaryPath, workspace string,
	environment []string,
	workID string,
	wantState func(string) bool,
) factoryapi.WorkerSessionObservation {
	t.Helper()
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + url.PathEscape(prebuiltWorkscopeFactoryID) +
		"/worker-sessions?workId=" + url.QueryEscape(workID)
	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	var lastState string
	var lastStatus int
	var lastBody string
	var lastErr error
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr == nil {
				lastStatus = response.StatusCode
				body, readErr := io.ReadAll(response.Body)
				lastBody = strings.TrimSpace(string(body))
				var rest factoryapi.ListWorkerSessionsResponse
				decodeErr := json.Unmarshal(body, &rest)
				response.Body.Close()
				lastErr = errors.Join(readErr, decodeErr)
				if response.StatusCode == http.StatusOK && lastErr == nil && len(rest.Sessions) == 1 {
					lastState = string(rest.Sessions[0].State)
					if wantState(lastState) {
						cliOutput := runPrebuiltWorkscopeCLI(t, ctx, binaryPath, workspace, environment,
							"worker-sessions", "list", "--server", serverURL,
							"--session", prebuiltWorkscopeFactoryID, "--work-id", workID, "--output", "json",
						)
						var cli factoryapi.ListWorkerSessionsResponse
						if err := json.Unmarshal(cliOutput, &cli); err != nil {
							t.Fatalf("decode compiled live Work-scoped CLI list: %v; output=%s", err, strings.TrimSpace(string(cliOutput)))
						}
						assertPrebuiltWorkscopeLiveListParity(t, rest, cli)
						return rest.Sessions[0]
					}
				}
			} else {
				lastErr = requestErr
			}
		} else {
			lastErr = err
		}
		select {
		case err := <-daemon.done:
			t.Fatalf("prebuilt live Work Session daemon exited during %s observation: %v\nstdout=%s\nstderr=%s", lastState, err, daemon.stdout.String(), daemon.stderr.String())
		case <-ctx.Done():
			t.Fatalf("wait for compiled Work-scoped Worker Session state: %v; last=%q status=%d body=%s error=%v", ctx.Err(), lastState, lastStatus, lastBody, lastErr)
		case <-deadline.C:
			t.Fatalf("compiled Work-scoped Worker Session did not reach expected state; last=%q status=%d body=%s error=%v", lastState, lastStatus, lastBody, lastErr)
		case <-ticker.C:
		}
	}
}

func assertPrebuiltWorkscopeLiveListParity(
	t *testing.T,
	rest, cli factoryapi.ListWorkerSessionsResponse,
) {
	t.Helper()
	if len(rest.Sessions) != 1 || len(cli.Sessions) != 1 {
		t.Fatalf("compiled live HTTP/CLI Work-scoped list counts differ: HTTP=%d CLI=%d", len(rest.Sessions), len(cli.Sessions))
	}
	httpObservation, cliObservation := rest.Sessions[0], cli.Sessions[0]
	if httpObservation.WorkerSessionId != cliObservation.WorkerSessionId ||
		httpObservation.AttemptId != cliObservation.AttemptId ||
		httpObservation.State != cliObservation.State ||
		httpObservation.WorkId == nil || cliObservation.WorkId == nil ||
		*httpObservation.WorkId != *cliObservation.WorkId ||
		!reflect.DeepEqual(httpObservation.WorkIds, cliObservation.WorkIds) ||
		!reflect.DeepEqual(httpObservation.FactorySessionId, cliObservation.FactorySessionId) {
		t.Fatalf("compiled live HTTP/CLI Worker Session identity differs: HTTP=%#v CLI=%#v", httpObservation, cliObservation)
	}
}

const prebuiltWorkscopePowerShellCodex = `$ErrorActionPreference = "Stop"
$state = $env:FACTORY_RELIABILITY_WORKSCOPE_PROVIDER_STATE
$null = [Console]::In.ReadToEnd()
[System.IO.File]::WriteAllText((Join-Path $state "started"), "started")
while (-not (Test-Path -LiteralPath (Join-Path $state "release"))) { Start-Sleep -Milliseconds 25 }
[Console]::Out.WriteLine('{"type":"turn.started"}')
[Console]::Out.WriteLine('{"type":"item.completed","item":{"id":"workscope-live-message","type":"agent_message","text":"Workscope live fixture COMPLETE"}}')
[Console]::Out.WriteLine('{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}')
[Console]::Out.Flush()
[System.IO.File]::WriteAllText((Join-Path $state "completed"), "completed")
`

const prebuiltWorkscopeShellCodex = `#!/bin/sh
set -eu
state=${FACTORY_RELIABILITY_WORKSCOPE_PROVIDER_STATE:?provider state directory is required}
cat >/dev/null
printf '%s' started > "$state/started"
while [ ! -f "$state/release" ]; do sleep 0.025; done
printf '%s\n' '{"type":"turn.started"}'
printf '%s\n' '{"type":"item.completed","item":{"id":"workscope-live-message","type":"agent_message","text":"Workscope live fixture COMPLETE"}}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}'
printf '%s' completed > "$state/completed"
`
