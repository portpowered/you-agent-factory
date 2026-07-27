package script_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type templateCaptureCommandRunner struct {
	mu      sync.Mutex
	request platformprocess.CommandRequest
}

func (r *templateCaptureCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.request = req
	r.mu.Unlock()

	return platformprocess.CommandResult{Stdout: []byte(strings.Join(req.Args, "\n"))}, nil
}

func (r *templateCaptureCommandRunner) LastRequest() platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.request
}

func updateScriptFixtureFactory(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	mutate(cfg)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func writeFixtureFile(t *testing.T, dir string, pathParts []string, content string) {
	t.Helper()

	path := filepath.Join(append([]string{dir}, pathParts...)...)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeScriptWorkerArgs(t *testing.T, dir string, args []string) {
	t.Helper()

	lines := []string{"---", "type: SCRIPT_WORKER", "command: echo", "args:"}
	for _, arg := range args {
		lines = append(lines, "  - "+quoteYAMLString(arg))
	}
	lines = append(lines, "---", "Execute the script.")
	writeFixtureFile(t, dir, []string{"workers", "script-worker", "AGENTS.md"}, strings.Join(lines, "\n")+"\n")
}

func writeNamedWorkstationPromptTemplate(t *testing.T, dir, workstationName, templateBody string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	content := "---\ntype: MODEL_WORKSTATION\n---\n" + templateBody + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func writeNamedWorkerAgents(t *testing.T, dir, workerName, content string) {
	t.Helper()
	support.WriteAgentConfig(t, dir, workerName, content)
}

func configureResourceGatedTemplateWorkstation(t *testing.T, dir string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["resources"] = []any{
			map[string]any{"name": "aaa-slot", "capacity": 1},
			map[string]any{"name": "zzz-slot", "capacity": 1},
		}

		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstation["resources"] = []any{
			map[string]any{"name": "aaa-slot", "capacity": 1},
			map[string]any{"name": "zzz-slot", "capacity": 1},
		}
	})
}

func configureTwoInputResourceGatedTemplateWorkstation(t *testing.T, dir, workstationName, workerName string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["workTypes"] = []any{
			map[string]any{
				"name": "zeta-resource",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
			map[string]any{
				"name": "alpha-resource",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
		}
		cfg["resources"] = []any{
			map[string]any{"name": "repo-slot", "capacity": 1},
			map[string]any{"name": "gpu-slot", "capacity": 1},
		}
		cfg["workers"] = []any{map[string]any{"name": workerName}}
		cfg["workstations"] = []any{map[string]any{
			"name":   workstationName,
			"worker": workerName,
			"inputs": []any{
				map[string]any{"workType": "zeta-resource", "state": "init"},
				map[string]any{"workType": "alpha-resource", "state": "init"},
			},
			"outputs": []any{
				map[string]any{"workType": "zeta-resource", "state": "done"},
				map[string]any{"workType": "alpha-resource", "state": "done"},
			},
			"onFailure": []map[string]any{{"workType": "zeta-resource", "state": "failed"}},
			"resources": []any{
				map[string]any{"name": "repo-slot", "capacity": 1},
				map[string]any{"name": "gpu-slot", "capacity": 1},
			},
		}}
	})
}

func writeTwoInputResourceSeeds(t *testing.T, dir string) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "zeta-input-name",
		WorkID:     "zeta-work",
		WorkTypeID: "zeta-resource",
		TraceID:    "trace-two-input-resources",
		Payload:    []byte("zeta-payload"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "alpha-input-name",
		WorkID:     "alpha-work",
		WorkTypeID: "alpha-resource",
		TraceID:    "trace-two-input-resources",
		Payload:    []byte("alpha-payload"),
	})
}

func twoInputTemplateArgs() []string {
	return []string{
		`first_name={{ (index .Inputs 0).Name }}`,
		`first_payload={{ (index .Inputs 0).Payload }}`,
		`second_name={{ (index .Inputs 1).Name }}`,
		`second_payload={{ (index .Inputs 1).Payload }}`,
		`inputs={{ len .Inputs }}`,
	}
}

func quoteYAMLString(value string) string {
	return strconv.Quote(value)
}

func assertCommandArgs(t *testing.T, req platformprocess.CommandRequest, want []string) {
	t.Helper()

	if !reflect.DeepEqual(req.Args, want) {
		t.Fatalf("command args = %#v, want %#v", req.Args, want)
	}
}

func assertProviderArgsPrompt(t *testing.T, req platformprocess.CommandRequest, want string) {
	t.Helper()

	if len(req.Args) == 0 {
		t.Fatal("provider args were empty")
	}
	if got := req.Args[len(req.Args)-1]; got != want {
		t.Fatalf("provider prompt arg = %q, want %q", got, want)
	}
}

func assertProviderStdin(t *testing.T, req platformprocess.CommandRequest, want string) {
	t.Helper()

	if got := string(req.Stdin); got != want {
		t.Fatalf("provider stdin = %q, want %q", got, want)
	}
}

func assertSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func assertProviderWorkCompleted(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	assertSessionPlaces(t, listed, map[string]int{
		"task:complete": 1, "task:init": 0, "task:failed": 0,
	})
}

func assertDispatchOutput(t *testing.T, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output == nil || *payload.Output != want {
			t.Fatalf("dispatch output = %#v, want %q", payload.Output, want)
		}
		return
	}
	t.Fatalf("Factory Event history has no dispatch response: %#v", events)
}

func assertDispatchErrorContains(t *testing.T, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, payload := range dispatchResponses(t, events) {
		if payload.Error != nil && strings.Contains(*payload.Error, want) {
			return
		}
	}
	t.Fatalf("dispatch responses do not contain error %q", want)
}

func hasFactoryEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func factoryEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func dispatchResponses(t *testing.T, events []factoryapi.FactoryEvent) []factoryapi.DispatchResponseEventPayload {
	t.Helper()
	var responses []factoryapi.DispatchResponseEventPayload
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		responses = append(responses, payload)
	}
	return responses
}
