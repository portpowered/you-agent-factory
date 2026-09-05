package execution_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const fscp01DispatchSourceUnproven = "UNPROVEN"

type fscp01DispatchFieldSource struct {
	Active   string
	Terminal string
	Evidence string
}

// This is deliberately a field-level disposition table rather than an
// ownership assertion. Public list/detail values are observable, but the
// current response does not identify whether a value came from a Runtime
// snapshot, durable execution state, or a Recording projection. A recording-
// only/replay witness is required before any source claim can be made.
var fscp01DispatchFieldSources = map[string]fscp01DispatchFieldSource{
	"id":                    fscp01UnprovenDispatchFieldSource(),
	"status":                fscp01UnprovenDispatchFieldSource(),
	"confirmationState":     fscp01UnprovenDispatchFieldSource(),
	"dispatchKind":          fscp01UnprovenDispatchFieldSource(),
	"phase":                 fscp01UnprovenDispatchFieldSource(),
	"label":                 fscp01UnprovenDispatchFieldSource(),
	"attempt":               fscp01UnprovenDispatchFieldSource(),
	"retryable":             fscp01UnprovenDispatchFieldSource(),
	"failureClassification": fscp01UnprovenDispatchFieldSource(),
	"failureDetail":         fscp01UnprovenDispatchFieldSource(),
	"runnerId":              fscp01UnprovenDispatchFieldSource(),
	"presetId":              fscp01UnprovenDispatchFieldSource(),
	"modelProvider":         fscp01UnprovenDispatchFieldSource(),
	"model":                 fscp01UnprovenDispatchFieldSource(),
	"reasoningEffort":       fscp01UnprovenDispatchFieldSource(),
	"provider":              fscp01UnprovenDispatchFieldSource(),
	"providerSessionRefs":   fscp01UnprovenDispatchFieldSource(),
	"usage":                 fscp01UnprovenDispatchFieldSource(),
	"warnings":              fscp01UnprovenDispatchFieldSource(),
	"outputArtifactIds":     fscp01UnprovenDispatchFieldSource(),
	"artifactIds":           fscp01UnprovenDispatchFieldSource(),
	"javascript":            fscp01UnprovenDispatchFieldSource(),
	"petri":                 fscp01UnprovenDispatchFieldSource(),
	"sessionId":             fscp01UnprovenDispatchFieldSource(),
	"orchestratorKind":      fscp01UnprovenDispatchFieldSource(),
	"statusTransitions":     fscp01UnprovenDispatchFieldSource(),
}

func fscp01UnprovenDispatchFieldSource() fscp01DispatchFieldSource {
	return fscp01DispatchFieldSource{
		Active:   fscp01DispatchSourceUnproven,
		Terminal: fscp01DispatchSourceUnproven,
		Evidence: "public list/detail field observed; source is not distinguishable at this boundary",
	}
}

const fscp01LiveDispatchCorrelationWorkflow = `return (async function () {
  const child = await agent.run({
    prompt: "` + dispatchCorrelationChildPrompt + `",
    label: "` + dispatchCorrelationChildLabel + `",
    modelProvider: "codex",
    model: "fscp01-codex-model",
  });
  return { child };
})();`

