package script_payload_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type missionHostOutput struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *missionHostOutput) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(data)
}
func (b *missionHostOutput) text() string { b.mu.Lock(); defer b.mu.Unlock(); return b.Buffer.String() }

type missionHost struct {
	url    string
	cmd    *exec.Cmd
	done   chan struct{}
	err    error // written before done closes
	output missionHostOutput
}

// One real journey crosses CLI startup, durable restart and real Python stdin.
// Artifact compilation belongs to the invoking build/CI lane, never this test.
func TestPrebuiltCLIRestoredGatedMissionPayload(t *testing.T) {
	t.Parallel()
	binary := os.Getenv("INFINITE_YOU_INTEGRATION_BINARY")
	if binary == "" {
		t.Skip("set INFINITE_YOU_INTEGRATION_BINARY to a prebuilt CLI")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifact=%s sha256=%x", binary, sha256.Sum256(artifact))
	python, err := exec.LookPath("python")
	if err != nil {
		t.Fatal(err)
	}
	version, err := exec.Command(python, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("python=%s", version)
	project, factory, home := preparePayloadFactory(t, python)
	source, successor := filepath.Join(project, "source.json"), filepath.Join(project, "successor.json")
	sessionID := uuid.NewString()
	host := startMissionHost(t, binary, project, factory, home, sessionID, "", source)
	const payload = `{"mission":"restart café 😀 and release exact cross-batch target","title":"own submitted mission"}`
	runMissionCLI(t, binary, project, home, host.url, sessionID, "submit", "batch", `{"requestId":"idea-batch","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"held-idea","name":"idea","workTypeName":"idea","payload":{"title":"not the mission"}}]}`)
	runMissionCLI(t, binary, project, home, host.url, sessionID, "submit", "batch", `{"requestId":"thoughts-batch","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"restored-thought","name":"thought","workTypeName":"thoughts","tags":{"project":"worker-session-visibility"},"payload":`+payload+`}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"thought","targetWorkId":"held-idea","requiredState":"complete"}]}`)
	before := readMissionWork(t, host.url, sessionID)
	assertRealMissionWork(t, before, payload, "init")
	if _, err := os.Stat(filepath.Join(project, "thought.json")); !os.IsNotExist(err) {
		t.Fatalf("blocked mission executed: %v", err)
	}
	stopMissionHost(t, host)
	newSessionID := uuid.NewString()
	host = startMissionHost(t, binary, project, factory, home, newSessionID, source, successor)
	after := readMissionWork(t, host.url, newSessionID)
	assertRealMissionWork(t, after, payload, "init")
	if !reflect.DeepEqual(before.Tags, after.Tags) || !reflect.DeepEqual(before.Relations, after.Relations) {
		t.Fatal("restart changed tag/relation")
	}
	if _, err := os.Stat(filepath.Join(project, "thought.json")); !os.IsNotExist(err) {
		t.Fatalf("restored blocked mission executed: %v", err)
	}
	missionHTTP(t, http.MethodPost, host.url+"/factory-sessions/"+newSessionID+"/work/held-idea/move", []byte(`{"stateName":"complete"}`))
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		after = readMissionWork(t, host.url, newSessionID)
		if after.State != nil && after.State.Name == "mission-ready" {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("mission did not route: %#v; host=%s", after, host.output.text())
		}
	}
	assertRealMissionWork(t, after, payload, "mission-ready")
	assertCompletePayloadIngress(t, project, map[string]string{"thought": payload})
	stopMissionHost(t, host)
	assertRealMissionFingerprint(t, successor, payload)
	t.Logf("restored %s -> %s; mission-ready; real ingress and SCRIPT_REQUEST bytes=%d sha256=%x", sessionID, newSessionID, len(payload), sha256.Sum256([]byte(payload)))
}

func assertRealMissionFingerprint(t *testing.T, recording, payload string) {
	t.Helper()
	count := 0
	for _, event := range testutil.LoadReplayArtifact(t, recording).Events {
		if event.Type != "SCRIPT_REQUEST" {
			continue
		}
		count++
		var request factoryapi.ScriptRequestEventPayload
		if err := json.Unmarshal(event.Payload, &request); err != nil {
			t.Fatal(err)
		}
		if request.StdinByteLength == nil || *request.StdinByteLength != int64(len(payload)) || request.StdinSha256 == nil || *request.StdinSha256 != fmt.Sprintf("%x", sha256.Sum256([]byte(payload))) {
			t.Fatalf("recorded request disagrees with real ingress: %#v", request)
		}
	}
	if count != 1 {
		t.Fatalf("script attempt count=%d, want one", count)
	}
}

func startMissionHost(t *testing.T, binary, project, factory, home, sessionID, resume, recording string) *missionHost {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	args := []string{"run", "--dir", factory, "--session", sessionID, "--continuously", "--with-server", "--listen", address, "--record", recording}
	if resume != "" {
		args = append(args, "--resume", resume)
	}
	host := &missionHost{url: "http://" + address, done: make(chan struct{})}
	host.cmd = exec.CommandContext(t.Context(), binary, args...)
	host.cmd.Dir, host.cmd.Env = project, builtcliacceptance.ProcessEnvForIsolatedHome(home)
	host.cmd.Stdout, host.cmd.Stderr = &host.output, &host.output
	if err := host.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { host.err = host.cmd.Wait(); close(host.done) }()
	t.Cleanup(func() {
		select {
		case <-host.done:
			return
		default:
		}
		_ = host.cmd.Process.Kill()
		<-host.done
	})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, host.url+"/factory-sessions/"+sessionID, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return host
			}
		}
		select {
		case <-ticker.C:
		case <-host.done:
			t.Fatalf("host stopped before readiness: %v %s", host.err, host.output.text())
		case <-ctx.Done():
			t.Fatalf("host not ready: %s", host.output.text())
		}
	}
}

