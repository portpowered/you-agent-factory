package script_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type payloadEchoCommandRunner struct{}

func (payloadEchoCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if len(req.Args) != 1 {
		return platformprocess.CommandResult{}, fmt.Errorf("expected one payload argument, got %d", len(req.Args))
	}
	return platformprocess.CommandResult{Stdout: []byte(req.Args[0])}, nil
}

type stdinRouteCommandRunner struct {
	mu       sync.Mutex
	requests []platformprocess.CommandRequest
}

func (r *stdinRouteCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.requests = append(r.requests, req)
	r.mu.Unlock()
	if len(req.Args) != 1 || req.Args[0] != "--payload-stdin" {
		return platformprocess.CommandResult{}, fmt.Errorf("payload must travel only on stdin")
	}
	var value map[string]any
	label := "supervision"
	if json.Unmarshal(req.Stdin, &value) == nil {
		if mission, exists := value["mission"]; exists {
			text, ok := mission.(string)
			if !ok {
				return platformprocess.CommandResult{ExitCode: 2, Stderr: []byte("thoughts.mission must be a string")}, nil
			}
			if strings.TrimSpace(text) != "" {
				label = "mission"
			}
		}
	}
	return platformprocess.CommandResult{Stdout: []byte(label)}, nil
}

type validationPayloadCommandRunner struct{}

func (validationPayloadCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if len(req.Args) != 2 || req.Args[0] != "validation-witness" || req.Args[1] != "--payload-stdin" {
		return platformprocess.CommandResult{}, fmt.Errorf("validation name/reader arguments=%q", req.Args)
	}
	return platformprocess.CommandResult{Stdout: req.Stdin}, nil
}

type scriptPayloadCase struct {
	name, payload, state, stdin, workName string
	args                                  []string
	runner                                platformprocess.CommandRunner
	routing                               bool
}

// The existing shared spine owns process startup and cleanup. Every payload
// case owns its Factory directory, explicit Session and controlled command edge.
func newScriptPayloadScenarios(t *testing.T) []scriptSharedScenario {
	t.Helper()
	cases := []scriptPayloadCase{
		{name: "SubmittedJSONPayload", payload: `{"title":"thoughts","mission":"measure café safely"}`, state: "complete", args: []string{`{{ (index .Inputs 0).Payload }}`}, runner: payloadEchoCommandRunner{}},
		{name: "ProjectCycleShortText", payload: `"continue-cycle"`, state: "complete", args: []string{`{{ (index .Inputs 0).Payload }}`}, runner: payloadEchoCommandRunner{}},
		{name: "ValidationNameAndMissionJSON", payload: `{"role":"retrospective","project":"payload","mission":"measure café","budget":{"paid":"0"}}`, state: "complete", workName: "validation-witness", args: []string{`{{ (index .Inputs 0).Name }}`, "--payload-stdin"}, stdin: `{{ (index .Inputs 0).Payload }}`, runner: validationPayloadCommandRunner{}},
	}
	for _, tc := range []struct{ name, payload, state, stdin string }{
		{"Mission", `{"title":"thought","mission":"measure café"}`, "mission-ready", ""},
		{"Ordinary", `{"title":"thought"}`, "supervising", ""},
		{"Blank", `{"mission":"  "}`, "supervising", ""},
		{"InvalidMission", `{"mission":42}`, "reporting-failed", ""},
		{"TemplateError", `{"mission":"must not dispatch"}`, "reporting-failed", "{{"},
	} {
		stdin := tc.stdin
		if stdin == "" {
			stdin = `{{ (index .Inputs 0).Payload }}`
		}
		cases = append(cases, scriptPayloadCase{name: tc.name, payload: tc.payload, state: tc.state, stdin: stdin, workName: "thought-witness", args: []string{"--payload-stdin"}, runner: &stdinRouteCommandRunner{}, routing: true})
	}
	scenarios := make([]scriptSharedScenario, 0, len(cases))
	for _, tc := range cases {
		dir, workType, name := writeScriptPayloadFactory(t, tc)
		scenarios = append(scenarios, scriptSharedScenario{
			name: tc.name, factoryDir: dir, runner: newScriptSharedCommandRunner(tc.runner),
			run: func(t *testing.T, fixture *scriptSharedSpineFixture) {
				runScriptPayloadScenario(t, fixture, tc, dir, workType, name)
			},
		})
	}

	return scenarios
}

func assertPayloadWorkText(t *testing.T, listed factoryapi.ListWorkResponse, workType, state, want string) {
	t.Helper()
	for _, item := range listed.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != state {
			continue
		}
		if item.Content == nil || len(*item.Content) == 0 {
			t.Fatalf("%s:%s Work has no content", workType, state)
		}
		part, err := (*item.Content)[0].AsWorkTextContentPart()
		if err != nil {
			t.Fatal(err)
		}
		if part.Text != want {
			t.Fatalf("Work text=%q, want %q", part.Text, want)
		}
		return
	}
	t.Fatalf("no Work found in %s:%s", workType, state)
}

