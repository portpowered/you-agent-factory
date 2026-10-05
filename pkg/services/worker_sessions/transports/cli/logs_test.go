package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
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

func TestReadLogsFollowCommittedTailAndRingGap(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"terminal", "gap", "stale"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			var available atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/events") {
					if r.URL.Query().Get("after_position") != "1" || r.URL.Query().Get("stream_generation_id") != "" {
						t.Errorf("live handoff cursor: %s", r.URL)
					}
					available.Store(true)
					writeLogsFollowSource(t, w, source)
					return
				}
				page := followTestPage(1, 1, "1")
				switch r.URL.Query().Get("nextToken") {
				case "1":
					page.Events = nil
					if available.Load() {
						page = followTestPage(2, 3, "2")
					}
				case "2":
					page = followTestPage(3, 3, "")
				}
				if err := json.NewEncoder(w).Encode(page); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			var output bytes.Buffer
			err := NewRead(testHTTPProtocol(t))(ReadConfig{Context: ctx, Server: server.URL, WorkerSessionID: "worker", View: "logs", Follow: true, Limit: 1, JSON: true, Output: &output})
			if err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(&output)
			for position := int64(1); position <= 3; position++ {
				var event factoryapi.WorkerSessionEvent
				if err := decoder.Decode(&event); err != nil || event.Event.Position != position || event.WorkerSessionId != "worker" {
					t.Fatalf("ordered committed output at %d: %+v %v", position, event, err)
				}
			}
			if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
				t.Fatalf("follow duplicated a frame or emitted a page: %v", err)
			}
		})
	}
}

func followTestPage(position, head int64, token string) factoryapi.WorkerSessionLogPage {
	page := factoryapi.WorkerSessionLogPage{
		WorkerSessionId: "worker", RecordingGenerationId: "generation", CommittedPosition: head, Health: factoryapi.COMPLETE,
		Events: []factoryapi.WorkerSessionEvent{{WorkerSessionId: "worker", Delivery: "RECORD", Event: factoryapi.WorkerSessionEventRecord{Position: position}}},
	}
	if token != "" {
		page.NextToken = &token
	} else {
		page.Events[0].Delivery = "TERMINAL_REPLAY"
	}
	return page
}

func writeLogsFollowSource(t *testing.T, w http.ResponseWriter, source string) {
	t.Helper()
	if source == "stale" {
		w.WriteHeader(http.StatusConflict)
		_, err := io.WriteString(w, `{"code":"WORKER_SESSION_EVENT_CURSOR_STALE","message":"evicted"}`)
		if err != nil {
			t.Error(err)
		}
		return
	}
	frame := `{"delivery":"TERMINAL","workerSessionId":"worker","event":{"position":3}}`
	if source == "gap" {
		frame = `{"delivery":"SOURCE_FAILURE","errorCode":"WORKER_SESSION_STREAM_GAP"}`
	}
	w.Header().Set("Content-Type", "text/event-stream")
	if _, err := fmt.Fprintf(w, "data: %s\n\n", frame); err != nil {
		t.Error(err)
	}
}

func TestReadLogsFollowCancellationClosesLiveObserver(t *testing.T) {
	t.Parallel()
	started, closed := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/events") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			close(closed)
			return
		}
		page := followTestPage(1, 1, "1")
		if r.URL.Query().Get("nextToken") != "" {
			page.Events = nil
		}
		if err := json.NewEncoder(w).Encode(page); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	readOperation := NewRead(testHTTPProtocol(t))
	go func() {
		done <- readOperation(ReadConfig{Context: ctx, Server: server.URL, WorkerSessionID: "worker", View: "logs", Follow: true, Output: io.Discard})
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("live observer did not open")
	}
	cancel()
	select {
	case err := <-done:
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != "WORKER_SESSION_LOGS_INTERRUPTED" || !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled follow: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("follow did not join observer")
	}
	select {
	case <-closed:
	case <-time.After(30 * time.Second):
		t.Fatal("follow left live response open")
	}
}

