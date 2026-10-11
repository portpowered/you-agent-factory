package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

type manyFollow struct {
	lines  chan string
	done   chan error
	cancel context.CancelFunc
}
type manyLineWriter struct{ lines chan string }

func (w manyLineWriter) Write(data []byte) (int, error) {
	w.lines <- string(data)
	return len(data), nil
}

func startManyFollow(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, host string, jsonOutput bool, ids ...string) manyFollow {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	args := []string{"you", "--server", host}
	if jsonOutput {
		args = append(args, "--json")
	}
	args = append(args, "worker-sessions", "follow", "--terminal")
	args = append(args, ids...)
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	lines := make(chan string, 10)
	inputs.Input.Stdout = manyLineWriter{lines}
	done := make(chan error, 1)
	go func() { done <- fixture.process.Execute(inputs.Input) }()
	return manyFollow{lines, done, cancel}
}
func manyAwaitLine(t *testing.T, follow manyFollow, id, state string) string {
	t.Helper()
	select {
	case line := <-follow.lines:
		if strings.Count(line, "\n") != 1 || !strings.Contains(line, id) || !strings.Contains(line, state) {
			t.Fatalf("terminal line=%q", line)
		}
		return line
	case <-time.After(30 * time.Second):
		t.Fatal("terminal line ceiling")
	}
	return ""
}
func manyAwaitDone(t *testing.T, follow manyFollow, code string) error {
	var observed error
	t.Helper()
	select {
	case err := <-follow.done:
		observed = err
		if code == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			var diagnostic *workersessionscli.CLIError
			if code == "WORKER_SESSION_STREAM_INTERRUPTED" && errors.Is(err, context.Canceled) {
				return err
			}
			if !errors.As(err, &diagnostic) || diagnostic.Code != code {
				t.Fatalf("typed error=%v want=%s", err, code)
			}
		}
	case <-time.After(30 * time.Second):
		t.Fatal("follow did not return")
	}
	select {
	case line := <-follow.lines:
		t.Fatalf("duplicate/false terminal=%q", line)
	default:
	}
	return observed
}
func manyStart(t *testing.T, fixture *invokeContinuePackageFixture, name string) (*invokeContinueScenario, *t7GatedProviderRunner, string) {
	t.Helper()
	scenario := fixture.scenario(t, name)
	runner := scenario.providerRunner.(*t7GatedProviderRunner)
	id := scenarioScopedID(scenario, "direct")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		t7ReleaseAndJoin(t, ctx, runner)()
		scenario.close(t)
	})
	status, body := t7HTTP(t, t.Context(), http.MethodPost, fixture.baseURL+"/worker-sessions", map[string]any{
		"requestId": id + "-request", "workerSessionId": id, "prompt": "USER_PRIVATE", "provider": "codex", "model": "controlled-model", "workingRoot": scenario.workingDirectory,
	})
	if status != http.StatusAccepted {
		t.Fatalf("START=%d %s", status, body)
	}
	t19AwaitSignal(t, t.Context(), runner.started, "direct attempt started")
	return scenario, runner, id
}

