package recording_sidecar_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// An abrupt OS host loss must preserve the acknowledged prefix without restoring
// a live execution handle or inventing a completed capture on the next host.
func TestWorkerSessionHistoryRestartIncomplete(t *testing.T) {
	t.Parallel()
	for _, direct := range []bool{false, true} {
		name := "factory"
		if direct {
			name = "direct"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runIncompleteHistoryRestart(t, direct)
		})
	}
}

func runIncompleteHistoryRestart(t *testing.T, direct bool) {
	t.Helper()
	binary, project, home, factory, ready, env := incompleteHistoryFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	if direct {
		consumeHistorySeeds(t, factory)
	}
	first := startHistoryHost(t, ctx, binary, project, env, "--dir", factory, "--continuously")
	if direct {
		historyCLI(t, ctx, binary, project, env, "--remote", "--server", first.url, "--json", "worker-sessions", "invoke",
			"--async", "--workstation", "process", "--worker-type", "SCRIPT_WORKER", "--user-message", "Capture partial progress")
	}
	observation, prefix := awaitHistoryPrefix(t, ctx, binary, project, env, first.url, ready)
	assertLiveHistoryObservation(t, direct, observation)
	assertDeliveredHistoryFollow(t, ctx, binary, project, env, first.url, prefix)
	stillActive := historyRows(t, ctx, binary, project, env, first.url, "active")
	if len(stillActive) != 1 || stillActive[0].WorkerSessionId != observation.WorkerSessionId || stillActive[0].State != "RUNNING" {
		t.Fatalf("observer cancellation changed worker: %+v", stillActive)
	}
	if err := first.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.done:
		first.stopped = true
	case <-ctx.Done():
		t.Fatal("killed host did not join")
	}
	prepareHistoryRestart(t, factory, home)
	second := startHistoryHost(t, ctx, binary, project, env)
	active := historyRows(t, ctx, binary, project, env, second.url, "active")
	if len(active) != 0 {
		t.Fatalf("dead execution restored active: %+v", active)
	}
	archived := historyRows(t, ctx, binary, project, env, second.url, "archived")
	if len(archived) != 1 {
		t.Fatalf("incomplete archive = %+v", archived)
	}
	assertIncompleteHistoryObservation(t, observation, archived[0])
	assertArchivedHistoryControlRefused(t, ctx, binary, project, env, second.url, observation.WorkerSessionId)
	replayed := historyLogPage(t, ctx, binary, project, env, second.url, observation.WorkerSessionId, "")
	assertIncompleteHistoryPrefix(t, prefix, replayed)
	tail := historyLogPage(t, ctx, binary, project, env, second.url, observation.WorkerSessionId, *prefix.NextToken)
	if len(tail.Events) != 0 || tail.CommittedPosition != prefix.CommittedPosition || tail.Health != "INCOMPLETE" {
		t.Fatalf("reconnect duplicated or invented records: %+v", tail)
	}
	second.stop(t, ctx, binary, project, env)
}

func assertArchivedHistoryControlRefused(t *testing.T, ctx context.Context, binary, project string, env []string, server, id string) {
	t.Helper()
	command := exec.CommandContext(ctx, binary, "--remote", "--server", server, "--json", "worker-sessions", "cancel", id)
	command.Dir, command.Env = project, env
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err == nil {
		t.Fatal("archived execution accepted live cancellation")
	}
	var diagnostic struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &diagnostic); err != nil || diagnostic.Code != "NOT_FOUND" || stdout.Len() != 0 {
		t.Fatalf("archived control refusal: %s; stderr=%s; decode=%v", &stdout, &stderr, err)
	}
}

func assertLiveHistoryObservation(t *testing.T, direct bool, observation api.WorkerSessionObservation) {
	t.Helper()
	if observation.Direct != direct || (!direct && observation.FactorySessionId == nil) || observation.State != "RUNNING" {
		t.Fatalf("live observation (direct=%t) = %+v", direct, observation)
	}
}

func assertDeliveredHistoryFollow(t *testing.T, ctx context.Context, binary, project string, env []string, server string, prefix api.WorkerSessionLogPage) {
	t.Helper()
	observeCtx, stop := context.WithCancel(ctx)
	defer stop()
	output := &historyOutput{changed: make(chan struct{}, 1)}
	var stderr bytes.Buffer
	command := exec.CommandContext(observeCtx, binary, "--server", server, "--json", "worker-sessions", "read",
		"--worker-session-id", prefix.WorkerSessionId, "--view", "logs", "--follow")
	command.Dir, command.Env, command.Stdout, command.Stderr = project, env, output, &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	joined := false
	defer func() {
		stop()
		if !joined {
			<-done
		}
	}()
	for {
		data := output.snapshot()
		if bytes.HasSuffix(data, []byte("\n")) && bytes.Contains(data, []byte("history-restart partial")) {
			stop()
			<-done
			joined = true
			break
		}
		select {
		case <-output.changed:
		case err := <-done:
			joined = true
			t.Fatalf("live observer exited: %v: %s", err, &stderr)
		case <-ctx.Done():
			t.Fatal("delivered follow did not emit prefix")
		}
	}
	var followed []api.WorkerSessionEvent
	for _, line := range bytes.Split(bytes.TrimSpace(output.snapshot()), []byte("\n")) {
		var event api.WorkerSessionEvent
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("follow frame: %v: %s", err, line)
		}
		followed = append(followed, event)
	}
	if !reflect.DeepEqual(followed, prefix.Events) {
		t.Fatalf("follow changed acknowledged records: followed=%+v captured=%+v", followed, prefix.Events)
	}
}

