package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelartifacts "github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	"google.golang.org/protobuf/proto"
)

type recordingInvocationProtocolClient struct {
	request  models.InvocationProtocolRequest
	response models.InvocationProtocolResponse
	err      error
}

func (client *recordingInvocationProtocolClient) Predict(
	_ context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	client.request = request
	return client.response, client.err
}

func TestNewInvocationRuntimeFailsClosedWhenOmniProtocolIsUnbound(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("scope:unbound-omni")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationOMNI)
	if !ok {
		t.Fatal("GenericOperationContract(OMNI) = false")
	}
	runtime := newInvocationRuntime(nil, nil)
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Scope: scope, Holder: "unbound-test", Model: models.ModelReference{NameOrURI: "llm"},
			Operation: models.OperationOMNI,
			Inputs: []models.InferenceInput{
				{Name: "prompt", Modality: models.ModalityText, Content: "Write a haiku"},
			},
		},
		Operation: operation,
	})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol {
		t.Fatalf("Invoke error = %v, failure = %#v, want typed backend-protocol failure", err, failure)
	}
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("Invoke error = %v, want ErrUnavailable cause", err)
	}
	if len(result.Content) != 0 {
		t.Fatalf("Invoke result = %#v, want no fallback content", result)
	}
}

func TestNewInvocationRuntimeForwardsOmniInputsAndDeclaredUsage(t *testing.T) {
	t.Parallel()

	client := &recordingInvocationProtocolClient{
		response: models.InvocationProtocolResponse{
			Text:  "fixture answer",
			Usage: `{"tokens":3}`,
		},
	}
	result := invokeBoundOmni(t, client)
	assertBoundOmniResult(t, result, client)
}

func invokeBoundOmni(t *testing.T, client *recordingInvocationProtocolClient) inference.InvocationRuntimeResult {
	t.Helper()
	runtime := newInvocationRuntime(client, nil)
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:bound-omni")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationOMNI)
	if !ok {
		t.Fatal("GenericOperationContract(OMNI) = false")
	}
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Scope: scope, Holder: "bound-test", Model: models.ModelReference{NameOrURI: "llm"},
			Operation: models.OperationOMNI,
			Inputs: []models.InferenceInput{
				{Name: "prompt", Modality: models.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: "compare these"},
				{Name: "image", Modality: models.ModalityImage, ContentType: "image/png", MediaType: "image/png", Content: "PNG-A"},
				{Name: "image", Modality: models.ModalityImage, ContentType: "image/png", MediaType: "image/png", Content: "PNG-A"},
				{Name: "video", Modality: models.ModalityVideo, ContentType: "video/mp4", MediaType: "video/mp4", Content: "MP4-CLIP"},
			},
		},
		Operation: operation,
	})
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	return result
}

func assertBoundOmniResult(
	t *testing.T,
	result inference.InvocationRuntimeResult,
	client *recordingInvocationProtocolClient,
) {
	t.Helper()
	assertBoundOmniContent(t, result)
	assertBoundOmniArtifact(t, result)
	assertBoundOmniRequest(t, client)
}

func assertBoundOmniContent(t *testing.T, result inference.InvocationRuntimeResult) {
	t.Helper()
	if len(result.Content) != 2 || result.Content[0].Name != "text" || result.Content[0].Content != "fixture answer" || result.Content[1].Name != "usage" {
		t.Fatalf("Invoke content = %#v, want text and declared usage", result.Content)
	}
	if result.Content[0].Modality != models.ModalityText || result.Content[0].ContentType != "text/plain" || result.Content[0].MediaType != "text/plain" ||
		result.Content[1].Modality != models.ModalityJSON || result.Content[1].ContentType != "application/json" || result.Content[1].MediaType != "application/json" ||
		result.Content[1].Content != `{"tokens":3}` {
		t.Fatalf("Invoke content metadata = %#v, want typed text and JSON usage", result.Content)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("Invoke artifacts = %#v, want one forwarded descriptor", result.Artifacts)
	}
}

func assertBoundOmniArtifact(t *testing.T, result inference.InvocationRuntimeResult) {
	t.Helper()
	if len(result.Artifacts) != 1 {
		t.Fatalf("Invoke artifacts = %#v, want one forwarded descriptor", result.Artifacts)
	}
	artifact := result.Artifacts[0]
	if artifact.RefValue != "" || artifact.SourcePath != "" || artifact.Name != "text" ||
		artifact.MediaType != "text/plain" || artifact.SizeBytes != int64(len([]byte("fixture answer"))) {
		t.Fatalf("Invoke artifact = %#v, want detached zero-reference text metadata", artifact)
	}
}

