package localai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestPinnedGRPCHostProtocolNegotiatorSelectsAudioCPPBackend(t *testing.T) {
	t.Parallel()
	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Result{Success: true})
	negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
	_, err := negotiator.Negotiate(context.Background(), "grpc://127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
		Configuration: modelseffects.ResolvedHostConfiguration{
			ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
			Backend:         "localai-audio-cpp", ModelName: "index-tts2.5",
			ModelPath: "/models/index-tts2_5-orig.gguf",
			Platform:  models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", Accelerator: "cuda"},
		},
	})
	if err != nil {
		t.Fatalf("Negotiate audio.cpp: %v", err)
	}
	if got := connection.loadRequest.GetOptions(); len(got) != 1 || got[0] != localAIAudioCPPBackendOption {
		t.Fatalf("audio.cpp LoadModel options = %v, want backend:best", got)
	}
}

func TestPinnedGRPCHostProtocolNegotiatorSendsWindowsCPUProjectorPlacementOption(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Result{Success: true, Message: "loaded"})
	modelFile := filepath.Join("models", "llm", "model.gguf")
	mmprojFile := filepath.Join("models", "llm", "mmproj-F16.gguf")
	negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
	result, err := negotiator.Negotiate(context.Background(), "grpc://127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
		Configuration: modelseffects.ResolvedHostConfiguration{
			ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
			Backend:         "localai-llamacpp",
			ModelName:       models.BuiltInModelNameLLM,
			Platform:        models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"},
			ModelPath:       modelFile,
			MMProjPath:      mmprojFile,
		},
	})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	assertWindowsCPUProjectorResult(t, result, connection)
	assertWindowsCPUProjectorRequest(t, &connection.loadRequest, modelFile, mmprojFile)
	assertWindowsCPUProjectorWire(t, connection.loadPayload, modelFile, mmprojFile)
}

func assertWindowsCPUProjectorResult(t *testing.T, result modelseffects.HostProtocolNegotiationResult, connection *recordingGRPCConnection) {
	t.Helper()
	if !result.Ready || !equalStrings(connection.methods, []string{localAIHealthMethod, localAILoadModelMethod}) || connection.closed != 1 {
		t.Fatalf("load transport facts = methods %#v, ready %t, closed %d, want Health/LoadModel/ready/one close", connection.methods, result.Ready, connection.closed)
	}
}

func assertWindowsCPUProjectorRequest(t *testing.T, request *ModelOptions, modelFile, mmprojFile string) {
	t.Helper()
	if request.GetModel() != models.BuiltInModelNameLLM || request.GetEmbeddings() ||
		request.GetModelFile() != modelFile || request.GetMMProj() != mmprojFile ||
		request.GetModelPath() != filepath.Dir(modelFile) || request.GetNBatch() != localAIModelBatchSize || request.GetThreads() != 4 {
		t.Fatalf("load request model=%q modelFile=%q mmproj=%q modelPath=%q nBatch=%d options=%v, want unchanged model, paths, directory, and batch size", request.GetModel(), request.GetModelFile(), request.GetMMProj(), request.GetModelPath(), request.GetNBatch(), request.GetOptions())
	}
	assertProjectorOption(t, request.GetOptions())
}

func assertWindowsCPUProjectorWire(t *testing.T, payload []byte, modelFile, mmprojFile string) {
	t.Helper()
	expected := appendStringField(nil, 1, models.BuiltInModelNameLLM)
	expected = appendVarintField(expected, 4, localAIModelBatchSize)
	expected = appendVarintField(expected, 15, 4)
	expected = appendStringField(expected, 21, modelFile)
	expected = appendStringField(expected, 41, mmprojFile)
	expected = appendStringField(expected, 59, filepath.Dir(modelFile))
	expected = appendStringField(expected, 62, localAIDisableProjectorGPUOption)
	if !bytes.Equal(payload, expected) {
		t.Fatalf("Windows CPU projector LoadModel wire bytes = %x, want exact field-62 option bytes %x", payload, expected)
	}
	decoded := decodeLoadModelPayload(t, payload)
	if decoded.GetModel() != models.BuiltInModelNameLLM || decoded.GetEmbeddings() ||
		decoded.GetModelFile() != modelFile || decoded.GetMMProj() != mmprojFile ||
		decoded.GetModelPath() != filepath.Dir(modelFile) || decoded.GetNBatch() != localAIModelBatchSize {
		t.Fatalf("decoded LoadModel = %#v, want unchanged fields and one colon-delimited option", decoded)
	}
	assertProjectorOption(t, decoded.GetOptions())
}

