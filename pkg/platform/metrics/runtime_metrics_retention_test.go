package metrics

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

func TestRuntimeMetricsRootRetentionProtectsOpenWriterAcrossFactoryIdentities(t *testing.T) {
	root := t.TempDir()
	paths, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewReserver(): %v", err)
	}
	firstOpener := newRetentionTestOpener(t, paths)
	secondOpener := newRetentionTestOpener(t, paths)
	started := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	live := openRetentionTestSink(t, firstOpener, RuntimeMetricsOpeningRequest{
		SessionID:         "factory-one-session",
		RuntimeInstanceID: "factory-one-runtime",
		RootDirectory:     root,
		StartTimeUTC:      started,
		CollisionID:       "live",
		Config:            RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	defer live.Close()
	writeRetentionMetric(t, live, "live.before_sweep")
	claimPath := runtimeMetricsClaimPath(live.Path())
	assertRetentionPathExists(t, claimPath, "active claim before sweep")

	retention, err := NewRuntimeMetricsRetention(
		platformfilesystem.Local{},
		func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	// The second opener models another factory/process sharing the user-global
	// root; the sweep must respect the first opener's OS-held claim.
	other := openRetentionTestSink(t, secondOpener, RuntimeMetricsOpeningRequest{
		SessionID:         "factory-two-session",
		RuntimeInstanceID: "factory-two-runtime",
		RootDirectory:     root,
		StartTimeUTC:      time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC),
		CollisionID:       "other",
		Config:            RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	defer other.Close()
	writeRetentionMetric(t, other, "other.factory")
	report, err := retention.Sweep(context.Background(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	assertOpenWriterSweep(t, report, err, live.Path())
	writeRetentionMetric(t, live, "live.after_sweep")
	assertRetentionRecordCount(t, live.Path(), 2)

	if err := live.Close(); err != nil {
		t.Fatalf("close live metrics sink: %v", err)
	}
	assertRetentionPathExists(t, claimPath, "stable claim marker after close")
	report, err = retention.Sweep(context.Background(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	assertClosedRetentionSweep(t, report, err, live.Path())
}

func newRetentionTestOpener(t *testing.T, paths platformartifact.Reserver) *RuntimeMetricsOpener {
	t.Helper()
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	scheduler, err := NewRuntimeMetricsRetentionScheduler(retention, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetentionScheduler(): %v", err)
	}
	opener, err := NewRuntimeMetricsOpener(paths, scheduler, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsOpener(): %v", err)
	}
	return opener
}

func openRetentionTestSink(
	t *testing.T,
	opener *RuntimeMetricsOpener,
	request RuntimeMetricsOpeningRequest,
) *RuntimeMetricsSink {
	t.Helper()
	sink, err := opener.Open(request)
	if err != nil {
		t.Fatalf("open runtime metrics sink: %v", err)
	}
	return sink
}

func writeRetentionMetric(t *testing.T, sink *RuntimeMetricsSink, name string) {
	t.Helper()
	if err := sink.WriteMetric(context.Background(), map[string]any{"metric_name": name}); err != nil {
		t.Fatalf("write metric %q: %v", name, err)
	}
}

func assertRetentionPathExists(t *testing.T, path, description string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s %q unavailable: %v", description, path, err)
	}
}

func assertRetentionPathAbsent(t *testing.T, path, description string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s %q remains, stat error = %v", description, path, err)
	}
}

func assertRetentionRecordCount(t *testing.T, path string, want int) {
	t.Helper()
	if got := len(readRuntimeMetricsRecords(t, path)); got != want {
		t.Fatalf("metrics record count = %d, want %d", got, want)
	}
}

func assertOpenWriterSweep(t *testing.T, report RuntimeMetricsRetentionReport, err error, livePath string) {
	t.Helper()
	if err != nil {
		t.Fatalf("Sweep(open writer): %v", err)
	}
	if report.Protected.Files != 1 {
		t.Fatalf("Protected.Files = %d, want one active writer", report.Protected.Files)
	}
	assertRetentionPathExists(t, livePath, "live metrics path during sweep")
}

func assertClosedRetentionSweep(t *testing.T, report RuntimeMetricsRetentionReport, err error, livePath string) {
	t.Helper()
	if err != nil {
		t.Fatalf("Sweep(after close): %v", err)
	}
	if report.Removed.Files == 0 {
		t.Fatalf("Sweep(after close) = %#v, want closed artifact removal", report)
	}
	assertRetentionPathAbsent(t, livePath, "closed expired metrics path")
}

func TestRuntimeMetricsRootRetentionYieldsWhenRootSweepIsAlreadyClaimed(t *testing.T) {
	root := t.TempDir()
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	rootLock, err := coordination.LockRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("LockRoot(): %v", err)
	}
	defer rootLock.Close()

	retention, err := NewRuntimeMetricsRetention(
		platformfilesystem.Local{},
		func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
		coordination,
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	report, err := retention.Sweep(context.Background(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Sweep(held root): %v", err)
	}
	if !report.Skipped {
		t.Fatal("Sweep(held root) skipped = false, want safe yield")
	}
}

func TestRuntimeMetricsCoordinationCancelsWaitingLocksAndClassifiesBusyClaims(t *testing.T) {
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	if _, err := coordination.TryLockRoot(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("TryLockRoot(blank) = %v, want path validation", err)
	}
	root := t.TempDir()
	rootLock, err := coordination.LockRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("LockRoot(): %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordination.LockRoot(canceled, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("LockRoot(canceled while busy) = %v, want context.Canceled", err)
	}
	if _, err := coordination.TryLockRoot(root); !errors.Is(err, ErrRuntimeMetricsRootBusy) {
		t.Fatalf("TryLockRoot(held) = %v, want ErrRuntimeMetricsRootBusy", err)
	}
	if err := rootLock.Close(); err != nil {
		t.Fatalf("rootLock.Close(): %v", err)
	}

	artifact := filepath.Join(root, "2026", "08", "24", "010000.000000000-runtime-metrics-session-runtime-collision.log")
	claim, err := coordination.Claim(artifact)
	if err != nil {
		t.Fatalf("Claim(): %v", err)
	}
	if _, err := coordination.TryClaim(artifact); !errors.Is(err, ErrRuntimeMetricsArtifactBusy) {
		t.Fatalf("TryClaim(held) = %v, want ErrRuntimeMetricsArtifactBusy", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatalf("claim.Close(): %v", err)
	}
	if _, err := coordination.TryClaimMarker(filepath.Join(root, "missing-marker")); err == nil {
		t.Fatal("TryClaimMarker(missing) succeeded, want filesystem error")
	}
}

// These component witnesses use real host locks and scenario-owned paths. They
// prove safe rejection and handoff without launching another OS process.
func TestRuntimeMetricsCoordinationMissingMarkerDoesNotCreateAndRecovers(t *testing.T) {
	t.Parallel()
	coordination := runtimeMetricsCoordination{}
	marker := filepath.Join(t.TempDir(), "selected.active")
	if lock, err := coordination.TryClaimMarker(" "); lock != nil || err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("blank marker claim = %v, %v, want path validation", lock, err)
	}
	lock, err := coordination.TryClaimMarker(marker)
	if lock != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing marker claim = %v, %v, want not-exist without a lock", lock, err)
	}
	assertRetentionPathAbsent(t, marker, "missing marker must not be created")
	const content = "preserve marker bytes"
	if err := os.WriteFile(marker, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err = coordination.TryClaimMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	if peer, err := coordination.TryClaimMarker(marker); peer != nil || !errors.Is(err, ErrRuntimeMetricsArtifactBusy) {
		t.Fatalf("owned marker claim = %v, %v, want active", peer, err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	assertRetentionPreservedContent(t, marker, content)
	recovered, err := coordination.TryClaimMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeMetricsCoordinationRejectsSymlinkMarkerAndRecovers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "customer-file")
	const content = "customer content outside the claim"
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "selected.active")
	if err := os.Symlink(target, marker); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	coordination := runtimeMetricsCoordination{}
	lock, err := coordination.TryClaimMarker(marker)
	if lock != nil || err == nil || !strings.Contains(err.Error(), "is a symlink") || !strings.Contains(err.Error(), strconv.Quote(marker)) {
		t.Fatalf("symlink marker claim = %v, %v, want safe path rejection", lock, err)
	}
	assertRetentionPreservedContent(t, target, content)
	if got, err := os.Readlink(marker); err != nil || got != target {
		t.Fatalf("rejected marker changed: %q, %v", got, err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("regular marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err = coordination.TryClaimMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	assertRetentionPreservedContent(t, target, content)
	assertRetentionPreservedContent(t, marker, "regular marker")
}

func TestRuntimeMetricsCoordinationRejectsSymlinkRootAndRecovers(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	target := filepath.Join(parent, "customer-directory")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	customer := filepath.Join(target, "customer-file")
	const content = "preserve customer directory contents"
	if err := os.WriteFile(customer, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "selected-root")
	if err := os.Symlink(target, root); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	coordination := runtimeMetricsCoordination{}
	lock, err := coordination.TryLockRoot(root)
	if lock != nil || err == nil || !strings.Contains(err.Error(), "is not a directory") || !strings.Contains(err.Error(), strconv.Quote(root)) {
		t.Fatalf("symlink root acquisition = %v, %v, want identity rejection", lock, err)
	}
	assertRetentionPathAbsent(t, rootLockPath(target), "rejected root must not create a target lock")
	assertRetentionPreservedContent(t, customer, content)
	if got, err := os.Readlink(root); err != nil || got != target {
		t.Fatalf("rejected root changed: %q, %v", got, err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	lock, err = coordination.TryLockRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	assertRetentionPreservedContent(t, customer, content)
}

func TestRuntimeMetricsCoordinationRejectsMarkerPermissionsAndRecovers(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission rejection requires Linux CI")
	}
	for _, denied := range []string{"inspect", "open"} {
		t.Run(denied, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			marker := filepath.Join(root, "selected.active")
			const content = "preserve rejected marker bytes"
			if err := os.WriteFile(marker, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			blocked := marker
			if denied == "inspect" {
				blocked = root
			}
			if err := os.Chmod(blocked, 0); err != nil {
				t.Fatal(err)
			}
			// Restore permissions before TempDir cleanup even on assertion failure.
			t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
			coordination := runtimeMetricsCoordination{}
			lock, err := coordination.TryClaimMarker(marker)
			if err == nil && lock != nil {
				_ = lock.Close()
				t.Skip("host identity bypasses POSIX permissions")
			}
			if lock != nil || !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), denied+" runtime metrics coordination file") || !strings.Contains(err.Error(), strconv.Quote(marker)) {
				t.Fatalf("denied marker %s = %v, %v, want permission cause and operation", denied, lock, err)
			}
			if err := os.Chmod(blocked, 0o700); err != nil {
				t.Fatal(err)
			}
			assertRetentionPreservedContent(t, marker, content)
			lock, err = coordination.TryClaimMarker(marker)
			if err != nil {
				t.Fatal(err)
			}
			if err := lock.Close(); err != nil {
				t.Fatal(err)
			}
			assertRetentionPreservedContent(t, marker, content)
		})
	}
}

func TestRuntimeMetricsCoordinationCloseRetainsFailureAndRecovers(t *testing.T) {
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "selected.active")
	const content = "preserve closed-owner marker"
	if err := os.WriteFile(marker, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	coordination := runtimeMetricsCoordination{}
	owner, err := coordination.TryClaimMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	lock, ok := owner.(*runtimeMetricsLock)
	if !ok {
		t.Fatalf("unexpected owning lock type %T", owner)
	}
	if err := lock.file.Close(); err != nil {
		t.Fatal(err)
	}
	first := owner.Close()
	var nativeError syscall.Errno
	if !errors.Is(first, os.ErrClosed) || !errors.As(first, &nativeError) || nativeError == 0 {
		t.Fatalf("close lost native unlock or file-close cause: %v", first)
	}
	recovered, err := coordination.TryClaimMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recovered.Close() })
	if again := owner.Close(); !errors.Is(again, first) {
		t.Fatalf("repeated close changed retained failure: %v, want %v", again, first)
	}
	if peer, err := coordination.TryClaimMarker(marker); peer != nil || !errors.Is(err, ErrRuntimeMetricsArtifactBusy) {
		t.Fatalf("failed old close disturbed recovered owner: %v, %v", peer, err)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}
	assertRetentionPreservedContent(t, marker, content)
}

func TestRuntimeMetricsCoordinationRejectsClosedHandleAndRecovers(t *testing.T) {
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "selected.active")
	file, err := os.OpenFile(marker, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireRuntimeMetricsFile(t.Context(), file, marker, false, true)
	if lock != nil || err == nil || errors.Is(err, ErrRuntimeMetricsArtifactBusy) || !strings.Contains(err.Error(), strconv.Quote(marker)) {
		t.Fatalf("invalid handle claim = %v, %v, want OS error with selected path", lock, err)
	}
	var nativeError syscall.Errno
	if !errors.As(err, &nativeError) || nativeError == 0 {
		t.Fatalf("native lock cause lost: %v", err)
	}
	if _, err := file.Stat(); err == nil {
		t.Fatalf("rejected handle remained usable: %v", err)
	}
	coordination := runtimeMetricsCoordination{}
	lock, err = coordination.TryClaimMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	assertRetentionPreservedContent(t, marker, "")
}

// Done is consulted only after the real host lock reports contention. The
// existing context boundary supplies a deterministic cancellation barrier.
type metricsWaitingContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (ctx *metricsWaitingContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func TestRuntimeMetricsCoordinationCancelsContendedWaitAndRecovers(t *testing.T) {
	t.Parallel()
	coordination := runtimeMetricsCoordination{}
	root := t.TempDir()
	owner, err := coordination.LockRoot(nil, root) //nolint:staticcheck // Prove the supported nil-context normalization.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	waiter := &metricsWaitingContext{Context: ctx, waiting: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		lock, err := coordination.LockRoot(waiter, root)
		if lock != nil {
			_ = lock.Close()
		}
		result <- err
	}()
	select {
	case <-waiter.waiting:
	case <-time.After(30 * time.Second):
		t.Fatal("contended lock did not enter its cancellation wait")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("contended wait = %v, want context.Canceled", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("contended lock did not acknowledge cancellation")
	}
	if lock, err := coordination.TryLockRoot(root); lock != nil || !errors.Is(err, ErrRuntimeMetricsRootBusy) {
		t.Fatalf("canceled waiter changed owner lock: %v, %v", lock, err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := coordination.LockRoot(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeMetricsCoordinationRejectsFileRootAndRecovers(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "selected-root")
	const content = "customer content"
	if err := os.WriteFile(root, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	coordination := runtimeMetricsCoordination{}
	lock, err := coordination.TryLockRoot(root)
	if lock != nil || err == nil || !strings.Contains(err.Error(), "create runtime metrics coordination root") {
		t.Fatalf("file root = (%v, %v), want rejected acquisition", lock, err)
	}
	if data, err := os.ReadFile(root); err != nil || string(data) != content {
		t.Fatalf("rejected root content = %q, %v", data, err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	lock, err = coordination.TryLockRoot(root)
	if err != nil {
		t.Fatalf("corrected root acquisition: %v", err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	if _, err := coordination.TryLockRoot(root); !errors.Is(err, ErrRuntimeMetricsRootBusy) {
		t.Fatalf("healthy root ownership = %v, want busy", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeMetricsCoordinationRejectsDirectoryMarkerAndRecovers(t *testing.T) {
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "selected-marker.active")
	if err := os.Mkdir(marker, 0o700); err != nil {
		t.Fatal(err)
	}
	protected := filepath.Join(marker, "customer.txt")
	if err := os.WriteFile(protected, []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	coordination := runtimeMetricsCoordination{}
	lock, err := coordination.TryClaimMarker(marker)
	if lock != nil || err == nil || !strings.Contains(err.Error(), "open runtime metrics coordination file") {
		t.Fatalf("directory marker = (%v, %v), want rejected acquisition", lock, err)
	}
	if data, err := os.ReadFile(protected); err != nil || string(data) != "preserved" {
		t.Fatalf("protected marker content = %q, %v", data, err)
	}
	// Move the unsafe candidate intact; a valid marker can then be selected.
	if err := os.Rename(marker, marker+".protected"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err = coordination.TryClaimMarker(marker)
	if err != nil {
		t.Fatalf("corrected marker acquisition: %v", err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(marker+".protected", "customer.txt")); err != nil || string(data) != "preserved" {
		t.Fatalf("recovery changed protected content = %q, %v", data, err)
	}
}

func TestRuntimeMetricsCoordinationKeepsPeerClaimsIndependentDuringHandoff(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first := filepath.Join(root, "2026", "08", "24", "first.log")
	peer := filepath.Join(root, "2026", "08", "24", "peer.log")
	coordination := runtimeMetricsCoordination{}
	owner, err := coordination.Claim(first)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	peerOwner, err := coordination.TryClaim(peer)
	if err != nil {
		t.Fatalf("independent peer claim: %v", err)
	}
	t.Cleanup(func() { _ = peerOwner.Close() })
	if _, err := coordination.TryClaim(first); !errors.Is(err, ErrRuntimeMetricsArtifactBusy) {
		t.Fatalf("active selected claim = %v, want busy", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	replacement, err := coordination.TryClaim(first)
	if err != nil {
		t.Fatalf("released claim handoff: %v", err)
	}
	t.Cleanup(func() { _ = replacement.Close() })
	if err := owner.Close(); err != nil {
		t.Fatalf("old owner repeated close: %v", err)
	}
	for _, path := range []string{first, peer} {
		if _, err := coordination.TryClaim(path); !errors.Is(err, ErrRuntimeMetricsArtifactBusy) {
			t.Fatalf("handoff disturbed live claim %q: %v", path, err)
		}
	}
}

func TestRuntimeMetricsRootRetentionReclaimsReleasedStaleClaimAndMarker(t *testing.T) {
	root := t.TempDir()
	artifact := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "stale-claim-runtime-stale-collision", 9)
	claimPath := runtimeMetricsClaimPath(artifact)
	if err := os.MkdirAll(filepath.Dir(claimPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(claim directory): %v", err)
	}
	if err := os.WriteFile(claimPath, nil, 0o600); err != nil {
		t.Fatalf("WriteFile(stale claim): %v", err)
	}

	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	if _, err := retention.Sweep(context.Background(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	}); err != nil {
		t.Fatalf("Sweep(stale claim): %v", err)
	}
	if _, err := os.Stat(artifact); !os.IsNotExist(err) {
		t.Fatalf("artifact protected by released stale claim, stat error = %v", err)
	}
	if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
		t.Fatalf("released stale claim marker remains after cleanup, stat error = %v", err)
	}
}

func TestRuntimeMetricsRootRetentionPreservesLiveClaimMarkerAfterArtifactMissing(t *testing.T) {
	root := t.TempDir()
	artifact := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "live-missing-runtime-live-collision", 9)
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	claim, err := coordination.Claim(artifact)
	if err != nil {
		t.Fatalf("Claim(): %v", err)
	}
	claimPath := runtimeMetricsClaimPath(artifact)
	if err := os.Remove(artifact); err != nil {
		t.Fatalf("Remove(artifact): %v", err)
	}

	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	if _, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	}); err != nil {
		t.Fatalf("Sweep(live missing artifact): %v", err)
	}
	assertRetentionPathExists(t, claimPath, "live claim marker after missing-artifact sweep")

	if err := claim.Close(); err != nil {
		t.Fatalf("Close(live claim): %v", err)
	}
	if _, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	}); err != nil {
		t.Fatalf("Sweep(released missing artifact): %v", err)
	}
	assertRetentionPathAbsent(t, claimPath, "released claim marker after missing-artifact sweep")
}

func TestRuntimeMetricsRootRetentionProtectsArtifactChangedDuringClaim(t *testing.T) {
	root := t.TempDir()
	artifact := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "changed-runtime-changed-collision", 9)
	coordination := &metricsTestCoordination{
		tryRootLock: &metricsTestCloser{},
		tryClaim:    &metricsTestCloser{},
		onTryClaim: func(path string) {
			data, err := os.ReadFile(path)
			if err != nil {
				return
			}
			_ = os.WriteFile(path, append(data, 'x'), 0o600)
		},
	}
	retention, err := NewRuntimeMetricsRetention(
		platformfilesystem.Local{},
		func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) },
		coordination,
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Sweep(changed artifact): %v", err)
	}
	if report.Protected.Files != 1 || report.Protected.Bytes != 10 || report.Removed.Files != 0 {
		t.Fatalf("changed-artifact report = %#v, want protected 10-byte artifact", report)
	}
	assertRetentionPathExists(t, artifact, "changed artifact")
}

func TestRuntimeMetricsRootRetentionBoundsClaimMarkersAcrossEightCycles(t *testing.T) {
	root := t.TempDir()
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	for cycle := 0; cycle < 8; cycle++ {
		artifact := writeRetentionArtifact(
			t, root, "2026/07/01", fmt.Sprintf("%06d.000000000", cycle+1),
			fmt.Sprintf("session-%d-runtime-cycle-collision", cycle), 9,
		)
		claim, err := coordination.Claim(artifact)
		if err != nil {
			t.Fatalf("Claim(cycle %d): %v", cycle+1, err)
		}
		if err := claim.Close(); err != nil {
			t.Fatalf("Close(cycle %d): %v", cycle+1, err)
		}
		report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
			RootDirectory: root,
			Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
		})
		if err != nil {
			t.Fatalf("Sweep(cycle %d): %v", cycle+1, err)
		}
		if report.Removed.Files != 1 || report.After.Files != 0 {
			t.Fatalf("cycle %d report = %#v, want one removed artifact and no remaining artifacts", cycle+1, report)
		}
		if got := countRuntimeMetricsClaimMarkers(t, root); got != 0 {
			t.Fatalf("cycle %d claim marker count = %d, want zero", cycle+1, got)
		}
	}
}

func TestRuntimeMetricsRootRetentionPreservesMalformedClaimEntries(t *testing.T) {
	root := t.TempDir()
	artifact := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "malformed-runtime-malformed-collision", 9)
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	claim, err := coordination.Claim(artifact)
	if err != nil {
		t.Fatalf("Claim(): %v", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	claimsDirectory := filepath.Join(root, runtimeMetricsClaimsDirectory)
	malformed := filepath.Join(claimsDirectory, strings.Repeat("a", sha256HexLength)+runtimeMetricsClaimSuffix)
	if err := os.WriteFile(malformed, []byte("not empty"), 0o600); err != nil {
		t.Fatalf("WriteFile(malformed): %v", err)
	}
	unexpected := filepath.Join(claimsDirectory, "operator-note.txt")
	if err := os.WriteFile(unexpected, []byte("preserve"), 0o600); err != nil {
		t.Fatalf("WriteFile(unexpected): %v", err)
	}

	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Sweep(): %v", err)
	}
	if len(report.Failures) != 2 {
		t.Fatalf("Failures = %#v, want malformed and unexpected claim entries", report.Failures)
	}
	assertRetentionPathAbsent(t, runtimeMetricsClaimPath(artifact), "well-formed orphan claim marker")
	assertRetentionPathExists(t, malformed, "malformed claim marker")
	assertRetentionPathExists(t, unexpected, "unexpected claim entry")
}

func TestRuntimeMetricsRootRetentionRetriesFailedClaimMarkerRemoval(t *testing.T) {
	root := t.TempDir()
	artifact := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "retry-marker-runtime-retry-collision", 9)
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	claim, err := coordination.Claim(artifact)
	if err != nil {
		t.Fatalf("Claim(): %v", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	claimPath := runtimeMetricsClaimPath(artifact)
	filesystem := &failRuntimeMetricsClaimMarkerRemoveFileSystem{
		Local:    platformfilesystem.Local{},
		failPath: claimPath,
	}
	retention, err := NewRuntimeMetricsRetention(
		filesystem,
		func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
		coordination,
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Sweep(first): %v", err)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("first sweep failures = %#v, want marker removal failure", report.Failures)
	}
	assertRetentionPathExists(t, claimPath, "claim marker after failed removal")

	filesystem.failPath = ""
	if _, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1},
	}); err != nil {
		t.Fatalf("Sweep(retry): %v", err)
	}
	assertRetentionPathAbsent(t, claimPath, "claim marker after successful retry")
}

func countRuntimeMetricsClaimMarkers(t *testing.T, root string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, runtimeMetricsClaimsDirectory))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatalf("ReadDir(claim markers): %v", err)
	}
	count := 0
	for _, entry := range entries {
		if _, valid := runtimeMetricsClaimMarkerPath(filepath.Join(root, runtimeMetricsClaimsDirectory), entry.Name()); valid {
			count++
		}
	}
	return count
}

type failRuntimeMetricsClaimMarkerRemoveFileSystem struct {
	platformfilesystem.Local
	failPath string
}

func (filesystem *failRuntimeMetricsClaimMarkerRemoveFileSystem) Remove(path string) error {
	if path == filesystem.failPath {
		return errors.New("claim marker removal failed")
	}
	return filesystem.Local.Remove(path)
}

func TestRuntimeMetricsCoordinationClaimClosePreservesConcurrentReplacement(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(root, "2026", "08", "22", "010000.000000000-runtime-metrics-session-runtime-collision.log")
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		t.Fatalf("NewRuntimeMetricsCoordination(): %v", err)
	}
	initial, err := coordination.Claim(artifact)
	if err != nil {
		t.Fatalf("Claim(initial): %v", err)
	}
	claimPath := runtimeMetricsClaimPath(artifact)
	assertRetentionPathExists(t, claimPath, "initial stable claim marker")

	replacementReady := make(chan io.Closer, 1)
	replacementErr := make(chan error, 1)
	startReplacement := make(chan struct{})
	go func() {
		<-startReplacement
		for {
			replacement, claimErr := coordination.TryClaim(artifact)
			if claimErr == nil {
				replacementReady <- replacement
				return
			}
			if !errors.Is(claimErr, ErrRuntimeMetricsArtifactBusy) {
				replacementErr <- claimErr
				return
			}
			runtime.Gosched()
		}
	}()
	close(startReplacement)
	if err := initial.Close(); err != nil {
		t.Fatalf("Close(initial claim): %v", err)
	}

	var replacement io.Closer
	select {
	case err := <-replacementErr:
		t.Fatalf("TryClaim(replacement): %v", err)
	case replacement = <-replacementReady:
	}
	if err := replacement.Close(); err != nil {
		t.Fatalf("Close(replacement claim): %v", err)
	}
	assertRetentionPathExists(t, claimPath, "replacement stable claim marker")
}

func TestRuntimeMetricsRootRetentionPrunesDifferentCurrentFilenamesAcrossDates(t *testing.T) {
	root := t.TempDir()
	expired := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "historical-session-runtime-old-collision", 11)
	recent := writeRetentionArtifact(t, root, "2026/08/20", "020000.000000000", "new-session-runtime-new-collision", 17)
	unrecognized := filepath.Join(root, "2026", "07", "01", "keep.txt")
	if err := os.WriteFile(unrecognized, []byte("leave me"), 0o600); err != nil {
		t.Fatalf("WriteFile(unrecognized): %v", err)
	}

	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 10, MaxBackups: 1, MaxAge: 30},
	})
	if err != nil {
		t.Fatalf("Sweep(): %v", err)
	}

	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired artifact exists after root sweep, stat error = %v", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("recent artifact was removed by root sweep: %v", err)
	}
	if _, err := os.Stat(unrecognized); err != nil {
		t.Fatalf("unrecognized file was changed by root sweep: %v", err)
	}

	if report.Before != (RuntimeMetricsRetentionTotals{Files: 2, Bytes: 28}) {
		t.Fatalf("Before = %#v, want two recognized files and 28 bytes", report.Before)
	}
	if report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 17}) {
		t.Fatalf("After = %#v, want recent artifact only", report.After)
	}
	if report.Removed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 11}) {
		t.Fatalf("Removed = %#v, want expired artifact totals", report.Removed)
	}
	if report.Protected != (RuntimeMetricsRetentionTotals{}) || report.Failed != (RuntimeMetricsRetentionTotals{}) {
		t.Fatalf("unexpected protected/failed totals: protected=%#v failed=%#v", report.Protected, report.Failed)
	}
}

