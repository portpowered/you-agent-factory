package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIConcurrentSessionRequestsRemainIsolated proves overlapping Factory Session
// HTTP reads stay correlated to their originating session identity while unrelated
// in-flight invocations hold concurrent server work.
func TestAPIConcurrentSessionRequestsRemainIsolated(t *testing.T) {
	t.Parallel()
	dir := scaffoldConcurrentRequestsFactory(t)
	blocking := newBlockingInvocationRunner()
	server := c06SharedHTTPServer(t).newScenario(t, "concurrent-session-isolation", dir)
	server.registerRunner(t, []string{dir}, blocking)

	firstSessionID := server.openSession(t, dir)
	secondSessionID := server.openSession(t, dir)
	if firstSessionID == secondSessionID {
		t.Fatalf("Factory Session id %q was reused", firstSessionID)
	}
	sessionIDs := []string{firstSessionID, secondSessionID}

	type invocationHandle struct {
		cancel context.CancelFunc
		done   chan error
	}
	handles := make([]invocationHandle, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func(sessionID string) {
			done <- postBlockingInvocation(ctx, server.URL(), sessionID, blocking)
		}(sessionID)
		handles = append(handles, invocationHandle{cancel: cancel, done: done})
	}

	for range sessionIDs {
		select {
		case <-blocking.started:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for overlapping invocation to enter flight")
		}
	}

	var wg sync.WaitGroup
	var mismatchMu sync.Mutex
	mismatches := make([]string, 0)
	recordMismatch := func(message string) {
		mismatchMu.Lock()
		mismatches = append(mismatches, message)
		mismatchMu.Unlock()
	}
	for round := 0; round < 10; round++ {
		for _, expectedID := range sessionIDs {
			wg.Add(1)
			go func(expectedID string) {
				defer wg.Done()
				session, err := fetchFactorySession(server.URL(), expectedID)
				if err != nil {
					recordMismatch(expectedID + " session GET: " + err.Error())
					return
				}
				if session.Id != expectedID {
					recordMismatch(expectedID + " returned " + session.Id)
				}
				status, err := fetchFactorySessionStatus(server.URL(), expectedID)
				if err != nil {
					recordMismatch(expectedID + " status GET: " + err.Error())
					return
				}
				if status.RuntimeStatus == "" {
					recordMismatch(expectedID + " status missing runtimeStatus")
				}
			}(expectedID)
		}
	}
	wg.Wait()
	if len(mismatches) > 0 {
		t.Fatalf("concurrent session observations crossed identities: %v", mismatches)
	}

	for _, handle := range handles {
		handle.cancel()
	}
	for _, handle := range handles {
		select {
		case err := <-handle.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("blocking invocation after cancel: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for cancelled invocation to return")
		}
	}
}

