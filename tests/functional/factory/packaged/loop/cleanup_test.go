package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type loopCleanupAction struct {
	name string
	fn   func() error
}

type loopCleanupStack struct {
	mu      sync.Mutex
	actions []loopCleanupAction
	once    sync.Once
	err     error
}

func newLoopCleanupStack() *loopCleanupStack {
	return &loopCleanupStack{}
}

func (stack *loopCleanupStack) add(name string, fn func() error) {
	if stack == nil || fn == nil {
		return
	}
	stack.mu.Lock()
	defer stack.mu.Unlock()
	stack.actions = append(stack.actions, loopCleanupAction{name: name, fn: fn})
}

func (stack *loopCleanupStack) run() error {
	if stack == nil {
		return nil
	}
	stack.once.Do(func() {
		stack.mu.Lock()
		actions := append([]loopCleanupAction(nil), stack.actions...)
		stack.mu.Unlock()

		var errs []error
		for index := len(actions) - 1; index >= 0; index-- {
			action := actions[index]
			if err := action.fn(); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", action.name, err))
			}
		}
		stack.mu.Lock()
		stack.err = errors.Join(errs...)
		stack.mu.Unlock()
	})
	stack.mu.Lock()
	defer stack.mu.Unlock()
	return stack.err
}

type loopRunnerReleaser interface {
	Release()
}

func releaseLoopRunner(runner platformprocess.CommandRunner) error {
	if runner == nil {
		return nil
	}
	releaser, ok := runner.(loopRunnerReleaser)
	if ok {
		releaser.Release()
	}
	return nil
}

func (scenario *loopScenario) removeRoot() error {
	if scenario == nil || strings.TrimSpace(scenario.rootDir) == "" {
		return nil
	}
	var errs []error
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove scenario root %q: %w", scenario.rootDir, err))
	}
	_, statErr := os.Stat(scenario.rootDir)
	switch {
	case errors.Is(statErr, os.ErrNotExist):
		scenario.rootRemoved = true
	case statErr == nil:
		errs = append(errs, fmt.Errorf("scenario root %q remains", scenario.rootDir))
	default:
		errs = append(errs, fmt.Errorf("stat scenario root %q: %w", scenario.rootDir, statErr))
	}
	if scenario.lifecycleTracked {
		if err := scenario.fixture.lifecycle.close(
			scenario.sessionID,
			scenario.sessionAbsent,
			scenario.rootRemoved,
		); err != nil {
			errs = append(errs, fmt.Errorf("record scenario cleanup: %w", err))
		}
	}
	return errors.Join(errs...)
}

func closeLoopSession(baseURL, sessionID string) (bool, error) {
	var errs []error
	if err := runLoopCleanupStep(func(ctx context.Context) error {
		return terminateLoopSession(ctx, baseURL, sessionID)
	}); err != nil {
		errs = append(errs, err)
	}
	if err := runLoopCleanupStep(func(ctx context.Context) error {
		return waitForLoopSessionStopped(ctx, baseURL, sessionID)
	}); err != nil {
		errs = append(errs, err)
	}
	if err := runLoopCleanupStep(func(ctx context.Context) error {
		return deleteLoopSession(ctx, baseURL, sessionID)
	}); err != nil {
		errs = append(errs, err)
	}
	absent := false
	if err := runLoopCleanupStep(func(ctx context.Context) error {
		var err error
		absent, err = observeLoopSessionAbsent(ctx, baseURL, sessionID)
		return err
	}); err != nil {
		errs = append(errs, err)
	}
	return absent, errors.Join(errs...)
}

func runLoopCleanupStep(fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), loopSharedFixtureTimeout)
	defer cancel()
	return fn(ctx)
}

func terminateLoopSession(ctx context.Context, baseURL, sessionID string) error {
	endpoint := loopSessionEndpoint(baseURL, sessionID, "/terminate")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("build terminate request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("terminate Factory Session: %w", err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return fmt.Errorf("read terminate response: %w", readErr)
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	if response.StatusCode == http.StatusNotFound ||
		(response.StatusCode == http.StatusConflict && strings.Contains(string(body), `"outcome":"TERMINAL_SESSION"`)) {
		return nil
	}
	return fmt.Errorf(
		"terminate Factory Session status = %d: %s",
		response.StatusCode,
		strings.TrimSpace(string(body)),
	)
}

func waitForLoopSessionStopped(ctx context.Context, baseURL, sessionID string) error {
	// The public status projection is the only package-visible stop observation;
	// the ticker is a bounded observation interval, not a readiness delay.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var (
		lastStatus factoryapi.StatusResponse
		lastErr    error
	)
	for {
		status, absent, err := readLoopSessionStatus(ctx, baseURL, sessionID)
		lastStatus = status
		lastErr = err
		if absent || (err == nil && loopSessionStopped(status)) {
			return nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return fmt.Errorf(
				"wait for Factory Session %q to stop; last status=%#v; last error=%v: %w",
				sessionID, lastStatus, lastErr, ctx.Err(),
			)
		}
	}
}