// TestFSCP01DispatchReadFieldProvenanceMatrix proves active and terminal
// public dispatch list/detail reads against real root-built sessions and emits
// an explicit provenance disposition for every returned JSON field. It also
// joins each selected dispatch to its canonical Worker Session association and
// records the current public inference-attempt evidence, including an explicit
// UNPROVEN result when that event is not emitted for the dispatch.
func TestFSCP01DispatchReadFieldProvenanceMatrix(t *testing.T) {
	t.Parallel()

	t.Run("active", func(t *testing.T) {
		t.Parallel()
		acquireExecutionFixtureSlot(t)
		gate := make(chan struct{})
		var release sync.Once
		releaseGate := func() { release.Do(func() { close(gate) }) }
		runner := support.NewGatedSuccessCommandRunner("fscp01 active provider output", gate)
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                support.ScaffoldFactory(t, map[string]any{"name": "fscp01-dispatch-active"}),
			WaitForServiceModeRuntime: true,
			Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
		})
		t.Cleanup(func() { server.Stop(t) })
		t.Cleanup(releaseGate)

		started := startFSCP01DispatchWorkflowAsync(t, server.URL(), fscp01LiveDispatchCorrelationWorkflow)
		if strings.TrimSpace(started.SessionId) == "" {
			t.Fatal("active session id is empty")
		}

		waitForDurableSessionStatus(t, server.URL(), started.SessionId, factoryapi.FactorySessionDurableLifecycleStatusRunning, 5*time.Second)
		listed := waitForFSCP01DispatchWithLabelStatus(t, server.URL(), started.SessionId, dispatchCorrelationChildLabel, factoryapi.FactoryDispatchStatusRUNNING)
		if listed.SessionId != started.SessionId {
			t.Fatalf("active dispatch list sessionId = %q, want %q", listed.SessionId, started.SessionId)
		}
		summary := requireFSCP01DispatchSummaryByLabel(t, listed, dispatchCorrelationChildLabel)
		if summary.Status != factoryapi.FactoryDispatchStatusRUNNING {
			t.Fatalf("active dispatch summary status = %q, want RUNNING", summary.Status)
		}
		detail := getFactorySessionDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertFSCP01DispatchListDetail(t, started.SessionId, summary, detail)
		facts := observeFSCP01CanonicalDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertFSCP01DispatchAttemptAndWorkerIdentity(t, detail, facts)
		recordFSCP01DispatchFieldSources(t, "active", summary, detail)
		releaseGate()
		waitForDurableSessionStatus(t, server.URL(), started.SessionId, factoryapi.FactorySessionDurableLifecycleStatusSucceeded, 5*time.Second)
	})

	t.Run("terminal", func(t *testing.T) {
		t.Parallel()
		acquireExecutionFixtureSlot(t)
		dir := support.ScaffoldFactory(t, map[string]any{"name": "fscp01-dispatch-terminal"})
		runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
			Stdout: []byte("fscp01 terminal provider output"),
		})
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
		})
		t.Cleanup(func() { server.Stop(t) })

		started := startFSCP01DispatchWorkflowSync(t, server.URL(), fscp01LiveDispatchCorrelationWorkflow)
		if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
			t.Fatalf("terminal session status = %q, want SUCCEEDED", started.Status)
		}
		listed := listFactorySessionDispatches(t, server.URL(), started.SessionId)
		if listed.SessionId != started.SessionId || len(listed.Dispatches) == 0 {
			t.Fatalf("terminal dispatch list = %#v, want rows scoped to %q", listed, started.SessionId)
		}
		summary := requireFSCP01DispatchSummaryByLabel(t, listed, dispatchCorrelationChildLabel)
		if summary.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("terminal dispatch summary status = %q, want COMPLETED", summary.Status)
		}
		if runner.CallCount() != 1 {
			t.Fatalf("terminal provider command calls = %d, want 1", runner.CallCount())
		}
		if request := runner.LastRequest(); strings.ToLower(strings.TrimSpace(request.Command)) != "codex" {
			t.Fatalf("terminal provider command = %q, want codex", request.Command)
		}
		detail := getFactorySessionDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertFSCP01DispatchListDetail(t, started.SessionId, summary, detail)
		facts := observeFSCP01CanonicalDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertFSCP01DispatchAttemptAndWorkerIdentity(t, detail, facts)
		recordFSCP01DispatchFieldSources(t, "terminal", summary, detail)

		// A second terminal list/detail read is the public stability check for the
		// fields with explicit dispositions in the current matrix.
		secondSummary := requireFSCP01DispatchSummary(t, listFactorySessionDispatches(t, server.URL(), started.SessionId), summary.Id)
		secondDetail := getFactorySessionDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertFSCP01DispatchListDetail(t, started.SessionId, secondSummary, secondDetail)

		missing := readFSCP01DispatchError(t, strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches/fscp01-missing-dispatch")
		assertFSCP01DispatchNotFound(t, missing, "missing dispatch")
		foreign := readFSCP01DispatchError(t, strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/fscp01-foreign-session/dispatches/"+summary.Id)
		assertFSCP01DispatchNotFound(t, foreign, "foreign session dispatch")
	})
}

func startFSCP01DispatchWorkflowAsync(
	t *testing.T,
	serverURL, source string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()

	payload := postFSCP01DispatchWorkflow(t, serverURL, "fscp01-dispatch-async", source, "async")
	var started factoryapi.FactorySessionExecutionResponse
	if err := json.Unmarshal(payload, &started); err != nil {
		t.Fatalf("decode FSCP-01 dispatch async response: %v", err)
	}
	return started
}

