package runtime_startup_test

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func startupOutputValue(output, label string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, label) {
			return strings.TrimSpace(strings.TrimPrefix(line, label))
		}
	}
	return ""
}

func pathUnderDirectory(path, directory string) bool {
	path = filepath.Clean(path)
	directory = filepath.Clean(directory)
	if path == directory {
		return true
	}
	return strings.HasPrefix(path, directory+string(os.PathSeparator))
}

// TestReplayArtifactUnderResolvedHomeStartsAfterDisclosure proves a real
// replay artifact below the resolved home is opened only after the process has
// crossed the home-disclosure boundary and initialized runtime artifacts.
func TestReplayArtifactUnderResolvedHomeStartsAfterDisclosure(t *testing.T) {
	workingDirectory := t.TempDir()
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Factory directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(idleCurrentFactoryJSON), 0o600); err != nil {
		t.Fatalf("write Factory config: %v", err)
	}
	homeDir := t.TempDir()
	replayPath := filepath.Join(homeDir, ".you-agent-factory", "recordings", "root-discovery.replay.json")

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	execute := func(args []string) (string, string, error) {
		var stdout, stderr bytes.Buffer
		stdinIsTTY := true
		stdoutIsTTY := false
		err := process.Execute(root.Input{
			Args:             args,
			Env:              append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir),
			Stdin:            strings.NewReader(""),
			Stdout:           &stdout,
			Stderr:           &stderr,
			Context:          t.Context(),
			WorkingDirectory: workingDirectory,
			StdinIsTTY:       &stdinIsTTY,
			StdoutIsTTY:      &stdoutIsTTY,
		})
		return stdout.String(), stderr.String(), err
	}

	if stdout, stderr, err := execute([]string{"you", "run", "--dir", factoryDir, "--record", replayPath}); err != nil {
		t.Fatalf("record run error = %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Stat(replayPath); err != nil {
		t.Fatalf("recorded replay artifact %q: %v", replayPath, err)
	}

	stdout, stderr, err := execute([]string{"you", "run", "--dir", factoryDir, "--replay", replayPath, "--no-record"})
	if err != nil {
		t.Fatalf("replay run error = %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.HasPrefix(stdout, "Home directory: "+homeDir+"\n") {
		t.Fatalf("replay stdout = %q, want resolved home disclosure first", stdout)
	}
	for _, label := range []string{"Runtime log: ", "Runtime metrics: "} {
		path := startupOutputValue(stdout, label)
		if !pathUnderDirectory(path, homeDir) {
			t.Fatalf("replay %s path = %q, want beneath resolved home %q", label, path, homeDir)
		}
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("stat replay %s artifact %q: %v", label, path, statErr)
		}
	}
	if stderr != "" {
		t.Fatalf("replay stderr = %q, want empty", stderr)
	}

	resumeSuccessorPath := filepath.Join(homeDir, ".you-agent-factory", "recordings", "root-discovery-successor.replay.json")
	stdout, stderr, err = execute([]string{"you", "run", "--dir", factoryDir, "--resume", replayPath, "--record", resumeSuccessorPath})
	if err != nil {
		t.Fatalf("resume run error = %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Stat(resumeSuccessorPath); err != nil {
		t.Fatalf("resume successor artifact %q: %v", resumeSuccessorPath, err)
	}
	if !strings.HasPrefix(stdout, "Home directory: "+homeDir+"\n") {
		t.Fatalf("resume stdout = %q, want resolved home disclosure first", stdout)
	}
	if stderr != "" {
		t.Fatalf("resume stderr = %q, want empty", stderr)
	}
}

// TestServerInitializationFailureStopsBeforeRuntimeArtifacts proves a
// startup contention error is returned before runtime log/metrics creation or
// listener readiness output can occur.
func TestServerInitializationFailureStopsBeforeRuntimeArtifacts(t *testing.T) {
	workingDirectory := t.TempDir()
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Factory directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(idleCurrentFactoryJSON), 0o600); err != nil {
		t.Fatalf("write Factory config: %v", err)
	}

	var listenerCalls atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		SystemInitializationInspectPath: func(string) (fs.FileInfo, error) {
			return nil, fmt.Errorf("%w: another process is staging packaged factories", interfaces.ErrFactoryInstallationContention)
		},
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			listenerCalls.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	stdout, stderr, executeErr := executeFactoryArgs(
		t, process, workingDirectory, []string{"you", "server"}, false, t.Context(),
	)
	if executeErr == nil {
		t.Fatalf("Process.Execute(server) error = nil; stdout=%q stderr=%q", stdout, stderr)
	}
	if strings.Contains(stdout, "Factory initiated:") ||
		strings.Contains(stdout, "Runtime log:") ||
		strings.Contains(stdout, "Runtime metrics:") ||
		strings.Contains(stdout, "Dashboard URL:") {
		t.Fatalf("startup output exposed runtime readiness after initialization failure: %q", stdout)
	}
	if listenerCalls.Load() != 0 {
		t.Fatalf("listener calls = %d, want zero after initialization failure", listenerCalls.Load())
	}
	if !strings.Contains(stderr, "packaged factory installation contention") {
		t.Fatalf("stderr = %q, want bounded contention diagnostic", stderr)
	}
}

func executeFactoryArgs(
	t *testing.T,
	process interface{ Execute(root.Input) error },
	workingDirectory string,
	args []string,
	stdoutIsTTY bool,
	ctx context.Context,
) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	stdinIsTTY := true
	home := t.TempDir()
	err := process.Execute(root.Input{
		Args:             args,
		Env:              append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          ctx,
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	return stdout.String(), stderr.String(), err
}

const idleCurrentFactoryJSON = `{
  "name": "current",
  "workTypes": [
    {
      "name": "task",
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "complete", "type": "TERMINAL"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "workers": [{"name": "processor"}],
  "workstations": [
    {
      "name": "process",
      "inputs": [{"workType": "task", "state": "init"}],
      "outputs": [{"workType": "task", "state": "complete"}],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "worker": "processor"
    }
  ]
}`