func incompleteHistoryFixture(t *testing.T) (binary, project, home, factory, ready string, env []string) {
	t.Helper()
	binary = os.Getenv("INFINITE_YOU_PREBUILT_ARTIFACT")
	if binary == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("INFINITE_YOU_PREBUILT_ARTIFACT is required")
		}
		t.Skip("prebuilt CLI required")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("prebuilt artifact sha256=%x", sha256.Sum256(artifact))
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	project = t.TempDir()
	home = filepath.Join(project, "home")
	temp := filepath.Join(project, "temp")
	for _, path := range []string{home, temp} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	ready = filepath.Join(project, "ready")
	factory = writeCleanupFactory(t, project, node, ready, filepath.Join(project, "release"))
	// This fixture never finishes while the host owns it. The PID gate lets
	// cleanup target only this test's script after the host is killed.
	script := "const fs=require('fs'); fs.writeFileSync(process.argv[2],String(process.pid)); console.log('history-restart partial'); setInterval(()=>{},1000);"
	if err := os.WriteFile(filepath.Join(project, "worker.cjs"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	env = cleanupEnvironment(home, temp)
	cleanupHistoryChild(t, ready)
	return binary, project, home, factory, ready, env
}

func cleanupHistoryChild(t *testing.T, ready string) {
	t.Helper()
	t.Cleanup(func() {
		data, readErr := os.ReadFile(ready)
		if readErr != nil {
			return
		}
		pid, parseErr := strconv.Atoi(string(data))
		if parseErr != nil || pid <= 0 {
			t.Errorf("fixture PID: %q (%v)", data, parseErr)
			return
		}
		child, findErr := os.FindProcess(pid)
		if findErr == nil {
			_ = child.Kill()
		}
	})
}

func prepareHistoryRestart(t *testing.T, factory, home string) {
	t.Helper()
	consumeHistorySeeds(t, factory)
	for _, name := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("not a provider directory"), 0o000); err != nil {
			t.Fatal(err)
		}
	}
}

func consumeHistorySeeds(t *testing.T, factory string) {
	t.Helper()
	// Consume input so the new host cannot admit a replacement for the seed.
	seeds, err := filepath.Glob(filepath.Join(factory, "inputs", "task", "*", "seed-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range seeds {
		if err := os.Remove(seed); err != nil {
			t.Fatal(err)
		}
	}
}

func assertIncompleteHistoryObservation(t *testing.T, observation, ended api.WorkerSessionObservation) {
	t.Helper()
	if ended.WorkerSessionId != observation.WorkerSessionId || ended.AttemptId != observation.AttemptId ||
		ended.State == "RUNNING" || ended.State == "COMPLETED" || ended.ConfirmationState != "UNCONFIRMED" ||
		ended.RecordingHealth == nil || *ended.RecordingHealth != "INCOMPLETE" || ended.EndedAt != nil || ended.DurationMillis != nil {
		t.Fatalf("invented recovered outcome: %+v", ended)
	}
}

func assertIncompleteHistoryPrefix(t *testing.T, prefix, replayed api.WorkerSessionLogPage) {
	t.Helper()
	if replayed.Health != "INCOMPLETE" || replayed.CommittedPosition != prefix.CommittedPosition ||
		replayed.RecordingGenerationId != prefix.RecordingGenerationId || !reflect.DeepEqual(replayed.Events, prefix.Events) {
		t.Fatalf("lost acknowledged prefix: before=%+v after=%+v", prefix, replayed)
	}
	if prefix.NextToken == nil {
		t.Fatal("live prefix omitted reconnect token")
	}
}

func awaitHistoryPrefix(t *testing.T, ctx context.Context, binary, project string, env []string, server, ready string) (api.WorkerSessionObservation, api.WorkerSessionLogPage) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			rows := historyRows(t, ctx, binary, project, env, server, "active")
			if len(rows) == 1 {
				page := historyLogPage(t, ctx, binary, project, env, server, rows[0].WorkerSessionId, "")
				encoded, err := json.Marshal(page.Events)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(encoded, []byte("history-restart partial")) && page.Health == "INCOMPLETE" {
					return rows[0], page
				}
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("live capture prefix did not commit")
		}
	}
}

func historyRows(t *testing.T, ctx context.Context, binary, project string, env []string, server, selector string) []api.WorkerSessionObservation {
	t.Helper()
	data := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "list", "--history", selector)
	var rows api.ListWorkerSessionsResponse
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatalf("history %s: %v: %s", selector, err, data)
	}
	return rows.Sessions
}

func historyLogPage(t *testing.T, ctx context.Context, binary, project string, env []string, server, id, token string) api.WorkerSessionLogPage {
	t.Helper()
	args := []string{"--server", server, "--json", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs"}
	if token != "" {
		args = append(args, "--next-token", token)
	}
	data := historyCLI(t, ctx, binary, project, env, args...)
	var page api.WorkerSessionLogPage
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatalf("captured prefix: %v: %s", err, data)
	}
	return page
}