func TestRuntimeMetricsRootRetentionAppliesAgeThenOldestFirstAggregateSize(t *testing.T) {
	root := t.TempDir()
	oldest := writeRetentionArtifact(t, root, "2026/08/01", "010000.000000000", "session-a-runtime-a-unique-a", 600_000)
	middle := writeRetentionArtifact(t, root, "2026/08/10", "020000.000000000", "session-b-runtime-b-unique-b", 600_000)
	recent := writeRetentionArtifact(t, root, "2026/08/20", "030000.000000000", "session-c-runtime-c-unique-c", 600_000)

	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 365},
	})
	if err != nil {
		t.Fatalf("Sweep(): %v", err)
	}

	for name, path := range map[string]string{"oldest": oldest, "middle": middle} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s artifact remains after aggregate-size pruning, stat error = %v", name, err)
		}
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("newest artifact was not retained within aggregate budget: %v", err)
	}
	if report.Before != (RuntimeMetricsRetentionTotals{Files: 3, Bytes: 1_800_000}) {
		t.Fatalf("Before = %#v, want three artifacts and 1,800,000 bytes", report.Before)
	}
	if report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 600_000}) {
		t.Fatalf("After = %#v, want newest artifact only", report.After)
	}
	if report.Removed != (RuntimeMetricsRetentionTotals{Files: 2, Bytes: 1_200_000}) {
		t.Fatalf("Removed = %#v, want oldest two artifacts", report.Removed)
	}
}

