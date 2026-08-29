package cross

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCrossEmptyFactorySessionHasZeroWorkAndLeavesHostUsable(t *testing.T) {
	// CASE-X-003: opening a valid Factory without submitting Work must expose
	// zero session-scoped Work counts while the shared host remains usable.
	fixture := sharedCrossProcess(t)
	session := openSharedCrossLiveSession(t)
	live := readLiveSession(t, fixture.baseURL, session.sessionID)
	counts := live.Runtime.Progress.Categories
	if counts.Initial != 0 || counts.Processing != 0 || counts.Terminal != 0 || counts.Failed != 0 {
		t.Fatalf("empty Factory session categories = %#v, want all zero", counts)
	}
	status := readLiveSessionStatus(t, fixture.baseURL, session.sessionID)
	if status.Categories != counts {
		t.Fatalf("empty Factory session status categories = %#v, want %#v", status.Categories, counts)
	}
	hostStatus := support.GetJSON[factoryapi.StatusResponse](t, fixture.baseURL+"/status")
	if strings.TrimSpace(hostStatus.RuntimeStatus) == "" {
		t.Fatal("shared host status runtimeStatus is empty after empty-session read")
	}
	for _, event := range support.GetFactoryEventsForSessionAt(t, fixture.baseURL, session.sessionID) {
		if event.Context.SessionId != nil && *event.Context.SessionId != session.sessionID {
			t.Fatalf("empty Factory event sessionId = %q, want %q", *event.Context.SessionId, session.sessionID)
		}
	}
}

func TestCrossMalformedFactoryOpenReturnsValidationAndNoSessionRoute(t *testing.T) {
	// CASE-X-004: malformed explicit opens stop at validation and do not create
	// a live session, route, or session event stream.
	fixture := sharedCrossProcess(t)
	dir, err := os.MkdirTemp(filepath.Join(fixture.rootDir, "scenarios"), "malformed-")
	if err != nil {
		t.Fatalf("create malformed Factory directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{"name":"malformed","workTypes":[`), 0o644); err != nil {
		t.Fatalf("write malformed Factory config: %v", err)
	}
	beforeRoutes := fixture.router.routeCount()
	statusCode, body := postCrossOpenRequest(t, fixture.baseURL, dir)
	if statusCode != http.StatusBadRequest {
		t.Fatalf("malformed Factory open status = %d, want 400; body=%s", statusCode, body)
	}
	diagnostic := strings.ToLower(string(body))
	if !strings.Contains(diagnostic, "factory") && !strings.Contains(diagnostic, "invalid") && !strings.Contains(diagnostic, "validation") {
		t.Fatalf("malformed Factory open body = %q, want validation diagnostic", body)
	}
	if got := fixture.router.routeCount(); got != beforeRoutes {
		t.Fatalf("routes after malformed Factory open = %d, want unchanged %d", got, beforeRoutes)
	}
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, fixture.baseURL+"/factory-sessions?scope=live")
	for _, summary := range listed.Sessions {
		if filepath.Clean(summary.FolderPath) == filepath.Clean(dir) {
			t.Fatalf("malformed Factory appeared in live session list: %#v", summary)
		}
	}
}

func TestCrossExplicitSessionsKeepKeyedLifecycleAndStatusState(t *testing.T) {
	// CASE-X-005: two independently opened live sessions can be controlled and
	// observed concurrently without sharing lifecycle state or event identity.
	fixture := sharedCrossProcess(t)
	first := openSharedCrossLiveSession(t)
	second := openSharedCrossLiveSession(t)
	if first.sessionID == second.sessionID {
		t.Fatalf("explicit session ids must differ: first=%q second=%q", first.sessionID, second.sessionID)
	}
	if got := fixture.router.routeCount(); got != 2 {
		t.Fatalf("registered scenario routes = %d, want 2", got)
	}
	applyAcceptedLifecycleControl(t, fixture.baseURL, first.sessionID, factoryapi.FactorySessionLifecycleControlKindPause)
	assertLiveSessionLifecycleControlStatus(t, fixture.baseURL, first.sessionID, factoryapi.FactorySessionDurableLifecycleStatusPaused)
	secondLive := readLiveSession(t, fixture.baseURL, second.sessionID)
	if secondLive.Runtime.LifecycleControlStatus != nil &&
		*secondLive.Runtime.LifecycleControlStatus == factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("second session inherited first session pause: %#v", secondLive.Runtime)
	}

	type statusResult struct {
		sessionID string
		status    factoryapi.StatusResponse
		err       error
	}
	results := make(chan statusResult, 2)
	var wg sync.WaitGroup
	for _, sessionID := range []string{first.sessionID, second.sessionID} {
		sessionID := sessionID
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, err := getCrossSessionStatus(fixture.baseURL, sessionID)
			results <- statusResult{sessionID: sessionID, status: status, err: err}
		}()
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent status read for %s: %v", result.sessionID, result.err)
		}
		if result.status.RuntimeStatus == "" {
			t.Fatalf("concurrent status read for %s has empty runtimeStatus", result.sessionID)
		}
		for _, event := range support.GetFactoryEventsForSessionAt(t, fixture.baseURL, result.sessionID) {
			if event.Context.SessionId != nil && *event.Context.SessionId != result.sessionID {
				t.Fatalf("session %s observed event for %q", result.sessionID, *event.Context.SessionId)
			}
		}
	}
	applyAcceptedLifecycleControl(t, fixture.baseURL, first.sessionID, factoryapi.FactorySessionLifecycleControlKindResume)
}

