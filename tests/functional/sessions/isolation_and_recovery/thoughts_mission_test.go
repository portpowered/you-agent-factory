package isolation_and_recovery_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type restoredMissionRunner struct {
	mu    sync.Mutex
	stdin [][]byte
}

func (r *restoredMissionRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stdin = append(r.stdin, append([]byte(nil), request.Stdin...))
	var payload struct{ Mission string }
	label := "supervision"
	if json.Unmarshal(request.Stdin, &payload) == nil && payload.Mission != "" {
		label = "mission"
	}
	return platformprocess.CommandResult{Stdout: []byte(label)}, nil
}

func thoughtsRestartConfig() map[string]any {
	return map[string]any{
		"name": "restored-thoughts",
		"workTypes": []any{
			map[string]any{"name": "idea", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "complete", "type": "TERMINAL"}}},
			map[string]any{"name": "thoughts", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "mission-ready", "type": "TERMINAL"}, map[string]any{"name": "supervising", "type": "TERMINAL"}, map[string]any{"name": "reporting-failed", "type": "FAILED"}}},
		},
		"workers":      []any{map[string]any{"name": "router", "type": "SCRIPT_WORKER", "command": "python", "args": []string{"--payload-stdin"}, "stdin": `{{ (index .Inputs 0).Payload }}`}},
		"workstations": []any{map[string]any{"name": "route", "type": "CLASSIFIER_WORKSTATION", "worker": "router", "inputs": []any{map[string]any{"workType": "thoughts", "state": "init"}}, "classificationRoutes": []any{map[string]any{"label": "mission", "outputs": []any{map[string]any{"workType": "thoughts", "state": "mission-ready"}}}, map[string]any{"label": "supervision", "outputs": []any{map[string]any{"workType": "thoughts", "state": "supervising"}}}}, "onFailure": []any{map[string]any{"workType": "thoughts", "state": "reporting-failed"}}, "workPropagation": map[string]any{"mode": "PRESERVE_INPUT"}}},
	}
}

// A durable customer's two host generations are ordered. Both generations use
// the same root-built process and real recording storage, with a controlled
// script edge. Distinct explicit sessions prove reconstruction, not a reopen
// of an already-live runtime. Public controls release the restored prerequisite.
func TestThoughtsMissionPayloadSurvivesRestart(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, thoughtsRestartConfig())
	project, home := t.TempDir(), t.TempDir()
	source, successor := filepath.Join(project, "source.json"), filepath.Join(project, "successor.json")
	runner := &restoredMissionRunner{}
	process := support.BuildProcess(t, serviceedges.Edges{
		ScriptCommandRunner: runner,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return ctx.Value(restartProbeServerKey{}).(*support.ProcessAPIServer).Start(ctx, request)
		},
	})
	sessionIDs := []string{uuid.NewString(), uuid.NewString()}
	var before factoryapi.Work
	const payload = `{"mission":"restore café 😀 without substituting the idea","title":"own mission"}`
	for generation, sessionID := range sessionIDs {
		api := support.NewProcessAPIServer()
		args := []string{"you", "run", "--dir", dir, "--session", sessionID, "--continuously", "--with-server", "--server", "http://127.0.0.1:1"}
		if generation == 0 {
			args = append(args, "--record", source)
		} else {
			args = append(args, "--resume", source, "--record", successor)
		}
		inputs := support.FakeInputs(context.WithValue(t.Context(), restartProbeServerKey{}, api), args)
		inputs.Input.WorkingDirectory = project
		inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
		command := startRestartProbeCommand(t, process, inputs.Input)
		baseURL := restartProbeReadyURL(t, api, command)
		if generation == 0 {
			submitThoughtsRestartBatch(t, process, inputs.Input.Env, project, baseURL, sessionID, `{"requestId":"idea-batch","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"held-idea","name":"idea","workTypeName":"idea","payload":{"title":"not the mission"}}]}`)
			submitThoughtsRestartBatch(t, process, inputs.Input.Env, project, baseURL, sessionID, `{"requestId":"thoughts-batch","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"restored-thought","name":"thought","workTypeName":"thoughts","tags":{"project":"worker-session-visibility"},"payload":`+payload+`}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"thought","targetWorkId":"held-idea","requiredState":"complete"}]}`)
		}
		item := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(baseURL, sessionID, "/work/restored-thought"))
		assertRestoredThoughtIdentity(t, item, payload, "init")
		assertNoThoughtsRestartDispatch(t, baseURL, sessionID)
		if generation == 0 {
			before = item
			restartProbeShutdown(t, baseURL, command)
			continue
		}
		if !reflect.DeepEqual(before.Payload, item.Payload) || !reflect.DeepEqual(before.Tags, item.Tags) || !reflect.DeepEqual(before.Relations, item.Relations) {
			t.Fatal("restoration changed payload, tags or target")
		}
		restartProbePost(t, support.SessionWorkURL(baseURL, sessionID, "/work/held-idea/move"), []byte(`{"stateName":"complete"}`))
		support.WaitForSessionTerminalStatus(t, baseURL, sessionID, 15*time.Second)
		item = support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(baseURL, sessionID, "/work/restored-thought"))
		runner.mu.Lock()
		stdin := append([][]byte(nil), runner.stdin...)
		runner.mu.Unlock()
		if len(stdin) != 1 || string(stdin[0]) != payload {
			t.Fatalf("restored script stdin=%q", stdin)
		}
		assertRestoredThoughtIdentity(t, item, payload, "mission-ready")
		assertRestoredThoughtFingerprint(t, support.GetFactoryEventsForSessionAt(t, baseURL, sessionID), payload)
		restartProbeShutdown(t, baseURL, command)
	}
	// A third read-only host resumes the completed recording. Public reads
	// establish persistence/replay of both optional fields, not just live emission.
	api := support.NewProcessAPIServer()
	replayID := uuid.NewString()
	inputs := support.FakeInputs(context.WithValue(t.Context(), restartProbeServerKey{}, api), []string{"you", "run", "--dir", dir, "--session", replayID, "--continuously", "--with-server", "--server", "http://127.0.0.1:1", "--resume", successor, "--record", filepath.Join(project, "replay.json")})
	inputs.Input.WorkingDirectory = project
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	command := startRestartProbeCommand(t, process, inputs.Input)
	baseURL := restartProbeReadyURL(t, api, command)
	assertRestoredThoughtFingerprint(t, support.GetFactoryEventsForSessionAt(t, baseURL, replayID), payload)
	restartProbeShutdown(t, baseURL, command)
}

