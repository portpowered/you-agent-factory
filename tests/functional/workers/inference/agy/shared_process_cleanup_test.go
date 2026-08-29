package agy

import (
	"context"
	"errors"
	"testing"
)

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
