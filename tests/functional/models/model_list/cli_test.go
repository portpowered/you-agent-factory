package model_list_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestProcessModelsList_UsesServerFlagAndReturnsCatalogJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"results":[{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[]}]}`)
	}))
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", strings.TrimSuffix(server.URL, "/"), "models", "list",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models list) error = %v\nstderr=%s", err, inputs.Stderr())
	}

	var response factoryapi.ListModelsResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode models list output: %v\n%s", err, inputs.Stdout())
	}
	if len(response.Results) != 1 || response.Results[0].Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("list response = %#v, want OMNIVOICE_Q4_K_M", response)
	}
}

func TestProcessModelsInspect_UsesResolvedModelNameArgument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M" {
			t.Fatalf("path = %q, want /models/OMNIVOICE_Q4_K_M", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"capabilities":[],"diagnostics":{}}`)
	}))
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", strings.TrimSuffix(server.URL, "/"),
		"models", "inspect", "OMNIVOICE_Q4_K_M",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models inspect) error = %v\nstderr=%s", err, inputs.Stderr())
	}

	var response factoryapi.ModelDetail
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode models inspect output: %v\n%s", err, inputs.Stdout())
	}
	if response.Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("inspect response = %#v, want OMNIVOICE_Q4_K_M", response)
	}
}
