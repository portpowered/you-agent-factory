package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	acpfixture "github.com/portpowered/infinite-you/tests/functional/workers/invoke_continue/testdata/acpfixture"
)

type subagentMissingLocator struct {
	executable string
	calls      atomic.Int32
}

func (locator *subagentMissingLocator) LookPath(name string) (string, error) {
	if name == locator.executable {
		locator.calls.Add(1)
		return "", errors.New("private lookup diagnostic")
	}
	return name, nil
}

// F01-F05 share one immutable process per saved default/locator shape. All
// children refuse admission, so full public lists have a stable observation
// window even while parallel peers run. Only controlled effects are installed.
type subagentRefusalFixture struct {
	host                                             invokeContinueStartedProcess
	home, dir, executable, configPath, workersBefore string
	factoriesBefore                                  []string
	before                                           []byte
	locator                                          *subagentMissingLocator
	launches, pipes                                  *atomic.Int32
	route                                            *invokeContinueStaticCommandRoute
}

func TestSubagentUnavailableStartParity(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{"absent-path", "unresolved-name", "missing-default"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			fixture := newSubagentRefusalFixture(t, shape)
			if shape == "missing-default" {
				t.Run("run-removed", func(t *testing.T) {
					t.Parallel()
					body := executeSubagentMCP(t, t.Context(), fixture.host, fixture.home, fixture.dir, map[string]any{"action": "RUN", "prompt": "private synthetic input"})
					if !strings.Contains(body, "worker_session.run_removed") || !strings.Contains(body, "START") {
						t.Fatal("RUN lost migration guidance")
					}
					fixture.assertUnchanged(t)
				})
			}
			for _, mode := range []string{"cli", "mcp", "http"} {
				for _, selection := range []string{"default", "explicit", "unknown"} {
					if shape == "missing-default" && selection == "explicit" {
						continue
					}
					t.Run(mode+"-"+selection, func(t *testing.T) {
						t.Parallel()
						assertSubagentRefusal(t, fixture.host, fixture.home, fixture.dir, mode, selection, shape, fixture.executable)
						fixture.assertUnchanged(t)
					})
				}
			}
			t.Cleanup(func() {
				if shape != "missing-default" && fixture.locator.calls.Load() == 0 {
					t.Error("selected host did not consult executable locator")
				}
			})
		})
	}
}