// LocalAI b224c96db6f4b87306a33a808650bfce63b12588
// backend/cpp/llama-cpp/grpc-server.cpp (SHA-256
// ab35d77710ca9664e7c18cdf6b2d3815dba1b5861dc3d046229826eb0ec212b0,
// lines 634-639 and 913-917) splits options at the first colon and
// consumes mmproj_use_gpu=false as the projector setting.
func assertProjectorOption(t *testing.T, options []string) {
	t.Helper()
	if len(options) != 1 {
		t.Fatalf("projector options = %q, want projector placement only", options)
	}
	optionName, optionValue, ok := strings.Cut(options[0], ":")
	if !ok || optionName != "mmproj_use_gpu" || optionValue != "false" {
		t.Fatalf("pinned LocalAI parser tokens = %q/%q/%t, want mmproj_use_gpu/false/true", optionName, optionValue, ok)
	}
}

type projectorPlacementCase struct {
	name       string
	backend    string
	modelName  string
	platform   models.AssetHostPlatform
	mmproj     string
	withMMProj bool
	embeddings bool
}

func TestPinnedGRPCHostProtocolNegotiatorConfinesProjectorPlacementOption(t *testing.T) {
	t.Parallel()

	tests := []projectorPlacementCase{
		{
			name:      "empty MMProj",
			backend:   "localai-llamacpp",
			modelName: models.BuiltInModelNameLLM,
			platform:  models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"},
		},
		{
			name:      "whitespace MMProj",
			backend:   "localai-llamacpp",
			modelName: models.BuiltInModelNameLLM,
			platform:  models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"},
			mmproj:    " \t ",
		},
		{
			name:       "non-Windows",
			backend:    "localai-llamacpp",
			modelName:  models.BuiltInModelNameLLM,
			platform:   models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
			withMMProj: true,
		},
		{
			name:       "non-amd64",
			backend:    "localai-llamacpp",
			modelName:  models.BuiltInModelNameLLM,
			platform:   models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "arm64"},
			withMMProj: true,
		},
		{
			name:       "non-llamacpp",
			backend:    "localai-whisper",
			modelName:  models.BuiltInModelNameLLM,
			platform:   models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"},
			withMMProj: true,
		},
		{
			name:       "non-LLM",
			backend:    "localai-llamacpp",
			modelName:  models.BuiltInModelNameEmbed,
			platform:   models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"},
			withMMProj: true,
			embeddings: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertProjectorPlacementOptionConfined(t, test)
		})
	}
}

func assertProjectorPlacementOptionConfined(t *testing.T, test projectorPlacementCase) {
	t.Helper()
	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Result{Success: true, Message: "loaded"})
	modelRoot := t.TempDir()
	modelFile := filepath.Join(modelRoot, "model.gguf")
	mmprojFile := test.mmproj
	if test.withMMProj {
		mmprojFile = filepath.Join(modelRoot, "mmproj-F16.gguf")
	}
	negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
	result, err := negotiator.Negotiate(context.Background(), "127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
		Configuration: modelseffects.ResolvedHostConfiguration{
			ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
			Backend:         test.backend,
			ModelName:       test.modelName,
			Platform:        test.platform,
			ModelPath:       modelFile,
			MMProjPath:      mmprojFile,
		},
	})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if !result.Ready || result.Backend != test.backend || connection.closed != 1 {
		t.Fatalf("Negotiate() result = %#v, closes = %d, want ready/backend:%q/one close", result, connection.closed, test.backend)
	}
	assertProjectorPlacementRequest(t, connection.loadPayload, test, modelFile, mmprojFile)
}

