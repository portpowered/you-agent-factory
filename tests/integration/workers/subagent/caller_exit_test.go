package subagent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// The invoking lane supplies one compiled artifact. One canonical host retains
// both controlled attempts while actual CLI and MCP processes return and exit.
// This small integration witness owns the OS boundary; functional tests own the
// route/error/deadline matrix. No provider executable or test subprocess runs.
func TestDirectStartCallerExit(t *testing.T) {
	binary := prebuiltCLI(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	directory, home := t.TempDir(), t.TempDir()
	writeCurrentFactory(t, directory)
	env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	runner := &heldProvider{started: make(chan platformprocess.CommandRequest, 2), stopped: make(chan struct{}, 2)}
	host := startHost(t, ctx, directory, home, env, runner)
	baseline := factorySessionIDs(t, ctx, host)
	if len(baseline) == 0 {
		t.Fatal("host must expose its idle baseline Factory Session")
	}
	for _, mode := range []string{"cli", "mcp"} {
		t.Run(mode, func(t *testing.T) {
			id := "caller-exit-" + mode
			var admission []byte
			if mode == "cli" {
				admission = runCLI(t, ctx, binary, directory, env, "--server", host, "--json", "worker-sessions", "start",
					"--request-id", id+"-request", "--worker-session-id", id, "--provider", "codex", "--model", "controlled-model",
					"--working-root", directory, "held "+mode)
			} else {
				admission = runMCP(t, ctx, binary, directory, env, host, map[string]any{
					"action": "START", "requestId": id + "-request", "workerSessionId": id,
					"prompt": "held " + mode, "provider": "codex", "model": "controlled-model", "workingRoot": directory,
				})
			}
			if !bytes.Contains(admission, []byte(id)) || !bytes.Contains(admission, []byte(`"accepted":true`)) {
				t.Fatalf("caller acceptance = %s", admission)
			}
			select {
			case command := <-runner.started:
				if command.Command != "codex" || string(command.Stdin) != "held "+mode {
					t.Fatalf("selected effect = %#v", command)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			// Both helpers have Waited for their real caller process before READ.
			show := runCLI(t, ctx, binary, directory, env, "--server", host, "--json", "worker-sessions", "show", "--worker-session-id", id)
			assertActive(t, show, id)
			read := runMCP(t, ctx, binary, directory, env, host, map[string]any{"action": "READ", "workerSessionId": id})
			assertActive(t, read, id)
			select {
			case <-runner.stopped:
				t.Fatal("caller exit canceled a retained provider")
			default:
			}
		})
	}
	if after := factorySessionIDs(t, ctx, host); !slices.Equal(after, baseline) {
		t.Fatalf("direct START created a Factory Session: before=%v after=%v", baseline, after)
	}
}

func factorySessionIDs(t *testing.T, ctx context.Context, host string) []string {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/factory-sessions", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Factory Session listing status = %d", response.StatusCode)
	}
	var listing struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listing); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(listing.Sessions))
	for _, session := range listing.Sessions {
		if session.ID == "" {
			t.Fatal("Factory Session listing omitted identity")
		}
		ids = append(ids, session.ID)
	}
	slices.Sort(ids)
	return ids
}

func prebuiltCLI(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("INFINITE_YOU_PREBUILT_ARTIFACT"))
	if path == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("caller-exit integration requires INFINITE_YOU_PREBUILT_ARTIFACT")
		}
		t.Skip("invoking build lane must supply INFINITE_YOU_PREBUILT_ARTIFACT")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || !filepath.IsAbs(path) {
		t.Fatalf("invalid prebuilt CLI %q: %v", path, err)
	}
	return path
}

