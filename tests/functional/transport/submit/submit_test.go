package submit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const functionalBatch = `{
	"requestId": "functional-submit-batch",
	"type": "FACTORY_REQUEST_BATCH",
	"works": [
		{"name": "review", "workTypeName": "task", "payload": {"title": "Review"}}
	]
}`

// TestSubmitFamilyExecutesThroughRootBuiltProcess proves the public submit
// commands can run repeatedly through the one package-owned root process.
func TestSubmitFamilyExecutesThroughRootBuiltProcess(t *testing.T) {
	fixture := packageSubmitFixture

	t.Run("batch dry-run", func(t *testing.T) {
		result := fixture.execute(t, []string{
			"you", "--server", "http://127.0.0.1:1",
			"submit", "batch", "--dry-run", functionalBatch,
		}, context.Background(), "", true)
		if result.err != nil {
			t.Fatalf("batch dry-run error = %v", result.err)
		}
		for _, marker := range []string{
			"requestId: functional-submit-batch",
			"batchSource: inline",
			"dry-run: no request sent",
		} {
			if !strings.Contains(result.stdout, marker) {
				t.Fatalf("batch dry-run output omitted %q: %q", marker, result.stdout)
			}
		}
	})

	t.Run("unary named session", func(t *testing.T) {
		payloadRoot := fixture.tempDir(t)
		payloadPath := filepath.Join(payloadRoot, "request.md")
		if err := writeSubmitFile(payloadPath, "# Review\n\nCheck the release."); err != nil {
			t.Fatalf("write unary payload: %v", err)
		}
		server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			submitJSONResponse(w, http.StatusCreated, submitAcceptedResponse(
				"functional-unary-request", "functional-unary-trace", "functional-unary-work", "release-review", "task",
			))
		})

		result := fixture.execute(t, []string{
			"you", "--server", server.URL(), "--json",
			"submit", "--name", "release-review", "--work-type-name", "task",
			"--payload", payloadPath, "--session", "functional-session",
		}, context.Background(), "", true)
		if result.err != nil {
			t.Fatalf("unary submit error = %v", result.err)
		}
		requests := server.requestsSnapshot()
		if len(requests) != 1 || requests[0].Method != http.MethodPost || requests[0].Path != "/factory-sessions/functional-session/work" {
			t.Fatalf("unary requests = %#v", requests)
		}
		for _, marker := range []string{
			`"sessionId":"functional-session"`,
			`"name":"release-review"`,
			`"workTypeName":"task"`,
		} {
			if !strings.Contains(result.stdout, marker) {
				t.Fatalf("unary output omitted %q: %q", marker, result.stdout)
			}
		}
	})

	t.Run("unary JSON payload", func(t *testing.T) {
		payloadRoot := fixture.tempDir(t)
		payloadPath := filepath.Join(payloadRoot, "request.json")
		if err := writeSubmitFile(payloadPath, `{"title":"Review JSON"}`); err != nil {
			t.Fatalf("write JSON payload: %v", err)
		}
		server := newSubmitHTTPServer(t, fixture.ledger, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			submitJSONResponse(w, http.StatusCreated, submitAcceptedResponse(
				"functional-json-request", "functional-json-trace", "functional-json-work", "json-review", "task",
			))
		})

		result := fixture.execute(t, []string{
			"you", "--server", server.URL(), "--json",
			"submit", "--name", "json-review", "--work-type-name", "task",
			"--payload", payloadPath,
		}, context.Background(), "", true)
		if result.err != nil {
			t.Fatalf("unary JSON submit error = %v", result.err)
		}
		requests := server.requestsSnapshot()
		if len(requests) != 1 || requests[0].Method != http.MethodPost || requests[0].Path != "/factory-sessions/~default/work" {
			t.Fatalf("unary JSON requests = %#v", requests)
		}
		for _, marker := range []string{
			`"name":"json-review"`,
			`"workTypeName":"task"`,
		} {
			if !strings.Contains(result.stdout, marker) {
				t.Fatalf("unary JSON output omitted %q: %q", marker, result.stdout)
			}
		}
	})
}

