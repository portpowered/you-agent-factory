package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func testRuntimeScope(t *testing.T) modelinference.RuntimeScopeRef {
	t.Helper()
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("cli-parity:test-scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	return scope
}

func testModelLease(t *testing.T) modelinference.ModelLeaseRef {
	t.Helper()
	lease, err := (modelinference.ModelLeaseRef{}).Parse("cli-parity:test-lease")
	if err != nil {
		t.Fatalf("parse model lease: %v", err)
	}
	return lease
}

func testArtifactRef(t *testing.T, path string) modelinference.InferenceArtifactRef {
	t.Helper()
	ref, err := (modelinference.InferenceArtifactRef{}).Parse(path)
	if err != nil {
		t.Fatalf("parse artifact ref: %v", err)
	}
	return ref
}

type copyArtifactExporter struct{}

func (copyArtifactExporter) ExportInvocationArtifact(sourcePath, destinationPath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(destinationPath, data, 0o644)
}

func parityInvokeScope(t *testing.T) modelscli.InvokeRuntimeScope {
	t.Helper()
	return modelscli.InvokeRuntimeScope{Scope: testRuntimeScope(t)}
}

func parityRootService(t *testing.T, root stubModelsRoot) modelscli.Service {
	t.Helper()
	service := modelscli.NewService(modelscli.Config{
		Models: root,
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return parityInvokeScope(t), nil
		},
		Artifacts: copyArtifactExporter{},
	})
	if service == nil {
		t.Fatal("NewService() = nil, want Models CLI service")
	}
	return service
}

func TestRootAdapter_ListJSONPreservesAcceptedOutput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	service := parityRootService(t, stubModelsRoot{
		listModels: func(context.Context) (modelinference.List, error) {
			return modelinference.List{
				Results: []modelinference.Summary{{
					Name: "OMNIVOICE_Q4_K_M",
					ManagedRuntime: modelinference.Runtime{
						ReadinessState: modelinference.ReadinessStateReady,
						LifecycleState: modelinference.LifecycleStateInstalled,
					},
					Operations: []modelinference.Operation{{Name: "TTS"}},
				}},
			}, nil
		},
	})
	if err := service.List(modelscli.ListConfig{
		Context: context.Background(),
		JSON:    true,
		Output:  &out,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var response factoryapi.ListModelsResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("json output invalid: %v\n%s", err, out.String())
	}
	if len(response.Results) != 1 || response.Results[0].Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("List() JSON = %#v, want OMNIVOICE_Q4_K_M", response.Results)
	}
}

func TestRootAdapter_InspectSuccessPreservesHumanOutput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	service := parityRootService(t, stubModelsRoot{
		getModel: func(_ context.Context, name string) (modelinference.Detail, error) {
			return modelinference.Detail{
				Summary: modelinference.Summary{
					Name: name,
					ManagedRuntime: modelinference.Runtime{
						ReadinessState: modelinference.ReadinessStateReady,
						LifecycleState: modelinference.LifecycleStateInstalled,
					},
					Operations: []modelinference.Operation{{Name: "TTS"}},
				},
			}, nil
		},
	})
	if err := service.Inspect(modelscli.InspectConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &out,
	}); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Name:\tOMNIVOICE_Q4_K_M", "Readiness:\tREADY", "TTS"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Inspect() output missing %q:\n%s", want, got)
		}
	}
}

