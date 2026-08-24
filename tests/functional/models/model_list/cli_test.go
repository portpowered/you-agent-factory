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

// TestProcessModelsList_UsesServerFlagAndReturnsCatalogJSON proves list routing and JSON catalog output.
func TestProcessModelsList_UsesServerFlagAndReturnsCatalogJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"results":[{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"managedRuntime":{"identity":"OMNIVOICE_Q4_K_M","revision":"rev1","cacheBytes":1234,"readinessState":"READY","lifecycleState":"INSTALLED","locality":"LOCAL","supportedOperations":[]}}]}`)
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
	if response.Results[0].ManagedRuntime.Revision == nil ||
		*response.Results[0].ManagedRuntime.Revision != "rev1" ||
		response.Results[0].ManagedRuntime.CacheBytes == nil ||
		*response.Results[0].ManagedRuntime.CacheBytes != 1234 {
		t.Fatalf("managed runtime cache facts = revision=%v bytes=%v, want rev1/1234", response.Results[0].ManagedRuntime.Revision, response.Results[0].ManagedRuntime.CacheBytes)
	}
}

// TestProcessModelsInspect_UsesResolvedModelNameArgument proves inspect forwards the resolved model identity.
func TestProcessModelsInspect_UsesResolvedModelNameArgument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M" {
			t.Fatalf("path = %q, want /models/OMNIVOICE_Q4_K_M", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"managedRuntime":{"identity":"OMNIVOICE_Q4_K_M","cachePath":"/tmp/models/OMNIVOICE_Q4_K_M/rev1","revision":"rev1","cacheBytes":1234,"readinessState":"READY","lifecycleState":"INSTALLED","locality":"LOCAL","supportedOperations":[]},"capabilities":[],"diagnostics":{}}`)
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
	if response.ManagedRuntime.CachePath == nil ||
		*response.ManagedRuntime.CachePath != "/tmp/models/OMNIVOICE_Q4_K_M/rev1" ||
		response.ManagedRuntime.Revision == nil || *response.ManagedRuntime.Revision != "rev1" ||
		response.ManagedRuntime.CacheBytes == nil || *response.ManagedRuntime.CacheBytes != 1234 {
		t.Fatalf("inspect cache facts = path=%v revision=%v bytes=%v, want resolved path/rev1/1234", response.ManagedRuntime.CachePath, response.ManagedRuntime.Revision, response.ManagedRuntime.CacheBytes)
	}
}

// TestProcessModelsPull_UsesServerFlagAndReturnsPullJSON proves pull routing and JSON pull output.
func TestProcessModelsPull_UsesServerFlagAndReturnsPullJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M/pull" {
			t.Fatalf("path = %q, want /models/OMNIVOICE_Q4_K_M/pull", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"/tmp/models/OMNIVOICE_Q4_K_M","revision":"rev1","downloadedFiles":[{"path":"weights.gguf","bytes":42}],"managedRuntimePull":{"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY"}}`)
	}))
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", strings.TrimSuffix(server.URL, "/"),
		"models", "pull", "OMNIVOICE_Q4_K_M",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models pull) error = %v\nstderr=%s", err, inputs.Stderr())
	}

	var response factoryapi.ModelPullResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatalf("decode models pull output: %v\n%s", err, inputs.Stdout())
	}
	if response.ModelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("pull response = %#v, want OMNIVOICE_Q4_K_M", response)
	}
	if response.Outcome != factoryapi.ModelPullOutcomePULLED ||
		response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY ||
		response.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("pull response = %#v, want PULLED/INSTALLED_SUCCESSFULLY/READY", response)
	}
}

// TestProcessModelsList_ReturnsHumanReadableCatalog proves list human presentation.
func TestProcessModelsList_ReturnsHumanReadableCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"results":[{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"managedRuntime":{"identity":"OMNIVOICE_Q4_K_M","revision":"rev1","cacheBytes":1234,"readinessState":"READY","lifecycleState":"INSTALLED","locality":"LOCAL","supportedOperations":[]}}]}`)
	}))
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", strings.TrimSuffix(server.URL, "/"), "models", "list",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models list) error = %v\nstderr=%s", err, inputs.Stderr())
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", "TTS", "NAME", "CACHE SIZE", "1.21 KiB (1234 bytes)"} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("list output missing %q:\n%s", want, inputs.Stdout())
		}
	}
}