func assertProjectorPlacementRequest(t *testing.T, payload []byte, test projectorPlacementCase, modelFile, mmprojFile string) {
	t.Helper()
	decoded := decodeLoadModelPayload(t, payload)
	if decoded.GetModel() != test.modelName || decoded.GetEmbeddings() != test.embeddings ||
		decoded.GetModelFile() != modelFile || decoded.GetMMProj() != strings.TrimSpace(mmprojFile) ||
		decoded.GetModelPath() != filepath.Dir(modelFile) || decoded.GetNBatch() != localAIModelBatchSize {
		t.Fatalf("%s decoded LoadModel = %#v, want unchanged fields and no projector option", test.name, decoded)
	}
	wantOptions := []string(nil)
	if !equalStrings(decoded.GetOptions(), wantOptions) {
		t.Fatalf("%s decoded LoadModel options = %q, want %q", test.name, decoded.GetOptions(), wantOptions)
	}
	if containsWireField(t, payload, 62) != (len(wantOptions) > 0) {
		t.Fatalf("%s LoadModel field 62 presence does not match options: %x", test.name, payload)
	}
}

func decodeLoadModelPayload(t *testing.T, payload []byte) *ModelOptions {
	t.Helper()
	decoded := &ModelOptions{}
	if err := proto.Unmarshal(payload, decoded); err != nil {
		t.Fatalf("independent LoadModel wire decode: %v", err)
	}
	return decoded
}

func wireFieldNumbers(t *testing.T, payload []byte) []protowire.Number {
	t.Helper()
	fields := make([]protowire.Number, 0)
	for len(payload) > 0 {
		number, wireType, tagSize := protowire.ConsumeTag(payload)
		if tagSize < 0 {
			t.Fatalf("ConsumeTag(%x) error = %v", payload, protowire.ParseError(tagSize))
		}
		valueSize := protowire.ConsumeFieldValue(number, wireType, payload[tagSize:])
		if valueSize < 0 {
			t.Fatalf("ConsumeFieldValue(%x) error = %v", payload, protowire.ParseError(valueSize))
		}
		fields = append(fields, number)
		payload = payload[tagSize+valueSize:]
	}
	return fields
}

func containsWireField(t *testing.T, payload []byte, want protowire.Number) bool {
	t.Helper()
	for _, number := range wireFieldNumbers(t, payload) {
		if number == want {
			return true
		}
	}
	return false
}

func TestPinnedGRPCHostNegotiatorPropagatesDeadlineToBlockedLoadModel(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for controlled LocalAI peer: %v", err)
	}
	backend := &blockedLoadModelBackend{
		health:     make(chan time.Time, 1),
		loadModel:  make(chan time.Time, 1),
		cancelled:  make(chan time.Time, 1),
		clientDone: make(chan struct{}),
	}
	server := grpcgo.NewServer()
	server.RegisterService(&localAIBackendServiceDesc, backend)
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(time.Second):
			t.Errorf("controlled LocalAI gRPC server did not stop")
		}
	})

	const readinessBudget = 150 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), readinessBudget)
	defer cancel()
	go func() {
		<-ctx.Done()
		close(backend.clientDone)
	}()
	deadlineAt, _ := ctx.Deadline()
	modelPath := filepath.Join(t.TempDir(), "fixture.gguf")
	negotiator := NewPinnedGRPCHostProtocolNegotiator(platformgrpc.NetworkDialer{})
	resultCh := make(chan protocolNegotiationOutcome, 1)
	go func() {
		result, negotiateErr := negotiator.Negotiate(
			ctx,
			listener.Addr().String(),
			modelseffects.HostProtocolNegotiationRequest{Configuration: modelseffects.ResolvedHostConfiguration{
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
				Backend:         "localai-llamacpp",
				ModelName:       models.BuiltInModelNameEmbed,
				ModelPath:       modelPath,
			}},
		)
		resultCh <- protocolNegotiationOutcome{result: result, err: negotiateErr}
	}()

	healthAt := awaitWitnessSignal(t, backend.health, resultCh, "Health")
	loadModelAt := awaitWitnessSignal(t, backend.loadModel, resultCh, "LoadModel")
	outcome := awaitProtocolNegotiationOutcome(t, resultCh)
	serverCanceledAt := awaitWitnessSignal(t, backend.cancelled, resultCh, "LoadModel cancellation")
	if outcome.result.Ready || !errors.Is(outcome.err, context.DeadlineExceeded) {
		t.Fatalf("Negotiate at the readiness deadline = result %#v, error %v; want not-ready/context.DeadlineExceeded", outcome.result, outcome.err)
	}
	if healthAt.After(loadModelAt) || !loadModelAt.Before(deadlineAt) || serverCanceledAt.Before(deadlineAt) {
		t.Fatalf("witness phase order is invalid: health=%s load_model=%s deadline=%s server_cancel=%s", healthAt, loadModelAt, deadlineAt, serverCanceledAt)
	}
	t.Logf("LOCALAI-PROTOCOL-DEADLINE endpoint=%s health=%s load_model=%s peer_deadline=%s server_cancel=%s result=%T ready=false", listener.Addr(), healthAt.UTC().Format(time.RFC3339Nano), loadModelAt.UTC().Format(time.RFC3339Nano), deadlineAt.UTC().Format(time.RFC3339Nano), serverCanceledAt.UTC().Format(time.RFC3339Nano), outcome.err)
}