func assertBoundOmniRequest(t *testing.T, client *recordingInvocationProtocolClient) {
	t.Helper()
	if client.request.Operation != models.OperationOMNI || client.request.Prompt != "compare these" {
		t.Fatalf("protocol request = %#v, want OMNI prompt", client.request)
	}
	if len(client.request.Inputs) != 4 {
		t.Fatalf("protocol input count = %d, want ordered prompt/image/image/video inputs", len(client.request.Inputs))
	}
	want := []struct {
		slot, modality, mediaType, content string
	}{
		{slot: "prompt", modality: string(models.ModalityText), mediaType: "text/plain", content: "compare these"},
		{slot: "image", modality: string(models.ModalityImage), mediaType: "image/png", content: "PNG-A"},
		{slot: "image", modality: string(models.ModalityImage), mediaType: "image/png", content: "PNG-A"},
		{slot: "video", modality: string(models.ModalityVideo), mediaType: "video/mp4", content: "MP4-CLIP"},
	}
	for index, expected := range want {
		got := client.request.Inputs[index]
		if got.Slot != expected.slot || string(got.Modality) != expected.modality || got.MediaType != expected.mediaType || got.Content != expected.content {
			t.Fatalf("protocol input[%d] = slot=%q modality=%q mediaType=%q bytes=%d, want slot=%q modality=%q mediaType=%q bytes=%d", index, got.Slot, got.Modality, got.MediaType, len([]byte(got.Content)), expected.slot, expected.modality, expected.mediaType, len([]byte(expected.content)))
		}
	}
}

func TestNewInvocationRuntimeUsesFailClosedFallbackForNonOmni(t *testing.T) {
	t.Parallel()

	runtime := newInvocationRuntime(nil, nil)
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationTTS)
	if !ok {
		t.Fatal("GenericOperationContract(TTS) = false")
	}
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Operation: models.OperationTTS,
			Inputs:    []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "hello"}},
		},
		Operation: operation,
	})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassConfiguration {
		t.Fatalf("non-OMNI Invoke error = %v, failure = %#v, want typed configuration failure", err, failure)
	}
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("non-OMNI Invoke error = %v, want ErrUnavailable cause", err)
	}
	if len(result.Content) != 0 || len(result.Artifacts) != 0 {
		t.Fatalf("non-OMNI content = %#v artifacts = %#v, want no output", result.Content, result.Artifacts)
	}
}

func TestInferenceRuntimeFailsClosedWithoutProductionAdapters(t *testing.T) {
	t.Parallel()

	runtime, err := inferenceRuntime(invocationRuntimeOptions{})
	if err != nil {
		t.Fatalf("inferenceRuntime: %v", err)
	}
	composed, ok := runtime.(operationInvocationRuntime)
	if !ok {
		t.Fatalf("inferenceRuntime type = %T, want operationInvocationRuntime", runtime)
	}
	if _, ok := composed.generic.(failClosedInvocationRuntime); !ok {
		t.Fatalf("production default generic runtime = %T, want fail-closed runtime", composed.generic)
	}

	catalog := models.GenericOperationCatalog{}
	for _, operationName := range []string{models.OperationTTS, models.OperationASR, models.OperationEMBED} {
		t.Run(operationName, func(t *testing.T) {
			operation, ok := catalog.GenericOperationContract(operationName)
			if !ok {
				t.Fatalf("GenericOperationContract(%q) = false", operationName)
			}
			result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
				Request: models.InvokeModelRequest{
					Model: models.ModelReference{NameOrURI: "fixture-model"}, Operation: operationName,
					Inputs: []models.InferenceInput{{Name: "input", Content: "must not be echoed"}},
				},
				Operation: operation,
			})
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassConfiguration {
				t.Fatalf("Invoke error = %v, failure = %#v, want typed configuration failure", err, failure)
			}
			if !errors.Is(err, models.ErrUnavailable) {
				t.Fatalf("Invoke error = %v, want ErrUnavailable cause", err)
			}
			if len(result.Content) != 0 || len(result.Artifacts) != 0 {
				t.Fatalf("Invoke result = %#v, want no partial output", result)
			}
		})
	}
}

