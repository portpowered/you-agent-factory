package watch_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The reusable graph selects each controlled command by its owned Factory
// directory. No provider execution policy or process-global output mode changes.
type observationCommandRouter struct{ routes sync.Map }
type observationCommand struct {
	arrived chan struct{}
	release chan struct{}
	once    sync.Once
	result  platformprocess.CommandRunner
}

func (r *observationCommandRouter) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	value, ok := r.routes.Load(filepath.Clean(req.WorkDir))
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("unowned observation command directory %q", req.WorkDir)
	}
	command := value.(*observationCommand)
	command.once.Do(func() { close(command.arrived) })
	select {
	case <-command.release:
		return command.result.Run(ctx, req)
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

type liveObservation struct {
	host             *selectedWatchHost
	session          string
	command          *observationCommand
	watch            *support.ProcessCommand
	out, diagnostics *ledgerOutput
	cancel           context.CancelFunc
	response         *http.Response
	reader           *bufio.Reader
}

func TestWorkWatchLiveProviderObservation(t *testing.T) {
	ensureWatchFixture(t)
	host := startSelectedWatchHost(t, observationWatchProcess)
	t.Run("WATCH1 WATCH2 WATCH3 provider results and retained cohort", func(t *testing.T) {
		runLiveProviderCohort(t, host)
	})
	t.Run("WATCH4 WATCH8 PEER1 cancellation leaves provider peer and cursor usable", func(t *testing.T) {
		runLiveProviderPeer(t, host)
	})
}

func runLiveProviderCohort(t *testing.T, host *selectedWatchHost) {
	t.Parallel()
	s := newLiveObservation(t, host, true, false)
	first := s.submit(t, "first")
	second := s.submit(t, "second")
	s.awaitCommand(t)
	close(s.command.release)
	s.awaitDispatches(t, 2)
	// Actual dispatch relocations precede operator moves; the dispatch event
	// itself still emits no line.
	waitForLedgerLines(t, s.out, 2, "automatic review relocations")
	s.move(t, first, "processing")
	s.move(t, second, "processing")
	s.move(t, first, "complete")
	waitForLedgerLines(t, s.out, 3, "first terminal cohort member")
	select {
	case <-s.watch.Done():
		t.Fatal("finite watch lost unfinished cohort member")
	default:
	}
	s.move(t, second, "complete")
	s.finish(t)
	lines := decodeWatchLines(t, s.out.String())
	assertLiveObservationLines(t, lines, s.session, []string{first, second})
	s.assertCanonicalParity(t, lines)
	out, diagnostics := newLedgerOutput(), newLedgerOutput()
	input := controlledWatchInput(t, t.Context(), host.endpoint, false, out, diagnostics)
	input.Args = []string{"you", "--server", host.endpoint, "work", "watch", "--session", s.session}
	retained := support.StartProcessCommand(t, host.process, input)
	waitForLedgerCommand(t, retained, out, diagnostics)
	if out.String() != s.out.String() {
		t.Fatalf("retained transitions differ:\nlive=%s\nretained=%s", s.out.String(), out.String())
	}
}

func runLiveProviderPeer(t *testing.T, host *selectedWatchHost) {
	t.Parallel()
	a := newLiveObservation(t, host, false, true)
	b := newLiveObservation(t, host, true, false)
	aWork := a.submit(t, "held")
	a.awaitCommand(t)
	bWork := b.submit(t, "peer")
	b.awaitCommand(t)
	// Peer provider output and transitions progress while A's provider is held.
	close(b.command.release)
	b.awaitDispatches(t, 1)
	b.move(t, bWork, "processing")
	b.move(t, bWork, "complete")
	b.finish(t)
	bLines := decodeWatchLines(t, b.out.String())
	assertLiveObservationLines(t, bLines, b.session, []string{bWork})
	b.assertCanonicalParity(t, bLines)
	bad := workWatchInputs(t, []string{"you", "--server", host.endpoint, "--json", "work", "show", "unknown", "--session", a.session})
	if err := host.process.Execute(bad.Input); err == nil || bad.Stdout() != "" {
		t.Fatalf("unknown Work read=%v stdout=%q", err, bad.Stdout())
	}
	runUnstructuredFollow(t, a, aWork)
	// A's watcher cancellation leaves B's runtime and cursor alive.
	bLater := b.submit(t, "after-peer-cancel")
	b.awaitDispatches(t, 1)
	b.move(t, bLater, "complete")
	events := support.GetFactoryEventsAfterForSessionAt(t, host.endpoint, b.session, support.FactoryEventReadCursor{AfterEventID: bLines[len(bLines)-1].EventID})
	found := false
	for _, event := range events {
		if int64(event.Context.Sequence) <= bLines[len(bLines)-1].Sequence {
			t.Fatalf("peer cursor regressed: %+v", event.Context)
		}
		if event.Type == factoryapi.FactoryEventTypeWorkStateChange {
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil || payload.WorkId != bLater {
				t.Fatalf("foreign peer transition: %+v %v", payload, err)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("peer cursor lost later admission/transition")
	}
}

func runUnstructuredFollow(t *testing.T, a *liveObservation, aWork string) {
	t.Helper()
	close(a.command.release)
	a.awaitDispatches(t, 1)
	a.move(t, aWork, "processing")
	a.move(t, aWork, "failed")
	waitForLedgerLines(t, a.out, 3, "unstructured failed Work")
	select {
	case <-a.watch.Done():
		t.Fatal("follow stopped at failed terminal Work")
	default:
	}
	aLater := a.submit(t, "follow-later")
	a.awaitDispatches(t, 1)
	a.move(t, aLater, "complete")
	waitForLedgerLines(t, a.out, 2, "later followed transition")
	aLines := decodeWatchLines(t, a.out.String())
	if len(aLines) != 5 || !aLines[2].Terminal || aLines[2].ToState != "failed" || aLines[4].WorkID != aLater || !aLines[4].Terminal {
		t.Fatalf("follow/failed transitions=%+v", aLines)
	}
	for _, line := range aLines {
		if line.SessionID != a.session || len(line.StructuredResult) != 0 {
			t.Fatalf("unstructured attribution/result=%+v", line)
		}
	}
	a.cancel()
	select {
	case <-a.watch.Done():
	case <-time.After(selectedWatchCeiling):
		t.Fatal("owned watch did not cancel")
	}
	if !errors.Is(a.watch.Err(), context.Canceled) {
		t.Fatalf("watch cancellation=%v", a.watch.Err())
	}
	a.watch.AcceptError()
	assertExpectedWatchCancellationDiagnostic(t, a.diagnostics.String())
}

func newLiveObservation(t *testing.T, host *selectedWatchHost, structured, follow bool) *liveObservation {
	t.Helper()
	config := observationFactoryConfig(structured, "review")
	dir := support.ScaffoldFactory(t, config)
	command := &observationCommand{arrived: make(chan struct{}), release: make(chan struct{}), result: support.NewStaticSuccessCommandRunner(observationOutput(structured))}
	observationCommands.routes.Store(filepath.Clean(dir), command)
	t.Cleanup(func() { observationCommands.routes.Delete(filepath.Clean(dir)) })
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal([]byte(host.execute(t, "session", "create", "--dir", dir)), &opened); err != nil {
		t.Fatal(err)
	}
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatal("missing selected session")
	}
	t.Cleanup(func() { host.execute(t, "session", "terminate", opened.Session.Id) })
	return attachLiveObservation(t, host, opened.Session.Id, command, follow)
}

func attachLiveObservation(t *testing.T, host *selectedWatchHost, session string, command *observationCommand, follow bool) *liveObservation {
	t.Helper()
	s := &liveObservation{host: host, session: session, command: command, out: newLedgerOutput(), diagnostics: newLedgerOutput()}
	ctx, cancel := context.WithCancel(t.Context())
	s.cancel = cancel
	t.Cleanup(cancel)
	// A real canonical stream supplies provider completion signals and verifies
	// no transition is inferred from dispatch-only execution.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, support.SessionEventsURL(host.endpoint, s.session), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "text/event-stream")
	s.response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if s.response.StatusCode != http.StatusOK {
		t.Fatalf("canonical stream status=%d", s.response.StatusCode)
	}
	s.reader = bufio.NewReader(s.response.Body)
	t.Cleanup(func() { _ = s.response.Body.Close() })
	gate := newSelectedWatchDisconnectGate(t, host.endpoint, s.session)
	input := controlledWatchInput(t, ctx, gate.server.URL, follow, s.out, s.diagnostics)
	input.Args = []string{"you", "--server", gate.server.URL, "work", "watch"}
	if session != "~default" {
		input.Args = append(input.Args, "--session", session)
	}
	if follow {
		input.Args = append(input.Args, "--follow")
	}
	s.watch = support.StartProcessCommand(t, host.process, input)
	gate.command, gate.diagnostics = s.watch, s.diagnostics
	gate.next(t)
	select {
	case <-gate.attached:
	case <-s.watch.Done():
		t.Fatalf("watch rejected stream: %v", s.watch.Err())
	case <-time.After(selectedWatchCeiling):
		t.Fatal("watch headers unavailable")
	}
	return s
}

func (s *liveObservation) submit(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.md")
	if err := os.WriteFile(path, []byte("controlled observation"), 0o600); err != nil {
		t.Fatal(err)
	}
	return decodeSubmittedWorkID(t, s.host.execute(t, "submit", "--session", s.session, "--name", name, "--work-type-name", "task", "--payload", path))
}

func (s *liveObservation) awaitCommand(t *testing.T) {
	t.Helper()
	select {
	case <-s.command.arrived:
	case <-time.After(selectedWatchCeiling):
		t.Fatal("controlled provider command did not arrive")
	}
}

func (s *liveObservation) awaitDispatches(t *testing.T, count int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), selectedWatchCeiling)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = s.response.Body.Close() })
	defer stop()
	for count > 0 {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			t.Fatalf("canonical dispatch wait: %v", err)
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatal(err)
		}
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil || payload.Outcome == factoryapi.WorkOutcomeFailed {
				t.Fatalf("provider dispatch: %+v %v", payload, err)
			}
			count--
		}
	}
}