func stopMissionHost(t *testing.T, host *missionHost) {
	t.Helper()
	missionHTTP(t, http.MethodPost, host.url+"/shutdown", []byte(`{}`))
	select {
	case <-host.done:
		if host.err != nil {
			t.Fatalf("host shutdown: %v %s", host.err, host.output.text())
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("host did not join: %s", host.output.text())
	}
}

func runMissionCLI(t *testing.T, binary, project, home, baseURL, sessionID string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, append([]string{"--server", baseURL, "--session", sessionID, "--json"}, args...)...)
	command.Dir, command.Env = project, builtcliacceptance.ProcessEnvForIsolatedHome(home)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("CLI: %v %s", err, output)
	}
}

func missionHTTP(t *testing.T, method, endpoint string, body []byte) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("%s %s: %d %s", method, endpoint, response.StatusCode, data)
	}
	return data
}

func readMissionWork(t *testing.T, baseURL, sessionID string) factoryapi.Work {
	t.Helper()
	var item factoryapi.Work
	if err := json.Unmarshal(missionHTTP(t, http.MethodGet, baseURL+"/factory-sessions/"+sessionID+"/work/restored-thought", nil), &item); err != nil {
		t.Fatal(err)
	}
	return item
}

func assertRealMissionWork(t *testing.T, item factoryapi.Work, payload, state string) {
	t.Helper()
	var expected any
	if err := json.Unmarshal([]byte(payload), &expected); err != nil {
		t.Fatal(err)
	}
	if item.WorkId == nil || *item.WorkId != "restored-thought" || item.State == nil || item.State.Name != state || !reflect.DeepEqual(item.Payload, expected) {
		t.Fatalf("public Work=%#v, want submitted payload in %s", item, state)
	}
	if item.Tags == nil || (*item.Tags)["project"] != "worker-session-visibility" || item.Relations == nil || len(*item.Relations) != 1 || (*item.Relations)[0].TargetWorkId == nil || *(*item.Relations)[0].TargetWorkId != "held-idea" {
		t.Fatalf("public Work lost tags/relation: %#v", item)
	}
}
