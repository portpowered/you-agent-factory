package addressing_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F2-11: archived history proves a real captured representation exists while
// the same process retains the terminal live observation. Both representations
// must still address one identity, without requiring another root graph.
func assertUniqueCapturedSuccessor(t *testing.T, f *replayFixture, owner replayOwner, successor string) {
	t.Helper()
	rows, err := support.WaitForObservation(15*time.Second, func() ([]factoryapi.WorkerSessionObservation, error) {
		return addressingHistoryRows(t, f, "archived", successor)
	}, func(rows []factoryapi.WorkerSessionObservation) bool { return len(rows) != 0 })
	if err != nil {
		t.Fatalf("successor capture did not become readable: %v", err)
	}
	assertSingleSuccessorRow(t, f, owner, successor, rows)
	rows, err = addressingHistoryRows(t, f, "all", successor)
	if err != nil {
		t.Fatal(err)
	}
	assertSingleSuccessorRow(t, f, owner, successor, rows)
	for _, scope := range []string{"", "?factorySessionId=" + owner.session} {
		status, raw := f.http(t, "GET", "/worker-sessions/"+successor+scope, nil)
		if status != http.StatusOK {
			t.Fatalf("unique live/captured show = %d: %s", status, raw)
		}
		var observation factoryapi.WorkerSessionObservation
		if err := json.Unmarshal(raw, &observation); err != nil {
			t.Fatal(err)
		}
		assertSingleSuccessorRow(t, f, owner, successor, []factoryapi.WorkerSessionObservation{observation})
	}
	var observation factoryapi.WorkerSessionObservation
	raw := f.cli(t, false, "show", "--worker-session-id", successor)
	if err := json.Unmarshal(raw, &observation); err != nil {
		t.Fatal(err)
	}
	assertSingleSuccessorRow(t, f, owner, successor, []factoryapi.WorkerSessionObservation{observation})
}

func addressingHistoryRows(t *testing.T, f *replayFixture, history, worker string) ([]factoryapi.WorkerSessionObservation, error) {
	t.Helper()
	status, raw := f.http(t, "GET", "/worker-sessions?history="+history, nil)
	if status != http.StatusOK {
		return nil, fmt.Errorf("%s history = %d: %s", history, status, raw)
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	var rows []factoryapi.WorkerSessionObservation
	for _, row := range response.Sessions {
		if row.WorkerSessionId == worker {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func assertSingleSuccessorRow(t *testing.T, f *replayFixture, owner replayOwner, successor string, rows []factoryapi.WorkerSessionObservation) {
	t.Helper()
	if len(rows) != 1 {
		t.Fatalf("live/captured successor has %d identities: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.WorkerSessionId != successor || row.FactorySessionId == nil || *row.FactorySessionId != owner.session ||
		row.State != "COMPLETED" || !reflect.DeepEqual(row.WorkIds, []string{owner.work}) ||
		row.PredecessorWorkerSessionId == nil || *row.PredecessorWorkerSessionId != f.worker {
		t.Fatalf("live/captured successor lost selected identity: %+v", row)
	}
}

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