func readLoopSessionStatus(
	ctx context.Context,
	baseURL, sessionID string,
) (factoryapi.StatusResponse, bool, error) {
	endpoint := loopSessionEndpoint(baseURL, sessionID, "/status")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return factoryapi.StatusResponse{}, false, fmt.Errorf("build session status request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.StatusResponse{}, false, fmt.Errorf("read session status: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return factoryapi.StatusResponse{}, true, nil
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.StatusResponse{}, false, fmt.Errorf(
			"session status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)),
		)
	}
	var status factoryapi.StatusResponse
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return factoryapi.StatusResponse{}, false, fmt.Errorf("decode session status: %w", err)
	}
	return status, false, nil
}

func loopSessionStopped(status factoryapi.StatusResponse) bool {
	switch status.RuntimeStatus {
	case string(factorydefinitions.RuntimeStatusIdle), string(factorydefinitions.RuntimeStatusFinished):
		return true
	default:
		return false
	}
}

func deleteLoopSession(ctx context.Context, baseURL, sessionID string) error {
	endpoint := loopSessionEndpoint(baseURL, sessionID, "")
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build delete request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("delete Factory Session: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotFound {
		return nil
	}
	body, _ := io.ReadAll(response.Body)
	return fmt.Errorf(
		"delete Factory Session status = %d: %s",
		response.StatusCode,
		strings.TrimSpace(string(body)),
	)
}

func observeLoopSessionAbsent(ctx context.Context, baseURL, sessionID string) (bool, error) {
	endpoint := loopSessionEndpoint(baseURL, sessionID, "")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("build deleted-session request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false, fmt.Errorf("read deleted Factory Session: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return true, nil
	}
	body, _ := io.ReadAll(response.Body)
	return false, fmt.Errorf(
		"GET deleted Factory Session status = %d, want 404: %s",
		response.StatusCode,
		strings.TrimSpace(string(body)),
	)
}

func loopSessionEndpoint(baseURL, sessionID, suffix string) string {
	return strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) + suffix
}

func TestLoopCleanupStackRunsAllActionsOnceAndRetainsFailures(t *testing.T) {
	firstErr := errors.New("first cleanup failure")
	secondErr := errors.New("second cleanup failure")
	var (
		order []string
		calls = make(map[string]int)
	)
	stack := newLoopCleanupStack()
	stack.add("first", func() error {
		calls["first"]++
		order = append(order, "first")
		return firstErr
	})
	stack.add("second", func() error {
		calls["second"]++
		order = append(order, "second")
		return secondErr
	})
	stack.add("third", func() error {
		calls["third"]++
		order = append(order, "third")
		return nil
	})

	err := stack.run()
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("cleanup error = %v, want both cleanup failures", err)
	}
	if got, want := strings.Join(order, ","), "third,second,first"; got != want {
		t.Fatalf("cleanup order = %q, want %q", got, want)
	}
	if repeated := stack.run(); repeated == nil || !errors.Is(repeated, firstErr) || !errors.Is(repeated, secondErr) {
		t.Fatalf("repeated cleanup error = %v, want retained failures", repeated)
	}
	for name, count := range calls {
		if count != 1 {
			t.Fatalf("cleanup action %q calls = %d, want 1", name, count)
		}
	}
}

func TestBlockingLoopRunnerReleaseIsIdempotent(t *testing.T) {
	runner := newBlockingLoopRunner()
	result := make(chan error, 1)
	ctx, cancel := context.WithTimeout(t.Context(), loopSharedFixtureTimeout)
	defer cancel()
	go func() {
		_, err := runner.Run(context.Background(), platformprocess.CommandRequest{})
		result <- err
	}()
	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatalf("blocking runner did not start: %v", ctx.Err())
	}

	runner.Release()
	runner.Release()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("released runner error = %v, want nil", err)
		}
	case <-ctx.Done():
		t.Fatalf("idempotent runner release did not unblock Run: %v", ctx.Err())
	}
}