func writeScriptPayloadFactory(t *testing.T, tc scriptPayloadCase) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	workType := "task"
	workstation := map[string]any{"name": "run-script", "worker": "runner", "inputs": []any{map[string]any{"workType": workType, "state": "init"}}, "outputs": []any{map[string]any{"workType": workType, "state": "complete"}}, "onFailure": []any{map[string]any{"workType": workType, "state": "failed"}}, "definition": map[string]any{"type": "SCRIPT_RUN", "worker": "runner", "body": "Run the script."}}
	states := []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "complete", "type": "TERMINAL"}, map[string]any{"name": "failed", "type": "FAILED"}}
	if tc.routing {
		workType = "thoughts"
		states = []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "mission-ready", "type": "TERMINAL"}, map[string]any{"name": "supervising", "type": "TERMINAL"}, map[string]any{"name": "reporting-failed", "type": "FAILED"}}
		workstation = map[string]any{"name": "route", "type": "CLASSIFIER_WORKSTATION", "worker": "runner", "inputs": []any{map[string]any{"workType": workType, "state": "init"}}, "classificationRoutes": []any{map[string]any{"label": "mission", "outputs": []any{map[string]any{"workType": workType, "state": "mission-ready"}}}, map[string]any{"label": "supervision", "outputs": []any{map[string]any{"workType": workType, "state": "supervising"}}}}, "onFailure": []any{map[string]any{"workType": workType, "state": "reporting-failed"}}, "workPropagation": map[string]any{"mode": "PRESERVE_INPUT"}}
	}
	worker := map[string]any{"name": "runner", "type": "SCRIPT_WORKER", "command": "python", "args": tc.args}
	if tc.stdin != "" {
		worker["stdin"] = tc.stdin
	}
	config := map[string]any{"name": "script-payload", "workTypes": []any{map[string]any{"name": workType, "states": states}}, "workers": []any{worker}, "workstations": []any{workstation}}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	name := tc.workName
	if name == "" {
		name = "payload-witness"
	}
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{Name: name, WorkTypeID: workType, Payload: []byte(tc.payload)})
	return dir, workType, name
}

func runScriptPayloadScenario(t *testing.T, fixture *scriptSharedSpineFixture, tc scriptPayloadCase, dir, workType, name string) {
	t.Helper()

	sessionID := fixture.openSession(t, dir)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, scriptSharedSpineTimeout)
	listed := listScriptSessionWork(t, fixture.baseURL, sessionID)
	if got := support.CountWorkAtCustomerState(listed, workType+":"+tc.state); got != 1 {
		t.Fatalf("target state count=%d, want 1; Works=%#v", got, listed)
	}
	if !tc.routing {
		var text string
		if json.Unmarshal([]byte(tc.payload), &text) != nil {
			text = tc.payload
		}
		assertPayloadWorkText(t, listed, workType, tc.state, text)
		return
	}
	var want any
	if err := json.Unmarshal([]byte(tc.payload), &want); err != nil {
		t.Fatal(err)
	}
	for _, item := range listed.Results {
		if !reflect.DeepEqual(item.Payload, want) {
			t.Fatalf("submitted payload changed: %#v, want %#v", item.Payload, want)
		}
		if item.Name != name {
			t.Fatalf("lost Work name: %#v", item)
		}
	}
	edge := tc.runner.(*stdinRouteCommandRunner)
	edge.mu.Lock()
	defer edge.mu.Unlock()
	if tc.stdin == "{{" {
		if len(edge.requests) != 0 {
			t.Fatal("invalid template launched command")
		}
	} else if len(edge.requests) != 1 || string(edge.requests[0].Stdin) != tc.payload {
		t.Fatalf("received requests=%#v, want complete submitted payload", edge.requests)
	}
	// Public event reads own the API event contract; ordinary Work journeys
	// continue to use the existing shared public invocation spine.
	requests := 0
	for _, event := range support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID) {
		if event.Type != factoryapi.FactoryEventTypeScriptRequest {
			continue
		}
		requests++
		payload, err := event.Payload.AsScriptRequestEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		if payload.StdinByteLength == nil || *payload.StdinByteLength != int64(len(tc.payload)) ||
			payload.StdinSha256 == nil || *payload.StdinSha256 != fmt.Sprintf("%x", sha256.Sum256([]byte(tc.payload))) {
			t.Fatalf("SCRIPT_REQUEST fingerprint does not match received payload: %#v", payload)
		}
	}
	wantRequests := 1
	if tc.stdin == "{{" {
		wantRequests = 0
	}
	if requests != wantRequests {
		t.Fatalf("SCRIPT_REQUEST count = %d, want %d", requests, wantRequests)
	}
}
