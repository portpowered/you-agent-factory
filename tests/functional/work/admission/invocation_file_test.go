package admission_test

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

// File submission uses real owned files and HTTP with the parent's reusable
// process and controlled provider command runner. Each parallel case owns its
// sessions; failed requests must never create Work or dispatch events.
func runFileSubmissionSessions(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()
	t.Run("success isolation", func(t *testing.T) {
		t.Parallel()
		runFileSubmissionIsolation(t, server)
	})
	for _, name := range []string{"read", "parse", "reject", "session"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runFileSubmissionFailure(t, server, name)
		})
	}
}

func runFileSubmissionIsolation(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()
	a, closeA := openFileSubmissionSession(t, server)
	b, closeB := openFileSubmissionSession(t, server)
	t.Run("concurrent submissions", func(t *testing.T) {
		for _, session := range []string{a, b} {
			t.Run(session, func(t *testing.T) {
				t.Parallel()
				input, err := submitOwnedBatchFile(t, server, session, fileSubmissionJSON(session), false)
				if err != nil {
					t.Fatalf("submit: %v; stdout=%s; stderr=%s", err, input.Stdout(), input.Stderr())
				}
				assertBatchSubmitAcknowledgment(t, []byte(input.Stdout()), session, "item")
			})
		}
	})
	for _, session := range []string{a, b} {
		listed := listRelationshipSessionWork(t, server.URL(), session)
		if len(listed.Results) != 1 || support.StringPointerValue(listed.Results[0].WorkId) != "work-"+session || support.StringPointerValue(listed.Results[0].RequestId) != session {
			t.Fatalf("session %s Work = %#v", session, listed)
		}
		item := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(server.URL(), session, "/work/work-"+session))
		if support.StringPointerValue(item.RequestId) != session {
			t.Fatalf("get Work = %#v", item)
		}
		assertFileSubmissionEvents(t, server, session, session, true)
	}
	closeA()
	if len(listRelationshipSessionWork(t, server.URL(), b).Results) != 1 {
		t.Fatal("closing peer changed live session")
	}
	closeB()
}

func runFileSubmissionFailure(t *testing.T, server *support.FunctionalAPIServer, name string) {
	t.Helper()
	peer, _ := openFileSubmissionSession(t, server)
	selected := peer
	body := fileSubmissionJSON(peer)
	diagnostic := ""
	switch name {
	case "read":
		diagnostic = "batch file not found:"
	case "parse":
		body, diagnostic = "{", "parse"
	case "reject":
		body, diagnostic = strings.TrimSuffix(body, `]}`)+`,{"name":"invalid","workTypeName":"unknown"}]}`, "unknown"
	case "session":
		selected, diagnostic = "00000000-0000-4000-8000-000000000099", "not_found"
	}
	input, err := submitOwnedBatchFile(t, server, selected, body, name == "read")
	assertFileSubmissionDiagnostic(t, name, diagnostic, peer, input, err)
	if strings.Contains(input.Stdout(), `"accepted":true`) {
		t.Fatalf("failed file acknowledged: %s", input.Stdout())
	}
	if got := listRelationshipSessionWork(t, server.URL(), peer); len(got.Results) != 0 {
		t.Fatalf("failed file created Work: %#v", got)
	}
	assertFileSubmissionEvents(t, server, peer, peer, false)
}

func assertFileSubmissionDiagnostic(t *testing.T, name, diagnostic, peer string, input *support.CapturedInputs, err error) {
	t.Helper()
	if err == nil || !strings.Contains(strings.ToLower(err.Error()+input.Stderr()), diagnostic) {
		t.Fatalf("%s diagnostic = %v, stderr=%s", name, err, input.Stderr())
	}
	if name == "session" {
		for _, marker := range []string{"(404)", "code=NOT_FOUND", "family=NOT_FOUND"} {
			if !strings.Contains(err.Error()+input.Stderr(), marker) {
				t.Fatalf("session diagnostic missing %q: %v; %s", marker, err, input.Stderr())
			}
		}
	}
	if name == "reject" {
		assertBatchSubmitRejected(t, []byte(input.Stderr()), err, peer)
	}
}

func openFileSubmissionSession(t *testing.T, server *support.FunctionalAPIServer) (string, func()) {
	t.Helper()
	dir := support.ScaffoldFactory(t, batchWorkTypeSelectionFactoryConfig())
	configureSubmissionCodexWorkers(t, dir, "mock-worker")
	session, closeSession := openSharedRelationshipSession(t, server.URL(), dir)
	return session.Id, closeSession
}

func fileSubmissionJSON(id string) string {
	return fmt.Sprintf(`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":"item","workId":%q,"workTypeName":"task","payload":{"title":"owned file"}}]}`, id, "work-"+id)
}

func submitOwnedBatchFile(t *testing.T, server *support.FunctionalAPIServer, session, body string, missing bool) (*support.CapturedInputs, error) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "request.json")
	if !missing {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	input := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "--session", session, "--json", "submit", "batch", path})
	input.Input.Env = recoveryActivationHomeEnvironment(home)
	input.Input.WorkingDirectory = home
	err := server.Execute(t, input.Input)
	return input, err
}

func assertFileSubmissionEvents(t *testing.T, server *support.FunctionalAPIServer, session, request string, accepted bool) {
	t.Helper()
	found := 0
	for _, event := range support.GetFactoryEventsForSessionAt(t, server.URL(), session) {
		if event.Type == factoryapi.FactoryEventTypeWorkRequest {
			if !accepted || support.StringPointerValue(event.Context.RequestId) != request {
				t.Fatalf("unexpected request event: %#v", event)
			}
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil {
				t.Fatal(err)
			}
			works := support.FactoryWorksValue(payload.Works)
			if len(works) != 1 || support.StringPointerValue(works[0].WorkId) != "work-"+request {
				t.Fatalf("request facts = %#v", payload)
			}
			found++
		}
		// RUN_REQUEST also records session startup without a Work dispatch.
		if !accepted && (event.Context.DispatchId != nil || event.Type == factoryapi.FactoryEventTypeModelRequest) {
			t.Fatalf("failed file dispatched: type=%s context=%#v", event.Type, event.Context)
		}
	}
	if accepted && found != 1 {
		t.Fatalf("request events = %d, want 1", found)
	}
}