func newSubagentRefusalFixture(t *testing.T, shape string) subagentRefusalFixture {
	t.Helper()
	rootDir := t.TempDir()
	dir, home, err := prepareInvokeContinuePackageRoot(t, rootDir)
	if err != nil {
		t.Fatal(err)
	}
	executable := "unresolved-acp-agent"
	if shape == "absent-path" {
		executable = filepath.Join(rootDir, "private executable", "absent-agent.exe")
	}
	profile := map[string]any{"defaults": map[string]any{"workerModelProvider": "codex", "workerModel": "fallback-model", "futureDefault": "retained"}, "futureRoot": map[string]any{"owner": "retained"},
		"workers": map[string]any{"acp": map[string]any{"integrations": []any{map[string]any{"id": "controlled-acp", "name": "custom-acp", "transport": "stdio", "command": `"` + filepath.ToSlash(executable) + `" acp --private-argument`}}}}}
	if shape != "missing-default" {
		profile["defaults"].(map[string]any)["subagent"] = map[string]any{"provider": "custom-acp", "model": "controlled-model"}
	}
	fixture := subagentRefusalFixture{dir: dir, home: home, executable: executable, configPath: filepath.Join(home, ".you-agent-factory", "config.json"), locator: &subagentMissingLocator{executable: filepath.ToSlash(executable)}, launches: &atomic.Int32{}, pipes: &atomic.Int32{}, route: &invokeContinueStaticCommandRoute{}}
	writeSubagentProfile(t, fixture.configPath, profile)
	fixture.host, err = startInvokeContinuePackageProcessWithEdges(t, dir, home, fixture.route, serviceedges.Edges{
		ProvidersExecutableLocator:    fixture.locator,
		PlatformProcessCommandFactory: acpfixture.RejectCommandFactory(func() { fixture.launches.Add(1) }),
		ProvidersStdioPipeFactory: func() (platformprocess.StdioChannel, error) {
			fixture.pipes.Add(1)
			return nil, errors.New("unexpected stdio")
		},
		ProviderCatalogProbe: func(_ context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
			descriptor.Readiness = providers.ReadinessReady
			return descriptor, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanupSubagentHost(t, fixture.host)
	fixture.before, err = os.ReadFile(fixture.configPath)
	if err != nil {
		t.Fatal(err)
	}
	status, body := t7HTTP(t, t.Context(), http.MethodGet, fixture.host.baseURL+"/worker-sessions", nil)
	if status != http.StatusOK {
		t.Fatal("Worker Session list unavailable")
	}
	fixture.workersBefore, fixture.factoriesBefore = body, subagentFactorySessionIDs(t, fixture.host.baseURL)
	return fixture
}

func (fixture subagentRefusalFixture) assertUnchanged(t *testing.T) {
	t.Helper()
	status, workers := t7HTTP(t, t.Context(), http.MethodGet, fixture.host.baseURL+"/worker-sessions", nil)
	factories := subagentFactorySessionIDs(t, fixture.host.baseURL)
	if status != http.StatusOK || workers != fixture.workersBefore || !slices.Equal(factories, fixture.factoriesBefore) {
		t.Fatal("refusal changed public membership")
	}
	if fixture.launches.Load() != 0 || fixture.pipes.Load() != 0 || len(fixture.route.requests()) != 0 {
		t.Fatal("refusal invoked provider/process/stdio or fallback")
	}
	after, err := os.ReadFile(fixture.configPath)
	if err != nil || string(after) != string(fixture.before) {
		t.Fatal("START rewrote saved defaults/siblings")
	}
}

func writeSubagentProfile(t *testing.T, path string, profile any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func cleanupSubagentHost(t *testing.T, host invokeContinueStartedProcess) {
	t.Helper()
	t.Cleanup(func() {
		if err := host.command.stop(); err != nil {
			t.Error(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := host.process.Close(ctx); err != nil {
			t.Error(err)
		}
	})
}

func assertSubagentRefusal(t *testing.T, host invokeContinueStartedProcess, home, dir, mode, selection, shape, executable string) {
	t.Helper()
	id := mode + "-" + selection
	document := map[string]any{"requestId": id + "-request", "workerSessionId": id, "prompt": "private synthetic input", "workingRoot": dir}
	code, status := "PROVIDER_UNAVAILABLE", http.StatusServiceUnavailable
	if selection == "explicit" {
		document["provider"], document["model"] = "custom-acp", "explicit-model"
	}
	if selection == "unknown" {
		document["provider"], document["model"] = "unknown-acp", "explicit-model"
		code, status = "SUBAGENT_PROVIDER_INVALID", http.StatusBadRequest
	}
	if shape == "missing-default" && selection == "default" {
		code, status = "SUBAGENT_ROUTE_INVALID", http.StatusBadRequest
	}
	body := executeSubagentPrompt(t, host, home, dir, mode, document, status)
	if !strings.Contains(body, code) || strings.Contains(body, `"accepted":true`) {
		t.Fatalf("%s refusal = %s", mode, body)
	}
	if mode == "mcp" {
		typed := "worker_session.invalid_request"
		if status == http.StatusServiceUnavailable {
			typed = "worker_session.unavailable"
		}
		if !strings.Contains(body, typed) || !strings.Contains(body, `"upstreamCode":"`+code+`"`) || status == http.StatusServiceUnavailable && !strings.Contains(body, `"retryable":true`) {
			t.Fatalf("typed MCP refusal = %s", body)
		}
	}
	for _, private := range []string{executable, "private-argument", "private lookup diagnostic", "private synthetic input", "fallback-model"} {
		if strings.Contains(body, private) {
			t.Fatal("refusal exposed private command/config/input")
		}
	}
}

// The same prompt crosses each public transport. A CLI failure must have no
// acknowledgment; the MCP protocol returns its existing structured envelope.
func executeSubagentPrompt(t *testing.T, host invokeContinueStartedProcess, home, dir, mode string, document map[string]any, expectedStatus int) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if mode == "http" {
		status, body := t7HTTP(t, ctx, http.MethodPost, host.baseURL+"/worker-sessions", document)
		if status != expectedStatus {
			t.Fatalf("HTTP status = %d, want %d: %s", status, expectedStatus, body)
		}
		return body
	}
	if mode == "mcp" {
		return executeSubagentMCP(t, ctx, host, home, dir, document)
	}
	args := []string{"you", "--server", host.baseURL, "--json", "worker-sessions", "start", "--request-id", document["requestId"].(string), "--worker-session-id", document["workerSessionId"].(string), "--working-root", dir}
	for _, key := range []string{"provider", "model"} {
		if value, ok := document[key].(string); ok {
			args = append(args, "--"+key, value)
		}
	}
	args = append(args, document["prompt"].(string))
	input := support.FakeInputs(ctx, args)
	input.Input.Env, input.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	err := host.process.Execute(input.Input)
	if expectedStatus == http.StatusAccepted {
		if err != nil {
			t.Fatalf("CLI START: %v %s", err, input.Stderr())
		}
		return input.Stdout()
	}
	if err == nil || input.Stdout() != "" {
		t.Fatal("unavailable CLI START acknowledged")
	}
	return input.Stderr()
}

func executeSubagentMCP(t *testing.T, ctx context.Context, host invokeContinueStartedProcess, home, dir string, document map[string]any) string {
	t.Helper()
	input := support.FakeInputs(ctx, []string{"you", "--server", host.baseURL, "server", "mcp", "--project-root", dir})
	input.Input.Env, input.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	reader, writer := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	input.Input.Stdin, input.Input.Stdout = reader, outputWriter
	done := make(chan error, 1)
	go func() {
		err := host.process.Execute(input.Input)
		_ = outputWriter.CloseWithError(err)
		_ = reader.CloseWithError(err)
		done <- err
	}()
	defer func() {
		_ = writer.Close()
		_ = outputReader.Close()
		if err := <-done; err != nil {
			t.Errorf("MCP: %v", err)
		}
	}()
	encoder, decoder := json.NewEncoder(writer), json.NewDecoder(outputReader)
	call := func(id int, method string, params any) json.RawMessage {
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := decoder.Decode(&response); err != nil || response.ID != id || len(response.Error) > 0 {
			t.Fatal("MCP protocol response unavailable")
		}
		return response.Result
	}
	call(1, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "subagent-parity", "version": "test"}})
	arguments := make(map[string]any, len(document)+1)
	for key, value := range document {
		arguments[key] = value
	}
	if _, supplied := arguments["action"]; !supplied {
		arguments["action"] = "START"
	}
	result := decodeRequesterMCPResult(t, call(2, "tools/call", map[string]any{"name": "you.subagent", "arguments": arguments}))
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
