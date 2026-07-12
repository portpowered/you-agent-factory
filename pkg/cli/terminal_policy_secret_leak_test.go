package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
)

const (
	terminalPolicySecretPrompt = "SECRET_PROMPT_BODY_do-not-emit-712407"
	terminalPolicySecretToken  = "ghp_secretToken712407932abcdef"
)

func TestRootCommand_TerminalPolicyNeverLeaksPromptOrSecretsAcrossModes(t *testing.T) {
	t.Run("quiet operational failure", func(t *testing.T) {
		assertTerminalPolicySecretLeakContract(t, []string{
			"run",
			"--factory", writeInvalidGoalFactory(t),
			"--no-record",
			"--quiet",
			terminalPolicySecretPrompt,
		})
	})

	t.Run("normal operational failure", func(t *testing.T) {
		assertTerminalPolicySecretLeakContract(t, []string{
			"run",
			"--factory", writeInvalidGoalFactory(t),
			"--no-record",
			terminalPolicySecretPrompt,
		})
	})

	t.Run("verbose operational failure", func(t *testing.T) {
		assertTerminalPolicySecretLeakContract(t, []string{
			"run",
			"--factory", writeInvalidGoalFactory(t),
			"--no-record",
			"--verbose",
			terminalPolicySecretPrompt,
		})
	})
}

func TestRootCommand_SubmitDiagnosticsNeverLeakPromptOrSecretsAcrossModes(t *testing.T) {
	modes := []struct {
		name string
		args []string
	}{
		{name: "normal", args: nil},
		{name: "verbose", args: []string{"--verbose"}},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"traceId":"trace-terminal-policy"}`))
			}))
			defer srv.Close()

			payloadPath := filepath.Join(t.TempDir(), "secret-payload.md")
			if err := os.WriteFile(payloadPath, []byte("# "+terminalPolicySecretPrompt+"\n\n"+terminalPolicySecretToken), 0o644); err != nil {
				t.Fatal(err)
			}

			originalSubmit := submitWork
			defer func() {
				submitWork = originalSubmit
			}()

			submitWork = func(submitcli.SubmitConfig) error {
				return nil
			}

			var stdout, stderr bytes.Buffer
			root := NewRootCommand()
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			args := append([]string{}, mode.args...)
			args = append(args,
				"submit",
				"--name", "terminal-policy-secret-test",
				"--work-type-name", "task",
				"--payload", payloadPath,
				"--server", srv.URL,
			)
			root.SetArgs(args)

			if err := root.Execute(); err != nil {
				t.Fatalf("execute submit: %v", err)
			}

			assertNoTerminalPolicySecrets(t, stdout.String()+stderr.String())
		})
	}
}

func writeInvalidGoalFactory(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(goalFailureBaselineInvalidTopologyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return factoryPath
}

func assertTerminalPolicySecretLeakContract(t *testing.T, args []string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected operational failure")
	}
	if !strings.Contains(err.Error(), "invalid graph references") {
		t.Fatalf("error = %q, want invalid topology failure", err.Error())
	}

	assertNoTerminalPolicySecrets(t, stdout.String()+stderr.String())
}

func assertNoTerminalPolicySecrets(t *testing.T, capture string) {
	t.Helper()

	for _, forbidden := range []string{
		terminalPolicySecretPrompt,
		terminalPolicySecretToken,
	} {
		if strings.Contains(capture, forbidden) {
			t.Fatalf("terminal or diagnostics capture leaked %q:\n%s", forbidden, capture)
		}
	}
}
