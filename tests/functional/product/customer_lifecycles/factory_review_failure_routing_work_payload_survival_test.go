package customer_lifecycles_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// structuredPayload is a JSON-object payload shaped like an operator batch
// item: it is not plain text, which is the shape that used to be dropped.
func structuredPayload(marker string) map[string]any {
	return map[string]any{
		"contract": marker + "-contract-sentinel",
		"scope":    []any{"alpha", "beta"},
		"nested":   map[string]any{"limit": 7},
	}
}

type promptCapture struct {
	mu      sync.Mutex
	prompts []string
	changed chan struct{}
}

func (capture *promptCapture) respond(_ context.Context, request platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
	capture.mu.Lock()
	capture.prompts = append(capture.prompts, providerCommandPrompt(request))
	close(capture.changed)
	capture.changed = make(chan struct{})
	capture.mu.Unlock()
	return reviewFailureAccepted("captured"), nil
}

func (capture *promptCapture) awaitContaining(t *testing.T, sentinel string) string {
	t.Helper()
	deadline := time.NewTimer(reviewFailureEventTimeout)
	defer deadline.Stop()
	for {
		capture.mu.Lock()
		for _, prompt := range capture.prompts {
			if strings.Contains(prompt, sentinel) {
				capture.mu.Unlock()
				return prompt
			}
		}
		changed := capture.changed
		capture.mu.Unlock()
		select {
		case <-changed:
		case <-deadline.C:
			capture.mu.Lock()
			prompts := append([]string(nil), capture.prompts...)
			capture.mu.Unlock()
			t.Fatalf("no worker prompt contained %q; prompts = %q", sentinel, prompts)
			return ""
		}
	}
}

func openPayloadScenario(t *testing.T) (*reviewFailureScenario, *promptCapture) {
	t.Helper()
	capture := &promptCapture{changed: make(chan struct{})}
	return openReviewFailureScenario(t, reviewFailureRouteConfig{provider: capture.respond}), capture
}

func workByID(t *testing.T, works []factoryapi.Work, id string) factoryapi.Work {
	t.Helper()
	for _, item := range works {
		if item.WorkId != nil && *item.WorkId == id {
			return item
		}
	}
	t.Fatalf("Work %q not found in %d listed works", id, len(works))
	return factoryapi.Work{}
}

func assertPayloadHasSentinel(t *testing.T, label string, payload any, sentinel string) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil || !strings.Contains(string(encoded), sentinel) {
		t.Fatalf("%s payload = %s (err %v), want it to contain %q", label, encoded, err, sentinel)
	}
}

func testFactoryreviewfailureroutingWorkPayload_ReachesWorkerPromptForNormallyAdmittedIdea(t *testing.T) {
	t.Parallel()
	scenario, capture := openPayloadScenario(t)
	sentinel := scenario.marker + "-contract-sentinel"
	scenario.submit(t, scenario.marker+"-req", reviewFailureSeed{
		Name: scenario.marker + "-idea", WorkID: scenario.marker + "-idea", WorkType: "idea",
		TraceID: scenario.marker + "-trace", PayloadValue: structuredPayload(scenario.marker),
	})
	capture.awaitContaining(t, sentinel)
}

func testFactoryreviewfailureroutingWorkPayload_ReachesWorkerPromptForIdeaAdmittedWithExplicitState(t *testing.T) {
	t.Parallel()
	scenario, capture := openPayloadScenario(t)
	sentinel := scenario.marker + "-contract-sentinel"
	scenario.submit(t, scenario.marker+"-req", reviewFailureSeed{
		Name: scenario.marker + "-idea", WorkID: scenario.marker + "-idea", WorkType: "idea", State: "init",
		TraceID: scenario.marker + "-trace", PayloadValue: structuredPayload(scenario.marker),
	})
	capture.awaitContaining(t, sentinel)
}

func testFactoryreviewfailureroutingWorkPayload_ReachesWorkerPromptForIdeaReleasedByDependsOnGate(t *testing.T) {
	t.Parallel()
	scenario, capture := openPayloadScenario(t)
	sentinel := scenario.marker + "-contract-sentinel"
	gateName := scenario.marker + "-gate"
	ideaName := scenario.marker + "-idea"
	scenario.submit(t, scenario.marker+"-req", reviewFailureSeed{
		Name: ideaName, WorkID: ideaName, WorkType: "idea",
		TraceID: scenario.marker + "-trace", PayloadValue: structuredPayload(scenario.marker),
		DependsOn: gateName, DependsOnState: "complete",
	}, reviewFailureSeed{
		Name: gateName, WorkID: gateName, WorkType: "idea", State: "complete",
		TraceID: scenario.marker + "-trace", Payload: "gate",
	})
	capture.awaitContaining(t, sentinel)
}

func testFactoryreviewfailureroutingWorkPayload_IsReturnedByWorkRead(t *testing.T) {
	t.Parallel()
	scenario, _ := openPayloadScenario(t)
	sentinel := scenario.marker + "-contract-sentinel"
	id := scenario.marker + "-idea"
	scenario.submit(t, scenario.marker+"-req", reviewFailureSeed{
		Name: id, WorkID: id, WorkType: "idea", State: "to-complete",
		TraceID: scenario.marker + "-trace", PayloadValue: structuredPayload(scenario.marker),
	})

	read := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(scenario.fixture.baseURL, scenario.sessionID, "/work/"+id))
	assertPayloadHasSentinel(t, "GET /work/{id}", read.Payload, sentinel)
	listed := workByID(t, scenario.listWorks(t), id)
	assertPayloadHasSentinel(t, "GET /work list item", listed.Payload, sentinel)
}

func testFactoryreviewfailureroutingWorkPayload_IsRecordedInWorkRequestEvent(t *testing.T) {
	t.Parallel()
	scenario, _ := openPayloadScenario(t)
	sentinel := scenario.marker + "-contract-sentinel"
	id := scenario.marker + "-idea"
	scenario.submit(t, scenario.marker+"-req", reviewFailureSeed{
		Name: id, WorkID: id, WorkType: "idea", State: "to-complete",
		TraceID: scenario.marker + "-trace", PayloadValue: structuredPayload(scenario.marker),
	})

	found := false
	for _, event := range support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID) {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil || payload.Works == nil {
			continue
		}
		for _, item := range *payload.Works {
			if item.WorkId != nil && *item.WorkId == id {
				found = true
				assertPayloadHasSentinel(t, "WORK_REQUEST event work", item.Payload, sentinel)
			}
		}
	}
	if !found {
		t.Fatalf("no WORK_REQUEST event recorded Work %q", id)
	}
}
