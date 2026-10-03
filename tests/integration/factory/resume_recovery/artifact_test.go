package resume_recovery_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// The build lane supplies one artifact. These two cases exercise OS exit,
// filesystem and HTTP behavior; the larger policy matrix stays functional.
func TestPrebuiltResume(t *testing.T) {
	binary := resumeArtifact(t)
	t.Run("consumed-failed-cron", func(t *testing.T) {
		t.Parallel()
		fixture := newArtifactFixture(t, false)
		daemon := fixture.start(t, binary)
		fixture.waitReady(t, daemon)
		output := fixture.command(t, binary, "work", "list", "--server", fixture.endpoint, "--session", fixture.session, "--json")
		var board factoryapi.ListWorkResponse
		if err := json.Unmarshal(output, &board); err != nil {
			t.Fatal(err)
		}
		assertBoard(t, board, map[string]string{"ordinary-init": "init", "ordinary-blocked": "blocked"})
		for _, item := range board.Results {
			if item.WorkId != nil && *item.WorkId == "historical-cron" {
				t.Fatal("consumed cron was reseeded")
			}
		}
		assertCronHistory(t, fixture.ctx, fixture.endpoint, fixture.session, workersOutcomeFailed)
		fixture.shutdown(t, daemon)
		fixture.assertSourceAndPort(t)
	})
	t.Run("corrupt-recording", func(t *testing.T) {
		t.Parallel()
		fixture := newArtifactFixture(t, true)
		cmd := fixture.resumeCommand(binary)
		output, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() == 0 {
			t.Fatalf("corrupt resume exit: %v; %s", err, output)
		}
		var response factoryapi.ErrorResponse
		if err := json.Unmarshal(bytes.TrimSpace(output), &response); err != nil {
			t.Fatalf("error envelope: %v; %s", err, output)
		}
		if string(response.Code) != "SERVER_START_FAILED" || response.Family != factoryapi.ErrorFamilyInternalServerError {
			t.Fatalf("error envelope = %#v", response)
		}
		for _, want := range []string{"work-corrupt", "task:missing", "not present in the current Factory topology"} {
			if !strings.Contains(response.Message, want) {
				t.Fatalf("diagnostic %q missing %q", response.Message, want)
			}
		}
		if bytes.Contains(output, []byte("PRIVATE-PROMPT")) {
			t.Fatal("private content leaked")
		}
		fixture.assertSourceAndPort(t)
	})
}

func resumeArtifact(t *testing.T) string {
	t.Helper()
	path := os.Getenv("INFINITE_YOU_INTEGRATION_BINARY")
	if path == "" {
		path = os.Getenv("INFINITE_YOU_PREBUILT_ARTIFACT")
	}
	if path == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("prebuilt CLI required; tests never build or discover an ambient CLI")
		}
		t.Skip("set INFINITE_YOU_PREBUILT_ARTIFACT to run compiled resume cases")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(abs)
	if err != nil || len(data) == 0 {
		t.Fatalf("read artifact: %v", err)
	}
	t.Logf("artifact=%s sha256=%x", abs, sha256.Sum256(data))
	return abs
}

type artifactFixture struct {
	ctx                                                 context.Context
	cwd, checkout, source, successor, endpoint, session string
	env                                                 []string
	payload                                             []byte
}

type artifactDaemon struct {
	cmd            *exec.Cmd
	done           chan struct{}
	err            error
	stdout, stderr bytes.Buffer
}

func (fixture *artifactFixture) resumeCommand(binary string) *exec.Cmd {
	cmd := exec.CommandContext(fixture.ctx, binary, "run", "--continuously", "--session", fixture.session,
		"--dir", fixture.cwd, "--resume", fixture.source, "--record", fixture.successor,
		"--with-mock-workers", "--with-server", "--listen", strings.TrimPrefix(fixture.endpoint, "http://"), "--quiet")
	cmd.Dir, cmd.Env = fixture.checkout, fixture.env
	return cmd
}

func (fixture *artifactFixture) start(t *testing.T, binary string) *artifactDaemon {
	t.Helper()
	daemon := &artifactDaemon{cmd: fixture.resumeCommand(binary), done: make(chan struct{})}
	daemon.cmd.Stdout, daemon.cmd.Stderr = &daemon.stdout, &daemon.stderr
	if err := daemon.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { daemon.err = daemon.cmd.Wait(); close(daemon.done) }()
	t.Cleanup(func() {
		select {
		case <-daemon.done:
		default:
			_ = daemon.cmd.Process.Kill()
			<-daemon.done
		}
		fixture.assertSourceAndPort(t)
	})
	return daemon
}

