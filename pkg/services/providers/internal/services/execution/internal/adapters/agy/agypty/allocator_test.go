package agypty

import (
	"context"
	"errors"
	"io"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"

	"go.uber.org/goleak"
)

type failingHost struct {
	pty platformpty.Allocation
	err error
}

type observedAllocationHost struct {
	allocation platformpty.Allocation
	err        error
	calls      int
	starts     int
	ctx        context.Context
}

func (h *observedAllocationHost) Allocate(ctx context.Context) (platformpty.Allocation, error) {
	h.calls++
	h.ctx = ctx
	return h.allocation, h.err
}

func (h *observedAllocationHost) Start(platformpty.ProcessLaunch, platformpty.Allocation) (platformpty.Process, io.ReadCloser, error) {
	h.starts++
	return nil, nil, errors.New("unexpected Start")
}

type observedAllocation struct{ closes int }

func (a *observedAllocation) Close() error {
	a.closes++
	return nil
}

func (*observedAllocation) Kind() platformpty.Kind { return platformpty.KindConPTY }

func TestAllocatorUsesCompletedHostAfterAdmission(t *testing.T) {
	t.Parallel()
	wantErr := context.DeadlineExceeded
	host := &observedAllocationHost{err: wantErr}
	allocator, err := NewAllocator(host, testPTYClock, platformclock.Real{})
	if err != nil || host.calls != 0 || host.starts != 0 {
		t.Fatalf("construction = (%v, %v), host = %+v", allocator, err, host)
	}
	if _, err := allocator.Allocate(context.Background(), ProcessLaunch{}, DefaultSessionConfig()); err == nil {
		t.Fatal("invalid launch was admitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	launch := ProcessLaunch{Executable: "agy", Argv: []string{"agy"}}
	if _, err := allocator.Allocate(ctx, launch, DefaultSessionConfig()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission = %v", err)
	}
	if host.calls != 0 {
		t.Fatalf("rejected admission reached host %d times", host.calls)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	if _, err := allocator.Allocate(ctx, launch, DefaultSessionConfig()); err != wantErr {
		t.Fatalf("native failure = %v, want exact %v", err, wantErr)
	}
	if host.calls != 1 || host.ctx != ctx || host.starts != 0 {
		t.Fatalf("allocation host = %+v, want one call with original context and no start", host)
	}
}

func TestAllocatorSessionsCloseOnlyTheirOwnedResource(t *testing.T) {
	t.Parallel()
	first, peer := &observedAllocation{}, &observedAllocation{}
	firstSession := allocateObservedSession(t, first)
	peerSession := allocateObservedSession(t, peer)
	for range 2 {
		if err := firstSession.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if first.closes != 1 || peer.closes != 0 {
		t.Fatalf("first/peer closes = %d/%d, want 1/0", first.closes, peer.closes)
	}
	if err := peerSession.Close(); err != nil {
		t.Fatal(err)
	}
	if first.closes != 1 || peer.closes != 1 {
		t.Fatalf("first/peer closes = %d/%d, want 1/1", first.closes, peer.closes)
	}
}

func allocateObservedSession(t *testing.T, allocation *observedAllocation) PTYSession {
	t.Helper()
	host := &observedAllocationHost{allocation: allocation}
	allocator, err := NewAllocator(host, testPTYClock, platformclock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := allocator.Allocate(context.Background(), ProcessLaunch{Executable: "agy", Argv: []string{"agy"}}, DefaultSessionConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if host.calls != 1 || host.starts != 0 || session.(*platformSession).PTYKind() != PTYKindConPTY || allocation.closes != 0 {
		t.Fatalf("allocated session = %v, host = %+v, resource = %+v", session, host, allocation)
	}
	return session
}

func (h failingHost) Allocate(context.Context) (platformpty.Allocation, error) { return h.pty, h.err }
func (failingHost) Start(platformpty.ProcessLaunch, platformpty.Allocation) (platformpty.Process, io.ReadCloser, error) {
	return nil, nil, errors.New("unexpected Start")
}

func TestAllocator_PreservesUnsupportedHostFailure(t *testing.T) {
	t.Parallel()
	allocator, err := NewAllocator(failingHost{err: platformpty.ErrUnsupportedPlatform}, testPTYClock, platformclock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = allocator.Allocate(context.Background(), ProcessLaunch{Executable: "agy", Argv: []string{"agy"}}, DefaultSessionConfig())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Allocate() error = %v", err)
	}
}

func TestAllocator_WrapsNativeAllocationFailure(t *testing.T) {
	t.Parallel()
	allocator, err := NewAllocator(failingHost{err: errors.New("native failure")}, testPTYClock, platformclock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = allocator.Allocate(context.Background(), ProcessLaunch{Executable: "agy", Argv: []string{"agy"}}, DefaultSessionConfig())
	if !errors.Is(err, ErrPTYAllocationFailed) {
		t.Fatalf("Allocate() error = %v", err)
	}
}

func TestAllocator_RejectsNilNativeAllocation(t *testing.T) {
	t.Parallel()
	allocator, err := NewAllocator(failingHost{}, testPTYClock, platformclock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = allocator.Allocate(context.Background(), ProcessLaunch{Executable: "agy", Argv: []string{"agy"}}, DefaultSessionConfig())
	if !errors.Is(err, ErrPTYAllocationFailed) {
		t.Fatalf("Allocate() error = %v", err)
	}
}

// TestMain fails the package when a test leaves goroutines running, which
// otherwise surfaces as teardown hangs and cross-test interference.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