// TestProcessModelsInspect_ReturnsHumanReadableDetail proves inspect human presentation.
func TestProcessModelsInspect_ReturnsHumanReadableDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M" {
			t.Fatalf("path = %q, want /models/OMNIVOICE_Q4_K_M", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"managedRuntime":{"identity":"OMNIVOICE_Q4_K_M","cachePath":"/tmp/models/OMNIVOICE_Q4_K_M/rev1","revision":"rev1","cacheBytes":1234,"readinessState":"READY","lifecycleState":"INSTALLED","locality":"LOCAL","supportedOperations":[]},"capabilities":[],"diagnostics":{}}`)
	}))
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", strings.TrimSuffix(server.URL, "/"),
		"models", "inspect", "OMNIVOICE_Q4_K_M",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models inspect) error = %v\nstderr=%s", err, inputs.Stderr())
	}
	for _, want := range []string{
		"Name:\tOMNIVOICE_Q4_K_M", "Revision:\trev1",
		"Cache Size:\t1.21 KiB (1234 bytes)",
		"Cache Path:\t/tmp/models/OMNIVOICE_Q4_K_M/rev1", "TTS",
	} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("inspect output missing %q:\n%s", want, inputs.Stdout())
		}
	}
}

func TestProcessModelsRemoteFailuresExposeSafeDiagnosticsAndHTTPMetadata(t *testing.T) {
	tests := []struct {
		name       string
		command    []string
		status     int
		body       string
		wantOutput []string
		forbidden  []string
	}{
		{
			name:    "typed list failure",
			command: []string{"models", "list"},
			status:  http.StatusServiceUnavailable,
			body:    `{"code":"SERVICE_UNAVAILABLE","family":"SERVICE_UNAVAILABLE","message":"catalog temporarily unavailable"}`,
			wantOutput: []string{
				"SERVICE_UNAVAILABLE", "catalog temporarily unavailable",
				"debug: http method=GET", "status=503",
			},
		},
		{
			name:    "malformed inspect success",
			command: []string{"models", "inspect", "voice"},
			status:  http.StatusOK,
			body:    `{`,
			wantOutput: []string{
				"CLI_COMMAND_FAILED", "debug: http method=GET", "status=200",
			},
			forbidden: []string{"query-secret"},
		},
		{
			name:    "malformed pull failure",
			command: []string{"models", "pull", "voice"},
			status:  http.StatusBadGateway,
			body:    `upstream body=credential-secret`,
			wantOutput: []string{
				"CLI_COMMAND_FAILED", "debug: http method=POST", "status=502",
			},
			forbidden: []string{"credential-secret"},
		},
		{
			name:       "remove success",
			command:    []string{"models", "remove", "voice"},
			status:     http.StatusOK,
			body:       `{"modelName":"voice","outcome":"REMOVED","revision":"rev1","cachePath":"/tmp/voice","bytesRemoved":42}`,
			wantOutput: []string{"voice", "REMOVED", `"bytesRemoved":42`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.command[1] == "remove" && r.Method != http.MethodDelete {
					t.Errorf("remove method = %s, want DELETE", r.Method)
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			t.Cleanup(server.Close)

			process := support.BuildProcess(t, serviceedges.Edges{})
			args := []string{"you", "--json", "--debug", "--server", strings.TrimSuffix(server.URL, "/")}
			args = append(args, test.command...)
			inputs := support.FakeInputs(t.Context(), args)
			err := process.Execute(inputs.Input)
			if test.status >= http.StatusBadRequest || test.body == "{" {
				if err == nil {
					t.Fatalf("Process.Execute(%v) error = nil, want failure\nstdout=%s\nstderr=%s", test.command, inputs.Stdout(), inputs.Stderr())
				}
			} else if err != nil {
				t.Fatalf("Process.Execute(%v) error = %v\nstdout=%s\nstderr=%s", test.command, err, inputs.Stdout(), inputs.Stderr())
			}
			combined := inputs.Stdout() + inputs.Stderr()
			for _, want := range test.wantOutput {
				if !strings.Contains(combined, want) {
					t.Fatalf("models %s output missing %q:\n%s", test.name, want, combined)
				}
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(combined, forbidden) {
					t.Fatalf("models %s output leaked %q:\n%s", test.name, forbidden, combined)
				}
			}
		})
	}
}
