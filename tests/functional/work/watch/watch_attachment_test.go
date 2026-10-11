package watch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestJoinedSessionCancelRecordedReadbackAndFlushFault(t *testing.T) {
	ensureWatchFixture(t)
	for _, fault := range []bool{false, true} {
		t.Run(fmt.Sprintf("recording flush fault=%t", fault), func(t *testing.T) {
			t.Parallel()
			session := uuid.NewString()
			config := observationFactoryConfig(true, "complete")
			config["workTypes"].([]map[string]any)[0]["handlingBehavior"] = []string{"DEFAULT"}
			dir := support.ScaffoldFactory(t, config)
			command := newHeldObservationCommand(t)
			command.canceled, command.join = make(chan struct{}), make(chan struct{})
			observationCommands.routes.Store(filepath.Clean(dir), command)
			t.Cleanup(func() { observationCommands.routes.Delete(filepath.Clean(dir)) })
			selectedPath := filepath.Join(dir, "canceled.__factory_session_id__.json")
			path := strings.ReplaceAll(selectedPath, "__factory_session_id__", session)
			host, running := startWatchHost(t, observationWatchProcess, dir, "--session", session, "--record", selectedPath)
			selected := attachLiveObservation(t, host, session, command, false)
			peer := newLiveObservation(t, host, true, false, "complete")
			primary := startObservationInvocation(t, selected)
			selected.awaitCommand(t)
			var joined sync.Once
			releaseJoin := func() { joined.Do(func() { close(command.join) }) }
			t.Cleanup(releaseJoin)
			cause := &fs.PathError{Op: "publish canceled recording", Path: path, Err: fs.ErrPermission}
			if fault {
				observationRecordingFailures.Store(filepath.Clean(path), cause)
				t.Cleanup(func() { observationRecordingFailures.Delete(filepath.Clean(path)) })
			}
			cancelJoinedObservation(t, selected, releaseJoin)
			outcome := awaitObservationInvocation(t, primary)
			if outcome.Status != factoryapi.InvocationTerminalStatusFailed || outcome.ErrorCode == nil || *outcome.ErrorCode != "INVOCATION_INTERRUPTED" || outcome.PrimaryResult != nil {
				t.Fatalf("canceled recorded invocation=%+v", outcome)
			}
			peerWork := peer.submit(t, "peer-after-recording-cancel")
			peer.awaitCommand(t)
			close(peer.command.release)
			peer.finish(t)
			assertWorkWatchTransitionLines(t, decodeWatchLines(t, peer.out.String()), peer.session, peerWork, [][2]string{{"init", "complete"}})
			selected.cancel()
			selected.watch.AcceptError()
			host.execute(t, "server", "stop")
			select {
			case <-running.Done():
			case <-time.After(selectedWatchCeiling):
				t.Fatal("recorded host did not join after public stop")
			}
			if fault {
				if !errors.Is(running.Err(), cause) {
					t.Fatalf("recording flush lost original cause: %v", running.Err())
				}
				running.AcceptError()
				return // A failed flush is never evidence of durable cancellation.
			}
			if running.Err() != nil {
				t.Fatalf("recorded host shutdown=%v", running.Err())
			}
			assertCanceledRecordingReadback(t, session, path)
		})
	}
}

func assertCanceledRecordingReadback(t *testing.T, session, path string) {
	t.Helper()
	// The public historical query reconstructs only the chosen artifact. It
	// cannot borrow the live ledger or dispatch the retained retryable Work.
	history, err := observationRecordings.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: recordings.RecordingID(session),
			Artifact: recordings.RecordingArtifactReference(path), Scope: recordings.CanonicalEventScope{FactorySessionID: session}},
	})
	if err != nil {
		t.Fatalf("public canceled recording readback: %v", err)
	}
	responses := 0
	for index, event := range history.Events {
		if index > 0 && event.Sequence <= history.Events[index-1].Sequence {
			t.Fatal("canceled recording lost canonical order")
		}
		if event.Kind == "WORK_STATE_CHANGE" {
			t.Fatalf("canceled primary recording routed unfinished Work: %+v", event)
		}
		if event.Kind != "DISPATCH_RESPONSE" {
			continue
		}
		responses++
		var payload factoryapi.DispatchResponseEventPayload
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil || payload.Outcome != factoryapi.WorkOutcomeCanceled || payload.Cancellation == nil || payload.StructuredResult != nil {
			t.Fatalf("recorded interruption=%+v error=%v", payload, err)
		}
	}
	if responses != 1 || history.Status.State != recordings.RecordingFinalized {
		t.Fatalf("recorded canceled responses=%d status=%+v", responses, history.Status)
	}
}

