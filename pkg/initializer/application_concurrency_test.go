package initializer_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
)

func TestApplicationParentCancellationCancelsAndJoinsEveryWaitableLifecycle(t *testing.T) {
	t.Parallel()

	graph, lifecycles := newOwnedLifecycleGraph(nil)
	application, err := initializer.NewApplication(initializer.ModeCLI, graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()
	waitForOwnedStarts(t, lifecycles)
	cancel()
	if err := receiveRunResult(t, done); err != nil {
		t.Fatalf("Run() cancellation error = %v", err)
	}
	assertOwnedLifecyclesStopped(t, lifecycles)
}

func TestAPIApplicationParentCancellationJoinsListenerLifecycle(t *testing.T) {
	t.Parallel()

	graph, lifecycles := newOwnedLifecycleGraph(nil)
	graph.lifecycles.API = graph.lifecycles.CLI
	graph.lifecycles.CLI = nil
	application, err := initializer.NewApplication(initializer.ModeAPI, graph)
	if err != nil {
		t.Fatalf("NewApplication(API) error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()
	waitForOwnedStarts(t, lifecycles)
	cancel()
	if err := receiveRunResult(t, done); err != nil {
		t.Fatalf("Run(API) cancellation error = %v", err)
	}
	assertOwnedLifecyclesStopped(t, lifecycles)
}

func TestApplicationPeerFailureCancelsPeersAndReturnsAfterEveryJoin(t *testing.T) {
	t.Parallel()

	graph, lifecycles := newOwnedLifecycleGraph(nil)
	application, err := initializer.NewApplication(initializer.ModeCLI, graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- application.Run(context.Background()) }()
	waitForOwnedStarts(t, lifecycles)
	peerErr := errors.New("runtime loop failed")
	lifecycles[0].complete(peerErr)
	if err := receiveRunResult(t, done); !errors.Is(err, peerErr) {
		t.Fatalf("Run() error = %v, want runtime peer failure", err)
	}
	assertOwnedLifecyclesStopped(t, lifecycles)
}

func TestApplicationReportsSimultaneousFailuresInPlanOrder(t *testing.T) {
	t.Parallel()

	report := make(chan struct{})
	graph, lifecycles := newOwnedLifecycleGraph(report)
	application, err := initializer.NewApplication(initializer.ModeCLI, graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- application.Run(context.Background()) }()
	waitForOwnedStarts(t, lifecycles)
	runtimeErr := errors.New("runtime failed")
	workersErr := errors.New("workers failed")
	lifecycles[0].complete(runtimeErr)
	lifecycles[1].complete(workersErr)
	close(report)
	runErr := receiveRunResult(t, done)
	if !errors.Is(runErr, runtimeErr) || !errors.Is(runErr, workersErr) {
		t.Fatalf("Run() error = %v, want both simultaneous failures", runErr)
	}
	if runtimeIndex, workersIndex := strings.Index(runErr.Error(), "runtime sidecar"), strings.Index(runErr.Error(), "workers sidecar"); runtimeIndex < 0 || workersIndex < 0 || runtimeIndex >= workersIndex {
		t.Fatalf("Run() error order = %q, want runtime before workers", runErr)
	}
}

func TestApplicationCannotReturnWhileOwnedJoinIsBlocked(t *testing.T) {
	t.Parallel()

	graph, lifecycles := newOwnedLifecycleGraph(nil)
	joinRelease := make(chan struct{})
	lifecycles[0].joinRelease = joinRelease
	application, err := initializer.NewApplication(initializer.ModeCLI, graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()
	waitForOwnedStarts(t, lifecycles)
	cancel()
	select {
	case err := <-done:
		t.Fatalf("Run() returned before blocked join was released: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(joinRelease)
	if err := receiveRunResult(t, done); err != nil {
		t.Fatalf("Run() cancellation error = %v", err)
	}
}

func TestApplicationShutdownIsIdempotentWhenCancellationAndPeerExitRace(t *testing.T) {
	t.Parallel()

	report := make(chan struct{})
	graph, lifecycles := newOwnedLifecycleGraph(report)
	application, err := initializer.NewApplication(initializer.ModeCLI, graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- application.Run(ctx) }()
	waitForOwnedStarts(t, lifecycles)
	peerErr := errors.New("runtime raced with cancellation")
	lifecycles[0].complete(peerErr)

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- application.Shutdown(context.Background()) }()
	cancel()
	close(report)
	if err := receiveRunResult(t, runDone); !errors.Is(err, peerErr) {
		t.Fatalf("Run() error = %v, want peer failure", err)
	}
	if err := receiveRunResult(t, shutdownDone); err != nil {
		t.Fatalf("concurrent Shutdown() error = %v", err)
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("repeated Shutdown() error = %v", err)
	}
	assertOwnedLifecyclesStopped(t, lifecycles)
	graph.mu.Lock()
	closes := graph.closes
	graph.mu.Unlock()
	if closes != 1 {
		t.Fatalf("graph closes = %d, want one", closes)
	}
}

type ownedLifecycleGraph struct {
	lifecycles initializer.ApplicationLifecycles
	mu         sync.Mutex
	closes     int
}

func newOwnedLifecycleGraph(report <-chan struct{}) (*ownedLifecycleGraph, []*ownedLifecycle) {
	component := func() *ownedLifecycle {
		return &ownedLifecycle{started: make(chan struct{}), done: make(chan struct{}), report: report}
	}
	lifecycles := []*ownedLifecycle{component(), component(), component(), component()}
	graph := &ownedLifecycleGraph{lifecycles: initializer.ApplicationLifecycles{
		Runtime: lifecycles[0], Workers: lifecycles[1], Dashboard: lifecycles[2], CLI: lifecycles[3],
	}}
	return graph, lifecycles
}

func (g *ownedLifecycleGraph) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closes++
	return nil
}

func (g *ownedLifecycleGraph) Lifecycles() initializer.ApplicationLifecycles { return g.lifecycles }

func (g *ownedLifecycleGraph) RuntimeLogMetadata() runtimehost.RuntimeLogDiagnostics {
	return runtimehost.RuntimeLogDiagnostics{}
}

type ownedLifecycle struct {
	started     chan struct{}
	done        chan struct{}
	report      <-chan struct{}
	joinRelease <-chan struct{}
	completeOne sync.Once
	mu          sync.Mutex
	err         error
	stopCalls   int
}

func (l *ownedLifecycle) Start(ctx context.Context) error {
	close(l.started)
	go func() {
		<-ctx.Done()
		l.complete(ctx.Err())
	}()
	return nil
}

func (l *ownedLifecycle) Stop(context.Context) error {
	l.mu.Lock()
	l.stopCalls++
	l.mu.Unlock()
	l.complete(context.Canceled)
	<-l.done
	return nil
}

func (l *ownedLifecycle) Wait(context.Context) error {
	<-l.done
	if l.report != nil {
		<-l.report
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

func (l *ownedLifecycle) complete(err error) {
	l.completeOne.Do(func() {
		if l.joinRelease != nil {
			<-l.joinRelease
		}
		l.mu.Lock()
		l.err = err
		l.mu.Unlock()
		close(l.done)
	})
}

func waitForOwnedStarts(t *testing.T, lifecycles []*ownedLifecycle) {
	t.Helper()
	for _, lifecycle := range lifecycles {
		select {
		case <-lifecycle.started:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for lifecycle start")
		}
	}
}

func receiveRunResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for application run")
		return nil
	}
}

func assertOwnedLifecyclesStopped(t *testing.T, lifecycles []*ownedLifecycle) {
	t.Helper()
	for index, lifecycle := range lifecycles {
		lifecycle.mu.Lock()
		stopCalls := lifecycle.stopCalls
		lifecycle.mu.Unlock()
		if stopCalls != 1 {
			t.Fatalf("lifecycle %d stop calls = %d, want one", index, stopCalls)
		}
		select {
		case <-lifecycle.done:
		default:
			t.Fatalf("lifecycle %d was not joined", index)
		}
	}
}