func TestInferenceRuntimeRoutesOmniBeforeGenericBackend(t *testing.T) {
	t.Parallel()

	scope := mustRoutingScope(t)
	genericCalls := 0
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Backend: func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
			genericCalls++
			return []models.InferenceContent{{Name: "generic", Content: "generic answer"}}, nil, nil
		},
		Client: &recordingInvocationProtocolClient{response: models.InvocationProtocolResponse{Text: "omni answer"}},
	})
	if err != nil {
		t.Fatalf("inferenceRuntime: %v", err)
	}
	catalog := models.GenericOperationCatalog{}
	omni, _ := catalog.GenericOperationContract(models.OperationOMNI)
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request:   models.InvokeModelRequest{Scope: scope, Holder: "routing-test", Model: models.ModelReference{NameOrURI: "llm"}, Operation: models.OperationOMNI, Inputs: []models.InferenceInput{{Name: "prompt", Modality: models.ModalityText, Content: "hello"}}},
		Operation: omni,
	})
	if err != nil || len(result.Content) != 1 || result.Content[0].Content != "omni answer" {
		t.Fatalf("OMNI route = result:%#v error:%v", result, err)
	}
	if genericCalls != 0 {
		t.Fatalf("generic calls after OMNI = %d, want 0", genericCalls)
	}

	tts, _ := catalog.GenericOperationContract(models.OperationTTS)
	result, err = runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request:   models.InvokeModelRequest{Scope: scope, Holder: "routing-test", Model: models.ModelReference{NameOrURI: "tts"}, Operation: models.OperationTTS, Inputs: []models.InferenceInput{{Name: "text", Modality: models.ModalityText, Content: "speak"}}},
		Operation: tts,
	})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassConfiguration || !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("TTS route = result:%#v error:%v failure:%#v, want fail-closed configuration failure", result, err, failure)
	}
	if len(result.Content) != 0 || len(result.Artifacts) != 0 {
		t.Fatalf("TTS route = result:%#v, want no output", result)
	}
	if genericCalls != 0 {
		t.Fatalf("generic calls after TTS = %d, want 0", genericCalls)
	}
}

func TestInferenceRuntimeRoutesASRBeforeGenericBackend(t *testing.T) {
	t.Parallel()

	scope := mustRoutingScope(t)
	asrCalls := 0
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Backend: func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
			return []models.InferenceContent{{Name: "generic"}}, nil, nil
		},
		ASR: func(context.Context, models.ASRBackendRequest) (models.ASRBackendResponse, error) {
			asrCalls++
			return models.ASRBackendResponse{
				Text:     "transcript",
				Segments: []models.ASRBackendSegment{{ID: 1, Start: 0, End: 100, Text: "transcript"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("inferenceRuntime: %v", err)
	}
	asr, _ := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationASR)
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{Scope: scope, Holder: "routing-test", Model: models.ModelReference{NameOrURI: "asr"}, Operation: models.OperationASR, Inputs: []models.InferenceInput{
			{Name: "audio", Modality: models.ModalityAudio, ContentType: "audio/wav", MediaType: "audio/wav", Content: string([]byte{0, 1, 2})},
		}},
		Operation: asr,
	})
	if err != nil || len(result.Content) != 2 || result.Content[0].Name != "transcript" || result.Content[1].Name != "segments" {
		t.Fatalf("ASR route = result:%#v error:%v", result, err)
	}
	if asrCalls != 1 {
		t.Fatalf("ASR calls = %d, want 1", asrCalls)
	}
}

func mustRoutingScope(t *testing.T) models.RuntimeScopeRef {
	t.Helper()
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:runtime-routing")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	return scope
}

func TestInvocationProtocolAdapterPreservesFailureAndOperationFallback(t *testing.T) {
	t.Parallel()

	if !isOMNIOperation(inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{Operation: models.OperationOMNI}}) {
		t.Fatal("isOMNIOperation should inspect the request operation when the catalog operation is empty")
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:protocol-error")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	client := &recordingInvocationProtocolClient{err: errors.New("fixture protocol failure")}
	runtime := newInvocationRuntime(client, nil)
	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request:   models.InvokeModelRequest{Scope: scope, Holder: "protocol-error", Model: models.ModelReference{NameOrURI: "llm"}, Operation: models.OperationOMNI, Inputs: []models.InferenceInput{{Name: "prompt", Modality: models.ModalityText, Content: "hello"}}},
		Operation: models.Operation{},
	})
	if result.Content != nil || err == nil || !strings.Contains(err.Error(), "fixture protocol failure") {
		t.Fatalf("protocol failure = result:%#v error:%v, want typed failure preserving cause", result, err)
	}
}

