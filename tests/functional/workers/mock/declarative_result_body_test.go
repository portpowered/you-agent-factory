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
	for _, mode := range []string{"inference", "agent", "batch"} {
		for _, flag := range []string{"equals", "optional-path"} {
			t.Run("matched-rejection/"+mode+"/"+flag, func(t *testing.T) {
				t.Parallel()
				runMatchedMockRejection(t, host, mode, flag)
			})
		}
	}
	for _, mode := range []string{"inference", "agent"} {
		t.Run("empty-config/"+mode, func(t *testing.T) {
			t.Parallel()
			runTypedEmptyMockAcceptance(t, host, mode)
		})
	}
	for _, flag := range []string{"equals", "optional-path"} {
		t.Run("topology-only/"+flag, func(t *testing.T) {
			t.Parallel()
			runTopologyOnlyMockConfig(t, host, flag)
		})
	}
}

// B1's name-only worker is a supported topology placeholder, not an execution
// definition. Preserve that public routing behavior alongside the typed
// rejection/gate witnesses: mock policy must not invent a provider attempt.
func runTopologyOnlyMockConfig(t *testing.T, host *support.FunctionalAPIServer, flag string) {
	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes":    []map[string]any{batchWorkTypeConfig("task")},
		"workers":      []map[string]string{{"name": "processor"}},
		"workstations": []map[string]any{batchWorkstationConfig("process-task", "processor", "task", "complete", "failed")},
	})
	// Match the original factory.json-only fixture, without synthesizing an
	// execution definition in the scaffolder's default workstation document.
	if err := os.Remove(filepath.Join(dir, "workstations", "process-task", "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	gate := support.NewMockWorkerGate(t)
	path := writeBatchMockWorkersConfig(t, workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		ID: "topology-rejection", RunType: workers.MockWorkerRunTypeReject,
		RejectConfig: &workers.MockWorkerRejectConfig{Stderr: configuredRejectStderr}, GateConfig: gate.Config(30 * time.Second),
	}}})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{WorkID: "target", WorkTypeID: "task", TraceID: id, Payload: []byte("preserved topology payload")})
	args := []string{"you", "run", "--session=" + id, "--dir", dir, "--continuously", "--quiet", "--no-record"}
	if flag == "equals" {
		args = append(args, "--with-mock-workers="+path)
	} else {
		args = append(args, "--with-mock-workers", path)
	}
	ctx, cancel := context.WithCancel(t.Context())
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.WorkingDirectory, inputs.Input.Env = dir, sharedWorkersMockEnvironment(t, publishedDirectMockHome(t))
	joined := make(chan struct{})
	go func() { defer close(joined); _ = host.Execute(t, inputs.Input) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-joined:
		case <-time.After(30 * time.Second):
			t.Error("topology invocation did not join")
		}
	})
	support.WaitForSessionTerminalStatus(t, host.URL(), id, 20*time.Second)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, host.URL()+"/factory-sessions/"+id+"/work")
	if support.CountWorkAtCustomerState(listed, "task:complete") != 1 || support.CountWorkAtCustomerState(listed, "task:failed") != 0 {
		t.Fatalf("topology Work outcome = %+v", listed)
	}
	dispatches := support.ObserveDispatchEvents(t, support.GetFactoryEventsForSessionAt(t, host.URL(), id))
	if len(dispatches) != 1 || dispatches[0].Response == nil || dispatches[0].Response.Outcome != factoryapi.WorkOutcomeAccepted ||
		support.StringPointerValue(dispatches[0].Response.Output) != "" || !support.DispatchObservationIncludesWork(dispatches[0], "target") {
		t.Fatalf("topology dispatch = %+v, want accepted correlated traversal without provider output", dispatches)
	}
	if _, err := os.Stat(gate.Config(30 * time.Second).ArrivedFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("topology traversal reached mock execution gate: %v", err)
	}
}

