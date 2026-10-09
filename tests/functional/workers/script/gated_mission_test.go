package script_test

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

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each directory has an immutable runner and its own prerequisite release.
// Completing the worker (rather than manually moving Work) exercises the
// scheduler's completion/release/dispatch path without an intervening restart.
type gatedMissionRunner struct {
	arrived chan struct{}
	release chan struct{}
	once    sync.Once
	router  stdinRouteCommandRunner
}

func (r *gatedMissionRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if len(req.Args) == 1 && req.Args[0] == "hold-prerequisite" {
		r.once.Do(func() { close(r.arrived) })
		select {
		case <-r.release:
			return platformprocess.CommandResult{Stdout: []byte("prerequisite completed")}, nil
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	return r.router.Run(ctx, req)
}

func newGatedMissionScenarios(t *testing.T) []scriptSharedScenario {
	t.Helper()
	var scenarios []scriptSharedScenario
	for _, crossBatch := range []bool{true, false} {
		name := "UntaggedSameBatchMission"
		if crossBatch {
			name = "TaggedCrossBatchMission"
		}
		dir := writeGatedMissionFactory(t)
		runner := &gatedMissionRunner{arrived: make(chan struct{}), release: make(chan struct{})}
		scenarios = append(scenarios, scriptSharedScenario{
			name: name, factoryDir: dir, runner: newScriptSharedCommandRunner(runner),
			run: func(t *testing.T, fixture *scriptSharedSpineFixture) {
				runGatedMissionScenario(t, fixture, dir, name, crossBatch, runner)
			},
		})
	}
	return scenarios
}

func writeGatedMissionFactory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	config := map[string]any{
		"name": "gated-thoughts",
		"workTypes": []any{
			map[string]any{"name": "idea", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "complete", "type": "TERMINAL"}, map[string]any{"name": "failed", "type": "FAILED"}}},
			map[string]any{"name": "thoughts", "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "mission-ready", "type": "TERMINAL"}, map[string]any{"name": "supervising", "type": "TERMINAL"}, map[string]any{"name": "reporting-failed", "type": "FAILED"}}},
		},
		"workers": []any{
			map[string]any{"name": "hold", "type": "SCRIPT_WORKER", "command": "controlled", "args": []string{"hold-prerequisite"}},
			map[string]any{"name": "router", "type": "SCRIPT_WORKER", "command": "python", "args": []string{"--payload-stdin"}, "stdin": `{{ (index .Inputs 0).Payload }}`},
		},
		"workstations": []any{
			map[string]any{"name": "finish-idea", "worker": "hold", "inputs": []any{map[string]any{"workType": "idea", "state": "init"}}, "outputs": []any{map[string]any{"workType": "idea", "state": "complete"}}, "onFailure": []any{map[string]any{"workType": "idea", "state": "failed"}}, "definition": map[string]any{"type": "SCRIPT_RUN", "worker": "hold", "body": "Finish the prerequisite."}},
			map[string]any{"name": "route", "type": "CLASSIFIER_WORKSTATION", "worker": "router", "inputs": []any{map[string]any{"workType": "thoughts", "state": "init"}}, "classificationRoutes": []any{map[string]any{"label": "mission", "outputs": []any{map[string]any{"workType": "thoughts", "state": "mission-ready"}}}, map[string]any{"label": "supervision", "outputs": []any{map[string]any{"workType": "thoughts", "state": "supervising"}}}}, "onFailure": []any{map[string]any{"workType": "thoughts", "state": "reporting-failed"}}, "workPropagation": map[string]any{"mode": "PRESERVE_INPUT"}},
		},
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func submitGatedMissionBatch(t *testing.T, fixture *scriptSharedSpineFixture, sessionID, requestID string, works []any, relations []any) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"requestId": requestID, "type": "FACTORY_REQUEST_BATCH", "works": works, "relations": relations})
	if err != nil {
		t.Fatal(err)
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", fixture.baseURL, "--session", sessionID, "--json", "submit", "batch", string(raw)})
	inputs.Input.Env = sharedScriptProcessEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostDir
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("submit batch: %v; stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
}

func runGatedMissionScenario(t *testing.T, fixture *scriptSharedSpineFixture, dir, name string, crossBatch bool, runner *gatedMissionRunner) {
	t.Helper()
	sessionID := fixture.openSession(t, dir)
	var release sync.Once
	unblock := func() { release.Do(func() { close(runner.release) }) }
	t.Cleanup(unblock)
	missionID, prerequisiteID := name+"-mission", name+"-idea"
	payload := `{"mission":"` + name + ` café 😀","title":"own submitted mission"}`
	prerequisite := map[string]any{"workId": prerequisiteID, "name": "idea", "workTypeName": "idea", "payload": map[string]string{"title": "dissimilar prerequisite without mission"}}
	mission := map[string]any{"workId": missionID, "name": "thought", "workTypeName": "thoughts", "payload": json.RawMessage(payload)}
	relation := map[string]any{"type": "DEPENDS_ON", "sourceWorkName": "thought", "requiredState": "complete"}
	if crossBatch {
		mission["tags"] = map[string]string{"project": "worker-session-visibility"}
		relation["targetWorkId"] = prerequisiteID
		submitGatedMissionBatch(t, fixture, sessionID, name+"-batch-a", []any{prerequisite}, nil)
		submitGatedMissionBatch(t, fixture, sessionID, name+"-batch-b", []any{mission}, []any{relation})
	} else {
		relation["targetWorkName"] = "idea"
		submitGatedMissionBatch(t, fixture, sessionID, name+"-batch", []any{prerequisite, mission}, []any{relation})
	}
	select {
	case <-runner.arrived:
	case <-time.After(scriptSharedSpineTimeout):
		t.Fatal("prerequisite command did not start")
	}
	before := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(fixture.baseURL, sessionID, "/work/"+missionID))
	if before.State == nil || before.State.Name != "init" {
		t.Fatalf("blocked Work=%#v", before)
	}
	assertGatedMissionIdentity(t, before, missionID, prerequisiteID, payload, crossBatch)
	for _, event := range support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID) {
		if event.Type == factoryapi.FactoryEventTypeDispatchRequest {
			request, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatal(err)
			}
			if request.TransitionId == "route" {
				t.Fatal("thoughts dispatched before gate release")
			}
		}
	}
	unblock()
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, scriptSharedSpineTimeout)
	after := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(fixture.baseURL, sessionID, "/work/"+missionID))
	if after.State == nil || after.State.Name != "mission-ready" {
		t.Fatalf("released Work=%#v", after)
	}
	assertGatedMissionIdentity(t, after, missionID, prerequisiteID, payload, crossBatch)
	runner.router.mu.Lock()
	requests := append([]platformprocess.CommandRequest(nil), runner.router.requests...)
	runner.router.mu.Unlock()
	if len(requests) != 1 || string(requests[0].Stdin) != payload {
		t.Fatalf("router requests=%#v, want own payload exactly once", requests)
	}
	assertGatedMissionEvents(t, support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID), payload)
	fixture.closeSession(t, sessionID)
}

