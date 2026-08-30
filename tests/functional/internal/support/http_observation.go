package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const sessionStopObservationTimeout = 10 * time.Second

const (
	statusObservationInitialRetry = 5 * time.Millisecond
	statusObservationMaxRetry     = 250 * time.Millisecond
)

func GetDefaultSession(t testing.TB, baseURL string) factoryapi.FactorySession {
	t.Helper()
	response := GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+factorysessions.DefaultSessionID,
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode default live Factory Session: %v", err)
	}
	return session
}

// GetJSON reads one successful public HTTP response into its generated
// contract type.
func GetJSON[T any](t testing.TB, endpoint string) T {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(payload)))
	}
	var result T
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode GET %s: %v", endpoint, err)
	}
	return result
}

// WaitForStatus polls the public status contract until accept succeeds.
func WaitForStatus(
	t testing.TB,
	baseURL string,
	timeout time.Duration,
	accept func(factoryapi.StatusResponse) bool,
) factoryapi.StatusResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/status"
	status, err := waitForStatusAt(endpoint, timeout, accept)
	if err != nil {
		t.Fatalf("timed out waiting for status at %s: %v", endpoint, err)
	}
	return status
}

func WaitForRuntimeIdle(t testing.TB, baseURL string, timeout time.Duration) factoryapi.StatusResponse {
	t.Helper()
	return WaitForStatus(t, baseURL, timeout, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus == string(interfaces.RuntimeStatusIdle)
	})
}

// WaitForTerminalStatus is retained for excluded provider, provider-session,
// and worker lanes whose continuous hosts expose completion through the
// status projection while the host remains live. Owned standalone paths use
// the canonical Factory Event observer instead.
func WaitForTerminalStatus(
	t testing.TB,
	baseURL string,
	timeout time.Duration,
) factoryapi.StatusResponse {
	t.Helper()
	return WaitForSessionTerminalStatus(t, baseURL, factorysessions.DefaultSessionID, timeout)
}

func ListDefaultSessionWork(t testing.TB, baseURL string) factoryapi.ListWorkResponse {
	t.Helper()
	return GetJSON[factoryapi.ListWorkResponse](t, DefaultSessionWorkURL(baseURL, "/work"))
}

// ListDefaultSessionWorkerSessions reads the public Worker Session
// observation projection correlated with one Work identity.
func ListDefaultSessionWorkerSessions(t testing.TB, baseURL, workID string) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	if strings.TrimSpace(workID) == "" {
		t.Fatal("workID is empty")
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + factorysessions.DefaultSessionID + "/worker-sessions?workId=" + url.QueryEscape(workID)
	return GetJSON[factoryapi.ListWorkerSessionsResponse](t, endpoint)
}

// GetDefaultSessionWorkByID reads one public Work item through the same
// session-scoped GET /work/{id} surface customers use for detail reads.
func GetDefaultSessionWorkByID(t testing.TB, baseURL, workID string) factoryapi.Work {
	t.Helper()
	if strings.TrimSpace(workID) == "" {
		t.Fatal("workID is empty")
	}
	return GetJSON[factoryapi.Work](
		t,
		DefaultSessionWorkURL(baseURL, "/work/"+url.PathEscape(workID)),
	)
}

// UpsertDefaultSessionWorkRequest submits a canonical batch through the same
// public session-scoped HTTP edge used by customers.
func UpsertDefaultSessionWorkRequest(
	t testing.TB,
	baseURL string,
	request factoryapi.WorkRequest,
) factoryapi.UpsertWorkRequestResponse {
	t.Helper()

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal Work request: %v", err)
	}
	endpoint := DefaultSessionWorkURL(
		baseURL,
		"/work-requests/"+url.PathEscape(request.RequestId),
	)
	httpRequest, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build PUT %s: %v", endpoint, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("PUT %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("PUT %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode PUT %s: %v", endpoint, err)
	}
	return result
}

// SubmitDefaultSessionWork submits the public single-Work compatibility shape.
func SubmitDefaultSessionWork(
	t testing.TB,
	baseURL string,
	request factoryapi.SubmitWorkRequest,
) factoryapi.SubmitWorkResponse {
	t.Helper()

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal submit Work request: %v", err)
	}
	endpoint := DefaultSessionWorkURL(baseURL, "/work")
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	return result
}

// OpenFactorySessionAt opens one live Factory Session through the public API.
func OpenFactorySessionAt(
	t testing.TB,
	baseURL string,
	folderPath string,
) factoryapi.OpenFactorySessionResponse {
	t.Helper()

	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: folderPath})
	if err != nil {
		t.Fatalf("marshal open Factory Session request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(response.Body).Decode(&opened); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("open Factory Session response = %#v, want session id", opened)
	}
	return opened
}

