package start_retry_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The public close must cancel only the selected session's running worker.
// All four exact command edges enter before close. The three peers stay gated
// until their live routes and event generations are observed after cleanup.
func testSelectedProviderCancellation(t *testing.T, sessions factorysessions.Service, scenarios []initialOpeningScenario, gate *selectedProviderGate, baseURL string) {
	t.Helper()
	defer gate.unblock()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	done := make([]chan error, len(scenarios))
	joinSelectedProviderCommands(t, cancel, gate, done)
	history := make([]*factorydefinitions.FactoryEventStream, len(scenarios))
	identities := make([]factoryapi.FactorySessionStreamIdentity, len(scenarios))
	for i, scenario := range scenarios {
		startInitialOpeningSession(t, sessions, scenario.request())
		identities[i] = selectedProviderStreamIdentity(t, baseURL, scenario.candidateID)
		done[i] = make(chan error, 1)
		go func() {
			defer close(done[i])
			done[i] <- selectedProviderInvoke(ctx, sessions, scenario.candidateID)
		}()
	}
	awaitSelectedProviders(t, ctx, gate)
	for i, scenario := range scenarios {
		history[i] = initialOpeningHistory(t, sessions, scenario.candidateID)
		if history[i].StreamGenerationID != identities[i].StreamGenerationID {
			t.Fatal("session route and retained history selected different generations")
		}
		for j := range i {
			if identities[j].StreamGenerationID == identities[i].StreamGenerationID {
				t.Fatal("explicit sessions shared an event generation")
			}
		}
	}
	closeInitialOpeningSession(t, sessions, scenarios[0].candidateID)
	select {
	case err := <-done[0]:
		if err == nil {
			t.Fatal("closed candidate returned successful worker output")
		}
	case <-ctx.Done():
		t.Fatal("closing the candidate did not cancel its invocation")
	}
	assertInitialOpeningNotPublished(t, sessions, scenarios[0].candidateID)
	for i := 1; i < len(scenarios); i++ {
		scenario := scenarios[i]
		id := scenario.candidateID
		assertInitialOpeningHistoryPreserved(t, sessions, id, history[i])
		if !reflect.DeepEqual(selectedProviderStreamIdentity(t, baseURL, id), identities[i]) {
			t.Fatal("closing another session retargeted a peer stream")
		}
		assertGatedPeerWork(t, initialOpeningScenario{peerID: id}, baseURL, done[i])
	}
	gate.unblock()
	for i := 1; i < len(scenarios); i++ {
		awaitSelectedProviderCompletion(t, ctx, done[i])
		assertInitialOpeningHistoryPreserved(t, sessions, scenarios[i].candidateID, history[i])
		assertSelectedProviderResponses(t, ctx, sessions, scenarios[i].candidateID)
		if err := selectedProviderInvoke(ctx, sessions, scenarios[i].candidateID); err != nil {
			t.Fatal(err)
		}
	}
}