func (s *liveObservation) move(t *testing.T, work, state string) {
	t.Helper()
	s.host.execute(t, "work", "move", work, state, "--session", s.session)
}

func (s *liveObservation) finish(t *testing.T) {
	t.Helper()
	waitForLedgerCommand(t, s.watch, s.out, s.diagnostics)
	if s.diagnostics.String() != "" || !strings.HasSuffix(s.out.String(), "\n") {
		t.Fatalf("framing stdout=%q stderr=%q", s.out.String(), s.diagnostics.String())
	}
}

func assertLiveObservationLines(t *testing.T, lines []workWatchLine, session string, works []string) {
	t.Helper()
	if len(lines) != 3*len(works) {
		t.Fatalf("transitions=%+v want three per Work", lines)
	}
	for _, work := range works {
		var selected []workWatchLine
		for _, line := range lines {
			if line.WorkID == work {
				selected = append(selected, line)
			}
		}
		assertWorkWatchTransitionLines(t, selected, session, work, [][2]string{{"init", "review"}, {"review", "processing"}, {"processing", "complete"}})
		if string(selected[0].StructuredResult) != `{"message":"owned result"}` || len(selected[1].StructuredResult) != 0 || len(selected[2].StructuredResult) != 0 {
			t.Fatalf("first-transition native result=%s later=%s", selected[0].StructuredResult, selected[1].StructuredResult)
		}
	}
	for i, line := range lines {
		if (line.Source != "api" && line.Source != "dispatch") || (i > 0 && line.Sequence <= lines[i-1].Sequence) {
			t.Fatalf("canonical order/source=%+v", lines)
		}
	}
}

