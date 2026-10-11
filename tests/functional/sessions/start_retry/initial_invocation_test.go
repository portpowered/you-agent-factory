package start_retry_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap/zaptest/observer"
)

// All four command invocations reach their selected provider before current
// selection moves. Captured streams are read only after their command joins.
func testOpeningPolicyOverlap(t *testing.T, process support.Process, sessions factorysessions.Service, scenarios []initialOpeningScenario, gate *selectedProviderGate, serverURL string, logs *observer.ObservedLogs) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	defer gate.unblock()
	inputs := make([]*support.CapturedInputs, len(scenarios))
	done := make([]chan error, len(scenarios))
	for i, scenario := range scenarios {
		request := scenario.request()
		request.RuntimeSelection.Verbose = i == 1
		startInitialOpeningSession(t, sessions, request)
		args := openingPolicyArgs(scenario, serverURL, i)
		inputs[i] = support.FakeInputs(ctx, args)
		inputs[i].Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
		inputs[i].Input.WorkingDirectory = scenario.candidateDir
		done[i] = make(chan error, 1)
		go func(i int) { done[i] <- process.Execute(inputs[i].Input); close(done[i]) }(i)
		t.Cleanup(func() {
			cancel()
			gate.unblock()
			select {
			case <-done[i]:
			case <-time.After(30 * time.Second):
				t.Error("owned policy command did not join")
			}
		})
	}
	awaitOpeningPolicyProviders(t, ctx, gate, inputs, done)
	// Opening selects this peer as current through the public boundary while
	// the explicit invocations retain their already-acquired handles.
	peerHistory := scenarios[0].startPeer(t, sessions)
	for _, scenario := range scenarios {
		initialOpeningHistory(t, sessions, scenario.candidateID)
		assertInitialOpeningDiagnostics(t, logs, scenario.candidateID, scenario.candidateDir)
	}
	gate.unblock()
	for i, scenario := range scenarios {
		select {
		case err := <-done[i]:
			if err != nil {
				t.Fatalf("policy command: %v stdout=%s stderr=%s", err, inputs[i].Stdout(), inputs[i].Stderr())
			}
		case <-ctx.Done():
			t.Fatal("policy command did not complete")
		}
		assertOpeningPolicyOutput(t, sessions, scenario, inputs[i], logs, i)
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenarios[0].peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, scenarios[0].peerID)
}

func awaitOpeningPolicyProviders(t *testing.T, ctx context.Context, gate *selectedProviderGate, inputs []*support.CapturedInputs, done []chan error) {
	t.Helper()
	seen := map[string]bool{}
	for range inputs {
		select {
		case request := <-gate.entered:
			id := gate.paths[request.WorkDir]
			prompt := string(request.Stdin) + strings.Join(request.Args, " ")
			if id == "" || seen[id] || !strings.Contains(prompt, "selected-control-"+id) || !strings.Contains(prompt, "private-policy-"+id) {
				t.Fatalf("policy provider lost selected prompt: dir=%s prompt=%q", request.WorkDir, prompt)
			}
			seen[id] = true
		case <-ctx.Done():
			for i := range inputs {
				select {
				case err := <-done[i]:
					t.Fatalf("policy %d ended before provider overlap: %v stdout=%s stderr=%s", i, err, inputs[i].Stdout(), inputs[i].Stderr())
				default:
				}
			}
			t.Fatal("quiet, normal, verbose and NDJSON providers did not overlap")
		}
	}
}

func assertOpeningPolicyOutput(t *testing.T, sessions factorysessions.Service, scenario initialOpeningScenario, inputs *support.CapturedInputs, logs *observer.ObservedLogs, mode int) {
	t.Helper()
	switch mode {
	case 0:
		if inputs.Stdout() != scenario.candidateID+" COMPLETE" || inputs.Stderr() != "" {
			t.Fatalf("quiet result framing: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
		}
	case 1:
		assertOpeningPolicySingleJSON(t, scenario.candidateID, inputs.Stdout())
	case 2:
		if !strings.HasSuffix(strings.TrimSpace(inputs.Stdout()), scenario.candidateID+" COMPLETE") || !strings.Contains(inputs.Stdout(), "Work") {
			t.Fatalf("human stream lost lifecycle or final result: %q", inputs.Stdout())
		}
		if strings.Contains(inputs.Stdout(), "\"recordType\"") || strings.Contains(inputs.Stdout()+inputs.Stderr(), "\x1b[") {
			t.Fatalf("human stream contains structured records or interactive controls: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
		}
	case 3:
		assertOpeningPolicyNDJSON(t, sessions, scenario, inputs.Stdout())
	}
	secret := openingPolicySecret(scenario)
	if strings.Contains(inputs.Stdout()+inputs.Stderr(), secret) {
		t.Fatal("terminal policy exposed declared secret")
	}
	for _, entry := range logs.All() {
		if strings.Contains(fmt.Sprint(entry.Message, entry.ContextMap()), secret) {
			t.Fatal("selected logger exposed declared secret")
		}
	}
	initialOpeningHistory(t, sessions, scenario.candidateID)
	assertOpeningPolicyResponseCorrelation(t, sessions, scenario.candidateID, logs)
}

func assertOpeningPolicySingleJSON(t *testing.T, sessionID, output string) {
	t.Helper()
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("single JSON framing: %v stdout=%q", err, output)
	}
	assertOpeningPolicyTerminal(t, sessionID, response)
}

func assertOpeningPolicyTerminal(t *testing.T, sessionID string, response factoryapi.InvocationResponse) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusCompleted || response.SessionId == nil || *response.SessionId != sessionID || response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("selected policy result = %+v", response)
	}
	content, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil || content.Text != sessionID+" COMPLETE" {
		t.Fatalf("selected policy content = %+v, %v", content, err)
	}
}

