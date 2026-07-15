package acceptance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
)

func TestBuiltCLIHarness_IsolatesHomeAndLogDirectoriesAcrossSessions(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))

	first := harness.NewSession(t)
	second := harness.NewSession(t)

	if first.HomeDir == second.HomeDir || first.LogDir == second.LogDir {
		t.Fatalf("sessions share paths: first home=%q log=%q second home=%q log=%q",
			first.HomeDir, first.LogDir, second.HomeDir, second.LogDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	firstResult, err := first.Run(ctx, "config", "init")
	first.RequireSuccess(t, "first-config-init", firstResult, err)

	secondResult, err := second.Run(ctx, "config", "init")
	second.RequireSuccess(t, "second-config-init", secondResult, err)

	firstConfig := defaultpaths.OperatorConfigPath(first.HomeDir)
	secondConfig := defaultpaths.OperatorConfigPath(second.HomeDir)
	if firstConfig == secondConfig {
		t.Fatalf("operator config paths collided: %q", firstConfig)
	}
	for _, path := range []string{firstConfig, secondConfig} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("operator config %q: %v", path, err)
		}
	}

	if strings.Contains(firstResult.Stdout+firstResult.Stderr, second.HomeDir) {
		t.Fatalf("first session output leaked second home %q:\nstdout=%q\nstderr=%q",
			second.HomeDir, firstResult.Stdout, firstResult.Stderr)
	}
	if _, err := os.Stat(first.LogDir); err != nil {
		t.Fatalf("first log dir %q: %v", first.LogDir, err)
	}
	if _, err := os.Stat(second.LogDir); err != nil {
		t.Fatalf("second log dir %q: %v", second.LogDir, err)
	}
	if info, err := os.Stat(filepath.Join(first.HomeDir, ".you-agent-factory")); err != nil {
		t.Fatalf("first home state dir: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("first home state path = %q, want directory", filepath.Join(first.HomeDir, ".you-agent-factory"))
	}
}

func TestBuiltCLIHarness_NonZeroExitIncludesDiagnostics(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := session.Run(ctx, "definitely-not-a-real-subcommand")
	if err == nil {
		t.Fatalf("expected failure, got result=%#v", result)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for unknown subcommand")
	}

	var failure *builtcliacceptance.ScenarioFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error type = %T, want *builtcliacceptance.ScenarioFailure: %v", err, err)
	}
	if failure.ExitCode == 0 {
		t.Fatalf("failure exit code = 0, want non-zero")
	}
	if strings.TrimSpace(failure.StderrTail) == "" && strings.TrimSpace(failure.StdoutTail) == "" {
		t.Fatalf("failure diagnostics missing stdout/stderr tails: %#v", failure)
	}
	if failure.HomeDir != session.HomeDir || failure.LogDir != session.LogDir {
		t.Fatalf("failure paths = home %q log %q, want home %q log %q",
			failure.HomeDir, failure.LogDir, session.HomeDir, session.LogDir)
	}
	if failure.BinaryPath != harness.BinaryPath {
		t.Fatalf("failure binary = %q, want %q", failure.BinaryPath, harness.BinaryPath)
	}
}

func TestBuiltCLIHarness_WithNoExternalServerReservesUnusedPort(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	flags := session.ServerFlags()
	if len(flags) != 2 || flags[0] != "--server" || flags[1] != session.ServerURL {
		t.Fatalf("server flags = %#v, want --server %q", flags, session.ServerURL)
	}
	if !strings.HasPrefix(session.ServerURL, "http://127.0.0.1:") {
		t.Fatalf("server URL = %q, want local loopback URL", session.ServerURL)
	}
}
