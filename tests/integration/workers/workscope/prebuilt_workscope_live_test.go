package workscope_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	prebuiltWorkscopeLiveRequestID              = "workscope-live-request"
	prebuiltWorkscopeLiveWorkerSession          = "workscope-live-worker-session"
	prebuiltWorkscopeLiveDispatchID             = "workscope-live-dispatch"
	prebuiltWorkscopeLiveProviderControlAddress = "FACTORY_RELIABILITY_WORKSCOPE_PROVIDER_CONTROL_ADDRESS"
	prebuiltWorkscopeLiveProviderOutput         = "{\"type\":\"turn.started\"}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"workscope-live-message\",\"type\":\"agent_message\",\"text\":\"Workscope live fixture COMPLETE\"}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n"
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
	for _, directory := range []string{factoryDir, workspace, home} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create Work Session integration directory %s: %v", directory, err)
		}
	}
	writePrebuiltWorkscopeLiveFactory(t, factoryDir)
	writePrebuiltWorkscopeGatedCodex(t, providerDir)
	providerControlListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for controlled Codex readiness: %v", err)
	}
	t.Cleanup(func() { _ = providerControlListener.Close() })
	environment := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	environment = replacePrebuiltWorkscopeEnv(environment, "PATH", providerDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	environment = replacePrebuiltWorkscopeEnv(environment, prebuiltWorkscopeLiveProviderControlAddress, providerControlListener.Addr().String())
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
		net.JoinHostPort("127.0.0.1", fmt.Sprint(port)), "--no-record",
	)
	waitForPrebuiltWorkscopeLiveServer(t, ctx, daemon, serverURL)
	factorySessionID := readPrebuiltWorkscopeLiveFactorySessionID(t, ctx, serverURL)
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
	providerControl := waitForPrebuiltWorkscopeProviderStarted(t, ctx, daemon, providerControlListener)
	defer providerControl.Close()
	assertPrebuiltWorkscopeLiveDirectAssociation(t, ctx, serverURL, workID, factorySessionID)
	active := readPrebuiltWorkscopeLiveObservation(t, ctx, serverURL, binaryPath, factoryDir, environment, factorySessionID, workID,
		func(state string) bool { return state == "STARTING" || state == "RUNNING" },
	)
	if active.WorkerSessionId != prebuiltWorkscopeLiveWorkerSession {
		t.Fatalf("running Worker Session identity = %q, want %q", active.WorkerSessionId, prebuiltWorkscopeLiveWorkerSession)
	}
	eventStream := openPrebuiltWorkscopeLiveEventStream(t, ctx, serverURL, factorySessionID, active.WorkerSessionId)
	defer eventStream.Close()
	if _, err := io.WriteString(providerControl, "release\n"); err != nil {
		t.Fatalf("release controlled Codex provider: %v", err)
	}
	waitForPrebuiltWorkscopeTerminalEvent(t, eventStream, active.WorkerSessionId)
	finished := readPrebuiltWorkscopeLiveObservation(t, ctx, serverURL, binaryPath, factoryDir, environment, factorySessionID, workID,
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
	waitForPrebuiltWorkscopeOutput(t, ctx, daemon, "Dashboard URL: "+serverURL)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(serverURL, "/")+"/status", nil)
	if err != nil {
		t.Fatalf("build compiled live Worker Session readiness request: %v", err)
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("read compiled live Worker Session status after startup signal: %v", err)
	}
	var status factoryapi.StatusResponse
	decodeErr := json.NewDecoder(response.Body).Decode(&status)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || decodeErr != nil || status.FactoryState != "RUNNING" {
		t.Fatalf("compiled live Worker Session status = %d/%#v, decode=%v, want 200/RUNNING", response.StatusCode, status, decodeErr)
	}
}