func TestRuntimeMetricsRootRetentionDoesNotRemoveUnknownEntriesRecursively(t *testing.T) {
	root := t.TempDir()
	unsafeDir := filepath.Join(root, "2026", "07", "01")
	if err := os.MkdirAll(unsafeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(unsafeDir): %v", err)
	}
	unsafeArtifact := filepath.Join(unsafeDir, "010000.000000000-runtime-metrics-session-unsafe-runtime-unsafe.log")
	if err := os.WriteFile(unsafeArtifact, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile(unsafeArtifact): %v", err)
	}
	unknown := filepath.Join(unsafeDir, "operator-note.txt")
	if err := os.WriteFile(unknown, []byte("preserve"), 0o600); err != nil {
		t.Fatalf("WriteFile(unknown): %v", err)
	}

	safeArtifact := writeRetentionArtifact(t, root, "2026/07/02", "020000.000000000", "session-safe-runtime-safe-collision", 3)
	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	_, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 10, MaxBackups: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Sweep(): %v", err)
	}

	if _, err := os.Stat(unsafeArtifact); !os.IsNotExist(err) {
		t.Fatalf("eligible artifact in mixed directory remains, stat error = %v", err)
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatalf("unknown file was removed from mixed directory: %v", err)
	}
	if _, err := os.Stat(unsafeDir); err != nil {
		t.Fatalf("mixed date directory should remain for unknown file: %v", err)
	}
	if _, err := os.Stat(safeArtifact); !os.IsNotExist(err) {
		t.Fatalf("eligible complete date directory artifact remains, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(safeArtifact)); !os.IsNotExist(err) {
		t.Fatalf("complete eligible date directory remains, stat error = %v", err)
	}
}