// F1-F3 exercise retained/live handoff on the shared canonical host. Each route
// owns a direct attempt; START uses no per-call Factory Session.
func TestManySessionTerminalFollowCompletionOrder(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.follow", "rest/streamWorkerSessionEventsByWorkerSessionId")
	fixture := ensureInvokeContinuePackageFixture(t)
	a, ar, aid := manyStart(t, fixture, "sam-watch-a")
	_, br, bid := manyStart(t, fixture, "sam-watch-b")
	_, cr, cid := manyStart(t, fixture, "sam-watch-c")
	follow := startManyFollow(t, fixture, a, fixture.baseURL, false, aid, bid, cid, aid, bid)
	for _, peer := range []struct {
		id string
		r  *t7GatedProviderRunner
	}{{bid, br}, {aid, ar}, {cid, cr}} {
		close(peer.r.release)
		manyAwaitLine(t, follow, peer.id, "COMPLETED")
	}
	manyAwaitDone(t, follow, "")
}
func TestManySessionTerminalFollowRetainedAndHandoff(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	retained, rr, rid := manyStart(t, fixture, "sam-watch-retained")
	_, ar, aid := manyStart(t, fixture, "sam-watch-active")
	close(rr.release)
	t19AwaitSignal(t, t.Context(), rr.stopped, "retained provider finished")
	// A first public observer joins publication, establishing a retained terminal.
	first := startManyFollow(t, fixture, retained, fixture.baseURL, false, rid)
	manyAwaitLine(t, first, rid, "COMPLETED")
	manyAwaitDone(t, first, "")
	follow := startManyFollow(t, fixture, retained, fixture.baseURL, false, rid, aid, rid, aid)
	manyAwaitLine(t, follow, rid, "COMPLETED")
	close(ar.release)
	manyAwaitLine(t, follow, aid, "COMPLETED")
	manyAwaitDone(t, follow, "")
	handoff, hr, hid := manyStart(t, fixture, "sam-watch-handoff")
	across := startManyFollow(t, fixture, handoff, fixture.baseURL, false, hid, hid)
	close(hr.release)
	manyAwaitLine(t, across, hid, "COMPLETED")
	manyAwaitDone(t, across, "")
}
func TestManySessionTerminalFollowMixedStates(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario, completed, cid := manyStart(t, fixture, "sam-watch-completed")
	_, failed, fid := manyStart(t, fixture, "sam-watch-failed")
	_, _, cancelID := manyStart(t, fixture, "sam-watch-canceled")
	_, _, terminateID := manyStart(t, fixture, "sam-watch-terminated")
	follow := startManyFollow(t, fixture, scenario, fixture.baseURL, true, cid, fid, cancelID, terminateID)
	close(completed.release)
	manyAwaitLine(t, follow, cid, "COMPLETED")
	close(failed.release)
	if line := manyAwaitLine(t, follow, fid, "FAILED"); strings.Contains(line, "T7 detached") {
		t.Fatal("failure used partial output")
	}
	for _, control := range []struct{ id, action, state string }{{cancelID, "cancel", "CANCELED"}, {terminateID, "terminate", "TERMINATED"}} {
		status, body := t7HTTP(t, t.Context(), http.MethodPost, fixture.baseURL+"/worker-sessions/"+control.id+"/"+control.action, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("control=%d %s", status, body)
		}
		manyAwaitLine(t, follow, control.id, control.state)
	}
	manyAwaitDone(t, follow, "")
}
func TestManySessionTerminalFollowDetachAndSummary(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario, runner, id := manyStart(t, fixture, "sam-watch-detach")
	follow := startManyFollow(t, fixture, scenario, fixture.baseURL, false, id)
	peer := startManyFollow(t, fixture, scenario, fixture.baseURL, false, id)
	follow.cancel()
	manyAwaitDone(t, follow, "WORKER_SESSION_STREAM_INTERRUPTED")
	assertSubagentDeadlineObservation(t, t.Context(), fixture.baseURL, id, "RUNNING", "")
	select {
	case <-runner.stopped:
		t.Fatal("detach stopped attempt")
	default:
	}
	reattached := startManyFollow(t, fixture, scenario, fixture.baseURL, false, id, id)
	close(runner.release)
	manyAwaitLine(t, peer, id, "COMPLETED")
	manyAwaitDone(t, peer, "")
	manyAwaitLine(t, reattached, id, "COMPLETED")
	manyAwaitDone(t, reattached, "")
	summary, sr, sid := manyStart(t, fixture, "sam-watch-summary")
	text := startManyFollow(t, fixture, summary, fixture.baseURL, false, sid)
	close(sr.release)
	line := manyAwaitLine(t, text, sid, "COMPLETED")
	manyAwaitDone(t, text, "")
	if !strings.Contains(line, `result="Published\r\nsummary"`) {
		t.Fatalf("escaped summary=%q", line)
	}
	jsonFollow := startManyFollow(t, fixture, summary, fixture.baseURL, true, sid)
	line = manyAwaitLine(t, jsonFollow, sid, "COMPLETED")
	manyAwaitDone(t, jsonFollow, "")
	var fields map[string]string
	if err := json.Unmarshal([]byte(line), &fields); err != nil || fields["result"] != "Published\r\nsummary" || len(fields) != 3 {
		t.Fatalf("NDJSON=%q err=%v", line, err)
	}
}

