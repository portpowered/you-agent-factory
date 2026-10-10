package workscope_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Reuse the compiled-artifact server/provider fixtures for a small Work pipe
// witness. The full behavioral matrix belongs to the functional Work lane.
func TestPrebuiltWorkFiniteObservationAndPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	f, listener := startWorkObservationArtifact(t, ctx)
	a := f.openWorkSession(t, ctx)
	b := f.openWorkSession(t, ctx)
	aWatch := f.attachWorkWatch(t, ctx, a, true)
	bWatch := f.attachWorkWatch(t, ctx, b, false)
	aEvents := openWorkArtifactEvents(t, ctx, f.serverURL, a)
	bEvents := openWorkArtifactEvents(t, ctx, f.serverURL, b)
	aWork := f.submitWork(t, ctx, a)
	aControl := acceptWorkArtifactProvider(t, listener)
	bWork := f.submitWork(t, ctx, b)
	bControl := acceptWorkArtifactProvider(t, listener)
	// B completes while A's real provider command and follow observer are held.
	if _, err := io.WriteString(bControl, "release\n"); err != nil {
		t.Fatal(err)
	}
	readWorkArtifactEvent(t, bEvents, factoryapi.FactoryEventTypeDispatchResponse)
	if bWatch.stdout.String() != "" || aWatch.stdout.String() != "" {
		t.Fatal("dispatch-only observation fabricated a transition")
	}
	for _, state := range []string{"processing", "complete"} {
		f.workCLI(t, ctx, "work", "move", bWork, state, "--session", b)
	}
	bOutput := finishWorkArtifactWatch(t, ctx, bWatch, false)
	assertWorkArtifactLines(t, bOutput, b, bWork, bEvents)
	retained := f.workCLI(t, ctx, "work", "watch", "--session", b)
	if string(retained) != bOutput {
		t.Fatalf("live/retained mismatch: %s / %s", bOutput, retained)
	}
	f.assertWorkAbsence(t, ctx, a)
	// Cancel the owned executable observer, then prove peer reads/cursor remain
	// usable. Functional tests separately prove graceful invocation cancellation.
	if err := aWatch.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	finishWorkArtifactWatch(t, ctx, aWatch, true)
	var shown factoryapi.Work
	if err := json.Unmarshal(f.workCLI(t, ctx, "--json", "work", "show", bWork, "--session", b), &shown); err != nil || shown.State == nil || shown.State.Name != "complete" {
		t.Fatalf("peer selected state=%+v %v", shown, err)
	}
	if _, err := io.WriteString(aControl, "release\n"); err != nil {
		t.Fatal(err)
	}
	readWorkArtifactEvent(t, aEvents, factoryapi.FactoryEventTypeDispatchResponse)
	f.workCLI(t, ctx, "work", "move", aWork, "complete", "--session", a)
	// A fresh peer admission after cancellation must cross the provider boundary.
	later := f.submitWork(t, ctx, b)
	laterControl := acceptWorkArtifactProvider(t, listener)
	if _, err := io.WriteString(laterControl, "release\n"); err != nil {
		t.Fatal(err)
	}
	readWorkArtifactEvent(t, bEvents, factoryapi.FactoryEventTypeDispatchResponse)
	f.workCLI(t, ctx, "work", "move", later, "complete", "--session", b)
	event := readWorkArtifactEvent(t, bEvents, factoryapi.FactoryEventTypeWorkStateChange)
	payload, err := event.Payload.AsWorkStateChangeEventPayload()
	if err != nil || payload.WorkId != later || payload.ToState != "complete" {
		t.Fatalf("peer cursor: %+v %v", event, err)
	}
	stopPrebuiltWorkscopeDaemon(t, ctx, f.daemon, f.binary, f.factoryDir, f.environment, f.serverURL)
	t.Logf("I-WORK artifact=%s sha256=%s sessions=%s,%s live/retained_exit=0 canonical_lines=2 peer_after_cancel=%s provider=controlled-Codex cleanup=server/providers/watchers-joined", f.binary, sha256FileDigest(t, f.binary), a, b, later)
}

