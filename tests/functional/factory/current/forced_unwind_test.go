package current

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	currentFactoryForcedUnwindEnv       = "YOU_FUNCTIONAL_CURRENT_FACTORY_FORCED_UNWIND"
	currentFactoryForcedUnwindReportEnv = "YOU_FUNCTIONAL_CURRENT_FACTORY_FORCED_UNWIND_REPORT"
)

type currentFactoryForcedUnwindReport struct {
	ProcessInvocationStopped bool   `json:"process_invocation_stopped"`
	ProcessClosed            bool   `json:"process_closed"`
	ListenerURL              string `json:"listener_url,omitempty"`
	ListenerClosed           bool   `json:"listener_closed"`
	RootDir                  string `json:"root_dir,omitempty"`
	RootAbsent               bool   `json:"root_absent"`
	SessionID                string `json:"session_id,omitempty"`
	SessionDeleted           bool   `json:"session_deleted"`
}

var (
	currentFactoryForcedUnwindFixture *sharedCurrentFactoryAPI
	currentFactoryForcedUnwindSession *sharedCurrentFactorySession
)

// TestMain writes the forced-unwind observation after testing has run all
// t.Cleanup callbacks. The report is opt-in so ordinary package runs retain
// their existing output and exit behavior.
func TestMain(m *testing.M) {
	code := m.Run()
	if err := writeCurrentFactoryForcedUnwindReport(); err != nil {
		fmt.Fprintf(os.Stderr, "write Current Factory forced-unwind report: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// failCurrentFactoryForcedUnwindAfterAssertion deliberately fails after a
// real shared Current Factory server and named session have been exercised.
// It is enabled only by the one-shot child-process characterization command;
// the normal denominator therefore retains its existing top-level tests.
func failCurrentFactoryForcedUnwindAfterAssertion(
	t *testing.T,
	fixture *sharedCurrentFactoryAPI,
) {
	t.Helper()
	if os.Getenv(currentFactoryForcedUnwindEnv) != "1" {
		return
	}
	if fixture == nil {
		t.Fatal("Current Factory forced-unwind fixture is unavailable")
	}

	currentFactoryForcedUnwindFixture = fixture
	session := fixture.openSession(t, "forced-unwind")
	currentFactoryForcedUnwindSession = session
	current := getCurrentFactoryForSession(t, session.serverURL, session.id)
	if current.Name == "" {
		t.Fatal("forced-unwind Current Factory response has empty name")
	}

	t.Fatal("intentional Current Factory forced-unwind characterization failure")
}

func writeCurrentFactoryForcedUnwindReport() error {
	path := strings.TrimSpace(os.Getenv(currentFactoryForcedUnwindReportEnv))
	if path == "" {
		return nil
	}
	report := currentFactoryForcedUnwindReport{}
	if fixture := currentFactoryForcedUnwindFixture; fixture != nil {
		report.ListenerURL = fixture.server.URL()
		// FunctionalAPIServer.Done closes after Process.Execute returns, and
		// Process.Execute cannot return before the injected API starter returns
		// after closing its listener. Pairing it with the root Close result keeps
		// this observation deterministic without a network probe or polling.
		report.ListenerClosed = fixture.processClosed && currentFactoryChannelClosed(fixture.server.Done())
		report.RootDir = fixture.rootDir
		report.RootAbsent = currentFactoryPathAbsent(fixture.rootDir)
		report.ProcessInvocationStopped = currentFactoryChannelClosed(fixture.server.Done())
		report.ProcessClosed = fixture.processClosed
	}
	if session := currentFactoryForcedUnwindSession; session != nil {
		report.SessionID = session.id
		report.SessionDeleted = session.closed
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

func currentFactoryPathAbsent(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

func currentFactoryChannelClosed(channel <-chan struct{}) bool {
	if channel == nil {
		return false
	}
	select {
	case <-channel:
		return true
	default:
		return false
	}
}