func TestRuntimeMetricsRootRetentionDoesNotFollowRecognizedSymlink(t *testing.T) {
	root := t.TempDir()
	target := writeRetentionArtifact(t, t.TempDir(), "2026/07/01", "010000.000000000", "outside-runtime-target", 5)
	linkDir := filepath.Join(root, "2026", "07", "01")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(linkDir): %v", err)
	}
	link := filepath.Join(linkDir, "010000.000000000-runtime-metrics-session-link-runtime-link.log")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	retention := newTestRuntimeMetricsRetention(t, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC))
	report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Sweep(): %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("recognized symlink was removed or followed: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target was changed by root sweep: %v", err)
	}
	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat(recognized symlink): %v", err)
	}
	if report.Protected.Files != 1 || report.Protected.Bytes != linkInfo.Size() {
		t.Fatalf("Protected = %#v, want recognized symlink counted as protected", report.Protected)
	}
}

func TestRuntimeMetricsRootRetentionReportsDeterministicTotals(t *testing.T) {
	firstRoot := installRetentionReportFixture(t)
	secondRoot := installRetentionReportFixture(t)
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	first := newTestRuntimeMetricsRetention(t, now)
	second := newTestRuntimeMetricsRetention(t, now)
	request := func(root string) RuntimeMetricsRetentionRequest {
		return RuntimeMetricsRetentionRequest{
			RootDirectory: root,
			Config:        RuntimeMetricsConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 30},
		}
	}
	firstReport, err := first.Sweep(t.Context(), request(firstRoot))
	if err != nil {
		t.Fatalf("first Sweep(): %v", err)
	}
	secondReport, err := second.Sweep(t.Context(), request(secondRoot))
	if err != nil {
		t.Fatalf("second Sweep(): %v", err)
	}
	firstReport.RootDirectory = ""
	secondReport.RootDirectory = ""
	if !reflect.DeepEqual(firstReport, secondReport) {
		t.Fatalf("reports differ for equivalent root states:\nfirst=%#v\nsecond=%#v", firstReport, secondReport)
	}
}

func newTestRuntimeMetricsRetention(t *testing.T, now time.Time) *RuntimeMetricsRetention {
	t.Helper()
	retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	return retention
}

func TestRuntimeMetricsRetentionValidatesConstruction(t *testing.T) {
	if retention, err := NewRuntimeMetricsRetention(nil, time.Now); retention != nil || err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewRuntimeMetricsRetention(nil filesystem) = (%#v, %v)", retention, err)
	}
	if retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, nil); retention != nil || err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("NewRuntimeMetricsRetention(nil clock) = (%#v, %v)", retention, err)
	}
	coordination := &metricsTestCoordination{}
	if retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, coordination, coordination); retention != nil || err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("NewRuntimeMetricsRetention(two coordinators) = (%#v, %v)", retention, err)
	}
	if retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, nil); retention != nil || err == nil || !strings.Contains(err.Error(), "coordination is required") {
		t.Fatalf("NewRuntimeMetricsRetention(nil coordinator) = (%#v, %v)", retention, err)
	}

	var nilRetention *RuntimeMetricsRetention
	if _, err := nilRetention.Sweep(context.Background(), RuntimeMetricsRetentionRequest{RootDirectory: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "retention is not configured") {
		t.Fatalf("nil retention Sweep() = %v, want configuration error", err)
	}
}

func TestRuntimeMetricsRetentionValidatesSweepRequests(t *testing.T) {
	root := t.TempDir()
	coordination := &metricsTestCoordination{}
	retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, func() time.Time {
		return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	}, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := retention.Sweep(canceled, RuntimeMetricsRetentionRequest{RootDirectory: root}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Sweep() = %v, want context.Canceled", err)
	}
	for _, request := range []RuntimeMetricsRetentionRequest{
		{RootDirectory: ""},
		{RootDirectory: "   "},
		{RootDirectory: filepath.Join(root, "missing")},
	} {
		if _, err := retention.Sweep(nil, request); err == nil {
			t.Fatalf("Sweep(%#v) succeeded, want request validation failure", request)
		}
	}
	fileRoot := filepath.Join(root, "file")
	if err := os.WriteFile(fileRoot, []byte("not a root"), 0o600); err != nil {
		t.Fatalf("WriteFile(file root): %v", err)
	}
	if _, err := retention.Sweep(nil, RuntimeMetricsRetentionRequest{RootDirectory: fileRoot}); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("Sweep(file root) = %v, want directory validation", err)
	}

	zeroClock, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, func() time.Time { return time.Time{} }, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(zero clock): %v", err)
	}
	if _, err := zeroClock.Sweep(nil, RuntimeMetricsRetentionRequest{RootDirectory: root}); err == nil || !strings.Contains(err.Error(), "clock returned zero") {
		t.Fatalf("Sweep(zero clock) = %v, want clock validation", err)
	}
}

func TestRuntimeMetricsRetentionReportsCoordinationOutcomes(t *testing.T) {
	root := t.TempDir()
	coordination := &metricsTestCoordination{}
	retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, coordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	busyCoordination := &metricsTestCoordination{tryLockRootErr: ErrRuntimeMetricsRootBusy}
	busyRetention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, busyCoordination)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(busy): %v", err)
	}
	busyReport, err := busyRetention.Sweep(nil, RuntimeMetricsRetentionRequest{RootDirectory: root})
	if err != nil || !busyReport.Skipped || busyReport.RootDirectory != filepath.Clean(root) {
		t.Fatalf("Sweep(busy) = %#v, %v, want skipped root report", busyReport, err)
	}

	coordination.tryLockRootErr = errors.New("root coordination failed")
	if _, err := retention.Sweep(nil, RuntimeMetricsRetentionRequest{RootDirectory: root}); err == nil || !strings.Contains(err.Error(), "coordinate runtime metrics sweep") {
		t.Fatalf("Sweep(coordination failure) = %v, want coordination context", err)
	}
	coordination.tryLockRootErr = nil
	coordination.tryRootLock = &metricsTestCloser{err: errors.New("root close failed")}
	if _, err := retention.Sweep(nil, RuntimeMetricsRetentionRequest{RootDirectory: root}); err == nil || !strings.Contains(err.Error(), "release runtime metrics sweep coordination") {
		t.Fatalf("Sweep(root close failure) = %v, want release context", err)
	}

	if _, err := os.Stat(filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("validation fixture changed unexpectedly: %v", err)
	}
}

