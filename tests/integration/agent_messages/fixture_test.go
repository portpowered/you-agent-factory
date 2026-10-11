package agent_messages_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type artifactFixture struct {
	ctx         context.Context
	binary, dir string
	env         []string
}

func newArtifactFixture(t *testing.T) *artifactFixture {
	t.Helper()
	binary := os.Getenv("INFINITE_YOU_INTEGRATION_BINARY")
	if binary == "" {
		binary = os.Getenv("INFINITE_YOU_PREBUILT_ARTIFACT")
	}
	if binary == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("prebuilt CLI is required")
		}
		t.Skip("set INFINITE_YOU_INTEGRATION_BINARY to the upstream artifact")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifact sha256=%x; OS=%s; controlled provider only", sha256.Sum256(artifact), runtime.GOOS)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	// Per-invocation environment overrides are deliberately not restartable.
	// The controlled provider's artifact location belongs to the host environment.
	env = append(env, "AM_T5_CREDENTIAL_ROOT="+dir)
	provider := installProviderChild(t, dir)
	for i, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "PATH") {
			env[i] = key + "=" + provider + string(os.PathListSeparator) + value
		}
	}
	definition, err := os.ReadFile(testutil.MustRepoPath(t, "tests/functional_test/testdata/executor_success/factory.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), definition, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	t.Cleanup(cancel)
	return &artifactFixture{ctx: ctx, binary: binary, dir: dir, env: env}
}

func (f *artifactFixture) cli(t *testing.T, env []string, args ...string) []byte {
	t.Helper()
	command := exec.CommandContext(f.ctx, f.binary, args...)
	command.Dir, command.Env = f.dir, env
	var out, diagnostic bytes.Buffer
	command.Stdout, command.Stderr = &out, &diagnostic
	if err := command.Run(); err != nil {
		t.Fatalf("compiled CLI: %v; %s", err, diagnostic.String())
	}
	return out.Bytes()
}

func (f *artifactFixture) message(t *testing.T, url string, caller map[string]string, result any, args ...string) {
	t.Helper()
	env := callerEnvironment(f.env, caller)
	argv := append([]string{"--json", "--server", url, "message"}, args...)
	argv = append(argv, "--output", "json")
	if err := json.Unmarshal(f.cli(t, env, argv...), result); err != nil {
		t.Fatal(err)
	}
}

func callerEnvironment(base []string, caller map[string]string) []string {
	env := append([]string{}, base...)
	if caller != nil {
		env = append(env, "YOU_WORKER_SESSION_ID="+caller["YOU_WORKER_SESSION_ID"], "YOU_WORKER_SESSION_TOKEN="+caller["YOU_WORKER_SESSION_TOKEN"])
	}
	return env
}

func (f *artifactFixture) http(t *testing.T, url, method, path string, value any, caller map[string]string) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(f.ctx, method, url+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if caller != nil {
		req.Header.Set("X-You-Worker-Session-Id", caller["YOU_WORKER_SESSION_ID"])
		req.Header.Set("Authorization", "Bearer "+caller["YOU_WORKER_SESSION_TOKEN"])
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, body
}

func (f *artifactFixture) readThread(t *testing.T, url, id string) api.AgentMessageListResponse {
	t.Helper()
	var page api.AgentMessageListResponse
	f.message(t, url, nil, &page, "list", "--thread", id)
	if len(page.Messages) != 2 {
		t.Fatalf("compiled thread length=%d", len(page.Messages))
	}
	return page
}

type artifactHost struct {
	url     string
	command *exec.Cmd
	done    chan error
	output  *hostOutput
	stopped bool
}

func (f *artifactFixture) startHost(t *testing.T) *artifactHost {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	output := &hostOutput{changed: make(chan struct{}, 1)}
	command := exec.CommandContext(f.ctx, f.binary, "run", "--dir", f.dir, "--continuously", "--with-server", "--listen", address)
	command.Dir, command.Env, command.Stdout, command.Stderr = f.dir, f.env, output, output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	host := &artifactHost{url: "http://" + address, command: command, done: make(chan error, 1), output: output}
	go func() { host.done <- command.Wait() }()
	t.Cleanup(func() {
		if !host.stopped {
			_ = command.Process.Kill()
			<-host.done
		}
	})
	for {
		if bytes.Contains(output.snapshot(), []byte("Dashboard URL: "+host.url)) {
			return host
		}
		select {
		case <-output.changed:
		case err := <-host.done:
			host.stopped = true
			t.Fatalf("host before readiness: %v %s", err, output.snapshot())
		case <-f.ctx.Done():
			t.Fatalf("host readiness: %v %s", f.ctx.Err(), output.snapshot())
		}
	}
}

func (h *artifactHost) stop(t *testing.T, f *artifactFixture) {
	t.Helper()
	f.cli(t, f.env, "--server", h.url, "server", "stop")
	select {
	case err := <-h.done:
		h.stopped = true
		if err != nil {
			t.Fatalf("host join: %v %s", err, h.output.snapshot())
		}
	case <-f.ctx.Done():
		t.Fatal("host did not join")
	}
}

type hostOutput struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	changed chan struct{}
}

func (o *hostOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	n, err := o.buffer.Write(data)
	o.mu.Unlock()
	select {
	case o.changed <- struct{}{}:
	default:
	}
	return n, err
}
func (o *hostOutput) snapshot() []byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	return bytes.Clone(o.buffer.Bytes())
}