func (fixture *artifactFixture) waitReady(t *testing.T, daemon *artifactDaemon) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	// An OS child has no injectable readiness channel. Poll its session's public
	// Work read (the global status addresses the default session), then assert
	// the actual restored board via the compiled CLI.
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(fixture.ctx, http.MethodGet, fixture.endpoint+"/factory-sessions/"+fixture.session+"/work", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(req)
		if err == nil {
			var board factoryapi.ListWorkResponse
			err = json.NewDecoder(response.Body).Decode(&board)
			_ = response.Body.Close()
			if err == nil && response.StatusCode == http.StatusOK && len(board.Results) >= 2 {
				return
			}
		}
		select {
		case <-daemon.done:
			t.Fatalf("resume exited before readiness: %v; %s %s", daemon.err, &daemon.stdout, &daemon.stderr)
		case <-fixture.ctx.Done():
			t.Fatal("resume readiness: ", fixture.ctx.Err())
		case <-ticker.C:
		}
	}
}

func (fixture *artifactFixture) command(t *testing.T, binary string, args ...string) []byte {
	t.Helper()
	cmd := exec.CommandContext(fixture.ctx, binary, args...)
	cmd.Dir, cmd.Env = fixture.checkout, fixture.env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("CLI %v: %v; %s", args, err, &stderr)
	}
	return output
}

func (fixture *artifactFixture) shutdown(t *testing.T, daemon *artifactDaemon) {
	t.Helper()
	req, err := http.NewRequestWithContext(fixture.ctx, http.MethodPost, fixture.endpoint+"/shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("shutdown HTTP %d", response.StatusCode)
	}
	select {
	case <-daemon.done:
		if daemon.err != nil {
			t.Fatalf("resume exit: %v; %s", daemon.err, &daemon.stderr)
		}
	case <-fixture.ctx.Done():
		t.Fatal("owned process did not join: ", fixture.ctx.Err())
	}
}

func (fixture *artifactFixture) assertSourceAndPort(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(fixture.source)
	if err != nil || !bytes.Equal(data, fixture.payload) {
		t.Fatalf("source changed: %v", err)
	}
	listener, err := net.Listen("tcp", strings.TrimPrefix(fixture.endpoint, "http://"))
	if err != nil {
		t.Fatalf("owned port not released: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}

func newArtifactFixture(t *testing.T, corrupt bool) *artifactFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	t.Cleanup(cancel)
	cwd, home := t.TempDir(), t.TempDir()
	// Invoke this package from the build lane's disposable checkout. The child
	// owns a separate synthetic Factory directory and fresh profile.
	checkout, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	session := uuid.NewString()
	factory, payload := failedCronFactory(), cronPayload(t, session, workersOutcomeFailed)
	if corrupt {
		factory, payload = corruptResumeFactory(), corruptResumePayload(t, session)
	}
	writeFixture(t, filepath.Join(cwd, "factory.json"), mustJSON(t, factory))
	if !corrupt {
		writeFixture(t, filepath.Join(cwd, "workstations", "cron-refresh", "AGENTS.md"), []byte("---\ntype: MODEL_WORKSTATION\n---\nSynthetic no-op.\n"))
	}
	source := filepath.Join(cwd, "source.json")
	writeFixture(t, source, payload)
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "http://" + probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	return &artifactFixture{ctx: ctx, cwd: cwd, checkout: checkout, session: session, source: source,
		successor: filepath.Join(cwd, "successor.json"), endpoint: endpoint, payload: payload,
		env: isolatedEnvironment(home)}
}

func writeFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func isolatedEnvironment(home string) []string {
	values := map[string]string{"HOME": home, "USERPROFILE": home, "APPDATA": filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA": filepath.Join(home, "AppData", "Local"), "HOMEDRIVE": "", "HOMEPATH": ""}
	env := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, overridden := values[strings.ToUpper(key)]; !overridden {
			env = append(env, entry)
		}
	}
	for key, value := range values {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	return env
}