func TestManySessionTerminalFollowObservationFailures(t *testing.T) {
	for _, fault := range []struct{ name, code string }{{"eof", "WORKER_SESSION_STREAM_CLOSED"}, {"source", "TEST_SOURCE_FAILURE"}, {"gap", "WORKER_SESSION_STREAM_GAP"}, {"unknown", "WORKER_SESSION_NOT_FOUND"}, {"unavailable", "WORKER_SESSION_STREAM_UNAVAILABLE"}} {
		t.Run(fault.name, func(t *testing.T) {
			t.Parallel()
			fixture := ensureInvokeContinuePackageFixture(t)
			scenario, runner, id := manyStart(t, fixture, "sam-watch-fault-"+fault.name)
			_, peerRunner, peerID := manyStart(t, fixture, "sam-watch-fault-"+fault.name+"-peer")
			release := make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			host := manyFaultHost(t, fixture.baseURL, fault.name, release)
			follow := startManyFollow(t, fixture, scenario, host, false, id, peerID, "fault-"+fault.name)
			close(runner.release)
			manyAwaitLine(t, follow, id, "COMPLETED")
			close(release)
			err := manyAwaitDone(t, follow, fault.code)
			if !strings.Contains(err.Error(), "fault-"+fault.name) || !strings.Contains(err.Error(), host) {
				t.Fatalf("failed observation identity=%v", err)
			}
			assertSubagentDeadlineObservation(t, t.Context(), fixture.baseURL, peerID, "RUNNING", "")
			select {
			case <-peerRunner.stopped:
				t.Fatal("observation failure controlled peer attempt")
			default:
			}
			reattach := startManyFollow(t, fixture, scenario, fixture.baseURL, false, peerID)
			close(peerRunner.release)
			manyAwaitLine(t, reattach, peerID, "COMPLETED")
			manyAwaitDone(t, reattach, "")
		})
	}
}
func manyFaultHost(t *testing.T, host, fault string, release <-chan struct{}) string {
	t.Helper()
	target, err := url.Parse(host)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/fault-") {
			proxy.ServeHTTP(w, r)
			return
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		if fault == "unknown" {
			w.WriteHeader(404)
			return
		}
		if fault == "unavailable" {
			w.WriteHeader(503)
			_, _ = io.WriteString(w, `{"code":"WORKER_SESSION_STREAM_UNAVAILABLE","message":"controlled unavailable stream"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if fault == "source" {
			_, _ = fmt.Fprintf(w, "data: {\"workerSessionId\":%q,\"delivery\":\"SOURCE_FAILURE\",\"errorCode\":\"TEST_SOURCE_FAILURE\"}\n\n", "fault-"+fault)
		}
		if fault == "gap" {
			_, _ = fmt.Fprintf(w, "data: {\"workerSessionId\":%q,\"delivery\":\"EVENT\",\"event\":{\"payload\":{\"kind\":\"STREAM_GAP\"}}}\n\n", "fault-"+fault)
		}
		w.(http.Flusher).Flush()
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestManySessionTerminalFollowSelectionErrors(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "sam-watch-usage")
	defer scenario.close(t)
	for _, ids := range [][]string{nil, {""}, {" "}} {
		follow := startManyFollow(t, fixture, scenario, fixture.baseURL, false, ids...)
		manyAwaitDone(t, follow, "WORKER_SESSION_FOLLOW_INVALID")
	}
	server := httptest.NewServer(http.NotFoundHandler())
	unavailable := server.URL
	server.Close()
	follow := startManyFollow(t, fixture, scenario, unavailable, false, "not-on-another-host")
	err := manyAwaitDone(t, follow, "FACTORY_UNREACHABLE")
	if !strings.Contains(err.Error(), unavailable) || !strings.Contains(err.Error(), "not-on-another-host") {
		t.Fatal(err)
	}
}
