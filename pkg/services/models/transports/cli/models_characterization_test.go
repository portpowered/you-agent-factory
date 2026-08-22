package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
)

// TestModelsCLICharacterizationSuccessProjections pins the current human and
// JSON projections at the public Models CLI service boundary. The Models root
// is a deterministic collaborator because the real local runtime shells out
// to an executable that is intentionally absent from this repository.
func TestModelsCLICharacterizationSuccessProjections(t *testing.T) {
	t.Parallel()

	t.Run("list", func(t *testing.T) {
		service := characterizationModelsCLIService(t, stubModelsRoot{
			listModels: func(context.Context) (modelinference.List, error) {
				return modelinference.List{Results: []modelinference.Summary{
					characterizationModelSummary(),
				}}, nil
			},
		})

		var human bytes.Buffer
		if err := service.List(modelscli.ListConfig{Context: context.Background(), Output: &human}); err != nil {
			t.Fatalf("List() human error = %v", err)
		}
		if got, want := human.String(), "NAME\tREADINESS\tLIFECYCLE\tLOCALITY\tOPERATIONS\tMODALITIES\tRESOURCES\nOMNIVOICE_Q4_K_M\tREADY\tINSTALLED\tLOCAL\tTTS\tAUDIO,TEXT\t1\n"; got != want {
			t.Fatalf("List() human = %q, want %q", got, want)
		}

		var structured bytes.Buffer
		if err := service.List(modelscli.ListConfig{
			Context: context.Background(), JSON: true, Output: &structured,
		}); err != nil {
			t.Fatalf("List() JSON error = %v", err)
		}
		assertCharacterizationJSON(t, structured.String(), `{"results":[{"loadState":"UNLOADED","managedRuntime":{"diagnostics":{"cache":"omnivoice-cache"},"identity":"OMNIVOICE_Q4_K_M","lifecycleState":"INSTALLED","locality":"LOCAL","readinessState":"READY","supportedOperations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}]},"modalities":["TEXT","AUDIO"],"name":"OMNIVOICE_Q4_K_M","operations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}],"providerLocality":"LOCAL","resources":[{"backend":"LLAMACPP","capacity":1,"loadPolicy":"ON_DEMAND","model":"OMNIVOICE_Q4_K_M","name":"omnivoice-cache","type":"MODEL"}],"status":"READY"}]}`)
	})

	t.Run("inspect", func(t *testing.T) {
		service := characterizationModelsCLIService(t, stubModelsRoot{
			getModel: func(context.Context, string) (modelinference.Detail, error) {
				return characterizationModelDetail(), nil
			},
		})

		var human bytes.Buffer
		if err := service.Inspect(modelscli.InspectConfig{
			Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &human,
		}); err != nil {
			t.Fatalf("Inspect() human error = %v", err)
		}
		if got, want := human.String(), "Name:\tOMNIVOICE_Q4_K_M\nReadiness:\tREADY\nLifecycle:\tINSTALLED\nLocality:\tLOCAL\nOperations:\tTTS\nModalities:\tAUDIO,TEXT\nResources:\t1\nCapabilities:\n- tts-executor\tLOCAL\tTTS\nDiagnostics:\n- cache=omnivoice-cache\n"; got != want {
			t.Fatalf("Inspect() human = %q, want %q", got, want)
		}

		var structured bytes.Buffer
		if err := service.Inspect(modelscli.InspectConfig{
			Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", JSON: true, Output: &structured,
		}); err != nil {
			t.Fatalf("Inspect() JSON error = %v", err)
		}
		assertCharacterizationJSON(t, structured.String(), `{"capabilities":[{"modelProvider":"CODEX","operations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}],"providerLocality":"LOCAL","resourceNames":["omnivoice-cache"],"worker":"tts-executor"}],"diagnostics":{"statusReason":"managed runtime is discoverable"},"loadState":"UNLOADED","managedRuntime":{"diagnostics":{"cache":"omnivoice-cache"},"identity":"OMNIVOICE_Q4_K_M","lifecycleState":"INSTALLED","locality":"LOCAL","readinessState":"READY","supportedOperations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}]},"modalities":["TEXT","AUDIO"],"name":"OMNIVOICE_Q4_K_M","operations":[{"inputs":[{"contentTypes":["TEXT"],"name":"text","required":true}],"name":"TTS","outputs":[{"contentTypes":["AUDIO"],"name":"audio"}]}],"providerLocality":"LOCAL","resources":[{"backend":"LLAMACPP","capacity":1,"loadPolicy":"ON_DEMAND","model":"OMNIVOICE_Q4_K_M","name":"omnivoice-cache","type":"MODEL"}],"status":"READY"}`)
	})

	t.Run("pull", func(t *testing.T) {
		service := characterizationModelsCLIService(t, stubModelsRoot{
			pullModel: func(_ context.Context, name string) (modelinference.PullResult, error) {
				return modelinference.PullResult{
					ModelName: name, ProviderLocality: "LOCAL", Outcome: "PULLED",
					CachePath: "/models/OMNIVOICE_Q4_K_M/rev-2026", Revision: "rev-2026",
					ManagedPullOutcome: "INSTALLED_SUCCESSFULLY", ReadinessState: "READY",
					DownloadedFiles: []modelinference.DownloadedFile{
						{Path: "weights.gguf", Bytes: 42, SHA256: "abc123"},
						{Path: "config.json", Bytes: 7},
					},
				}, nil
			},
		})

		var human bytes.Buffer
		if err := service.Pull(modelscli.PullConfig{
			Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &human,
		}); err != nil {
			t.Fatalf("Pull() human error = %v", err)
		}
		if got, want := human.String(), "MODEL\tPULL OUTCOME\tREADINESS\tLIFECYCLE\tREVISION\tCACHE PATH\nOMNIVOICE_Q4_K_M\tINSTALLED_SUCCESSFULLY\tREADY\tINSTALLED\trev-2026\t/models/OMNIVOICE_Q4_K_M/rev-2026\nFILES\nconfig.json\t7\nweights.gguf\t42\n"; got != want {
			t.Fatalf("Pull() human = %q, want %q", got, want)
		}

		var structured bytes.Buffer
		if err := service.Pull(modelscli.PullConfig{
			Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", JSON: true, Output: &structured,
		}); err != nil {
			t.Fatalf("Pull() JSON error = %v", err)
		}
		assertCharacterizationJSON(t, structured.String(), `{"cachePath":"/models/OMNIVOICE_Q4_K_M/rev-2026","downloadedFiles":[{"bytes":42,"path":"weights.gguf","sha256":"abc123"},{"bytes":7,"path":"config.json"}],"managedRuntimePull":{"cachePath":"/models/OMNIVOICE_Q4_K_M/rev-2026","downloadedFiles":[{"bytes":42,"path":"weights.gguf","sha256":"abc123"},{"bytes":7,"path":"config.json"}],"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY","revision":"rev-2026"},"modelName":"OMNIVOICE_Q4_K_M","outcome":"PULLED","providerLocality":"LOCAL","revision":"rev-2026"}`)
	})
}

