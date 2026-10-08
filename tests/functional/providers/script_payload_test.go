package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type payloadEchoCommandRunner struct{}

func (payloadEchoCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if len(req.Args) != 1 {
		return platformprocess.CommandResult{}, fmt.Errorf("expected one payload argument, got %d", len(req.Args))
	}
	return platformprocess.CommandResult{Stdout: []byte(req.Args[0])}, nil
}

func configurePayloadArgument(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "factory.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	worker := config["workers"].([]any)[0].(map[string]any)
	worker["args"] = []string{`{{ (index .Inputs 0).Payload }}`}
	raw, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestScriptWorkerReceivesSubmittedJSONPayload(t *testing.T) {
	t.Parallel()
	dir := installPackagedScriptRuntimeFixture(t, "submitted-script-payload", "fixture")
	// Seed a customer-authored batch, rather than passing an internal token.
	payload := `{"title":"thoughts","mission":"measure café safely"}`
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{Name: "payload-witness", WorkTypeID: "task", Payload: []byte(payload)})
	configurePayloadArgument(t, dir)
	_, listed := RunFactory(t, dir, dir, payloadEchoCommandRunner{}, 10*time.Second)
	assertListedWorkText(t, listed, "task", "complete", payload)
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
func TestScriptPayloadRoutesAndFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, payload, state, stdin string }{
		{name: "mission", payload: `{"title":"thought","mission":"measure café"}`, state: "mission-ready"},
		{name: "ordinary", payload: `{"title":"thought"}`, state: "supervising"},
		{name: "blank", payload: `{"mission":"  "}`, state: "supervising"},
		{name: "invalid mission", payload: `{"mission":42}`, state: "reporting-failed"},
		{name: "template error", payload: `{"mission":"must not dispatch"}`, state: "reporting-failed", stdin: "{{"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			stdin := tc.stdin
			if stdin == "" {
				stdin = `{{ (index .Inputs 0).Payload }}`
			}
			config := map[string]any{
				"name":         "thoughts-routing",
				"workTypes":    []any{map[string]any{"name": "thoughts", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "mission-ready", "type": "TERMINAL"}, map[string]any{"name": "supervising", "type": "TERMINAL"}, map[string]any{"name": "reporting-failed", "type": "FAILED"}}}},
				"workers":      []any{map[string]any{"name": "router", "type": "SCRIPT_WORKER", "command": "python", "args": []string{"--payload-stdin"}, "stdin": stdin}},
				"workstations": []any{map[string]any{"name": "route", "type": "CLASSIFIER_WORKSTATION", "worker": "router", "inputs": []any{map[string]any{"workType": "thoughts", "state": "init"}}, "classificationRoutes": []any{map[string]any{"label": "mission", "outputs": []any{map[string]any{"workType": "thoughts", "state": "mission-ready"}}}, map[string]any{"label": "supervision", "outputs": []any{map[string]any{"workType": "thoughts", "state": "supervising"}}}}, "onFailure": []any{map[string]any{"workType": "thoughts", "state": "reporting-failed"}}, "workPropagation": map[string]any{"mode": "PRESERVE_INPUT"}}},
			}
			raw, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "factory.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			testutil.WriteSeedRequest(t, dir, work.SubmitRequest{Name: "thought-witness", WorkTypeID: "thoughts", Payload: []byte(tc.payload)})
			edge := &stdinRouteCommandRunner{}
			_, listed := RunFactory(t, dir, dir, edge, 10*time.Second)
			if got := support.CountWorkAtCustomerState(listed, "thoughts:"+tc.state); got != 1 {
				t.Fatalf("target state count=%d, want 1; Works=%#v", got, listed)
			}
			var want any
			if err := json.Unmarshal([]byte(tc.payload), &want); err != nil {
				t.Fatal(err)
			}
			for _, item := range listed.Results {
				if !reflect.DeepEqual(item.Payload, want) {
					t.Fatalf("submitted payload changed: %#v, want %#v", item.Payload, want)
				}
			}
			edge.mu.Lock()
			defer edge.mu.Unlock()
			if tc.stdin != "" {
				if len(edge.requests) != 0 {
					t.Fatal("invalid template launched command")
				}
				return
			}
			if len(edge.requests) != 1 || string(edge.requests[0].Stdin) != tc.payload {
				t.Fatalf("received requests=%#v, want complete submitted payload", edge.requests)
			}
			for _, item := range listed.Results {
				if item.Name != "thought-witness" {
					t.Fatalf("lost Work name: %#v", item)
				}
			}
		})
	}
}

type validationPayloadCommandRunner struct{}

func (validationPayloadCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if len(req.Args) != 2 || req.Args[0] != "validation-witness" || req.Args[1] != "--payload-stdin" {
		return platformprocess.CommandResult{}, fmt.Errorf("validation name/reader arguments=%q", req.Args)
	}
	return platformprocess.CommandResult{Stdout: req.Stdin}, nil
}
func TestValidationSetupReceivesNameAndMissionJSON(t *testing.T) {
	t.Parallel()
	dir := installPackagedScriptRuntimeFixture(t, "validation-payload", "fixture")
	path := filepath.Join(dir, "factory.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	worker := config["workers"].([]any)[0].(map[string]any)
	worker["args"] = []string{`{{ (index .Inputs 0).Name }}`, "--payload-stdin"}
	worker["stdin"] = `{{ (index .Inputs 0).Payload }}`
	raw, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	payload := `{"role":"retrospective","project":"payload","mission":"measure café","budget":{"paid":"0"}}`
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{Name: "validation-witness", WorkTypeID: "task", Payload: []byte(payload)})
	_, listed := RunFactory(t, dir, dir, validationPayloadCommandRunner{}, 10*time.Second)
	assertListedWorkText(t, listed, "task", "complete", payload)
}
func TestProjectCycleDeciderReceivesShortText(t *testing.T) {
	t.Parallel()
	dir := installPackagedScriptRuntimeFixture(t, "cycle-payload", "fixture")
	configurePayloadArgument(t, dir)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{Name: "cycle-witness", WorkTypeID: "task", Payload: []byte(`"continue-cycle"`)})
	_, listed := RunFactory(t, dir, dir, payloadEchoCommandRunner{}, 10*time.Second)
	assertListedWorkText(t, listed, "task", "complete", "continue-cycle")
}
