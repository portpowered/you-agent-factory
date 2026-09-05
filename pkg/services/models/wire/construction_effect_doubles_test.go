package wire

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type constructionEdges struct {
	assetPlatform     models.AssetHostPlatform
	assetHTTP         modelseffects.AssetHTTPDoer
	assetEndpoints    models.RuntimeAssetEndpoints
	assetMkdirAll     modelseffects.AssetMakeDirectories
	assetStat         modelseffects.AssetInspectPath
	assetHome         modelseffects.AssetResolveHomeDirectory
	assetWriteFile    modelseffects.AssetWriteFile
	assetRename       modelseffects.AssetRenamePath
	assetRemove       modelseffects.AssetRemovePath
	assetReadFile     modelseffects.AssetReadFile
	assetReadDir      modelseffects.AssetReadDirectory
	assetCreate       modelseffects.AssetCreateFile
	assetOpen         modelseffects.AssetOpenFile
	assetCoordination modelseffects.AssetStagingCoordination
	processLauncher   modelseffects.HostProcessLauncher
	hostHTTP          modelseffects.HostHTTPDoer
	hostClock         modelseffects.HostClock
	runtimeRunner     platformprocess.CommandRunner
	runtimeHTTP       modelseffects.RuntimeHTTPDoer
	runtimeInspect    modelseffects.RuntimeInspectFile
	runtimeTempDir    modelseffects.RuntimeTempDirectory
	runtimeTempFile   modelseffects.RuntimeCreateTempFile
	now               func() time.Time
	issuerEntropy     platformrandom.Source
}

