package factory_events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const fscp01ResponseTeardownWorkflow = `return (async function () {
  const artifactRef = workflow.artifact({
    kind: "log",
    label: "fscp01-response-teardown-artifact",
    content: { message: "canonical reads follow response teardown" },
  });
  const prefix = await agent.run({
    prompt: "fscp01 response teardown prefix COMPLETE",
    label: "fscp01-response-teardown-prefix",
    modelProvider: "codex",
    model: "fscp01-response-prefix-model",
  });
  const child = await agent.run({
    prompt: "fscp01 response teardown child COMPLETE",
    label: "fscp01-response-teardown-child",
    modelProvider: "codex",
    model: "fscp01-response-model",
  });
  return { artifactRef, prefix, child };
})();`

// TestFSCP01ResponseTeardownThenCanonicalReconnectAndArtifactReads opens a
// live session-scoped Response Event stream, observes an ordered prefix, and
// deliberately cancels the subscription while the controlled provider is
// held. The invocation then completes independently, after which the
// Recordings-owned canonical event reconnect and artifact list/detail/read
// path is exercised, including its typed negative reads.
func TestFSCP01ResponseTeardownThenCanonicalReconnectAndArtifactReads(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	var release sync.Once
	releaseGate := func() { release.Do(func() { close(gate) }) }
	runner := newFSCP01FirstThenGatedRunner(gate)
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "fscp01-response-teardown",
	})
	locations := newFSCP01ResponseRunLocations(t)
	logFSCP01ResponseRunDeclaration(t, locations, dir, "one root; gated invocation survives response teardown")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env:                       locations.Env,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	logFSCP01ResponseBoundPort(t, server.URL())
	t.Cleanup(func() { server.Stop(t) })
	t.Cleanup(releaseGate)

	started := startFSCP01ResponseTeardownExecution(t, server.URL())
	if strings.TrimSpace(started.SessionId) == "" {
		t.Fatal("response teardown durable session id is empty")
	}
	waitFSCP01DurableStatus(t, server.URL(), started.SessionId, factoryapi.FactorySessionDurableLifecycleStatusRunning)

	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(server.URL(), started.SessionId),
	)
	runner.WaitForSecondCall(t)
	// The first child supplies a retained response prefix. The second provider
	// command remains gated while the stream observes that prefix, so the status
	// assertion below proves an in-flight teardown rather than cancellation after
	// the durable invocation reached its terminal boundary.
	framesBeforeTeardown := []support.FactoryResponseEventFrame{
		stream.NextFrame(5 * time.Second),
		stream.NextFrame(5 * time.Second),
	}
	assertFSCP01ResponseTeardownFrames(t, started.SessionId, framesBeforeTeardown)
	if runner.CallCount() != 2 {
		t.Fatalf("controlled provider command calls before teardown = %d, want 2", runner.CallCount())
	}
	preTeardown, err := readFSCP01DurableSession(t, server.URL(), started.SessionId)
	if err != nil {
		t.Fatalf("read pre-teardown durable session %q: %v", started.SessionId, err)
	}
	if preTeardown.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("pre-teardown durable status = %q, want RUNNING while provider gate is closed", preTeardown.Status)
	}

	// Close is a deliberate client teardown, not the terminal session boundary:
	// the provider command remains gated and the durable invocation is still
	// active while the subscription is torn down. Release the provider only
	// after the canceled stream outcome has been observed.
	stream.Close()
	stream.WaitClosed(5 * time.Second)
	frames := append([]support.FactoryResponseEventFrame(nil), framesBeforeTeardown...)
	for {
		result := stream.TryNextFrameResult(5 * time.Second)
		if result.Outcome == support.FactoryResponseEventStreamOutcomeFrame {
			frames = append(frames, result.Frame)
			continue
		}
		if result.Outcome != support.FactoryResponseEventStreamOutcomeCanceled {
			t.Fatalf("response stream teardown outcome = %q (%s), want deliberate cancellation", result.Outcome, result.Diagnostic())
		}
		break
	}
	assertFSCP01ResponseTeardownFrames(t, started.SessionId, frames)
	releaseGate()

	status := waitFSCP01DurableTerminal(t, server.URL(), started.SessionId)
	if status.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("post-teardown durable status = %q, want SUCCEEDED", status.Status)
	}

	fullRead, cursorEvent := assertFSCP01CanonicalEventReads(t, server.URL(), started.SessionId)
	artifactCount := assertFSCP01CanonicalArtifactReads(t, server.URL(), started.SessionId, dir)
	finalRead := support.GetFactoryEventsForSessionAt(t, server.URL(), started.SessionId)
	assertFactoryEventsSameRelativeOrder(t, fullRead, finalRead)
	t.Logf("FSCP-01 response handoff evidence: session=%s responseFramesBeforeTeardown=%d responseFramesTotal=%d firstCursor=%s responseOutcome=CANCELED canonicalEvents=%d artifacts=%d", started.SessionId, len(framesBeforeTeardown), len(frames), cursorEvent.Id, len(fullRead), artifactCount)
}