func TestNewServiceWithInvocationProtocolConstructsRoot(t *testing.T) {
	t.Parallel()

	service, err := validConstructionEdges().newServiceWithInvocationProtocol(&recordingInvocationProtocolClient{})
	if err != nil {
		t.Fatalf("NewServiceWithBackendArtifactResolverAndInvocationProtocolAndDialer: %v", err)
	}
	if service == nil {
		t.Fatal("NewServiceWithBackendArtifactResolverAndInvocationProtocolAndDialer returned nil service")
	}
}

func TestPinnedHostProtocolNegotiatorWrapperRejectsNilDialer(t *testing.T) {
	t.Parallel()

	if negotiator := NewPinnedGRPCHostProtocolNegotiator(nil); negotiator != nil {
		t.Fatalf("NewPinnedGRPCHostProtocolNegotiator(nil) = %T, want nil", negotiator)
	}
}

func TestRevisionResolverAdapterSelectsOnlyFirstOverride(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"absent", "nil-first", "success", "failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			wantErr := errors.New("selected revision resolver error")
			calls := 0
			resolvers := []func(context.Context, string) (string, error){func(got context.Context, source string) (string, error) {
				calls++
				if got != ctx || source != "hf://selected/repository@main" {
					t.Fatalf("revision context/source = %v/%q, want selected request", got, source)
				}
				if mode == "failure" {
					return "selected-result", wantErr
				}
				return "selected-result", nil
			}, func(context.Context, string) (string, error) {
				t.Fatal("used second revision override")
				return "", nil
			}}
			if mode == "absent" {
				resolvers = nil
			} else if mode == "nil-first" {
				resolvers[0] = nil
			}
			selected := firstRevisionResolver(resolvers)
			if mode == "absent" || mode == "nil-first" {
				if selected != nil || calls != 0 {
					t.Fatal("absent first override must remain absent")
				}
				return
			}
			result, err := selected(ctx, "hf://selected/repository@main")
			if mode == "success" {
				wantErr = nil
			}
			if result != "selected-result" || err != wantErr || calls != 1 {
				t.Fatalf("selected resolver = %q, %v, calls=%d, want unchanged result/error once", result, err, calls)
			}
		})
	}
}

func TestPinnedHostProtocolWrapperForwardsSelectedSymlinkResolver(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"absent", "nil-first", "identity", "escape", "failure", "missing"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			configuration := symlinkNegotiationConfiguration(t)
			root := filepath.Dir(configuration.ModelPath)
			calls := []string{}
			resolver := modelseffects.HostResolveSymlinks(func(path string) (string, error) {
				calls = append(calls, path)
				switch mode {
				case "escape":
					if path != root {
						return filepath.Join(filepath.Dir(root), "escaped.gguf"), nil
					}
				case "failure":
					return "", errors.New("controlled symlink failure")
				case "missing":
					return "", os.ErrNotExist
				}
				return path, nil
			})
			resolvers := []modelseffects.HostResolveSymlinks{resolver, func(string) (string, error) {
				t.Fatal("negotiation used the second symlink override")
				return "", nil
			}}
			if mode == "absent" {
				resolvers = nil
			} else if mode == "nil-first" {
				resolvers[0] = nil
			}
			connection := &symlinkNegotiationConnection{t: t}
			ctx := t.Context()
			dialer := symlinkNegotiationDialer(func(got context.Context, endpoint string) (platformgrpc.Connection, error) {
				if got != ctx || endpoint != "selected:50051" {
					t.Fatalf("dial context/endpoint = %v/%q, want selected values", got, endpoint)
				}
				return connection, nil
			})
			result, err := NewPinnedGRPCHostProtocolNegotiator(dialer, resolvers...).Negotiate(ctx, "selected:50051", modelseffects.HostProtocolNegotiationRequest{Configuration: configuration})
			assertSymlinkNegotiationResult(t, mode, configuration, result, err, connection, calls)
		})
	}
}