func TestRuntimeMetricsRetentionPreservesFailureReportsDuringInventoryAndRemoval(t *testing.T) {
	root := t.TempDir()
	artifact := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "failure-runtime-failure-collision", 9)
	failureFS := &retentionFailureFileSystem{Local: platformfilesystem.Local{}, removeErr: errors.New("remove denied")}
	coordination := &metricsTestCoordination{
		tryRootLock: &metricsTestCloser{},
		tryClaim:    &metricsTestCloser{},
	}
	retention, err := NewRuntimeMetricsRetention(
		failureFS,
		func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) },
		coordination,
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(): %v", err)
	}
	report, err := retention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{
		RootDirectory: root,
		Config:        RuntimeMetricsConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	})
	if err != nil {
		t.Fatalf("Sweep(remove failure): %v", err)
	}
	if report.Failed.Files != 1 || report.Removed.Files != 0 || len(report.Failures) == 0 {
		t.Fatalf("remove-failure report = %#v, want one protected candidate failure", report)
	}
	assertRetentionPathExists(t, artifact, "failed removal artifact")

	walkRoot := t.TempDir()
	walkFS := &incompleteRetentionFileSystem{Local: platformfilesystem.Local{}, walkErr: errors.New("directory disappeared")}
	walkRetention, err := NewRuntimeMetricsRetention(
		walkFS,
		func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) },
		&metricsTestCoordination{tryRootLock: &metricsTestCloser{}},
	)
	if err != nil {
		t.Fatalf("NewRuntimeMetricsRetention(incomplete): %v", err)
	}
	walkReport, err := walkRetention.Sweep(t.Context(), RuntimeMetricsRetentionRequest{RootDirectory: walkRoot})
	if err != nil {
		t.Fatalf("Sweep(incomplete inventory): %v", err)
	}
	if len(walkReport.Failures) == 0 || !strings.Contains(walkReport.Failures[len(walkReport.Failures)-1].Error.Error(), "inventory is incomplete") {
		t.Fatalf("incomplete inventory report = %#v, want cleanup-skip failure", walkReport)
	}
}

type retentionFailureFileSystem struct {
	platformfilesystem.Local
	removeErr     error
	failPath      string
	lstatErr      error
	readDirErr    error
	walkErr       error
	walkReturnErr error
	failWalkCall  int
	walkCalls     int
}

func (filesystem *retentionFailureFileSystem) WalkDir(root string, visit fs.WalkDirFunc) error {
	filesystem.walkCalls++
	if filesystem.walkCalls == filesystem.failWalkCall {
		return filesystem.walkReturnErr
	}
	return filesystem.Local.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if path == filesystem.failPath && filesystem.walkErr != nil {
			return visit(path, entry, filesystem.walkErr)
		}
		return visit(path, entry, walkErr)
	})
}

// A failed initial inventory must leave artifacts intact. A failed final
// inventory cannot undo safe pruning, but must retain its cause alongside root
// release failure and leave orphan claims for a later complete sweep.
func TestRuntimeMetricsRetentionInventoryFailureReleasesRootAndRecovers(t *testing.T) {
	t.Parallel()
	for _, stage := range []struct {
		name string
		walk int
	}{
		{name: "before pruning", walk: 1},
		{name: "after pruning", walk: 2},
	} {
		for _, failure := range []struct {
			name  string
			cause error
		}{
			{name: "filesystem rejected", cause: fs.ErrPermission},
			{name: "filesystem canceled", cause: context.Canceled},
		} {
			t.Run(stage.name+"/"+failure.name, func(t *testing.T) {
				t.Parallel()
				assertRetentionInventoryFailureRecovery(t, stage.walk, failure.cause)
			})
		}
	}
}

func assertRetentionInventoryFailureRecovery(t *testing.T, failedWalk int, cause error) {
	t.Helper()
	root, healthy, marker, unknown := installOrphanMarkerRecoveryFixture(t)
	expired := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "expired-runtime-expired", 11)
	closeCause := errors.New("release root rejected")
	rootLock, claim, markerLock := &metricsTestCloser{err: closeCause}, &metricsTestCloser{}, &metricsTestCloser{}
	coordination := &metricsTestCoordination{tryRootLock: rootLock, tryClaim: claim, tryClaimMarker: markerLock}
	filesystem := &retentionFailureFileSystem{
		Local: platformfilesystem.Local{}, failWalkCall: failedWalk, walkReturnErr: cause,
	}
	retention, err := NewRuntimeMetricsRetention(filesystem, func() time.Time {
		return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	}, coordination)
	if err != nil {
		t.Fatal(err)
	}
	request := RuntimeMetricsRetentionRequest{RootDirectory: root, Config: RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1}}
	report, err := retention.Sweep(t.Context(), request)
	if !errors.Is(err, cause) || !errors.Is(err, closeCause) || !strings.Contains(err.Error(), strconv.Quote(root)) {
		t.Fatalf("failed inventory = %#v, %v, want root and both causes", report, err)
	}
	assertFailedInventoryReleases(t, report, failedWalk-1, rootLock, claim, markerLock)
	if failedWalk == 1 {
		assertRetentionPreservedContent(t, expired, "mmmmmmmmmmm")
	} else {
		assertRetentionPathAbsent(t, expired, "safely pruned before inventory failure")
	}
	assertRetentionPathExists(t, marker, "orphan marker after failed inventory")
	assertRetentionPreservedContent(t, healthy, "mmmmmmmmm")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
	filesystem.failWalkCall = 0
	rootLock.err = nil
	report, err = retention.Sweep(t.Context(), request)
	if err != nil || len(report.Failures) != 0 || report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) {
		t.Fatalf("recovered inventory = %#v, %v", report, err)
	}
	if rootLock.closed != 2 || claim.closed != 1 || markerLock.closed != 1 {
		t.Fatalf("recovery releases root=%d claim=%d marker=%d, want 2/1/1", rootLock.closed, claim.closed, markerLock.closed)
	}
	assertRetentionPathAbsent(t, expired, "expired artifact after recovery")
	assertRetentionPathAbsent(t, marker, "orphan marker after complete inventory")
	assertRetentionPreservedContent(t, healthy, "mmmmmmmmm")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
}

func assertFailedInventoryReleases(
	t *testing.T, report RuntimeMetricsRetentionReport, wantClaims int,
	rootLock, claim, markerLock *metricsTestCloser,
) {
	t.Helper()
	if rootLock.closed != 1 || claim.closed != wantClaims || markerLock.closed != 0 {
		t.Fatalf("failure releases root=%d claim=%d marker=%d, want 1/%d/0", rootLock.closed, claim.closed, markerLock.closed, wantClaims)
	}
	if report.Removed != (RuntimeMetricsRetentionTotals{Files: wantClaims, Bytes: int64(wantClaims * 11)}) {
		t.Fatalf("failed inventory removal = %#v", report.Removed)
	}
}

// An incomplete inventory must preserve orphan claims, while independently
// inspected expired artifacts remain eligible. Retrying the same component
// after the filesystem recovers must remove only safe metrics and markers.
func TestRuntimeMetricsRetentionIncompleteInventoryPreservesClaimsAndRecovers(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"artifact inspection", "partial walk"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			root, healthy, marker, unknown := installOrphanMarkerRecoveryFixture(t)
			selected := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "selected-runtime-selected", 11)
			peer := writeRetentionArtifact(t, root, "2026/07/02", "010000.000000000", "peer-runtime-peer", 7)
			cause := errors.New("selected inventory operation rejected")
			filesystem := &retentionFailureFileSystem{Local: platformfilesystem.Local{}, failPath: selected}
			if failure == "artifact inspection" {
				filesystem.lstatErr = cause
			} else {
				filesystem.walkErr = cause
			}
			rootLock, claim, markerLock := &metricsTestCloser{}, &metricsTestCloser{}, &metricsTestCloser{}
			coordination := &metricsTestCoordination{tryRootLock: rootLock, tryClaim: claim, tryClaimMarker: markerLock}
			retention, err := NewRuntimeMetricsRetention(filesystem, func() time.Time {
				return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
			}, coordination)
			if err != nil {
				t.Fatal(err)
			}
			request := RuntimeMetricsRetentionRequest{RootDirectory: root, Config: RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1}}
			report, err := retention.Sweep(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			assertIncompleteRetentionInventory(t, report, selected, marker, cause)
			if rootLock.closed != 1 || claim.closed != 1 || markerLock.closed != 0 {
				t.Fatalf("failure releases root=%d claim=%d marker=%d, want 1/1/0", rootLock.closed, claim.closed, markerLock.closed)
			}
			assertRetentionPathAbsent(t, peer, "independently expired peer")
			assertRetentionPathExists(t, marker, "orphan claim after incomplete inventory")
			assertRetentionPreservedContent(t, selected, "mmmmmmmmmmm")
			assertRetentionPreservedContent(t, healthy, "mmmmmmmmm")
			assertRetentionPreservedContent(t, unknown, "preserve customer content")
			filesystem.failPath = ""
			report, err = retention.Sweep(t.Context(), request)
			if err != nil || len(report.Failures) != 0 || report.Removed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 11}) || report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) {
				t.Fatalf("recovered sweep = %#v, %v", report, err)
			}
			if rootLock.closed != 2 || claim.closed != 2 || markerLock.closed != 1 {
				t.Fatalf("recovery releases root=%d claim=%d marker=%d, want 2/2/1", rootLock.closed, claim.closed, markerLock.closed)
			}
			assertRetentionPathAbsent(t, selected, "recovered expired artifact")
			assertRetentionPathAbsent(t, marker, "recovered orphan claim")
			assertRetentionPreservedContent(t, healthy, "mmmmmmmmm")
			assertRetentionPreservedContent(t, unknown, "preserve customer content")
		})
	}
}

func assertIncompleteRetentionInventory(t *testing.T, report RuntimeMetricsRetentionReport, selected, marker string, cause error) {
	t.Helper()
	if report.Failed.Files != 1 || report.Removed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 7}) || len(report.Failures) != 2 {
		t.Fatalf("incomplete sweep = %#v", report)
	}
	if report.Failures[0].Path != selected || !errors.Is(report.Failures[0].Error, cause) || report.Failures[1].Path != filepath.Dir(marker) || !strings.Contains(report.Failures[1].Error.Error(), "inventory is incomplete") {
		t.Fatalf("incomplete inventory diagnostics = %#v", report.Failures)
	}
}

