package model_list_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestProcessModelsList_RoutesThroughCompositionProviderWithoutServer proves
// local models list reaches the owned adapter through the explicit process
// composition port rather than the remote HTTP bootstrap path.
func TestProcessModelsList_RoutesThroughCompositionProviderWithoutServer(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "models", "list"})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(local models list) error = %v\nstderr=%s", err, inputs.Stderr())
	}

	var response factoryapi.ListModelsResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode local models list output: %v\n%s", err, inputs.Stdout())
	}
	if response.Results == nil {
		t.Fatalf("list response = %#v, want results array", response)
	}
}

// TestProcessModelsList_ReusesCatalogScopeAcrossCommands proves repeated local
// models list/inspect/pull commands reuse the process-scoped catalog scope
// opened by the process composition port.
func TestProcessModelsList_ReusesCatalogScopeAcrossCommands(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	commands := [][]string{
		{"you", "--json", "models", "list"},
		{"you", "--json", "models", "inspect", "OMNIVOICE_Q4_K_M"},
		{"you", "--json", "models", "pull", "OMNIVOICE_Q4_K_M"},
		{"you", "--json", "models", "list"},
	}
	for i, args := range commands {
		inputs := support.FakeInputs(t.Context(), args)
		err := process.Execute(inputs.Input)
		switch args[3] {
		case "list":
			if err != nil {
				t.Fatalf("Process.Execute(local models list #%d) error = %v\nstderr=%s", i+1, err, inputs.Stderr())
			}
			var response factoryapi.ListModelsResponse
			if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
				t.Fatalf("decode local models list output #%d: %v\n%s", i+1, err, inputs.Stdout())
			}
			if response.Results == nil {
				t.Fatalf("list response #%d = %#v, want results array", i+1, response)
			}
		case "inspect", "pull":
			if err == nil {
				t.Fatalf("Process.Execute(local models %s #%d) error = nil, want failure without catalog fixture", args[3], i+1)
			}
			stderr := inputs.Stderr()
			support.RequireNotFoundCLIDiagnostic(t, stderr)
		default:
			t.Fatalf("unexpected command args: %v", args)
		}
	}
}

// TestProcessModelsPull_RoutesThroughCompositionProviderWithoutServer proves
// local models pull reaches the owned adapter through the explicit process
// composition port rather than the remote HTTP bootstrap path.
func TestProcessModelsPull_RoutesThroughCompositionProviderWithoutServer(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "pull", "OMNIVOICE_Q4_K_M",
	})
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatalf("Process.Execute(local models pull) error = nil, want failure without catalog fixture")
	}
	support.RequireNotFoundCLIDiagnostic(t, inputs.Stderr())
}

// TestProcessModelsInvokeJSON_RoutesThroughCompositionProviderWithoutServer proves
// local models invoke --json reaches the owned adapter through the explicit
// process composition port rather than the remote HTTP bootstrap path.
func TestProcessModelsInvokeJSON_RoutesThroughCompositionProviderWithoutServer(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "hello from owned invoke",
	})
	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatalf("Process.Execute(local models invoke --json) error = nil, want failure without catalog fixture")
	}
	stderr := inputs.Stderr()
	if strings.Contains(stderr, "localhost:7437") || strings.Contains(stderr, "connection refused") {
		t.Fatalf("stderr = %q, want owned Models-root path rather than remote HTTP bootstrap", stderr)
	}
}

func TestProcessModelsInvokeMissingFactoryLayoutReportsSearchedRoot(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	workingDirectory, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatalf("get working directory: %v", getwdErr)
	}
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "missing layout",
	})
	inputs.Input.WorkingDirectory = workingDirectory
	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute(models invoke) error = nil, want missing-layout failure")
	}

	var response factoryapi.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(inputs.Stderr()), &response); decodeErr != nil {
		t.Fatalf("decode missing-layout diagnostic: %v\nstderr=%s", decodeErr, inputs.Stderr())
	}
	wantRoot := filepath.Join(workingDirectory, "factory")
	if response.Code != factoryapi.ErrorResponseCode("CURRENT_FACTORY_NOT_FOUND") ||
		response.Family != factoryapi.ErrorFamilyNotFound ||
		!strings.Contains(response.Message, wantRoot) {
		t.Fatalf("missing-layout response = %#v, want CURRENT_FACTORY_NOT_FOUND/NOT_FOUND with searched root %q", response, wantRoot)
	}
}

// local models inspect uses the process-composed owned adapter path.
func TestProcessModelsInspect_RoutesThroughCompositionProviderWithoutServer(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "inspect", "OMNIVOICE_Q4_K_M",
	})
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatalf("Process.Execute(local models inspect) error = nil, want not-found failure without catalog fixture")
	}
	support.RequireNotFoundCLIDiagnostic(t, inputs.Stderr())
}