// TestSubmitFamilyEnqueuesWorkBeforeDownstreamStructuredOutputFailure proves
// live submit admission remains visible when the real provider boundary later
// rejects the worker's structured output.
func TestSubmitFamilyEnqueuesWorkBeforeDownstreamStructuredOutputFailure(t *testing.T) {
	fixture := packageSubmitFixture
	factoryDir := fixture.tempDir(t)
	writeSubmitFactory(t, factoryDir)

	daemonInvocation := fixture.newInvocation(t,
		[]string{"you", "run", "--dir", factoryDir, "--continuously", "--with-server", "--quiet", "--no-record"},
		context.Background(), "", true, factoryDir, nil)
	_ = fixture.startInvocation(t, daemonInvocation)
	api := fixture.starter.next(t)
	baseURL := api.WaitForURL(t)

	opened := support.OpenFactorySessionAt(t, baseURL, factoryDir)
	sessionID := opened.Session.Id
	fixture.ledger.sessionStarted()
	sessionClosed := false
	t.Cleanup(func() {
		if sessionClosed {
			return
		}
		support.CloseFactorySessionAt(t, baseURL, sessionID)
		fixture.ledger.sessionClosed()
	})

	payloadRoot := fixture.tempDir(t)
	payloadPath := filepath.Join(payloadRoot, "request.txt")
	if err := writeSubmitFile(payloadPath, "execute live submit"); err != nil {
		t.Fatalf("write live payload: %v", err)
	}
	beforeCalls := fixture.runner.callCount()
	result := fixture.execute(t, []string{
		"you", "--server", baseURL,
		"submit", "--name", "live-submit", "--work-type-name", "task",
		"--payload", payloadPath, "--session", sessionID,
	}, context.Background(), "", true)
	if result.err != nil {
		t.Fatalf("live unary submit error = %v\nstdout=%q\nstderr=%q", result.err, result.stdout, result.stderr)
	}
	if !strings.Contains(result.stdout, "Submitted: live-submit (task)") {
		t.Fatalf("live unary output = %q", result.stdout)
	}

	status := waitForSubmitSessionFailure(t, baseURL, sessionID)
	if status.Categories.Failed != 1 || status.Categories.Terminal != 0 {
		t.Fatalf("explicit session status = %#v, want one failed Work and no completed Work", status)
	}
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, sessionWorkURL(baseURL, sessionID))
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed CLI-submitted work = %d, want 1: %#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed CLI-submitted work = %d, want 0: %#v", got, listed)
	}
	if got := fixture.runner.callCount() - beforeCalls; got != 1 {
		t.Fatalf("provider command calls for admitted Work = %d, want 1", got)
	}
	if fixture.runner.activeCount() != 0 {
		t.Fatalf("active provider commands = %d, want 0", fixture.runner.activeCount())
	}
	support.CloseFactorySessionAt(t, baseURL, sessionID)
	fixture.ledger.sessionClosed()
	sessionClosed = true
}

func writeSubmitFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

func sessionWorkURL(baseURL, sessionID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
}

func waitForSubmitSessionFailure(t testing.TB, baseURL, sessionID string) factoryapi.StatusResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/status"
	deadline := time.NewTimer(submitFixtureTimeout)
	defer deadline.Stop()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	var lastErr error
	for {
		request, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			t.Fatalf("build session status request: %v", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			var status factoryapi.StatusResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&status)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil {
				completed := status.Categories.Terminal + status.Categories.Failed
				if completed > 0 && status.Categories.Initial == 0 && status.Categories.Processing == 0 {
					return status
				}
				lastErr = fmt.Errorf("session status is not terminal: %#v", status)
			} else if decodeErr != nil {
				lastErr = decodeErr
			} else {
				lastErr = fmt.Errorf("status code %d", response.StatusCode)
			}
		} else {
			lastErr = err
		}

		select {
		case <-poll.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for failed submit session: %v", lastErr)
		}
	}
}