func validConstructionEdges() constructionEdges {
	assetCoordination, err := platformlocking.New(platformlocking.LocalFileSystem{})
	if err != nil {
		panic(err)
	}
	return constructionEdges{
		assetPlatform: models.AssetHostPlatform{
			OperatingSystem: runtime.GOOS,
			Architecture:    runtime.GOARCH,
		},
		assetHTTP:         http.DefaultClient,
		assetMkdirAll:     os.MkdirAll,
		assetStat:         os.Stat,
		assetHome:         os.UserHomeDir,
		assetWriteFile:    os.WriteFile,
		assetRename:       os.Rename,
		assetRemove:       os.Remove,
		assetReadFile:     os.ReadFile,
		assetReadDir:      os.ReadDir,
		assetCreate:       func(path string) (io.WriteCloser, error) { return os.Create(path) },
		assetOpen:         func(path string) (io.ReadCloser, error) { return os.Open(path) },
		assetCoordination: assetCoordination,
		processLauncher:   inertProcessLauncher{},
		hostHTTP:          http.DefaultClient,
		hostClock:         inertHostClock{},
		runtimeRunner:     inertCommandRunner{},
		runtimeHTTP:       http.DefaultClient,
		runtimeInspect:    os.Stat,
		runtimeTempDir:    os.TempDir,
		runtimeTempFile: func(dir, pattern string) (modelseffects.RuntimeTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		now:           func() time.Time { return time.Unix(123, 456) },
		issuerEntropy: platformrandom.CryptoSource{},
	}
}

func (edges constructionEdges) newServiceWithInvocationProtocol(
	client InvocationProtocolClient,
) (models.Service, error) {
	return NewServiceWithBackendArtifactResolverAndInvocationProtocolAndDialer(
		edges.assetPlatform,
		edges.assetHTTP,
		edges.assetEndpoints,
		edges.assetMkdirAll,
		edges.assetStat,
		edges.assetHome,
		edges.assetWriteFile,
		edges.assetRename,
		edges.assetRemove,
		edges.assetReadFile,
		edges.assetReadDir,
		edges.assetCreate,
		edges.assetOpen,
		edges.processLauncher,
		edges.hostHTTP,
		edges.hostClock,
		edges.runtimeRunner,
		edges.runtimeHTTP,
		edges.runtimeInspect,
		edges.runtimeTempDir,
		edges.runtimeTempFile,
		zap.NewNop(),
		edges.now,
		edges.issuerEntropy,
		nil,
		nil,
		nil,
		modelseffects.LocalRuntimeHooks{},
		nil,
		nil,
		nil,
		edges.assetCoordination,
		nil,
		client,
		nil,
		nil,
		nil,
		nil,
	)
}

type inertProcessLauncher struct{}

func (inertProcessLauncher) Start(
	context.Context,
	modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	panic("process launcher called during readiness inspection")
}

type inertHostClock struct{}

func (inertHostClock) Now() time.Time {
	return time.Unix(0, 0)
}

func (inertHostClock) NewTimer(time.Duration) modelseffects.HostTimer {
	panic("host timer created during readiness inspection")
}

type inertCommandRunner struct{}

func (inertCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	panic("runtime command called during readiness inspection")
}

type recordingHTTPDoer struct {
	name  string
	calls int
}

func (doer *recordingHTTPDoer) Do(*http.Request) (*http.Response, error) {
	doer.calls++
	panic(doer.name + " client invoked during inert construction")
}

type recordingProcessLauncher struct{ starts int }

func (launcher *recordingProcessLauncher) Start(
	context.Context,
	modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.starts++
	panic("process launcher invoked during inert construction")
}

type recordingHostClock struct {
	nowCalls   int
	timerCalls int
}

func (clock *recordingHostClock) Now() time.Time {
	clock.nowCalls++
	panic("host clock invoked during inert construction")
}

func (clock *recordingHostClock) NewTimer(time.Duration) modelseffects.HostTimer {
	clock.timerCalls++
	panic("host timer created during inert construction")
}

type recordingCommandRunner struct{ calls int }

func (runner *recordingCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls++
	panic("runtime command runner invoked during inert construction")
}

type recordingAssetMkdirAll struct{ calls int }

func (effect *recordingAssetMkdirAll) mkdirAll(string, os.FileMode) error {
	effect.calls++
	panic("asset mkdir invoked during inert construction")
}

type recordingAssetStat struct{ calls int }

func (effect *recordingAssetStat) stat(string) (os.FileInfo, error) {
	effect.calls++
	panic("asset stat invoked during inert construction")
}

type recordingAssetHome struct{ calls int }

func (effect *recordingAssetHome) home() (string, error) {
	effect.calls++
	panic("asset home invoked during inert construction")
}

type recordingAssetWriteFile struct{ calls int }

func (effect *recordingAssetWriteFile) write(string, []byte, os.FileMode) error {
	effect.calls++
	panic("asset write invoked during inert construction")
}

type recordingAssetRename struct{ calls int }

func (effect *recordingAssetRename) rename(string, string) error {
	effect.calls++
	panic("asset rename invoked during inert construction")
}

type recordingAssetRemove struct{ calls int }

func (effect *recordingAssetRemove) remove(string) error {
	effect.calls++
	panic("asset remove invoked during inert construction")
}

type recordingAssetReadFile struct{ calls int }

func (effect *recordingAssetReadFile) read(string) ([]byte, error) {
	effect.calls++
	panic("asset read file invoked during inert construction")
}

type recordingAssetReadDir struct{ calls int }

func (effect *recordingAssetReadDir) readDir(string) ([]os.DirEntry, error) {
	effect.calls++
	panic("asset read dir invoked during inert construction")
}

type recordingAssetCreate struct{ calls int }

func (effect *recordingAssetCreate) create(string) (io.WriteCloser, error) {
	effect.calls++
	panic("asset create invoked during inert construction")
}

type recordingAssetOpen struct{ calls int }

func (effect *recordingAssetOpen) open(string) (io.ReadCloser, error) {
	effect.calls++
	panic("asset open invoked during inert construction")
}

type recordingRuntimeInspect struct{ calls int }

func (effect *recordingRuntimeInspect) inspect(string) (os.FileInfo, error) {
	effect.calls++
	panic("runtime inspect invoked during inert construction")
}

type recordingRuntimeTempDir struct{ calls int }

func (effect *recordingRuntimeTempDir) tempDir() string {
	effect.calls++
	panic("runtime temp dir invoked during inert construction")
}

type recordingRuntimeTempFile struct{ calls int }

func (effect *recordingRuntimeTempFile) create(string, string) (modelseffects.RuntimeTempFile, error) {
	effect.calls++
	panic("runtime temp file invoked during inert construction")
}

type recordingProcessClock struct{ calls int }

func (clock *recordingProcessClock) now() time.Time {
	clock.calls++
	panic("process clock invoked during inert construction")
}

func TestBackendInvocationRuntimeMapsContentAndArtifacts(t *testing.T) {
	t.Parallel()

	artifactRef, err := (models.InferenceArtifactRef{}).Parse("artifact://fixture-output")
	if err != nil {
		t.Fatalf("parse artifact reference: %v", err)
	}
	request := models.InvokeModelRequest{Operation: models.OperationEMBED}
	var received models.InvokeModelRequest
	runtime := backendInvocationRuntime{backend: func(
		_ context.Context,
		request models.InvokeModelRequest,
	) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		received = request
		return []models.InferenceContent{{Name: "embedding", Content: "[1,2]"}}, []models.InferenceArtifact{{
			Artifact: artifactRef, Name: "fixture-output", MediaType: "application/octet-stream",
			SizeBytes: 2, Properties: map[string]string{"source": "fixture"},
		}}, nil
	}}

	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{Request: request})
	if err != nil {
		t.Fatalf("backendInvocationRuntime.Invoke() error = %v", err)
	}
	if received.Operation != request.Operation {
		t.Fatalf("backend request = %#v, want operation %q", received, request.Operation)
	}
	if len(result.Content) != 1 || result.Content[0].Name != "embedding" {
		t.Fatalf("runtime content = %#v, want embedding output", result.Content)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].RefValue != artifactRef.String() ||
		result.Artifacts[0].MediaType != "application/octet-stream" {
		t.Fatalf("runtime artifacts = %#v, want mapped fixture artifact", result.Artifacts)
	}

	wantErr := errors.New("fixture backend failed")
	runtime.backend = func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		return nil, nil, wantErr
	}
	if _, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{}); !errors.Is(err, wantErr) {
		t.Fatalf("backend error = %v, want %v", err, wantErr)
	}
}

