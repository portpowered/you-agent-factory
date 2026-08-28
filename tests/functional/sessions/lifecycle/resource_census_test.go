package lifecycle_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const lifecycleDurableSessionPrefix = "dur-sess-"

// lifecycleResourceLedger is deliberately package-local. It makes cleanup
// ownership observable at the existing functional-test boundary without
// changing the shared functional-test support package or adding test cases to
// the package denominator.
type lifecycleResourceLedger struct {
	mu sync.Mutex

	nextInvocation int

	processBuilds map[string]int
	processCloses map[string]int

	listenerStarts int
	listenerCloses int

	invocations map[string]*lifecycleInvocationResource
	sessions    map[string]*lifecycleSessionResource
}

type lifecycleInvocationResource struct {
	id         string
	owner      string
	closeCount int
}

type lifecycleSessionResource struct {
	id               string
	owner            string
	folderPath       string
	durable          bool
	closeCount       int
	publicAbsent     bool
	terminalObserved bool
	pathRemoved      bool
}

func newLifecycleResourceLedger() *lifecycleResourceLedger {
	return &lifecycleResourceLedger{
		processBuilds: make(map[string]int),
		processCloses: make(map[string]int),
		invocations:   make(map[string]*lifecycleInvocationResource),
		sessions:      make(map[string]*lifecycleSessionResource),
	}
}

func (ledger *lifecycleResourceLedger) recordProcessBuild(role string) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processBuilds[role]++
}

func (ledger *lifecycleResourceLedger) closeProcess(role string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	builds := ledger.processBuilds[role]
	if builds == 0 {
		return fmt.Errorf("owner=%q resource=process role=%q close has no matching BuildProcess", role, role)
	}
	closes := ledger.processCloses[role]
	if closes >= builds {
		return fmt.Errorf("owner=%q resource=process role=%q close count=%d want exactly once", role, role, closes+1)
	}
	ledger.processCloses[role] = closes + 1
	return nil
}

func (ledger *lifecycleResourceLedger) recordListenerStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.listenerStarts++
}

func (ledger *lifecycleResourceLedger) recordListenerClose() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.listenerCloses++
}

func (ledger *lifecycleResourceLedger) beginInvocation(owner string) string {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.nextInvocation++
	if strings.TrimSpace(owner) == "" {
		owner = "unknown"
	}
	id := fmt.Sprintf("lifecycle-invocation-%d", ledger.nextInvocation)
	ledger.invocations[id] = &lifecycleInvocationResource{id: id, owner: owner}
	return id
}

func (ledger *lifecycleResourceLedger) closeInvocation(id string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	resource, ok := ledger.invocations[id]
	if !ok {
		return fmt.Errorf("owner=%q resource=invocation-stream id=%q close is unregistered", "lifecycle-ledger", id)
	}
	if resource.closeCount != 0 {
		return fmt.Errorf("owner=%q resource=invocation-stream id=%q close count=%d want exactly once", resource.owner, id, resource.closeCount+1)
	}
	resource.closeCount = 1
	return nil
}

func (ledger *lifecycleResourceLedger) registerSession(
	owner string,
	sessionID string,
	folderPath string,
) (bool, error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, fmt.Errorf("owner=%q resource=session id is empty", owner)
	}
	if existing, exists := ledger.sessions[sessionID]; exists {
		if existing.durable && existing.closeCount == 1 {
			return false, nil
		}
		return false, fmt.Errorf("owner=%q resource=session id=%q registered more than once", owner, sessionID)
	}
	if strings.TrimSpace(folderPath) != "" {
		if !filepath.IsAbs(folderPath) {
			return false, fmt.Errorf("owner=%q resource=temporary-factory path=%q is not absolute", owner, folderPath)
		}
		folderPath = filepath.Clean(folderPath)
	}
	ledger.sessions[sessionID] = &lifecycleSessionResource{
		id:         sessionID,
		owner:      owner,
		folderPath: folderPath,
		durable:    isLifecycleDurableSession(sessionID),
	}
	return true, nil
}