// Only the loopback connection is substituted. All responses and retained
// cursors are emitted by the real Factory Session handlers and recording ledger.
const workWatchAttachmentTimeout = 30 * time.Second
const workWatchEventPath = "/factory-sessions/~default/events"

type selectedWatchDisconnectGate struct {
	server      *httptest.Server
	command     *support.ProcessCommand
	diagnostics *ledgerOutput
	requests    chan url.Values
	attached    chan struct{}
	count       atomic.Int64
	mu          sync.Mutex
	body        *selectedWatchBody
}

func newSelectedWatchDisconnectGate(t *testing.T, endpoint, session string) *selectedWatchDisconnectGate {
	t.Helper()
	target, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	gate := &selectedWatchDisconnectGate{requests: make(chan url.Values, 8), attached: make(chan struct{}, 8)}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(response *http.Response) error {
		if response.Request.URL.Path == "/factory-sessions/"+session+"/events" && response.StatusCode == http.StatusOK {
			body := &selectedWatchBody{ReadCloser: response.Body}
			gate.mu.Lock()
			gate.body = body
			gate.mu.Unlock()
			response.Body = body
			gate.attached <- struct{}{}
		}
		return nil
	}
	gate.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/factory-sessions/"+session+"/events" {
			gate.count.Add(1)
			gate.requests <- r.URL.Query()
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(gate.server.Close)
	return gate
}

type selectedWatchBody struct {
	io.ReadCloser
	disconnected atomic.Bool
}

func (b *selectedWatchBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	if b.disconnected.Load() {
		return n, io.EOF
	}
	return n, err
}

func (g *selectedWatchDisconnectGate) disconnect() {
	g.mu.Lock()
	body := g.body
	g.mu.Unlock()
	body.disconnected.Store(true)
	_ = body.Close()
}

func (g *selectedWatchDisconnectGate) next(t *testing.T) url.Values {
	t.Helper()
	select {
	case <-g.command.Done():
		t.Fatalf("watch ended before attachment: %v: %s", g.command.Err(), g.diagnostics.String())
		return nil
	case query := <-g.requests:
		return query
	case <-time.After(selectedWatchCeiling):
		t.Fatal("public event stream did not attach")
		return nil
	}
}

func (g *selectedWatchDisconnectGate) assertCount(t *testing.T, want int64) {
	t.Helper()
	if got := g.count.Load(); got != want {
		t.Fatalf("event stream requests=%d want=%d", got, want)
	}
}