func TestOperationInvocationRuntimeSelectsASROnlyForASROperations(t *testing.T) {
	t.Parallel()

	generic := &recordingInvocationRuntime{result: inference.InvocationRuntimeResult{Content: []models.InferenceContent{{Content: "generic"}}}}
	asr := &recordingInvocationRuntime{result: inference.InvocationRuntimeResult{Content: []models.InferenceContent{{Content: "asr"}}}}
	runtime := operationInvocationRuntime{generic: generic, asr: asr}

	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{Operation: models.OperationASR},
	})
	if err != nil || len(result.Content) != 1 || result.Content[0].Content != "asr" {
		t.Fatalf("ASR runtime result = (%#v, %v), want ASR result", result, err)
	}
	if asr.calls != 1 || generic.calls != 0 {
		t.Fatalf("runtime calls after ASR = generic:%d asr:%d, want generic:0 asr:1", generic.calls, asr.calls)
	}

	result, err = runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request:   models.InvokeModelRequest{Operation: "embed"},
		Operation: models.Operation{Name: models.OperationEMBED},
	})
	if err != nil || len(result.Content) != 1 || result.Content[0].Content != "generic" {
		t.Fatalf("generic runtime result = (%#v, %v), want generic result", result, err)
	}
	if generic.calls != 1 || asr.calls != 1 {
		t.Fatalf("runtime calls after generic = generic:%d asr:%d, want generic:1 asr:1", generic.calls, asr.calls)
	}

	result, err = runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{}, Operation: models.Operation{Name: models.OperationASR},
	})
	if err != nil || len(result.Content) != 1 || result.Content[0].Content != "asr" {
		t.Fatalf("operation-only ASR result = (%#v, %v), want ASR result", result, err)
	}
}

