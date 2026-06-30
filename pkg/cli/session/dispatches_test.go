package session

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestDispatches_DurableSessionJSONUsesListFactorySessionDispatchesResponse(t *testing.T) {
	label := "step-one"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/factory-sessions/dur-sess-js-interrupted-001/dispatches"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionDispatchesResponse{
			SessionId: "dur-sess-js-interrupted-001",
			Dispatches: []factoryapi.FactorySessionDispatchSummary{
				{
					Id:           "dispatch-1",
					Status:       factoryapi.FactoryDispatchStatusCOMPLETED,
					DispatchKind: factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
					Label:        &label,
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := Dispatches(DispatchesConfig{
		Server:    srv.URL,
		SessionID: "dur-sess-js-interrupted-001",
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Dispatches durable JSON: %v", err)
	}

	var listed factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatalf("decode dispatches JSON: %v", err)
	}
	if listed.SessionId != "dur-sess-js-interrupted-001" {
		t.Fatalf("sessionId = %q", listed.SessionId)
	}
	if len(listed.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one dispatch", listed.Dispatches)
	}
}

func TestDispatches_DurableSessionHumanOutputRendersDispatchSummaries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionDispatchesResponse{
			SessionId: "dur-sess-js-interrupted-001",
			Dispatches: []factoryapi.FactorySessionDispatchSummary{
				{
					Id:           "dispatch-1",
					Status:       factoryapi.FactoryDispatchStatusCOMPLETED,
					DispatchKind: factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
				},
				{
					Id:           "dispatch-2",
					Status:       factoryapi.FactoryDispatchStatusINTERRUPTED,
					DispatchKind: factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := Dispatches(DispatchesConfig{
		Server:    srv.URL,
		SessionID: "dur-sess-js-interrupted-001",
		Output:    &out,
	}); err != nil {
		t.Fatalf("Dispatches durable human: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Factory session dur-sess-js-interrupted-001 dispatches (2):",
		"- dispatch-1 COMPLETED JAVASCRIPT_AGENT",
		"- dispatch-2 INTERRUPTED JAVASCRIPT_AGENT",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestDispatches_RejectsNonDurableSessionID(t *testing.T) {
	err := Dispatches(DispatchesConfig{
		Server:    "http://127.0.0.1:1",
		SessionID: "session-beta",
		Output:    ioDiscard{},
	})
	if err == nil {
		t.Fatal("expected error for non-durable session id")
	}
	if !strings.Contains(err.Error(), "dur-sess-*") {
		t.Fatalf("error = %q, want durable session requirement", err.Error())
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