func startHost(t *testing.T, ctx context.Context, directory, home string, env []string, runner platformprocess.CommandRunner) string {
	t.Helper()
	ready := make(chan string, 1)
	process, err := root.BuildProcess(ctx, serviceedges.Edges{
		FactorySessionsWorkingDirectory:    platformfilesystem.Local{WorkingDirectory: directory},
		FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
		ProviderCommandRunner:              runner,
		ProviderCatalogProbe: func(_ context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
			descriptor.Readiness = providers.ReadinessReady
			return descriptor, nil
		},
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			server := httptest.NewServer(request.Handler)
			defer server.Close()
			address := server.Listener.Addr().(*net.TCPAddr)
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Host: address.IP.String(), Port: address.Port})
			}
			ready <- server.URL
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	hostCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	var hostErr error
	go func() {
		hostErr = process.Execute(root.Input{Context: hostCtx, Args: []string{"you", "server"}, Env: env,
			WorkingDirectory: directory, Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
		close(done)
	}()
	t.Cleanup(func() {
		stop()
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		select {
		case <-done:
		case <-closeCtx.Done():
			t.Error("host did not join before integration ceiling")
		}
		if err := process.Close(closeCtx); err != nil {
			t.Error(err)
		}
	})
	select {
	case host := <-ready:
		return host
	case <-done:
		t.Fatalf("host exited before readiness: %v", hostErr)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return ""
}

func writeCurrentFactory(t *testing.T, directory string) {
	t.Helper()
	factory := filepath.Join(directory, "factory")
	if err := os.MkdirAll(factory, 0o755); err != nil {
		t.Fatal(err)
	}
	definition := `{"name":"caller-exit","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"name":"processor"}],"workstations":[{"name":"process","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}],"worker":"processor"}]}`
	if err := os.WriteFile(filepath.Join(factory, "factory.json"), []byte(definition), 0o600); err != nil {
		t.Fatal(err)
	}
}

type heldProvider struct {
	started chan platformprocess.CommandRequest
	stopped chan struct{}
}

func (runner *heldProvider) Run(ctx context.Context, command platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.started <- command
	<-ctx.Done()
	runner.stopped <- struct{}{}
	return platformprocess.CommandResult{}, ctx.Err()
}

func runCLI(t *testing.T, ctx context.Context, binary, directory string, env []string, args ...string) []byte {
	t.Helper()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir, command.Env = directory, env
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("compiled caller %q: %v; stderr=%s stdout=%s", args, err, stderr.String(), output)
	}
	return output
}

// Explicitly own stdin and Wait so EOF proves actual MCP process exit. The
// scanner accepts only the requested JSON-RPC response, ignoring notifications.
func runMCP(t *testing.T, ctx context.Context, binary, directory string, env []string, host string, arguments map[string]any) []byte {
	t.Helper()
	command := exec.CommandContext(ctx, binary, "--server", host, "server", "mcp", "--project-root", directory)
	command.Dir, command.Env = directory, env
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	sendMCP(t, stdin, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "caller-exit", "version": "1"}}})
	_ = readMCP(t, scanner, 1)
	sendMCP(t, stdin, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	sendMCP(t, stdin, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "you.subagent", "arguments": arguments}})
	result := readMCP(t, scanner, 2)
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("compiled MCP caller exit: %v; stderr=%s", err, stderr.String())
	}
	return result
}

func sendMCP(t *testing.T, stream io.Writer, message any) {
	t.Helper()
	if err := json.NewEncoder(stream).Encode(message); err != nil {
		t.Fatal(err)
	}
}

func readMCP(t *testing.T, scanner *bufio.Scanner, id int) []byte {
	t.Helper()
	for scanner.Scan() {
		var response struct {
			ID     int `json:"id"`
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.ID != id {
			continue
		}
		if len(response.Error) != 0 || response.Result.IsError {
			t.Fatalf("MCP response: %s", scanner.Bytes())
		}
		if id == 1 {
			return nil
		}
		if len(response.Result.Content) != 1 {
			t.Fatalf("MCP content = %s", scanner.Bytes())
		}
		return []byte(response.Result.Content[0].Text)
	}
	t.Fatalf("MCP ended before response %d: %v", id, scanner.Err())
	return nil
}

func assertActive(t *testing.T, body []byte, id string) {
	t.Helper()
	if !bytes.Contains(body, []byte(id)) || !bytes.Contains(body, []byte(`"state":"RUNNING"`)) {
		t.Fatalf("fresh observation after caller exit = %s", body)
	}
}