func TestInferenceRuntimeUsesASRBackendAndMapsResponse(t *testing.T) {
	t.Parallel()

	artifactRef, err := (models.InferenceArtifactRef{}).Parse("artifact://transcript")
	if err != nil {
		t.Fatalf("parse artifact reference: %v", err)
	}
	var received models.ASRBackendRequest
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Backend: func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
			return []models.InferenceContent{{Content: "generic"}}, nil, nil
		},
		ASR: func(_ context.Context, request models.ASRBackendRequest) (models.ASRBackendResponse, error) {
			received = request
			return models.ASRBackendResponse{
				Text:      "hello fixture",
				Segments:  []models.ASRBackendSegment{{ID: 1, Start: 0, End: 1000, Text: "hello fixture"}},
				Artifacts: []models.InferenceArtifact{{Artifact: artifactRef, Name: "transcript", MediaType: "audio/wav"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("inferenceRuntime() error = %v", err)
	}

	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{
		Operation: models.OperationASR,
		Inputs: []models.InferenceInput{{
			Name: "audio", Modality: models.ModalityAudio, MediaType: "audio/wav", Content: "wav fixture",
		}},
		Parameters: []models.OperationParameter{{Name: "temperature", Value: 0.2}},
	}})
	if err != nil {
		t.Fatalf("ASR runtime Invoke() error = %v", err)
	}
	if string(received.Audio) != "wav fixture" || received.MediaType != "audio/wav" {
		t.Fatalf("ASR backend request = %#v, want detached audio request", received)
	}
	if received.Parameters["temperature"] != 0.2 {
		t.Fatalf("ASR backend parameters = %#v, want temperature", received.Parameters)
	}
	if len(result.Content) != 2 || result.Content[0].Name != "transcript" || result.Content[1].Name != "segments" {
		t.Fatalf("ASR runtime content = %#v, want transcript and segments", result.Content)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].RefValue != artifactRef.String() {
		t.Fatalf("ASR runtime artifacts = %#v, want transcript artifact", result.Artifacts)
	}

	result, err = runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{Request: models.InvokeModelRequest{Operation: models.OperationEMBED}})
	if err != nil || len(result.Content) != 1 || result.Content[0].Content != "generic" {
		t.Fatalf("generic runtime result = (%#v, %v), want generic output", result, err)
	}
}

func TestInferenceRuntimeUsesPinnedEmbeddingBackendWhenEdgeIsAbsent(t *testing.T) {
	t.Parallel()

	response, err := proto.Marshal(&localai.EmbeddingResult{Embeddings: []float32{0.1, -0.2, 0.3}})
	if err != nil {
		t.Fatalf("marshal embedding response: %v", err)
	}
	connection := &embeddingRuntimeConnection{response: response}
	dialer := &embeddingRuntimeDialer{connection: connection}
	runtime, err := inferenceRuntime(invocationRuntimeOptions{Dialer: dialer})
	if err != nil {
		t.Fatalf("inferenceRuntime() error = %v", err)
	}

	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Operation: models.OperationEMBED,
			Inputs: []models.InferenceInput{{
				Name: "text", Modality: models.ModalityText,
				ContentType: "text/plain", MediaType: "text/plain", Content: "Find similar work",
			}},
		},
		Operation: models.Operation{Name: models.OperationEMBED},
		HostSlot:  inference.HostHandleSlot{Endpoint: "grpc://127.0.0.1:50051"},
	})
	if err != nil {
		t.Fatalf("embedding runtime Invoke() error = %v", err)
	}
	assertPinnedEmbeddingResult(t, result)
	assertPinnedEmbeddingTransport(t, dialer, connection)
	assertPinnedEmbeddingRequest(t, connection)
}

func TestInferenceRuntimePreservesExplicitGenericBackendOverPinnedEmbedding(t *testing.T) {
	t.Parallel()

	connection := &embeddingRuntimeConnection{}
	dialer := &embeddingRuntimeDialer{connection: connection}
	genericCalls := 0
	runtime, err := inferenceRuntime(invocationRuntimeOptions{
		Backend: func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
			genericCalls++
			return []models.InferenceContent{{
				Name: "embedding", Modality: models.ModalityJSON,
				ContentType: "application/json", MediaType: "application/json", Content: "[0.25]",
			}}, nil, nil
		},
		Dialer: dialer,
	})
	if err != nil {
		t.Fatalf("inferenceRuntime() error = %v", err)
	}

	result, err := runtime.Invoke(context.Background(), inference.InvocationRuntimeRequest{
		Request: models.InvokeModelRequest{
			Operation: models.OperationEMBED,
			Inputs: []models.InferenceInput{{
				Name: "text", Modality: models.ModalityText,
				ContentType: "text/plain", MediaType: "text/plain", Content: "fixture text",
			}},
		},
		Operation: models.Operation{Name: models.OperationEMBED},
	})
	if err != nil {
		t.Fatalf("explicit generic EMBED error = %v", err)
	}
	if genericCalls != 1 || len(result.Content) != 1 || result.Content[0].Content != "[0.25]" {
		t.Fatalf("explicit generic EMBED result = calls:%d content:%#v, want one generic result", genericCalls, result.Content)
	}
	if dialer.endpoint != "" || connection.method != "" || connection.closed != 0 {
		t.Fatalf("pinned EMBED transport = endpoint:%q method:%q closed:%d, want unused", dialer.endpoint, connection.method, connection.closed)
	}
}

