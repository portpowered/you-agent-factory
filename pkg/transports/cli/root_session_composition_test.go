package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
)

func TestSessionCommandCompositionUsesTypedSessionsCLIAdapter(t *testing.T) {
	t.Parallel()

	called := false
	factory := NewCommandFactory(CommandOperations{
		ShowSession: func(cfg session.ShowConfig) error {
			called = true
			if cfg.SessionID != "session-beta" {
				t.Fatalf("SessionID = %q, want session-beta", cfg.SessionID)
			}
			return nil
		},
	})
	if factory.SessionsCLI == nil {
		t.Fatal("SessionsCLI adapter is missing from composed factory")
	}

	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"session", "show", "session-beta"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute session show: %v", err)
	}
	if !called {
		t.Fatal("typed Sessions adapter was not invoked through production composition")
	}
}

func TestSessionCreatePreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "create", "--dir", "/workspace/fleet",
		"--validate-only", "--target-kind", "named", "--target-name", "alpha",
	}
	runSessionCompositionCases(t, args, errors.New("session operation failed"), func(result error) CommandOperations {
		return CommandOperations{CreateSession: func(cfg session.CreateConfig) error {
			if cfg.Server != "https://factory.example" || cfg.Dir != "/workspace/fleet" ||
				!cfg.ValidateOnly || cfg.TargetKind != "named" || cfg.TargetName != "alpha" ||
				!cfg.JSON || !cfg.Verbose {
				t.Fatalf("create config = %#v", cfg)
			}
			return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	})
}

func TestSessionDeletePreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{"--verbose", "--json", "session", "delete", "session-beta", "--port", "9444"}
	runSessionCompositionCases(t, args, errors.New("session operation failed"), func(result error) CommandOperations {
		return CommandOperations{DeleteSession: func(cfg session.DeleteConfig) error {
			if cfg.SessionID != "session-beta" || cfg.Port != 9444 || !cfg.JSON || !cfg.Verbose {
				t.Fatalf("delete config = %#v", cfg)
			}
			return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	})
}

func TestSessionListPreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "list", "--scope", "live",
	}
	runSessionCompositionCases(t, args, errors.New("session operation failed"), func(result error) CommandOperations {
		return CommandOperations{ListSessions: func(cfg session.ListConfig) error {
			if cfg.Context == nil || cfg.Server != "https://factory.example" ||
				cfg.Scope != "live" || !cfg.JSON || !cfg.Verbose {
				t.Fatalf("list config = %#v", cfg)
			}
			return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	})
}

func TestSessionShowPreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "show", "session-beta",
	}
	runSessionCompositionCases(t, args, context.Canceled, func(result error) CommandOperations {
		return CommandOperations{ShowSession: func(cfg session.ShowConfig) error {
			if cfg.Context == nil || cfg.Server != "https://factory.example" ||
				cfg.SessionID != "session-beta" || !cfg.JSON || !cfg.Verbose {
				t.Fatalf("show config = %#v", cfg)
			}
			return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	})
}

func TestSessionDispatchesPreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "dispatches", "dur-sess-review-001",
		"--phase", "review", "--status", "COMPLETED",
	}
	runSessionCompositionCases(t, args, errors.New("session operation failed"), func(result error) CommandOperations {
		return CommandOperations{ListSessionDispatches: func(cfg session.DispatchesConfig) error {
			if cfg.Context == nil || cfg.Server != "https://factory.example" ||
				cfg.SessionID != "dur-sess-review-001" || cfg.Phase != "review" ||
				cfg.Status != "COMPLETED" || !cfg.JSON || !cfg.Verbose {
				t.Fatalf("dispatches config = %#v", cfg)
			}
			return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	})
}

func TestSessionPausePreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "pause",
	}
	runSessionCompositionCases(t, args, errors.New("session lifecycle operation failed"), func(result error) CommandOperations {
		return CommandOperations{PauseSession: func(cfg session.LifecycleControlConfig) error {
			if cfg.Context == nil || cfg.Server != "https://factory.example" ||
				cfg.SessionID != "" || !cfg.JSON || !cfg.Verbose {
				t.Fatalf("pause config = %#v", cfg)
			}
			return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	})
}

func TestSessionResumePreservesBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	args := []string{
		"--verbose", "--json", "--server", "https://factory.example",
		"session", "resume", "session-beta",
	}
	runSessionCompositionCases(t, args, context.Canceled, func(result error) CommandOperations {
		return CommandOperations{ResumeSession: func(cfg session.LifecycleControlConfig) error {
			if cfg.Context == nil || cfg.Server != "https://factory.example" ||
				cfg.SessionID != "session-beta" || !cfg.JSON || !cfg.Verbose {
				t.Fatalf("resume config = %#v", cfg)
			}
			return writeSessionCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	})
}

func runSessionCompositionCases(
	t *testing.T,
	args []string,
	wantError error,
	operations func(error) CommandOperations,
) {
	t.Helper()
	t.Run("success", func(t *testing.T) {
		stdout, stderr, err := executeSessionComposition(t, operations(nil), args)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if stdout != "session-ok\n" || stderr != "session-diagnostic\n" {
			t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
		}
	})
	t.Run("failure", func(t *testing.T) {
		stdout, stderr, err := executeSessionComposition(t, operations(wantError), args)
		if !errors.Is(err, wantError) {
			t.Fatalf("Execute() error = %v, want %v", err, wantError)
		}
		if stdout != "" || stderr != fmt.Sprintf("Error: %v\n", wantError) {
			t.Fatalf("failure stdout = %q, stderr = %q", stdout, stderr)
		}
	})
}

func executeSessionComposition(
	t *testing.T,
	operations CommandOperations,
	args []string,
) (string, string, error) {
	t.Helper()
	factory := NewCommandFactory(operations)
	if factory.SessionsCLI == nil {
		t.Fatal("SessionsCLI adapter is missing from production composition")
	}
	root := factory.NewCommand(nil, nil, nil)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func writeSessionCompositionOutput(output, diagnostics io.Writer, result error) error {
	if result != nil {
		return result
	}
	if _, err := fmt.Fprintln(output, "session-ok"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(diagnostics, "session-diagnostic")
	return err
}