// TestModelsCLICharacterizationInvokeProjectionsAndLifecycle pins both
// invocation output modes and the current release behavior. The release
// observations are characterized, not endorsed: the Models root reports the
// lease disposition from its single invocation boundary and the CLI closes
// its presentation scope after either result.
func TestModelsCLICharacterizationInvokeProjectionsAndLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("audio file", func(t *testing.T) {
		var scopeClosed, leaseReleased bool
		var exportedSource, exportedDestination string
		service := characterizationModelsCLIServiceWithLifecycle(t, stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{Model: characterizationModelDetail()}, nil
			},
			acquireModelLease: func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
				return modelinference.AcquireModelLeaseResult{Lease: modelinference.ModelLease{Lease: testModelLease(t)}}, nil
			},
			invokeModelWithLease: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				if request.ResponseMode != modelinference.ResponseModeAudioStream {
					t.Fatalf("InvokeModelWithLease() response mode = %q, want AUDIO_STREAM", request.ResponseMode)
				}
				result := modelinference.InvokeModelResult{
					ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS",
					LeaseDisposition: modelinference.InvocationLeaseReleased,
					Artifacts:        []modelinference.InferenceArtifact{{Artifact: testArtifactRef(t, "characterized-source.wav")}},
				}
				leaseReleased = result.LeaseDisposition == modelinference.InvocationLeaseReleased
				return result, nil
			},
		}, &scopeClosed, func(sourcePath, destinationPath string) error {
			exportedSource, exportedDestination = sourcePath, destinationPath
			return nil
		})

		var out bytes.Buffer
		if err := service.Invoke(modelscli.InvokeConfig{
			Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS",
			Text: "hello world", OutputPath: "speech.wav", Output: &out,
		}); err != nil {
			t.Fatalf("Invoke() audio error = %v", err)
		}
		if got, want := out.String(), "Wrote audio: speech.wav\n"; got != want {
			t.Fatalf("Invoke() audio stdout = %q, want %q", got, want)
		}
		if strings.Contains(out.String(), "{") {
			t.Fatalf("Invoke() audio stdout unexpectedly contains JSON: %q", out.String())
		}
		if exportedSource != "characterized-source.wav" || exportedDestination != "speech.wav" {
			t.Fatalf("exported artifact = (%q, %q), want characterized-source.wav/speech.wav", exportedSource, exportedDestination)
		}
		if !scopeClosed || !leaseReleased {
			t.Fatalf("Invoke() lifecycle = scopeClosed:%t leaseReleased:%t, want both true", scopeClosed, leaseReleased)
		}
	})

	t.Run("JSON and inference failure", func(t *testing.T) {
		var scopeClosed, leaseReleased bool
		service := characterizationModelsCLIServiceWithLifecycle(t, stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{Model: characterizationModelDetail()}, nil
			},
			acquireModelLease: func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
				return modelinference.AcquireModelLeaseResult{Lease: modelinference.ModelLease{Lease: testModelLease(t)}}, nil
			},
			invokeModelWithLease: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				result := modelinference.InvokeModelResult{LeaseDisposition: modelinference.InvocationLeaseReleased}
				leaseReleased = result.LeaseDisposition == modelinference.InvocationLeaseReleased
				return result, errors.New("characterized inference failure")
			},
		}, &scopeClosed, nil)

		var out bytes.Buffer
		err := service.Invoke(modelscli.InvokeConfig{
			Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS",
			Text: "hello world", JSON: true, Output: &out,
		})
		if err == nil || err.Error() != "characterized inference failure" {
			t.Fatalf("Invoke() failure = %v, want exact characterized inference failure", err)
		}
		if out.Len() != 0 {
			t.Fatalf("Invoke() failure stdout = %q, want empty", out.String())
		}
		if !scopeClosed || !leaseReleased {
			t.Fatalf("Invoke() failure lifecycle = scopeClosed:%t leaseReleased:%t, want both true", scopeClosed, leaseReleased)
		}
	})

	t.Run("JSON success", func(t *testing.T) {
		service := characterizationModelsCLIService(t, stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{Model: characterizationModelDetail()}, nil
			},
			acquireModelLease: func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
				return modelinference.AcquireModelLeaseResult{Lease: modelinference.ModelLease{Lease: testModelLease(t)}}, nil
			},
			invokeModelWithLease: func(context.Context, modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				return modelinference.InvokeModelResult{
					ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS",
					LeaseDisposition: modelinference.InvocationLeaseReleased,
					Content:          []modelinference.InferenceContent{{ContentType: "audio/wav", Content: "characterized-speech.wav"}},
				}, nil
			},
		})

		var out bytes.Buffer
		if err := service.Invoke(modelscli.InvokeConfig{
			Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS",
			Text: "hello world", JSON: true, Output: &out,
		}); err != nil {
			t.Fatalf("Invoke() JSON error = %v", err)
		}
		assertCharacterizationJSON(t, out.String(), `{"bindings":[{"content":[{"text":"hello world","type":"text"}],"slot":"text","source":"INPUT"}],"content":[{"contentType":"audio/wav","file":"characterized-speech.wav","slot":"audio","type":"AUDIO","url":""}],"modelName":"OMNIVOICE_Q4_K_M","operation":"TTS","providerLocality":"LOCAL","worker":"tts-executor"}`)
	})
}