func assertGatedMissionIdentity(t *testing.T, item factoryapi.Work, missionID, prerequisiteID, payload string, tagged bool) {
	t.Helper()
	var want any
	if err := json.Unmarshal([]byte(payload), &want); err != nil {
		t.Fatal(err)
	}
	if support.StringPointerValue(item.WorkId) != missionID || item.Name != "thought" || !reflect.DeepEqual(item.Payload, want) {
		t.Fatalf("Work identity/payload changed: %#v", item)
	}
	if tagged && (item.Tags == nil || (*item.Tags)["project"] != "worker-session-visibility") {
		t.Fatalf("Work project tag lost: %#v", item.Tags)
	}
	if item.Relations == nil || len(*item.Relations) != 1 {
		t.Fatalf("Work relation lost: %#v", item.Relations)
	}
	relation := (*item.Relations)[0]
	if relation.Type != factoryapi.RelationTypeDependsOn || support.StringPointerValue(relation.TargetWorkId) != prerequisiteID {
		t.Fatalf("Work target changed: %#v", relation)
	}
}

func assertGatedMissionEvents(t *testing.T, events []factoryapi.FactoryEvent, payload string) {
	t.Helper()
	requests, responses := 0, 0
	completionSequence, dispatchSequence := -1, -1
	completionTick, dispatchTick := -1, -1
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeScriptRequest:
			request, err := event.Payload.AsScriptRequestEventPayload()
			if err != nil {
				t.Fatal(err)
			}
			if request.TransitionId != "route" {
				continue
			}
			requests++
			assertGatedMissionFingerprint(t, request, payload)
		case factoryapi.FactoryEventTypeDispatchResponse:
			response, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatal(err)
			}
			if response.TransitionId == "finish-idea" {
				completionSequence, completionTick = event.Context.Sequence, event.Context.Tick
			}
			if response.TransitionId == "route" {
				responses++
				if support.StringPointerValue(response.SelectedClassificationLabel) != "mission" {
					t.Fatalf("router label=%#v", response.SelectedClassificationLabel)
				}
			}
		case factoryapi.FactoryEventTypeDispatchRequest:
			request, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatal(err)
			}
			if request.TransitionId == "route" {
				dispatchSequence, dispatchTick = event.Context.Sequence, event.Context.Tick
			}
		}
	}
	assertGatedMissionOrdering(t, requests, responses, completionSequence, dispatchSequence, completionTick, dispatchTick)
	t.Logf("gate completion/route sequence=%d/%d tick=%d; stdin bytes=%d sha256=%x", completionSequence, dispatchSequence, dispatchTick, len(payload), sha256.Sum256([]byte(payload)))
}

func assertGatedMissionFingerprint(t *testing.T, request factoryapi.ScriptRequestEventPayload, payload string) {
	t.Helper()
	if request.StdinByteLength == nil || *request.StdinByteLength != int64(len(payload)) || request.StdinSha256 == nil || *request.StdinSha256 != fmt.Sprintf("%x", sha256.Sum256([]byte(payload))) {
		t.Fatalf("fingerprint differs from exact ingress: %#v", request)
	}
}

func assertGatedMissionOrdering(t *testing.T, requests, responses, completionSequence, dispatchSequence, completionTick, dispatchTick int) {
	t.Helper()
	if requests != 1 || responses != 1 || completionSequence < 0 || dispatchSequence <= completionSequence {
		t.Fatalf("route counts=%d/%d, completion/dispatch sequence=%d/%d", requests, responses, completionSequence, dispatchSequence)
	}
	// Async DISPATCH_RESPONSE records the observed completion tick. beginTick
	// increments the clock before draining that result; this is the first
	// scheduler pass that can release and dispatch its dependent. No idle tick,
	// operator move, or host restart intervenes in this gate-release reaction.
	if dispatchTick != completionTick+1 {
		t.Fatalf("completion/dispatch ticks=%d/%d, want immediate next scheduler pass", completionTick, dispatchTick)
	}
}
