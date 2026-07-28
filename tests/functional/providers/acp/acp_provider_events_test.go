package acp_test

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestACPUpdatesPublishExistingFactorySessionResponseEventsInOrder(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP response events"}`))
	writeACPWorker(t, dir, "cursor-acp")
	t.Setenv(acpHelperEnvironment, "1")

	var starts atomic.Int32
	_, listed, factoryEvents, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{
			PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
			ProvidersExecutableLocator:    availableExecutableLocator{},
		},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1", got)
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP process starts = %d, want 1", starts.Load())
	}
	assertACPProviderSession(t, factoryEvents)
	assertResponseEventsStayOutOfFactoryReplay(t, factoryEvents)
	assertACPResponseEventSequence(t, responseEvents)
}

func TestACPFailureRetainsPartialSnapshotAndTerminalErrorEvent(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP partial failure"}`))
	writeACPWorker(t, dir, "cursor-acp")
	t.Setenv(acpHelperEnvironment, "fail")

	var starts atomic.Int32
	_, listed, _, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1", got)
	}
	var partial, terminalError bool
	for _, event := range responseEvents {
		if event.Provenance.Provider != "cursor-acp" {
			continue
		}
		switch {
		case event.Kind == "MESSAGE" && event.Phase == "COMPLETED":
			payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
			if err != nil {
				t.Fatalf("decode partial message: %v", err)
			}
			partial = payload.Partial != nil && *payload.Partial
		case event.Kind == "ERROR" && event.Phase == "FAILED":
			payload, err := event.Payload.AsFactoryResponseEventErrorPayload()
			if err != nil {
				t.Fatalf("decode ACP error: %v", err)
			}
			terminalError = payload.Code == "ACP_PROMPT_FAILED" && strings.Contains(payload.Message, "functional ACP prompt failure")
		}
	}
	if !partial || !terminalError {
		t.Fatalf("partial snapshot=%v terminal error=%v; events=%#v", partial, terminalError, responseEvents)
	}
}

func TestACPAuthenticationRequiredMapsToCanonicalWorkerFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP auth"}`))
	writeACPWorker(t, dir, "cursor-acp")
	t.Setenv(acpHelperEnvironment, "auth")

	var starts atomic.Int32
	_, listed, factoryEvents := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1", got)
	}
	for _, event := range factoryEvents {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.FailureDetail != nil && payload.FailureDetail.Reason == factoryapi.WorkFailureTypeAuthFailure && strings.Contains(payload.FailureDetail.Message, "Agent login") {
			return
		}
	}
	t.Fatalf("Factory events omitted actionable ACP authentication failure: %#v", factoryEvents)
}

func TestACPModelIsAppliedOnlyThroughAdvertisedSessionConfig(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP model config"}`))
	writeACPWorker(t, dir, "cursor-acp")
	t.Setenv(acpHelperEnvironment, "model")

	var starts atomic.Int32
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1", got)
	}
}

func TestACPReceivesCanonicalWorkResourceAsSDKResourceLink(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	writeACPWorker(t, dir, "cursor-acp")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name: "resource input", WorkTypeID: "task",
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeImage, URL: "https://example.test/fixture.png", Label: "fixture", ContentType: "image/png",
		}},
	})
	t.Setenv(acpHelperEnvironment, "resource")

	var starts atomic.Int32
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1", got)
	}
}

func assertResponseEventsStayOutOfFactoryReplay(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	responseKinds := map[string]bool{"MESSAGE": true, "REASONING": true, "TOOL": true, "FILE_CHANGE": true, "PLAN": true, "USAGE": true, "ERROR": true}
	for _, event := range events {
		if responseKinds[string(event.Type)] {
			t.Fatalf("response event kind %q leaked into canonical Factory replay", event.Type)
		}
	}
}

func assertACPResponseEventSequence(t *testing.T, events []factoryapi.FactoryResponseEvent) {
	t.Helper()
	want := []string{"MESSAGE", "REASONING", "TOOL", "PLAN", "USAGE", "MESSAGE", "TOOL", "FILE_CHANGE", "MESSAGE"}
	got := make([]string, 0, len(events))
	var previous int64
	for _, event := range events {
		if event.Provenance.Provider != "cursor-acp" {
			continue
		}
		if event.Sequence <= previous {
			t.Fatalf("response event sequence = %d after %d", event.Sequence, previous)
		}
		previous = event.Sequence
		got = append(got, string(event.Kind))
		if event.ProviderSessionRef == nil || *event.ProviderSessionRef != "acp-session-functional-1" {
			t.Fatalf("response event ProviderSessionRef = %#v", event.ProviderSessionRef)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("ACP response event kinds = %v, want %v (all events: %#v)", got, want, events)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("ACP response event kinds = %v, want %v", got, want)
		}
	}
	final := events[len(events)-1]
	if final.Kind != "MESSAGE" || final.Phase != "COMPLETED" || final.Provenance.Representation != "SNAPSHOT" {
		t.Fatalf("terminal ACP response event = %#v, want completed MESSAGE snapshot", final)
	}
}