func startWorkObservationArtifact(t *testing.T, ctx context.Context) (selectedArtifactScenario, net.Listener) {
	t.Helper()
	root := t.TempDir()
	f := selectedArtifactScenario{binary: resolvePrebuiltWorkscopeBinary(t), factoryDir: filepath.Join(root, "factory")}
	host, home, provider := filepath.Join(root, "host"), filepath.Join(root, "home"), filepath.Join(root, "provider")
	for _, dir := range []string{host, home, f.factoryDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writePrebuiltWorkscopeLiveFactory(t, host)
	writeSelectedArtifactFactory(t, f.factoryDir)
	path := filepath.Join(f.factoryDir, "factory.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Keep provider dispatch output nonterminal; only real public moves are
	// canonical transitions. Dispatch-only completion must never be inferred.
	raw = []byte(strings.ReplaceAll(string(raw), `{"name":"complete","type":"TERMINAL"}`, `{"name":"review","type":"PROCESSING"},{"name":"processing","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}`))
	raw = []byte(strings.ReplaceAll(string(raw), `"outputs":[{"workType":"task","state":"complete"}]`, `"outputs":[{"workType":"task","state":"review"}]`))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	writePrebuiltWorkscopeGatedCodex(t, provider)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	f.environment = builtcliacceptance.ProcessEnvForIsolatedHome(home)
	f.environment = replacePrebuiltWorkscopeEnv(f.environment, "PATH", provider+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.environment = replacePrebuiltWorkscopeEnv(f.environment, prebuiltWorkscopeLiveProviderControlAddress, listener.Addr().String())
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil || port == 7437 {
		t.Fatalf("owned port=%d %v", port, err)
	}
	f.serverURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	f.daemon = startPrebuiltWorkscopeDaemon(t, ctx, f.binary, host, f.environment, "run", "--dir", host, "--continuously", "--with-server", "--listen", fmt.Sprintf("127.0.0.1:%d", port))
	t.Cleanup(func() { _, _, _ = f.daemon.stopMemoryMonitor() })
	waitForPrebuiltWorkscopeLiveServer(t, ctx, f.daemon, f.serverURL)
	return f, listener
}

func (f selectedArtifactScenario) openWorkSession(t *testing.T, ctx context.Context) string {
	t.Helper()
	dir := t.TempDir()
	copyPrebuiltWorkscopeDirectory(t, f.factoryDir, dir)
	var response factoryapi.OpenFactorySessionResponse
	selectedArtifactPOST(t, ctx, f.serverURL+"/factory-sessions", factoryapi.OpenFactorySessionRequest{FolderPath: dir}, http.StatusOK, &response)
	if response.Session == nil || response.Session.IsDefault {
		t.Fatalf("selected session=%+v", response)
	}
	return response.Session.Id
}

func (f selectedArtifactScenario) submitWork(t *testing.T, ctx context.Context, session string) string {
	t.Helper()
	var response factoryapi.SubmitWorkResponse
	selectedArtifactPOST(t, ctx, f.serverURL+"/factory-sessions/"+session+"/work", factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: map[string]string{"title": "compiled Work observation"}}, http.StatusCreated, &response)
	if response.WorkId == nil {
		t.Fatal("missing Work ID")
	}
	return *response.WorkId
}

func acceptWorkArtifactProvider(t *testing.T, listener net.Listener) net.Conn {
	t.Helper()
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetReadDeadline(time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "started" {
		t.Fatalf("provider readiness=%q %v", line, err)
	}
	return conn
}

func (f selectedArtifactScenario) workCLI(t *testing.T, ctx context.Context, args ...string) []byte {
	t.Helper()
	return runPrebuiltWorkscopeCLI(t, ctx, f.binary, f.factoryDir, f.environment, append([]string{"--server", f.serverURL}, args...)...)
}

func (f selectedArtifactScenario) attachWorkWatch(t *testing.T, ctx context.Context, session string, follow bool) *prebuiltWorkscopeDaemon {
	t.Helper()
	target, err := url.Parse(f.serverURL)
	if err != nil {
		t.Fatal(err)
	}
	attached := make(chan struct{}, 1)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(r *http.Response) error {
		if r.Request.URL.Path == "/factory-sessions/"+session+"/events" && r.StatusCode == http.StatusOK {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "text/event-stream") {
				return fmt.Errorf("invalid SSE headers: %v", r.Header)
			}
			select {
			case attached <- struct{}{}:
			default:
			}
		}
		return nil
	}
	server := httptest.NewServer(proxy)
	t.Cleanup(server.Close)
	args := []string{"--server", server.URL, "work", "watch", "--session", session}
	if follow {
		args = append(args, "--follow")
	}
	watch := startPrebuiltWorkscopeDaemon(t, ctx, f.binary, f.factoryDir, f.environment, args...)
	t.Cleanup(func() { _, _, _ = watch.stopMemoryMonitor() })
	select {
	case <-attached:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case err := <-watch.done:
		t.Fatalf("watch before admission: %v %s", err, watch.stderr.String())
	}
	return watch
}

func finishWorkArtifactWatch(t *testing.T, ctx context.Context, watch *prebuiltWorkscopeDaemon, killed bool) string {
	t.Helper()
	select {
	case err := <-watch.done:
		watch.mu.Lock()
		watch.stopped = true
		watch.mu.Unlock()
		if (err != nil) != killed {
			t.Fatalf("watch exit=%v killed=%t stdout=%s stderr=%s", err, killed, watch.stdout.String(), watch.stderr.String())
		}
	case <-ctx.Done():
		t.Fatalf("watch did not finish: %v", ctx.Err())
	}
	if watch.stderr.String() != "" {
		t.Fatalf("watch stderr=%s", watch.stderr.String())
	}
	return watch.stdout.String()
}

func openWorkArtifactEvents(t *testing.T, ctx context.Context, base, session string) *bufio.Reader {
	t.Helper()
	r := doPrebuiltWorkscopeGET(t, ctx, http.DefaultClient, base+"/factory-sessions/"+session+"/events")
	if r.StatusCode != http.StatusOK || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE headers=%d %v", r.StatusCode, r.Header)
	}
	t.Cleanup(func() { _ = r.Body.Close() })
	return bufio.NewReader(r.Body)
}

func readWorkArtifactEvent(t *testing.T, reader *bufio.Reader, kind factoryapi.FactoryEventType) factoryapi.FactoryEvent {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != kind {
			continue
		}
		if kind == factoryapi.FactoryEventTypeDispatchResponse {
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil || payload.Outcome == factoryapi.WorkOutcomeFailed {
				t.Fatalf("controlled dispatch=%+v %v", payload, err)
			}
		}
		return event
	}
}