func TestRootAdapter_PullSuccessPreservesHumanAndJSONOutput(t *testing.T) {
	t.Parallel()

	root := stubModelsRoot{
		pullModel: func(_ context.Context, name string) (modelinference.PullResult, error) {
			return modelinference.PullResult{
				ModelName:          name,
				ProviderLocality:   "LOCAL",
				Outcome:            "PULLED",
				CachePath:          "/tmp/models/" + name,
				Revision:           "rev1",
				ManagedPullOutcome: "INSTALLED_SUCCESSFULLY",
				ReadinessState:     "READY",
				DownloadedFiles:    []modelinference.DownloadedFile{{Path: "weights.gguf", Bytes: 42}},
			}, nil
		},
	}
	service := parityRootService(t, root)

	var human bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Output: &human,
	}); err != nil {
		t.Fatalf("Pull() human error = %v", err)
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", "INSTALLED_SUCCESSFULLY", "weights.gguf"} {
		if !strings.Contains(human.String(), want) {
			t.Fatalf("Pull() human output missing %q:\n%s", want, human.String())
		}
	}

	var jsonOut bytes.Buffer
	if err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", JSON: true, Output: &jsonOut,
	}); err != nil {
		t.Fatalf("Pull() JSON error = %v", err)
	}
	var response factoryapi.ModelPullResponse
	if err := json.Unmarshal(jsonOut.Bytes(), &response); err != nil {
		t.Fatalf("Pull() JSON invalid: %v\n%s", err, jsonOut.String())
	}
	if response.ModelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("Pull() JSON model = %q, want OMNIVOICE_Q4_K_M", response.ModelName)
	}
}

func TestRootAdapter_InvokeJSONResolvesThroughModelsRootCatalogAndInference(t *testing.T) {
	t.Parallel()

	scope := testRuntimeScope(t)
	var gotCatalog, gotAcquire, gotInvoke bool
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(_ context.Context, request modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				gotCatalog = true
				if request.Scope != scope || request.Name != "OMNIVOICE_Q4_K_M" || request.Operation != "TTS" {
					t.Fatalf("GetCatalogModel request = %#v", request)
				}
				return modelinference.GetModelResult{
					Model: modelinference.Detail{
						Summary: modelinference.Summary{
							Name:             "OMNIVOICE_Q4_K_M",
							ProviderLocality: modelinference.LocalityLocal,
							Operations: []modelinference.Operation{{
								Name: "TTS",
								Inputs: []modelinference.OperationSlot{{
									Name: "text", ContentTypes: []string{"text"},
								}},
							}},
						},
						Capabilities: []modelinference.Capability{{
							Worker:           "tts-worker",
							ProviderLocality: modelinference.LocalityLocal,
							Operations: []modelinference.Operation{{
								Name: "TTS",
								Inputs: []modelinference.OperationSlot{{
									Name: "text", ContentTypes: []string{"text"},
								}},
							}},
						}},
					},
				}, nil
			},
			acquireModelLease: func(_ context.Context, request modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
				gotAcquire = true
				if request.Scope != scope || request.Name != "OMNIVOICE_Q4_K_M" || request.Holder != "you-models-cli-invoke" {
					t.Fatalf("AcquireModelLease request = %#v", request)
				}
				return modelinference.AcquireModelLeaseResult{
					Lease: modelinference.ModelLease{Lease: testModelLease(t)},
				}, nil
			},
			invokeModelWithLease: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				gotInvoke = true
				if request.Scope != scope || request.ModelName != "OMNIVOICE_Q4_K_M" || request.Operation != "TTS" {
					t.Fatalf("InvokeModelWithLease request = %#v", request)
				}
				if request.Input.Content != "hello world" {
					t.Fatalf("InvokeModelWithLease input = %#v", request.Input)
				}
				return modelinference.InvokeModelResult{
					ModelName: "OMNIVOICE_Q4_K_M",
					Operation: "TTS",
					Content:   []modelinference.InferenceContent{{ContentType: "text/plain", Content: "synthesized"}},
				}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
	})

	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Operation: "TTS",
		Text:      "hello world",
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !gotCatalog || !gotAcquire || !gotInvoke {
		t.Fatalf("invoke path calls: catalog=%v acquire=%v invoke=%v", gotCatalog, gotAcquire, gotInvoke)
	}
	for _, want := range []string{
		"OMNIVOICE_Q4_K_M",
		`"operation":"TTS"`,
		`"worker":"tts-worker"`,
		`"providerLocality":"LOCAL"`,
		`"bindings"`,
		`"text":"hello world"`,
	} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("Invoke() JSON missing %q:\n%s", want, out.String())
		}
	}
}