func (filesystem *retentionFailureFileSystem) Remove(path string) error {
	if filesystem.removeErr != nil && (filesystem.failPath == "" || path == filesystem.failPath) {
		return filesystem.removeErr
	}
	return filesystem.Local.Remove(path)
}

func TestRuntimeMetricsRetentionRetriesRejectedArtifactWithoutBlockingPeerPruning(t *testing.T) {
	for _, failure := range []string{"busy claim", "claim error", "claimed inspection", "removal"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			selected := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "selected-runtime-selected", 9)
			peer := writeRetentionArtifact(t, root, "2026/07/02", "010000.000000000", "peer-runtime-peer", 7)
			unknown := filepath.Join(root, "customer-note.txt")
			if err := os.WriteFile(unknown, []byte("preserve customer content"), 0o600); err != nil {
				t.Fatal(err)
			}
			cause := errors.New("selected artifact operation failed")
			rootLock, claim := &metricsTestCloser{}, &metricsTestCloser{}
			filesystem := &retentionFailureFileSystem{Local: platformfilesystem.Local{}}
			coordination := &metricsTestCoordination{tryRootLock: rootLock, tryClaim: claim}
			coordination.onTryClaim = func(path string) {
				configureRejectedArtifactClaim(failure, path, selected, cause, filesystem, coordination)
			}
			retention, err := NewRuntimeMetricsRetention(filesystem, func() time.Time {
				return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
			}, coordination)
			if err != nil {
				t.Fatal(err)
			}
			request := RuntimeMetricsRetentionRequest{RootDirectory: root, Config: RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1}}
			report, err := retention.Sweep(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			assertRejectedArtifactSweep(t, failure, report, selected, cause, rootLock, claim)
			assertRetentionPathAbsent(t, peer, "independently pruned peer")
			assertRetentionPreservedContent(t, selected, "mmmmmmmmm")
			assertRetentionPreservedContent(t, unknown, "preserve customer content")
			coordination.onTryClaim, coordination.tryClaimErr = nil, nil
			filesystem.failPath, filesystem.lstatErr, filesystem.removeErr = "", nil, nil
			report, err = retention.Sweep(t.Context(), request)
			if err != nil || len(report.Failures) != 0 || report.Removed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) || report.After.Files != 0 {
				t.Fatalf("recovered sweep = %#v, %v", report, err)
			}
			wantClosed := 2
			if failure == "claimed inspection" || failure == "removal" {
				wantClosed++
			}
			if rootLock.closed != 2 || claim.closed != wantClosed {
				t.Fatalf("recovery releases: root=%d claim=%d, want 2/%d", rootLock.closed, claim.closed, wantClosed)
			}
			assertRetentionPathAbsent(t, selected, "pruned recovered artifact")
			assertRetentionPreservedContent(t, unknown, "preserve customer content")
		})
	}
}

func configureRejectedArtifactClaim(
	failure, path, selected string, cause error,
	filesystem *retentionFailureFileSystem, coordination *metricsTestCoordination,
) {
	coordination.tryClaimErr = nil
	if path != selected {
		return
	}
	switch failure {
	case "busy claim":
		coordination.tryClaimErr = ErrRuntimeMetricsArtifactBusy
	case "claim error":
		coordination.tryClaimErr = cause
	case "claimed inspection":
		filesystem.failPath, filesystem.lstatErr = selected, cause
	case "removal":
		filesystem.failPath, filesystem.removeErr = selected, cause
	}
}

func assertRejectedArtifactSweep(
	t *testing.T, failure string, report RuntimeMetricsRetentionReport,
	selected string, cause error, rootLock, claim *metricsTestCloser,
) {
	t.Helper()
	if report.Removed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 7}) {
		t.Fatalf("removed = %#v, want only the healthy peer", report.Removed)
	}
	if failure == "busy claim" {
		if report.Protected != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) || len(report.Failures) != 0 || report.Failed.Files != 0 {
			t.Fatalf("busy artifact report = %#v", report)
		}
	} else {
		if report.Failed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) || len(report.Failures) == 0 || report.Failures[0].Path != selected || !errors.Is(report.Failures[0].Error, cause) {
			t.Fatalf("failed artifact report = %#v, want selected path and cause", report)
		}
	}
	wantClosed := 1
	if failure == "claimed inspection" || failure == "removal" {
		wantClosed++
	}
	if rootLock.closed != 1 || claim.closed != wantClosed {
		t.Fatalf("failed sweep releases: root=%d claim=%d, want 1/%d", rootLock.closed, claim.closed, wantClosed)
	}
}

func (filesystem *retentionFailureFileSystem) Lstat(path string) (fs.FileInfo, error) {
	if path == filesystem.failPath && filesystem.lstatErr != nil {
		return nil, filesystem.lstatErr
	}
	return filesystem.Local.Lstat(path)
}

func (filesystem *retentionFailureFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	if path == filesystem.failPath && filesystem.readDirErr != nil {
		return nil, filesystem.readDirErr
	}
	return filesystem.Local.ReadDir(path)
}

func TestRuntimeMetricsRetentionPreservesOrphanMarkerOnFailureAndReapsAfterRecovery(t *testing.T) {
	for _, failure := range []string{"busy", "claim", "nil lock", "close", "marker inspection", "directory inspection", "directory read"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			root, artifact, marker, unknown := installOrphanMarkerRecoveryFixture(t)
			claimsDirectory := filepath.Dir(marker)
			cause := errors.New("selected marker operation failed")
			rootLock := &metricsTestCloser{}
			markerLock := &metricsTestCloser{}
			coordination := &metricsTestCoordination{tryRootLock: rootLock, tryClaimMarker: markerLock}
			filesystem := &retentionFailureFileSystem{Local: platformfilesystem.Local{}}
			configureOrphanMarkerFailure(failure, cause, marker, filesystem, coordination, markerLock)
			retention, err := NewRuntimeMetricsRetention(filesystem, func() time.Time {
				return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
			}, coordination)
			if err != nil {
				t.Fatal(err)
			}
			request := RuntimeMetricsRetentionRequest{RootDirectory: root, Config: RuntimeMetricsConfig{MaxSize: 1, MaxAge: 1}}
			report, err := retention.Sweep(t.Context(), request)
			if err != nil {
				t.Fatalf("Sweep(failed marker cleanup): %v", err)
			}
			assertOrphanMarkerFailureReport(t, failure, report, marker, claimsDirectory, cause)
			wantClosed := 0
			if failure == "close" {
				wantClosed = 1
			}
			if rootLock.closed != 1 || markerLock.closed != wantClosed {
				t.Fatalf("released locks: root=%d marker=%d, want 1/%d", rootLock.closed, markerLock.closed, wantClosed)
			}
			assertRetentionPathExists(t, marker, "marker after rejected cleanup")

			// Recover the selected effect on the same retention owner. No retry
			// may erase the healthy artifact or unrelated customer content.
			filesystem.failPath = ""
			coordination.tryClaimMarkerErr = nil
			recoveredLock := &metricsTestCloser{}
			coordination.tryClaimMarker = recoveredLock
			report, err = retention.Sweep(t.Context(), request)
			if err != nil || len(report.Failures) != 0 || report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) {
				t.Fatalf("Sweep(recovered cleanup) = %#v, %v", report, err)
			}
			if rootLock.closed != 2 || recoveredLock.closed != 1 {
				t.Fatalf("recovery releases: root=%d marker=%d, want 2/1", rootLock.closed, recoveredLock.closed)
			}
			assertRetentionPathAbsent(t, marker, "recovered orphan marker")
			assertRetentionPreservedContent(t, artifact, "mmmmmmmmm")
			assertRetentionPreservedContent(t, unknown, "preserve customer content")
		})
	}
}

// Marker cleanup never removes customer content or traverses a replacement
// directory. Repairing the selected path must permit the same owner to recover.
func TestRuntimeMetricsRetentionProtectsUnsafeMarkerPathsAndRecovers(t *testing.T) {
	for _, replacement := range []string{"nonempty marker", "marker directory", "claims directory file"} {
		t.Run(replacement, func(t *testing.T) {
			t.Parallel()
			assertUnsafeMarkerRecovery(t, replacement)
		})
	}
}

func assertUnsafeMarkerRecovery(t *testing.T, replacement string) {
	t.Helper()
	root, retained, marker, unknown := installOrphanMarkerRecoveryFixture(t)
	expired := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "expired-runtime-expired", 7)
	selected, contentPath, diagnostic := replaceRetentionMarkerPath(t, replacement, marker)
	rootLock, artifactLock, markerLock := &metricsTestCloser{}, &metricsTestCloser{}, &metricsTestCloser{}
	coordination := &metricsTestCoordination{tryRootLock: rootLock, tryClaim: artifactLock, tryClaimMarker: markerLock}
	retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, func() time.Time {
		return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	}, coordination)
	if err != nil {
		t.Fatal(err)
	}
	request := RuntimeMetricsRetentionRequest{RootDirectory: root, Config: RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1}}
	report, err := retention.Sweep(t.Context(), request)
	assertUnsafeMarkerReport(t, report, err, selected, diagnostic)
	if rootLock.closed != 1 || artifactLock.closed != 1 || markerLock.closed != 0 {
		t.Fatalf("unsafe releases root=%d artifact=%d marker=%d, want 1/1/0", rootLock.closed, artifactLock.closed, markerLock.closed)
	}
	assertRetentionPathAbsent(t, expired, "independently pruned peer")
	assertRetentionPreservedContent(t, contentPath, "replacement customer content")
	assertRetentionPreservedContent(t, retained, "mmmmmmmmm")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
	repairRetentionMarkerPath(t, replacement, marker, contentPath)
	report, err = retention.Sweep(t.Context(), request)
	if err != nil || len(report.Failures) != 0 || report.Removed.Files != 0 ||
		report.Before != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) || report.After != report.Before {
		t.Fatalf("recovered marker sweep = %#v, %v", report, err)
	}
	if rootLock.closed != 2 || artifactLock.closed != 1 || markerLock.closed != 1 {
		t.Fatalf("recovery releases root=%d artifact=%d marker=%d, want 2/1/1", rootLock.closed, artifactLock.closed, markerLock.closed)
	}
	assertRetentionPathAbsent(t, marker, "recovered zero-byte orphan marker")
	assertRetentionPreservedContent(t, retained, "mmmmmmmmm")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
}

