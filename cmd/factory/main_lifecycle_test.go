package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
)

// entrypointOutcome is what a customer observes from one `you` invocation.
type entrypointOutcome struct {
	exitCode int
	stdout   string
	stderr   string
}

// runEntrypoint drives the real cmd/factory entrypoint closure end to end.
//
// runProcess reads os.Args and the real os.Stdout/os.Stderr handles, so the
// only way to observe the customer-visible result of the full
// root.BuildProcess -> Process.Execute -> Process.Close lifecycle is to replace
// those process globals for the duration of the call. The helper deliberately
// does not stub root.BuildProcess: the canonical production composition with an
// empty edges.Edges is the subject under test.
func runEntrypoint(t *testing.T, args ...string) entrypointOutcome {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	originalArgs, originalStdout, originalStderr := os.Args, os.Stdout, os.Stderr
	stdout, collectStdout := captureEntrypointStream(t)
	stderr, collectStderr := captureEntrypointStream(t)
	restore := func() {
		os.Args, os.Stdout, os.Stderr = originalArgs, originalStdout, originalStderr
	}
	t.Cleanup(restore)

	os.Args = args
	os.Stdout, os.Stderr = stdout, stderr
	exitCode := runProcess()
	restore()

	return entrypointOutcome{
		exitCode: exitCode,
		stdout:   collectStdout(),
		stderr:   collectStderr(),
	}
}

// captureEntrypointStream returns a pipe writer to install as a process stream
// and a collector that closes the writer and returns everything written to it.
// The reader is drained concurrently so a command that outsizes the pipe buffer
// cannot deadlock the entrypoint under test.
func captureEntrypointStream(t *testing.T) (*os.File, func() string) {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("open entrypoint stream pipe: %v", err)
	}
	var collected bytes.Buffer
	var drained sync.WaitGroup
	drained.Go(func() {
		_, _ = io.Copy(&collected, reader)
	})
	return writer, func() string {
		_ = writer.Close()
		drained.Wait()
		_ = reader.Close()
		return collected.String()
	}
}

// TestRunProcessCompletesCanonicalLifecycleAndMapsExitCode is the caller-level
// guard for cmd/factory/main.go:runProcess. Every other entrypoint test either
// replaces runProcess outright or calls processExitCode as a pure function, so
// the composed lifecycle -- build the canonical process, execute the selected
// command against the real process streams, close the process, join the close
// result into the command result, and map that to an exit code -- had no direct
// assertion before this test.
//
// The successful cases carry the close-error join: runProcess computes
// errors.Join(executeErr, process.Close(closeCtx)) before mapping to an exit
// code, so a lifecycle owner that failed to close would surface here as exit 1
// even though the command itself succeeded.
func TestRunProcessCompletesCanonicalLifecycleAndMapsExitCode(t *testing.T) {
	t.Run("help succeeds and closes cleanly", func(t *testing.T) {
		outcome := runEntrypoint(t, "you", "--help")

		if outcome.exitCode != exitSuccess {
			t.Fatalf(
				"runProcess(you --help) exit = %d, want %d; a non-nil Close joined into the command result would land here\nstdout:\n%s\nstderr:\n%s",
				outcome.exitCode,
				exitSuccess,
				outcome.stdout,
				outcome.stderr,
			)
		}
		if !strings.Contains(outcome.stdout, "Run and manage CPN-based workflow factories") {
			t.Fatalf("runProcess(you --help) stdout = %q, want the customer root help banner", outcome.stdout)
		}
	})

	t.Run("command output reaches the process stdout", func(t *testing.T) {
		outcome := runEntrypoint(t, "you", "docs", "agents")

		if outcome.exitCode != exitSuccess {
			t.Fatalf(
				"runProcess(you docs agents) exit = %d, want %d\nstdout:\n%s\nstderr:\n%s",
				outcome.exitCode,
				exitSuccess,
				outcome.stdout,
				outcome.stderr,
			)
		}
		if !strings.Contains(outcome.stdout, "# Agents") {
			t.Fatalf("runProcess(you docs agents) stdout = %q, want the packaged agents topic", outcome.stdout)
		}
	})

	t.Run("unknown command fails without inventing success", func(t *testing.T) {
		outcome := runEntrypoint(t, "you", "definitely-not-a-command")

		if outcome.exitCode != exitFailure {
			t.Fatalf(
				"runProcess(you definitely-not-a-command) exit = %d, want %d\nstdout:\n%s\nstderr:\n%s",
				outcome.exitCode,
				exitFailure,
				outcome.stdout,
				outcome.stderr,
			)
		}
		if strings.Contains(outcome.stdout, "Run and manage CPN-based workflow factories") {
			t.Fatalf("runProcess(unknown command) printed the success help banner to stdout: %q", outcome.stdout)
		}
	})
}

// TestRunProcessReusesNoStateAcrossSequentialInvocations proves the entrypoint
// builds and closes one whole process per invocation: a second run after a
// failed run still reaches a clean success, so nothing the failed lifecycle
// acquired survives into the next process.
func TestRunProcessReusesNoStateAcrossSequentialInvocations(t *testing.T) {
	failed := runEntrypoint(t, "you", "definitely-not-a-command")
	if failed.exitCode != exitFailure {
		t.Fatalf("first runProcess exit = %d, want %d", failed.exitCode, exitFailure)
	}

	succeeded := runEntrypoint(t, "you", "--help")
	if succeeded.exitCode != exitSuccess {
		t.Fatalf(
			"second runProcess exit = %d, want %d after a failed lifecycle\nstderr:\n%s",
			succeeded.exitCode,
			exitSuccess,
			succeeded.stderr,
		)
	}
	if strings.Contains(succeeded.stdout, "definitely-not-a-command") {
		t.Fatalf("second runProcess stdout leaked the first invocation: %q", succeeded.stdout)
	}
}
