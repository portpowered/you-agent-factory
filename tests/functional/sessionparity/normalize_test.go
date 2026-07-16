package sessionparity

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizers_MapRepresentativeRealCustomerShapes(t *testing.T) {
	rest, cli, mcp := representativeObservationBundles(t)
	want, err := NormalizeREST(rest)
	if err != nil {
		t.Fatalf("NormalizeREST: %v", err)
	}
	for _, test := range []struct {
		name      string
		normalize func([]byte) (Projection, error)
		input     []byte
	}{{"CLI JSON", NormalizeCLIJSON, cli}, {"MCP", NormalizeMCP, mcp}} {
		t.Run(test.name, func(t *testing.T) {
			got, normalizeErr := test.normalize(test.input)
			if normalizeErr != nil {
				t.Fatalf("normalize: %v", normalizeErr)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("projection mismatch\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
	if got, wantID := want.Results[0].ID, "dur-sess-parity-001:result"; got != wantID {
		t.Fatalf("derived result correlation id = %q, want %q", got, wantID)
	}
}

func TestNormalizers_RejectEveryMissingRequiredScalarFact(t *testing.T) {
	rest, _, _ := representativeObservationBundles(t)
	for _, field := range []string{
		"identity.sessionId", "lifecycle.status", "hashes.sourceHash",
		"progress.totalDispatches", "progress.completedDispatches",
		"progress.failedDispatches", "progress.inFlightDispatches",
	} {
		t.Run(field, func(t *testing.T) {
			mutated := removeRequiredScalar(t, rest, field)
			_, err := NormalizeREST(mutated)
			want := &NormalizationError{Interface: "REST", Field: field, Reason: "is required"}
			if !reflect.DeepEqual(err, want) {
				t.Fatalf("NormalizeREST error = %#v, want %#v", err, want)
			}
		})
	}
}

func TestNormalizeMCP_RejectsReorderedCanonicalEventCursors(t *testing.T) {
	_, _, mcp := representativeObservationBundles(t)
	var bundle map[string]any
	if err := json.Unmarshal(mcp, &bundle); err != nil {
		t.Fatalf("unmarshal MCP bundle: %v", err)
	}
	eventsResponse := bundle["events"].(map[string]any)
	eventsResult := eventsResponse["result"].(map[string]any)
	events := eventsResult["events"].([]any)
	events[0], events[1] = events[1], events[0]
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal MCP bundle: %v", err)
	}
	_, err = NormalizeMCP(encoded)
	want := &NormalizationError{Interface: "MCP", Field: "eventCursors[1]", Reason: "must retain a correlated, strictly ordered event cursor"}
	if !reflect.DeepEqual(err, want) {
		t.Fatalf("NormalizeMCP error = %#v, want %#v", err, want)
	}
}

func TestNormalizeCLIJSON_PreservesCustomerVisibleCollectionOrder(t *testing.T) {
	_, cli, _ := representativeObservationBundles(t)
	projection, err := NormalizeCLIJSON(cli)
	if err != nil {
		t.Fatalf("NormalizeCLIJSON: %v", err)
	}
	if got, want := projection.Dispatches[0].ID, "dispatch-2"; got != want {
		t.Fatalf("first dispatch = %q, want %q", got, want)
	}
	if got, want := projection.EventCursors[0].Sequence, int64(11); got != want {
		t.Fatalf("first event sequence = %d, want %d", got, want)
	}
}

func TestNormalizers_PreserveDistinctLargeIntegerResultValues(t *testing.T) {
	rest, cli, mcp := representativeObservationBundles(t)
	for _, test := range []struct {
		name      string
		normalize func([]byte) (Projection, error)
		input     []byte
	}{
		{"REST", NormalizeREST, rest},
		{"CLI JSON", NormalizeCLIJSON, cli},
		{"MCP", NormalizeMCP, mcp},
	} {
		t.Run(test.name, func(t *testing.T) {
			firstInput := replacePrimaryResult(t, test.input, "9007199254740992")
			secondInput := replacePrimaryResult(t, test.input, "9007199254740993")
			first := normalizeObservation(t, test.normalize, firstInput)
			second := normalizeObservation(t, test.normalize, secondInput)

			if got, want := first.Results[0].Value, "[9007199254740992]"; got != want {
				t.Fatalf("first normalized result = %q, want %q", got, want)
			}
			if got, want := second.Results[0].Value, "[9007199254740993]"; got != want {
				t.Fatalf("second normalized result = %q, want %q", got, want)
			}
			if differences := Compare(first, second); !containsDifference(differences, "results[0].value") {
				t.Fatalf("large integer result change was not reported: %#v", differences)
			}
		})
	}
}

func replacePrimaryResult(t *testing.T, observation []byte, number string) []byte {
	t.Helper()
	old := []byte(`"primaryResult":[{"type":"text","text":"done"}]`)
	if !bytes.Contains(observation, old) {
		old = []byte(`"primaryResult":[{"text":"done","type":"text"}]`)
	}
	updated := bytes.Replace(observation, old, []byte(`"primaryResult":[`+number+`]`), 1)
	if bytes.Equal(updated, observation) {
		t.Fatal("representative observation did not contain the expected primary result")
	}
	return updated
}

func representativeObservationBundles(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	session := json.RawMessage(`{
		"sessionId":"dur-sess-parity-001","status":"SUCCEEDED","phase":"completed",
		"sourceHash":"sha256:source","resolvedSource":{"kind":"WORKFLOW_NAME","sourceHash":"sha256:source"},
		"requestedPolicy":{"policyHash":"sha256:requested"},
		"effectivePolicyHash":"sha256:effective",
		"progress":{"totalDispatches":2,"completedDispatches":2,"failedDispatches":0,"inFlightDispatches":0}
	}`)
	dispatches := json.RawMessage(`{"sessionId":"dur-sess-parity-001","dispatches":[
		{"id":"dispatch-2","status":"COMPLETED","dispatchKind":"JAVASCRIPT_TASK"},
		{"id":"dispatch-1","status":"COMPLETED","dispatchKind":"JAVASCRIPT_TASK"}
	]}`)
	artifacts := json.RawMessage(`{"sessionId":"dur-sess-parity-001","artifacts":[
		{"id":"artifact-1","kind":"FINAL_RESULT","visibility":"PUBLIC"}
	]}`)
	result := json.RawMessage(`{"sessionId":"dur-sess-parity-001","resultStatus":"FINAL","sessionStatus":"SUCCEEDED",
		"primaryResult":[{"type":"text","text":"done"}]}`)
	cliResult := json.RawMessage(`{"resultStatus":"FINAL","sessionId":"dur-sess-parity-001","sessionStatus":"SUCCEEDED",
		"primaryResult":[{"text":"done","type":"text"}]}`)
	events := json.RawMessage(`[
		{"id":"cursor-11","type":"DISPATCH_RESPONSE","schemaVersion":"agent-factory.event.v1","context":{"sequence":11,"sessionId":"dur-sess-parity-001","tick":1,"eventTime":"2026-07-16T00:00:00Z"},"payload":{}},
		{"id":"cursor-12","type":"SESSION_RESULT_UPDATED","schemaVersion":"agent-factory.event.v1","context":{"sequence":12,"sessionId":"dur-sess-parity-001","tick":2,"eventTime":"2026-07-16T00:00:01Z"},"payload":{}}
	]`)
	rest := marshalBundle(t, session, dispatches, artifacts, result, events, false)
	cli := marshalBundle(t, session, dispatches, artifacts, cliResult, events, false)
	mcpEvents := mustMarshal(t, map[string]any{"sessionId": "dur-sess-parity-001", "events": json.RawMessage(events)})
	mcp := marshalBundle(t, session, dispatches, artifacts, result, mcpEvents, true)
	return rest, cli, mcp
}

func marshalBundle(t *testing.T, session, dispatches, artifacts, result, events json.RawMessage, mcp bool) []byte {
	t.Helper()
	values := map[string]json.RawMessage{
		"session": session, "dispatches": dispatches, "artifacts": artifacts, "result": result, "events": events,
	}
	if mcp {
		for field, value := range values {
			values[field] = mustMarshal(t, map[string]any{"jsonrpc": "2.0", "id": "request-" + field, "result": value})
		}
	}
	return mustMarshal(t, values)
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal observation: %v", err)
	}
	return encoded
}

func removeRequiredScalar(t *testing.T, observation []byte, field string) []byte {
	t.Helper()
	var bundle map[string]any
	if err := json.Unmarshal(observation, &bundle); err != nil {
		t.Fatalf("unmarshal observation: %v", err)
	}
	session := bundle["session"].(map[string]any)
	switch field {
	case "identity.sessionId":
		delete(session, "sessionId")
	case "lifecycle.status":
		delete(session, "status")
	case "hashes.sourceHash":
		delete(session, "sourceHash")
		delete(session["resolvedSource"].(map[string]any), "sourceHash")
	default:
		progress := session["progress"].(map[string]any)
		paths := map[string]string{
			"progress.totalDispatches": "totalDispatches", "progress.completedDispatches": "completedDispatches",
			"progress.failedDispatches": "failedDispatches", "progress.inFlightDispatches": "inFlightDispatches",
		}
		delete(progress, paths[field])
	}
	return mustMarshal(t, bundle)
}
