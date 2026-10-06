package customer_journeys_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	policyFixtureTimeout       = 15 * time.Second
	policyBehaviorSessionCount = 3
)

var (
	policyFixtureMu     sync.Mutex
	sharedPolicyFixture *policyFixture
)

// The parent owns the process and home until all CLI policy journeys finish.
func initializePolicyFixture(t *testing.T) {
	sharedPolicyFixture = nil
	t.Cleanup(func() {
		if fixture := sharedPolicyFixture; fixture != nil {
			if err := fixture.shutdown(); err != nil {
				t.Errorf("close policy fixture: %v", err)
			}
		}
	})
}

func policyFixtureForTest(t *testing.T) *policyFixture {
	t.Helper()
	policyFixtureMu.Lock()
	defer policyFixtureMu.Unlock()
	if sharedPolicyFixture == nil {
		sharedPolicyFixture = newPolicyFixture(t)
	}
	return sharedPolicyFixture
}

type policyFixture struct {
	process        support.ApplicationProcess
	providerRunner *support.RecordingCommandRunner
	homeDir        string
	sessionMu      sync.Mutex
	sessions       map[string]policySession
	closed         map[string]struct{}
}

type policySession struct {
	requestID string
	rootDir   string
	homeDir   string
}

func newPolicyFixture(t *testing.T) *policyFixture {
	t.Helper()
	homeDir, err := os.MkdirTemp("", "you-functional-policy-home-")
	if err != nil {
		t.Fatalf("create policy home: %v", err)
	}
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		FactorySessionsWorkingDirectory:    platformfilesystem.Local{WorkingDirectory: homeDir},
		FactorySessionResolveHomeDirectory: func() (string, error) { return homeDir, nil },
		FactoryRuntimeWorkflowHome:         func() (string, error) { return homeDir, nil },
		ProviderCommandRunner:              runner,
	})
	if err != nil {
		_ = os.RemoveAll(homeDir)
		t.Fatalf("build policy process: %v", err)
	}
	return &policyFixture{
		process: process, providerRunner: runner, homeDir: homeDir,
		sessions: make(map[string]policySession, policyBehaviorSessionCount),
		closed:   make(map[string]struct{}, policyBehaviorSessionCount),
	}
}

func policyCustomerEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") || strings.EqualFold(name, runcli.ModelCacheDirEnvironment) {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir, runcli.ModelCacheDirEnvironment+"="+filepath.Join(homeDir, ".agent-factory", "models"))
}

func (fixture *policyFixture) trackInvocationSession(
	t testing.TB,
	result factoryapi.InvocationResponse,
	rootDir string,
	homeDir string,
) {
	t.Helper()
	if result.SessionId == nil || strings.TrimSpace(*result.SessionId) == "" {
		t.Fatalf("policy invocation result = %#v, want explicit Factory Session ID", result)
	}
	fixture.trackSession(t, *result.SessionId, result.RequestId, rootDir, homeDir)
}

func (fixture *policyFixture) trackSession(
	t testing.TB,
	sessionID string,
	requestID string,
	rootDir string,
	homeDir string,
) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("policy Factory Session ID is empty")
	}
	if strings.TrimSpace(rootDir) == "" {
		t.Fatal("policy Factory Session root is empty")
	}

	fixture.sessionMu.Lock()
	if _, exists := fixture.sessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("policy Factory Session ID %q was reused", sessionID)
	}
	for existingID, session := range fixture.sessions {
		if filepath.Clean(session.rootDir) == filepath.Clean(rootDir) {
			fixture.sessionMu.Unlock()
			t.Fatalf("policy Factory Session roots reused by %q and %q: %s", existingID, sessionID, rootDir)
		}
		if strings.TrimSpace(requestID) != "" && session.requestID == requestID {
			fixture.sessionMu.Unlock()
			t.Fatalf("policy request ID %q was reused", requestID)
		}
	}
	fixture.sessions[sessionID] = policySession{
		requestID: requestID,
		rootDir:   rootDir,
		homeDir:   homeDir,
	}
	fixture.sessionMu.Unlock()

	t.Cleanup(func() {
		fixture.closeSession(t, sessionID)
		fixture.markSessionClosed(sessionID)
	})
}

func (fixture *policyFixture) closeSession(t testing.TB, sessionID string) {
	t.Helper()

	fixture.sessionMu.Lock()
	session, ok := fixture.sessions[sessionID]
	fixture.sessionMu.Unlock()
	if !ok {
		t.Errorf("policy Factory Session %q was not tracked during cleanup", sessionID)
		return
	}

	// Completed local invocations release their invocation-local session service
	// before Process.Execute returns. The public control probe therefore accepts
	// the not-found terminal observation while still surfacing other errors.
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "session", "terminate", sessionID,
	})
	inputs.Input.Env = policyCustomerEnvironment(session.homeDir)
	inputs.Input.WorkingDirectory = session.rootDir
	err := fixture.process.Execute(inputs.Input)
	missing := fmt.Sprintf(`factory session %q not found`, sessionID)
	terminal := string(factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession)
	if err != nil && !strings.Contains(err.Error(), missing) && !strings.Contains(inputs.Stdout(), terminal) {
		t.Errorf(
			"Process.Execute(session terminate %q) error = %v\nstdout:\n%s\nstderr:\n%s",
			sessionID,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if err == nil {
		var response factoryapi.FactorySessionLifecycleControlResponse
		if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); decodeErr != nil {
			t.Errorf("decode local session terminate %q: %v\nstdout:\n%s", sessionID, decodeErr, inputs.Stdout())
		} else if response.SessionId != sessionID {
			t.Errorf("local session terminate id = %q, want %q", response.SessionId, sessionID)
		}
	}
}

func (fixture *policyFixture) markSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	if _, alreadyClosed := fixture.closed[sessionID]; alreadyClosed {
		fixture.sessionMu.Unlock()
		return
	}
	fixture.closed[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
}

func (fixture *policyFixture) trackedSessionCount() int {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	return len(fixture.sessions)
}

func (fixture *policyFixture) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), policyFixtureTimeout)
	defer cancel()
	closeErr := fixture.process.Close(ctx)
	fixture.sessionMu.Lock()
	tracked, closed := len(fixture.sessions), len(fixture.closed)
	fixture.sessionMu.Unlock()
	if tracked != closed {
		closeErr = errors.Join(closeErr, fmt.Errorf("policy sessions closed = %d/%d", closed, tracked))
	}
	return errors.Join(closeErr, os.RemoveAll(fixture.homeDir))
}
