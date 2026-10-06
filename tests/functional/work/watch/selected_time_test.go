package watch_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const selectedWatchCeiling = 30 * time.Second

type selectedWatchHost struct {
	process  support.ApplicationProcess
	endpoint string
}

func startSelectedWatchHost(t *testing.T, process support.ApplicationProcess) *selectedWatchHost {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	entry := &watchHostListener{listener: listener, ready: make(chan struct{})}
	watchHostListeners.Store(port, entry)
	t.Cleanup(func() { watchHostListeners.Delete(port); _ = listener.Close() })
	host := &selectedWatchHost{process: process, endpoint: "http://" + listener.Addr().String()}
	dir := support.ScaffoldFactory(t, workWatchFactoryConfig())
	inputs := workWatchInputs(t, []string{"you", "run", "--listen", listener.Addr().String(), "--dir", dir,
		"--continuously", "--with-server", "--quiet", "--no-record"})
	// The process retains its runtime log sink until Close. Its host profile
	// therefore shares the package process lifetime, rather than t.TempDir.
	profile, err := os.MkdirTemp(watchProfileRoot, "host-")
	if err != nil {
		t.Fatal(err)
	}
	inputs.Input.Env = isolatedHomeEnvironment(profile)
	inputs.Input.WorkingDirectory = profile

	inputs.Input.Context = context.Background()
	command := support.StartProcessCommand(t, process, inputs.Input)
	for {
		select {
		case timer := <-selectedWatchScheduler.startup:
			timer.wake(selectedWatchScheduler.Now())
		case <-entry.ready:
			return host
		case <-command.Done():
			t.Fatalf("host ended before readiness: %v\n%s", command.Err(), inputs.Stderr())
		case <-time.After(selectedWatchCeiling):
			t.Fatalf("host readiness unavailable: %s", inputs.Stderr())
		}
	}
}