func symlinkNegotiationConfiguration(t *testing.T) modelseffects.ResolvedHostConfiguration {
	t.Helper()
	manifest, err := modelartifacts.DefaultModelRoleManifest()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := manifest.Model(models.BuiltInModelNameTTS)
	if !ok {
		t.Fatal("TTS role manifest missing")
	}
	root := t.TempDir()
	files := make([]string, 0, len(definition.Artifacts))
	for _, artifact := range definition.Artifacts {
		path := filepath.Join(root, artifact.Path)
		if err := os.WriteFile(path, []byte("controlled role"), 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	return modelseffects.ResolvedHostConfiguration{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         "localai-vibevoice", ModelName: models.BuiltInModelNameTTS,
		Revision: definition.Publication.Revision, ModelPath: files[0], ModelFiles: files,
	}
}

func assertSymlinkNegotiationResult(t *testing.T, mode string, configuration modelseffects.ResolvedHostConfiguration, result modelseffects.HostProtocolNegotiationResult, err error, connection *symlinkNegotiationConnection, calls []string) {
	t.Helper()
	if connection.closed != 1 || connection.health != 1 {
		t.Fatalf("health/close = %d/%d, want one each", connection.health, connection.closed)
	}
	if mode == "escape" || mode == "failure" {
		if !errors.Is(err, models.ErrHostProtocolIncompatible) || result.Ready || connection.load != nil || len(calls) == 0 {
			t.Fatalf("invalid resolved layout = %#v, %v, load=%v calls=%v; want incompatible before load", result, err, connection.load, calls)
		}
		return
	}
	wantResult := modelseffects.HostProtocolNegotiationResult{Ready: true, Backend: configuration.Backend, ProtocolVersion: configuration.ProtocolVersion}
	if err != nil || result != wantResult || connection.load == nil {
		t.Fatalf("negotiation = %#v, %v, load=%v; want selected ready backend", result, err, connection.load)
	}
	wantOptions := []string{"tokenizer=" + configuration.ModelFiles[1], "voice=" + configuration.ModelFiles[2]}
	if connection.load.ModelFile != configuration.ModelPath || !reflect.DeepEqual(connection.load.Options, wantOptions) {
		t.Fatalf("load = %#v, want selected model and role options %v", connection.load, wantOptions)
	}
	assertSelectedSymlinkCalls(t, mode, configuration, calls)
}

func assertSelectedSymlinkCalls(t *testing.T, mode string, configuration modelseffects.ResolvedHostConfiguration, calls []string) {
	t.Helper()
	wantCalls := []string{}
	if mode == "identity" || mode == "missing" {
		for _, path := range configuration.ModelFiles {
			wantCalls = append(wantCalls, filepath.Dir(configuration.ModelPath))
			if mode == "identity" {
				wantCalls = append(wantCalls, path)
			}
		}
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("resolver calls = %v, want %v", calls, wantCalls)
	}
}

type symlinkNegotiationConnection struct {
	t              *testing.T
	health, closed int
	load           *localai.ModelOptions
}

type symlinkNegotiationDialer func(context.Context, string) (platformgrpc.Connection, error)

func (dialer symlinkNegotiationDialer) Dial(ctx context.Context, endpoint string) (platformgrpc.Connection, error) {
	return dialer(ctx, endpoint)
}

func (connection *symlinkNegotiationConnection) Invoke(_ context.Context, method string, payload []byte) ([]byte, error) {
	if strings.HasSuffix(method, "/Health") {
		connection.health++
	} else if strings.HasSuffix(method, "/LoadModel") {
		connection.load = &localai.ModelOptions{}
		if err := proto.Unmarshal(payload, connection.load); err != nil {
			connection.t.Fatal(err)
		}
	} else {
		connection.t.Fatalf("unexpected protocol method %q", method)
	}
	return proto.Marshal(&localai.Result{Success: true})
}

func (connection *symlinkNegotiationConnection) Close() error {
	connection.closed++
	return nil
}

func TestModelsConstructionRejectsMissingFunctionEffects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		remove func(*constructionEdges)
	}{
		{"asset make-directories effect", func(e *constructionEdges) { e.assetMkdirAll = nil }},
		{"asset inspect-path effect", func(e *constructionEdges) { e.assetStat = nil }},
		{"asset resolve-home effect", func(e *constructionEdges) { e.assetHome = nil }},
		{"asset write-file effect", func(e *constructionEdges) { e.assetWriteFile = nil }},
		{"asset rename-path effect", func(e *constructionEdges) { e.assetRename = nil }},
		{"asset remove-path effect", func(e *constructionEdges) { e.assetRemove = nil }},
		{"asset read-file effect", func(e *constructionEdges) { e.assetReadFile = nil }},
		{"asset read-directory effect", func(e *constructionEdges) { e.assetReadDir = nil }},
		{"asset create-file effect", func(e *constructionEdges) { e.assetCreate = nil }},
		{"asset open-file effect", func(e *constructionEdges) { e.assetOpen = nil }},
		{"model runtime file inspector", func(e *constructionEdges) { e.runtimeInspect = nil }},
		{"model runtime temporary directory resolver", func(e *constructionEdges) { e.runtimeTempDir = nil }},
		{"model runtime temporary file creator", func(e *constructionEdges) { e.runtimeTempFile = nil }},
		{"process clock", func(e *constructionEdges) { e.now = nil }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			edges := validConstructionEdges()
			test.remove(&edges)
			service, err := edges.newServiceWithInvocationProtocol(nil)
			want := "construct Models: " + test.name + " is required"
			if service != nil || err == nil || err.Error() != want {
				t.Fatalf("construction = (%T, %v), want nil service and %q", service, err, want)
			}
		})
	}
}

