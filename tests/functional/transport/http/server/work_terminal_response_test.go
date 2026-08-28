package server_test

import (
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	workTerminalResponseText      = "ordered terminal text part"
	workTerminalResponseJSONKey   = "verdict"
	workTerminalResponseJSONWant  = "ordered-json-part"
	workTerminalResponseAccepted  = "preserved ordered content COMPLETE"
	workTerminalResponseWaitLimit = 30 * time.Second
)

// TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary
// submits Work whose terminal result carries two differently typed ordered
// content parts (text then JSON) and proves the public HTTP boundary keeps both
// discriminated types and payloads in the submitted order on the terminal read,
// and that a failure terminal outcome carrying the same content is never
// reported as success.
func TestWorkTerminalResponsePreservesOrderedTypedContentThroughPublicBoundary(t *testing.T) {
	t.Run("terminal success keeps ordered typed parts", func(t *testing.T) {
		dir := scaffoldC06HTTPFactory(t, workTerminalResponseFactoryConfig())
		fixture := c06SharedHTTPServer(t)
		scenario := fixture.newScenario(t, "work-terminal-success", dir)
		scenario.registerRunner(t, []string{dir}, support.NewStaticSuccessCommandRunner(workTerminalResponseAccepted))
		sessionID := scenario.openSession(t, dir)
		workID := submitOrderedTypedWork(t, scenario.URL(), sessionID)

		status := support.WaitForSessionTerminalStatus(t, scenario.URL(), sessionID, workTerminalResponseWaitLimit)
		if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
			t.Fatalf("status categories = %+v, want one terminal and zero failed", status.Categories)
		}
		assertTerminalWorkFactoryEvent(t, scenario.URL(), sessionID, workID, factoryapi.WorkOutcomeAccepted)
		assertScenarioRunnerCalled(t, scenario)

		listed := support.GetJSON[factoryapi.ListWorkResponse](t, c06SessionWorkURL(scenario.URL(), sessionID, "/work"))
		if len(listed.Results) != 1 {
			t.Fatalf("listed work = %#v, want exactly one work item", listed.Results)
		}
		assertOrderedTypedTerminalContent(t, listed.Results[0].Content, "GET /work content")

		terminal := support.GetJSON[factoryapi.Work](t, c06SessionWorkURL(scenario.URL(), sessionID, "/work/"+workID))
		assertOrderedTypedTerminalContent(t, terminal.Content, "GET /work/{id} content")
		if terminal.State == nil || terminal.State.Type != factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("terminal work state = %#v, want a TERMINAL state type", terminal.State)
		}
	})

	t.Run("terminal failure is not reported as success", func(t *testing.T) {
		dir := scaffoldC06HTTPFactory(t, workTerminalResponseFactoryConfig())
		fixture := c06SharedHTTPServer(t)
		scenario := fixture.newScenario(t, "work-terminal-failure", dir)
		scenario.registerRunner(t, []string{dir}, support.NewShapedProviderCommandRunner(
			platformprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte("provider refused the ordered content dispatch"),
			},
		))
		sessionID := scenario.openSession(t, dir)
		workID := submitOrderedTypedWork(t, scenario.URL(), sessionID)

		status := support.WaitForSessionTerminalStatus(t, scenario.URL(), sessionID, workTerminalResponseWaitLimit)
		if status.Categories.Failed != 1 || status.Categories.Terminal != 0 {
			t.Fatalf("status categories = %+v, want one failed and zero terminal", status.Categories)
		}
		assertTerminalWorkFactoryEvent(t, scenario.URL(), sessionID, workID, factoryapi.WorkOutcomeFailed)
		assertScenarioRunnerCalled(t, scenario)

		failed := support.GetJSON[factoryapi.Work](t, c06SessionWorkURL(scenario.URL(), sessionID, "/work/"+workID))
		if failed.State == nil || failed.State.Type != factoryapi.WorkStateTypeFAILED {
			t.Fatalf("work state after provider failure = %#v, want a FAILED state type", failed.State)
		}
		// The same ordered typed content is still readable on the failed Work, so
		// the failure classification is what distinguishes it from success rather
		// than content loss.
		assertOrderedTypedTerminalContent(t, failed.Content, "failed GET /work/{id} content")
	})
}

func assertScenarioRunnerCalled(t *testing.T, scenario *c06SharedHTTPScenario) {
	t.Helper()
	if scenario == nil || scenario.route == nil {
		t.Fatal("c06 scenario has no controlled provider route")
	}
	if calls := scenario.route.calls.Load(); calls == 0 {
		t.Fatal("c06 controlled provider runner received no calls")
	}
}