func assertUnsafeMarkerReport(t *testing.T, report RuntimeMetricsRetentionReport, err error, selected, diagnostic string) {
	t.Helper()
	if err != nil || report.Before != (RuntimeMetricsRetentionTotals{Files: 2, Bytes: 16}) ||
		report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) ||
		report.Removed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 7}) ||
		report.Failed.Files != 0 || report.Protected.Files != 0 {
		t.Fatalf("unsafe marker sweep = %#v, %v", report, err)
	}
	if len(report.Failures) != 1 || report.Failures[0].Path != selected || report.Failures[0].Error.Error() != diagnostic {
		t.Fatalf("unsafe marker diagnostic = %#v, want %q at %q", report.Failures, diagnostic, selected)
	}
}

func replaceRetentionMarkerPath(t *testing.T, replacement, marker string) (selected, contentPath, diagnostic string) {
	t.Helper()
	selected, contentPath = marker, marker
	diagnostic = "claim marker is not a zero-byte regular file"
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	switch replacement {
	case "marker directory":
		if err := os.Mkdir(marker, 0o700); err != nil {
			t.Fatal(err)
		}
		contentPath = filepath.Join(marker, "customer.txt")
	case "claims directory file":
		selected, contentPath = filepath.Dir(marker), filepath.Dir(marker)
		diagnostic = "claim marker path is not a directory"
		if err := os.Remove(selected); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(contentPath, []byte("replacement customer content"), 0o600); err != nil {
		t.Fatal(err)
	}
	return selected, contentPath, diagnostic
}

func repairRetentionMarkerPath(t *testing.T, replacement, marker, contentPath string) {
	t.Helper()
	// Delete only explicitly created scenario-owned paths, after preservation
	// assertions. Never recursively remove the protected replacement.
	if err := os.Remove(contentPath); err != nil {
		t.Fatal(err)
	}
	if replacement == "marker directory" {
		if err := os.Remove(marker); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func installOrphanMarkerRecoveryFixture(t *testing.T) (root, artifact, marker, unknown string) {
	t.Helper()
	root = t.TempDir()
	artifact = writeRetentionArtifact(t, root, "2026/08/24", "010000.000000000", "retained-runtime-retained", 9)
	claimsDirectory := filepath.Join(root, runtimeMetricsClaimsDirectory)
	if err := os.MkdirAll(claimsDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	marker = filepath.Join(claimsDirectory, strings.Repeat("a", sha256HexLength)+runtimeMetricsClaimSuffix)
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	unknown = filepath.Join(root, "customer-note.txt")
	if err := os.WriteFile(unknown, []byte("preserve customer content"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, artifact, marker, unknown
}

func configureOrphanMarkerFailure(
	failure string, cause error, marker string,
	filesystem *retentionFailureFileSystem,
	coordination *metricsTestCoordination,
	markerLock *metricsTestCloser,
) {
	switch failure {
	case "busy":
		coordination.tryClaimMarkerErr = ErrRuntimeMetricsArtifactBusy
	case "claim":
		coordination.tryClaimMarkerErr = cause
	case "nil lock":
		coordination.tryClaimMarker = nil
	case "close":
		markerLock.err = cause
	case "marker inspection":
		filesystem.failPath, filesystem.lstatErr = marker, cause
	case "directory inspection":
		filesystem.failPath, filesystem.lstatErr = filepath.Dir(marker), cause
	case "directory read":
		filesystem.failPath, filesystem.readDirErr = filepath.Dir(marker), cause
	}
}

func assertOrphanMarkerFailureReport(
	t *testing.T, failure string, report RuntimeMetricsRetentionReport,
	marker, claimsDirectory string, cause error,
) {
	t.Helper()
	wantFailures := 1
	if failure == "busy" {
		wantFailures = 0
	}
	if len(report.Failures) != wantFailures || report.Removed.Files != 0 || report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) {
		t.Fatalf("failed cleanup report = %#v, want preserved artifact and %d failures", report, wantFailures)
	}
	if wantFailures == 0 {
		return
	}
	wantPath := marker
	if strings.HasPrefix(failure, "directory") {
		wantPath = claimsDirectory
	}
	if report.Failures[0].Path != wantPath || (failure != "nil lock" && !errors.Is(report.Failures[0].Error, cause)) {
		t.Fatalf("failure = %#v, want selected path and original cause", report.Failures[0])
	}
}

func assertRetentionPreservedContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("preserved file %q = %q, %v, want %q", path, got, err, want)
	}
}

type incompleteRetentionFileSystem struct {
	platformfilesystem.Local
	walkErr error
}

func (filesystem *incompleteRetentionFileSystem) WalkDir(root string, walk fs.WalkDirFunc) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if err := walk(root, fs.FileInfoToDirEntry(info), nil); err != nil {
		return err
	}
	return walk(filepath.Join(root, "010000.000000000-runtime-metrics-failed-runtime-failed.log"), nil, filesystem.walkErr)
}

// Changes at the existing claim/removal effects model another owner changing a
// candidate after inventory. Sweep must revalidate it without recursive removal.
func TestRuntimeMetricsRetentionRevalidatesDisappearedOrReplacedArtifactAndRecovers(t *testing.T) {
	for _, transition := range []string{
		"missing before claim", "directory before claim", "inspection failure before claim",
		"missing after claim", "directory after claim", "missing during removal",
	} {
		t.Run(transition, func(t *testing.T) {
			t.Parallel()
			assertRetentionArtifactTransitionRecovery(t, transition)
		})
	}
}

func assertRetentionArtifactTransitionRecovery(t *testing.T, transition string) {
	t.Helper()
	root := t.TempDir()
	selected := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "selected-runtime-selected", 11)
	peer := writeRetentionArtifact(t, root, "2026/07/02", "010000.000000000", "peer-runtime-peer", 7)
	unknown := filepath.Join(root, "customer-note.txt")
	if err := os.WriteFile(unknown, []byte("preserve customer content"), 0o600); err != nil {
		t.Fatal(err)
	}
	rootLock, claim := &metricsTestCloser{}, &metricsTestCloser{}
	coordination := &metricsTestCoordination{tryRootLock: rootLock, tryClaim: claim}
	filesystem := &retentionTransitionFileSystem{Local: platformfilesystem.Local{}}
	configureRetentionArtifactTransition(t, transition, selected, filesystem, coordination)
	retention, err := NewRuntimeMetricsRetention(filesystem, func() time.Time {
		return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	}, coordination)
	if err != nil {
		t.Fatal(err)
	}
	request := RuntimeMetricsRetentionRequest{RootDirectory: root, Config: RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1}}
	report, err := retention.Sweep(t.Context(), request)
	assertRetentionArtifactTransitionReport(t, transition, report, err, selected)
	wantClaims := 2
	if strings.HasSuffix(transition, "before claim") {
		wantClaims = 1
	}
	if rootLock.closed != 1 || claim.closed != wantClaims {
		t.Fatalf("transition releases: root=%d claim=%d, want 1/%d", rootLock.closed, claim.closed, wantClaims)
	}
	assertRetentionPathAbsent(t, peer, "independently pruned peer")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
	coordination.onTryClaim, filesystem.beforeRemove, filesystem.beforeReadDir = nil, nil, nil
	restoreRetentionArtifact(t, transition, selected)
	report, err = retention.Sweep(t.Context(), request)
	if err != nil || len(report.Failures) != 0 || report.Removed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 11}) || report.After.Files != 0 || report.Protected.Files != 0 {
		t.Fatalf("recovered sweep = %#v, %v", report, err)
	}
	if rootLock.closed != 2 || claim.closed != wantClaims+1 {
		t.Fatalf("recovery releases: root=%d claim=%d, want 2/%d", rootLock.closed, claim.closed, wantClaims+1)
	}
	assertRetentionPathAbsent(t, selected, "recovered eligible artifact")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
}

func configureRetentionArtifactTransition(
	t *testing.T, transition, selected string,
	filesystem *retentionTransitionFileSystem, coordination *metricsTestCoordination,
) {
	t.Helper()
	change := func(path string) {
		changeRetentionArtifact(t, transition, selected, path)
	}
	switch {
	case strings.HasSuffix(transition, "before claim"):
		// The directory read follows inventory and precedes candidate inspection.
		// Change only this scenario's selected artifact at that filesystem effect.
		filesystem.beforeReadDir = func(path string) {
			if path != filepath.Dir(selected) {
				return
			}
			filesystem.beforeReadDir = nil
			if transition == "inspection failure before claim" {
				filesystem.rejectedInspection = selected
				return
			}
			change(selected)
		}
	case transition == "missing during removal":
		filesystem.beforeRemove = change
	default:
		coordination.onTryClaim = change
	}
}