func TestModelsConstructionRejectsNilAndTypedNilRequiredEffects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		remove func(*constructionEdges, bool)
	}{
		{"asset HTTP client", func(e *constructionEdges, typed bool) {
			e.assetHTTP = nil
			if typed {
				e.assetHTTP = (*recordingHTTPDoer)(nil)
			}
		}},
		{"model host HTTP client", func(e *constructionEdges, typed bool) {
			e.hostHTTP = nil
			if typed {
				e.hostHTTP = (*recordingHTTPDoer)(nil)
			}
		}},
		{"model runtime HTTP client", func(e *constructionEdges, typed bool) {
			e.runtimeHTTP = nil
			if typed {
				e.runtimeHTTP = (*recordingHTTPDoer)(nil)
			}
		}},
		{"model host process launcher", func(e *constructionEdges, typed bool) {
			e.processLauncher = nil
			if typed {
				e.processLauncher = (*recordingProcessLauncher)(nil)
			}
		}},
		{"model host clock", func(e *constructionEdges, typed bool) {
			e.hostClock = nil
			if typed {
				e.hostClock = (*recordingHostClock)(nil)
			}
		}},
		{"model runtime command runner", func(e *constructionEdges, typed bool) {
			e.runtimeRunner = nil
			if typed {
				e.runtimeRunner = (*recordingCommandRunner)(nil)
			}
		}},
	}
	for _, test := range cases {
		for _, typed := range []bool{false, true} {
			name := "nil/"
			if typed {
				name = "typed-nil/"
			}
			t.Run(name+test.name, func(t *testing.T) {
				t.Parallel()
				edges := validConstructionEdges()
				test.remove(&edges, typed)
				service, err := edges.newServiceWithInvocationProtocol(nil)
				want := "construct Models: " + test.name + " is required"
				if service != nil || err == nil || err.Error() != want {
					t.Fatalf("construction = (%T, %v), want nil service and %q", service, err, want)
				}
			})
		}
	}
}

func TestModelsConstructionAllowsAbsentProtocolOverride(t *testing.T) {
	t.Parallel()
	service, err := validConstructionEdges().newServiceWithInvocationProtocol(nil)
	if err != nil || service == nil {
		t.Fatalf("construction without optional protocol = (%T, %v), want inert service", service, err)
	}
}

func TestModelsConstructionPreservesIssuerEntropyFailure(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("controlled issuer entropy failure")
	edges := validConstructionEdges()
	edges.issuerEntropy = platformrandom.SourceFunc(func(bound int64) (int64, error) {
		if bound != 256 {
			t.Fatalf("issuer entropy bound = %d, want byte bound 256", bound)
		}
		return 0, wantErr
	})
	service, err := edges.newServiceWithInvocationProtocol(nil)
	if service != nil || !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "construct Models Runtime Scopes issuer identity") {
		t.Fatalf("construction = (%T, %v), want nil service and preserved issuer failure", service, err)
	}
}
