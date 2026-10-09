package addressing_test

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// F2-11: the selected attempt awaits the real command cancellation callback
// while a retained equal-ID owner stays independently readable. Concurrent
// retries must join that attempt and preserve its owner through terminality.
func TestSelectedLegacyCollisionConcurrentInterrupt(t *testing.T) {
	t.Parallel()
	runner := &addressingRunner{started: make(chan struct{}, 2), release: make(chan struct{}),
		cancelObserved: make(chan struct{}), cancelReturn: make(chan struct{})}
	f := newReplayFixtureWithRunner(t, runner, 1)
	var unblock sync.Once
	var requests sync.WaitGroup
	t.Cleanup(func() {
		unblock.Do(func() { close(runner.cancelReturn) })
		runner.releaseOnce.Do(func() { close(runner.release) })
		requests.Wait()
	})
	live := openAddressingOwner(t, f)
	invokeAddressingSource(t, f, live)
	awaitAddressingStart(t, runner, f, live)
	body, flags := controlInput("interrupt")
	body["factorySessionId"], body["resumeMode"] = live.session, "recorded"
	path := "/worker-sessions/" + f.worker + "/interrupt"
	first := make(chan []byte, 1)
	requests.Add(1)
	go func() {
		defer requests.Done()
		status, raw := f.http(t, "POST", path, body)
		if status != http.StatusAccepted {
			t.Errorf("concurrent selected interrupt = %d: %s", status, raw)
		}
		first <- raw
	}()
	select {
	case <-runner.cancelObserved:
	case <-time.After(15 * time.Second):
		t.Fatal("selected command did not observe cancellation")
	}
	assertAddressingEffects(t, runner, 1, 1)
	assertNotFoundSuccessor(t, f, body["successorWorkerSessionId"].(string))
	assertPeerDuringPendingInterrupt(t, f, body)
	args := append([]string{"interrupt", f.worker}, flags...)
	args = append(args, "--session", live.session, "--resume-mode", "recorded")
	retry := make(chan []byte, 1)
	requests.Add(1)
	go func() {
		defer requests.Done()
		retry <- f.cli(t, false, args...)
	}()
	unblock.Do(func() { close(runner.cancelReturn) })
	a := awaitAddressingControl(t, first)
	assertSameJSON(t, a, awaitAddressingControl(t, retry))
	awaitAddressingStart(t, runner, f, live)
	assertAddressingEffects(t, runner, 2, 1)
	assertAddressingState(t, f, live, "CANCELED")
	successor := body["successorWorkerSessionId"].(string)
	assertAddressingSuccessor(t, f, live, successor)
	runner.releaseOnce.Do(func() { close(runner.release) })
	waitAddressingCompleted(t, f, successor, live.session)
	assertSameJSON(t, a, f.cli(t, false, args...))
	assertPeerDuringPendingInterrupt(t, f, body)
	assertAddressingEffects(t, runner, 2, 1)
}

func assertPeerDuringPendingInterrupt(t *testing.T, f *replayFixture, body map[string]any) {
	t.Helper()
	peer := f.owners[0]
	status, raw := f.http(t, "GET", "/worker-sessions/"+f.worker+"?factorySessionId="+peer.session, nil)
	if status != http.StatusOK {
		t.Fatalf("peer read during selected control = %d: %s", status, raw)
	}
	assertOwner(t, f, peer, raw)
	changed := make(map[string]any, len(body))
	for key, value := range body {
		changed[key] = value
	}
	changed["factorySessionId"] = peer.session
	status, raw = f.http(t, "POST", "/worker-sessions/"+f.worker+"/interrupt", changed)
	if status != http.StatusConflict {
		t.Fatalf("peer request-ID reuse = %d: %s", status, raw)
	}
	assertErrorCode(t, raw, "WORKER_SESSION_INTERRUPT_REQUEST_ID_CONFLICT")
	assertValidationPhase(t, raw)
}

func awaitAddressingControl(t *testing.T, result <-chan []byte) []byte {
	t.Helper()
	select {
	case raw := <-result:
		return raw
	case <-time.After(15 * time.Second):
		t.Fatal("selected control did not finish after cancellation callback returned")
		return nil
	}
}