func (s *liveObservation) assertCanonicalParity(t *testing.T, lines []workWatchLine) {
	t.Helper()
	events := support.GetFactoryEventsForSessionAt(t, s.host.endpoint, s.session)
	var index int
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkStateChange {
			continue
		}
		payload, err := event.Payload.AsWorkStateChangeEventPayload()
		if err != nil || index >= len(lines) {
			t.Fatalf("unexpected canonical transition: %+v %v", event, err)
		}
		line := lines[index]
		if event.Id != line.EventID || int64(event.Context.Sequence) != line.Sequence || payload.WorkId != line.WorkID || payload.FromState != line.FromState || payload.ToState != line.ToState {
			t.Fatalf("canonical/watch mismatch: %+v %+v", event, line)
		}
		index++
	}
	if index != len(lines) {
		t.Fatal("watch emitted a noncanonical transition")
	}
	s.assertTerminalWorkStates(t, lines)
}

func (s *liveObservation) assertTerminalWorkStates(t *testing.T, lines []workWatchLine) {
	t.Helper()
	for _, line := range lines {
		if !line.Terminal {
			continue
		}
		var selected factoryapi.Work
		if err := json.Unmarshal([]byte(s.host.execute(t, "work", "show", line.WorkID, "--session", s.session)), &selected); err != nil {
			t.Fatal(err)
		}
		if selected.State == nil || selected.State.Name != line.ToState {
			t.Fatalf("Work/transition disagreement: %+v %+v", selected, line)
		}
	}
}

