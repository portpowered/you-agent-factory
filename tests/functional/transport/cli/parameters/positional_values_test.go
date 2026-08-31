package parameters_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRunAcceptsOnePositionalPrompt proves a single positional prompt that
// contains spaces and Unicode characters survives through the public CLI
// observation edge with the exact customer-supplied string intact.
func TestRunAcceptsOnePositionalPrompt(t *testing.T) {
	prompt := "Ship the café résumé plan"

	factoryDir := support.ScaffoldSingleStepFactory(t, "positional-values")
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	observation := executeParameterObservation(t, []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		prompt,
	})
	if observation.Parse.CommandPath != "you run" {
		t.Fatalf("observed command path = %q, want you run", observation.Parse.CommandPath)
	}
	if len(observation.Parse.Positionals) != 1 {
		t.Fatalf("observed positional count = %d, want 1: %#v", len(observation.Parse.Positionals), observation.Parse.Positionals)
	}
	if got := observation.Parse.Positionals[0]; got != prompt {
		t.Fatalf("observed positional prompt = %q, want %q", got, prompt)
	}
}

// TestRunRejectsExtraPositionalValues proves surplus positional prompt values
// on you run --factory are rejected with a stable diagnostic before any worker
// provider dispatch can start.
func TestRunRejectsExtraPositionalValues(t *testing.T) {
	factoryDir := scaffoldSinglePositionalInvocationFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	beforeProviderCalls := parameterProcesses.providerRunner.CallCount()
	inputs := parameterInputs(t, []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"first prompt",
		"second prompt",
	})

	executeErr := parameterProcesses.handlerRuntime.execute(inputs.Input)
	if executeErr == nil {
		t.Fatalf(
			"Process.Execute(extra positional prompts) succeeded; stdout:\n%s\nstderr:\n%s",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	diagnostic := executeErr.Error() + "\n" + inputs.Stderr()
	for _, want := range []string{
		"INVOCATION_ARGUMENT_POSITIONAL_OVERFLOW",
		"received 2 positional arguments but the active invocationSignature only accepts 1",
	} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf(
				"extra positional diagnostic missing %q:\n%s",
				want,
				diagnostic,
			)
		}
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); err != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\nstderr:\n%s", err, inputs.Stderr())
	}
	if response.Code != factoryapi.ErrorResponseCode("INVOCATION_ARGUMENT_POSITIONAL_OVERFLOW") ||
		response.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("ErrorResponse = %#v, want positional-overflow code and BAD_REQUEST", response)
	}
	if got := parameterProcesses.providerRunner.CallCount() - beforeProviderCalls; got != 0 {
		t.Fatalf("provider dispatch call delta = %d, want 0", got)
	}
}

// TestOptionalSessionIDUsesDefaultWhenOmitted proves optional session identity on
// you session pause targets the documented default session when the session
// positional is omitted and routes to an explicit override positional when one
// is supplied.
func TestOptionalSessionIDUsesDefaultWhenOmitted(t *testing.T) {
	t.Run("omitted session positional targets default session", func(t *testing.T) {
		var gotPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			_, _ = io.Copy(io.Discard, r.Body)
			writeSessionPauseResponse(t, w, factorysessions.DefaultSessionID)
		}))
		t.Cleanup(server.Close)

		inputs := parameterInputs(t, []string{
			"you", "--remote", "--server", server.URL,
			"session", "pause",
		})

		if err := parameterProcesses.handlerRuntime.execute(inputs.Input); err != nil {
			t.Fatalf(
				"Process.Execute(session pause default targeting) error = %v\nstdout:\n%s\nstderr:\n%s",
				err,
				inputs.Stdout(),
				inputs.Stderr(),
			)
		}
		wantPath := "/factory-sessions/" + factorysessions.DefaultSessionID + "/pause"
		if gotPath != wantPath {
			t.Fatalf("observed request path = %q, want %q", gotPath, wantPath)
		}
	})

	t.Run("explicit session positional overrides default targeting", func(t *testing.T) {
		overrideSessionID := "session-customer-override"
		var gotPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			_, _ = io.Copy(io.Discard, r.Body)
			writeSessionPauseResponse(t, w, overrideSessionID)
		}))
		t.Cleanup(server.Close)

		inputs := parameterInputs(t, []string{
			"you", "--remote", "--server", server.URL,
			"session", "pause", overrideSessionID,
		})

		if err := parameterProcesses.handlerRuntime.execute(inputs.Input); err != nil {
			t.Fatalf(
				"Process.Execute(session pause override targeting) error = %v\nstdout:\n%s\nstderr:\n%s",
				err,
				inputs.Stdout(),
				inputs.Stderr(),
			)
		}
		wantPath := "/factory-sessions/" + overrideSessionID + "/pause"
		if gotPath != wantPath {
			t.Fatalf("observed request path = %q, want %q", gotPath, wantPath)
		}
	})
}

func writeSessionPauseResponse(t *testing.T, w http.ResponseWriter, sessionID string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := io.WriteString(w, `{
		"sessionId":"`+sessionID+`",
		"operation":"PAUSE",
		"outcome":"ACCEPTED",
		"status":"PAUSED"
	}`)
	if err != nil {
		t.Fatalf("write session pause response: %v", err)
	}
}

func scaffoldSinglePositionalInvocationFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": "positional-values",
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":     "input",
					"required": true,
					"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
				},
			},
		},
		"workTypes": []any{map[string]any{
			"name":             "task",
			"handlingBehavior": []any{"DEFAULT"},
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "processor",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		}},
	})
}