func TestRootAdapter_InvokeAudioExportsArtifactThroughModelsRoot(t *testing.T) {
	t.Parallel()

	audioBytes := []byte("RIFF....WAVE")
	streamFile := filepath.Join(t.TempDir(), "stream.wav")
	if err := os.WriteFile(streamFile, audioBytes, 0o644); err != nil {
		t.Fatalf("write stream file: %v", err)
	}
	scope := testRuntimeScope(t)
	lease := testModelLease(t)
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{}, nil
			},
			acquireModelLease: func(context.Context, modelinference.AcquireModelLeaseRequest) (modelinference.AcquireModelLeaseResult, error) {
				return modelinference.AcquireModelLeaseResult{
					Lease: modelinference.ModelLease{Lease: lease},
				}, nil
			},
			invokeModelWithLease: func(_ context.Context, request modelinference.InvokeModelRequest) (modelinference.InvokeModelResult, error) {
				if request.ResponseMode != modelinference.ResponseModeAudioStream {
					t.Fatalf("ResponseMode = %q, want AUDIO_STREAM", request.ResponseMode)
				}
				return modelinference.InvokeModelResult{
					Artifacts: []modelinference.InferenceArtifact{{
						Artifact: testArtifactRef(t, streamFile),
					}},
				}, nil
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return modelscli.InvokeRuntimeScope{Scope: scope}, nil
		},
		Artifacts: copyArtifactExporter{},
	})

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	var out bytes.Buffer
	if err := service.Invoke(modelscli.InvokeConfig{
		Context:    context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		OutputPath: outputPath,
		Output:     &out,
	}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !bytes.Equal(got, audioBytes) {
		t.Fatalf("output bytes = %q, want %q", got, audioBytes)
	}
	if !strings.Contains(out.String(), "Wrote audio: "+outputPath) {
		t.Fatalf("Invoke() stdout = %q, want wrote-audio confirmation", out.String())
	}
}

func TestRootAdapter_ValidationFailuresPreserveExitRelevantErrors(t *testing.T) {
	t.Parallel()

	service := parityRootService(t, stubModelsRoot{})
	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "pull missing model name",
			run: func() error {
				return service.Pull(modelscli.PullConfig{Context: context.Background(), Output: io.Discard})
			},
			want: "model name is required",
		},
		{
			name: "invoke missing operation",
			run: func() error {
				return service.Invoke(modelscli.InvokeConfig{
					Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M",
					Text: "hello", Output: io.Discard,
				})
			},
			want: "--operation is required",
		},
		{
			name: "invoke missing text",
			run: func() error {
				return service.Invoke(modelscli.InvokeConfig{
					Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M",
					Operation: "TTS", Output: io.Discard,
				})
			},
			want: "--text is required",
		},
		{
			name: "invoke missing output path",
			run: func() error {
				return service.Invoke(modelscli.InvokeConfig{
					Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M",
					Operation: "TTS", Text: "hello", Output: io.Discard,
				})
			},
			want: "--output is required unless --json is set",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRootAdapter_PullMapsClassifiedModelsRootFailure(t *testing.T) {
	t.Parallel()

	service := parityRootService(t, stubModelsRoot{
		pullModel: func(context.Context, string) (modelinference.PullResult, error) {
			return modelinference.PullResult{
				ModelName:          "OMNIVOICE_Q4_K_M",
				ManagedPullOutcome: "SOURCE_FETCH_FAILED",
				ReadinessState:     "FAILED",
			}, &modelinference.PullError{
				Result: modelinference.PullResult{
					ModelName:          "OMNIVOICE_Q4_K_M",
					ManagedPullOutcome: "SOURCE_FETCH_FAILED",
					ReadinessState:     "FAILED",
				},
				Cause: errors.New("source fetch failed"),
			}
		},
	})
	var out bytes.Buffer
	err := service.Pull(modelscli.PullConfig{
		Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", JSON: true, Output: &out,
	})
	if err == nil {
		t.Fatal("Pull() error = nil, want classified pull failure")
	}
	if !strings.Contains(err.Error(), "SOURCE_FETCH_FAILED") {
		t.Fatalf("Pull() error = %q, want classified outcome", err.Error())
	}
	var response factoryapi.ModelPullResponse
	if decodeErr := json.Unmarshal(out.Bytes(), &response); decodeErr != nil {
		t.Fatalf("Pull() JSON invalid: %v\n%s", decodeErr, out.String())
	}
	if response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED {
		t.Fatalf("pull outcome = %s, want SOURCE_FETCH_FAILED", response.ManagedRuntimePull.PullOutcome)
	}
}

func TestRootAdapter_InvokeMapsReadinessFailuresFromModelsRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		root   func() error
		wantIs error
	}{
		{
			name: "missing",
			root: func() error {
				return modelinference.Runtime{
					Identity:       "OMNIVOICE_Q4_K_M",
					ReadinessState: modelinference.ReadinessStateMissing,
					LifecycleState: modelinference.LifecycleStateNotInstalled,
				}.InvocationError()
			},
			wantIs: modelinference.ErrMissing,
		},
		{
			name: "loading",
			root: func() error {
				return modelinference.Runtime{
					Identity:       "OMNIVOICE_Q4_K_M",
					ReadinessState: modelinference.ReadinessStateLoading,
					LifecycleState: modelinference.LifecycleStateLoading,
				}.InvocationError()
			},
			wantIs: modelinference.ErrLoading,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			service := modelscli.NewService(modelscli.Config{
				Models: stubModelsRoot{
					getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
						return modelinference.GetModelResult{}, tt.root()
					},
				},
				OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
					return parityInvokeScope(t), nil
				},
			})
			err := service.Invoke(modelscli.InvokeConfig{
				Context: context.Background(),
				ModelName: "OMNIVOICE_Q4_K_M",
				Operation: "TTS",
				Text:      "hello world",
				JSON:      true,
				Output:    io.Discard,
			})
			if err == nil {
				t.Fatal("Invoke() error = nil, want readiness failure")
			}
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("Invoke() error = %v, want errors.Is %v", err, tt.wantIs)
			}
		})
	}
}

