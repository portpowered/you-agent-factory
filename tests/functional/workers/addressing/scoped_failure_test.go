package addressing_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// F2-12 crosses actual cancellation/join and opening persistence. Only the
// selected successor's external recording write fails; the replay peer stays
// readable through the same public CLI/HTTP boundaries.
func TestSelectedLegacyCollisionAdmissionFailure(t *testing.T) {
	t.Parallel()
	runner := &addressingRunner{started: make(chan struct{}, 2), release: make(chan struct{})}
	body, flags := controlInput("interrupt")
	store := &selectedOpeningFailureStore{successor: body["successorWorkerSessionId"].(string)}
	f := newReplayFixtureWithEdges(t, runner, 1, serviceedges.Edges{
		WorkerRecordingWriter:        store,
		WorkerRecordingStoreObserver: func(real recordings.WorkerRecordingStore) { store.WorkerRecordingStore = real },
	})
	t.Cleanup(func() { runner.releaseOnce.Do(func() { close(runner.release) }) })
	live := openAddressingOwner(t, f)
	invokeAddressingSource(t, f, live)
	awaitAddressingStart(t, runner, f, live)
	body["factorySessionId"] = live.session
	body["resumeMode"] = "recorded"
	path := "/worker-sessions/" + f.worker + "/interrupt"
	status, first := f.http(t, "POST", path, body)
	assertSelectedAdmissionFailure(t, status, first, f.worker, store.successor)
	args := append([]string{"interrupt", f.worker}, flags...)
	args = append(args, "--session", live.session, "--resume-mode", "recorded")
	cli := f.cli(t, true, args...)
	assertErrorCode(t, cli, "WORKER_SESSION_INTERRUPT_SUCCESSOR_ADMISSION_FAILED")
	var cliError struct{ Phase string }
	if err := json.Unmarshal(cli, &cliError); err != nil || cliError.Phase != "SUCCESSOR_ADMISSION" || strings.Contains(string(cli), "private-") {
		t.Fatalf("CLI partial admission failure: %s: %v", cli, err)
	}
	status, repeated := f.http(t, "POST", path, body)
	assertSelectedAdmissionFailure(t, status, repeated, f.worker, store.successor)
	assertSameJSON(t, first, repeated)
	assertAddressingEffects(t, runner, 1, 1)
	assertAddressingState(t, f, live, "CANCELED")
	status, peer := f.http(t, "GET", "/worker-sessions/"+f.worker+"?factorySessionId="+f.owners[0].session, nil)
	if status != http.StatusOK {
		t.Fatalf("peer after admission failure = %d: %s", status, peer)
	}
	assertOwner(t, f, f.owners[0], peer)
}

type selectedOpeningFailureStore struct {
	recordings.WorkerRecordingStore
	successor string
}

func (s *selectedOpeningFailureStore) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if record.WorkerSessionID == s.successor {
		return errors.New("private-selected-opening-failure")
	}
	return s.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
}

func assertSelectedAdmissionFailure(t *testing.T, status int, raw []byte, source, successor string) {
	t.Helper()
	var result struct {
		Code, Phase, SourceWorkerSessionID, SuccessorWorkerSessionID string
		Source                                                       *struct{ WorkerSessionID, State string }
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusServiceUnavailable || result.Code != "WORKER_SESSION_INTERRUPT_SUCCESSOR_ADMISSION_FAILED" ||
		result.Phase != "SUCCESSOR_ADMISSION" || result.SourceWorkerSessionID != source || result.SuccessorWorkerSessionID != successor ||
		result.Source == nil || result.Source.WorkerSessionID != source || result.Source.State != "CANCELED" || strings.Contains(string(raw), "private-") {
		t.Fatalf("selected partial admission failure = %d: %s", status, raw)
	}
}
