package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Provider completion races the prefix/live handoff when the first durable
// frame reaches stdout. The observer must drain the real asynchronous capture
// without losing or duplicating any event. Only the provider effect is gated.
type capturedFollowOutput struct {
	bytes.Buffer
	release func()
	once    sync.Once
}

func (w *capturedFollowOutput) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	w.once.Do(w.release)
	return n, err
}

func readCapturedLiveFollow(t *testing.T, server *support.FunctionalAPIServer, id string, finish chan struct{}) []factoryapi.WorkerSessionEvent {
	t.Helper()
	assertCapturedFollowDetach(t, server, id)
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--follow", "--limit", "1", "--server", server.URL(), "--output", "json"})
	output := &capturedFollowOutput{release: func() { close(finish) }}
	inputs.Input.Stdout = output
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("live committed follow: %v %s", err, inputs.Stderr())
	}
	return decodeCapturedFollow(t, output.String())
}

func assertCapturedFollowDetach(t *testing.T, server *support.FunctionalAPIServer, id string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--follow", "--server", server.URL(), "--output", "json"})
	output := &capturedFollowOutput{release: cancel}
	inputs.Input.Stdout = output
	err := server.Execute(t, inputs.Input)
	if !errors.Is(err, context.Canceled) || len(decodeCapturedFollow(t, output.String())) == 0 {
		t.Fatalf("detached follow lost its captured prefix or cancellation: %v %s", err, output.String())
	}
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+id)
	if shown.State != factoryapi.WorkerSessionObservationStateRunning {
		t.Fatalf("detached observer stopped Worker: %+v", shown)
	}
}

func decodeCapturedFollow(t *testing.T, text string) []factoryapi.WorkerSessionEvent {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewBufferString(text))
	var events []factoryapi.WorkerSessionEvent
	for {
		var event factoryapi.WorkerSessionEvent
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			return events
		} else if err != nil {
			t.Fatalf("follow NDJSON: %v %s", err, text)
		}
		events = append(events, event)
	}
}

func assertCapturedFollowPages(t *testing.T, server *support.FunctionalAPIServer, full factoryapi.WorkerSessionLogPage, followed []factoryapi.WorkerSessionEvent, resume string) {
	t.Helper()
	if !reflect.DeepEqual(full.Events, followed) {
		t.Fatalf("live follow changed committed events: follow=%+v page=%+v", followed, full.Events)
	}
	for _, token := range []string{"", resume} {
		page := readCapturedContinuation(t, server, full.WorkerSessionId, token)
		ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
		inputs := support.FakeInputs(ctx, []string{"you", "worker-sessions", "read", "--worker-session-id", full.WorkerSessionId, "--view", "logs", "--follow", "--limit", "1", "--next-token", token, "--server", server.URL(), "--output", "json"})
		err := server.Execute(t, inputs.Input)
		cancel()
		if err != nil || !reflect.DeepEqual(decodeCapturedFollow(t, inputs.Stdout()), page.Events) {
			t.Fatalf("acknowledged follow changed committed tail: %v %s", err, inputs.Stdout())
		}
	}
}