func runSelectedDurationIsolation(t *testing.T, selected, legacy *selectedWatchHost) {
	t.Helper()
	arrived, release := make(chan struct{}, 2), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"works":[]}`)
	}))
	t.Cleanup(server.Close)
	commands := make([]*support.ProcessCommand, 0, 2)
	outputs := make([]*ledgerOutput, 0, 2)
	for _, host := range []*selectedWatchHost{selected, legacy} {
		stdout, stderr := newLedgerOutput(), newLedgerOutput()
		input := controlledWatchInput(t, t.Context(), server.URL, false, stdout, stderr)
		input.Args = []string{"you", "--server", server.URL, "--verbose", "--json", "work", "list", "--session", "duration-observation"}

		commands = append(commands, support.StartProcessCommand(t, host.process, input))
		outputs = append(outputs, stderr)
	}
	for range commands {
		select {
		case <-arrived:
		case <-time.After(selectedWatchCeiling):
			t.Fatal("duration request did not arrive")
		}
	}
	selectedWatchSource.millis.Add(37)
	close(release)
	for index, command := range commands {
		select {
		case <-command.Done():
		case <-time.After(selectedWatchCeiling):
			t.Fatal("duration command did not finish")
		}
		if err := command.Err(); err != nil {
			t.Fatalf("duration command: %v\n%s", err, outputs[index].String())
		}
		want := "durationMillis=37"
		if index == 1 {
			want = "durationMillis=0"
		}
		if !strings.Contains(outputs[index].String(), want) {
			t.Fatalf("selected process duration want %s: %s", want, outputs[index].String())
		}
	}
}

func TestJoinedSessionCancelResolvesPrimaryWaiterAndPreservesPeer(t *testing.T) {
	ensureWatchFixture(t)
	host := startSelectedWatchHost(t, observationWatchProcess)
	t.Run("primary interruption and useful peer result", func(t *testing.T) {
		t.Parallel()
		selected := newLiveObservation(t, host, true, false, "complete")
		peer := newLiveObservation(t, host, true, false, "complete")
		selected.command.canceled, selected.command.join = make(chan struct{}), make(chan struct{})
		var joined sync.Once
		releaseJoin := func() { joined.Do(func() { close(selected.command.join) }) }
		t.Cleanup(releaseJoin)
		primary := startObservationInvocation(t, selected)
		other := startObservationInvocation(t, peer)
		selected.awaitCommand(t)
		peer.awaitCommand(t)
		cancelJoinedObservation(t, selected, releaseJoin)
		result := awaitObservationInvocation(t, primary)
		if result.Status != factoryapi.InvocationTerminalStatusFailed || result.ErrorCode == nil || *result.ErrorCode != "INVOCATION_INTERRUPTED" || result.PrimaryResult != nil {
			t.Fatalf("joined primary outcome=%+v, want interrupted non-success without primary result", result)
		}
		select {
		case <-peer.watch.Done():
			t.Fatal("selected cancel stopped the held peer watch")
		default:
		}
		close(peer.command.release)
		peerResult := awaitObservationInvocation(t, other)
		encoded, err := json.Marshal(peerResult.PrimaryResult)
		if err != nil || peerResult.Status != factoryapi.InvocationTerminalStatusCompleted || !strings.Contains(string(encoded), "owned result") {
			t.Fatalf("independent peer outcome=%+v error=%v", peerResult, err)
		}
		peer.finish(t)
		peer.assertCanonicalParity(t, decodeWatchLines(t, peer.out.String()))
		select {
		case <-selected.watch.Done():
		case <-time.After(selectedWatchCeiling):
			t.Fatal("primary finite watch did not drain after interruption")
		}
		if selected.watch.Err() == nil || !strings.Contains(selected.diagnostics.String(), "unfinished Work") {
			t.Fatalf("primary finite diagnostic=%q error=%v", selected.diagnostics.String(), selected.watch.Err())
		}
		selected.watch.AcceptError()
		events := support.GetFactoryEventsForSessionAt(t, host.endpoint, selected.session)
		var workID string
		for _, event := range events {
			if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
				if event.Context.WorkIds == nil || len(*event.Context.WorkIds) != 1 {
					t.Fatalf("missing interrupted Work correlation: %+v", event)
				}
				workID = (*event.Context.WorkIds)[0]
			}
		}
		if workID == "" || !strings.Contains(selected.diagnostics.String(), workID) {
			t.Fatalf("primary unfinished Work=%q diagnostic=%q", workID, selected.diagnostics.String())
		}
		selected.assertCanonicalParity(t, decodeWatchLines(t, selected.out.String()))
		later := peer.submit(t, "after-primary-interruption")
		peer.awaitDispatches(t, 2)
		var item factoryapi.Work
		if err := json.Unmarshal([]byte(host.execute(t, "work", "show", later, "--session", peer.session)), &item); err != nil || item.State == nil || item.State.Type != factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("later peer Work=%+v error=%v", item, err)
		}
	})
}

type observationInvocationOutcome struct {
	result factoryapi.InvocationResponse
	err    error
}

// The omitted selector is a host-owned singular default, so this cell is
// serialized; explicit peer sessions retain independent gates on the same host.
func TestWorkWatchJoinedDefaultCancelDrainsMixedCohort(t *testing.T) {
	ensureWatchFixture(t)
	dir := support.ScaffoldFactory(t, observationFactoryConfig(true, "complete"))
	command := newHeldObservationCommand(t)
	observationCommands.routes.Store(filepath.Clean(dir), command)
	t.Cleanup(func() { observationCommands.routes.Delete(filepath.Clean(dir)) })
	host, _ := startWatchHost(t, observationWatchProcess, dir, "--no-record")
	completed := runAutomaticDefaultWatch(t, host, command)
	first := decodeWatchLines(t, completed)[0].WorkID
	command = newHeldObservationCommand(t)
	command.canceled, command.join = make(chan struct{}), make(chan struct{})
	observationCommands.routes.Store(filepath.Clean(dir), command)
	var joined sync.Once
	releaseJoin := func() { joined.Do(func() { close(command.join) }) }
	t.Cleanup(releaseJoin)
	selected := &liveObservation{host: host, session: "~default", command: command}
	unfinished := selected.submit(t, "unfinished-default")
	selected.awaitCommand(t)
	selected = attachLiveObservation(t, host, "~default", command, false)
	peer := newLiveObservation(t, host, true, false, "complete")
	peerWork := peer.submit(t, "independent-default-cancel-peer")
	peer.awaitCommand(t)
	cancelJoinedObservation(t, selected, releaseJoin)
	select {
	case <-selected.watch.Done():
	case <-time.After(selectedWatchCeiling):
		t.Fatal("default mixed cohort did not drain after joined cancellation")
	}
	if selected.watch.Err() == nil || !strings.Contains(selected.diagnostics.String(), unfinished) || strings.Contains(selected.diagnostics.String(), first) {
		t.Fatalf("mixed unfinished diagnostic=%q error=%v", selected.diagnostics.String(), selected.watch.Err())
	}
	selected.watch.AcceptError()
	if selected.out.String() != completed {
		t.Fatalf("cancellation emitted a noncanonical transition: got=%q want=%q", selected.out.String(), completed)
	}
	selected.assertCanonicalParity(t, decodeWatchLines(t, selected.out.String()))
	close(peer.command.release)
	peer.finish(t)
	lines := decodeWatchLines(t, peer.out.String())
	assertWorkWatchTransitionLines(t, lines, peer.session, peerWork, [][2]string{{"init", "complete"}})
	if string(lines[0].StructuredResult) != observationOutput(true) {
		t.Fatalf("peer useful result=%+v", lines)
	}
	peer.assertCanonicalParity(t, lines)
}

// HTTP invocation plus CLI cancel is an explicit API/CLI parity cell: both
// entry points must resolve the same selected invocation and canonical Work.
func startObservationInvocation(t *testing.T, s *liveObservation) <-chan observationInvocationOutcome {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.host.endpoint+"/factory-sessions/"+s.session+"/invocations", bytes.NewBufferString(`{"sourceKind":"text","content":[{"type":"text","text":"held primary"}]}`))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	done := make(chan observationInvocationOutcome, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		var outcome observationInvocationOutcome
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			defer response.Body.Close()
			err = json.NewDecoder(response.Body).Decode(&outcome.result)
			if response.StatusCode != http.StatusOK {
				err = fmt.Errorf("invocation HTTP status=%d", response.StatusCode)
			}
		}
		outcome.err = err
		done <- outcome
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-joined:
		case <-time.After(selectedWatchCeiling):
			t.Error("owned invocation HTTP request did not join")
		}
	})
	return done
}

func awaitObservationInvocation(t *testing.T, done <-chan observationInvocationOutcome) factoryapi.InvocationResponse {
	t.Helper()
	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		return outcome.result
	case <-time.After(selectedWatchCeiling):
		t.Fatal("primary invocation did not complete from canonical observation")
		return factoryapi.InvocationResponse{}
	}
}