func TestCrossCancelGatedJavaScriptSessionPreservesAnotherSession(t *testing.T) {
	// CASE-X-006: cancel the real busy-loop JavaScript session through its
	// public control endpoint and prove a sibling live session remains usable.
	fixture := sharedCrossProcess(t)
	javascript := startSharedCrossJavaScriptSession(t)
	waitForDurableSessionStatus(t, fixture.baseURL, javascript.sessionID, factoryapi.FactorySessionDurableLifecycleStatusRunning, sessionCompatTimeout)
	petri := openSharedCrossLiveSession(t)
	response := applyAcceptedLifecycleControl(t, fixture.baseURL, javascript.sessionID, factoryapi.FactorySessionLifecycleControlKindCancel, fixture.nextRequestID())
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceling && response.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf("cancel response status = %q, want CANCELING or CANCELED", response.Status)
	}
	waitForDurableSessionTerminal(t, fixture.baseURL, javascript.sessionID, sessionCompatTimeout)
	canceled := readDurableSession(t, fixture.baseURL, javascript.sessionID)
	if canceled.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf("canceled JavaScript session status = %q, want CANCELED", canceled.Status)
	}
	if got := readLiveSessionStatus(t, fixture.baseURL, petri.sessionID); strings.TrimSpace(got.RuntimeStatus) == "" {
		t.Fatalf("sibling Petri session status after JavaScript cancellation = %#v", got)
	}
}

func TestCrossLifecycleRequestIDIsIdempotent(t *testing.T) {
	// CASE-X-007: replaying one lifecycle request ID returns the prior outcome
	// and does not append a second lifecycle mutation to the event ledger.
	fixture := sharedCrossProcess(t)
	javascript := startSharedCrossJavaScriptSession(t)
	waitForDurableSessionStatus(t, fixture.baseURL, javascript.sessionID, factoryapi.FactorySessionDurableLifecycleStatusRunning, sessionCompatTimeout)
	requestID := fixture.nextRequestID()
	first := applyAcceptedLifecycleControl(t, fixture.baseURL, javascript.sessionID, factoryapi.FactorySessionLifecycleControlKindPause, requestID)
	beforeReplay := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, javascript.sessionID)
	second := applyAcceptedLifecycleControl(t, fixture.baseURL, javascript.sessionID, factoryapi.FactorySessionLifecycleControlKindPause, requestID)
	afterReplay := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, javascript.sessionID)
	if first.Operation != second.Operation || first.Outcome != second.Outcome || first.SessionId != second.SessionId || first.Status != second.Status {
		t.Fatalf("idempotent lifecycle responses differ: first=%#v second=%#v", first, second)
	}
	if len(afterReplay) != len(beforeReplay) {
		t.Fatalf("event count after repeated lifecycle request = %d, want unchanged %d", len(afterReplay), len(beforeReplay))
	}
	assertNonDecreasingSessionSequences(t, javascript.sessionID, afterReplay)
	applyAcceptedLifecycleControl(t, fixture.baseURL, javascript.sessionID, factoryapi.FactorySessionLifecycleControlKindResume, fixture.nextRequestID())
}

func TestCrossCleanupReleasesEarlyExitSessionAndAllowsReuse(t *testing.T) {
	// CASE-X-008: t.Cleanup owns the early-return path, after which the same
	// process, API listener, and keyed route registry can serve another session.
	fixture := sharedCrossProcess(t)
	t.Run("early-return", func(t *testing.T) {
		session := openSharedCrossLiveSession(t)
		_ = readLiveSession(t, fixture.baseURL, session.sessionID)
		return
	})
	if got := fixture.router.routeCount(); got != 0 {
		t.Fatalf("routes after early-return cleanup = %d, want 0", got)
	}
	t.Run("reuse", func(t *testing.T) {
		session := openSharedCrossLiveSession(t)
		stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(fixture.baseURL, session.sessionID))
		stream.Close()
		session.close(t)
	})
	if got := fixture.router.routeCount(); got != 0 {
		t.Fatalf("routes after reuse cleanup = %d, want 0", got)
	}
}

func postCrossOpenRequest(t *testing.T, baseURL, folderPath string) (int, []byte) {
	t.Helper()
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: folderPath})
	if err != nil {
		t.Fatalf("marshal malformed open request: %v", err)
	}
	response, err := http.Post(strings.TrimSuffix(baseURL, "/")+"/factory-sessions", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST malformed Factory open: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read malformed Factory open response: %v", err)
	}
	return response.StatusCode, body
}

func getCrossSessionStatus(baseURL, sessionID string) (factoryapi.StatusResponse, error) {
	response, err := http.Get(strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/status")
	if err != nil {
		return factoryapi.StatusResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.StatusResponse{}, fmt.Errorf("status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var status factoryapi.StatusResponse
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return factoryapi.StatusResponse{}, err
	}
	return status, nil
}

func assertNonDecreasingSessionSequences(t *testing.T, sessionID string, events []factoryapi.FactoryEvent) {
	t.Helper()
	previous := -1
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("event sequence assertion for %s saw event for %q", sessionID, *event.Context.SessionId)
		}
		if event.Context.SessionSequence == nil {
			continue
		}
		if *event.Context.SessionSequence < previous {
			t.Fatalf("session %s event sessionSequence = %d after %d, want non-decreasing", sessionID, *event.Context.SessionSequence, previous)
		}
		previous = *event.Context.SessionSequence
	}
}