func TestPinnedGRPCHostProtocolNegotiatorUsesSelectedArchiveAccelerator(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name, accelerator string
		gpuLayers         int32
	}{
		{name: "CPU archive on CUDA host", accelerator: "cpu", gpuLayers: 0},
		{name: "CUDA archive on CUDA host", accelerator: "cuda", gpuLayers: 99},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			connection := &recordingGRPCConnection{}
			connection.response, _ = proto.Marshal(&Result{Success: true})
			negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
			_, err := negotiator.Negotiate(context.Background(), "grpc://127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
				Configuration: modelseffects.ResolvedHostConfiguration{
					ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
					Backend:         "localai-llamacpp", ModelName: models.BuiltInModelNameEmbed,
					ModelPath:       "/models/embed.gguf",
					Platform:        models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64", CUDAAvailable: true},
					BackendArtifact: modelseffects.BackendArtifactSelection{Accelerator: testCase.accelerator},
				},
			})
			if err != nil {
				t.Fatalf("negotiate %s: %v", testCase.name, err)
			}
			if got := connection.loadRequest.GetNGPULayers(); got != testCase.gpuLayers {
				t.Fatalf("GPU layers = %d, want %d", got, testCase.gpuLayers)
			}
		})
	}
}

func awaitProtocolNegotiationOutcome(
	t *testing.T,
	result <-chan protocolNegotiationOutcome,
) protocolNegotiationOutcome {
	t.Helper()
	select {
	case outcome := <-result:
		return outcome
	case <-time.After(3 * time.Second):
		t.Fatal("LoadModel gRPC call did not return after readiness deadline")
		return protocolNegotiationOutcome{}
	}
}

type protocolNegotiationOutcome struct {
	result modelseffects.HostProtocolNegotiationResult
	err    error
}

func awaitWitnessSignal[T any](
	t *testing.T,
	signal <-chan T,
	result <-chan protocolNegotiationOutcome,
	phase string,
) T {
	t.Helper()
	select {
	case observed := <-signal:
		return observed
	case outcome := <-result:
		t.Fatalf("Negotiate returned before %s was observed: result=%#v error=%v", phase, outcome.result, outcome.err)
		var zero T
		return zero
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s from controlled LocalAI peer", phase)
		var zero T
		return zero
	}
}

type blockedLoadModelBackend struct {
	health     chan time.Time
	loadModel  chan time.Time
	cancelled  chan time.Time
	clientDone chan struct{}
}

func (backend *blockedLoadModelBackend) Health(context.Context, *HealthMessage) (*Reply, error) {
	backend.health <- time.Now()
	return &Reply{}, nil
}

func (backend *blockedLoadModelBackend) LoadModel(ctx context.Context, _ *ModelOptions) (*Result, error) {
	backend.loadModel <- time.Now()
	<-ctx.Done()
	backend.cancelled <- time.Now()
	// Keep the peer from winning the race with the client's deadline.
	<-backend.clientDone
	return nil, status.Error(codes.Canceled, "controlled LoadModel cancellation")
}

func (*blockedLoadModelBackend) Predict(context.Context, *PredictOptions) (*Reply, error) {
	return nil, status.Error(codes.Unimplemented, "not used by readiness witness")
}

