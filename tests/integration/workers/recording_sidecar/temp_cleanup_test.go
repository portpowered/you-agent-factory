package recording_sidecar_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	rootprocess "github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// This single integration journey consumes an externally built CLI and runs a
// real local script worker. Only the child process's owned TEMP/TMP is removed.
func TestWorkerRecordingSurvivesOSTempCleanup(t *testing.T) {
	t.Parallel()
	binary := os.Getenv("YOU_RECORDING_TEST_BINARY")
	if binary == "" {
		binary = os.Getenv("INFINITE_YOU_INTEGRATION_BINARY")
	}
	if binary == "" {
		t.Skip("prebuilt CLI required: set YOU_RECORDING_TEST_BINARY")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("prebuilt artifact: %v", err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("local script worker prerequisite: %v", err)
	}
	project := t.TempDir()
	ownedTemp := filepath.Join(project, "owned-os-temp")
	home := filepath.Join(project, "home")
	for _, path := range []string{ownedTemp, home} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ready, release := filepath.Join(project, "ready"), filepath.Join(project, "release")
	factory := writeCleanupFactory(t, project, node, ready, release)
	recordPath := filepath.Join(project, "factory-recording.json")
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "run", "--dir", factory, "--session", uuid.NewString(), "--quiet", "--record", recordPath)
	command.Dir = project
	command.Env = cleanupEnvironment(home, ownedTemp)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() { cancel() })
	waitForCleanupGate(t, ctx, ready, done)
	assertCleanupWorkerHistory(t, project, recordings.WorkerRecordingStatusIncomplete)
	// The path is a fixed child of this test's isolated project; no host-wide
	// cleanup or path discovery is performed.
	if err := os.RemoveAll(ownedTemp); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(release, []byte("continue"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CLI after temp cleanup: %v\nstdout=%s\nstderr=%s", err, &stdout, &stderr)
		}
	case <-ctx.Done():
		t.Fatal("CLI timed out after cleanup")
	}
	assertCleanupWorkerHistory(t, project, recordings.WorkerRecordingStatusComplete)
	artifact := testutil.LoadReplayArtifact(t, recordPath)
	for _, event := range artifact.Events {
		if string(event.Type) != "DISPATCH_RESPONSE" {
			continue
		}
		var payload struct {
			Outcome string `json:"outcome"`
			Output  string `json:"output"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Outcome == "ACCEPTED" && strings.Contains(payload.Output, "after-temp-cleanup") {
			return
		}
	}
	t.Fatal("public recorded result omitted accepted post-cleanup script output")
}

func writeCleanupFactory(t *testing.T, project, node, ready, release string) string {
	t.Helper()
	factory := filepath.Join(project, "factory")
	for _, path := range []string{filepath.Join(factory, "workers", "worker"), filepath.Join(factory, "workstations", "process")} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := `{"name":"temp-cleanup","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"name":"worker"}],"workstations":[{"name":"process","worker":"worker","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"done"}],"onFailure":[{"workType":"task","state":"failed"}]}]}`
	script := `const fs=require('fs'),os=require('os'); const [ready,release]=process.argv.slice(2); fs.writeFileSync(ready,os.tmpdir()); const timer=setInterval(()=>{if(fs.existsSync(release)){clearInterval(timer); if(fs.existsSync(os.tmpdir()))process.exit(2); console.log('after-temp-cleanup COMPLETE');}},20);`
	scriptPath := filepath.Join(project, "worker.cjs")
	commandJSON, _ := json.Marshal(node)
	argsJSON, _ := json.Marshal([]string{scriptPath, ready, release})
	files := map[string]string{
		filepath.Join(factory, "factory.json"):                         config,
		filepath.Join(factory, "workers", "worker", "AGENTS.md"):       fmt.Sprintf("---\ntype: SCRIPT_WORKER\ncommand: %s\nargs: %s\n---\n", commandJSON, argsJSON),
		filepath.Join(factory, "workstations", "process", "AGENTS.md"): "---\ntype: MODEL_WORKSTATION\n---\nExecute the script.\n",
		scriptPath: script,
	}
	for path, data := range files {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	testutil.WriteSeedFile(t, factory, "task", []byte(`{"title":"temp cleanup"}`))
	return factory
}

func cleanupEnvironment(home, temp string) []string {
	env := make([]string, 0, len(os.Environ())+6)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "HOME", "USERPROFILE", "TEMP", "TMP", "TMPDIR", "YOU_HOME":
			continue
		}
		env = append(env, entry)
	}
	return append(env, "HOME="+home, "USERPROFILE="+home, "YOU_HOME="+home, "TEMP="+temp, "TMP="+temp, "TMPDIR="+temp)
}

func waitForCleanupGate(t *testing.T, ctx context.Context, ready string, done <-chan error) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if data, err := os.ReadFile(ready); err == nil {
			if string(data) != filepath.Join(filepath.Dir(ready), "owned-os-temp") {
				t.Fatalf("worker OS temp = %q", data)
			}
			return
		}
		select {
		case err := <-done:
			t.Fatalf("CLI ended before worker ready gate: %v", err)
		case <-ctx.Done():
			t.Fatal("worker ready gate timed out")
		case <-ticker.C:
		}
	}
}

func assertCleanupWorkerHistory(t *testing.T, project string, want recordings.WorkerRecordingStatus) {
	t.Helper()
	root := filepath.Join(project, ".you-agent-factory", "worker-recordings")
	files, err := filepath.Glob(filepath.Join(root, "*.worker.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("durable journals = %v, error %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var header struct {
		RecordingID string `json:"recordingId"`
	}
	if err := json.Unmarshal(bytes.SplitN(data, []byte("\n"), 2)[0], &header); err != nil {
		t.Fatal(err)
	}
	process, err := rootprocess.BuildProcess(t.Context(), serviceedges.Edges{FactorySessionsWorkingDirectory: cleanupProjectDirectory(project)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := process.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	snapshot, err := rootprocess.WorkerRecordingReaderFromProcess(process).LoadWorkerRecording(t.Context(), header.RecordingID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Status != want {
		t.Fatalf("reloaded Worker history status = %v, want %v", snapshot.Sessions, want)
	}
}

type cleanupProjectDirectory string

func (directory cleanupProjectDirectory) Getwd() (string, error) { return string(directory), nil }