func TestReadLogsFollowRejectsIncompleteAndNoncontiguousHistory(t *testing.T) {
	t.Parallel()
	for _, health := range []factoryapi.WorkerSessionLogPageHealth{factoryapi.DEGRADED, factoryapi.INCOMPLETE} {
		t.Run(string(health), func(t *testing.T) {
			t.Parallel()
			page := followTestPage(1, 1, "")
			page.Health = health
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if err := json.NewEncoder(w).Encode(page); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			var output bytes.Buffer
			err := NewRead(testHTTPProtocol(t))(ReadConfig{Context: t.Context(), Server: server.URL, WorkerSessionID: "worker", View: "logs", Follow: true, Output: &output})
			var cliErr *CLIError
			if !errors.As(err, &cliErr) || cliErr.Code != "WORKER_SESSION_LOGS_GAP" || output.Len() == 0 {
				t.Fatalf("incomplete prefix became complete or disappeared: %v %s", err, output.String())
			}
		})
	}
	for _, position := range []int64{1, 3} {
		t.Run(strconv.FormatInt(position, 10), func(t *testing.T) {
			t.Parallel()
			follower := logsFollower{config: ReadConfig{WorkerSessionID: "worker", Output: io.Discard}, position: 1}
			if err := follower.writePage(followTestPage(position, 3, "")); err == nil {
				t.Fatal("duplicate or missing position accepted")
			}
		})
	}
}

func TestReadLogsFollowRejectsWrongIdentityAndGeneration(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"worker", "generation"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			follower := logsFollower{config: ReadConfig{WorkerSessionID: "worker", Output: &output}, generation: "generation"}
			page := followTestPage(1, 1, "")
			if field == "worker" {
				page.WorkerSessionId = "other-worker"
			} else {
				page.RecordingGenerationId = "other-generation"
			}
			if err := follower.writePage(page); err == nil || output.Len() != 0 {
				t.Fatalf("foreign capture emitted bytes: %v %s", err, output.String())
			}
		})
	}
}

func TestReadLogsFollowRequiresLogsAndExcludesArtifacts(t *testing.T) {
	t.Parallel()
	for _, config := range []ReadConfig{{Follow: true}, {Follow: true, View: "logs", ArtifactRef: "worker/1"}} {
		config.Context, config.Output, config.WorkerSessionID = t.Context(), io.Discard, "worker"
		if err := validateReadConfig(config); err == nil {
			t.Fatal("invalid follow mode accepted")
		}
	}
}

func TestReadLogsFollowRetainedPrefixWithoutLiveOwner(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/events") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":"WORKER_SESSION_NOT_FOUND","message":"no live owner"}`)
			return
		}
		page := followTestPage(1, 1, "1")
		page.Health = factoryapi.INCOMPLETE
		if r.URL.Query().Get("nextToken") != "" {
			page.Events = nil
		}
		if err := json.NewEncoder(w).Encode(page); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var output bytes.Buffer
	err := NewRead(testHTTPProtocol(t))(ReadConfig{Context: ctx, Server: server.URL, WorkerSessionID: "worker", View: "logs", Follow: true, Output: &output})
	var failure *CLIError
	if !errors.As(err, &failure) || failure.Code != "WORKER_SESSION_LOGS_GAP" || ctx.Err() != nil {
		t.Fatalf("retained incomplete prefix lost its meaning: %v", err)
	}
	var event factoryapi.WorkerSessionEvent
	decoder := json.NewDecoder(&output)
	if err := decoder.Decode(&event); err != nil || event.Event.Position != 1 {
		t.Fatalf("retained prefix lost: %+v %v", event, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		t.Fatalf("prefix duplicated or error emitted as an event: %v", err)
	}
}

func TestReadLogsFollowResumesEmptyLiveHead(t *testing.T) {
	t.Parallel()
	var committed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/events") {
			if r.URL.Query().Has("after_position") {
				t.Errorf("empty durable head invented Events position: %s", r.URL)
			}
			committed.Store(true)
			writeLogsFollowSource(t, w, "terminal")
			return
		}
		page := followTestPage(1, 1, "1")
		page.Events = nil
		page.Health = factoryapi.INCOMPLETE
		if committed.Load() {
			page = followTestPage(2, 2, "")
		}
		if err := json.NewEncoder(w).Encode(page); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var output bytes.Buffer
	err := NewRead(testHTTPProtocol(t))(ReadConfig{Context: ctx, Server: server.URL, WorkerSessionID: "worker", View: "logs", Follow: true, NextToken: "1", Output: &output})
	if err != nil {
		t.Fatal(err)
	}
	var event factoryapi.WorkerSessionEvent
	decoder := json.NewDecoder(&output)
	if err := decoder.Decode(&event); err != nil || event.Event.Position != 2 {
		t.Fatalf("resume lost its committed tail: %+v %v", event, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		t.Fatalf("resume repeated its acknowledged prefix: %v", err)
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