func assertOpeningPolicyNDJSON(t *testing.T, sessions factorysessions.Service, scenario initialOpeningScenario, output string) {
	t.Helper()
	sessionID := scenario.candidateID
	history := initialOpeningHistory(t, sessions, sessionID).History
	lines := strings.Split(strings.TrimSpace(output), "\n")
	events, terminal, startup := 0, 0, 0
	lastSequence := -1
	for i, line := range lines {
		var record struct {
			RecordType string                        `json:"recordType"`
			Event      factoryapi.FactoryEvent       `json:"event"`
			Response   factoryapi.InvocationResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("NDJSON line %d: %v: %q", i, err, line)
		}
		switch record.RecordType {
		case "factory_event":
			assertOpeningPolicyRecordedEvent(t, history, record.Event)
			if i < 2 {
				assertOpeningPolicyStartup(t, scenario, record.Event, i)
				startup++
			}
			events++
			if (record.Event.Context.SessionId != nil && *record.Event.Context.SessionId != sessionID) || record.Event.Context.Sequence <= lastSequence {
				t.Fatalf("NDJSON event crossed Session or order: %#v", record.Event)
			}
			lastSequence = record.Event.Context.Sequence
		case "invocation_result":
			terminal++
			if i != len(lines)-1 {
				t.Fatalf("NDJSON terminal is not the final line: %d", i)
			}
			assertOpeningPolicyTerminal(t, sessionID, record.Response)
		default:
			t.Fatalf("unexpected NDJSON record: %s", line)
		}
	}
	if events == 0 || terminal != 1 || startup != 2 {
		t.Fatalf("NDJSON events=%d terminals=%d startup=%d, want lifecycle, two startup frames and one terminal", events, terminal, startup)
	}
}

// Lifecycle frames can omit Session identity. Their selected public history,
// including the complete payload, is the attribution boundary for those frames.
func assertOpeningPolicyRecordedEvent(t *testing.T, history []factorydefinitions.FactoryEvent, event factoryapi.FactoryEvent) {
	t.Helper()
	for _, recorded := range history {
		if recorded.Context.Sequence != event.Context.Sequence {
			continue
		}
		// The public projection deliberately omits private execution facts,
		// such as the association event's model and reasoning effort.
		public, err := apisurface.FactoryEventToAPI(recorded)
		if err != nil {
			t.Fatal(err)
		}
		// Union payloads retain JSON bytes; object-key order is not framing.
		var got, want any
		for _, value := range []struct {
			event factoryapi.FactoryEvent
			into  *any
		}{{event, &got}, {public, &want}} {
			encoded, err := json.Marshal(value.event)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, value.into); err != nil {
				t.Fatal(err)
			}
		}
		// CLI presentation hides provider diagnostics and identity even when
		// the recording/API retains them. This fixture has no nested private
		// observations; the selected result and all remaining fields must match.
		if event.Type == factoryapi.FactoryEventTypeModelResponse || event.Type == factoryapi.FactoryEventTypeInferenceResponse || event.Type == factoryapi.FactoryEventTypeAgentRunResponse {
			gotPayload := got.(map[string]any)["payload"].(map[string]any)
			wantPayload := want.(map[string]any)["payload"].(map[string]any)
			for _, key := range []string{"diagnostics", "providerSession"} {
				if _, exposed := gotPayload[key]; exposed {
					t.Fatalf("NDJSON exposed private %s", key)
				}
				delete(wantPayload, key)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("NDJSON event differs from selected public history: got=%+v want=%+v", event, recorded)
		}
		return
	}
	t.Fatalf("NDJSON event is absent from selected public history: %+v", event)
}

