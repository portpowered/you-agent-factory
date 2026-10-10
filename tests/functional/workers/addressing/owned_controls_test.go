package addressing_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

// Two live owners share the existing package process. CLI/HTTP parity is the
// public control contract; the command edge controls only attachment and join.
func TestWorkerSessionOwnedControls(t *testing.T) {
	t.Parallel()
	selectedRunner := newOwnedControlRunner()
	f := newReplayFixtureWithRunner(t, selectedRunner, 0)
	selected := openAddressingOwner(t, f)
	invokeAddressingSource(t, f, selected)
	awaitAddressingStart(t, selectedRunner.addressingRunner, f, selected)
	peerRunner := newOwnedControlRunner()
	peerFixture := &replayFixture{process: f.process, url: f.url, group: f.group, worker: uuid.NewString(), runner: peerRunner}
	peer := openAddressingOwner(t, peerFixture)
	invokeAddressingSource(t, peerFixture, peer)
	awaitAddressingStart(t, peerRunner.addressingRunner, peerFixture, peer)
	t.Cleanup(func() {
		selectedRunner.releaseOnce.Do(func() { close(selectedRunner.release) })
		peerRunner.releaseOnce.Do(func() { close(peerRunner.release) })
	})

	unknown := uuid.NewString()
	status, raw := f.http(t, "POST", "/worker-sessions/"+unknown+"/terminate", map[string]any{})
	assertNotFound(t, status, raw)
	assertErrorCode(t, f.cli(t, true, "terminate", unknown), "NOT_FOUND")
	assertAddressingState(t, f, selected, "RUNNING")
	assertAddressingState(t, peerFixture, peer, "RUNNING")
	assertAddressingEffects(t, selectedRunner.addressingRunner, 1, 0)
	assertAddressingEffects(t, peerRunner.addressingRunner, 1, 0)

	assertOwnedForceReplay(t, f, selectedRunner, selected)
	assertAddressingState(t, peerFixture, peer, "RUNNING")
	assertAddressingEffects(t, peerRunner.addressingRunner, 1, 0)
	// The independent peer still accepts its own ordinary cancellation after
	// the selected owner's terminal control and durable replay.
	peerFixture.cli(t, false, "terminate", peerFixture.worker)
	awaitOwnedControlJoin(t, peerRunner)
	assertAddressingState(t, peerFixture, peer, "TERMINATED")
	assertAddressingEffects(t, peerRunner.addressingRunner, 1, 1)
}

func assertOwnedForceReplay(t *testing.T, f *replayFixture, runner *ownedControlRunner, owner replayOwner) {
	t.Helper()
	status, raw := f.http(t, "GET", "/worker-sessions/"+f.worker, nil)
	var observation struct {
		AttemptID string `json:"attemptId"`
	}
	if err := json.Unmarshal(raw, &observation); err != nil || status != http.StatusOK || observation.AttemptID == "" {
		t.Fatalf("live attempt readback: status=%d error=%v", status, err)
	}
	request := uuid.NewString()
	body := map[string]any{"force": true, "requestId": request, "expectedAttemptId": observation.AttemptID}
	path := "/worker-sessions/" + f.worker + "/terminate"
	args := []string{"terminate", f.worker, "--force", "--request-id", request, "--expected-attempt-id", observation.AttemptID}
	first := f.cli(t, false, args...)
	var outcome struct {
		State, Outcome string
		Forced         bool
	}
	if err := json.Unmarshal(first, &outcome); err != nil || outcome.State != "TERMINATED" || outcome.Outcome != "APPLIED" || !outcome.Forced {
		t.Fatalf("owned force outcome = %s: %v", first, err)
	}
	awaitOwnedControlJoin(t, runner)
	status, repeated := f.http(t, "POST", path, body)
	if status != http.StatusOK {
		t.Fatalf("committed replay = %d: %s", status, repeated)
	}
	assertSameJSON(t, first, repeated)
	assertSameJSON(t, first, f.cli(t, false, args...))
	body["expectedAttemptId"] = uuid.NewString()
	status, changed := f.http(t, "POST", path, body)
	if status != http.StatusConflict {
		t.Fatalf("changed force tuple = %d: %s", status, changed)
	}
	assertErrorCode(t, changed, "WORKER_SESSION_CONTROL_CONFLICT")
	if runner.forceCalls.Load() != 1 {
		t.Fatalf("force replay signaled %d times", runner.forceCalls.Load())
	}
	assertAddressingState(t, f, owner, "TERMINATED")
}

type ownedControlRunner struct {
	*addressingRunner
	done       chan struct{}
	forceCalls atomic.Int32
}

func newOwnedControlRunner() *ownedControlRunner {
	return &ownedControlRunner{addressingRunner: &addressingRunner{started: make(chan struct{}, 1), release: make(chan struct{})}, done: make(chan struct{})}
}

func (runner *ownedControlRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.RunStreaming(ctx, request, nil)
}

func (runner *ownedControlRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	defer close(runner.done)
	if request.OwnedProcessObserver != nil {
		request.OwnedProcessObserver(runner)
	}
	return runner.addressingRunner.RunStreaming(ctx, request, observe)
}

func (runner *ownedControlRunner) ForceKill(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	runner.forceCalls.Add(1)
	runner.releaseOnce.Do(func() { close(runner.release) })
	select {
	case <-runner.done:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func awaitOwnedControlJoin(t *testing.T, runner *ownedControlRunner) {
	t.Helper()
	select {
	case <-runner.done:
	case <-t.Context().Done():
		t.Fatal("owned command did not join")
	}
}
