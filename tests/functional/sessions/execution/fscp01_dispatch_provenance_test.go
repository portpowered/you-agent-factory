package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	fscp01DispatchSourceRecording = "Recording projection"
	fscp01DispatchSourceDurable   = "durable execution state"
	fscp01DispatchSourceRuntime   = "Runtime (transient) state"
	fscp01DispatchSourceUnproven  = "UNPROVEN"
)

type fscp01DispatchFieldSource struct {
	Active   string
	Terminal string
	Evidence string
}

// This is deliberately a field-level observation table rather than an
// ownership assertion. Fields without a direct public witness remain
// UNPROVEN until a later convergence lane supplies one.
var fscp01DispatchFieldSources = map[string]fscp01DispatchFieldSource{
	"id":                    {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "list/detail identity and canonical dispatch context"},
	"status":                {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "active/terminal list and detail status"},
	"confirmationState":     {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public read state is stable at each lifecycle boundary"},
	"dispatchKind":          {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "list/detail parity and canonical dispatch lifecycle metadata"},
	"phase":                 {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public list/detail projection"},
	"label":                 {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public list/detail projection"},
	"attempt":               {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceDurable, Evidence: "public dispatch attempt joined to canonical dispatch identity"},
	"retryable":             {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public dispatch projection when present"},
	"failureClassification": {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public dispatch projection when present"},
	"failureDetail":         {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public dispatch projection when present"},
	"runnerId":              {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceUnproven, Evidence: "selection origin is not independently exposed by this witness"},
	"presetId":              {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceUnproven, Evidence: "selection origin is not independently exposed by this witness"},
	"modelProvider":         {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceDurable, Evidence: "public resolved execution projection when present"},
	"model":                 {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceDurable, Evidence: "public resolved execution projection when present"},
	"reasoningEffort":       {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceUnproven, Evidence: "provider selection origin is not independently exposed by this witness"},
	"provider":              {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceDurable, Evidence: "public resolved execution projection when present"},
	"providerSessionRefs":   {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceDurable, Evidence: "public provider-session correlation projection when present"},
	"usage":                 {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public usage projection when present"},
	"warnings":              {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public warning projection when present"},
	"outputArtifactIds":     {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public artifact lineage projection when present"},
	"artifactIds":           {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public artifact lineage projection when present"},
	"javascript":            {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceDurable, Evidence: "public orchestrator projection when present"},
	"petri":                 {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceUnproven, Evidence: "not emitted for the JavaScript fixture"},
	"sessionId":             {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceDurable, Evidence: "scoped detail identity"},
	"orchestratorKind":      {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceDurable, Evidence: "scoped detail execution identity"},
	"statusTransitions":     {Active: fscp01DispatchSourceRuntime, Terminal: fscp01DispatchSourceRecording, Evidence: "public lifecycle projection when present"},
}

// TestFSCP01DispatchReadFieldProvenanceMatrix proves active and terminal
// public dispatch list/detail reads against real root-built sessions and emits
// an explicit provenance label for every returned JSON field. It also joins
// each selected dispatch to its canonical Worker Session association and
// provider-attempt observation where the fixture emits one.
func TestFSCP01DispatchReadFieldProvenanceMatrix(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	t.Run("active", func(t *testing.T) {
		dir := scaffoldPartialResultFactory(t)
		provider := newPartialResultBlockingProvider(partialResultWorkflowName)
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			Edges:                     serviceedges.Edges{ProviderOverride: provider},
		})
		t.Cleanup(func() { server.Stop(t) })

		started := startPartialResultAsync(t, server.URL())
		if strings.TrimSpace(started.SessionId) == "" {
			t.Fatal("active session id is empty")
		}
		t.Cleanup(func() {
			releaseBlockedPartialResultSession(t, server.URL(), provider, started.SessionId)
		})

		waitForDurableSessionStatus(t, server.URL(), started.SessionId, factoryapi.FactorySessionDurableLifecycleStatusRunning, 5*time.Second)
		waitForFactoryDispatchStatus(t, server.URL(), started.SessionId, partialResultFirstDispatchID, factoryapi.FactoryDispatchStatusCOMPLETED, 5*time.Second)
		waitForFactoryDispatchStatus(t, server.URL(), started.SessionId, partialResultSecondDispatchID, factoryapi.FactoryDispatchStatusRUNNING, 5*time.Second)

		listed := listFactorySessionDispatches(t, server.URL(), started.SessionId)
		if listed.SessionId != started.SessionId {
			t.Fatalf("active dispatch list sessionId = %q, want %q", listed.SessionId, started.SessionId)
		}
		summary := requireFSCP01DispatchSummary(t, listed, partialResultSecondDispatchID)
		if summary.Status != factoryapi.FactoryDispatchStatusRUNNING {
			t.Fatalf("active dispatch summary status = %q, want RUNNING", summary.Status)
		}
		detail := getFactorySessionDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertFSCP01DispatchListDetail(t, started.SessionId, summary, detail)
		facts := observeFSCP01CanonicalDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertFSCP01DispatchAttemptAndWorkerIdentity(t, detail, facts)
		recordFSCP01DispatchFieldSources(t, "active", summary, detail)
	})

	t.Run("terminal", func(t *testing.T) {
		dir := scaffoldDispatchCorrelationFactory(t)
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir:                dir,
			WaitForServiceModeRuntime: true,
			Edges:                     serviceedges.Edges{ProviderOverride: fscp01CodexTerminalProvider()},
		})
		t.Cleanup(func() { server.Stop(t) })

		started := startDispatchCorrelationSync(t, server.URL(), dir)
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
		detail := getFactorySessionDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertFSCP01DispatchListDetail(t, started.SessionId, summary, detail)
		facts := observeFSCP01CanonicalDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertFSCP01DispatchAttemptAndWorkerIdentity(t, detail, facts)
		recordFSCP01DispatchFieldSources(t, "terminal", summary, detail)

		// A second terminal list/detail read is the public stability check for
		// fields classified as Recording projection or durable execution state.
		secondSummary := requireFSCP01DispatchSummary(t, listFactorySessionDispatches(t, server.URL(), started.SessionId), summary.Id)
		secondDetail := getFactorySessionDispatch(t, server.URL(), started.SessionId, summary.Id)
		assertFSCP01DispatchListDetail(t, started.SessionId, secondSummary, secondDetail)

		missing := readFSCP01DispatchError(t, strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches/fscp01-missing-dispatch")
		assertFSCP01DispatchNotFound(t, missing, "missing dispatch")
		foreign := readFSCP01DispatchError(t, strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/fscp01-foreign-session/dispatches/"+summary.Id)
		assertFSCP01DispatchNotFound(t, foreign, "foreign session dispatch")
	})
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
	if !facts.HasAssociation {
		t.Fatalf("public canonical/Worker Session history for dispatch %q has no Worker Session association", dispatchID)
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

func fscp01CodexTerminalProvider() testutil.NativeProvider {
	provider := testutil.NativeProvider{}
	provider.ExecuteFunc = func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
		return providers.ExecuteResult{
			Content: `{"text":"fscp01 terminal provider output","label":"dispatch-correlation-child"}`,
			SessionRef: &providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
				ID:       "fscp01-codex-provider-session",
			},
		}, nil
	}
	return provider
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
	if len(facts.Attempts) > 0 {
		if _, ok := facts.Attempts[int(*detail.Attempt)]; !ok {
			t.Fatalf("dispatch %q detail attempt = %d, canonical inference attempts = %#v", detail.Id, *detail.Attempt, facts.Attempts)
		}
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
	fields := marshalFSCP01JSONFields(t, summary)
	for field := range marshalFSCP01JSONFields(t, detail) {
		fields[field] = nil
	}
	fieldNames := make([]string, 0, len(fields))
	for field := range fields {
		fieldNames = append(fieldNames, field)
	}
	sort.Strings(fieldNames)
	for _, field := range fieldNames {
		entry := fscp01DispatchFieldSources[field]
		source := entry.Terminal
		if phase == "active" {
			source = entry.Active
		}
		if source == "" {
			source = fscp01DispatchSourceUnproven
		}
		t.Logf("FSCP-01 dispatch provenance phase=%s dispatch=%s field=%s source=%s evidence=%s", phase, detail.Id, field, source, entry.Evidence)
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
