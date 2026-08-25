package metrics

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

func TestRuntimeMetricsOpenerRejectsMissingExplicitSelections(t *testing.T) {
	paths, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatal(err)
	}
	opener := newRetentionTestOpener(t, paths)
	valid := RuntimeMetricsOpeningRequest{RuntimeInstanceID: "runtime", RootDirectory: t.TempDir(), StartTimeUTC: time.Now(), CollisionID: "collision"}
	tests := []struct {
		name string
		edit func(*RuntimeMetricsOpeningRequest)
		want string
	}{
		{name: "root", edit: func(r *RuntimeMetricsOpeningRequest) { r.RootDirectory = "" }, want: "runtime metrics root is required"},
		{name: "runtime", edit: func(r *RuntimeMetricsOpeningRequest) { r.RuntimeInstanceID = "" }, want: "runtime instance ID is required"},
		{name: "clock", edit: func(r *RuntimeMetricsOpeningRequest) { r.StartTimeUTC = time.Time{} }, want: "runtime metrics start time is required"},
		{name: "id", edit: func(r *RuntimeMetricsOpeningRequest) { r.CollisionID = "" }, want: "runtime metrics collision ID is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.edit(&request)
			_, err := opener.Open(request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Open() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRuntimeMetricsOpenerValidatesConstruction(t *testing.T) {
	paths := &metricsTestReserver{}
	lifecycle := &metricsTestRetentionLifecycle{}
	coordination := &metricsTestCoordination{}
	if opener, err := NewRuntimeMetricsOpener(nil, lifecycle); opener != nil || err == nil || !strings.Contains(err.Error(), "path reserver") {
		t.Fatalf("NewRuntimeMetricsOpener(nil): (%#v, %v)", opener, err)
	}
	if opener, err := NewRuntimeMetricsOpener(paths, nil); opener != nil || err == nil || !strings.Contains(err.Error(), "retention lifecycle") {
		t.Fatalf("NewRuntimeMetricsOpener(nil lifecycle): (%#v, %v)", opener, err)
	}
	if opener, err := NewRuntimeMetricsOpener(paths, lifecycle, coordination, coordination); opener != nil || err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("NewRuntimeMetricsOpener(two coordinators): (%#v, %v)", opener, err)
	}
	if opener, err := NewRuntimeMetricsOpener(paths, lifecycle, nil); opener != nil || err == nil || !strings.Contains(err.Error(), "coordination is required") {
		t.Fatalf("NewRuntimeMetricsOpener(nil coordinator): (%#v, %v)", opener, err)
	}
}

func TestRuntimeMetricsOpenerReportsRetentionAndRootLockFailures(t *testing.T) {
	opener, _, lifecycle, coordination, valid := newTestRuntimeMetricsOpener(t)
	startError := errors.New("retention start failed")
	lifecycle.startErr = startError
	if _, err := opener.Open(valid); !errors.Is(err, startError) {
		t.Fatalf("Open(start failure) = %v, want %v", err, startError)
	}
	lifecycle.startErr = nil
	lifecycle.lease = nil
	if _, err := opener.Open(valid); err == nil || !strings.Contains(err.Error(), "nil lease") {
		t.Fatalf("Open(nil lease) = %v, want nil lease validation", err)
	}
	lifecycle.lease = &metricsTestCloser{}

	lockError := errors.New("root lock failed")
	coordination.lockRootErr = lockError
	if _, err := opener.Open(valid); !errors.Is(err, lockError) || lifecycle.lease.(*metricsTestCloser).closed != 1 {
		t.Fatalf("Open(root lock failure) = %v, lease=%#v", err, lifecycle.lease)
	}
}

func TestRuntimeMetricsOpenerReleasesReservationFailures(t *testing.T) {
	opener, paths, lifecycle, coordination, valid := newTestRuntimeMetricsOpener(t)
	coordination.lockRootErr = nil
	lifecycle.lease = &metricsTestCloser{}

	reserveError := errors.New("reserve failed")
	paths.err = reserveError
	coordination.rootLock = &metricsTestCloser{}
	if _, err := opener.Open(valid); !errors.Is(err, reserveError) || coordination.rootLock.(*metricsTestCloser).closed != 1 || lifecycle.lease.(*metricsTestCloser).closed != 1 {
		t.Fatalf("Open(reserve failure) = %v, root lock=%#v lease=%#v", err, coordination.rootLock, lifecycle.lease)
	}
	paths.err = nil
	lifecycle.lease = &metricsTestCloser{}
	coordination.rootLock = &metricsTestCloser{}

	claimError := errors.New("claim failed")
	coordination.claimErr = claimError
	if _, err := opener.Open(valid); !errors.Is(err, claimError) || coordination.rootLock.(*metricsTestCloser).closed != 1 || lifecycle.lease.(*metricsTestCloser).closed != 1 {
		t.Fatalf("Open(claim failure) = %v, root lock=%#v lease=%#v", err, coordination.rootLock, lifecycle.lease)
	}
	coordination.claimErr = nil
	lifecycle.lease = &metricsTestCloser{}
	coordination.rootLock = &metricsTestCloser{err: errors.New("root lock close failed")}
	coordination.claim = &metricsTestCloser{}
	if _, err := opener.Open(valid); err == nil || !strings.Contains(err.Error(), "release runtime metrics startup coordination") || coordination.claim.(*metricsTestCloser).closed != 1 || lifecycle.lease.(*metricsTestCloser).closed != 1 {
		t.Fatalf("Open(root lock close failure) = %v, claim=%#v lease=%#v", err, coordination.claim, lifecycle.lease)
	}
}

func TestRuntimeMetricsOpenerClosesLifecycle(t *testing.T) {
	opener, _, lifecycle, _, valid := newTestRuntimeMetricsOpener(t)
	closeError := errors.New("lifecycle close failed")
	lifecycle.closeErr = closeError
	if err := opener.Close(context.Background()); !errors.Is(err, closeError) {
		t.Fatalf("opener.Close() = %v, want %v", err, closeError)
	}
	var nilOpener *RuntimeMetricsOpener
	if err := nilOpener.Close(context.Background()); err != nil {
		t.Fatalf("nil opener Close() = %v, want nil", err)
	}
	if _, err := nilOpener.Open(valid); err == nil || !strings.Contains(err.Error(), "opener is required") {
		t.Fatalf("nil opener Open() = %v, want configuration error", err)
	}
}

func newTestRuntimeMetricsOpener(t *testing.T) (
	*RuntimeMetricsOpener,
	*metricsTestReserver,
	*metricsTestRetentionLifecycle,
	*metricsTestCoordination,
	RuntimeMetricsOpeningRequest,
) {
	t.Helper()
	paths := &metricsTestReserver{path: filepath.Join(t.TempDir(), "metrics.log")}
	lifecycle := &metricsTestRetentionLifecycle{lease: &metricsTestCloser{}}
	coordination := &metricsTestCoordination{}
	opener, err := NewRuntimeMetricsOpener(paths, lifecycle, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsOpener(): %v", err)
	}
	valid := RuntimeMetricsOpeningRequest{
		RuntimeInstanceID: "runtime",
		RootDirectory:     t.TempDir(),
		StartTimeUTC:      time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
		CollisionID:       "collision",
	}
	return opener, paths, lifecycle, coordination, valid
}

type metricsTestCloser struct {
	err    error
	closed int
}

func (closer *metricsTestCloser) Close() error {
	if closer == nil {
		return nil
	}
	closer.closed++
	return closer.err
}

type metricsTestRetentionLifecycle struct {
	lease    io.Closer
	startErr error
	closeErr error
}

func (lifecycle *metricsTestRetentionLifecycle) Start(context.Context, RuntimeMetricsRetentionRequest) (io.Closer, error) {
	return lifecycle.lease, lifecycle.startErr
}

func (lifecycle *metricsTestRetentionLifecycle) Close(context.Context) error {
	return lifecycle.closeErr
}

type metricsTestReserver struct {
	path string
	err  error
}

func (reserver *metricsTestReserver) Reserve(string, time.Time, string, string) (string, error) {
	return reserver.path, reserver.err
}

func (reserver *metricsTestReserver) ReserveNamed(string, time.Time, string, string) (string, error) {
	return "", errors.New("unexpected ReserveNamed call")
}

func (reserver *metricsTestReserver) ReserveNamedWithCollision(string, time.Time, string, string) (string, error) {
	return "", errors.New("unexpected ReserveNamedWithCollision call")
}

type metricsTestCoordination struct {
	rootLock          io.Closer
	claim             io.Closer
	tryRootLock       io.Closer
	tryClaim          io.Closer
	tryClaimMarker    io.Closer
	lockRootErr       error
	claimErr          error
	tryLockRootErr    error
	tryClaimErr       error
	tryClaimMarkerErr error
	onTryClaim        func(string)
}

func (coordination *metricsTestCoordination) LockRoot(context.Context, string) (io.Closer, error) {
	if coordination.rootLock == nil {
		coordination.rootLock = &metricsTestCloser{}
	}
	return coordination.rootLock, coordination.lockRootErr
}

func (coordination *metricsTestCoordination) TryLockRoot(string) (io.Closer, error) {
	return coordination.tryRootLock, coordination.tryLockRootErr
}

func (coordination *metricsTestCoordination) Claim(string) (io.Closer, error) {
	if coordination.claim == nil {
		coordination.claim = &metricsTestCloser{}
	}
	return coordination.claim, coordination.claimErr
}

func (coordination *metricsTestCoordination) TryClaim(path string) (io.Closer, error) {
	if coordination.onTryClaim != nil {
		coordination.onTryClaim(path)
	}
	return coordination.tryClaim, coordination.tryClaimErr
}

func (coordination *metricsTestCoordination) TryClaimMarker(string) (io.Closer, error) {
	return coordination.tryClaimMarker, coordination.tryClaimMarkerErr
}
