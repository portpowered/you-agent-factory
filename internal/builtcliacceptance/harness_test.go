package builtcliacceptance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewHarnessSuppressesBrowserOpeningAndAllowsExplicitOverride(t *testing.T) {
	harness := NewHarness(t, t.TempDir())
	if harness.Edges.BrowserOpener == nil {
		t.Fatal("NewHarness BrowserOpener = nil, want functional-test no-op")
	}
	if err := harness.Edges.BrowserOpener(context.Background(), "http://127.0.0.1/dashboard/ui"); err != nil {
		t.Fatalf("default BrowserOpener() error = %v", err)
	}

	called := false
	harness.Edges.BrowserOpener = func(context.Context, string) error {
		called = true
		return nil
	}
	if err := harness.Edges.BrowserOpener(context.Background(), "http://127.0.0.1/dashboard/ui"); err != nil {
		t.Fatalf("override BrowserOpener() error = %v", err)
	}
	if !called {
		t.Fatal("explicit BrowserOpener override was not called")
	}
}

func TestProcessEnvForIsolatedHome_ReplacesHomeVariables(t *testing.T) {
	t.Setenv("HOME", "/real/home")
	t.Setenv("USERPROFILE", "/real/home")
	t.Setenv("HOMEDRIVE", "C:")
	t.Setenv("HOMEPATH", "\\Users\\real")
	t.Setenv("PATH", "/bin:/usr/bin")

	isolatedHome := filepath.Join(t.TempDir(), "isolated-home")
	env := ProcessEnvForIsolatedHome(isolatedHome)

	if got := envValue(env, "HOME"); got != isolatedHome {
		t.Fatalf("HOME = %q, want %q", got, isolatedHome)
	}
	if got := envValue(env, "USERPROFILE"); got != isolatedHome {
		t.Fatalf("USERPROFILE = %q, want %q", got, isolatedHome)
	}
	if got := envValue(env, "HOMEDRIVE"); got != filepath.VolumeName(isolatedHome) {
		t.Fatalf("HOMEDRIVE = %q, want %q", got, filepath.VolumeName(isolatedHome))
	}
	if got := envValue(env, "HOMEPATH"); got != string(os.PathSeparator) {
		t.Fatalf("HOMEPATH = %q, want %q", got, string(os.PathSeparator))
	}
	if got := envValue(env, "PATH"); got != "/bin:/usr/bin" {
		t.Fatalf("PATH = %q, want preserved PATH", got)
	}
	for _, entry := range env {
		if strings.HasPrefix(entry, "HOME=/real/home") || strings.HasPrefix(entry, "USERPROFILE=/real/home") {
			t.Fatalf("env still contains real home entry: %q", entry)
		}
	}
}

func TestScenarioFailure_ErrorIncludesDiagnostics(t *testing.T) {
	failure := &ScenarioFailure{
		Scenario:        "invalid-goal",
		Phase:           "run_process",
		Message:         "exit status 2",
		ExitCode:        2,
		StdoutTail:      "primary only",
		StderrTail:      "invalid goal syntax",
		HomeDir:         "/tmp/home",
		LogDir:          "/tmp/logs",
		ProcessBoundary: "root.BuildProcess",
	}

	got := failure.Error()
	for _, want := range []string{
		"scenario=invalid-goal",
		"run_process",
		"exit status 2",
		"exit_code=2",
		"stdout_tail=primary only",
		"stderr_tail=invalid goal syntax",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Error() = %q, want substring %q", got, want)
		}
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
