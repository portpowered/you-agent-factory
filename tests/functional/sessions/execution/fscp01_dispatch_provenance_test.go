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

const (
	fscp01DispatchSourcePass           = "PASS"
	fscp01DispatchSourceInconclusive   = "INCONCLUSIVE"
	fscp01InferenceEventNotEmitted     = "INFERENCE_EVENT_NOT_EMITTED_BY_FIXTURE"
	fscp01SourcePlanSHA256             = "058bf1a1e74cbc64dfedb89bb83f0cbc3b805f941d489bb24bd207e00371794a"
	fscp01DispatchLifecycleNotRecorded = "DISPATCH_LIFECYCLE_EVENTS_NOT_RECORDED_BY_FIXTURE"
)

type fscp01DispatchFieldSource struct {
	Active   string
	Terminal string
	Evidence string
}

// This is deliberately a field-level disposition table rather than an
// ownership assertion. The current JavaScript fixture has no
// source-distinguishing fact for ID or Model at an allowed functional
// boundary, so those values remain INCONCLUSIVE rather than promoting
// same-value equality to a provenance claim. All other observed values retain
// a stable, typed blocker as well.
var fscp01DispatchFieldSources = map[string]fscp01DispatchFieldSource{
	"id":                    fscp01InconclusiveDispatchFieldSource("DISPATCH_SOURCE_NOT_DISTINGUISHABLE_BY_CURRENT_FIXTURE"),
	"status":                fscp01InconclusiveDispatchFieldSource(fscp01DispatchLifecycleNotRecorded),
	"confirmationState":     fscp01InconclusiveDispatchFieldSource("READ_BOUNDARY_CONFIRMATION_IS_TRANSIENT"),
	"dispatchKind":          fscp01InconclusiveDispatchFieldSource("DISPATCH_KIND_NOT_RETAINED_BY_ASSOCIATION_EVENT"),
	"phase":                 fscp01InconclusiveDispatchFieldSource("JAVASCRIPT_PHASE_NOT_RETAINED_BY_ASSOCIATION_EVENT"),
	"label":                 fscp01InconclusiveDispatchFieldSource("DISPATCH_LABEL_NOT_RETAINED_BY_ASSOCIATION_EVENT"),
	"attempt":               fscp01InconclusiveDispatchFieldSource(fscp01InferenceEventNotEmitted),
	"retryable":             fscp01InconclusiveDispatchFieldSource("DISPATCH_RECONCILIATION_NOT_EMITTED_BY_FIXTURE"),
	"failureClassification": fscp01InconclusiveDispatchFieldSource("DISPATCH_RECONCILIATION_NOT_EMITTED_BY_FIXTURE"),
	"failureDetail":         fscp01InconclusiveDispatchFieldSource("DISPATCH_RECONCILIATION_NOT_EMITTED_BY_FIXTURE"),
	"runnerId":              fscp01InconclusiveDispatchFieldSource("RUNNER_NOT_RETAINED_BY_ASSOCIATION_EVENT"),
	"presetId":              fscp01InconclusiveDispatchFieldSource("PRESET_NOT_RETAINED_BY_ASSOCIATION_EVENT"),
	"modelProvider":         fscp01InconclusiveDispatchFieldSource("MODEL_PROVIDER_NOT_RETAINED_BY_ASSOCIATION_EVENT"),
	"model":                 fscp01InconclusiveDispatchFieldSource("DISPATCH_SOURCE_NOT_DISTINGUISHABLE_BY_CURRENT_FIXTURE"),
	"reasoningEffort":       fscp01InconclusiveDispatchFieldSource("REASONING_EFFORT_NOT_RETAINED_BY_ASSOCIATION_EVENT"),
	"provider":              fscp01InconclusiveDispatchFieldSource("PROVIDER_NOT_RETAINED_BY_ASSOCIATION_EVENT"),
	"providerSessionRefs":   fscp01InconclusiveDispatchFieldSource("PROVIDER_SESSION_REFERENCE_NOT_EMITTED_BY_FIXTURE"),
	"usage":                 fscp01InconclusiveDispatchFieldSource("DISPATCH_RECONCILIATION_NOT_EMITTED_BY_FIXTURE"),
	"warnings":              fscp01InconclusiveDispatchFieldSource("DISPATCH_RECONCILIATION_NOT_EMITTED_BY_FIXTURE"),
	"outputArtifactIds":     fscp01InconclusiveDispatchFieldSource("DISPATCH_ARTIFACT_RECONCILIATION_NOT_EMITTED_BY_FIXTURE"),
	"artifactIds":           fscp01InconclusiveDispatchFieldSource("DISPATCH_ARTIFACT_RECONCILIATION_NOT_EMITTED_BY_FIXTURE"),
	"javascript":            fscp01InconclusiveDispatchFieldSource("JAVASCRIPT_PROJECTION_NOT_RETAINED_AS_CANONICAL_DISPATCH_FACT"),
	"petri":                 fscp01InconclusiveDispatchFieldSource("FIXTURE_IS_JAVASCRIPT_ONLY"),
	"sessionId":             fscp01InconclusiveDispatchFieldSource("CANONICAL_ASSOCIATION_SCOPE_IS_DEFAULT_ALIAS"),
	"orchestratorKind":      fscp01InconclusiveDispatchFieldSource("ORCHESTRATOR_KIND_NOT_RETAINED_BY_ASSOCIATION_EVENT"),
	"statusTransitions":     fscp01InconclusiveDispatchFieldSource(fscp01DispatchLifecycleNotRecorded),
}