func TestPinnedGRPCProtocolClientFailsOnEmptyPredictResponse(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	client := NewPinnedGRPCProtocolClient(recordingGRPCDialer{connection: connection})
	response, err := client.Predict(
		WithInvocationEndpoint(context.Background(), "127.0.0.1:50051"),
		PredictRequest{Prompt: "describe"},
	)
	if err == nil {
		t.Fatal("Predict() error = nil, want typed failure for empty response")
	}
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Predict() error = %v, want typed InvocationFailure", err)
	}
	if failure.Class != models.InvocationFailureClassBackendProtocol {
		t.Fatalf("failure.Class = %v, want BackendProtocol", failure.Class)
	}
	if !strings.Contains(failure.Message, "LocalAI Predict response was empty") {
		t.Fatalf("failure.Message = %q, want it to contain %q", failure.Message, "LocalAI Predict response was empty")
	}
	if response.Text != "" {
		t.Fatalf("Predict() response.Text = %q, want empty", response.Text)
	}
}

func TestPinnedGRPCProtocolClientUsesChatDeltaTextWhenLegacyMessageIsEmpty(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Reply{ChatDeltas: []*ChatDelta{
		{Content: "generated "}, {Content: "from chat deltas"},
	}})
	client := NewPinnedGRPCProtocolClient(recordingGRPCDialer{connection: connection})
	response, err := client.Predict(
		WithInvocationEndpoint(context.Background(), "127.0.0.1:50051"),
		PredictRequest{Prompt: "describe"},
	)
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}
	if response.Text != "generated from chat deltas" {
		t.Fatalf("Predict() text = %q, want concatenated chat-delta content", response.Text)
	}
}