func observationFactoryConfig(structured bool, destination string) map[string]any {
	config := workWatchFactoryConfig()
	states := config["workTypes"].([]map[string]any)[0]["states"].([]map[string]any)
	config["workTypes"].([]map[string]any)[0]["states"] = append(states, map[string]any{"name": "review", "type": "PROCESSING"})
	config["workers"] = []map[string]any{{"name": "processor", "type": "AGENT_WORKER", "modelProvider": "CODEX", "model": "controlled"}}
	station := map[string]any{"name": "process", "type": "AGENT_RUN", "worker": "processor",
		"inputs":    []map[string]any{{"workType": "task", "state": "init"}},
		"outputs":   []map[string]any{{"workType": "task", "state": destination}},
		"onFailure": []map[string]any{{"workType": "task", "state": "failed"}}}
	if structured {
		station["outputSchema"] = `{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`
	}
	config["workstations"] = []map[string]any{station}
	return config
}

func observationOutput(structured bool) string {
	if structured {
		return `{"message":"owned result"}`
	}
	return "controlled unstructured output"
}

// Default selection is host-owned; explicit peers share its public server and
// command edge while each cohort is held until both Works have been admitted.
func TestWorkWatchDefaultAndExplicitCohortsOverlap(t *testing.T) {
	ensureWatchFixture(t)
	dir := support.ScaffoldFactory(t, observationFactoryConfig(true, "review"))
	command := &observationCommand{arrived: make(chan struct{}), release: make(chan struct{}), result: support.NewStaticSuccessCommandRunner(observationOutput(true))}
	observationCommands.routes.Store(filepath.Clean(dir), command)
	t.Cleanup(func() { observationCommands.routes.Delete(filepath.Clean(dir)) })
	host, _ := startWatchHost(t, observationWatchProcess, dir, "--no-record")
	selected := attachLiveObservation(t, host, "~default", command, false)
	peer := newLiveObservation(t, host, true, false)
	defaultWorks := []string{selected.submit(t, "default-first"), selected.submit(t, "default-second")}
	peerWorks := []string{peer.submit(t, "peer-first"), peer.submit(t, "peer-second")}
	selected.awaitCommand(t)
	peer.awaitCommand(t)
	close(command.release)
	close(peer.command.release)
	selected.awaitDispatches(t, 2)
	peer.awaitDispatches(t, 2)
	waitForLedgerLines(t, selected.out, 2, "default automatic relocations")
	waitForLedgerLines(t, peer.out, 2, "peer automatic relocations")
	for index, work := range defaultWorks {
		selected.move(t, work, "processing")
		peer.move(t, peerWorks[index], "processing")
	}
	selected.move(t, defaultWorks[0], "complete")
	peer.move(t, peerWorks[0], "complete")
	waitForLedgerLines(t, selected.out, 3, "default first terminal")
	waitForLedgerLines(t, peer.out, 3, "peer first terminal")
	for _, s := range []*liveObservation{selected, peer} {
		select {
		case <-s.watch.Done():
			t.Fatalf("%s completed with an unfinished cohort member", s.session)
		default:
		}
	}
	selected.move(t, defaultWorks[1], "complete")
	selected.finish(t)
	defaultLines := decodeWatchLines(t, selected.out.String())
	assertLiveObservationLines(t, defaultLines, selected.session, defaultWorks)
	selected.assertCanonicalParity(t, defaultLines)
	// Closing the default watch must preserve the explicit peer's remaining
	// member and its cursor on the same host.
	cursor := decodeWatchLines(t, peer.out.String())
	peer.move(t, peerWorks[1], "complete")
	peer.finish(t)
	peerLines := decodeWatchLines(t, peer.out.String())
	assertLiveObservationLines(t, peerLines, peer.session, peerWorks)
	peer.assertCanonicalParity(t, peerLines)
	events := support.GetFactoryEventsAfterForSessionAt(t, host.endpoint, peer.session, support.FactoryEventReadCursor{AfterEventID: cursor[len(cursor)-1].EventID})
	if len(events) == 0 || int64(events[0].Context.Sequence) <= cursor[len(cursor)-1].Sequence {
		t.Fatal("peer cursor did not survive default watch completion")
	}
}

