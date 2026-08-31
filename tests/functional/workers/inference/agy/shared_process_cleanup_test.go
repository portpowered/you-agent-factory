package agy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type agySharedScenarioRun struct {
	fixture   *agyProcessFixture
	sessionID string
	stream    *support.FactoryResponseEventStream

	closeOnce sync.Once
	closeErr  error

	sessionOnce     sync.Once
	sessionCloseErr error
	streamOnce      sync.Once
	streamState     sync.Mutex
	streamReadErr   error
	streamCloseErr  error
}

func (run *agySharedScenarioRun) close(t testing.TB) {
	t.Helper()
	if run == nil {
		return
	}
	if err := run.closeResources(); err != nil {
		t.Errorf("AGY scenario cleanup: %v", err)
	}
}

func (run *agySharedScenarioRun) closeResources() error {
	if run == nil {
		return nil
	}
	run.closeOnce.Do(func() {
		run.closeErr = run.closeResourcesOnce()
	})
	return run.closeErr
}

func (run *agySharedScenarioRun) closeResourcesOnce() error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), agySharedScenarioTimeout)
	defer cancel()
	var errs []error
	if err := run.closeSession(cleanupCtx); err != nil {
		errs = append(errs, err)
	}
	if err := run.closeStream(); err != nil {
		errs = append(errs, fmt.Errorf("close response stream for session %q: %w", run.sessionID, err))
	}
	if err := run.observedStreamError(); err != nil {
		errs = append(errs, fmt.Errorf("read response stream for session %q: %w", run.sessionID, err))
	}
	if strings.TrimSpace(run.sessionID) == "" {
		run.fixture.forgetRun(run.sessionID)
	}
	return errors.Join(errs...)
}

func (run *agySharedScenarioRun) closeSession(ctx context.Context) error {
	return run.closeSessionWith(ctx, closeAgyFactorySession)
}

func (run *agySharedScenarioRun) closeSessionKnownActive(ctx context.Context) error {
	return run.closeSessionWith(ctx, closeAgyActiveFactorySession)
}

func (run *agySharedScenarioRun) closeSessionWith(
	ctx context.Context,
	closeSession func(context.Context, string, string) error,
) error {
	if run == nil || strings.TrimSpace(run.sessionID) == "" {
		return nil
	}
	run.sessionOnce.Do(func() {
		var errs []error
		if err := closeSession(ctx, run.fixture.baseURL, run.sessionID); err != nil {
			errs = append(errs, fmt.Errorf("close Factory Session %q: %w", run.sessionID, err))
		} else {
			// A successful public DELETE (or an idempotent 404) is the deletion
			// witness. The following stream EOF check proves the session-owned
			// response stream was released without another serialized request.
			run.fixture.recordSessionDeleted(run.sessionID)
			run.fixture.forgetRun(run.sessionID)
		}
		run.sessionCloseErr = errors.Join(errs...)
	})
	return run.sessionCloseErr
}

func (run *agySharedScenarioRun) closeStream() error {
	if run == nil || run.stream == nil {
		return nil
	}
	run.streamOnce.Do(func() {
		run.stream.Close()
		result := run.stream.TryNextFrameResult(time.Nanosecond)
		run.fixture.recordStreamClosed()
		switch result.Outcome {
		case support.FactoryResponseEventStreamOutcomeEOF,
			support.FactoryResponseEventStreamOutcomeCanceled:
			return
		case support.FactoryResponseEventStreamOutcomeReadError:
			run.streamCloseErr = result.Err
		default:
			run.streamCloseErr = fmt.Errorf("response stream close outcome = %q", result.Outcome)
		}
	})
	return run.streamCloseErr
}

func (run *agySharedScenarioRun) recordStreamReadResult(result support.FactoryResponseEventStreamWaitResult) {
	if run == nil || result.Err == nil {
		return
	}
	run.streamState.Lock()
	defer run.streamState.Unlock()
	if run.streamReadErr == nil {
		run.streamReadErr = result.Err
	}
}

func (run *agySharedScenarioRun) observedStreamError() error {
	if run == nil {
		return nil
	}
	run.streamState.Lock()
	defer run.streamState.Unlock()
	return run.streamReadErr
}

func TestAgyCleanupPreservesAssertionAndCleanupErrors(t *testing.T) {
	t.Parallel()

	primaryErr := errors.New("golden assertion failed")
	cleanupErrs := []error{
		errors.New("session cleanup failed"),
		errors.New("daemon stop failed"),
		errors.New("process close failed"),
		errors.New("API shutdown wait failed"),
		errors.New("listener census failed"),
		errors.New("activity census failed"),
		errors.New("route release failed"),
		errors.New("route census failed"),
		errors.New("cleanup census failed"),
		errors.New("fixture root removal failed"),
	}
	state := struct {
		sessions, streams, activeCalls, routes int
		daemonOpen, processOpen, listenerOpen  bool
		rootExists                             bool
	}{
		sessions: 2, streams: 2, activeCalls: 1, routes: 2,
		daemonOpen: true, processOpen: true, listenerOpen: true, rootExists: true,
	}
	cleanupContext := func(index int, release func()) func(context.Context) error {
		return func(context.Context) error {
			release()
			return cleanupErrs[index]
		}
	}
	cleanupCheck := func(index int, release func()) func() error {
		return func() error {
			release()
			return cleanupErrs[index]
		}
	}

	err := cleanupAgyResources(context.Background(), primaryErr, agyCleanupOperations{
		closeSessions: cleanupContext(0, func() { state.sessions, state.streams = 0, 0 }),
		stopDaemon: cleanupContext(1, func() {
			state.daemonOpen = false
			state.activeCalls = 0
		}),
		closeProcess:  cleanupContext(2, func() { state.processOpen = false }),
		waitForAPI:    cleanupContext(3, func() { state.listenerOpen = false }),
		checkListener: cleanupCheck(4, func() { state.listenerOpen = false }),
		checkActivity: cleanupCheck(5, func() { state.activeCalls = 0 }),
		releaseRoutes: cleanupCheck(6, func() { state.routes = 0 }),
		checkRoutes:   cleanupCheck(7, func() { state.routes = 0 }),
		checkCensus: cleanupCheck(8, func() {
			state.sessions, state.streams, state.activeCalls, state.routes = 0, 0, 0, 0
		}),
		removeRoot: cleanupCheck(9, func() { state.rootExists = false }),
	})
	if !errors.Is(err, primaryErr) {
		t.Fatalf("cleanup error = %v, want primary assertion error preserved", err)
	}
	for _, want := range cleanupErrs {
		if !errors.Is(err, want) {
			t.Errorf("cleanup error = %v, want joined cleanup error %v", err, want)
		}
	}
	if state.sessions != 0 || state.streams != 0 || state.activeCalls != 0 || state.routes != 0 ||
		state.daemonOpen || state.processOpen || state.listenerOpen || state.rootExists {
		t.Fatalf("cleanup state = %#v, want every owned resource released", state)
	}
}
