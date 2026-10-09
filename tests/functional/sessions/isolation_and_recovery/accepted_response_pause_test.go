package isolation_and_recovery_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The CLI host uses root.BuildProcess + Process.Execute. Controls, Work and
// events are API-owned contracts. The provider command edge holds the original
// attempt until pause is acknowledged; there is no timing-dependent pause race.
func TestAcceptedResponseSurvivesPause(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{})
	runner := &pausedResponseRunner{runner: support.NewGatedSuccessCommandRunner("COMPLETE", gate), arrived: make(chan struct{}, 8)}
	dir := support.ScaffoldFactory(t, acceptedResponsePauseFactory())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
	for _, name := range []string{"process", "finish"} {
		support.WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })
	baseURL := server.URL()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	workName, siblingName := "original", "sibling"
	work := support.SubmitDefaultSessionWork(t, baseURL, factoryapi.SubmitWorkRequest{Name: &workName, WorkTypeName: "task", Payload: map[string]string{"title": "finish while paused"}})
	select {
	case <-runner.arrived:
	case <-ctx.Done():
		t.Fatalf("original provider execution did not start: Work=%#v events=%#v", support.ListDefaultSessionWork(t, baseURL), support.GetFactoryEventsAt(t, baseURL))
	}
	pause := postSessionsLifecycleControl(t, baseURL, factorysessions.DefaultSessionID, factoryapi.FactorySessionLifecycleControlKindPause)
	if pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause = %#v", pause)
	}
	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(baseURL))
	// A submission accepted after pause must remain buffered alongside the
	// downstream Work made eligible by the original result.
	sibling := support.SubmitDefaultSessionWork(t, baseURL, factoryapi.SubmitWorkRequest{Name: &siblingName, WorkTypeName: "task", Payload: map[string]string{"title": "wait for resume"}})
	close(gate)
	waitAcceptedPauseDispatchResponse(t, ctx, stream, *work.WorkId)
	listed := support.ListDefaultSessionWork(t, baseURL)
	assertAcceptedPauseOutput(t, listed, *work.WorkId)
	if dispatches := support.ObserveDispatchEvents(t, support.GetFactoryEventsAt(t, baseURL)); len(dispatches) != 1 {
		t.Fatalf("dispatches before resume = %d, want 1", len(dispatches))
	}
	if !support.HasWorkAtCustomerState(listed, *work.WorkId, support.WorkCustomerLocation("task", "processed")) {
		t.Fatalf("original Work before resume = %#v, want task:processed", listed.Results)
	}
	if support.HasWorkAtCustomerState(listed, *sibling.WorkId, support.WorkCustomerLocation("task", "processed")) {
		t.Fatal("paused sibling was dispatched")
	}
	session := support.GetDefaultSession(t, baseURL)
	if session.Runtime.LifecycleControlStatus == nil || *session.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("lifecycle after completion = %#v, want PAUSED", session.Runtime)
	}
	if calls := runner.calls.Load(); calls != 1 {
		t.Fatalf("provider calls before resume = %d, want 1", calls)
	}
	resume := postSessionsLifecycleControl(t, baseURL, factorysessions.DefaultSessionID, factoryapi.FactorySessionLifecycleControlKindResume)
	if resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume = %#v", resume)
	}
	support.WaitForRuntimeIdle(t, baseURL, time.Minute)
	listed = support.ListDefaultSessionWork(t, baseURL)
	for _, id := range []string{*work.WorkId, *sibling.WorkId} {
		if !support.HasWorkAtCustomerState(listed, id, support.WorkCustomerLocation("task", "complete")) {
			t.Fatalf("Work %q after resume = %#v, want task:complete", id, listed.Results)
		}
	}
	if calls := runner.calls.Load(); calls != 4 {
		t.Fatalf("provider calls after resume = %d, want two per Work", calls)
	}
}

func waitAcceptedPauseDispatchResponse(t *testing.T, ctx context.Context, stream *support.FactoryEventStream, workID string) {
	t.Helper()
	for {
		event := stream.NextEventContext(ctx)
		if string(event.Type) != "DISPATCH_RESPONSE" || event.Context.WorkIds == nil {
			continue
		}
		for _, id := range *event.Context.WorkIds {
			if id == workID {
				return
			}
		}
	}
}

type pausedResponseRunner struct {
	runner  platformprocess.CommandRunner
	arrived chan struct{}
	calls   atomic.Int32
}

func (runner *pausedResponseRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	select {
	case runner.arrived <- struct{}{}:
	default:
	}
	return runner.runner.Run(ctx, request)
}

func acceptedResponsePauseFactory() map[string]any {
	config := processExecuteRuntimeOpeningFactoryConfig()
	config["name"] = "accepted-response-pause"
	types := config["workTypes"].([]map[string]any)
	types[0]["states"] = append(types[0]["states"].([]map[string]string), map[string]string{"name": "processed", "type": "PROCESSING"})
	stations := config["workstations"].([]map[string]any)
	stations[0]["outputs"] = []map[string]string{{"workType": "task", "state": "processed"}}
	config["workstations"] = append(stations, map[string]any{
		"name": "finish", "worker": "worker-a",
		"inputs":    []map[string]string{{"workType": "task", "state": "processed"}},
		"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
		"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
	})
	return config
}

func assertAcceptedPauseOutput(t *testing.T, listed factoryapi.ListWorkResponse, workID string) {
	t.Helper()
	for _, item := range listed.Results {
		if item.WorkId == nil || *item.WorkId != workID || item.Content == nil {
			continue
		}
		for _, part := range *item.Content {
			text, err := part.AsWorkTextContentPart()
			if err == nil && text.Text == "COMPLETE" {
				return
			}
		}
	}
	t.Fatal("full accepted output missing from public Work before resume")
}