func (ledger *lifecycleResourceLedger) closeSession(
	sessionID string,
	publicAbsent bool,
	terminalObserved bool,
	pathRemoved bool,
) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	resource, ok := ledger.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return fmt.Errorf("owner=%q resource=session id=%q close is unregistered", "lifecycle-ledger", sessionID)
	}
	if resource.closeCount != 0 {
		return fmt.Errorf("owner=%q resource=session id=%q close count=%d want exactly once", resource.owner, resource.id, resource.closeCount+1)
	}
	resource.closeCount = 1
	resource.publicAbsent = publicAbsent
	resource.terminalObserved = terminalObserved
	resource.pathRemoved = pathRemoved
	return nil
}

func (ledger *lifecycleResourceLedger) sessionClosed(sessionID string) bool {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	resource, ok := ledger.sessions[strings.TrimSpace(sessionID)]
	return ok && resource.closeCount == 1
}

func (ledger *lifecycleResourceLedger) summary() string {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	processBuilds := 0
	processCloses := 0
	for _, count := range ledger.processBuilds {
		processBuilds += count
	}
	for _, count := range ledger.processCloses {
		processCloses += count
	}
	invocationsClosed := 0
	for _, resource := range ledger.invocations {
		if resource.closeCount == 1 {
			invocationsClosed++
		}
	}
	sessionsClosed := 0
	liveAbsent := 0
	durableTerminal := 0
	pathsRemoved := 0
	for _, resource := range ledger.sessions {
		if resource.closeCount == 1 {
			sessionsClosed++
		}
		if resource.publicAbsent {
			liveAbsent++
		}
		if resource.terminalObserved {
			durableTerminal++
		}
		if resource.pathRemoved {
			pathsRemoved++
		}
	}
	return fmt.Sprintf(
		"process-builds=%d process-closes=%d listener-starts=%d listener-closes=%d invocation-streams-opened=%d invocation-streams-closed=%d sessions-opened=%d sessions-closed=%d live-sessions-absent=%d durable-sessions-terminal=%d temporary-factory-paths-removed=%d",
		processBuilds,
		processCloses,
		ledger.listenerStarts,
		ledger.listenerCloses,
		len(ledger.invocations),
		invocationsClosed,
		len(ledger.sessions),
		sessionsClosed,
		liveAbsent,
		durableTerminal,
		pathsRemoved,
	)
}

func (ledger *lifecycleResourceLedger) validate(listenerClosed, rootRemoved bool) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	var validationErrors []error
	for _, role := range []string{"server", "client"} {
		builds := ledger.processBuilds[role]
		closes := ledger.processCloses[role]
		if builds != 1 {
			validationErrors = append(validationErrors,
				fmt.Errorf("owner=%q resource=process role=%q BuildProcess count=%d want exactly once", role, role, builds),
			)
		}
		if closes != 1 {
			validationErrors = append(validationErrors,
				fmt.Errorf("owner=%q resource=process role=%q Close count=%d want exactly once", role, role, closes),
			)
		}
	}
	for role, closes := range ledger.processCloses {
		if ledger.processBuilds[role] == 0 && closes != 0 {
			validationErrors = append(validationErrors,
				fmt.Errorf("owner=%q resource=process role=%q has %d close(s) without BuildProcess", role, role, closes),
			)
		}
	}
	if ledger.listenerStarts != 1 {
		validationErrors = append(validationErrors,
			fmt.Errorf("owner=%q resource=http-listener starts=%d want exactly once", "shared-lifecycle-fixture", ledger.listenerStarts),
		)
	}
	if ledger.listenerCloses != 1 || !listenerClosed {
		validationErrors = append(validationErrors,
			fmt.Errorf("owner=%q resource=http-listener closes=%d probe-closed=%t want one close and an unreachable listener", "shared-lifecycle-fixture", ledger.listenerCloses, listenerClosed),
		)
	}
	for _, resource := range ledger.invocations {
		if resource.closeCount != 1 {
			validationErrors = append(validationErrors,
				fmt.Errorf("owner=%q resource=invocation-stream id=%q close count=%d want exactly once", resource.owner, resource.id, resource.closeCount),
			)
		}
	}
	for _, resource := range ledger.sessions {
		if resource.closeCount != 1 {
			validationErrors = append(validationErrors,
				fmt.Errorf("owner=%q resource=session id=%q close count=%d want exactly once", resource.owner, resource.id, resource.closeCount),
			)
			continue
		}
		if resource.durable {
			if !resource.terminalObserved {
				validationErrors = append(validationErrors,
					fmt.Errorf("owner=%q resource=session id=%q durable terminal probe=false", resource.owner, resource.id),
				)
			}
		} else if !resource.publicAbsent {
			validationErrors = append(validationErrors,
				fmt.Errorf("owner=%q resource=session id=%q public absence probe=false", resource.owner, resource.id),
			)
		}
		if strings.TrimSpace(resource.folderPath) != "" && !resource.pathRemoved {
			validationErrors = append(validationErrors,
				fmt.Errorf("owner=%q resource=temporary-factory path=%q removed=false", resource.owner, resource.folderPath),
			)
		}
	}
	if !rootRemoved {
		validationErrors = append(validationErrors,
			fmt.Errorf("owner=%q resource=shared-fixture-root removed=false", "shared-lifecycle-fixture"),
		)
	}
	return errors.Join(validationErrors...)
}

