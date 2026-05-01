package functional_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

const cleanupSmokeProject = "acme-inventory"

var rootFactoryPathPattern = regexp.MustCompile(`factory/[A-Za-z0-9._/-]+`)

func assertSmokeTokenInPlace(t *testing.T, h *testutil.ServiceTestHarness, placeID string) {
	t.Helper()

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	for _, token := range snapshot.Marking.Tokens {
		if token != nil && token.PlaceID == placeID {
			return
		}
	}
	places := make([]string, 0, len(snapshot.Marking.Tokens))
	for _, token := range snapshot.Marking.Tokens {
		if token == nil {
			continue
		}
		places = append(places, fmt.Sprintf("%s:%s:%s", token.Color.WorkID, token.PlaceID, token.History.LastError))
	}
	t.Fatalf("expected token in place %q; observed places: %s", placeID, strings.Join(places, ", "))
}

func assertCleanupSmokeRuntimeContext(t *testing.T, dir string, call interfaces.ProviderInferenceRequest) {
	t.Helper()

	if call.WorkingDirectory != resolvedRuntimePath(dir, "/workspaces/acme-inventory/feature/acme-cleanup") {
		t.Fatalf("working directory = %q, want acme project path", call.WorkingDirectory)
	}
	if call.EnvVars["PROJECT"] != cleanupSmokeProject {
		t.Fatalf("PROJECT env = %q, want %s", call.EnvVars["PROJECT"], cleanupSmokeProject)
	}
	if call.EnvVars["CONTEXT_PROJECT"] != cleanupSmokeProject {
		t.Fatalf("CONTEXT_PROJECT env = %q, want %s", call.EnvVars["CONTEXT_PROJECT"], cleanupSmokeProject)
	}
	if call.EnvVars["BRANCH"] != "feature/acme-cleanup" {
		t.Fatalf("BRANCH env = %q, want feature/acme-cleanup", call.EnvVars["BRANCH"])
	}
	if len(call.InputTokens) != 1 {
		t.Fatalf("provider input tokens = %d, want 1", len(call.InputTokens))
	}
	assertCleanupSmokeTags(t, "provider input token", firstInputToken(call.InputTokens).Color.Tags)
}

func assertCleanupSmokeEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	var sawRequest bool
	var sawWorkInput bool
	var sawDispatch bool
	var sawTerminalOutput bool
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil || stringPointerValue(event.Context.RequestId) != "request-project-cleanup-smoke" || payload.Works == nil {
				continue
			}
			sawRequest = true
			if len(*payload.Works) != 1 {
				t.Fatalf("cleanup smoke request works = %d, want 1", len(*payload.Works))
			}
			sawWorkInput = true
			assertCleanupSmokeTags(t, "WORK_REQUEST item", generatedTags((*payload.Works)[0].Tags))
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil || payload.TransitionId != "process" {
				continue
			}
			for _, input := range dispatchInputWorksFromHistory(t, events, event, payload) {
				if stringPointerValue(input.WorkId) != "work-project-cleanup-smoke" {
					continue
				}
				sawDispatch = true
				assertCleanupSmokeTags(t, "DISPATCH_CREATED input", generatedTags(input.Tags))
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil || payload.OutputWork == nil {
				continue
			}
			for _, output := range *payload.OutputWork {
				if stringPointerValue(output.WorkId) != "work-project-cleanup-smoke" {
					continue
				}
				sawTerminalOutput = true
				assertCleanupSmokeTags(t, "DISPATCH_COMPLETED output work", generatedTags(output.Tags))
			}
		}
	}
	if !sawRequest || !sawWorkInput || !sawDispatch || !sawTerminalOutput {
		t.Fatalf(
			"cleanup smoke missing event boundary: request=%v input=%v dispatch=%v terminal=%v",
			sawRequest,
			sawWorkInput,
			sawDispatch,
			sawTerminalOutput,
		)
	}
}

func generatedTags(tags *factoryapi.StringMap) map[string]string {
	if tags == nil {
		return nil
	}
	return map[string]string(*tags)
}

func assertCleanupSmokeTags(t *testing.T, label string, tags map[string]string) {
	t.Helper()

	if tags["project"] != cleanupSmokeProject {
		t.Fatalf("%s project tag = %q, want %s", label, tags["project"], cleanupSmokeProject)
	}
	if tags["branch"] != "feature/acme-cleanup" {
		t.Fatalf("%s branch tag = %q, want feature/acme-cleanup", label, tags["branch"])
	}
	assertMapDoesNotContainPortOS(t, label, tags)
}

func assertInferenceRequestsDoNotContainPortOS(t *testing.T, calls []interfaces.ProviderInferenceRequest) {
	t.Helper()

	if len(calls) == 0 {
		t.Fatal("expected at least one provider request")
	}
	for i, call := range calls {
		data, err := json.Marshal(call)
		if err != nil {
			t.Fatalf("marshal provider request %d: %v", i, err)
		}
		assertValueDoesNotContainPortOS(t, fmt.Sprintf("provider request %d", i), string(data))
	}
}

func assertFactoryEventsDoNotContainPortOS(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	if len(events) == 0 {
		t.Fatal("expected at least one factory event")
	}
	for i, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal factory event %d: %v", i, err)
		}
		assertValueDoesNotContainPortOS(t, fmt.Sprintf("factory event %d (%s)", i, event.Type), string(data))
	}
}

func assertMapDoesNotContainPortOS(t *testing.T, label string, values map[string]string) {
	t.Helper()

	for key, value := range values {
		assertValueDoesNotContainPortOS(t, label+" key", key)
		assertValueDoesNotContainPortOS(t, label+" value", value)
	}
}

func assertValueDoesNotContainPortOS(t *testing.T, label string, value string) {
	t.Helper()

	normalized := strings.ToLower(value)
	if strings.Contains(normalized, "portos") ||
		strings.Contains(normalized, "port os") ||
		strings.Contains(normalized, "port_os") {
		t.Fatalf("%s contains Port OS coupling: %q", label, value)
	}
}

func assertTextOmitsRetiredEventNames(t *testing.T, label string, value string) {
	t.Helper()

	for _, retired := range []string{
		"RUN_STARTED",
		"INITIAL_STRUCTURE",
		"RELATIONSHIP_CHANGE",
		"DISPATCH_CREATED",
		"DISPATCH_COMPLETED",
		"FACTORY_STATE_CHANGE",
		"RUN_FINISHED",
	} {
		if strings.Contains(value, `"`+retired+`"`) {
			t.Fatalf("%s contains retired public event name %q", label, retired)
		}
	}
}
