package restart_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

// This finite batch consumes the invoking build's CLI and crosses the real
// persistence filesystem and OS exit boundaries. Continuous hosts separately
// retain the listener after their engine fails; this cell owns terminal stderr.
func TestPrebuiltDurableSnapshotRecoveryAndFailure(t *testing.T) {
	t.Parallel()
	binary := requireRestartCLIArtifact(t)
	factory := scaffoldBoardPersistenceFactory(t, boardPersistenceFactoryConfig())
	home := t.TempDir()
	writeBoardPersistenceAgentConfig(t, factory, "restart-blocker", boardPersistenceWorkerConfig(currentRestartWorkerExecutable(t)))
	release := filepath.Join(t.TempDir(), "released")
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(factory, ".you-agent-factory", "durable-sessions")
	if err := os.MkdirAll(filepath.Dir(directory), 0700); err != nil {
		t.Fatal(err)
	}
	const sentinel = "preserve obstructing file"
	if err := os.WriteFile(directory, []byte(sentinel), 0600); err != nil {
		t.Fatal(err)
	}
	batch := `{"requestId":"snapshot-writer-fault","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"snapshot-writer-fault","name":"persist","workTypeName":"task","state":"processing","content":[{"type":"text","text":"persist this Work"}]}]}`
	workPath := filepath.Join(t.TempDir(), "work.json")
	if err := os.WriteFile(workPath, []byte(batch), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	args := []string{"--json", "run", "--dir", factory, "--work", workPath, "--no-record"}
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = factory
	command.Env = append(builtcliacceptance.ProcessEnvForIsolatedHome(home), boardPersistenceHelperEnv+"="+boardPersistenceHelperEnvValue, boardPersistenceReleaseEnv+"="+release)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || ctx.Err() != nil {
		t.Fatalf("filesystem failure exit=%v context=%v stdout=%q stderr=%q", err, ctx.Err(), stdout.String(), stderr.String())
	}
	if strings.Count(stderr.String(), "create durable session persistence directory") != 1 {
		t.Fatalf("terminal persistence diagnostic missing or duplicated: %q", stderr.String())
	}
	var diagnostic struct{ Code, Message string }
	if err := json.Unmarshal(stderr.Bytes(), &diagnostic); err != nil || diagnostic.Code != "DURABLE_SESSION_PERSISTENCE_FAILED" || diagnostic.Message != "create durable session persistence directory failed" {
		t.Fatalf("unsafe or malformed terminal diagnostic: %q (%v)", stderr.String(), err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n")) {
		if len(line) > 0 && !json.Valid(line) {
			t.Fatalf("non-JSON stdout on --json failure: %q", line)
		}
	}
	contents, readErr := os.ReadFile(directory)
	if readErr != nil || string(contents) != sentinel {
		t.Fatalf("failure changed obstruction: %q %v", contents, readErr)
	}
	t.Logf("I2 artifact SHA256=%s argv=%q exit=1 stdout_bytes=%d stderr=%q", restartCLIArtifact.SHA256, args, stdout.Len(), stderr.String())
}