func (h *selectedWatchHost) execute(t *testing.T, args ...string) string {
	t.Helper()
	inputs := workWatchInputs(t, append([]string{"you", "--server", h.endpoint, "--json"}, args...))
	ctx, cancel := context.WithTimeout(context.Background(), selectedWatchCeiling)
	defer cancel()
	inputs.Input.Context = ctx
	done := make(chan error, 1)
	go func() { done <- h.process.Execute(inputs.Input) }()
	var err error
	for {
		select {
		case timer := <-selectedWatchScheduler.startup:
			timer.wake(selectedWatchScheduler.Now())
		case err = <-done:
			goto finished
		case <-time.After(selectedWatchCeiling):
			t.Fatalf("public command did not finish: %v", args)
		}
	}
finished:
	if err != nil {
		t.Fatalf("public command %v: %v\n%s\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs.Stdout()
}

type selectedWatchScenario struct {
	host           *selectedWatchHost
	session        string
	work           string
	gate           *selectedWatchDisconnectGate
	command        *support.ProcessCommand
	stdout, stderr *ledgerOutput
	cancel         context.CancelFunc
}

func newSelectedWatchScenario(t *testing.T, host *selectedWatchHost) *selectedWatchScenario {
	t.Helper()
	dir := support.ScaffoldFactory(t, workWatchFactoryConfig())
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal([]byte(host.execute(t, "session", "create", "--dir", dir)), &opened); err != nil {
		t.Fatal(err)
	}
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("missing public session identity: %+v", opened)
	}
	s := &selectedWatchScenario{host: host, session: opened.Session.Id, stdout: newLedgerOutput(), stderr: newLedgerOutput()}
	t.Cleanup(func() { host.execute(t, "session", "terminate", s.session) })
	payload := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payload, []byte("observe owned work"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.work = decodeSubmittedWorkID(t, host.execute(t, "submit", "--session", s.session,
		"--name", "selected-time", "--work-type-name", workWatchWorkType, "--payload", payload))
	s.gate = newSelectedWatchDisconnectGate(t, host.endpoint, s.session)
	ctx, cancel := context.WithCancel(t.Context())
	s.cancel = cancel
	t.Cleanup(cancel)
	input := controlledWatchInput(t, ctx, s.gate.server.URL, false, s.stdout, s.stderr)
	input.Args = []string{"you", "--server", s.gate.server.URL, "work", "watch", "--session", s.session}
	s.command = support.StartProcessCommand(t, host.process, input)
	s.gate.command = s.command
	s.gate.diagnostics = s.stderr
	s.gate.next(t)
	select {
	case <-s.gate.attached:
	case <-time.After(selectedWatchCeiling):
		t.Fatal("initial stream headers unavailable")
	}
	return s
}

func (s *selectedWatchScenario) move(t *testing.T, state string) {
	t.Helper()
	s.host.execute(t, "work", "move", s.work, state, "--session", s.session)
	waitForLedgerLines(t, s.stdout, 1, "owned public transition")
}

func (s *selectedWatchScenario) finish(t *testing.T) {
	t.Helper()
	s.move(t, "complete")
	select {
	case <-s.command.Done():
	case <-time.After(selectedWatchCeiling):
		t.Fatal("terminal watch did not finish")
	}
	if err := s.command.Err(); err != nil {
		t.Fatalf("watch: %v\n%s", err, s.stderr.String())
	}
	if s.stderr.String() != "" {
		t.Fatalf("unexpected diagnostics: %s", s.stderr.String())
	}
	assertWorkWatchTransitionLines(t, decodeWatchLines(t, s.stdout.String()), s.session, s.work,
		[][2]string{{"init", "processing"}, {"processing", "complete"}})
	if !strings.HasSuffix(s.stdout.String(), "\n") {
		t.Fatal("incomplete NDJSON frame")
	}
}

// Two graphs host and execute all journeys. The coordinated journey shares one
// advancement epoch; its watches own distinct sessions, streams and outputs.
func runWorkWatchSelectedProcessTime(t *testing.T) {
	selected := startSelectedWatchHost(t, selectedWatchProcess)
	legacy := startSelectedWatchHost(t, workWatchProcess)
	runSelectedDurationIsolation(t, selected, legacy) // CLI-F06, before source freezing.
	terminal := newSelectedWatchScenario(t, selected)
	reconnect := newSelectedWatchScenario(t, selected)
	canceled := newSelectedWatchScenario(t, selected)
	peer := newSelectedWatchScenario(t, selected)
	t.Run("CLI-F01 terminal", func(t *testing.T) {
		t.Parallel()
		runWorkWatchFollowsStateTransitionsUntilTerminal(t, terminal)
	})
	t.Run("CLI-F02 F03 F04 selected reconnect cancellation and peer", func(t *testing.T) {
		t.Parallel()
		runSelectedReconnectJourney(t, reconnect, canceled, peer)
	})
	t.Run("CLI-F05 frozen Now-only source uses wall scheduler", func(t *testing.T) {
		t.Parallel()
		frozen := legacyWatchSource.Now()
		s := newSelectedWatchScenario(t, legacy)
		s.move(t, "processing")
		first := decodeWatchLines(t, s.stdout.String())[0]
		s.gate.disconnect()
		query := s.gate.next(t)
		assertSelectedCursor(t, query, first)
		if !legacyWatchSource.Now().Equal(frozen) {
			t.Fatal("Now-only source advanced")
		}
		s.finish(t)
	})
}

func runSelectedReconnectJourney(t *testing.T, reconnect, canceled, peer *selectedWatchScenario) {
	t.Helper()
	for _, s := range []*selectedWatchScenario{reconnect, canceled, peer} {
		s.move(t, "processing")
	}
	first := decodeWatchLines(t, reconnect.stdout.String())[0]
	reconnect.gate.disconnect()
	canceled.gate.disconnect()
	awaitSelectedBackoff(t)
	awaitSelectedBackoff(t)
	selectedWatchScheduler.advance(99)
	reconnect.gate.assertCount(t, 1)
	canceled.gate.assertCount(t, 1)
	canceled.cancel()
	select {
	case <-canceled.command.Done():
	case <-time.After(selectedWatchCeiling):
		t.Fatal("pending watch cancellation did not finish")
	}
	if err := canceled.command.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error=%v", err)
	}
	canceled.command.AcceptError()
	assertExpectedWatchCancellationDiagnostic(t, canceled.stderr.String())
	// A healthy peer makes public progress while the reconnect is still pending.
	peer.finish(t)
	reconnect.gate.assertCount(t, 1)
	selectedWatchScheduler.advance(1)
	query := reconnect.gate.next(t)
	assertSelectedCursor(t, query, first)
	reconnect.finish(t)
	selectedWatchScheduler.advance(900)
	canceled.gate.assertCount(t, 1)
	lines := decodeWatchLines(t, canceled.stdout.String())
	if len(lines) != 1 || lines[0].SessionID != canceled.session || lines[0].WorkID != canceled.work || lines[0].Terminal {
		t.Fatalf("canceled watch framing/identity=%+v", lines)
	}
}

func awaitSelectedBackoff(t *testing.T) {
	t.Helper()
	select {
	case delay := <-selectedWatchScheduler.created:
		if delay != 100*time.Millisecond {
			t.Fatalf("first backoff=%v", delay)
		}
	case <-time.After(selectedWatchCeiling):
		t.Fatal("selected scheduler received no backoff")
	}
}

func assertSelectedCursor(t *testing.T, query map[string][]string, first workWatchLine) {
	t.Helper()
	if fmt.Sprint(query["after_sequence"]) != "["+fmt.Sprint(first.Sequence)+"]" ||
		fmt.Sprint(query["after_event_id"]) != "["+first.EventID+"]" {
		t.Fatalf("reconnect cursor=%v want sequence=%d event=%s", query, first.Sequence, first.EventID)
	}
}

// isolatedHomeEnvironment keeps configuration and model discovery in this
// scenario's home without changing the environment used by parallel tests.
func isolatedHomeEnvironment(home string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") ||
			strings.EqualFold(name, runcli.ModelCacheDirEnvironment) {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+home, "USERPROFILE="+home,
		runcli.ModelCacheDirEnvironment+"="+filepath.Join(home, ".agent-factory", "models"))
}