func startFSCP01DispatchWorkflowSync(
	t *testing.T,
	serverURL, source string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	payload := postFSCP01DispatchWorkflow(t, serverURL, "fscp01-dispatch-sync", source, "sync")
	var started factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(payload, &started); err != nil {
		t.Fatalf("decode FSCP-01 dispatch sync response: %v", err)
	}
	return started
}

func postFSCP01DispatchWorkflow(
	t *testing.T,
	serverURL, requestID, source, mode string,
) []byte {
	t.Helper()

	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   source,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal FSCP-01 dispatch %s request: %v", mode, err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + mode
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build FSCP-01 dispatch %s request: %v", mode, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read FSCP-01 dispatch %s response: %v", mode, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return body
}

func waitForFSCP01DispatchWithLabelStatus(
	t *testing.T,
	serverURL, sessionID, label string,
	want factoryapi.FactoryDispatchStatus,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()

	last, err := support.WaitForObservation(
		5*time.Second,
		func() (factoryapi.ListFactorySessionDispatchesResponse, error) {
			return listFactorySessionDispatches(t, serverURL, sessionID), nil
		},
		func(listed factoryapi.ListFactorySessionDispatchesResponse) bool {
			for _, dispatch := range listed.Dispatches {
				if dispatch.Label != nil && *dispatch.Label == label && dispatch.Status == want {
					return true
				}
			}
			return false
		},
	)
	if err != nil {
		t.Fatalf("dispatch label %q did not reach %s: %v; last dispatches=%#v", label, want, err, last.Dispatches)
	}
	return last
}

type fscp01CanonicalDispatchFacts struct {
	HasDispatchStart bool
	HasAssociation   bool
	WorkerSessionID  string
	Attempts         map[int]struct{}
}

func observeFSCP01CanonicalDispatch(
	t *testing.T,
	serverURL, sessionID, dispatchID string,
) fscp01CanonicalDispatchFacts {
	t.Helper()
	events := support.GetFactoryEventsForSessionAt(t, serverURL, sessionID)
	facts := fscp01CanonicalDispatchFacts{Attempts: make(map[int]struct{})}
	observeFSCP01CanonicalDispatchEvents(t, sessionID, dispatchID, events, &facts)
	// The active default-session stream retains Worker Session associations
	// while the durable session history retains dispatch queue/reconcile facts.
	// Read both public canonical surfaces and join only by the dispatch context.
	defaultEvents := support.GetFactoryEventsAt(t, serverURL)
	observeFSCP01CanonicalDispatchEvents(t, sessionID, dispatchID, defaultEvents, &facts)

	if !facts.HasAssociation {
		t.Fatalf("public canonical history for dispatch %q has no Worker Session association", dispatchID)
	}
	if !facts.HasDispatchStart {
		t.Fatalf("canonical history for dispatch %q has no DISPATCH_QUEUED or DISPATCH_REQUEST", dispatchID)
	}
	return facts
}

func observeFSCP01CanonicalDispatchEvents(
	t *testing.T,
	sessionID, dispatchID string,
	events []factoryapi.FactoryEvent,
	facts *fscp01CanonicalDispatchFacts,
) {
	t.Helper()
	for _, event := range events {
		if event.Context.DispatchId == nil || !fscp01CanonicalDispatchIDMatches(*event.Context.DispatchId, sessionID, dispatchID) {
			continue
		}
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID && *event.Context.SessionId != "~default" {
			t.Fatalf("canonical dispatch event %q sessionId = %q, want %q", event.Id, *event.Context.SessionId, sessionID)
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			if _, err := event.Payload.AsDispatchRequestEventPayload(); err != nil {
				t.Fatalf("decode canonical dispatch request %q: %v", event.Id, err)
			}
			facts.HasDispatchStart = true
		case factoryapi.FactoryEventTypeDispatchQueued:
			if _, err := event.Payload.AsDispatchQueuedEventPayload(); err != nil {
				t.Fatalf("decode canonical dispatch queued event %q: %v", event.Id, err)
			}
			facts.HasDispatchStart = true
		case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
			association, err := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
			if err != nil {
				t.Fatalf("decode canonical Worker Session association %q: %v", event.Id, err)
			}
			if strings.TrimSpace(association.WorkerSessionId) == "" {
				t.Fatalf("canonical Worker Session association %q has empty workerSessionId", event.Id)
			}
			if facts.HasAssociation && facts.WorkerSessionID != association.WorkerSessionId {
				t.Fatalf("dispatch %q changed Worker Session identity from %q to %q", dispatchID, facts.WorkerSessionID, association.WorkerSessionId)
			}
			facts.HasAssociation = true
			facts.WorkerSessionID = association.WorkerSessionId
		case factoryapi.FactoryEventTypeInferenceRequest:
			attempt, err := event.Payload.AsInferenceRequestEventPayload()
			if err != nil {
				t.Fatalf("decode canonical inference request %q: %v", event.Id, err)
			}
			facts.Attempts[attempt.Attempt] = struct{}{}
		case factoryapi.FactoryEventTypeInferenceResponse:
			attempt, err := event.Payload.AsInferenceResponseEventPayload()
			if err != nil {
				t.Fatalf("decode canonical inference response %q: %v", event.Id, err)
			}
			facts.Attempts[attempt.Attempt] = struct{}{}
		}
	}
}

