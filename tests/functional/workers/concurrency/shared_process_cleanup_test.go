package root_composition_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type concurrencyForcedCleanupReport struct {
	ApplicationPID        int      `json:"application_pid"`
	ProcessClosed         bool     `json:"process_closed"`
	ProcessCloseError     string   `json:"process_close_error,omitempty"`
	DaemonStopped         bool     `json:"daemon_stopped"`
	ListenerClosed        bool     `json:"listener_closed"`
	OpenedSessionIDs      []string `json:"opened_session_ids"`
	DeletedSessionIDs     []string `json:"deleted_session_ids"`
	ActiveRoutes          int      `json:"active_routes"`
	ActiveCalls           int      `json:"active_calls"`
	CommandCalls          int      `json:"command_calls"`
	CommandStarted        int      `json:"command_started"`
	CommandFinished       int      `json:"command_finished"`
	CanceledCalls         int      `json:"canceled_calls"`
	ResponseStreamsClosed int      `json:"response_streams_closed"`
	OwnedRootsAbsent      bool     `json:"owned_roots_absent"`
}

type concurrencyForcedCleanupProbe struct {
	fixture       *concurrencySharedProcessFixture
	sessions      []*concurrencySession
	streams       []*support.FactoryResponseEventStream
	streamsClosed int
}

func runConcurrencyForcedCleanupParent(t *testing.T) {
	t.Helper()
	reportPath := filepath.Join(t.TempDir(), "concurrency-forced-cleanup.json")
	command := exec.Command(os.Args[0], "-test.run=^TestConcurrencySharedProcess$")
	command.Env = append(os.Environ(), concurrencyForcedCleanupChildEnv+"=1", concurrencyForcedCleanupReportEnv+"="+reportPath)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("forced concurrency cleanup child exited successfully; output=%q", output)
	}
	if command.Process == nil || command.ProcessState == nil || command.ProcessState.ExitCode() == 0 {
		t.Fatalf("forced concurrency cleanup child exit state = %#v; output=%q", command.ProcessState, output)
	}
	if !strings.Contains(string(output), "intentional concurrency cleanup assertion") {
		t.Fatalf("forced concurrency cleanup child output omitted original assertion: %q", output)
	}
	payload, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read forced concurrency cleanup report: %v; output=%q", err, output)
	}
	var report concurrencyForcedCleanupReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatalf("decode forced concurrency cleanup report: %v; output=%q", err, output)
	}
	if report.ApplicationPID != command.Process.Pid || !report.ProcessClosed || report.ProcessCloseError != "" || !report.DaemonStopped || !report.ListenerClosed {
		t.Fatalf("forced concurrency process state = %#v, want closed child process/listener", report)
	}
	if len(report.OpenedSessionIDs) != 2 || !sameConcurrencyStringSet(report.OpenedSessionIDs, report.DeletedSessionIDs) {
		t.Fatalf("forced concurrency sessions opened=%v deleted=%v, want two deleted sessions", report.OpenedSessionIDs, report.DeletedSessionIDs)
	}
	if report.ActiveRoutes != 0 || report.ActiveCalls != 0 || report.CommandCalls != 2 || report.CommandStarted != 2 || report.CommandFinished != 2 || report.CanceledCalls != 2 || report.ResponseStreamsClosed != 2 || !report.OwnedRootsAbsent {
		t.Fatalf("forced concurrency cleanup state = %#v, want zero active resources and two canceled calls/streams", report)
	}
}

func runConcurrencyForcedCleanupChild(t *testing.T) {
	t.Helper()
	reportPath := strings.TrimSpace(os.Getenv(concurrencyForcedCleanupReportEnv))
	if reportPath == "" {
		t.Fatal("forced concurrency cleanup report path is required")
	}
	var fixture *concurrencySharedProcessFixture
	var probe *concurrencyForcedCleanupProbe
	// This cleanup is registered before the fixture so it observes state after
	// streams, sessions, command, process, routes, and roots have unwound.
	t.Cleanup(func() {
		if fixture == nil || probe == nil {
			return
		}
		fixture.processCloseMu.Lock()
		closeErr := fixture.processCloseErr
		fixture.processCloseMu.Unlock()
		opened, deleted := fixture.sessionIDs()
		report := concurrencyForcedCleanupReport{
			ApplicationPID:        os.Getpid(),
			ProcessClosed:         fixture.processClosed.Load(),
			ProcessCloseError:     closeErr,
			DaemonStopped:         fixture.command != nil && channelClosed(fixture.command.Done()),
			ListenerClosed:        channelClosed(fixture.apiClosed),
			OpenedSessionIDs:      opened,
			DeletedSessionIDs:     deleted,
			ActiveRoutes:          fixture.router.routeCount(),
			ResponseStreamsClosed: probe.streamsClosed,
			OwnedRootsAbsent:      concurrencyOwnedRootsAbsent(fixture),
		}
		for _, session := range probe.sessions {
			report.ActiveCalls += session.runner.activeCallCount()
			report.CommandCalls += session.runner.callCount()
			report.CommandFinished += session.runner.finishedCount()
			if session.runner.callCount() > 0 {
				report.CommandStarted++
			}
			report.CanceledCalls += session.runner.canceledCount()
		}
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Errorf("marshal forced concurrency cleanup report: %v", err)
			return
		}
		if err := os.WriteFile(reportPath, payload, 0o600); err != nil {
			t.Errorf("write forced concurrency cleanup report: %v", err)
		}
	})

	fixture = newConcurrencySharedProcessFixture(t)
	fixture.start(t)
	first := fixture.openCase(t, "CC-14-A", 1, concurrencyRunnerHold, "cc14-A", "", 0)
	second := fixture.openCase(t, "CC-14-B", 1, concurrencyRunnerHold, "cc14-B", "", 0)
	probe = &concurrencyForcedCleanupProbe{fixture: fixture, sessions: []*concurrencySession{first, second}}
	probe.streams = append(probe.streams,
		support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(fixture.baseURL, first.id)),
		support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(fixture.baseURL, second.id)),
	)
	t.Cleanup(func() {
		for _, stream := range probe.streams {
			stream.Close()
			stream.WaitClosed(concurrencySharedProcessTimeout)
			probe.streamsClosed++
		}
	})
	submitConcurrencyWork(t, first, first.marker)
	submitConcurrencyWork(t, second, second.marker)
	first.runner.waitStarted(t, concurrencySharedProcessTimeout)
	second.runner.waitStarted(t, concurrencySharedProcessTimeout)
	t.Fatal("intentional concurrency cleanup assertion after acquiring process, sessions, streams, routes, calls, and paths")
}

func (fixture *concurrencySharedProcessFixture) sessionIDs() ([]string, []string) {
	fixture.sessionsMu.Lock()
	defer fixture.sessionsMu.Unlock()
	opened := make([]string, 0, len(fixture.opened))
	for sessionID := range fixture.opened {
		opened = append(opened, sessionID)
	}
	deleted := make([]string, 0, len(fixture.closed))
	for sessionID := range fixture.closed {
		deleted = append(deleted, sessionID)
	}
	return opened, deleted
}

func channelClosed(channel <-chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}

func concurrencyOwnedRootsAbsent(fixture *concurrencySharedProcessFixture) bool {
	fixture.sessionsMu.Lock()
	paths := append([]string(nil), fixture.ownedDirs...)
	fixture.sessionsMu.Unlock()
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func sameConcurrencyStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
}

var _ platformprocess.CommandRunner = (*concurrencyCommandRouter)(nil)