// Compatibility default ownership is serialized across real stop/resume. The
// same process and command edge serve every host; explicit peers remain parallel.
func TestWorkWatchDefaultAutomaticFreshAndResumed(t *testing.T) {
	ensureWatchFixture(t)
	dir := support.ScaffoldFactory(t, observationFactoryConfig(true, "complete"))
	command := &observationCommand{arrived: make(chan struct{}), release: make(chan struct{}), result: support.NewStaticSuccessCommandRunner(observationOutput(true))}
	observationCommands.routes.Store(filepath.Clean(dir), command)
	t.Cleanup(func() { observationCommands.routes.Delete(filepath.Clean(dir)) })
	source := filepath.Join(t.TempDir(), "empty-default.json")
	host, run := startWatchHost(t, observationWatchProcess, dir, "--record", source)
	t.Run("empty default and explicit peer before recovery", func(t *testing.T) {
		s := attachLiveObservation(t, host, "~default", command, false)
		// An empty default never borrows an explicit peer's terminal history.
		peer := newLiveObservation(t, host, true, false)
		peerWork := peer.submit(t, "peer")
		peer.awaitCommand(t)
		close(peer.command.release)
		peer.awaitDispatches(t, 1)
		peer.move(t, peerWork, "complete")
		peer.finish(t)
		if s.out.String() != "" {
			t.Fatalf("default borrowed peer output: %s", s.out.String())
		}
		select {
		case <-s.watch.Done():
			t.Fatal("empty default completed from peer")
		default:
		}
		// Stop with an empty recorded cohort, then attach before resumed admission.
		s.cancel()
		s.watch.Stop(t)
		s.watch.AcceptError()
		assertExpectedWatchCancellationDiagnostic(t, s.diagnostics.String())
	})
	// The peer cleanup runs while its host is alive.
	run.Stop(t)
	resumed, resumedRun := startWatchHost(t, observationWatchProcess, dir, "--resume", source, "--record", filepath.Join(t.TempDir(), "successor.json"))
	runAutomaticDefaultWatch(t, resumed, command)
	resumedRun.Stop(t)
	// A separate fresh recorded host proves the omitted selector without replay.
	command = &observationCommand{arrived: make(chan struct{}), release: make(chan struct{}), result: support.NewStaticSuccessCommandRunner(observationOutput(true))}
	observationCommands.routes.Store(filepath.Clean(dir), command)
	freshSource := filepath.Join(t.TempDir(), "fresh.json")
	fresh, freshRun := startWatchHost(t, observationWatchProcess, dir, "--record", freshSource)
	terminalOutput := runAutomaticDefaultWatch(t, fresh, command)
	freshRun.Stop(t)
	terminalHost, terminalRun := startWatchHost(t, observationWatchProcess, dir, "--resume", freshSource, "--record", filepath.Join(t.TempDir(), "terminal-successor.json"))
	out, diagnostics := newLedgerOutput(), newLedgerOutput()
	input := controlledWatchInput(t, t.Context(), terminalHost.endpoint, false, out, diagnostics)
	input.Args = []string{"you", "--server", terminalHost.endpoint, "work", "watch"}
	retained := support.StartProcessCommand(t, terminalHost.process, input)
	waitForLedgerCommand(t, retained, out, diagnostics)
	if out.String() != terminalOutput || diagnostics.String() != "" {
		t.Fatalf("terminal resumed watch stdout=%s stderr=%s want=%s", out.String(), diagnostics.String(), terminalOutput)
	}
	s := &liveObservation{host: terminalHost, session: "~default"}
	s.assertCanonicalParity(t, decodeWatchLines(t, out.String()))
	terminalRun.Stop(t)
}

func runAutomaticDefaultWatch(t *testing.T, host *selectedWatchHost, command *observationCommand) string {
	t.Helper()
	s := attachLiveObservation(t, host, "~default", command, false)
	id := s.submit(t, "automatic")
	s.awaitCommand(t)
	close(command.release)
	s.finish(t)
	lines := decodeWatchLines(t, s.out.String())
	assertWorkWatchTransitionLines(t, lines, "~default", id, [][2]string{{"init", "complete"}})
	if lines[0].Source != "dispatch" || string(lines[0].StructuredResult) != observationOutput(true) {
		t.Fatalf("automatic native transition=%+v", lines)
	}
	s.assertCanonicalParity(t, lines)
	events := support.GetFactoryEventsForSessionAt(t, host.endpoint, "~default")
	var responseSequence int64 = -1
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			responseSequence = int64(event.Context.Sequence)
		}
	}
	if responseSequence < 0 || responseSequence >= lines[0].Sequence {
		t.Fatalf("result/relocation order: response=%d transition=%d", responseSequence, lines[0].Sequence)
	}
	out, diagnostics := newLedgerOutput(), newLedgerOutput()
	input := controlledWatchInput(t, t.Context(), host.endpoint, false, out, diagnostics)
	input.Args = []string{"you", "--server", host.endpoint, "work", "watch"}
	retained := support.StartProcessCommand(t, host.process, input)
	waitForLedgerCommand(t, retained, out, diagnostics)
	if out.String() != s.out.String() {
		t.Fatalf("retained default differs: live=%s retained=%s", s.out.String(), out.String())
	}
	return s.out.String()
}