func fscp01CanonicalDispatchIDMatches(canonicalID, sessionID, dispatchID string) bool {
	return canonicalID == dispatchID || canonicalID == sessionID+"/"+dispatchID
}

func assertFSCP01DispatchAttemptAndWorkerIdentity(
	t *testing.T,
	detail factoryapi.FactoryDispatch,
	facts fscp01CanonicalDispatchFacts,
) {
	t.Helper()
	if detail.Attempt == nil || *detail.Attempt < 1 {
		t.Fatalf("dispatch %q attempt = %#v, want one-based public attempt", detail.Id, detail.Attempt)
	}
	if len(facts.Attempts) == 0 {
		t.Logf("FSCP-01 dispatch identity: dispatch=%s attempt=%d workerSession=%s inferenceAttempts=[] source=%s evidence=canonical public stream emitted no inference request/response for this dispatch", detail.Id, *detail.Attempt, facts.WorkerSessionID, fscp01DispatchSourceUnproven)
		return
	}
	if _, ok := facts.Attempts[int(*detail.Attempt)]; !ok {
		t.Fatalf("dispatch %q detail attempt = %d, canonical inference attempts = %#v", detail.Id, *detail.Attempt, facts.Attempts)
	}
	t.Logf("FSCP-01 dispatch identity: dispatch=%s attempt=%d workerSession=%s inferenceAttempts=%v", detail.Id, *detail.Attempt, facts.WorkerSessionID, sortedFSCP01AttemptKeys(facts.Attempts))
}

func assertFSCP01DispatchListDetail(
	t *testing.T,
	sessionID string,
	summary factoryapi.FactorySessionDispatchSummary,
	detail factoryapi.FactoryDispatch,
) {
	t.Helper()
	assertDispatchListDetailPublicCorrelation(t, sessionID, summary, detail)
	if summary.Attempt != nil && detail.Attempt == nil {
		t.Fatalf("dispatch %q detail attempt is nil while list summary has %d", summary.Id, *summary.Attempt)
	}
	if summary.Attempt != nil && detail.Attempt != nil && *summary.Attempt != *detail.Attempt {
		t.Fatalf("dispatch %q attempt list/detail = %d/%d", summary.Id, *summary.Attempt, *detail.Attempt)
	}
	assertFSCP01SharedDispatchFieldsMatch(t, summary, detail)
}

func assertFSCP01SharedDispatchFieldsMatch(
	t *testing.T,
	summary factoryapi.FactorySessionDispatchSummary,
	detail factoryapi.FactoryDispatch,
) {
	t.Helper()
	summaryFields := marshalFSCP01JSONFields(t, summary)
	detailFields := marshalFSCP01JSONFields(t, detail)
	for field, summaryValue := range summaryFields {
		detailValue, ok := detailFields[field]
		if !ok {
			continue
		}
		if !bytes.Equal(summaryValue, detailValue) {
			t.Fatalf("dispatch %q shared field %q differs between list/detail: %s vs %s", summary.Id, field, summaryValue, detailValue)
		}
	}
}