func assertTerminalWorkFactoryEvent(
	t *testing.T,
	baseURL, sessionID, workID string,
	targetOutcome factoryapi.WorkOutcome,
) {
	t.Helper()

	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse ||
			event.Context.SessionId == nil ||
			*event.Context.SessionId != sessionID ||
			event.Context.WorkIds == nil {
			continue
		}
		containsWork := false
		for _, eventWorkID := range *event.Context.WorkIds {
			if eventWorkID == workID {
				containsWork = true
				break
			}
		}
		if !containsWork {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode terminal DISPATCH_RESPONSE event %q: %v", event.Id, err)
		}
		if payload.Outcome == targetOutcome {
			return
		}
	}
	t.Fatalf("Factory Event stream has no %s DISPATCH_RESPONSE for Work %q in session %q", targetOutcome, workID, sessionID)
}

func assertOrderedTypedTerminalContent(t *testing.T, content *factoryapi.WorkContent, surface string) {
	t.Helper()

	if content == nil || len(*content) != 2 {
		t.Fatalf("%s = %#v, want two ordered content parts", surface, content)
	}
	textPart, err := (*content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("%s part 0 is not a text part: %v", surface, err)
	}
	if textPart.Type != factoryapi.WorkContentPartTypeText {
		t.Fatalf("%s part 0 type = %q, want %q", surface, textPart.Type, factoryapi.WorkContentPartTypeText)
	}
	if textPart.Text != workTerminalResponseText {
		t.Fatalf("%s part 0 text = %q, want %q", surface, textPart.Text, workTerminalResponseText)
	}

	jsonPart, err := (*content)[1].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("%s part 1 is not a JSON part: %v", surface, err)
	}
	if jsonPart.Type != factoryapi.WorkContentPartTypeJSON {
		t.Fatalf("%s part 1 type = %q, want %q", surface, jsonPart.Type, factoryapi.WorkContentPartTypeJSON)
	}
	payload, ok := jsonPart.Json.(map[string]any)
	if !ok {
		t.Fatalf("%s part 1 json = %#v, want a JSON object", surface, jsonPart.Json)
	}
	if got, _ := payload[workTerminalResponseJSONKey].(string); got != workTerminalResponseJSONWant {
		t.Fatalf(
			"%s part 1 json[%q] = %#v, want %q",
			surface,
			workTerminalResponseJSONKey,
			payload[workTerminalResponseJSONKey],
			workTerminalResponseJSONWant,
		)
	}
}

// submitOrderedTypedWork submits one Work through the public session-scoped
// admission route with a text content part followed by a JSON content part and
// returns the public Work identity the submission was accepted under.
func submitOrderedTypedWork(t *testing.T, baseURL, sessionID string) string {
	t.Helper()

	submitted := support.SubmitSessionWorkAt(t, baseURL, sessionID, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task",
		Content:      workTerminalOrderedTypedContent(t),
	})
	if !submitted.Accepted {
		t.Fatalf("submit work response = %#v, want an accepted submission", submitted)
	}
	if submitted.WorkId == nil || strings.TrimSpace(*submitted.WorkId) == "" {
		t.Fatalf("submit work response = %#v, want a public work identity", submitted)
	}
	return *submitted.WorkId
}

// workTerminalResponseFactoryConfig authors PRESERVE_INPUT propagation so the
// ordered typed content submitted with the Work is the content the terminal
// state carries instead of being replaced by the workstation output text.
func workTerminalResponseFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-terminal-response",
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":            "process",
			"worker":          "worker-a",
			"workPropagation": map[string]any{"mode": "PRESERVE_INPUT"},
			"inputs":          []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":         []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure":       []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func workTerminalOrderedTypedContent(t *testing.T) *factoryapi.WorkContent {
	t.Helper()

	var textPart factoryapi.WorkContentPart
	if err := textPart.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: workTerminalResponseText,
	}); err != nil {
		t.Fatalf("encode text content part: %v", err)
	}
	var jsonPart factoryapi.WorkContentPart
	if err := jsonPart.FromWorkJsonContentPart(factoryapi.WorkJsonContentPart{
		Type: factoryapi.WorkContentPartTypeJSON,
		Json: map[string]any{workTerminalResponseJSONKey: workTerminalResponseJSONWant},
	}); err != nil {
		t.Fatalf("encode JSON content part: %v", err)
	}
	content := factoryapi.WorkContent{textPart, jsonPart}
	return &content
}