func TestRootAdapter_InvokeMapsModelsRootNotFound(t *testing.T) {
	t.Parallel()

	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{
			getCatalogModel: func(context.Context, modelinference.GetModelRequest) (modelinference.GetModelResult, error) {
				return modelinference.GetModelResult{}, modelinference.ErrNotFound
			},
		},
		OpenInvokeScope: func(context.Context, modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			return parityInvokeScope(t), nil
		},
	})
	err := service.Invoke(modelscli.InvokeConfig{
		Context: context.Background(),
		ModelName: "missing-model",
		Operation: "TTS",
		Text:      "hello world",
		JSON:      true,
		Output:    io.Discard,
	})
	if err == nil {
		t.Fatal("Invoke() error = nil, want not-found failure")
	}
	if !errors.Is(err, modelscli.ErrModelNotFound) {
		t.Fatalf("Invoke() error = %v, want ErrModelNotFound", err)
	}
}

func TestRootAdapter_ListHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := parityRootService(t, stubModelsRoot{
		listModels: func(callCtx context.Context) (modelinference.List, error) {
			if err := callCtx.Err(); err != nil {
				return modelinference.List{}, err
			}
			return modelinference.List{}, nil
		},
	})
	err := service.List(modelscli.ListConfig{Context: ctx, Output: io.Discard})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
}

func TestRootAdapter_InvokeHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := modelscli.NewService(modelscli.Config{
		Models: stubModelsRoot{},
		OpenInvokeScope: func(callCtx context.Context, _ modelscli.InvokeConfig) (modelscli.InvokeRuntimeScope, error) {
			if err := callCtx.Err(); err != nil {
				return modelscli.InvokeRuntimeScope{}, err
			}
			return parityInvokeScope(t), nil
		},
	})
	err := service.Invoke(modelscli.InvokeConfig{
		Context:   ctx,
		ModelName: "OMNIVOICE_Q4_K_M",
		Operation: "TTS",
		Text:      "hello world",
		JSON:      true,
		Output:    io.Discard,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Invoke() error = %v, want context.Canceled", err)
	}
}
