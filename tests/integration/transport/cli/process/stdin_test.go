package process_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestBuiltCLIRunDirectorySelectionIgnoresOpenStdin proves the descriptor
// lifetime behavior of a separately built CLI process. Both live and replay
// directory-selected runs must finish without an EOF from an unrelated pipe,
// with byte-for-byte equivalent terminal streams to a closed-stdin control.
func TestBuiltCLIRunDirectorySelectionIgnoresOpenStdin(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)
	factoryPath := writeStdinRunFactory(t, session.WorkDir)
	factoryDir := filepath.Dir(factoryPath)

	binaryPath := quietShutdownArtifact(t)

	baseArgs := append([]string{}, session.RuntimeLogDirFlags()...)
	baseArgs = append(baseArgs, session.ServerFlags()...)
	recordingPath := filepath.Join(session.WorkDir, "directory-run.replay.json")
	recordArgs := append(append([]string{}, baseArgs...),
		"run", "--dir", factoryDir, "--record", recordingPath, "--quiet",
	)
	if result, err := runBuiltCLIWithClosedStdin(t, binaryPath, session, recordArgs...); err != nil || result.ExitCode != 0 {
		t.Fatalf("create directory-run replay recording: result=%#v err=%v", result, err)
	}
	if _, err := os.Stat(recordingPath); err != nil {
		t.Fatalf("directory-run replay recording %q: %v", recordingPath, err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{name: "live", args: append(append([]string{}, baseArgs...), "run", "--dir", factoryDir, "--no-record", "--quiet")},
		{name: "replay", args: append(append([]string{}, baseArgs...), "run", "--dir", factoryDir, "--replay", recordingPath, "--no-record", "--quiet")},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			closed, closedErr := runBuiltCLIWithClosedStdin(t, binaryPath, session, test.args...)
			if closedErr != nil || closed.ExitCode != 0 {
				t.Fatalf("closed-stdin %s run: result=%#v err=%v", test.name, closed, closedErr)
			}

			open, openErr := runBuiltCLIWithOpenStdin(t, binaryPath, session, test.args...)
			if openErr != nil || open.ExitCode != 0 {
				t.Fatalf("open-stdin %s run: result=%#v err=%v", test.name, open, openErr)
			}
			assertQuietShutdownStderr(t, closed, open)
			if open != closed {
				t.Fatalf("open-stdin %s result=%#v differs from closed-stdin control=%#v", test.name, open, closed)
			}
		})
	}

	closedHelp, closedHelpErr := runBuiltCLIWithClosedStdin(t, binaryPath, session, "--help")
	if closedHelpErr != nil || closedHelp.ExitCode != 0 {
		t.Fatalf("closed-stdin help: result=%#v err=%v", closedHelp, closedHelpErr)
	}
	openHelp, openHelpErr := runBuiltCLIWithOpenStdin(t, binaryPath, session, "--help")
	if openHelpErr != nil || openHelp.ExitCode != 0 {
		t.Fatalf("open-stdin help: result=%#v err=%v", openHelp, openHelpErr)
	}
	if openHelp != closedHelp {
		t.Fatalf("open-stdin help result=%#v differs from closed-stdin control=%#v", openHelp, closedHelp)
	}
	if !strings.Contains(openHelp.Stdout, "Available Commands:") {
		t.Fatalf("open-stdin help omitted full command listing:\n%s", openHelp.Stdout)
	}
}

func runBuiltCLIWithClosedStdin(
	t testing.TB,
	binaryPath string,
	session *builtcliacceptance.Session,
	args ...string,
) (builtcliacceptance.RunResult, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create closed-stdin pipe: %v", err)
	}
	if err := writer.Close(); err != nil {
		_ = reader.Close()
		t.Fatalf("close closed-stdin pipe writer: %v", err)
	}
	return runBuiltCLIWithStdin(t, binaryPath, session, reader, args...)
}

func runBuiltCLIWithOpenStdin(
	t testing.TB,
	binaryPath string,
	session *builtcliacceptance.Session,
	args ...string,
) (builtcliacceptance.RunResult, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create open-stdin pipe: %v", err)
	}
	defer writer.Close()
	return runBuiltCLIWithStdin(t, binaryPath, session, reader, args...)
}

func runBuiltCLIWithStdin(
	t testing.TB,
	binaryPath string,
	session *builtcliacceptance.Session,
	stdin *os.File,
	args ...string,
) (builtcliacceptance.RunResult, error) {
	t.Helper()
	defer stdin.Close()

	command := exec.Command(binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()
	command.Stdin = stdin
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start built CLI with controlled stdin: %v", err)
	}

	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()
	var runErr error
	select {
	case runErr = <-waitResult:
	case <-time.After(120 * time.Second):
		// This is only a bounded failure guard for the OS-process test. The
		// assertion is completion while the writer remains open, not a latency
		// threshold.
		_ = command.Process.Kill()
		runErr = <-waitResult
		t.Fatalf("built CLI did not finish with controlled stdin: %v", runErr)
	}

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			t.Fatalf("built CLI wait: %v", runErr)
		}
		exitCode = exitErr.ExitCode()
	}
	return builtcliacceptance.RunResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, runErr
}

func assertQuietShutdownStderr(t *testing.T, results ...builtcliacceptance.RunResult) {
	t.Helper()
	for _, result := range results {
		if result.Stderr != "" {
			t.Fatalf("successful quiet shutdown emitted stderr: %q", result.Stderr)
		}
	}
}

// CI supplies the already built deliverable; this regression never compiles it.
func quietShutdownArtifact(t *testing.T) string {
	t.Helper()
	path := os.Getenv("INFINITE_YOU_INTEGRATION_BINARY")
	if path == "" {
		path = os.Getenv("INFINITE_YOU_PREBUILT_ARTIFACT")
	}
	if path == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("quiet shutdown requires the prebuilt CLI artifact")
		}
		t.Skip("set INFINITE_YOU_INTEGRATION_BINARY to run quiet shutdown proof")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() || info.Size() == 0 {
		t.Fatal("prebuilt CLI artifact is empty or a directory")
	}
	t.Logf("prebuilt quiet shutdown artifact=%s", abs)
	return abs
}