func assertWorkArtifactLines(t *testing.T, output, session, work string, events *bufio.Reader) {
	t.Helper()
	if !strings.HasSuffix(output, "\n") {
		t.Fatalf("incomplete NDJSON=%q", output)
	}
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("canonical lines=%d %s", len(lines), output)
	}
	var previous int64
	for i, raw := range lines {
		var line struct {
			SchemaVersion string    `json:"schemaVersion"`
			SessionID     string    `json:"sessionId"`
			WorkID        string    `json:"workId"`
			EventID       string    `json:"eventId"`
			Sequence      int64     `json:"sequence"`
			EventTime     time.Time `json:"eventTime"`
			FromState     string    `json:"fromState"`
			ToState       string    `json:"toState"`
			Source        string    `json:"source"`
			Terminal      bool      `json:"terminal"`
		}
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatal(err)
		}
		event := readWorkArtifactEvent(t, events, factoryapi.FactoryEventTypeWorkStateChange)
		payload, err := event.Payload.AsWorkStateChangeEventPayload()
		if err != nil || line.SchemaVersion != "you.work.watch.v1" || line.SessionID != session || line.WorkID != work || line.EventID != event.Id || line.Sequence != int64(event.Context.Sequence) || line.Sequence <= previous || line.EventTime.IsZero() || line.FromState != payload.FromState || line.ToState != payload.ToState || line.Terminal != (i == 1) || line.Source != "api" {
			t.Fatalf("NDJSON/canonical mismatch: %s event=%+v err=%v", raw, event, err)
		}
		previous = line.Sequence
	}
	t.Logf("compiled live/retained stdout=%s stderr=empty", output)
}

func (f selectedArtifactScenario) assertWorkAbsence(t *testing.T, ctx context.Context, known string) {
	t.Helper()
	for _, path := range []string{"missing-session/work", known + "/work/missing-work"} {
		r := doPrebuiltWorkscopeGET(t, ctx, http.DefaultClient, f.serverURL+"/factory-sessions/"+path)
		raw, status := readPrebuiltWorkscopeResponse(t, r)
		var failure factoryapi.ErrorResponse
		if status != http.StatusNotFound || json.Unmarshal(raw, &failure) != nil || failure.Code != factoryapi.ErrorResponseCodeNOTFOUND {
			t.Fatalf("typed absence=%d %s", status, raw)
		}
		t.Logf("compiled HTTP target=%s status=%d body=%s", path, status, raw)
	}
}
