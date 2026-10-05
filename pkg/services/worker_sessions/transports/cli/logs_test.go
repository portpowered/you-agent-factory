package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != "FACTORY_UNREACHABLE" {
		t.Fatalf("selected host failure classification: %v", err)
	}
}

func TestReadLogsCancellationIsInterrupted(t *testing.T) {
	t.Parallel()
	for _, ref := range []string{"", "worker/2"} {
		t.Run(ref, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			var output bytes.Buffer
			err := NewRead(testHTTPProtocol(t))(ReadConfig{
				Context: ctx, Server: "http://127.0.0.1:1", WorkerSessionID: "worker",
				View: "logs", ArtifactRef: ref, JSON: true, Output: &output,
			})
			var cliErr *CLIError
			if !errors.As(err, &cliErr) || cliErr.Code != "WORKER_SESSION_LOGS_INTERRUPTED" || !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled logs read: %v", err)
			}
		})
	}
}

type cancelLogsPayloadWriter struct {
	buffer bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelLogsPayloadWriter) Write(data []byte) (int, error) {
	n, err := w.buffer.Write(data)
	w.cancel()
	return n, err
}

func TestReadLogsArtifactCancellationDuringTransfer(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		if _, err := w.Write([]byte("captured-prefix")); err != nil {
			t.Error(err)
			return
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	// Only a diagnostic ceiling; cancellation is synchronized by the writer.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	output := &cancelLogsPayloadWriter{cancel: cancel}
	err := NewRead(testHTTPProtocol(t))(ReadConfig{
		Context: ctx, Server: server.URL, WorkerSessionID: "worker", View: "logs",
		ArtifactRef: "worker/2", JSON: true, Output: output,
	})
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != "WORKER_SESSION_LOGS_INTERRUPTED" || !errors.Is(err, context.Canceled) {
		t.Fatalf("payload transfer cancellation: %v", err)
	}
	if output.buffer.String() != "captured-prefix" {
		t.Fatalf("interruption changed raw payload output: %q", output.buffer.String())
	}
}