func waitForPrebuiltWorkscopeOutput(t *testing.T, ctx context.Context, daemon *prebuiltWorkscopeDaemon, expected string) {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for {
		output, changed := daemon.stdout.snapshot()
		if strings.Contains(output, expected) {
			return
		}
		select {
		case <-changed:
		case err := <-daemon.done:
			daemon.done <- err
			t.Fatalf("prebuilt live Work Session daemon exited before startup signal %q: %v\nstdout=%s\nstderr=%s", expected, err, output, daemon.stderr.String())
		case <-ctx.Done():
			t.Fatalf("wait for prebuilt live Work Session startup signal %q: %v\nstdout=%s\nstderr=%s", expected, ctx.Err(), output, daemon.stderr.String())
		case <-deadline.C:
			t.Fatalf("prebuilt live Work Session startup signal %q was not emitted\nstdout=%s\nstderr=%s", expected, output, daemon.stderr.String())
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

func waitForPrebuiltWorkscopeProviderStarted(
	t *testing.T,
	ctx context.Context,
	daemon *prebuiltWorkscopeDaemon,
	listener net.Listener,
) net.Conn {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	if tcpListener, ok := listener.(*net.TCPListener); ok {
		if err := tcpListener.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
			t.Fatalf("set controlled Codex provider accept ceiling: %v", err)
		}
	}
	type acceptResult struct {
		connection net.Conn
		err        error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		connection, err := listener.Accept()
		accepted <- acceptResult{connection: connection, err: err}
	}()
	var connection net.Conn
	select {
	case result := <-accepted:
		if result.err != nil {
			t.Fatalf("accept controlled Codex provider readiness signal: %v", result.err)
		}
		connection = result.connection
	case err := <-daemon.done:
		daemon.done <- err
		_ = listener.Close()
		t.Fatalf("prebuilt live Work Session daemon exited before provider readiness: %v\nstdout=%s\nstderr=%s", err, daemon.stdout.String(), daemon.stderr.String())
	case <-ctx.Done():
		_ = listener.Close()
		t.Fatalf("wait for controlled Codex provider readiness signal: %v", ctx.Err())
	case <-deadline.C:
		_ = listener.Close()
		t.Fatalf("controlled Codex provider did not connect\nstdout=%s\nstderr=%s", daemon.stdout.String(), daemon.stderr.String())
	}
	if err := connection.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		connection.Close()
		t.Fatalf("set controlled Codex provider handshake ceiling: %v", err)
	}
	started, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil || strings.TrimSpace(started) != "started" {
		connection.Close()
		t.Fatalf("controlled Codex provider readiness = %q, err=%v; want started", started, err)
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		connection.Close()
		t.Fatalf("clear controlled Codex provider handshake deadline: %v", err)
	}
	_ = listener.Close()
	return connection
}

func readPrebuiltWorkscopeLiveObservation(
	t *testing.T,
	ctx context.Context,
	serverURL, binaryPath, workspace string,
	environment []string,
	factorySessionID, workID string,
	wantState func(string) bool,
) factoryapi.WorkerSessionObservation {
	t.Helper()
	endpoint := strings.TrimSuffix(serverURL, "/") + sessionpath.WorkerSessionsCollectionPath(factorySessionID) + "?workId=" + url.QueryEscape(workID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build compiled Work-scoped Worker Session read: %v", err)
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("read compiled Work-scoped Worker Sessions: %v", err)
	}
	body, status := readPrebuiltWorkscopeResponse(t, response)
	if status != http.StatusOK {
		t.Fatalf("compiled Work-scoped Worker Session list status=%d, want 200; body=%s", status, strings.TrimSpace(string(body)))
	}
	var rest factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(body, &rest); err != nil {
		t.Fatalf("decode compiled Work-scoped Worker Session list: %v; body=%s", err, strings.TrimSpace(string(body)))
	}
	if len(rest.Sessions) != 1 || !wantState(string(rest.Sessions[0].State)) {
		t.Fatalf("compiled Work-scoped Worker Session list = %#v, want one observation in the expected lifecycle state", rest.Sessions)
	}
	cliOutput := runPrebuiltWorkscopeCLI(t, ctx, binaryPath, workspace, environment,
		"worker-sessions", "list", "--server", serverURL,
		"--session", factorySessionID, "--work-id", workID, "--output", "json",
	)
	var cli factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(cliOutput, &cli); err != nil {
		t.Fatalf("decode compiled live Work-scoped CLI list: %v; output=%s", err, strings.TrimSpace(string(cliOutput)))
	}
	assertPrebuiltWorkscopeLiveListParity(t, rest, cli)
	return rest.Sessions[0]
}

func openPrebuiltWorkscopeLiveEventStream(t *testing.T, ctx context.Context, serverURL, factorySessionID, workerSessionID string) io.ReadCloser {
	t.Helper()
	endpoint := strings.TrimSuffix(serverURL, "/") + sessionpath.FactorySessionWorkerSessionEventsPath(factorySessionID, workerSessionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build compiled Worker Session event stream request: %v", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := (&http.Client{}).Do(request)
	if err != nil {
		t.Fatalf("open compiled Worker Session event stream: %v", err)
	}
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("compiled Worker Session event stream status/content-type=%d/%q, read=%v body=%s", response.StatusCode, response.Header.Get("Content-Type"), readErr, strings.TrimSpace(string(body)))
	}
	return response.Body
}

func waitForPrebuiltWorkscopeTerminalEvent(t *testing.T, stream io.Reader, workerSessionID string) {
	t.Helper()
	reader := bufio.NewReader(stream)
	var data []string
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
			if line == "" && len(data) > 0 {
				var frame struct {
					Delivery        string  `json:"delivery"`
					WorkerSessionID string  `json:"workerSessionId"`
					ErrorCode       *string `json:"errorCode"`
					ErrorMessage    *string `json:"errorMessage"`
				}
				if decodeErr := json.Unmarshal([]byte(strings.Join(data, "\n")), &frame); decodeErr != nil {
					t.Fatalf("decode compiled Worker Session event: %v; data=%q", decodeErr, data)
				}
				if frame.WorkerSessionID != workerSessionID {
					t.Fatalf("compiled Worker Session event identity=%q, want %q", frame.WorkerSessionID, workerSessionID)
				}
				if frame.Delivery == "SOURCE_FAILURE" {
					code, message := "", ""
					if frame.ErrorCode != nil {
						code = *frame.ErrorCode
					}
					if frame.ErrorMessage != nil {
						message = *frame.ErrorMessage
					}
					t.Fatalf("compiled Worker Session stream failed: %s %s", code, message)
				}
				if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
					return
				}
				data = nil
			}
		}
		if err != nil {
			t.Fatalf("compiled Worker Session stream ended before terminal delivery: %v", err)
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
$address = $env:FACTORY_RELIABILITY_WORKSCOPE_PROVIDER_CONTROL_ADDRESS -split ':', 2
$control = [System.Net.Sockets.TcpClient]::new()
$control.Connect($address[0], [int]$address[1])
$stream = $control.GetStream()
$reader = [System.IO.StreamReader]::new($stream)
$writer = [System.IO.StreamWriter]::new($stream)
$writer.AutoFlush = $true
$null = [Console]::In.ReadToEnd()
$writer.WriteLine("started")
if ($reader.ReadLine() -ne "release") { throw "controlled Codex provider did not receive release" }
[Console]::Out.WriteLine('{"type":"turn.started"}')
[Console]::Out.WriteLine('{"type":"item.completed","item":{"id":"workscope-live-message","type":"agent_message","text":"Workscope live fixture COMPLETE"}}')
[Console]::Out.WriteLine('{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}')
[Console]::Out.Flush()
$writer.WriteLine("completed")
$writer.Dispose()
$reader.Dispose()
$control.Dispose()
`

const prebuiltWorkscopeShellCodex = `#!/bin/bash
set -eu
control=${FACTORY_RELIABILITY_WORKSCOPE_PROVIDER_CONTROL_ADDRESS:?provider control address is required}
control_host=${control%:*}
control_port=${control##*:}
cat >/dev/null
exec 3<>"/dev/tcp/$control_host/$control_port"
printf '%s\n' started >&3
IFS= read -r control_command <&3
[ "$control_command" = release ]
printf '%s\n' '{"type":"turn.started"}'
printf '%s\n' '{"type":"item.completed","item":{"id":"workscope-live-message","type":"agent_message","text":"Workscope live fixture COMPLETE"}}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}'
printf '%s\n' completed >&3
exec 3<&-
exec 3>&-
`
