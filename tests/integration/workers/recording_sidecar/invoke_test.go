package recording_sidecar_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Separate CLI observers inspect and stop the same host-owned attempt. The
// PATH-owned shim exercises production subprocess adaptation without remote IO.
func TestWorkerSessionInvokeHostedAsyncInspection(t *testing.T) {
	t.Parallel()
	f := newInvokeArtifactFixture(t)
	host := startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
	result := historyCLI(t, f.ctx, f.binary, f.project, f.env,
		"--remote", "--server", host.url, "--json", "worker-sessions", "invoke", "--execution", f.document, "--async")
	var accepted struct {
		Accepted bool   `json:"accepted"`
		ID       string `json:"workerSessionId"`
	}
	if err := json.Unmarshal(result, &accepted); err != nil || !accepted.Accepted || accepted.ID != "t7-artifact-session" {
		t.Fatalf("async admission = %s (%v)", result, err)
	}
	f.waitForProgress(t, host.url)
	show := f.cli(t, host.url, "show", "--worker-session-id", accepted.ID)
	if !bytes.Contains(show, []byte(`"RUNNING"`)) || !bytes.Contains(show, []byte("t7-artifact-model")) {
		t.Fatalf("live show = %s", show)
	}
	active := f.cli(t, host.url, "list", "--history", "active")
	if !bytes.Contains(active, []byte(accepted.ID)) {
		t.Fatalf("active list = %s", active)
	}
	control := f.cli(t, host.url, "cancel", accepted.ID)
	if !bytes.Contains(control, []byte(`"APPLIED"`)) {
		t.Fatalf("cancel = %s", control)
	}
	show = f.cli(t, host.url, "show", "--worker-session-id", accepted.ID)
	if !bytes.Contains(show, []byte(`"CANCELED"`)) || !bytes.Contains(show, []byte("OPERATOR_CANCEL")) {
		t.Fatalf("terminal show = %s", show)
	}
	logs := f.cli(t, host.url, "read", "--worker-session-id", accepted.ID, "--view", "logs")
	if !bytes.Contains(logs, []byte("t7-artifact-progress")) || !bytes.Contains(logs, []byte(`"COMPLETE"`)) || !bytes.Contains(logs, []byte(`"CANCELED"`)) {
		t.Fatalf("canceled captured history = %s", logs)
	}
	f.assertCommand(t)
	host.stop(t, f.ctx, f.binary, f.project, f.env)
}

func TestWorkerSessionInvokeLocalHistoryRestart(t *testing.T) {
	t.Parallel()
	f := newInvokeArtifactFixture(t)
	f.env = append(f.env, "T7_SHIM_COMPLETE=1")
	result := historyCLI(t, f.ctx, f.binary, f.project, f.env,
		"--json", "worker-sessions", "invoke", "--execution", f.document)
	if !bytes.Contains(result, []byte(`"COMPLETED"`)) || !bytes.Contains(result, []byte("history-restart")) {
		t.Fatalf("local synchronous result = %s", result)
	}
	f.assertCommand(t)
	for _, name := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(f.home, name), []byte("not a provider directory"), 0o000); err != nil {
			t.Fatal(err)
		}
	}
	host := startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
	snapshot := readHistorySnapshot(t, f.ctx, f.binary, f.project, f.env, host.url)
	if snapshot.Observation.WorkerSessionId != "t7-artifact-session" || snapshot.Observation.Model == nil || *snapshot.Observation.Model != "t7-artifact-model" {
		t.Fatalf("recovered invocation = %+v", snapshot.Observation)
	}
	show := f.cli(t, host.url, "show", "--worker-session-id", "t7-artifact-session")
	if !bytes.Contains(show, []byte(`"COMPLETED"`)) {
		t.Fatalf("archived show = %s", show)
	}
	// An archived execution has no live owner. A control cannot restore one.
	command := exec.CommandContext(f.ctx, f.binary, "--remote", "--server", host.url, "--json", "worker-sessions", "cancel", "t7-artifact-session")
	command.Dir, command.Env = f.project, f.env
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if err == nil && bytes.Contains(stdout.Bytes(), []byte(`"APPLIED"`)) {
		t.Fatalf("archived session acquired control authority: %s", &stdout)
	}
	if !bytes.Contains(f.cli(t, host.url, "show", "--worker-session-id", "t7-artifact-session"), []byte(`"COMPLETED"`)) {
		t.Fatal("archived control changed terminal")
	}
	t.Logf("archived cancel exit=%v stdout=%s stderr=%s", err, &stdout, &stderr)
	host.stop(t, f.ctx, f.binary, f.project, f.env)
}

type invokeArtifactFixture struct {
	ctx                             context.Context
	binary, project, home, document string
	env                             []string
}

func invokeArtifactBinary(t *testing.T) string {
	t.Helper()
	binary := os.Getenv("INFINITE_YOU_PREBUILT_ARTIFACT")
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
	t.Logf("artifact sha256=%x; shim sha256=%x; OS=%s", sha256.Sum256(artifact), sha256.Sum256([]byte(invokeCodexShim)), runtime.GOOS)
	return binary
}

