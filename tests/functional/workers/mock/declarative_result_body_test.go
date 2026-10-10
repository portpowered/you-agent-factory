package mock

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// All rows share an immutable effect-denying root process. CLI invocations own
// UUID sessions, homes, config, gates and output; the idle host has no Work.
func TestDeclarativeAcceptResultBody(t *testing.T) {
	t.Parallel()
	denied := &directExampleDeniedRunner{}
	var native atomic.Int32
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: publishedDirectMockFactory(t), WaitForServiceModeRuntime: true,
		Env:  sharedWorkersMockEnvironment(t, publishedDirectMockHome(t)),
		Args: []string{"--with-mock-workers"}, DeniedProcessAttempts: &native,
		Edges: serviceedges.Edges{ProviderCommandRunner: denied, ScriptCommandRunner: denied,
			ProvidersStdioPipeFactory: func() (platformprocess.StdioChannel, error) { native.Add(1); return nil, errors.New("ACP denied") },
		},
	})
	t.Cleanup(func() {
		if denied.calls.Load() != 0 || native.Load() != 0 {
			t.Errorf("external effects: command=%d native=%d", denied.calls.Load(), native.Load())
		}
	})
	for _, name := range []string{"F-M1-declared-gated-usage", "F-M2-business-failure-sibling", "F-M3-omitted-unmatched", "F-M4-invalid-preflight", "F-M5-opaque-unknown", "F-M6-concurrent-isolation"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if strings.HasPrefix(name, "F-M6") {
				runConcurrentDeclaredResults(t, host)
				return
			}
			runDeclaredResultScenario(t, host, name, nil)
		})
	}
}

func runDeclaredResultScenario(t *testing.T, host *support.FunctionalAPIServer, name string, rendezvous func(*testing.T)) {
	dir := declaredResultFactory(t)
	id := uuid.NewString()
	marker := name + "-" + id
	gate := support.NewMockWorkerGate(t)
	body := json.RawMessage(`{"decision":"ACCEPTED","output":"` + marker + `"}`)
	businessFailure := strings.HasPrefix(name, "F-M2")
	if businessFailure {
		body = json.RawMessage(`{"decision":"ACCEPTED","output":"` + marker + `","recorded_output_work":[{"workTypeId":"undeclared-output","state":"complete","content":[{"type":"text","text":"invalid proposal"}]}]}`)
	}
	if strings.HasPrefix(name, "F-M3") {
		body = nil
		marker = "mock worker accepted"
	}
	path := declaredResultConfig(t, dir, name, body, gate)
	tokens := int64(11)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{WorkID: "target", WorkTypeID: "task", TraceID: id, Payload: []byte(marker)})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{WorkID: "sibling", WorkTypeID: "task", TraceID: id + "-sibling", Payload: []byte("sibling input")})
	ctx, cancel := context.WithCancel(t.Context())
	args := []string{"you", "run", "--session=" + id, "--dir", dir, "--with-mock-workers=" + path, "--continuously", "--no-record"}
	if strings.HasPrefix(name, "F-M5") {
		args = append(args, "--verbose")
	} else {
		args = append(args, "--quiet")
	}
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = sharedWorkersMockEnvironment(t, publishedDirectMockHome(t))
	if strings.HasPrefix(name, "F-M4") {
		assertDeclaredSessionAbsent(t, host, id)
	}
	done := make(chan error, 1)
	joined := make(chan struct{})
	go func() { defer close(joined); done <- host.Execute(t, inputs.Input) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-joined:
			if t.Failed() {
				t.Logf("CLI stderr: %s", inputs.Stderr())
			}
		case <-time.After(30 * time.Second):
			t.Error("CLI did not join")
		}
	})
	if strings.HasPrefix(name, "F-M4") {
		assertDeclaredInvalidPreflight(t, host, id, done, inputs)
		return
	}
	gate.WaitForArrival(t, 30*time.Second)
	base := host.URL() + "/factory-sessions/" + url.PathEscape(id)
	sessions := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, base+"/worker-sessions?workId=target")
	if len(sessions.Sessions) != 1 || sessions.Sessions[0].EndedAt != nil {
		t.Fatalf("gate did not retain one active execution: %+v", sessions)
	}
	workerID := sessions.Sessions[0].WorkerSessionId
	if rendezvous != nil {
		rendezvous(t)
	}
	gate.Release()
	support.WaitForSessionTerminalStatus(t, host.URL(), id, 20*time.Second)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, base+"/work")
	assertDeclaredWork(t, listed, businessFailure)
	events := support.GetFactoryEventsForSessionAt(t, host.URL(), id)
	assertDeclaredDispatch(t, events, businessFailure, marker)
	assertDeclaredCapture(t, host, workerID, id, marker, tokens)
	assertDeclaredSiblingCapture(t, host, base, workerID, marker)
	if strings.HasPrefix(name, "F-M5") {
		cancel()
		select {
		case <-joined:
		case <-time.After(30 * time.Second):
			t.Fatal("diagnostic invocation did not join")
		}
		assertDeclaredOpaqueCapture(t, host, workerID)
	}
}

