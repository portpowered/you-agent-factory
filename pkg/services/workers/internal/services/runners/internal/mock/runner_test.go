package mock

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

func TestMockRunnerDispatchDecisions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []workers.MockWorkerConfig
		policy  workers.MockWorkerUnmatchedDispatchPolicy
		content string
		fails   bool
	}{
		{name: "unmatched accepts", content: "mock worker accepted"},
		{name: "unmatched passthrough", policy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough, content: "mock unmatched passthrough"},
		{name: "reject", entries: []workers.MockWorkerConfig{{RunType: workers.MockWorkerRunTypeReject}}, fails: true},
		{name: "missing script", entries: []workers.MockWorkerConfig{{RunType: workers.MockWorkerRunTypeScript}}, fails: true},
		{name: "missing command effect", entries: []workers.MockWorkerConfig{{RunType: workers.MockWorkerRunTypeScript, ScriptConfig: &workers.MockWorkerScriptConfig{Command: "fixture"}}}, fails: true},
		{name: "selection skips other worker", entries: []workers.MockWorkerConfig{{WorkerName: "other", RunType: workers.MockWorkerRunTypeReject}}, content: "mock worker accepted"},
		{name: "selection skips other workstation", entries: []workers.MockWorkerConfig{{WorkstationName: "other", RunType: workers.MockWorkerRunTypeReject}}, content: "mock worker accepted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner{config: &workers.MockWorkersConfig{MockWorkers: tc.entries, UnmatchedDispatchPolicy: tc.policy}}
			result, err := r.Execute(t.Context(), workers.RunnerExecutionRequest{RunnerID: Identity, WorkerType: "worker", WorkstationType: "station"})
			if (err != nil) != tc.fails || result.Content != tc.content {
				t.Fatalf("Execute() = %#v, %v", result, err)
			}
		})
	}
}

func TestMockRunnerScriptUsesInjectedEffect(t *testing.T) {
	for _, tc := range []struct {
		name string
		exit int
		err  error
	}{
		{name: "success"}, {name: "exit failure", exit: 2}, {name: "effect failure", err: errors.New("effect failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			r := &runner{config: &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{RunType: workers.MockWorkerRunTypeScript, ScriptConfig: &workers.MockWorkerScriptConfig{Command: "fixture", Args: []string{"input"}, WorkingDirectory: "workspace"}}}}, next: mockCommandEffect(func(_ context.Context, request workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
				calls++
				if request.Command != "fixture" || len(request.Args) != 1 || request.Args[0] != "input" || request.WorkDir != "workspace" {
					t.Fatalf("command request = %#v", request)
				}
				return workerprocess.CommandResult{Stdout: []byte(" result \n"), ExitCode: tc.exit}, tc.err
			})}
			result, err := r.Execute(t.Context(), workers.RunnerExecutionRequest{RunnerID: Identity})
			if calls != 1 || (err != nil) != (tc.exit != 0 || tc.err != nil) {
				t.Fatalf("calls=%d result=%#v error=%v", calls, result, err)
			}
			if err == nil && (result.Content != "result" || result.Diagnostics.Command.Stdout != " result \n") {
				t.Fatalf("script result = %#v", result)
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("effect error = %v", err)
			}
		})
	}
}

func TestMockRunnerRejectsCanceledAndInvalidRequests(t *testing.T) {
	r := &runner{config: &workers.MockWorkersConfig{}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.Execute(ctx, workers.RunnerExecutionRequest{RunnerID: Identity}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
	if _, err := r.Execute(t.Context(), workers.RunnerExecutionRequest{}); err == nil {
		t.Fatal("empty runner identity accepted")
	}
}

func TestMockInputSelectorsMatchIdentityAndRejectMissingInputs(t *testing.T) {
	selectors := []workers.MockWorkInputSelector{{WorkID: "work", WorkType: "task", State: "complete", TraceID: "trace", Channel: "output"}}
	token := map[string]any{"workId": "work", "workTypeId": "task", "state": "complete", "traceId": "trace", "tags": map[string]string{"channel": "output"}}
	if !mockWorkInputSelectorsMatch(selectors, []any{token}) {
		t.Fatal("matching token rejected")
	}
	if mockWorkInputSelectorsMatch(selectors, []any{func() {}, map[string]any{"workId": "other"}}) {
		t.Fatal("unmatched or unencodable input accepted")
	}
	selectors[0].InputName = "named"
	if mockWorkInputSelectorsMatch(selectors, []any{token}) {
		t.Fatal("unnamed token matched named input")
	}
	input := workers.WorkInput{WorkID: "work", WorkTypeID: "task", State: "complete", InputNames: []string{"named"}, Tags: map[string]string{"channel": "output"}}
	input.Lineage.TraceID = "trace"
	if !mockWorkInputSelectorsMatch(selectors, []any{input}) {
		t.Fatal("matching named Work input rejected")
	}
}

type mockCommandEffect func(context.Context, workerprocess.CommandRequest) (workerprocess.CommandResult, error)

func (effect mockCommandEffect) Run(ctx context.Context, request workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	return effect(ctx, request)
}

func TestMockRunnerDeclaredBodyIsDetachedAndUnchanged(t *testing.T) {
	t.Parallel()
	body := json.RawMessage(`{ "decision":"ACCEPTED", "output":{"invalid":"business"} }`)
	cfg := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{RunType: workers.MockWorkerRunTypeAccept, ResultBody: body}}}
	r, err := New(Config{WorkersConfig: cfg}, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	want := string(body)
	body[1] = 'x'
	result, err := r.Execute(t.Context(), workers.RunnerExecutionRequest{RunnerID: Identity})
	if err != nil || result.Content != want {
		t.Fatalf("Execute = %#v, %v", result, err)
	}
	if result.Diagnostics.Metadata[workers.ProviderResponseMetadataCompletionEvidence] != "provider_response" {
		t.Fatal("completion evidence missing")
	}
}