// M3 observes default acceptance with an explicitly empty configuration on
// both typed paths. The shared host denies every native execution effect.
func runTypedEmptyMockAcceptance(t *testing.T, host *support.FunctionalAPIServer, mode string) {
	dir := matchedMockRejectionFactory(t, mode)
	id := uuid.NewString()
	for _, name := range []string{"target", "sibling"} {
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkID: name, WorkTypeID: "task", TraceID: id + "-" + name, Payload: []byte(name),
		})
	}
	path := filepath.Join(dir, "empty-mock.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--session=" + id, "--dir", dir,
		"--with-mock-workers=" + path, "--continuously", "--quiet", "--no-record"})
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = sharedWorkersMockEnvironment(t, publishedDirectMockHome(t))
	joined := make(chan struct{})
	go func() { defer close(joined); _ = host.Execute(t, inputs.Input) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-joined:
		case <-time.After(30 * time.Second):
			t.Error("empty-mock invocation did not join")
		}
	})
	// Continuous execution retains the session for public terminal inspection;
	// this wait observes committed Work state rather than delaying the command.
	support.WaitForSessionTerminalStatus(t, host.URL(), id, 20*time.Second)
	base := host.URL() + "/factory-sessions/" + url.PathEscape(id)
	assertDeclaredWork(t, support.GetJSON[factoryapi.ListWorkResponse](t, base+"/work"), false)
	dispatches := support.ObserveDispatchEvents(t, support.GetFactoryEventsForSessionAt(t, host.URL(), id))
	if len(dispatches) != 2 {
		t.Fatalf("default acceptance dispatch count = %d, want two", len(dispatches))
	}
	seen := map[string]bool{}
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != "process-task" || dispatch.Response == nil ||
			dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted ||
			support.StringPointerValue(dispatch.Response.Output) != "mock worker accepted" ||
			dispatch.Response.FailureDetail != nil || dispatch.Response.Error != nil {
			t.Fatalf("empty-config terminal dispatch = %+v", dispatch)
		}
		for _, name := range []string{"target", "sibling"} {
			if support.DispatchObservationIncludesWork(dispatch, name) {
				if seen[name] {
					t.Fatalf("duplicate default acceptance for %s", name)
				}
				seen[name] = true
			}
		}
	}
	if !seen["target"] || !seen["sibling"] {
		t.Fatalf("default acceptance Work correlation = %v", seen)
	}
}

// The matched worker/workstation pair and both documented flag forms must
// select rejection rather than the host's empty-config acceptance. Each row
// owns its profile, UUID session and gate; all rows reuse the effect-denying
// process above. Public dispatch/Work outcomes prove assembled routing, which
// an isolated runner test cannot establish.
func matchedMockRejectionFactory(t *testing.T, mode string) string {
	t.Helper()
	workerType, workstationType := "INFERENCE_WORKER", "INFERENCE_RUN"
	if mode == "agent" {
		workerType, workstationType = "AGENT_WORKER", "AGENT_RUN"
	}
	workerConfig := map[string]any{"name": "processor", "type": workerType, "modelProvider": "CODEX", "model": "gpt-5", "executorProvider": "SCRIPT_WRAP"}
	workstationConfig := batchWorkstationConfig("process-task", "processor", "task", "complete", "failed")
	workstationConfig["type"] = workstationType
	if mode != "agent" {
		workerConfig["operations"] = []map[string]any{{"name": "OMNI",
			"inputs":  []map[string]any{{"name": "prompt", "contentTypes": []string{"TEXT"}, "required": true}},
			"outputs": []map[string]any{{"name": "completion", "contentTypes": []string{"TEXT"}}},
		}}
		workstationConfig["operation"] = "OMNI"
		workstationConfig["operationBindings"] = []map[string]any{{"slot": "prompt", "defaultContent": []map[string]string{{"type": "TEXT", "text": "Process the requested Work."}}}}
	}
	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes":    []map[string]any{batchWorkTypeConfig("task")},
		"workers":      []map[string]any{workerConfig},
		"workstations": []map[string]any{workstationConfig},
	})
	support.WriteAgentConfig(t, dir, "processor", "---\ntype: "+workerType+"\nmodelProvider: CODEX\nmodel: gpt-5\nexecutorProvider: SCRIPT_WRAP\n---\nProcess the requested Work.\n")
	workstationPath := filepath.Join(dir, "workstations", "process-task", "AGENTS.md")
	if err := os.WriteFile(workstationPath, []byte("---\ntype: "+workstationType+"\n---\nProcess {{ (index .Inputs 0).Payload }}.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runMatchedMockRejection(t *testing.T, host *support.FunctionalAPIServer, mode, flag string) {
	dir := matchedMockRejectionFactory(t, mode)
	id := uuid.NewString()
	args := []string{"you"}
	if mode == "batch" {
		args = append(args, "--json")
	}
	args = append(args, "run", "--session="+id, "--dir", dir, "--no-record")
	config := workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		ID: "matched-rejection", WorkerName: "processor", WorkstationName: "process-task",
		RunType:      workers.MockWorkerRunTypeReject,
		RejectConfig: &workers.MockWorkerRejectConfig{Stdout: configuredRejectStdout, Stderr: configuredRejectStderr},
	}}}
	var gate *support.MockWorkerGate
	if mode == "batch" {
		args = append(args, "--work", writeBatchWorksWithTypes(t,
			batchWorkSpec{Name: "first rejected Work", WorkTypeID: "task"},
			batchWorkSpec{Name: "second rejected Work", WorkTypeID: "task"}))
	} else {
		gate = support.NewMockWorkerGate(t)
		config.MockWorkers[0].WorkInputs = []workers.MockWorkInputSelector{{WorkID: "target"}}
		config.MockWorkers[0].GateConfig = gate.Config(30 * time.Second)
		for _, name := range []string{"target", "sibling"} {
			testutil.WriteSeedRequest(t, dir, work.SubmitRequest{WorkID: name, WorkTypeID: "task", TraceID: id + "-" + name, Payload: []byte(name)})
		}
		args = append(args, "--continuously", "--quiet")
	}
	path := writeBatchMockWorkersConfig(t, config)
	if flag == "equals" {
		args = append(args, "--with-mock-workers="+path)
	} else {
		args = append(args, "--with-mock-workers", path)
	}
	ctx, cancel := context.WithCancel(t.Context())
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = sharedWorkersMockEnvironment(t, publishedDirectMockHome(t))
	done := make(chan error, 1)
	joined := make(chan struct{})
	go func() { defer close(joined); done <- host.Execute(t, inputs.Input) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-joined:
			if t.Failed() {
				t.Logf("matched rejection stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
			}
		case <-time.After(30 * time.Second):
			t.Error("matched rejection invocation did not join")
		}
	})
	if mode == "batch" {
		assertMatchedMockBatchFailure(t, done, inputs)
		return
	}
	assertMatchedMockGatedFailure(t, host, id, gate)
}