func assertPinnedEmbeddingResult(t *testing.T, result inference.InvocationRuntimeResult) {
	t.Helper()
	if len(result.Content) != 1 || result.Content[0].Name != "embedding" {
		t.Fatalf("embedding runtime content = %#v, want canonical vector output", result.Content)
	}
	var vector []float64
	if err := json.Unmarshal([]byte(result.Content[0].Content), &vector); err != nil {
		t.Fatalf("decode embedding runtime content: %v", err)
	}
	if len(vector) != 3 || vector[0] != float64(float32(0.1)) || vector[1] != float64(float32(-0.2)) {
		t.Fatalf("embedding runtime vector = %#v, want three pinned values", vector)
	}
}

func assertPinnedEmbeddingTransport(t *testing.T, dialer *embeddingRuntimeDialer, connection *embeddingRuntimeConnection) {
	t.Helper()
	if dialer.endpoint != "grpc://127.0.0.1:50051" || connection.method != "/backend.Backend/Embedding" || connection.closed != 1 {
		t.Fatalf("embedding runtime transport = endpoint:%q method:%q closed:%d, want selected endpoint/Embedding/one close", dialer.endpoint, connection.method, connection.closed)
	}
}

func assertPinnedEmbeddingRequest(t *testing.T, connection *embeddingRuntimeConnection) {
	t.Helper()
	var request localai.PredictOptions
	if err := proto.Unmarshal(connection.request, &request); err != nil {
		t.Fatalf("decode embedding request: %v", err)
	}
	if request.GetPrompt() != "Find similar work" || request.GetEmbeddings() != "Find similar work" {
		t.Fatalf("embedding request prompt = %q, embeddings = %q, want input text in both shared and dedicated fields", request.GetPrompt(), request.GetEmbeddings())
	}
}

func TestCloneASRParametersDetachesNestedValues(t *testing.T) {
	t.Parallel()

	original := map[string]any{
		"nested": map[string]any{"value": "original"},
		"values": []any{map[string]any{"value": "item"}},
	}
	cloned := cloneInvocationParameters(original)
	cloned["nested"].(map[string]any)["value"] = "changed"
	cloned["values"].([]any)[0].(map[string]any)["value"] = "changed"
	if original["nested"].(map[string]any)["value"] != "original" ||
		original["values"].([]any)[0].(map[string]any)["value"] != "item" {
		t.Fatalf("cloneInvocationParameters mutated original = %#v", original)
	}
}

type recordingInvocationRuntime struct {
	result inference.InvocationRuntimeResult
	err    error
	calls  int
}

type embeddingRuntimeDialer struct {
	connection *embeddingRuntimeConnection
	endpoint   string
}

func (dialer *embeddingRuntimeDialer) Dial(
	_ context.Context,
	endpoint string,
) (platformgrpc.Connection, error) {
	dialer.endpoint = endpoint
	return dialer.connection, nil
}

type embeddingRuntimeConnection struct {
	method   string
	request  []byte
	response []byte
	closed   int
}

func (connection *embeddingRuntimeConnection) Invoke(
	_ context.Context,
	method string,
	request []byte,
) ([]byte, error) {
	connection.method = method
	connection.request = append([]byte(nil), request...)
	return connection.response, nil
}

func (connection *embeddingRuntimeConnection) Close() error {
	connection.closed++
	return nil
}

func (runtime *recordingInvocationRuntime) Invoke(context.Context, inference.InvocationRuntimeRequest) (inference.InvocationRuntimeResult, error) {
	runtime.calls++
	return runtime.result, runtime.err
}
