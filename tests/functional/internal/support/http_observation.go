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
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.StatusResponse
	var lastErr error
	for {
		response, err := http.Get(strings.TrimSuffix(baseURL, "/") + "/status")
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
				return last
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for status at %s: last=%#v error=%v", baseURL, last, lastErr)
		}
	}
}

func WaitForRuntimeIdle(t testing.TB, baseURL string, timeout time.Duration) factoryapi.StatusResponse {
	t.Helper()
	return WaitForStatus(t, baseURL, timeout, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus == string(interfaces.RuntimeStatusIdle)
	})
}

// WaitForTerminalStatus requires the terminal token count to remain stable
// briefly so watcher-backed seed discovery cannot be mistaken for completion
// after only the first file is observed.
//
// The stable window also covers repeater CONTINUE handoffs: under coverage-shard
// or parallel load, public categories can briefly show Initial=0 and
// Processing=0 while a Work token is consumed but not yet marked in-flight.
// Requiring IDLE/FINISHED runtime status plus a longer stable window prevents
// those gaps from looking like completion.
func WaitForTerminalStatus(
	t testing.TB,
	baseURL string,
	timeout time.Duration,
) factoryapi.StatusResponse {
	t.Helper()
	const stableWindow = 300 * time.Millisecond
	var (
		stableTotal int
		stableSince time.Time
	)
	return WaitForStatus(t, baseURL, timeout, func(status factoryapi.StatusResponse) bool {
		completed := status.Categories.Terminal + status.Categories.Failed
		// TotalTokens includes resource tokens, while status categories describe
		// customer Work. Terminality therefore depends on the absence of
		// initial/processing Work, not equality with the aggregate token count.
		if completed == 0 || status.Categories.Initial != 0 || status.Categories.Processing != 0 {
			stableTotal = 0
			stableSince = time.Time{}
			return false
		}
		switch status.RuntimeStatus {
		case string(interfaces.RuntimeStatusIdle), string(interfaces.RuntimeStatusFinished):
		default:
			stableTotal = 0
			stableSince = time.Time{}
			return false
		}
		stableValue := status.TotalTokens + completed
		if stableTotal != stableValue {
			stableTotal = stableValue
			stableSince = time.Now()
			return false
		}
		return time.Since(stableSince) >= stableWindow
	})
}

func ListDefaultSessionWork(t testing.TB, baseURL string) factoryapi.ListWorkResponse {
	t.Helper()
	return GetJSON[factoryapi.ListWorkResponse](t, DefaultSessionWorkURL(baseURL, "/work"))
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