func TestDecodePredictResponseRecordsReplyShape(t *testing.T) {
	t.Parallel()
	payload, err := proto.Marshal(&Reply{
		Message: []byte("text"), Tokens: 7, PromptTokens: 11,
		Audio: []byte{1, 2, 3}, ChatDeltas: []*ChatDelta{{Content: "private", ReasoningContent: "hidden"}, nil, {ReasoningContent: "think"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := decodePredictResponse(payload)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "text" || response.ReplyBytes != len(payload) || response.MessageBytes != 4 ||
		response.ChatDeltaCount != 3 || response.ReasoningBytes != 11 || response.GeneratedTokens != 7 || response.PromptTokens != 11 || response.AudioBytes != 3 {
		t.Fatalf("decoded reply shape = %#v, want wire and field counts", response)
	}
}

func TestDecodePredictResponseReasoningOnly(t *testing.T) {
	t.Parallel()
	payload, err := proto.Marshal(&Reply{
		Tokens: 256,
		ChatDeltas: []*ChatDelta{
			{ReasoningContent: "private reasoning"},
			{ReasoningContent: "more reasoning"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := decodePredictResponse(payload)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "" || response.ReasoningBytes != len("private reasoning")+len("more reasoning") || response.GeneratedTokens != 256 {
		t.Fatalf("decoded reasoning-only reply shape = %#v", response)
	}
}

// A reasoning-enabled LocalAI Predict reply leaves the legacy message bytes
// empty and streams the assistant answer as chat-delta content interleaved with
// private reasoning content that can itself contain a JSON object shaped like a
// final answer. Decoding must reassemble only the content deltas into the exact
// structured answer bytes while still reporting the reasoning bytes.
//
// The nil delta covers the empty-delta case, and exact equality against the
// whole expected answer already proves that neither the JSON decoy nor any
// reasoning prose leaked into the decoded text, so no narrower containment
// assertions are needed.
func TestDecodePredictResponseSeparatesStructuredFinalAnswerFromPrivateReasoning(t *testing.T) {
	t.Parallel()

	const finalAnswer = `{"status":"ok","findings":[{"file":"grpc_protocol.go","severity":"low"}],"count":1}`
	contentChunks := []string{`{"status":"ok",`, `"findings":[{"file":"grpc_protocol.go","severity":"low"}],`, `"count":1}`}
	reasoning := []string{
		"The operator asked for one structured answer. ",
		`draft shape: {"status":"ok","findings":[],"count":0}`,
		"the answer must stay a single object",
	}
	payload, err := proto.Marshal(&Reply{
		Tokens: 512, PromptTokens: 64,
		ChatDeltas: []*ChatDelta{
			{ReasoningContent: reasoning[0]},
			{Content: contentChunks[0]},
			{ReasoningContent: reasoning[1]},
			nil,
			{Content: contentChunks[1]},
			{ReasoningContent: reasoning[2]},
			{Content: contentChunks[2]},
		},
	})
	if err != nil {
		t.Fatalf("marshal reasoning reply payload: %v", err)
	}

	response, err := decodePredictResponse(payload)
	if err != nil {
		t.Fatalf("decodePredictResponse() error = %v", err)
	}
	if response.Text != finalAnswer {
		t.Fatalf("decoded text = %q, want the exact reassembled content deltas %q", response.Text, finalAnswer)
	}
	wantReasoningBytes := 0
	for _, fragment := range reasoning {
		wantReasoningBytes += len(fragment)
	}
	if response.ReasoningBytes != wantReasoningBytes {
		t.Fatalf("decoded reasoning bytes = %d, want %d from reasoning delta content only", response.ReasoningBytes, wantReasoningBytes)
	}
	if response.MessageBytes != 0 || response.ReplyBytes != len(payload) ||
		response.ChatDeltaCount != 7 || response.GeneratedTokens != 512 || response.PromptTokens != 64 {
		t.Fatalf("decoded reply diagnostics = %#v, want absent message bytes and one count per wire delta", response)
	}
}

func TestOmniJSONSchemaReachesMetadataWithoutChangingThinkingOrCaps(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "properties": map[string]any{
		"language": map[string]any{"const": "ko-KR"},
		"segments": map[string]any{"prefixItems": []any{map[string]any{"const": 4}, map[string]any{"const": 11}}},
	}}
	connection := &schemaHeaderConnection{recordingGRPCConnection: &recordingGRPCConnection{},
		headers: map[string][]string{"x-you-json-schema": {"1"}}}
	connection.response, _ = proto.Marshal(&Reply{
		ChatDeltas: []*ChatDelta{{Content: `{"language":"ko-KR"}`, ReasoningContent: "private comparison"}}})
	client := NewPinnedGRPCProtocolClient(schemaHeaderDialer{connection: connection})
	reply, err := client.Predict(WithInvocationEndpoint(t.Context(), "fixture"), PredictRequest{
		Prompt: "Translate", Inputs: []ProtocolInput{{Modality: models.ModalityImage, Content: "image"}},
		Parameters: []models.OperationParameter{{Name: "json_schema", Value: schema}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Text != `{"language":"ko-KR"}` {
		t.Fatalf("public text = %q", reply.Text)
	}
	if !reflect.DeepEqual(connection.methods, []string{localAIHealthMethod, localAIPredictMethod}) {
		t.Fatalf("schema method order = %v", connection.methods)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(connection.request.Metadata["json_schema"]), &got); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(schema)
	actual, _ := json.Marshal(got)
	if !bytes.Equal(want, actual) || connection.request.Grammar != "" || connection.request.Tokens != 0 {
		t.Fatalf("schema/caps = %#v, want preserved schema without raw grammar or caps", &connection.request)
	}
	if _, present := connection.request.Metadata["chat_template_kwargs"]; present {
		t.Fatal("schema changed thinking")
	}
	if len(connection.request.Images) != 1 {
		t.Fatal("schema dropped media")
	}
}

func assertOmniJSONSchemaInvalid(t *testing.T, err error) {
	t.Helper()
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassInvalidParameter ||
		failure.Operation != models.OperationOMNI || failure.Parameter != "json_schema" {
		t.Fatalf("error = %v, want typed schema InvalidParameter", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatal("schema error leaked input")
	}
}

// These fixtures expose transport metadata explicitly; a byte-only connection
// intentionally does not claim support for native structured generation.
type schemaHeaderDialer struct{ connection platformgrpc.Connection }

func (dialer schemaHeaderDialer) Dial(context.Context, string) (platformgrpc.Connection, error) {
	return dialer.connection, nil
}

type schemaHeaderConnection struct {
	*recordingGRPCConnection
	headers map[string][]string
}

func (connection *schemaHeaderConnection) InvokeWithHeaders(ctx context.Context, method string, payload []byte) ([]byte, map[string][]string, error) {
	body, err := connection.recordingGRPCConnection.Invoke(ctx, method, payload)
	return body, connection.headers, err
}

func TestOmniJSONSchemaRequiresNativeCapabilityBeforePredict(t *testing.T) {
	t.Parallel()
	for _, headers := range []map[string][]string{nil, {"x-you-json-schema": {"2"}}, {"x-you-json-schema": {"1", "2"}}} {
		connection := &schemaHeaderConnection{recordingGRPCConnection: &recordingGRPCConnection{}, headers: headers}
		client := NewPinnedGRPCProtocolClient(schemaHeaderDialer{connection: connection})
		_, err := client.Predict(WithInvocationEndpoint(t.Context(), "fixture"), PredictRequest{Prompt: "Answer",
			Parameters: []models.OperationParameter{{Name: "json_schema", Value: map[string]any{"type": "object"}}}})
		if !errors.Is(err, models.ErrHostProtocolIncompatible) || !reflect.DeepEqual(connection.methods, []string{localAIHealthMethod}) {
			t.Fatalf("error/methods = %v / %v, expected Health only", err, connection.methods)
		}
	}
	connection := &recordingGRPCConnection{}
	_, err := NewPinnedGRPCProtocolClient(recordingGRPCDialer{connection: connection}).Predict(
		WithInvocationEndpoint(t.Context(), "fixture"), PredictRequest{Prompt: "Answer",
			Parameters: []models.OperationParameter{{Name: "json_schema", Value: map[string]any{"type": "object"}}}})
	if !errors.Is(err, models.ErrHostProtocolIncompatible) || len(connection.methods) != 0 {
		t.Fatalf("missing header accessor error/methods = %v / %v", err, connection.methods)
	}
}

func TestOmniCodecSchemaPreflightFailsBeforeMediaEffects(t *testing.T) {
	t.Parallel()
	request := omniMediaRequest(t, models.InferenceInput{Name: "video", Modality: models.ModalityVideo, Content: "video"})
	request.Parameters = []models.OperationParameter{{Name: "json_schema", Value: map[string]any{"type": "object"}}}
	failure := errors.New("native schema unavailable")
	fixture := &scriptedOmniProtocol{schemaErr: failure}
	_, err := NewPinnedOmniCodec(fixture, func(context.Context, []byte) ([]byte, error) {
		t.Fatal("unsupported schema extracted video")
		return nil, nil
	}).Invoke(t.Context(), request)
	if !errors.Is(err, failure) || len(fixture.requests) != 0 {
		t.Fatalf("preflight error/calls = %v / %d", err, len(fixture.requests))
	}
	_, err = NewPinnedOmniCodec(&protocolFixture{}).Invoke(t.Context(), request)
	if !errors.Is(err, models.ErrHostProtocolIncompatible) {
		t.Fatalf("missing preflight = %v", err)
	}
}

func TestOmniSchemaCapabilityFailureDistinguishesUnknownSupport(t *testing.T) {
	t.Parallel()
	privateFailure := errors.New("private endpoint secret unavailable")
	for _, test := range []struct {
		name     string
		rpcError error
		cause    error
		message  string
	}{
		{"missing capability", nil, models.ErrHostProtocolIncompatible, "update or install a schema-capable backend"},
		{"Health unavailable", privateFailure, privateFailure, "check local backend readiness and connectivity, then retry"},
	} {
		t.Run(test.name, func(t *testing.T) {
			connection := &schemaHeaderConnection{recordingGRPCConnection: &recordingGRPCConnection{invokeErr: test.rpcError}}
			client := NewPinnedGRPCProtocolClient(schemaHeaderDialer{connection: connection})
			err := client.(JSONSchemaProtocolClient).ValidateJSONSchemaSupport(WithInvocationEndpoint(t.Context(), "fixture"))
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol ||
				failure.Parameter != omniJSONSchemaParameter || failure.Operation != models.OperationOMNI {
				t.Fatalf("failure = %v, want typed OMNI schema protocol failure", err)
			}
			if !errors.Is(err, test.cause) || !strings.Contains(err.Error(), test.message) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("cause/message = %v, want actionable safe failure preserving cause", err)
			}
			if !reflect.DeepEqual(connection.methods, []string{localAIHealthMethod}) {
				t.Fatalf("failed preflight methods = %v", connection.methods)
			}
		})
	}
}