func declaredResultFactory(t *testing.T) string {
	t.Helper()
	dir := publishedDirectMockFactory(t)
	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["workstations"].([]any)[0].(map[string]any)["outcomeFormat"] = "decision-envelope"
	data, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertDeclaredWork(t *testing.T, listed factoryapi.ListWorkResponse, failed bool) {
	t.Helper()
	if len(listed.Results) != 2 {
		t.Fatalf("Work count = %d, want target and sibling only", len(listed.Results))
	}
	for _, item := range listed.Results {
		want := factoryapi.WorkStateTypeTERMINAL
		if item.WorkId != nil && *item.WorkId == "target" && failed {
			want = factoryapi.WorkStateTypeFAILED
		}
		if item.State == nil || item.State.Type != want {
			t.Fatalf("Work outcome = %+v, want %s", item, want)
		}
	}
}

func assertDeclaredDispatch(t *testing.T, events []factoryapi.FactoryEvent, failed bool, marker string) {
	t.Helper()
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 2 {
		t.Fatalf("dispatches = %d, want two", len(dispatches))
	}
	for _, dispatch := range dispatches {
		if dispatch.Response == nil {
			t.Fatal("missing dispatch response")
		}
		target := support.DispatchObservationIncludesWork(dispatch, "target")
		want := factoryapi.WorkOutcomeAccepted
		if target && failed {
			want = factoryapi.WorkOutcomeFailed
		}
		if dispatch.Response.Outcome != want {
			t.Fatalf("dispatch outcome = %+v", dispatch.Response)
		}
		if !target && support.StringPointerValue(dispatch.Response.Output) != "mock worker accepted" {
			t.Fatalf("unmatched sibling output changed: %q", support.StringPointerValue(dispatch.Response.Output))
		}
		if target && !strings.Contains(support.StringPointerValue(dispatch.Response.Output), marker) {
			t.Fatalf("declared output = %q, want %q", support.StringPointerValue(dispatch.Response.Output), marker)
		}
		if target && failed && !strings.Contains(support.StringPointerValue(dispatch.Response.Error), "undeclared-output") {
			t.Fatalf("missing materialization error: %+v", dispatch.Response)
		}
	}
}

func assertDeclaredCapture(t *testing.T, host *support.FunctionalAPIServer, workerID, sessionID, marker string, tokens int64) {
	t.Helper()
	observation, err := support.WaitForObservation(5*time.Second, func() (factoryapi.WorkerSessionObservation, error) {
		return support.GetJSON[factoryapi.WorkerSessionObservation](t, host.URL()+"/worker-sessions/"+workerID), nil
	}, func(o factoryapi.WorkerSessionObservation) bool { return o.EndedAt != nil })
	if err != nil {
		t.Fatal(err)
	}
	if observation.State != "COMPLETED" || observation.TerminalCause == nil || *observation.TerminalCause != "COMPLETED" || observation.FactorySessionId == nil || *observation.FactorySessionId != sessionID {
		t.Fatalf("execution state=%s cause=%s scope=%s wantScope=%s", observation.State, support.StringPointerValue((*string)(observation.TerminalCause)), support.StringPointerValue(observation.FactorySessionId), sessionID)
	}
	assertDeclaredUsage(t, observation.TokenUsage, tokens)
	page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, host.URL()+"/worker-sessions/"+workerID+"/logs")
	data, err := json.Marshal(page.Events)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), marker) || page.Health != "COMPLETE" {
		t.Fatalf("captured logs = %s, health=%s", data, page.Health)
	}
}

func assertDeclaredInvalidPreflight(t *testing.T, host *support.FunctionalAPIServer, id string, done <-chan error, inputs *support.CapturedInputs) {
	t.Helper()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(inputs.Stderr()+err.Error(), "resultBody") {
			t.Fatalf("invalid config = %v %s", err, inputs.Stderr())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("invalid config did not return")
	}
	assertDeclaredSessionAbsent(t, host, id)
}

