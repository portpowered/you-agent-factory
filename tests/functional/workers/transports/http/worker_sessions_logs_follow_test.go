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
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Factory notifications use Factory Event coordinates, unlike the dedicated
// Worker capture. A real explicit-session attempt proves that following its
// durable prefix never substitutes a capture position into that stream.
func TestWorkerSessionCapturedLogsFactoryFollow(t *testing.T) {
	t.Parallel()
	finish := make(chan struct{})
	runner := newFunctionalWorkerGate(finish)
	dir := support.ScaffoldSingleStepFactory(t, "captured-factory-follow")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
	})
	opened := support.OpenFactorySessionAt(t, server.URL(), dir)
	submitted := support.SubmitSessionWorkAt(t, server.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{
		WorkTypeName: "task", Payload: map[string]string{"title": "Factory durable follow"},
	})
	runner.waitStarted(t)
	list := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, workerSessionsListURL(server.URL(), opened.Session.Id, *submitted.WorkId))
	if len(list.Sessions) != 1 {
		t.Fatalf("Factory follow target count=%d", len(list.Sessions))
	}
	id := list.Sessions[0].WorkerSessionId
	followed := readCapturedLiveFollow(t, server, id, finish)
	ended := waitCapturedTerminal(t, server.URL(), id)
	if !reflect.DeepEqual(followed, ended.Events) {
		t.Fatal("Factory follow lost or duplicated committed records")
	}
}

// Provider completion races the prefix/live handoff when the first durable
// frame reaches stdout. The observer must drain the real asynchronous capture
// without losing or duplicating any event. Only the provider effect is gated.
type capturedFollowOutput struct {
	bytes.Buffer
	release func()
	once    sync.Once
}

func assertCapturedFollowFailure(t *testing.T, server *support.FunctionalAPIServer, id, token, code string, prefix []factoryapi.WorkerSessionEvent) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--follow", "--next-token", token, "--server", server.URL(), "--output", "json"})
	err := server.Execute(t, inputs.Input)
	var failure interface{ CLIErrorCode() string }
	if !errors.As(err, &failure) || failure.CLIErrorCode() != code || ctx.Err() != nil {
		t.Fatalf("follow failure: want %s, got %v; diagnostics=%s", code, err, inputs.Stderr())
	}
	got := decodeCapturedFollow(t, inputs.Stdout())
	if len(got) != len(prefix) || (len(got) > 0 && !reflect.DeepEqual(got, prefix)) {
		t.Fatalf("failed follow changed committed prefix: got=%+v want=%+v", got, prefix)
	}
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