func assertSelectedProviderResponses(t *testing.T, ctx context.Context, sessions factorysessions.Service, id string) {
	t.Helper()
	subscription, err := sessions.SubscribeResponses(ctx, factorysessions.SessionResponseSubscriptionRequest{SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Cursor.Detach()
	var lastSequence int64
	var messages int
	// Invocation completion is asserted by selectedProviderInvoke. The public
	// response cursor independently proves the provider's attributed message;
	// this live Work route does not promise the durable child's terminal frame.
	for messages == 0 {
		events, err := subscription.Cursor.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.FactorySessionID != id || event.Sequence <= lastSequence || event.DispatchID == "" {
				t.Fatalf("peer response lost identity or ordering: %+v", event)
			}
			lastSequence = event.Sequence
			if event.Kind != factorysessions.ResponseEventKindMessage || event.Phase != factorysessions.ResponseEventPhaseCompleted || event.Provenance.Delivery != factorysessions.ResponseEventDeliveryNativeStream {
				continue
			}
			var message factorysessions.ResponseEventMessage
			if err := json.Unmarshal(event.Payload, &message); err != nil {
				t.Fatal(err)
			}
			if event.Provenance.Provider != "codex" || len(message.ContentBlocks) != 1 || message.ContentBlocks[0].Text != id+" COMPLETE" {
				t.Fatalf("peer response crossed provider/session output: %s", event.Payload)
			}
			messages++
		}
	}
	if messages != 1 {
		t.Fatalf("peer completed native response messages=%d, want one", messages)
	}
}

func joinSelectedProviderCommands(t *testing.T, cancel context.CancelFunc, gate *selectedProviderGate, done []chan error) {
	t.Helper()
	t.Cleanup(func() {
		cancel()
		gate.unblock()
		for _, completion := range done {
			if completion == nil {
				continue
			}
			select {
			case <-completion:
			case <-time.After(initialOpeningReadCeiling):
				t.Error("owned explicit invocation did not join")
			}
		}
	})
}

func selectedProviderStreamIdentity(t *testing.T, baseURL, id string) factoryapi.FactorySessionStreamIdentity {
	t.Helper()
	session := support.GetJSON[factoryapi.FactorySession](t, baseURL+"/factory-sessions/"+id)
	if session.Id != id || session.Runtime.StreamIdentity == nil || session.Runtime.StreamIdentity.FactorySessionID != id || session.Runtime.StreamIdentity.StreamGenerationID == "" {
		t.Fatalf("selected session stream identity = %#v, want %s", session, id)
	}
	return *session.Runtime.StreamIdentity
}

func awaitSelectedProviderCompletion(t *testing.T, ctx context.Context, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("peer did not retain its selected result after candidate cleanup: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("peer invocation did not finish after release")
	}
}

func awaitSelectedProviders(t *testing.T, ctx context.Context, gate *selectedProviderGate) {
	t.Helper()
	seen := make(map[string]bool)
	for range len(gate.paths) {
		select {
		case request := <-gate.entered:
			id := gate.paths[request.WorkDir]
			if id == "" || seen[id] {
				t.Fatalf("overlapping command identity = %q, seen=%v", id, seen)
			}
			seen[id] = true
		case <-ctx.Done():
			t.Fatal("all workers did not reach their owned command gates")
		}
	}
}

func assertGatedPeerWork(t *testing.T, scenario initialOpeningScenario, baseURL string, peerDone <-chan error) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, scenario.peerID, "/work"))
	if len(listed.Results) != 1 || fmt.Sprint(listed.Results[0].Payload) != scenario.peerID+" selected Work" {
		t.Fatalf("peer Work changed during candidate cleanup: %#v", listed)
	}
	select {
	case err := <-peerDone:
		t.Fatalf("peer invocation ended before its gate was released: %v", err)
	default:
	}
}

// Select the failed listener through the public host request. The exact edge
// fails once; retry uses the same session identity and listener request.
const completionFailurePort = 24188

func testCompletionHostFailure(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario, gate *selectedProviderGate, cause error, retryAPI *support.ProcessAPIServer) {
	t.Helper()
	defer gate.unblock()
	peer := scenario.request()
	peer.SessionID, peer.FolderPath = scenario.peerID, scenario.peerDir
	peer.RuntimeSelection.DefinitionSourcePath = scenario.peerDir + "/factory.json"
	peer.RuntimeSelection.ExecutionBaseDir, peer.RuntimeSelection.RuntimeInstanceID = scenario.peerDir, uuid.NewString()
	startInitialOpeningSession(t, sessions, peer)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- selectedProviderInvoke(ctx, sessions, scenario.peerID) }()
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("peer did not enter command gate")
	}
	history := initialOpeningHistory(t, sessions, scenario.peerID)
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--session", scenario.candidateID, "--dir", scenario.candidateDir, "--continuously", "--with-server", "--server", "http://127.0.0.1:24188", "--quiet", "--no-record"})
	inputs.Input.Env = append(os.Environ(), "HOME="+scenario.home, "USERPROFILE="+scenario.home)
	inputs.Input.WorkingDirectory = scenario.candidateDir
	err := process.Execute(inputs.Input)
	if !errors.Is(err, cause) && (err == nil || !strings.Contains(inputs.Stderr(), cause.Error())) {
		t.Fatalf("host failure lost primary cause: %v stderr=%s", err, inputs.Stderr())
	}
	assertInitialOpeningNotPublished(t, sessions, scenario.candidateID)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, history)
	select {
	case err := <-done:
		t.Fatalf("host rollback ended gated peer: %v", err)
	default:
	}
	retry := support.StartProcessCommand(t, process, inputs.Input)
	retryAPI.WaitForURL(t)
	t.Cleanup(func() { retry.Stop(t) })
	assertInitialOpeningInvocation(t, sessions, scenario.candidateID)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, history)
	gate.unblock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("peer lost result after retry: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("peer did not complete")
	}
	if err := selectedProviderInvoke(ctx, sessions, scenario.peerID); err != nil {
		t.Fatal(err)
	}
}