func assertOpeningPolicyStartup(t *testing.T, scenario initialOpeningScenario, event factoryapi.FactoryEvent, index int) {
	t.Helper()
	var source factoryapi.Factory
	switch {
	case index == 0 && event.Type == factoryapi.FactoryEventTypeRunRequest:
		payload, err := event.Payload.AsRunRequestEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		source = payload.Factory
	case index == 1 && event.Type == factoryapi.FactoryEventTypeInitialStructureRequest:
		payload, err := event.Payload.AsInitialStructureRequestEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		source = payload.Factory
	default:
		t.Fatalf("unexpected unattributed NDJSON event at %d: %s", index, event.Type)
	}
	if event.Context.Sequence != index || source.FactoryDirectory == nil || filepath.Clean(*source.FactoryDirectory) != scenario.candidateDir {
		t.Fatalf("NDJSON startup lost selected source or order: context=%#v source=%#v", event.Context, source)
	}
}

func assertOpeningPolicyResponseCorrelation(t *testing.T, sessions factorysessions.Service, id string, logs *observer.ObservedLogs) {
	t.Helper()
	subscription, err := sessions.SubscribeResponses(t.Context(), factorysessions.SessionResponseSubscriptionRequest{SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Cursor.Detach()
	events, err := subscription.Cursor.Next(t.Context())
	if err != nil || len(events) == 0 {
		t.Fatalf("selected policy responses = %+v, %v", events, err)
	}
	accepted := openingPolicyDiagnosticPositions(t, id, logs)
	var last int64
	for _, event := range events {
		if event.FactorySessionID != id || event.Sequence <= last || accepted[event.EventID] != uint64(event.Sequence) || !event.RecordedAt.Equal((initialOpeningClock{}).Now()) {
			t.Fatalf("selected policy response crossed identity, logger or clock: %+v", event)
		}
		last = event.Sequence
	}
}

func openingPolicyDiagnosticPositions(t *testing.T, id string, logs *observer.ObservedLogs) map[string]uint64 {
	t.Helper()
	accepted := map[string]uint64{}
	for _, entry := range logs.FilterMessage("events append outcome").All() {
		fields := entry.ContextMap()
		if fields["topic"] != "factory-session/"+id+"/response-events" || fields["outcome"] != "accepted" {
			continue
		}
		if fields["selected_backend"] != "initial-opening" || fields["source_id"] != id {
			t.Fatalf("selected response logger lost correlation: %+v", fields)
		}
		eventID, ok := fields["source_event_id"].(string)
		position, positionOK := fields["position"].(uint64)
		if !ok || !positionOK {
			t.Fatalf("response diagnostic omitted identity: %+v", fields)
		}
		accepted[eventID] = position
	}
	return accepted
}

func testInitialOpeningChildInvocation(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario, effects *initialOpeningEffects, serverURL string) {
	t.Helper()
	peerHistory := scenario.startPeer(t, sessions)
	startInitialOpeningSession(t, sessions, scenario.request())
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--remote", "--server", serverURL, "--session", scenario.candidateID,
		"--json", "run", "--factory", filepath.Join(scenario.candidateDir, "factory.json"), "--output", "primary", "--no-record", "invoke the opened session child",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("CLI child invocation: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode invocation response: %v stdout=%s", err, inputs.Stdout())
	}
	if response.SessionId == nil || *response.SessionId == "" {
		t.Fatalf("child invocation omitted durable session identity: %#v", response)
	}
	if !strings.Contains(inputs.Stdout(), "initial opening COMPLETE") {
		t.Fatalf("child failed to return selected provider output: %s effects=%v", inputs.Stdout(), effects.forScenario(scenario))
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("child invocation status = %s, want COMPLETED", response.Status)
	}
	read, err := sessions.GetSession(t.Context(), *response.SessionId)
	if err != nil || read.Status != factorysessions.LifecycleStatusSucceeded {
		t.Fatalf("read durable child completion: %#v, %v", read, err)
	}
	assertInitialChildResponses(t, sessions, *response.SessionId)
	assertInitialOpeningProviderSelection(t, effects, scenario.candidateDir)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
}

func assertInitialChildResponses(t *testing.T, sessions factorysessions.Service, sessionID string) {
	t.Helper()
	// The legacy response subscription selects only live sessions. This public
	// cursor selects the completed durable child's retained progress instead.
	subscription, err := sessions.SubscribeResponses(t.Context(), factorysessions.SessionResponseSubscriptionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("subscribe durable child responses: %v", err)
	}
	defer subscription.Cursor.Detach()
	events, err := subscription.Cursor.Next(t.Context())
	if err != nil {
		t.Fatalf("read retained child responses: %v", err)
	}
	var messages, terminals int
	var lastSequence int64
	for _, event := range events {
		if event.FactorySessionID != sessionID || event.Sequence <= lastSequence || event.DispatchID == "" {
			t.Fatalf("child response lost identity or order: session=%s dispatch=%s sequence=%d after=%d", event.FactorySessionID, event.DispatchID, event.Sequence, lastSequence)
		}
		lastSequence = event.Sequence
		if event.Provenance.NativeEventType == "STREAM_COMPLETED" && event.Phase == factorysessions.ResponseEventPhaseCompleted {
			terminals++
		}
		if assertInitialChildNativeMessage(t, event) {
			messages++
		}
	}
	if messages != 1 || terminals != 1 {
		t.Fatalf("retained child responses: native messages=%d terminal completions=%d, want one each", messages, terminals)
	}
}

func assertInitialChildNativeMessage(t *testing.T, event factorysessions.FactoryResponseEvent) bool {
	t.Helper()
	// Compatibility deltas may repeat text from the provider's snapshot.
	// Native provenance distinguishes callback delivery from result shaping.
	if event.Kind != factorysessions.ResponseEventKindMessage || event.Phase != factorysessions.ResponseEventPhaseCompleted || event.Provenance.Delivery != factorysessions.ResponseEventDeliveryNativeStream {
		return false
	}
	var message factorysessions.ResponseEventMessage
	if err := json.Unmarshal(event.Payload, &message); err != nil {
		t.Fatalf("decode provider message: %v", err)
	}
	if event.Provenance.Provider != "codex" || len(message.ContentBlocks) != 1 || message.ContentBlocks[0].Text != "initial opening COMPLETE" {
		t.Fatalf("child provider message = %s provenance=%#v", event.Payload, event.Provenance)
	}
	return true
}

// This public journey replaces the composed engine/dispatch unit fixture.
func testInitialOpeningAttributedHistory(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario, serverURL string) {
	t.Helper()
	peerBefore := scenario.startPeer(t, sessions)
	startInitialOpeningSession(t, sessions, scenario.request())
	for _, sessionID := range []string{scenario.candidateID, scenario.peerID} {
		requestID := "request-" + sessionID
		ctx, cancel := context.WithTimeout(t.Context(), initialOpeningReadCeiling)
		stream, err := sessions.SubscribeFactoryEventsForSession(ctx, sessionID, nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		inputs := support.FakeInputs(ctx, []string{"you", "--json", "--server", serverURL, "--session", sessionID,
			"submit", "batch", fmt.Sprintf(`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"workId":%q,"name":"attributed","workTypeName":"task","payload":{"title":"independent history"}}]}`, requestID, "work-"+sessionID)})
		inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
		inputs.Input.WorkingDirectory = scenario.candidateDir
		if err := process.Execute(inputs.Input); err != nil {
			cancel()
			t.Fatalf("submit %s: %v stdout=%s stderr=%s", sessionID, err, inputs.Stdout(), inputs.Stderr())
		}
		assertInitialOpeningAttributedEvents(t, ctx, stream, sessionID, requestID)
		assertInitialOpeningWorkProjection(t, process, scenario, serverURL, sessionID, requestID)
		cancel()
		if sessionID == scenario.candidateID {
			assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerBefore)
		}
	}
}

func assertInitialOpeningAttributedEvents(t *testing.T, ctx context.Context, stream *factorydefinitions.FactoryEventStream, sessionID, requestID string) {
	t.Helper()
	counts := map[factorydefinitions.FactoryEventType]int{}
	dispatchID := ""
	for {
		select {
		case event, ok := <-stream.Events:
			if !ok {
				t.Fatal("event stream closed before attributed completion")
			}
			if !matchesInitialOpeningRequest(event, requestID, dispatchID) {
				continue
			}

			if event.Type == factorydefinitions.FactoryEventTypeDispatchRequest {
				if event.Context.DispatchID == nil || *event.Context.DispatchID == "" {
					t.Fatal("attributed dispatch omitted its identity")
				}
				dispatchID = *event.Context.DispatchID
			}
			if event.Context.SessionID == nil || *event.Context.SessionID != sessionID {
				t.Fatalf("request %s reached another session: %#v", requestID, event.Context)
			}
			counts[event.Type]++
			if event.Type == factorydefinitions.FactoryEventTypeDispatchResponse {
				assertInitialOpeningCompletedEvents(t, counts, stream.History, requestID)

				return
			}
		case <-ctx.Done():
			t.Fatalf("attributed submission/dispatch completion: %v counts=%v", ctx.Err(), counts)
		}
	}
}

func testNestedInitialOpening(t *testing.T, sessions factorysessions.Service, process support.Process, serverURL, workspace, home string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", serverURL, "session", "create", "--dir", workspace, "--init-new-factory"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = workspace
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("create nested workspace: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var created struct {
		FolderPath string `json:"folderPath"`
		Session    struct {
			ID         string `json:"id"`
			FolderPath string `json:"folderPath"`
		} `json:"session"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &created); err != nil {
		t.Fatalf("decode created session: %v stdout=%s", err, inputs.Stdout())
	}
	if created.FolderPath != workspace || created.Session.FolderPath != workspace || created.Session.ID == "" {
		t.Fatalf("nested session identity = %#v, want workspace %s", created, workspace)
	}
	t.Cleanup(func() {
		_, err := sessions.Control(context.Background(), factorysessions.SessionControlRequest{SessionID: created.Session.ID, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlClose})
		if err != nil {
			t.Error(err)
		}
	})
	read, err := sessions.Get(t.Context(), factorysessions.SessionGetRequest{SessionID: created.Session.ID, Mode: factorysessions.SessionOperationModeLive})
	if err != nil || read.Session.FolderPath != workspace {
		t.Fatalf("read nested session = %#v, %v", read, err)
	}
	response, err := http.Get(serverURL + "/factory-sessions/" + url.PathEscape(created.Session.ID) + "/factory")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var current struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil || response.StatusCode != http.StatusOK || current.Name == "" || current.Name == "UNDEFINED" {
		t.Fatalf("nested Current Factory = %#v status=%d error=%v", current, response.StatusCode, err)
	}
}

func matchesInitialOpeningRequest(event factorydefinitions.FactoryEvent, requestID, dispatchID string) bool {
	if event.Context.RequestID != nil && *event.Context.RequestID == requestID {
		return true
	}
	return event.Type == factorydefinitions.FactoryEventTypeDispatchResponse && event.Context.DispatchID != nil && *event.Context.DispatchID == dispatchID
}

func runInitialOpeningCompatibilityScenarios(t *testing.T, sessions factorysessions.Service, process support.Process, serverURL, home string) {
	t.Helper()
	t.Run("nested workspace keeps folder identity and current Factory", func(t *testing.T) {
		t.Parallel()
		testNestedInitialOpening(t, sessions, process, serverURL, t.TempDir(), home)
	})
	t.Run("CLI submission dispatch and history stay independently attributed", func(t *testing.T) {
		t.Parallel()
		testInitialOpeningAttributedHistory(t, sessions, process, newInitialOpeningScenario(t), serverURL)
	})
}

func assertInitialOpeningCompletedEvents(t *testing.T, counts map[factorydefinitions.FactoryEventType]int, before []factorydefinitions.FactoryEvent, requestID string) {
	t.Helper()
	for _, kind := range []factorydefinitions.FactoryEventType{factorydefinitions.FactoryEventTypeWorkRequest, factorydefinitions.FactoryEventTypeDispatchRequest, factorydefinitions.FactoryEventTypeDispatchResponse} {
		if counts[kind] != 1 {
			t.Fatalf("request event attribution/order = %v, want one submission, dispatch and completion", counts)
		}
	}
	for _, old := range before {
		if old.Context.RequestID != nil && *old.Context.RequestID == requestID {
			t.Fatal("fresh request already present in prior history")
		}
	}
}

func assertInitialOpeningWorkProjection(t *testing.T, process support.Process, scenario initialOpeningScenario, serverURL, sessionID, requestID string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", serverURL, "--session", sessionID, "work", "list"})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("read projected Work: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var projected struct {
		Results []struct {
			WorkID    string `json:"workId"`
			RequestID string `json:"requestId"`
			Name      string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &projected); err != nil {
		t.Fatalf("decode projected Work: %v stdout=%s", err, inputs.Stdout())
	}
	found := false
	for _, item := range projected.Results {
		if item.Name != "attributed" {
			continue
		}
		if item.RequestID != requestID || item.WorkID != "work-"+sessionID {
			t.Fatalf("projected Work crossed session/request identity: %#v", item)
		}
		found = true
	}
	if !found {
		t.Fatalf("projected history lost submitted Work: %s", inputs.Stdout())
	}
}

func newBindingChildScenario(t *testing.T) initialOpeningScenario {
	t.Helper()
	scenario := newInitialOpeningChildScenario(t)
	path := filepath.Join(scenario.candidateDir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	config["orchestrator"].(map[string]any)["javascript"].(map[string]any)["inlineSource"].(map[string]any)["inline"] = `return parallel([
 {prompt: "scoped child 1", label: "child-1", modelProvider: "codex", model: "gpt-5-codex", resourceId: "opening-child-slot"},
 {prompt: "scoped child 2", label: "child-2", modelProvider: "codex", model: "gpt-5-codex", resourceId: "opening-child-slot"}
]);`
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return scenario
}

// Ordinary remote CLI invocation captures the selected live binding. Public
// durable reads observe the queued children before either provider is released.
func testBindingChildOverlap(t *testing.T, process support.Process, sessions factorysessions.Service, scenarios []initialOpeningScenario, gate *bindingChildGate, serverURL string) {
	t.Helper()
	defer gate.unblock()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	done := make([]chan bindingChildOutcome, len(scenarios))
	for i, scenario := range scenarios {
		startInitialOpeningSession(t, sessions, scenario.request())
		done[i] = startBindingChildCommand(t, ctx, process, scenario, serverURL, gate)
	}
	seen := map[string]bool{}
	for range scenarios {
		select {
		case request := <-gate.entered:
			if seen[request.WorkDir] || gate.selected[request.WorkDir] == "" {
				t.Fatalf("crossed or over-admitted child: %+v", request)
			}
			seen[request.WorkDir] = true
		case <-ctx.Done():
			t.Fatal("both selected child providers did not overlap")
		}
	}
	for _, scenario := range scenarios {
		assertBindingChildCapacity(t, ctx, process, scenario, serverURL)
	}
	gate.unblock()
	for i, scenario := range scenarios {
		select {
		case result := <-done[i]:
			if result.err != nil {
				t.Fatalf("CLI child invocation: %v", result.err)
			}
			assertBindingChildCompletion(t, ctx, sessions, result.id, scenario.candidateID)
		case <-ctx.Done():
			t.Fatal("CLI child invocation did not complete")
		}
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	for _, scenario := range scenarios {
		if gate.maximum[scenario.candidateDir] != 1 || gate.active[scenario.candidateDir] != 0 {
			t.Fatalf("child capacity over-admission or leak: max=%v active=%v", gate.maximum, gate.active)
		}
	}
}

func startBindingChildCommand(t *testing.T, ctx context.Context, process support.Process, scenario initialOpeningScenario, serverURL string, gate *bindingChildGate) chan bindingChildOutcome {
	t.Helper()
	inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", serverURL, "--session", scenario.candidateID,
		"--json", "run", "--factory", filepath.Join(scenario.candidateDir, "factory.json"), "--output", "primary", "--no-record", "scoped children"})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	done := make(chan bindingChildOutcome, 1)
	go func() {
		defer close(done)
		err := process.Execute(inputs.Input)
		var response factoryapi.InvocationResponse
		if err == nil {
			err = json.Unmarshal([]byte(inputs.Stdout()), &response)
		}
		id := ""
		if response.SessionId != nil {
			id = *response.SessionId
		}
		done <- bindingChildOutcome{id: id, err: err}
	}()
	t.Cleanup(func() {
		gate.unblock()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Error("owned child command did not join")
		}
	})
	return done
}

type bindingChildOutcome struct {
	id  string
	err error
}

func assertBindingChildCapacity(t *testing.T, ctx context.Context, process support.Process, scenario initialOpeningScenario, serverURL string) {
	t.Helper()
	// Requesting the existing capacity is a public no-op accounting read.
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "--server", serverURL, "session", "resource", "set",
		"opening-child-slot", "1", scenario.candidateID, "--request-id", uuid.NewString(), "--expected-revision", "0", "--reason", "observe held child capacity"})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("capacity accounting: %v %s", err, inputs.Stderr())
	}
	var capacity factoryapi.FactorySessionResourceCapacityResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &capacity); err != nil {
		t.Fatal(err)
	}
	if capacity.Outcome != factoryapi.FactorySessionResourceCapacityOutcomeNOOP || capacity.InUseCount != 1 || capacity.AvailableCount != 0 || capacity.EffectiveCapacity != 1 {
		t.Fatalf("selected held capacity: %+v", capacity)
	}
}

func assertBindingChildCompletion(t *testing.T, ctx context.Context, sessions factorysessions.Service, id, selected string) {
	t.Helper()
	assertBindingChildResult(t, ctx, sessions, id, selected)
	assertBindingChildDispatches(t, ctx, sessions, id)
	assertBindingChildResponses(t, ctx, sessions, id, selected)
}

func assertBindingChildResult(t *testing.T, ctx context.Context, sessions factorysessions.Service, id, selected string) {
	t.Helper()
	read, err := support.WaitForObservation(30*time.Second, func() (factorysessions.SessionReadResult, error) {
		return sessions.GetSession(ctx, id)
	}, func(value factorysessions.SessionReadResult) bool {
		return value.Status == factorysessions.LifecycleStatusSucceeded
	})
	if err != nil || read.Progress == nil || read.Progress.CompletedDispatches != 2 {
		t.Fatalf("selected child completion: %+v %v", read, err)
	}
	result, err := sessions.GetResult(ctx, id, factorysessions.ResultRequest{Mode: factorysessions.ResultModeFinal})
	if err != nil || strings.Count(string(result.PrimaryResult), selected+" COMPLETE") != 2 {
		t.Fatalf("selected child results: %+v %v", result, err)
	}
}

func assertBindingChildDispatches(t *testing.T, ctx context.Context, sessions factorysessions.Service, id string) {
	t.Helper()
	dispatches, err := sessions.ListDispatches(ctx, id)
	if err != nil || len(dispatches.Dispatches) != 2 {
		t.Fatalf("child dispatches: %+v %v", dispatches, err)
	}
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Attempt != 1 || dispatch.ModelProvider != "codex" || dispatch.ConfirmationState != factorysessions.ConfirmationStateUnconfirmed {
			t.Fatalf("child attempt/recording policy: %+v", dispatch)
		}
	}
}

func assertBindingChildResponses(t *testing.T, ctx context.Context, sessions factorysessions.Service, id, selected string) {
	t.Helper()
	subscription, err := sessions.SubscribeResponses(ctx, factorysessions.SessionResponseSubscriptionRequest{SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Cursor.Detach()
	events, err := subscription.Cursor.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	completed := map[string]bool{}
	var lastSequence int64
	for _, event := range events {
		if event.FactorySessionID != id || event.Sequence <= lastSequence || event.DispatchID == "" {
			t.Fatalf("crossed child progress: %+v", event)
		}
		lastSequence = event.Sequence
		if event.Phase == factorysessions.ResponseEventPhaseCompleted && event.Provenance.NativeEventType == "STREAM_COMPLETED" {
			completed[event.DispatchID] = true
		}
		if event.Provenance.Delivery == factorysessions.ResponseEventDeliveryNativeStream && event.Kind == factorysessions.ResponseEventKindMessage {
			var message factorysessions.ResponseEventMessage
			if err := json.Unmarshal(event.Payload, &message); err != nil {
				t.Fatal(err)
			}
			if len(message.ContentBlocks) != 1 || message.ContentBlocks[0].Text != selected+" COMPLETE" {
				t.Fatalf("crossed provider output: %s", event.Payload)
			}
		}
	}
	if len(completed) != 2 {
		t.Fatalf("lost child completion progress: %+v", completed)
	}
}

type bindingChildGate struct {
	mu              sync.Mutex
	selected        map[string]string
	active, maximum map[string]int
	entered         chan platformprocess.CommandRequest
	release         chan struct{}
	once            sync.Once
}

func (gate *bindingChildGate) unblock() { gate.once.Do(func() { close(gate.release) }) }
func (gate *bindingChildGate) run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	gate.mu.Lock()
	gate.active[request.WorkDir]++
	gate.maximum[request.WorkDir] = max(gate.maximum[request.WorkDir], gate.active[request.WorkDir])
	gate.mu.Unlock()
	defer func() { gate.mu.Lock(); gate.active[request.WorkDir]--; gate.mu.Unlock() }()
	select {
	case gate.entered <- request:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	select {
	case <-gate.release:
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(gate.selected[request.WorkDir] + " COMPLETE")}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func openingPolicyArgs(scenario initialOpeningScenario, serverURL string, i int) []string {
	args := []string{"you", "--remote", "--server", serverURL, "--session", scenario.candidateID,
		"run", "--factory", filepath.Join(scenario.candidateDir, "factory.json"), "--no-record", "--secret", openingPolicySecret(scenario), "selected policy Work"}
	switch i {
	case 0:
		args = append(args, "--quiet")
	case 1:
		args = append(args, "--verbose", "--json", "--output", "primary")
	case 3:
		args = append(args, "--json")
	}
	return args
}

// The sequence shares process safety time while separate invocation-local
// directories, identities and starters keep the live peer useful during replay.
func TestSelectedSchedulingStartupReplayCancellationAndRetry(t *testing.T) {
	t.Parallel()
	facts := clockwork.NewFakeClockAt(time.Date(2041, 2, 3, 4, 5, 6, 0, time.UTC))
	scheduler := newSelectedSchedulingClock()
	ready := make(chan struct{}, 16)
	bound := make(chan int, 16)
	joined := make(chan int, 16)
	failures := map[int]chan error{7438: make(chan error, 1), 7439: make(chan error, 1), 7440: make(chan error, 1), 7441: make(chan error, 1)}
	effects := &initialOpeningEffects{calls: make(map[string]int)}
	logCore, _ := observer.New(zap.InfoLevel)
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{Clock: facts, ProcessScheduler: scheduler, RecordingReadFile: os.ReadFile,
		ScriptCommandRunner: initialOpeningScriptRunner{effects: effects},
		ProcessLogger: zap.New(logCore).WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
			if entry.Message == "factory started" {
				ready <- struct{}{}
			}
			return nil
		})),
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			defer func() { joined <- request.Port }()
			request.OnBound(platformhttpserver.Binding{Host: "localhost", Port: request.Port})
			bound <- request.Port
			select {
			case <-ctx.Done():
				return nil
			case err := <-failures[request.Port]:
				return err
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, process)
	scenario := newInitialOpeningScenario(t)
	home := t.TempDir()
	hostDir := support.ScaffoldFactory(t, initialOpeningFactoryConfig())
	makeInputs := func(dir string, args ...string) *support.CapturedInputs {
		inputs := support.FakeInputs(t.Context(), append([]string{"you", "run", "--session", uuid.NewString(), "--dir", dir}, args...))
		inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
		inputs.Input.WorkingDirectory = dir
		return inputs
	}
	boot := makeInputs(hostDir, "--continuously", "--with-server", "--listen", "localhost:7438", "--quiet", "--no-record")
	support.InitializeCustomerHomeWithProcess(t, process, boot.Input.Env, hostDir)
	host := support.StartProcessCommand(t, process, boot.Input)
	if err := selectedSchedulingActivate(t, scheduler, func() error { selectedSchedulingReceive(t, bound); selectedSchedulingReceive(t, ready); return nil }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { host.Stop(t) })
	sessions := process.FactorySessions().FactorySessions().(factorysessions.Service)
	var peerHistory *factorydefinitions.FactoryEventStream
	if err := selectedSchedulingActivate(t, scheduler, func() error { peerHistory = scenario.startPeer(t, sessions); return nil }); err != nil {
		t.Fatal(err)
	}
	closeInitialOpeningSession(t, sessions, scenario.peerID)
	livePeer := newInitialOpeningScenario(t)
	if err := selectedSchedulingActivate(t, scheduler, func() error { livePeer.startPeer(t, sessions); return nil }); err != nil {
		t.Fatal(err)
	}
	for index, action := range []string{"complete", "api failure", "cancel", "retry"} {
		assertSelectedStartupSequence(t, process, sessions, makeInputs, facts, scheduler, ready, joined, failures, scenario, livePeer, peerHistory, index, action)
	}
	host.Stop(t)
	selectedSchedulingReceive(t, joined)
}

// All values belong to this one scenario; no invocation-wide lock gates peers.
func assertSelectedStartupSequence(t *testing.T, process support.Process, sessions factorysessions.Service, makeInputs func(string, ...string) *support.CapturedInputs, facts *clockwork.FakeClock, scheduler *selectedSchedulingClock, ready <-chan struct{}, joined <-chan int, failures map[int]chan error, scenario, livePeer initialOpeningScenario, peerHistory *factorydefinitions.FactoryEventStream, index int, action string) {
	t.Helper()
	// Each local CLI run opens its own Factory Session; addressed peer sessions
	// use the public Start identity contract above.
	dir := newInitialOpeningScenario(t).candidateDir
	workPath := filepath.Join(t.TempDir(), "startup.json")
	if err := os.WriteFile(workPath, []byte(`{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"startup","workTypeName":"task","state":"init","payload":"selected"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	port := 7439 + index%3
	inputs := makeInputs(dir, "--work", workPath, "--continuously", "--with-server", "--listen", "localhost:"+fmt.Sprint(port), "--no-record")
	command := support.StartProcessCommand(t, process, inputs.Input)
	var timer *selectedSchedulingTimer
	for timer == nil {
		select {
		case candidate := <-scheduler.created:
			if candidate.duration == 10*time.Millisecond {
				scheduler.acknowledge(candidate)
			}
			if candidate.duration == 250*time.Millisecond && !candidate.stopped.Load() {
				timer = candidate
			}
		case <-command.Done():
			t.Fatalf("startup ended before timer: %v stdout=%s stderr=%s", command.Err(), inputs.Stdout(), inputs.Stderr())
		case <-time.After(30 * time.Second):
			t.Fatal("startup readability timer missing")
		}
	}
	facts.Advance(time.Hour)
	if err := selectedSchedulingActivate(t, scheduler, func() error { assertInitialOpeningInvocation(t, sessions, livePeer.peerID); return nil }); err != nil {
		t.Fatal(err)
	}
	assertInitialOpeningReplay(t, process, filepath.Join(scenario.peerDir, "peer.jsonl"), scenario.peerID, peerHistory)
	select {
	case <-ready:
		t.Fatal("replay/fact progression published readiness")
	default:
	}
	if action == "api failure" {
		cause := errors.New("selected startup API failure")
		failures[port] <- cause
		command.AcceptError()
		selectedSchedulingReceive(t, command.Done())
		if !errors.Is(command.Err(), cause) {
			t.Fatalf("startup cause %v", command.Err())
		}
	} else if action == "cancel" {
		command.Stop(t)
	} else {
		scheduler.SetTick(int(scheduler.Now().Sub(time.Unix(0, 0))/time.Millisecond) + 250)
		select {
		case <-ready:
		case <-command.Done():
			t.Fatalf("startup failed after selected advance: %v stdout=%s stderr=%s", command.Err(), inputs.Stdout(), inputs.Stderr())
		case <-time.After(10 * time.Second):
			t.Fatalf("readiness output missing stdout=%s stderr=%s", inputs.Stdout(), inputs.Stderr())
		}
		command.Stop(t)
	}
	if !timer.stopped.Load() {
		t.Fatal("startup timer survived joined command")
	}
	if stopped := selectedSchedulingReceive(t, joined); stopped != port {
		t.Fatalf("joined port %d want %d", stopped, port)
	}
}