func newInvokeArtifactFixture(t *testing.T) invokeArtifactFixture {
	t.Helper()
	binary := invokeArtifactBinary(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	factory := filepath.Join(project, "factory")
	if err := os.Mkdir(factory, 0o700); err != nil {
		t.Fatal(err)
	}
	definition := `{"name":"t7-invocation","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],"workers":[],"workstations":[]}`
	if err := os.WriteFile(filepath.Join(factory, "factory.json"), []byte(definition), 0o600); err != nil {
		t.Fatal(err)
	}
	home, temp, provider := filepath.Join(project, "home"), filepath.Join(project, "temp"), filepath.Join(project, "provider")
	for _, path := range []string{home, temp, provider} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(provider, "shim.cjs")
	if err := os.WriteFile(script, []byte(invokeCodexShim), 0o600); err != nil {
		t.Fatal(err)
	}
	name, launcher := "codex", "#!/bin/sh\nexec '"+strings.ReplaceAll(node, "'", "'\"'\"'")+"' '"+script+"' \"$@\"\n"
	if runtime.GOOS == "windows" {
		name, launcher = "codex.cmd", "@echo off\r\n\""+node+"\" \"%~dp0shim.cjs\" %*\r\nexit /b %errorlevel%\r\n"
	}
	if err := os.WriteFile(filepath.Join(provider, name), []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
	env := cleanupEnvironment(home, temp)
	for i, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "PATH") {
			env[i] = key + "=" + provider + string(os.PathListSeparator) + value
		}
	}
	env = append(env, "T7_SHIM_RECORD="+filepath.Join(project, "command.json"))
	document := filepath.Join(project, "execution.json")
	raw, err := json.Marshal(map[string]any{
		"requestId": "t7-artifact-request", "workerSessionId": "t7-artifact-session",
		"execution": map[string]any{
			"workstationName": "direct", "workingDirectory": project, "workerType": "direct-worker",
			"runnerId": "codex", "executorProvider": "codex", "modelProvider": "codex", "model": "t7-artifact-model",
			"reasoningEffort": "HIGH", "systemPrompt": "t7 artifact system", "userMessage": "t7 artifact user",
			"envVars":  map[string]string{"T7_SYNTHETIC_SETTING": "controlled-value", "T7_SHIM_RECORD": filepath.Join(project, "command.json")},
			"dispatch": map[string]any{"dispatchId": "t7-artifact-attempt", "workstationName": "direct", "workerType": "direct-worker"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(document, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	return invokeArtifactFixture{ctx: ctx, binary: binary, project: project, home: home, document: document, env: env}
}

func (f invokeArtifactFixture) cli(t *testing.T, server string, args ...string) []byte {
	t.Helper()
	return historyCLI(t, f.ctx, f.binary, f.project, f.env, append([]string{"--remote", "--server", server, "--json", "worker-sessions"}, args...)...)
}

func (f invokeArtifactFixture) waitForProgress(t *testing.T, server string) {
	t.Helper()
	// Public finite log reads are the only cross-process progress boundary;
	// polling waits for a committed prefix, not elapsed time or provider files.
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		logs := f.cli(t, server, "read", "--worker-session-id", "t7-artifact-session", "--view", "logs")
		if bytes.Contains(logs, []byte("t7-artifact-progress")) {
			return
		}
		select {
		case <-ticker.C:
		case <-f.ctx.Done():
			t.Fatal(f.ctx.Err())
		}
	}
}

func (f invokeArtifactFixture) assertCommand(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.project, "command.json"))
	if err != nil {
		t.Fatal(err)
	}
	var command struct {
		Args                []string
		Stdin, Cwd, Setting string
	}
	if err := json.Unmarshal(raw, &command); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(command.Args, " ")
	if !strings.Contains(args, "--model t7-artifact-model") || !strings.Contains(args, `model_reasoning_effort="high"`) ||
		!strings.Contains(args, "t7 artifact system") || command.Stdin != "t7 artifact user" || command.Cwd != f.project || command.Setting != "controlled-value" {
		t.Fatalf("compiled command boundary = %s", raw)
	}
	t.Logf("controlled command boundary=%s", raw)
}

const invokeCodexShim = `const fs = require('fs');
const args = process.argv.slice(2);
if (!args.includes('exec')) { console.log('codex fixture 1.0'); process.exit(0); }
let input = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', data => input += data);
process.stdin.on('end', () => {
 fs.writeFileSync(process.env.T7_SHIM_RECORD, JSON.stringify({args, stdin: input, cwd: process.cwd(), setting: process.env.T7_SYNTHETIC_SETTING}));
 const progress = process.env.T7_SHIM_READY ? 'history-restart partial' : 't7-artifact-progress';
 if (process.env.T7_SHIM_READY) fs.writeFileSync(process.env.T7_SHIM_READY, String(process.pid));
 console.log(JSON.stringify({type:'item.completed', item:{id:'progress', type:'command_execution', command:'synthetic inspection', aggregated_output:progress, exit_code:0}}));
 if (process.env.T7_SHIM_COMPLETE === '1') {
  console.log(JSON.stringify({type:'thread.started', thread_id:'t7-artifact-thread'}));
  console.log(JSON.stringify({type:'item.completed', item:{id:'final', type:'agent_message', text:'history-restart COMPLETE'}}));
  console.log(JSON.stringify({type:'turn.completed', usage:{input_tokens:3, output_tokens:2}}));
 } else {
  // Windows launches .cmd through a parent process. This cooperative fixture
  // exits when that exact parent is canceled; it does not claim force-kill.
  const parent = process.ppid;
  setInterval(() => { if (process.platform === 'win32') { try { process.kill(parent, 0); } catch { process.exit(0); } } }, 50);
 }
});
`
