package model_list_test

import (
	"encoding/json"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestProcessModelsList_RoutesThroughPresentationCollaboratorWithoutServer proves
// local models list reaches the owned adapter through Factory Sessions
// ModelsCLIPresentationCollaborator rather than the remote HTTP bootstrap path.
func TestProcessModelsList_RoutesThroughPresentationCollaboratorWithoutServer(t *testing.T) {
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
// models list commands reuse the process-scoped catalog scope opened by the
// presentation collaborator.
func TestProcessModelsList_ReusesCatalogScopeAcrossCommands(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	for i := 0; i < 2; i++ {
		inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "models", "list"})
		if err := process.Execute(inputs.Input); err != nil {
			t.Fatalf("Process.Execute(local models list #%d) error = %v\nstderr=%s", i+1, err, inputs.Stderr())
		}
		var response factoryapi.ListModelsResponse
		if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
			t.Fatalf("decode local models list output #%d: %v\n%s", i+1, err, inputs.Stdout())
		}
		if response.Results == nil {
			t.Fatalf("list response #%d = %#v, want results array", i+1, response)
		}
	}
}

// local models inspect uses the presentation collaborator-owned adapter path.
func TestProcessModelsInspect_RoutesThroughPresentationCollaboratorWithoutServer(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "inspect", "OMNIVOICE_Q4_K_M",
	})
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatalf("Process.Execute(local models inspect) error = nil, want not-found failure without catalog fixture")
	}
	if !strings.Contains(inputs.Stderr(), "not found") && !strings.Contains(inputs.Stderr(), "NotFound") {
		t.Fatalf("stderr = %q, want not-found diagnostics from owned Models root path", inputs.Stderr())
	}
}