func declaredResultConfig(t *testing.T, dir, name string, body json.RawMessage, gate *support.MockWorkerGate) string {
	t.Helper()
	entry := workers.MockWorkerConfig{RunType: workers.MockWorkerRunTypeAccept, WorkInputs: []workers.MockWorkInputSelector{{WorkID: "target"}}, ResultBody: body, GateConfig: gate.Config(30 * time.Second)}
	zero, tokens := int64(0), int64(11)
	entry.Usage = &workers.MockWorkerUsageConfig{Provider: "codex", Model: "gpt-5", InputTokens: &tokens, OutputTokens: &zero}
	config := workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{entry}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(name, "F-M5") {
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		doc["futureTop"] = "synthetic-secret-top"
		row := doc["mockWorkers"].([]any)[0].(map[string]any)
		row["futureEntry"] = "synthetic-secret-entry"
		row["resultBody"].(map[string]any)["opaque"] = map[string]any{"futurePayload": true}
		data, err = json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "mock.json")
	if strings.HasPrefix(name, "F-M4") {
		data = []byte(`{"mockWorkers":[{"runType":"accept","resultBody":[]}]}`)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Neither session can finish its target until both independent executions
// reach their private gates. This proves overlapping Process.Execute calls.
func runConcurrentDeclaredResults(t *testing.T, host *support.FunctionalAPIServer) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	go func() {
		defer close(release)
		for i := 0; i < 2; i++ {
			select {
			case <-arrived:
			case <-ctx.Done():
				return
			}
		}
	}()
	ready := func(t *testing.T) {
		arrived <- struct{}{}
		select {
		case <-release:
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for _, name := range []string{"F-M6-alpha", "F-M6-beta"} {
		t.Run(name, func(t *testing.T) { t.Parallel(); runDeclaredResultScenario(t, host, name, ready) })
	}
}

func assertDeclaredUsage(t *testing.T, usage *factoryapi.ProviderSessionTokenUsage, tokens int64) {
	t.Helper()
	if usage == nil || usage.InputTokens == nil || int64(*usage.InputTokens) != tokens || usage.OutputTokens == nil || *usage.OutputTokens != 0 {
		t.Fatalf("usage = %+v", usage)
	}
}

func assertDeclaredSessionAbsent(t *testing.T, host *support.FunctionalAPIServer, id string) {
	t.Helper()
	response, err := http.Get(host.URL() + "/factory-sessions/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("invalid config admitted session: %d", response.StatusCode)
	}
}

func assertDeclaredSiblingCapture(t *testing.T, host *support.FunctionalAPIServer, base, targetID, marker string) {
	t.Helper()
	rows := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, base+"/worker-sessions?workId=sibling")
	if len(rows.Sessions) != 1 || rows.Sessions[0].WorkerSessionId == targetID {
		t.Fatalf("sibling association = %+v", rows)
	}
	sibling := rows.Sessions[0]
	if sibling.State != "COMPLETED" || sibling.TokenUsage != nil {
		t.Fatalf("sibling inherited target outcome/usage: %+v", sibling)
	}
	page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, host.URL()+"/worker-sessions/"+sibling.WorkerSessionId+"/logs")
	data, err := json.Marshal(page.Events)
	if err != nil {
		t.Fatal(err)
	}
	if page.Health != "COMPLETE" || !strings.Contains(string(data), "mock worker accepted") {
		t.Fatalf("default sibling capture = %s", data)
	}
	if marker != "mock worker accepted" && strings.Contains(string(data), marker) {
		t.Fatal("sibling captured another dispatch's declared body")
	}
}

// The verbose CLI's unknown-field warning is emitted to the real terminal
// logger, outside FakeInputs. Its path-only content is checked in the focused
// runner output; codec tests protect the metadata. This row protects opacity
// through the public captured logs as well as successful business processing.
func assertDeclaredOpaqueCapture(t *testing.T, host *support.FunctionalAPIServer, workerID string) {
	t.Helper()
	page := support.GetJSON[factoryapi.WorkerSessionLogPage](t, host.URL()+"/worker-sessions/"+workerID+"/logs")
	data, err := json.Marshal(page.Events)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"opaque", "futurePayload"} {
		if !strings.Contains(string(data), key) {
			t.Fatalf("opaque declared payload key %q missing from capture", key)
		}
	}
}