func submitThoughtsRestartBatch(t *testing.T, process support.Process, env []string, project, baseURL, sessionID, batch string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", baseURL, "--session", sessionID, "--json", "submit", "batch", batch})
	inputs.Input.Env, inputs.Input.WorkingDirectory = env, project
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("submit: %v; %s %s", err, inputs.Stdout(), inputs.Stderr())
	}
}

func assertRestoredThoughtIdentity(t *testing.T, item factoryapi.Work, payload, state string) {
	t.Helper()
	var expected any
	if err := json.Unmarshal([]byte(payload), &expected); err != nil {
		t.Fatal(err)
	}
	if item.State == nil || item.State.Name != state || support.StringPointerValue(item.WorkId) != "restored-thought" || item.Name != "thought" || !reflect.DeepEqual(item.Payload, expected) {
		t.Fatalf("restored Work=%#v, want own mission in %s", item, state)
	}
	if item.Tags == nil || (*item.Tags)["project"] != "worker-session-visibility" || item.Relations == nil || len(*item.Relations) != 1 {
		t.Fatalf("restored tags/relations=%#v", item)
	}
	relation := (*item.Relations)[0]
	if relation.Type != factoryapi.RelationTypeDependsOn || support.StringPointerValue(relation.TargetWorkId) != "held-idea" {
		t.Fatalf("restored relation=%#v", relation)
	}
}

func assertNoThoughtsRestartDispatch(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, sessionID) {
		if event.Type == factoryapi.FactoryEventTypeDispatchRequest || event.Type == factoryapi.FactoryEventTypeScriptRequest {
			t.Fatalf("blocked thoughts dispatched: %#v", event)
		}
	}
}

func assertRestoredThoughtFingerprint(t *testing.T, events []factoryapi.FactoryEvent, payload string) {
	t.Helper()
	count := 0
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeScriptRequest {
			continue
		}
		count++
		request, err := event.Payload.AsScriptRequestEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		if request.StdinByteLength == nil || *request.StdinByteLength != int64(len(payload)) || request.StdinSha256 == nil || *request.StdinSha256 != fmt.Sprintf("%x", sha256.Sum256([]byte(payload))) {
			t.Fatalf("public replay fingerprint=%#v", request)
		}
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, exists := fields["payload"].(map[string]any)["stdin"]; exists {
			t.Fatal("raw stdin leaked into public event")
		}
	}
	if count != 1 {
		t.Fatalf("public script request count=%d, want one", count)
	}
	t.Logf("public persisted/replayed stdin bytes=%d sha256=%x", len(payload), sha256.Sum256([]byte(payload)))
}