// TerminateFactorySessionAt requests forced termination of one live Factory
// Session through the public API. A terminal-session conflict is already a
// safe state for callers that are preparing the session for deletion.
func TerminateFactorySessionAt(t testing.TB, baseURL, sessionID string) {
	t.Helper()

	payload, err := json.Marshal(factoryapi.FactorySessionLifecycleControlRequest{})
	if err != nil {
		t.Fatalf("marshal terminate Factory Session request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/terminate"
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build terminate Factory Session request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST terminate Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return
	}
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode == http.StatusConflict && strings.Contains(string(body), `"outcome":"TERMINAL_SESSION"`) {
		return
	}
	t.Fatalf(
		"POST terminate Factory Session %q status = %d, want successful or terminal outcome: %s",
		sessionID,
		response.StatusCode,
		strings.TrimSpace(string(body)),
	)
}

// CloseFactorySessionAt terminates, then deletes, one live non-default
// Factory Session through the public API.
func CloseFactorySessionAt(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	TerminateFactorySessionAt(t, baseURL, sessionID)
	WaitForSessionStopped(t, baseURL, sessionID, sessionStopObservationTimeout)

	request, err := http.NewRequest(
		http.MethodDelete,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+sessionID,
		nil,
	)
	if err != nil {
		t.Fatalf("build close Factory Session request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("DELETE Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"DELETE Factory Session %q status = %d, want 204: %s",
			sessionID,
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}
}

// WaitForSessionStopped observes the public status endpoint until a live
// Factory Session reports the stopped runtime state required by deletion.
func WaitForSessionStopped(t testing.TB, baseURL, sessionID string, timeout time.Duration) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/status"
	_, err := waitForStatusAt(endpoint, timeout, func(status factoryapi.StatusResponse) bool {
		switch status.RuntimeStatus {
		case string(interfaces.RuntimeStatusIdle), string(interfaces.RuntimeStatusFinished):
			return true
		default:
			return false
		}
	})
	if err != nil {
		t.Fatalf("timed out waiting for Factory Session %q to stop: %v", sessionID, err)
	}
}

// SubmitSessionWorkAt submits work to one Factory Session through the public API.
func SubmitSessionWorkAt(
	t testing.TB,
	baseURL string,
	sessionID string,
	request factoryapi.SubmitWorkRequest,
) factoryapi.SubmitWorkResponse {
	t.Helper()

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal submit Work request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/work"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	return result
}

// WaitForSessionTerminalStatus retains the pre-existing session-scoped status
// contract used by live/shared-session callers. Its adaptive retry is bounded
// and has no fixed polling interval or stable-window success condition.
func WaitForSessionTerminalStatus(
	t testing.TB,
	baseURL string,
	sessionID string,
	timeout time.Duration,
) factoryapi.StatusResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/status"
	status, err := waitForStatusAt(endpoint, timeout, terminalSessionStatusIsComplete)
	if err != nil {
		t.Fatalf("timed out waiting for terminal Factory Session %q at %s: %v", sessionID, endpoint, err)
	}
	return status
}

var terminalSessionStatusIsComplete = func(status factoryapi.StatusResponse) bool {
	completed := status.Categories.Terminal + status.Categories.Failed
	if completed == 0 || status.Categories.Initial != 0 || status.Categories.Processing != 0 {
		return false
	}
	switch status.RuntimeStatus {
	case string(interfaces.RuntimeStatusIdle), string(interfaces.RuntimeStatusFinished):
		return true
	default:
		return false
	}
}

func waitForStatusAt(
	endpoint string,
	timeout time.Duration,
	accept func(factoryapi.StatusResponse) bool,
) (factoryapi.StatusResponse, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	// This generic readiness/stop helper is not a terminal completion barrier.
	// Use bounded adaptive retries for those non-terminal predicates; terminal
	// completion is synchronized through canonical Factory Events above.
	retryDelay := statusObservationInitialRetry
	var last factoryapi.StatusResponse
	var lastErr error
	for {
		response, err := http.Get(endpoint)
		if err == nil {
			func() {
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					lastErr = fmt.Errorf("status = %d", response.StatusCode)
					return
				}
				if err := json.NewDecoder(response.Body).Decode(&last); err != nil {
					lastErr = err
					return
				}
				lastErr = nil
			}()
			if lastErr == nil && accept(last) {
				return last, nil
			}
		} else {
			lastErr = err
		}
		retry := time.NewTimer(retryDelay)
		select {
		case <-retry.C:
			if retryDelay < statusObservationMaxRetry {
				retryDelay *= 2
				if retryDelay > statusObservationMaxRetry {
					retryDelay = statusObservationMaxRetry
				}
			}
		case <-deadline.C:
			if !retry.Stop() {
				select {
				case <-retry.C:
				default:
				}
			}
			return last, fmt.Errorf("last=%#v error=%v", last, lastErr)
		}
	}
}
