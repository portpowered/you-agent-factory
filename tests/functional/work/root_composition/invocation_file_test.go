package root_composition_test

import (
	"context"
	"encoding/json"
	"fmt"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// File invocation owns the public CLI preparation contract. The hosted sessions
// let each parallel leaf inspect its own terminal Work and rejected admissions.
func TestWorkInvocationFilePreparationPreservesPublicResults(t *testing.T) {
	t.Parallel()
	cases := newFlushCases(t)
	for index, scenario := range cases {
		scenario.name = []string{"regular file", "missing file", "directory"}[index]
		factoryPath := filepath.Join(scenario.dir, "factory.json")
		data, err := os.ReadFile(factoryPath)
		if err != nil {
			t.Fatal(err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatal(err)
		}
		cfg["workTypes"].([]any)[0].(map[string]any)["handlingBehavior"] = []string{"DEFAULT"}
		cfg["invocationSignature"] = map[string]any{"unknownNamedArgumentPolicy": "REJECT", "parameters": []any{map[string]any{"name": "document", "required": true, "typeHint": "FILE_PATH", "valueMode": "FILE_CONTENTS", "bindings": []any{map[string]any{"kind": "NAMED"}}}}}
		data, err = json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(factoryPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		support.WriteWorkstationConfig(t, scenario.dir, "process-task", "---\ntype: MODEL_WORKSTATION\n---\n${document}")
	}
	runner := support.NewStaticSuccessCommandRunner("file preparation COMPLETE")
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: flushCommandRunner{run: func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
			for _, scenario := range cases {
				if request.ExecutionScopeID == scenario.sessionID {
					scenario.dispatches.Add(1)
					want := fileInvocationText(scenario.sessionID)
					if string(request.Stdin) != want {
						return platformprocess.CommandResult{}, fmt.Errorf("provider input = %q, want %q", request.Stdin, want)
					}
					return runner.Run(ctx, request)
				}
			}
			return platformprocess.CommandResult{}, fmt.Errorf("unowned provider scope %q", request.ExecutionScopeID)
		}},
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			for _, scenario := range cases {
				if request.Port == scenario.port {
					return scenario.api.Start(ctx, request)
				}
			}
			return fmt.Errorf("unexpected listener port %d", request.Port)
		},
	})
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) { t.Parallel(); runFileInvocationCase(t, process, scenario) })
	}
}
func fileInvocationText(sessionID string) string {
	return "first line — 東京\r\n" + sessionID + "\nlast line"
}
func runFileInvocationCase(t *testing.T, process support.Process, scenario *flushCase) {
	t.Helper()
	baseURL, env := startFileInvocationHost(t, process, scenario)
	input, err := executeFileInvocation(t, process, scenario, baseURL, env)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, scenario.sessionID, "/work"))
	if scenario.name != "regular file" {
		assertFileInvocationRejected(t, scenario, input, err, listed)
		return
	}
	assertFileInvocationCompleted(t, scenario, input, err, listed)
}

func startFileInvocationHost(t *testing.T, process support.Process, scenario *flushCase) (string, []string) {
	t.Helper()
	env := recoveryActivationHomeEnvironment(scenario.home)
	support.InitializeCustomerHomeWithProcess(t, process, env, scenario.dir)
	ctx, cancel := context.WithCancel(t.Context())
	host := support.FakeInputs(ctx, []string{"you", "run", "--dir", scenario.dir, "--session", scenario.sessionID, "--continuously", "--with-server", "--server", "http://127.0.0.1:" + strconv.Itoa(scenario.port), "--no-record", "--quiet"})
	host.Input.Env, host.Input.WorkingDirectory = env, scenario.dir
	command := support.StartProcessCommand(t, process, host.Input)
	t.Cleanup(func() { cancel(); command.Stop(t) })
	return scenario.api.WaitForURL(t), env
}

func executeFileInvocation(t *testing.T, process support.Process, scenario *flushCase, baseURL string, env []string) (*support.CapturedInputs, error) {
	t.Helper()
	path := filepath.Join(scenario.dir, "owned input.txt")
	switch scenario.name {
	case "regular file":
		if err := os.WriteFile(path, []byte(fileInvocationText(scenario.sessionID)), 0o600); err != nil {
			t.Fatal(err)
		}
	case "directory":
		path = scenario.dir
	}
	input := support.FakeInputs(t.Context(), []string{"you", "--remote", "--server", baseURL, "run", "--factory", filepath.Join(scenario.dir, "factory.json"), "--session", scenario.sessionID, "--no-record", "--quiet", "--document", path})
	input.Input.Env, input.Input.WorkingDirectory = env, scenario.dir
	input.Input.Stdin = strings.NewReader("")
	err := process.Execute(input.Input)
	return input, err
}

func assertFileInvocationRejected(t *testing.T, scenario *flushCase, input *support.CapturedInputs, err error, listed factoryapi.ListWorkResponse) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "REMOTE_DURABLE_START_FAILED") || !strings.Contains(err.Error(), "(400)") || !strings.Contains(err.Error(), "could not read FILE_CONTENTS path") {
		t.Fatalf("file diagnostic = %v, stderr=%s", err, input.Stderr())
	}
	if len(listed.Results) != 0 || scenario.dispatches.Load() != 0 {
		t.Fatalf("rejected input admitted Work/dispatch = %#v/%d", listed, scenario.dispatches.Load())
	}
}

func assertFileInvocationCompleted(t *testing.T, scenario *flushCase, input *support.CapturedInputs, err error, listed factoryapi.ListWorkResponse) {
	t.Helper()
	if err != nil || !strings.Contains(input.Stdout(), "file preparation") {
		t.Fatalf("invocation output=%q, stderr=%q, error=%v", input.Stdout(), input.Stderr(), err)
	}
	if scenario.dispatches.Load() != 1 || len(listed.Results) != 1 {
		t.Fatalf("completed Work/dispatch = %#v/%d", listed, scenario.dispatches.Load())
	}
	assertFileInvocationTerminalWork(t, listed)
}

func assertFileInvocationTerminalWork(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	item := listed.Results[0]
	if item.State == nil || item.State.Name != "complete" || item.WorkId == nil || *item.WorkId == "" || item.RequestId == nil || *item.RequestId == "" || item.CurrentChainingTraceId == nil || *item.CurrentChainingTraceId == "" {
		t.Fatalf("terminal Work lost identity/lineage = %#v", item)
	}
	content, err := json.Marshal(item.Content)
	if err != nil || !strings.Contains(string(content), "file preparation") {
		t.Fatalf("terminal content = %s, error = %v", content, err)
	}
}