func lifecycleLedgerForTest(t testing.TB) *lifecycleResourceLedger {
	t.Helper()
	if lifecycleFixture == nil || lifecycleFixture.ledger == nil {
		t.Fatalf("owner=%q resource=lifecycle-ledger is unavailable", t.Name())
	}
	return lifecycleFixture.ledger
}

func isLifecycleDurableSession(sessionID string) bool {
	return strings.HasPrefix(strings.TrimSpace(sessionID), lifecycleDurableSessionPrefix)
}

func removeLifecycleSessionPath(t testing.TB, folderPath string) bool {
	t.Helper()
	if strings.TrimSpace(folderPath) == "" {
		return true
	}
	if err := os.RemoveAll(folderPath); err != nil {
		t.Errorf("owner=%q resource=temporary-factory path=%q remove: %v", t.Name(), folderPath, err)
		return false
	}
	if _, err := os.Stat(folderPath); err == nil {
		t.Errorf("owner=%q resource=temporary-factory path=%q remains after remove", t.Name(), folderPath)
		return false
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("owner=%q resource=temporary-factory path=%q absence probe: %v", t.Name(), folderPath, err)
		return false
	}
	return true
}

func assertDurableSessionTerminal(t testing.TB, baseURL, sessionID string) bool {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	client := &http.Client{Timeout: lifecycleCleanupProbeTimeout}
	defer client.CloseIdleConnections()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Errorf("owner=%q resource=session id=%q durable terminal probe: %v", t.Name(), sessionID, err)
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Errorf("owner=%q resource=session id=%q durable terminal probe status=%d body=%q", t.Name(), sessionID, response.StatusCode, strings.TrimSpace(string(body)))
		return false
	}
	var envelope factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Errorf("owner=%q resource=session id=%q decode durable terminal probe: %v", t.Name(), sessionID, err)
		return false
	}
	durable, err := envelope.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Errorf("owner=%q resource=session id=%q decode durable read model: %v", t.Name(), sessionID, err)
		return false
	}
	switch durable.Status {
	case factoryapi.FactorySessionDurableLifecycleStatusCanceled,
		factoryapi.FactorySessionDurableLifecycleStatusFailed,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated,
		factoryapi.FactorySessionDurableLifecycleStatusTimedOut:
		return true
	default:
		t.Errorf("owner=%q resource=session id=%q durable status=%q is not terminal", t.Name(), sessionID, durable.Status)
		return false
	}
}