func assertMatchedMockBatchFailure(t *testing.T, done <-chan error, inputs *support.CapturedInputs) {
	t.Helper()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("matched batch rejection succeeded: %s", inputs.Stdout())
		}
		if strings.TrimSpace(inputs.Stdout()) == "" {
			t.Fatalf("matched batch produced no report: %v stderr=%q", err, inputs.Stderr())
		}
	case <-time.After(30 * time.Second):
		t.Fatal("matched batch rejection did not finish")
	}
	report := decodeBatchProcessReport(t, inputs.Stdout())
	if report.Status != "FAILED" || len(report.Failures) != 2 {
		t.Fatalf("matched batch report = %+v, want two failed Works", report)
	}
	failures := map[string]bool{}
	for _, failure := range report.Failures {
		if failure.WorkState != "task:failed" || !strings.Contains(failure.Reason, stableProviderRefusalErr) {
			t.Fatalf("matched batch failure = %+v, want provider refusal", failure)
		}
		failures[failure.WorkName] = true
	}
	if !failures["first rejected Work"] || !failures["second rejected Work"] {
		t.Fatalf("matched batch failures = %+v", report.Failures)
	}
}

func assertMatchedMockGatedFailure(t *testing.T, host *support.FunctionalAPIServer, id string, gate *support.MockWorkerGate) {
	t.Helper()
	gate.WaitForArrival(t, 30*time.Second)
	base := host.URL() + "/factory-sessions/" + url.PathEscape(id)
	active := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, base+"/worker-sessions?workId=target")
	if len(active.Sessions) != 1 || active.Sessions[0].EndedAt != nil {
		t.Fatalf("matched gate did not hold target execution: %+v", active)
	}
	for _, dispatch := range support.ObserveDispatchEvents(t, support.GetFactoryEventsForSessionAt(t, host.URL(), id)) {
		if support.DispatchObservationIncludesWork(dispatch, "target") && dispatch.Response != nil {
			t.Fatalf("target returned before gate release: %+v", dispatch.Response)
		}
	}
	gate.Release()
	support.WaitForSessionTerminalStatus(t, host.URL(), id, 20*time.Second)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, base+"/work")
	assertDeclaredWork(t, listed, true)
	dispatches := support.ObserveDispatchEvents(t, support.GetFactoryEventsForSessionAt(t, host.URL(), id))
	if len(dispatches) != 2 {
		t.Fatalf("matched dispatch count = %d, want target and sibling", len(dispatches))
	}
	var targetCount, siblingCount int
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != "process-task" {
			t.Fatalf("matched workstation = %q", dispatch.Request.TransitionId)
		}
		if support.DispatchObservationIncludesWork(dispatch, "target") {
			targetCount++
			assertStableMockRejectDispatch(t, dispatch)
		} else if !support.DispatchObservationIncludesWork(dispatch, "sibling") || dispatch.Response == nil ||
			dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted || support.StringPointerValue(dispatch.Response.Output) != "mock worker accepted" {
			t.Fatalf("unmatched sibling inherited rejection: %+v", dispatch)
		} else {
			siblingCount++
		}
	}
	if targetCount != 1 || siblingCount != 1 {
		t.Fatalf("matched dispatch correlation: target=%d sibling=%d, want one each", targetCount, siblingCount)
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
		body, marker = businessInvalidResultExample(t)
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

func businessInvalidResultExample(t *testing.T) (json.RawMessage, string) {
	t.Helper()
	data, err := os.ReadFile(testutil.MustRepoPath(t, "docs/examples/mock-workers-result-body-business-invalid.json"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := workers.ParseMockWorkersConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	body := config.MockWorkers[0].ResultBody
	var declared struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(body, &declared); err != nil {
		t.Fatal(err)
	}
	return body, declared.Output
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