func fscp01InconclusiveDispatchFieldSource(blocker string) fscp01DispatchFieldSource {
	return fscp01DispatchFieldSource{
		Active:   fscp01DispatchSourceInconclusive,
		Terminal: fscp01DispatchSourceInconclusive,
		Evidence: blocker,
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
// joins each selected dispatch to its Recordings-owned Worker Session
// association and records the current public inference-attempt evidence,
// including an explicit typed result when that event is not emitted.
func TestFSCP01DispatchReadFieldProvenanceMatrix(t *testing.T) {
	t.Parallel()
	t.Run("active", func(t *testing.T) {
		t.Parallel()
		acquireExecutionFixtureSlot(t)
		gate := make(chan struct{})
		var release sync.Once
		releaseGate := func() { release.Do(func() { close(gate) }) }
		runner := support.NewGatedSuccessCommandRunner("fscp01 active provider output", gate)
		locations := newFSCP01RunLocations(t)
		dir := support.ScaffoldFactory(t, map[string]any{"name": "fscp01-dispatch-active"})
		logFSCP01RunDeclaration(t, locations, dir, "", "single root; provider-gated active observation")
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			Env:                       locations.Env,
			Edges: serviceedges.Edges{
				ProviderCommandRunner: runner,
			},
		})
		logFSCP01BoundPort(t, server.URL())
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
		locations := newFSCP01RunLocations(t)
		dir := support.ScaffoldFactory(t, map[string]any{"name": "fscp01-dispatch-terminal"})
		logFSCP01RunDeclaration(t, locations, dir, "", "single root; terminal provider observation")
		runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
			Stdout: []byte("fscp01 terminal provider output"),
		})
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			Env:                       locations.Env,
			Edges: serviceedges.Edges{
				ProviderCommandRunner: runner,
			},
		})
		logFSCP01BoundPort(t, server.URL())
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

		assertFSCP01DispatchNegativeReads(t, server.URL(), started.SessionId, summary.Id)
	})
}

func assertFSCP01DispatchNegativeReads(t *testing.T, serverURL, sessionID, dispatchID string) {
	t.Helper()
	missing := readFSCP01DispatchError(t, strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/dispatches/fscp01-missing-dispatch")
	assertFSCP01DispatchNotFound(t, missing, "missing dispatch")
	foreign := readFSCP01DispatchError(t, strings.TrimSuffix(serverURL, "/")+"/factory-sessions/fscp01-foreign-session/dispatches/"+dispatchID)
	assertFSCP01DispatchNotFound(t, foreign, "foreign session dispatch")
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
		t.Logf("FSCP-01 dispatch identity: dispatch=%s attempt=%d workerSession=%s inferenceAttempts=[] source=%s blocker=%s evidence=canonical public stream emitted no inference request/response for this dispatch", detail.Id, *detail.Attempt, facts.WorkerSessionID, fscp01DispatchSourceInconclusive, fscp01InferenceEventNotEmitted)
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
		if source != fscp01DispatchSourcePass && source != fscp01DispatchSourceInconclusive {
			t.Fatalf("dispatch %q field %q source = %q, want PASS or INCONCLUSIVE", detail.Id, field, source)
		}
		if source == fscp01DispatchSourcePass {
			t.Fatalf("dispatch %q field %q has PASS without a current source-distinguishing witness", detail.Id, field)
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