func changeRetentionArtifact(t *testing.T, transition, selected, path string) {
	t.Helper()
	if path != selected {
		return
	}
	if err := os.Remove(selected); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(transition, "directory ") {
		if err := os.Mkdir(selected, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(selected, "customer.txt"), []byte("replacement content"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func restoreRetentionArtifact(t *testing.T, transition, selected string) {
	t.Helper()
	if strings.HasPrefix(transition, "directory ") {
		// Remove only the scenario-owned replacement after proving its bytes
		// survived the failed candidate validation; never recursively delete it.
		if err := os.Remove(filepath.Join(selected, "customer.txt")); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(selected); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(selected), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(selected, bytes.Repeat([]byte("m"), 11), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertRetentionArtifactTransitionReport(
	t *testing.T, transition string, report RuntimeMetricsRetentionReport, err error, selected string,
) {
	t.Helper()
	if transition == "inspection failure before claim" {
		assertRetentionRejectedInspectionReport(t, report, err, selected)
		return
	}
	if err != nil || len(report.Failures) != 0 || report.Failed.Files != 0 || report.Before != (RuntimeMetricsRetentionTotals{Files: 2, Bytes: 18}) || report.After.Files != 0 {
		t.Fatalf("transition sweep = %#v, %v", report, err)
	}
	if strings.HasPrefix(transition, "directory ") {
		info, statErr := os.Stat(selected)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if !info.IsDir() || report.Protected != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: info.Size()}) || report.Removed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 7}) {
			t.Fatalf("replacement protection = %#v, info=%v", report, info)
		}
		assertRetentionPreservedContent(t, filepath.Join(selected, "customer.txt"), "replacement content")
		return
	}
	if report.Protected.Files != 0 || report.Removed != (RuntimeMetricsRetentionTotals{Files: 2, Bytes: 18}) {
		t.Fatalf("already missing candidate = %#v", report)
	}
	assertRetentionPathAbsent(t, selected, "already removed candidate")
}

func assertRetentionRejectedInspectionReport(t *testing.T, report RuntimeMetricsRetentionReport, err error, selected string) {
	t.Helper()
	if err != nil || report.Before != (RuntimeMetricsRetentionTotals{Files: 2, Bytes: 18}) ||
		report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 11}) ||
		report.Removed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 7}) ||
		report.Failed != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 11}) || report.Protected.Files != 0 {
		t.Fatalf("rejected inspection report = %#v, %v", report, err)
	}
	if len(report.Failures) != 1 || report.Failures[0].Path != selected || !errors.Is(report.Failures[0].Error, fs.ErrPermission) {
		t.Fatalf("inspection failure = %#v, want selected path and permission cause", report.Failures)
	}
	assertRetentionPreservedContent(t, selected, "mmmmmmmmmmm")
}

type retentionTransitionFileSystem struct {
	platformfilesystem.Local
	beforeRemove       func(string)
	beforeReadDir      func(string)
	rejectedInspection string
}

func (filesystem *retentionTransitionFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	if filesystem.beforeReadDir != nil {
		filesystem.beforeReadDir(path)
	}
	if path == filepath.Dir(filesystem.rejectedInspection) {
		// Reject whole-directory pruning so the candidate's own inspection
		// reports the fault. The final inventory must still see its intact bytes.
		return nil, fs.ErrPermission
	}
	return filesystem.Local.ReadDir(path)
}

func (filesystem *retentionTransitionFileSystem) Lstat(path string) (fs.FileInfo, error) {
	if path == filesystem.rejectedInspection {
		filesystem.rejectedInspection = ""
		return nil, fs.ErrPermission
	}
	return filesystem.Local.Lstat(path)
}

func (filesystem *retentionTransitionFileSystem) Remove(path string) error {
	if filesystem.beforeRemove != nil {
		filesystem.beforeRemove(path)
	}
	return filesystem.Local.Remove(path)
}

// Cancellation is delivered at the existing filesystem boundary, after Sweep
// has acquired its root. A fresh context must recover on the same component.
func TestRuntimeMetricsRetentionCancelsActiveStagesAndRecovers(t *testing.T) {
	for _, stage := range []string{"inventory", "age pruning", "size pruning", "marker cleanup"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			assertRetentionCanceledStageRecovery(t, stage)
		})
	}
}

func assertRetentionCanceledStageRecovery(t *testing.T, stage string) {
	t.Helper()
	root, retained, marker, unknown := installOrphanMarkerRecoveryFixture(t)
	expired := writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "expired-runtime-expired", 7)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	filesystem := &cancelStageRetentionFileSystem{stage: stage, cancel: cancel}
	rootLock, artifactLock, markerLock := &metricsTestCloser{}, &metricsTestCloser{}, &metricsTestCloser{}
	retention, err := NewRuntimeMetricsRetention(filesystem, func() time.Time {
		return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	}, &metricsTestCoordination{tryRootLock: rootLock, tryClaim: artifactLock, tryClaimMarker: markerLock})
	if err != nil {
		t.Fatal(err)
	}
	request := RuntimeMetricsRetentionRequest{RootDirectory: root, Config: RuntimeMetricsConfig{MaxAge: 1, MaxSize: 1}}
	if stage == "size pruning" {
		request.Config.MaxAge, request.Config.MaxBackups = 0, 1
	}
	report, err := retention.Sweep(ctx, request)
	wantClaims := 0
	if stage == "marker cleanup" {
		wantClaims = 1
	}
	assertRetentionCanceledStageReport(t, stage, report, err, wantClaims)
	if ctx.Err() != context.Canceled || rootLock.closed != 1 || artifactLock.closed != wantClaims || markerLock.closed != 0 {
		t.Fatalf("canceled %s = %#v, %v; releases root/artifact/marker=%d/%d/%d", stage, report, err, rootLock.closed, artifactLock.closed, markerLock.closed)
	}
	if wantClaims == 0 {
		assertRetentionPreservedContent(t, expired, "mmmmmmm")
	} else {
		assertRetentionPathAbsent(t, expired, "safely pruned artifact before cancellation")
	}
	assertRetentionPathExists(t, marker, "orphan marker after cancellation")
	assertRetentionPreservedContent(t, retained, "mmmmmmmmm")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
	filesystem.cancel = nil
	request.Config.MaxAge = 1
	report, err = retention.Sweep(t.Context(), request)
	assertRetentionRecoveredStageReport(t, report, err, wantClaims)
	if rootLock.closed != 2 || artifactLock.closed != 1 || markerLock.closed != 1 {
		t.Fatalf("recovered %s = %#v, %v; releases root/artifact/marker=%d/%d/%d", stage, report, err, rootLock.closed, artifactLock.closed, markerLock.closed)
	}
	assertRetentionPathAbsent(t, expired, "expired artifact after recovery")
	assertRetentionPathAbsent(t, marker, "orphan marker after recovery")
	assertRetentionPreservedContent(t, retained, "mmmmmmmmm")
	assertRetentionPreservedContent(t, unknown, "preserve customer content")
}

func assertRetentionCanceledStageReport(t *testing.T, stage string, report RuntimeMetricsRetentionReport, err error, wantClaims int) {
	t.Helper()
	operation := "prune runtime metrics:"
	switch stage {
	case "inventory":
		operation = "inventory runtime metrics under"
	case "marker cleanup":
		operation = "reap runtime metrics claim markers:"
	}
	if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), operation) || len(report.Failures) != 0 ||
		report.Removed != (RuntimeMetricsRetentionTotals{Files: wantClaims, Bytes: int64(wantClaims * 7)}) {
		t.Fatalf("canceled %s report = %#v, %v, want %q cancellation", stage, report, err, operation)
	}
}

func assertRetentionRecoveredStageReport(t *testing.T, report RuntimeMetricsRetentionReport, err error, priorClaims int) {
	t.Helper()
	if err != nil || len(report.Failures) != 0 || report.After != (RuntimeMetricsRetentionTotals{Files: 1, Bytes: 9}) ||
		report.Removed != (RuntimeMetricsRetentionTotals{Files: 1 - priorClaims, Bytes: int64((1 - priorClaims) * 7)}) {
		t.Fatalf("recovered sweep = %#v, %v", report, err)
	}
}

type cancelStageRetentionFileSystem struct {
	platformfilesystem.Local
	stage  string
	cancel context.CancelFunc
}

func (filesystem *cancelStageRetentionFileSystem) WalkDir(root string, visit fs.WalkDirFunc) error {
	err := filesystem.Local.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if filesystem.cancel != nil && filesystem.stage == "inventory" && entry != nil && isRuntimeMetricsArtifact(entry.Name()) {
			filesystem.cancel()
		}
		return visit(path, entry, walkErr)
	})
	if filesystem.cancel != nil && (filesystem.stage == "age pruning" || filesystem.stage == "size pruning") {
		filesystem.cancel()
	}
	return err
}

func (filesystem *cancelStageRetentionFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	entries, err := filesystem.Local.ReadDir(path)
	if filesystem.cancel != nil && filesystem.stage == "marker cleanup" && filepath.Base(path) == runtimeMetricsClaimsDirectory {
		filesystem.cancel()
	}
	return entries, err
}

func writeRetentionArtifact(t *testing.T, root, date, clock, suffix string, size int) string {
	t.Helper()
	datePath := filepath.Join(root, filepath.FromSlash(date))
	if err := os.MkdirAll(datePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", datePath, err)
	}
	path := filepath.Join(datePath, clock+"-runtime-metrics-"+suffix+".log")
	if err := os.WriteFile(path, bytes.Repeat([]byte("m"), size), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
	return path
}

func installRetentionReportFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRetentionArtifact(t, root, "2026/07/01", "010000.000000000", "session-old-runtime-old-collision", 600_000)
	writeRetentionArtifact(t, root, "2026/08/20", "020000.000000000", "session-recent-runtime-recent-collision", 600_000)
	return root
}