func recordFSCP01DispatchFieldSources(
	t *testing.T,
	phase string,
	summary factoryapi.FactorySessionDispatchSummary,
	detail factoryapi.FactoryDispatch,
) {
	t.Helper()
	observedFields := make(map[string][]string)
	for field := range marshalFSCP01JSONFields(t, summary) {
		observedFields[field] = append(observedFields[field], "list")
	}
	for field := range marshalFSCP01JSONFields(t, detail) {
		observedFields[field] = append(observedFields[field], "detail")
	}
	if len(observedFields) == 0 {
		t.Fatalf("dispatch %q returned no public fields to classify", detail.Id)
	}
	fieldNames := make([]string, 0, len(observedFields))
	for field := range observedFields {
		fieldNames = append(fieldNames, field)
	}
	sort.Strings(fieldNames)
	var source string
	for _, field := range fieldNames {
		entry, ok := fscp01DispatchFieldSources[field]
		if !ok {
			t.Fatalf("dispatch %q field %q has no explicit provenance disposition", detail.Id, field)
		}
		if strings.TrimSpace(entry.Evidence) == "" {
			t.Fatalf("dispatch %q field %q has no explicit observed evidence", detail.Id, field)
		}
		switch phase {
		case "active":
			source = entry.Active
		case "terminal":
			source = entry.Terminal
		default:
			t.Fatalf("dispatch %q has unknown provenance phase %q", detail.Id, phase)
		}
		if source != fscp01DispatchSourceUnproven {
			t.Fatalf("dispatch %q field %q source = %q, want explicit %s until source-distinguishing replay evidence exists", detail.Id, field, source, fscp01DispatchSourceUnproven)
		}
		t.Logf("FSCP-01 dispatch provenance phase=%s dispatch=%s field=%s source=%s observed=%s evidence=%s", phase, detail.Id, field, source, strings.Join(observedFields[field], "+"), entry.Evidence)
	}
}

func marshalFSCP01JSONFields(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal dispatch public projection: %v", err)
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode dispatch public projection fields: %v: %s", err, payload)
	}
	return fields
}

func requireFSCP01DispatchSummary(
	t *testing.T,
	listed factoryapi.ListFactorySessionDispatchesResponse,
	dispatchID string,
) factoryapi.FactorySessionDispatchSummary {
	t.Helper()
	for _, summary := range listed.Dispatches {
		if summary.Id == dispatchID {
			return summary
		}
	}
	t.Fatalf("dispatch list = %#v, want dispatch %q", listed.Dispatches, dispatchID)
	return factoryapi.FactorySessionDispatchSummary{}
}

func requireFSCP01DispatchSummaryByLabel(
	t *testing.T,
	listed factoryapi.ListFactorySessionDispatchesResponse,
	label string,
) factoryapi.FactorySessionDispatchSummary {
	t.Helper()
	for _, summary := range listed.Dispatches {
		if summary.Label != nil && *summary.Label == label {
			return summary
		}
	}
	t.Fatalf("dispatch list = %#v, want label %q", listed.Dispatches, label)
	return factoryapi.FactorySessionDispatchSummary{}
}

type fscp01DispatchHTTPError struct {
	Status   int
	Response factoryapi.ErrorResponse
}

func readFSCP01DispatchError(t *testing.T, endpoint string) fscp01DispatchHTTPError {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build dispatch negative read request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s error: %v", endpoint, err)
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		t.Fatalf("GET %s status = %d, want typed failure: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var typed factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &typed); err != nil {
		t.Fatalf("decode typed dispatch failure from GET %s: %v: %s", endpoint, err, strings.TrimSpace(string(body)))
	}
	if strings.TrimSpace(string(typed.Code)) == "" || strings.TrimSpace(string(typed.Family)) == "" {
		t.Fatalf("GET %s returned incomplete typed failure: %#v", endpoint, typed)
	}
	return fscp01DispatchHTTPError{Status: response.StatusCode, Response: typed}
}

func assertFSCP01DispatchNotFound(t *testing.T, got fscp01DispatchHTTPError, label string) {
	t.Helper()
	if got.Status != http.StatusNotFound || got.Response.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("%s error = %#v, want 404 NOT_FOUND", label, got)
	}
}

func sortedFSCP01AttemptKeys(attempts map[int]struct{}) []int {
	keys := make([]int, 0, len(attempts))
	for attempt := range attempts {
		keys = append(keys, attempt)
	}
	sort.Ints(keys)
	return keys
}