func characterizationModelsCLIService(t *testing.T, root stubModelsRoot) modelscli.Service {
	t.Helper()
	return characterizationModelsCLIServiceWithLifecycle(t, root, nil, nil)
}

func characterizationModelsCLIServiceWithLifecycle(
	t *testing.T,
	root stubModelsRoot,
	scopeClosed *bool,
	export func(string, string) error,
) modelscli.Service {
	t.Helper()
	scope := testRuntimeScope(t)
	closeScope := func(context.Context) error {
		if scopeClosed != nil {
			*scopeClosed = true
		}
		return nil
	}
	if export == nil {
		export = func(string, string) error { return nil }
	}
	service := modelscli.NewService(modelscli.Config{
		Models:    root,
		Artifacts: characterizationArtifactExporter{export: export},
		OpenCatalogScope: func(context.Context) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope, Close: closeScope}, nil
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope, Close: closeScope}, nil
		},
	})
	if service == nil {
		t.Fatal("NewService() = nil, want Models CLI service")
	}
	return service
}

type characterizationArtifactExporter struct {
	export func(string, string) error
}

func (exporter characterizationArtifactExporter) ExportInvocationArtifact(sourcePath, destinationPath string) error {
	return exporter.export(sourcePath, destinationPath)
}

func characterizationModelSummary() modelinference.Summary {
	return modelinference.Summary{
		Name: "OMNIVOICE_Q4_K_M", ProviderLocality: modelinference.LocalityLocal,
		Status: modelinference.StatusReady, LoadState: modelinference.LoadStateUnloaded,
		Operations: []modelinference.Operation{characterizationTTSOperation()},
		Modalities: []string{"TEXT", "AUDIO"},
		Resources: []modelinference.ResourceSummary{{
			Name: "omnivoice-cache", Type: "MODEL", Capacity: 1,
			Model: characterizationString("OMNIVOICE_Q4_K_M"), Backend: characterizationString("LLAMACPP"),
			LoadPolicy: characterizationString("ON_DEMAND"),
		}},
		ManagedRuntime: modelinference.Runtime{
			Identity: "OMNIVOICE_Q4_K_M", ReadinessState: modelinference.ReadinessStateReady,
			LifecycleState: modelinference.LifecycleStateInstalled, Locality: modelinference.LocalityLocal,
			SupportedOperations: []modelinference.Operation{characterizationTTSOperation()},
			Diagnostics:         map[string]string{"cache": "omnivoice-cache"},
		},
	}
}

func characterizationModelDetail() modelinference.Detail {
	summary := characterizationModelSummary()
	return modelinference.Detail{
		Summary: summary,
		Capabilities: []modelinference.Capability{{
			Worker: "tts-executor", ProviderLocality: modelinference.LocalityLocal,
			ModelProvider: characterizationString("CODEX"), Operations: []modelinference.Operation{characterizationTTSOperation()},
			ResourceNames: []string{"omnivoice-cache"},
		}},
		Diagnostics: map[string]string{"statusReason": "managed runtime is discoverable"},
	}
}

func characterizationTTSOperation() modelinference.Operation {
	required := true
	return modelinference.Operation{
		Name: "TTS",
		Inputs: []modelinference.OperationSlot{{
			Name: "text", ContentTypes: []string{"TEXT"}, Required: &required,
		}},
		Outputs: []modelinference.OperationSlot{{
			Name: "audio", ContentTypes: []string{"AUDIO"},
		}},
	}
}

func characterizationString(value string) *string {
	return &value
}

func assertCharacterizationJSON(t *testing.T, got, want string) {
	t.Helper()
	if strings.TrimSpace(got) != want {
		t.Fatalf("JSON = %q, want exact %q", strings.TrimSpace(got), want)
	}
}
