package subagent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

type watcherProvider struct{ gates map[string]chan struct{} }

func (p *watcherProvider) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	case <-p.gates[string(request.Stdin)]:
	}
	text := "Published\r\n" + string(request.Stdin)
	data, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{"id": "final", "type": "agent_message", "text": text}})
	return platformprocess.CommandResult{Stdout: append(data, []byte("\n{\"type\":\"turn.completed\"}\n")...)}, nil
}

type compiledWatcher struct {
	command *exec.Cmd
	lines   <-chan string
	stderr  *bytes.Buffer
}

func launchWatcher(t *testing.T, ctx context.Context, binary, directory string, env []string, host string, ids ...string) compiledWatcher {
	t.Helper()
	args := []string{"--server", host, "worker-sessions", "follow", "--terminal"}
	args = append(args, ids...)
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir, command.Env = directory, env
	prepareObserverSignal(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	lines := make(chan string, 10)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	return compiledWatcher{command, lines, stderr}
}
func compiledLine(t *testing.T, ctx context.Context, watcher compiledWatcher, id string) string {
	t.Helper()
	select {
	case line, ok := <-watcher.lines:
		if !ok || !strings.Contains(line, fmt.Sprintf("workerSessionId=%q", id)) {
			t.Fatalf("compiled line=%q stderr=%s", line, watcher.stderr)
		}
		return line
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return ""
}
func compiledExit(t *testing.T, watcher compiledWatcher, code int) {
	t.Helper()
	// Drain the pipe before Wait closes its handles.
	for line := range watcher.lines {
		t.Errorf("unexpected terminal line=%q", line)
	}
	err := watcher.command.Wait()
	actual := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			actual = exit.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	if actual != code {
		t.Fatalf("exit=%d want=%d stderr=%s", actual, code, watcher.stderr)
	}
}
func startWatcherAttempt(t *testing.T, ctx context.Context, binary, directory string, env []string, host, id string) {
	t.Helper()
	runCLI(t, ctx, binary, directory, env, "--server", host, "--json", "worker-sessions", "start", "--request-id", id+"-request", "--worker-session-id", id, "--provider", "codex", "--model", "controlled-model", "--working-root", directory, id)
}

// Two real callers/OS pipes consume one built artifact against canonical host
// wiring and controlled Providers. Unit/functional tests own the larger matrix.
func TestManySessionTerminalFollowLineWatcher(t *testing.T) {
	t.Parallel()
	binary := prebuiltCLI(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	directory, home := t.TempDir(), t.TempDir()
	writeCurrentFactory(t, directory)
	env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	runner := &watcherProvider{gates: map[string]chan struct{}{"a": make(chan struct{}), "b": make(chan struct{})}}
	host := startHost(t, ctx, directory, home, env, runner)
	for _, id := range []string{"a", "b"} {
		startWatcherAttempt(t, ctx, binary, directory, env, host, id)
	}
	watcher := launchWatcher(t, ctx, binary, directory, env, host, "a", "b", "a")
	close(runner.gates["b"])
	line := compiledLine(t, ctx, watcher, "b")
	if !strings.Contains(line, `state=COMPLETED result="Published\r\nb"`) {
		t.Fatal(line)
	}
	// b is observable before the a provider is released: real OS flush witness.
	close(runner.gates["a"])
	compiledLine(t, ctx, watcher, "a")
	compiledExit(t, watcher, 0)
}

func TestManySessionTerminalFollowExit(t *testing.T) {
	t.Parallel()
	binary := prebuiltCLI(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	directory, home := t.TempDir(), t.TempDir()
	writeCurrentFactory(t, directory)
	env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	runner := &watcherProvider{gates: map[string]chan struct{}{"retained": make(chan struct{}), "active": make(chan struct{})}}
	host := startHost(t, ctx, directory, home, env, runner)
	for _, id := range []string{"retained", "active"} {
		startWatcherAttempt(t, ctx, binary, directory, env, host, id)
	}
	close(runner.gates["retained"])
	watcher := launchWatcher(t, ctx, binary, directory, env, host, "retained", "active")
	compiledLine(t, ctx, watcher, "retained")
	if err := interruptObserver(watcher.command); err != nil {
		t.Fatal(err)
	}
	compiledExit(t, watcher, 130)
	assertActive(t, runCLI(t, ctx, binary, directory, env, "--server", host, "--json", "worker-sessions", "show", "--worker-session-id", "active"), "active")
	reattached := launchWatcher(t, ctx, binary, directory, env, host, "active")
	close(runner.gates["active"])
	compiledLine(t, ctx, reattached, "active")
	compiledExit(t, reattached, 0)
	missing := launchWatcher(t, ctx, binary, directory, env, host, "missing")
	compiledExit(t, missing, 1)
	if !strings.Contains(missing.stderr.String(), "WORKER_SESSION_NOT_FOUND") || !strings.Contains(missing.stderr.String(), "missing") || !strings.Contains(missing.stderr.String(), host) {
		t.Fatal(missing.stderr)
	}
	// Public host still answers after observer exits, including cancellation.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/worker-sessions/active", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal(response.Status)
	}
}