func waitFSCP01DurableTerminal(
	t *testing.T,
	baseURL, sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	last, err := support.WaitForObservation(
		15*time.Second,
		func() (factoryapi.FactorySessionDurableReadModel, error) {
			return readFSCP01DurableSession(t, baseURL, sessionID)
		},
		func(model factoryapi.FactorySessionDurableReadModel) bool {
			switch model.Status {
			case factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
				factoryapi.FactorySessionDurableLifecycleStatusFailed,
				factoryapi.FactorySessionDurableLifecycleStatusCanceled:
				return true
			default:
				return false
			}
		},
	)
	if err != nil {
		t.Fatalf("timed out waiting for durable session %q after response teardown: %v", sessionID, err)
	}
	return last
}

func waitFSCP01DurableStatus(
	t *testing.T,
	baseURL, sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()
	last, err := support.WaitForObservation(
		15*time.Second,
		func() (factoryapi.FactorySessionDurableReadModel, error) {
			return readFSCP01DurableSession(t, baseURL, sessionID)
		},
		func(model factoryapi.FactorySessionDurableReadModel) bool { return model.Status == want },
	)
	if err != nil {
		t.Fatalf("durable session %q status = %q, want %q: %v", sessionID, last.Status, want, err)
	}
}

func readFSCP01DurableSession(
	t *testing.T,
	baseURL, sessionID string,
) (factoryapi.FactorySessionDurableReadModel, error) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID
	response, err := http.Get(endpoint)
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	if response.StatusCode != http.StatusOK {
		return factoryapi.FactorySessionDurableReadModel{}, fmt.Errorf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope factoryapi.FactorySessionGetResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	return envelope.AsFactorySessionDurableReadModel()
}

func startFSCP01ResponseTeardownExecution(
	t *testing.T,
	serverURL string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "fscp01-response-teardown-async",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   fscp01ResponseTeardownWorkflow,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal response teardown execution request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/async"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build response teardown execution request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s response: %v", endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var started factoryapi.FactorySessionExecutionResponse
	if err := json.Unmarshal(body, &started); err != nil {
		t.Fatalf("decode response teardown execution response: %v", err)
	}
	return started
}

func assertFSCP01ResponseTeardownFrames(
	t *testing.T,
	sessionID string,
	frames []support.FactoryResponseEventFrame,
) {
	t.Helper()
	if len(frames) < 2 {
		t.Fatalf("response teardown frames = %d, want at least two ordered observations", len(frames))
	}
	seen := make(map[string]struct{}, len(frames))
	var previous int64
	for index, frame := range frames {
		event := frame.Event
		if event.FactorySessionId != sessionID {
			t.Fatalf("response frame[%d] sessionId = %q, want %q", index, event.FactorySessionId, sessionID)
		}
		if strings.TrimSpace(event.EventId) == "" {
			t.Fatalf("response frame[%d] has empty eventId", index)
		}
		if _, duplicate := seen[event.EventId]; duplicate {
			t.Fatalf("response event %q was delivered more than once", event.EventId)
		}
		seen[event.EventId] = struct{}{}
		if event.Sequence <= previous {
			t.Fatalf("response frame[%d] sequence = %d after %d, want strict order", index, event.Sequence, previous)
		}
		previous = event.Sequence
	}
}
