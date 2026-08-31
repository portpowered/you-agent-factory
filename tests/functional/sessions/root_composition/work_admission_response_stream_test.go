package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const workAdmissionResponseStreamPrimaryResult = "primary result COMPLETE"

// TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle
// proves work-admission and response-stream activate through public Sessions HTTP
// surfaces after runtime lifecycle on the shared root-built process. The event
// and response subscriptions are established before Work admission so the
// terminal observation cannot depend on a sampled status window.
func TestSessionsWorkAdmissionAndResponseStreamActivateThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	fixture := rootCompositionSharedProcess(t)

	dir := support.ScaffoldFactory(t, sessionsWorkAdmissionResponseStreamFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	fixture.registerCommandRunners(
		t,
		dir,
		support.NewStaticSuccessCommandRunner(workAdmissionResponseStreamPrimaryResult),
		nil,
	)

	workAdmissionBefore := fixture.effects.workRequest.Load()
	responseStreamBefore := fixture.effects.responseStreamCount()
	sessionID := fixture.openSession(t, dir)
	baseURL := fixture.baseURL
	eventStream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(baseURL, sessionID))
	responseStream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(baseURL, sessionID))
	t.Cleanup(func() {
		eventStream.Close()
		responseStream.Close()
	})
	workName := "fun-sessions-work-admission"
	submitted := support.SubmitSessionWorkAt(t, baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         &workName,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "fun-sessions work admission"},
	})
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submitted Work response = %#v, want canonical work identity", submitted)
	}
	waitForRootCompositionTerminalDispatch(t, eventStream, workID)

	invocation := postSessionsInvocation(
		t,
		baseURL,
		sessionID,
		sessionsTextInvocationRequest(t, "prove work-admission and response-stream activation"),
	)
	assertSessionsInvocationPrimaryResultText(t, invocation, workAdmissionResponseStreamPrimaryResult)
	frame := responseStream.NextFrame(rootCompositionSharedShutdownTimeout)
	if frame.Event.Kind == "" {
		t.Fatalf("response stream frame = %#v, want an observable response event after invocation", frame)
	}

	if got := fixture.effects.workRequest.Load() - workAdmissionBefore; got <= 0 {
		t.Fatalf("work-admission effect calls after public session operations = %d, want > 0 via edges", got)
	}
	if got := fixture.effects.responseStreamCount() - responseStreamBefore; got <= 0 {
		t.Fatalf("response-stream effect calls after public session operations = %d, want > 0 via edges", got)
	}
}

func sessionsWorkAdmissionResponseStreamFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
			"handlingBehavior": []string{"DEFAULT"},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func waitForRootCompositionTerminalDispatch(
	t *testing.T,
	stream *support.FactoryEventStream,
	workID string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), rootCompositionSharedShutdownTimeout)
	defer cancel()
	for {
		event := stream.NextEventContext(ctx)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse || event.Context.WorkIds == nil {
			continue
		}
		for _, candidate := range *event.Context.WorkIds {
			if candidate == workID {
				return
			}
		}
	}
}

func sessionsTextInvocationRequest(t *testing.T, text string) factoryapi.InvocationRequest {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build invocation text content: %v", err)
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	return factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	}
}

func postSessionsInvocation(
	t *testing.T,
	baseURL string,
	sessionID string,
	request factoryapi.InvocationRequest,
) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	endpoint := baseURL + "/factory-sessions/" + sessionID + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, payload)
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func assertSessionsInvocationPrimaryResultText(
	t *testing.T,
	response factoryapi.InvocationResponse,
	wantText string,
) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	if part.Text != wantText {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, wantText)
	}
}
