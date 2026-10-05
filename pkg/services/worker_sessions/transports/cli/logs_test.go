package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestReadLogsUsesSelectedHostAndFiniteCursor(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/worker-sessions/worker/logs" || r.URL.Query().Get("limit") != "1" || r.URL.Query().Get("nextToken") != "cursor" {
			t.Errorf("unexpected logs request: %s", r.URL.String())
		}
		if err := json.NewEncoder(w).Encode(factoryapi.WorkerSessionLogPage{
			WorkerSessionId: "worker", RecordingGenerationId: "generation", Health: "COMPLETE", Events: []factoryapi.WorkerSessionEvent{},
		}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	var output bytes.Buffer
	err := NewRead(testHTTPProtocol(t))(ReadConfig{Context: context.Background(), Server: server.URL, WorkerSessionID: "worker", View: "logs", Limit: 1, NextToken: "cursor", JSON: true, Output: &output})
	if err != nil {
		t.Fatal(err)
	}
	var result factoryapi.WorkerSessionLogPage
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.WorkerSessionId != "worker" || result.RecordingGenerationId != "generation" {
		t.Fatalf("lost identity: %+v", result)
	}
	server.Close()
	output.Reset()
	err = NewRead(testHTTPProtocol(t))(ReadConfig{Context: context.Background(), Server: server.URL, WorkerSessionID: "worker", View: "logs", JSON: true, Output: &output})
	if err == nil {
		t.Fatal("selected host failure became success")
	}
}