// TestAPICancelledRequestDoesNotCancelUnrelatedSession proves aborting one in-flight
// Factory Session HTTP request leaves other sessions queryable with intact identity and status.
func TestAPICancelledRequestDoesNotCancelUnrelatedSession(t *testing.T) {
	t.Parallel()
	dir := scaffoldConcurrentRequestsFactory(t)
	blocking := newBlockingInvocationRunner()
	server := c06SharedHTTPServer(t).newScenario(t, "cancelled-session-isolation", dir)
	server.registerRunner(t, []string{dir}, blocking)

	cancelTargetSessionID := server.openSession(t, dir)
	unrelatedSessionID := server.openSession(t, dir)
	if cancelTargetSessionID == unrelatedSessionID {
		t.Fatalf("cancel target session id %q equals unrelated session id %q", cancelTargetSessionID, unrelatedSessionID)
	}

	unrelatedBefore := getFactorySession(t, server.URL(), unrelatedSessionID)
	unrelatedStatusBefore := getFactorySessionStatus(t, server.URL(), unrelatedSessionID)
	if unrelatedBefore.Id != unrelatedSessionID {
		t.Fatalf("unrelated session id before cancel = %q, want %q", unrelatedBefore.Id, unrelatedSessionID)
	}
	if unrelatedStatusBefore.RuntimeStatus == "" {
		t.Fatalf("unrelated session status before cancel missing runtimeStatus")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelledDone := make(chan error, 1)
	go func() {
		cancelledDone <- postBlockingInvocation(ctx, server.URL(), cancelTargetSessionID, blocking)
	}()

	select {
	case <-blocking.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for cancellable invocation to enter flight")
	}

	cancel()

	select {
	case err := <-cancelledDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled invocation error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for cancelled invocation to return")
	}

	unrelatedAfter := getFactorySession(t, server.URL(), unrelatedSessionID)
	unrelatedStatusAfter := getFactorySessionStatus(t, server.URL(), unrelatedSessionID)
	if unrelatedAfter.Id != unrelatedSessionID {
		t.Fatalf("unrelated session id after cancel = %q, want %q", unrelatedAfter.Id, unrelatedSessionID)
	}
	if unrelatedStatusAfter.RuntimeStatus == "" {
		t.Fatalf("unrelated session status after cancel missing runtimeStatus")
	}
	if unrelatedStatusAfter.RuntimeStatus != unrelatedStatusBefore.RuntimeStatus {
		t.Fatalf(
			"unrelated session runtimeStatus after cancel = %q, want %q",
			unrelatedStatusAfter.RuntimeStatus,
			unrelatedStatusBefore.RuntimeStatus,
		)
	}
}

func scaffoldConcurrentRequestsFactory(t *testing.T) string {
	t.Helper()

	cfg := concurrentRequestsTestConfig()
	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func concurrentRequestsTestConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func getFactorySession(t *testing.T, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()

	session, err := fetchFactorySession(baseURL, sessionID)
	if err != nil {
		t.Fatalf("GET factory session %s: %v", sessionID, err)
	}
	return session
}

func getFactorySessionStatus(t *testing.T, baseURL, sessionID string) factoryapi.StatusResponse {
	t.Helper()

	status, err := fetchFactorySessionStatus(baseURL, sessionID)
	if err != nil {
		t.Fatalf("GET factory session status %s: %v", sessionID, err)
	}
	return status
}

func fetchFactorySession(baseURL, sessionID string) (factoryapi.FactorySession, error) {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID
	response, err := http.Get(endpoint)
	if err != nil {
		return factoryapi.FactorySession{}, fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		return factoryapi.FactorySession{}, fmt.Errorf(
			"GET %s status = %d: %s",
			endpoint,
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}
	var body factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return factoryapi.FactorySession{}, fmt.Errorf("decode GET %s: %w", endpoint, err)
	}
	session, err := body.AsFactorySession()
	if err != nil {
		return factoryapi.FactorySession{}, fmt.Errorf("decode factory session %s: %w", sessionID, err)
	}
	return session, nil
}

func fetchFactorySessionStatus(baseURL, sessionID string) (factoryapi.StatusResponse, error) {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/status"
	response, err := http.Get(endpoint)
	if err != nil {
		return factoryapi.StatusResponse{}, fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		return factoryapi.StatusResponse{}, fmt.Errorf(
			"GET %s status = %d: %s",
			endpoint,
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}
	var status factoryapi.StatusResponse
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return factoryapi.StatusResponse{}, fmt.Errorf("decode GET %s: %w", endpoint, err)
	}
	return status, nil
}

func postBlockingInvocation(
	ctx context.Context,
	baseURL string,
	sessionID string,
	blocking *blockingInvocationRunner,
) error {
	body, err := json.Marshal(textInvocationRequest("concurrent isolation invoke"))
	if err != nil {
		return err
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/invocations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}

func textInvocationRequest(text string) factoryapi.InvocationRequest {
	var part factoryapi.WorkContentPart
	_ = part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	})
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	return factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	}
}

type blockingInvocationRunner struct {
	started chan struct{}
}

func newBlockingInvocationRunner() *blockingInvocationRunner {
	return &blockingInvocationRunner{started: make(chan struct{}, 2)}
}

func (runner *blockingInvocationRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	select {
	case runner.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}
