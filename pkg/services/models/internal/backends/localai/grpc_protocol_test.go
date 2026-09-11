package localai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image/png"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelartifacts "github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestPinnedLocalAIModelOptionsDescriptorMatchesField62Contract(t *testing.T) {
	t.Parallel()

	fields := (&ModelOptions{}).ProtoReflect().Descriptor().Fields()
	options := fields.ByName("Options")
	if options == nil || options.Number() != 62 ||
		options.Cardinality() != protoreflect.Repeated || options.Kind() != protoreflect.StringKind {
		t.Fatalf("Options descriptor = %#v, want repeated string field 62", options)
	}
	mmproj := fields.ByName("MMProj")
	if mmproj == nil || mmproj.Number() != 41 ||
		mmproj.Cardinality() != protoreflect.Optional || mmproj.Kind() != protoreflect.StringKind {
		t.Fatalf("MMProj descriptor = %#v, want optional string field 41", mmproj)
	}
	for _, number := range []protoreflect.FieldNumber{15, 38} {
		if field := fields.ByNumber(number); field != nil {
			t.Fatalf("unexpected speculative ModelOptions field %d: %v", number, field)
		}
	}
}

func TestPinnedGRPCProtocolClientMapsOrderedOmniValuesToPinnedFields(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Reply{Message: []byte("generated"), Tokens: 3, PromptTokens: 2})
	client := NewPinnedGRPCProtocolClient(recordingGRPCDialer{connection: connection})
	request := PredictRequest{
		Prompt: "describe the inputs",
		Inputs: []ProtocolInput{
			{Slot: "prompt", Modality: models.ModalityText, Content: "describe the inputs"},
			{Slot: "image", Modality: models.ModalityImage, Content: "image-a.png"},
			{Slot: "audio", Modality: models.ModalityAudio, Reference: "audio-a.wav"},
			{Slot: "image", Modality: models.ModalityImage, Content: "image-b.png"},
			{Slot: "video", Modality: models.ModalityVideo, Content: "video-a.mp4"},
		},
		Parameters: []models.OperationParameter{{Name: "temperature", Value: 0.2}},
	}
	ctx := WithInvocationEndpoint(context.Background(), "grpc://127.0.0.1:50051")
	response, err := client.Predict(ctx, request)
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}
	if response.Text != "generated" || response.Usage == "" {
		t.Fatalf("Predict() response = %#v, want generated text and usage", response)
	}
	if connection.method != localAIPredictMethod || connection.closed != 1 {
		t.Fatalf("transport facts = method %q, closed %d, want Predict and one close", connection.method, connection.closed)
	}
	if connection.request.Prompt != request.Prompt ||
		!equalStrings(connection.request.Images, []string{
			base64.StdEncoding.EncodeToString([]byte("image-a.png")),
			base64.StdEncoding.EncodeToString([]byte("image-b.png")),
		}) ||
		!equalStrings(connection.request.Audios, []string{"audio-a.wav"}) ||
		!equalStrings(connection.request.Videos, []string{
			base64.StdEncoding.EncodeToString([]byte("video-a.mp4")),
		}) ||
		connection.request.Metadata["temperature"] != "0.2" {
		t.Fatal("pinned request fields do not preserve prompt/media order/metadata")
	}
}

func TestPinnedGRPCProtocolClientPreservesBinaryMediaThroughBase64Fields(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	client := NewPinnedGRPCProtocolClient(recordingGRPCDialer{connection: connection})
	image := string([]byte{0x00, 0xff, 0x10, 0x80, 0x7f})
	if _, err := client.Predict(
		WithInvocationEndpoint(context.Background(), "127.0.0.1:50051"),
		PredictRequest{Inputs: []ProtocolInput{{Modality: models.ModalityImage, Content: image}}},
	); err != nil {
		t.Fatalf("Predict() error = %v", err)
	}
	if len(connection.request.Images) != 1 {
		t.Fatalf("encoded images = %#v, want one image", connection.request.Images)
	}
	decoded, err := base64.StdEncoding.DecodeString(connection.request.Images[0])
	if err != nil {
		t.Fatalf("DecodeString(image) error = %v", err)
	}
	if string(decoded) != image {
		t.Fatalf("decoded image = %v, want original bytes %v", decoded, []byte(image))
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

func TestPinnedEmbeddingBackendMapsTextParametersAndResponse(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&EmbeddingResult{
		Embeddings: []float32{0.1, -0.2, 0.3},
	})
	backend := NewPinnedEmbeddingBackend(recordingGRPCDialer{connection: connection})
	response, err := backend(
		WithInvocationEndpoint(context.Background(), "grpc://127.0.0.1:50051"),
		models.EmbeddingBackendRequest{
			Text:       "Find similar work",
			Parameters: map[string]any{"normalize": true, "dimensions": 3},
		},
	)
	if err != nil {
		t.Fatalf("Embedding() error = %v", err)
	}
	if len(response.Embeddings) != 3 || response.Embeddings[0] != float64(float32(0.1)) ||
		response.Embeddings[1] != float64(float32(-0.2)) {
		t.Fatalf("Embedding() response = %#v, want three mapped values", response.Embeddings)
	}
	if connection.method != localAIEmbeddingMethod || connection.closed != 1 {
		t.Fatalf("embedding transport facts = method %q, closed %d, want Embedding and one close", connection.method, connection.closed)
	}
	if connection.embeddingRequest.GetPrompt() != "Find similar work" ||
		connection.embeddingRequest.GetEmbeddings() != "Find similar work" ||
		connection.embeddingRequest.GetMetadata()["normalize"] != "true" ||
		connection.embeddingRequest.GetMetadata()["dimensions"] != "3" {
		t.Fatalf("embedding request prompt=%q embeddings=%q metadata=%v, want dedicated embedding text and JSON metadata", connection.embeddingRequest.GetPrompt(), connection.embeddingRequest.GetEmbeddings(), connection.embeddingRequest.GetMetadata())
	}
}

func TestPinnedEmbeddingBackendFailsClosedWithoutEndpoint(t *testing.T) {
	t.Parallel()

	backend := NewPinnedEmbeddingBackend(recordingGRPCDialer{connection: &recordingGRPCConnection{}})
	_, err := backend(context.Background(), models.EmbeddingBackendRequest{Text: "secret input"})
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol {
		t.Fatalf("Embedding() error = %v, failure = %#v, want typed backend-protocol failure", err, failure)
	}
	if strings.Contains(err.Error(), "secret input") {
		t.Fatalf("Embedding() error leaked input: %v", err)
	}
}

func TestPinnedEmbeddingBackendClassifiesTransportFailures(t *testing.T) {
	for _, test := range []struct {
		name    string
		cause   error
		class   models.InvocationFailureClass
		message string
	}{
		{
			name:    "backend unavailable",
			cause:   status.Error(codes.Unavailable, "backend stopped"),
			class:   models.InvocationFailureClassBackendReadiness,
			message: "LocalAI backend is unavailable",
		},
		{
			name:    "protocol mismatch",
			cause:   status.Error(codes.FailedPrecondition, "wrong protocol"),
			class:   models.InvocationFailureClassBackendProtocol,
			message: "LocalAI backend protocol is incompatible",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			connection := &recordingGRPCConnection{invokeErr: test.cause}
			backend := NewPinnedEmbeddingBackend(recordingGRPCDialer{connection: connection})
			_, err := backend(
				WithInvocationEndpoint(context.Background(), "127.0.0.1:50051"),
				models.EmbeddingBackendRequest{Text: "query"},
			)
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) || failure.Class != test.class || !strings.Contains(failure.Message, test.message) {
				t.Fatalf("Embedding() error = %v, failure = %#v, want %s containing %q", err, failure, test.class, test.message)
			}
			if connection.closed != 1 {
				t.Fatalf("connection closes = %d, want one close after transport failure", connection.closed)
			}
		})
	}
}

func TestPinnedEmbeddingBackendReturnsContextCancellationBeforeDial(t *testing.T) {
	dialer := &countingEmbeddingDialer{}
	backend := NewPinnedEmbeddingBackend(dialer)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := backend(WithInvocationEndpoint(ctx, "127.0.0.1:50051"), models.EmbeddingBackendRequest{Text: "query"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Embedding() error = %v, want context cancellation", err)
	}
	if dialer.calls != 0 {
		t.Fatalf("dial calls = %d, want no dial after cancellation", dialer.calls)
	}
}

func TestPinnedGRPCHostProtocolNegotiatorUsesHealthRPC(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
	result, err := negotiator.Negotiate(context.Background(), "grpc://127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         "localai-llamacpp",
	})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if result.ProtocolVersion != modelseffects.PinnedHostProtocolVersion ||
		result.Backend != "localai-llamacpp" || !result.Ready {
		t.Fatalf("Negotiate() result = %#v, want ready pinned result", result)
	}
	if connection.method != localAIHealthMethod || connection.closed != 1 {
		t.Fatalf("health transport facts = method %q, closed %d, want Health and one close", connection.method, connection.closed)
	}
}

func TestPinnedGRPCHostProtocolNegotiatorLoadsDeclaredModelAfterHealth(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Result{Success: true, Message: "loaded"})
	negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
	modelFile := filepath.Join("models", "llm", "model.gguf")
	mmprojFile := filepath.Join("models", "llm", "mmproj-F16.gguf")
	result, err := negotiator.Negotiate(context.Background(), "grpc://127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         "localai-llamacpp",
		ModelName:       "llm",
		ModelPath:       modelFile,
		MMProjPath:      mmprojFile,
	})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if !result.Ready || !equalStrings(connection.methods, []string{localAIHealthMethod, localAILoadModelMethod}) || connection.closed != 1 {
		t.Fatalf("load transport facts = methods %#v, ready %t, closed %d, want Health/LoadModel/ready/one close", connection.methods, result.Ready, connection.closed)
	}
	if connection.loadRequest.GetModel() != "llm" ||
		connection.loadRequest.GetEmbeddings() ||
		connection.loadRequest.GetModelFile() != modelFile ||
		connection.loadRequest.GetMMProj() != mmprojFile ||
		connection.loadRequest.GetModelPath() != filepath.Dir(modelFile) ||
		connection.loadRequest.GetNBatch() != localAIModelBatchSize ||
		len(connection.loadRequest.GetOptions()) != 0 {
		t.Fatalf(
			"load request model=%q modelFile=%q mmproj=%q modelPath=%q nBatch=%d options=%v, want model name, file/projector paths, model directory, nonzero batch size, and no VibeVoice option",
			connection.loadRequest.GetModel(), connection.loadRequest.GetModelFile(), connection.loadRequest.GetMMProj(), connection.loadRequest.GetModelPath(), connection.loadRequest.GetNBatch(), connection.loadRequest.GetOptions(),
		)
	}
	expected := appendStringField(nil, 1, "llm")
	expected = appendVarintField(expected, 4, localAIModelBatchSize)
	expected = appendStringField(expected, 21, modelFile)
	expected = appendStringField(expected, 41, mmprojFile)
	expected = appendStringField(expected, 59, filepath.Dir(modelFile))
	if !bytes.Equal(connection.loadPayload, expected) {
		t.Fatalf("non-TTS LoadModel wire bytes = %x, want prior compatible bytes %x", connection.loadPayload, expected)
	}
}

func TestPinnedGRPCHostProtocolNegotiatorKeepsVibeVoiceOptionsPrivateToBuiltinTTS(t *testing.T) {
	t.Parallel()

	for _, modelName := range []string{"llm", models.BuiltInModelNameEmbed, "asr"} {
		modelName := modelName
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			connection := &recordingGRPCConnection{}
			connection.response, _ = proto.Marshal(&Result{Success: true, Message: "loaded"})
			negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
			modelFile := filepath.Join(t.TempDir(), "model.gguf")
			_, err := negotiator.Negotiate(context.Background(), "127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
				Backend:         "localai-llamacpp",
				ModelName:       modelName,
				ModelPath:       modelFile,
			})
			if err != nil {
				t.Fatalf("Negotiate() error = %v", err)
			}
			if len(connection.loadRequest.GetOptions()) != 0 || connection.loadRequest.GetMMProj() != "" {
				t.Fatalf("%s LoadModel options/mmproj = %#v/%q, want no private option or projector", modelName, connection.loadRequest.GetOptions(), connection.loadRequest.GetMMProj())
			}
		})
	}
}

func TestPinnedGRPCHostProtocolNegotiatorSerializesConfinedVibeVoiceRoleOptions(t *testing.T) {
	t.Parallel()

	manifest, err := modelartifacts.DefaultModelRoleManifest()
	if err != nil {
		t.Fatalf("DefaultModelRoleManifest: %v", err)
	}
	definition, ok := manifest.Model(models.BuiltInModelNameTTS)
	if !ok {
		t.Fatal("TTS role definition is missing")
	}
	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Result{Success: true, Message: "loaded"})
	negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
	modelRoot := t.TempDir()
	modelFile := filepath.Join(modelRoot, definition.Artifacts[0].Path)
	tokenizerFile := filepath.Join(modelRoot, definition.Artifacts[1].Path)
	voiceFile := filepath.Join(modelRoot, definition.Artifacts[2].Path)
	_, err = negotiator.Negotiate(context.Background(), "127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         "localai-vibevoice",
		ModelName:       models.BuiltInModelNameTTS,
		Revision:        definition.Publication.Revision,
		ModelPath:       modelFile,
		ModelFiles:      []string{modelFile, tokenizerFile, voiceFile},
	})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	wantOptions := []string{"tokenizer=" + tokenizerFile, "voice=" + voiceFile}
	if !equalStrings(connection.loadRequest.GetOptions(), wantOptions) {
		t.Fatalf("VibeVoice options = %#v, want confined tokenizer and voice options", connection.loadRequest.GetOptions())
	}

	expected := appendStringField(nil, 1, models.BuiltInModelNameTTS)
	expected = appendVarintField(expected, 4, localAIModelBatchSize)
	expected = appendStringField(expected, 21, modelFile)
	expected = appendStringField(expected, 59, filepath.Dir(modelFile))
	expected = appendStringField(expected, 62, "tokenizer="+tokenizerFile)
	expected = appendStringField(expected, 62, "voice="+voiceFile)
	if !bytes.Equal(connection.loadPayload, expected) {
		t.Fatalf("LoadModel wire bytes = %x, want exact pinned bytes %x", connection.loadPayload, expected)
	}
}

func TestPinnedGRPCHostProtocolNegotiatorRejectsInvalidVibeVoiceLayoutBeforeLoadRPC(t *testing.T) {
	t.Parallel()

	manifest, err := modelartifacts.DefaultModelRoleManifest()
	if err != nil {
		t.Fatalf("DefaultModelRoleManifest: %v", err)
	}
	definition, ok := manifest.Model(models.BuiltInModelNameTTS)
	if !ok {
		t.Fatal("TTS role definition is missing")
	}
	modelRoot := t.TempDir()
	modelFile := filepath.Join(modelRoot, definition.Artifacts[0].Path)
	tokenizerFile := filepath.Join(modelRoot, definition.Artifacts[1].Path)
	for _, testCase := range []struct {
		name  string
		files []string
	}{
		{name: "missing", files: nil},
		{name: "ambiguous", files: []string{modelFile, tokenizerFile, tokenizerFile}},
		{name: "traversal", files: []string{modelFile, tokenizerFile, filepath.Join(modelRoot, "..", definition.Artifacts[2].Path)}},
		{name: "absolute escape", files: []string{modelFile, tokenizerFile, filepath.Join(filepath.Dir(modelRoot), "outside", definition.Artifacts[2].Path)}},
		{name: "malformed", files: []string{modelFile, ""}},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			connection := &recordingGRPCConnection{}
			negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
			_, err := negotiator.Negotiate(context.Background(), "127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
				Backend:         "localai-vibevoice",
				ModelName:       models.BuiltInModelNameTTS,
				Revision:        definition.Publication.Revision,
				ModelPath:       modelFile,
				ModelFiles:      testCase.files,
			})
			if !errors.Is(err, models.ErrHostProtocolIncompatible) {
				t.Fatalf("Negotiate() error = %v, want typed protocol incompatibility", err)
			}
			if strings.Contains(err.Error(), definition.Artifacts[1].Path) || strings.Contains(err.Error(), definition.Artifacts[2].Path) {
				t.Fatalf("layout error leaked a path or role artifact name: %v", err)
			}
			if !equalStrings(connection.methods, []string{localAIHealthMethod}) || connection.closed != 1 {
				t.Fatalf("invalid layout transport facts = methods %#v closed %d, want health only and one close", connection.methods, connection.closed)
			}
		})
	}
}

func TestPinnedGRPCHostProtocolNegotiatorEnablesEmbeddingModeForBuiltinEmbed(t *testing.T) {
	t.Parallel()

	connection := &recordingGRPCConnection{}
	connection.response, _ = proto.Marshal(&Result{Success: true, Message: "loaded"})
	negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
	_, err := negotiator.Negotiate(context.Background(), "127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         "localai-llamacpp",
		ModelName:       models.BuiltInModelNameEmbed,
		ModelPath:       `C:\models\embed\model.gguf`,
	})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if !connection.loadRequest.GetEmbeddings() {
		t.Fatal("embedding load request flag = false, want pinned LocalAI embedding mode")
	}
}

func TestPinnedGRPCHostProtocolNegotiatorRejectsFailedOrMalformedLoadModel(t *testing.T) {
	t.Parallel()

	failed, _ := proto.Marshal(&Result{Message: "model rejected"})
	for _, test := range []struct {
		name     string
		response []byte
	}{
		{name: "unsuccessful result", response: failed},
		{name: "malformed result", response: []byte{0xff}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			connection := &recordingGRPCConnection{response: test.response}
			negotiator := NewPinnedGRPCHostProtocolNegotiator(recordingGRPCDialer{connection: connection})
			result, err := negotiator.Negotiate(context.Background(), "127.0.0.1:50051", modelseffects.HostProtocolNegotiationRequest{
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
				Backend:         "localai-llamacpp",
				ModelName:       "llm",
				ModelPath:       `C:\models\llm\model.gguf`,
			})
			if result.Ready || !errors.Is(err, models.ErrHostProtocolIncompatible) {
				t.Fatalf("Negotiate() = result %#v, error %v, want typed protocol failure", result, err)
			}
			if !equalStrings(connection.methods, []string{localAIHealthMethod, localAILoadModelMethod}) || connection.closed != 1 {
				t.Fatalf("load failure transport facts = methods %#v, closed %d, want Health/LoadModel and one close", connection.methods, connection.closed)
			}
		})
	}
}

func TestPinnedGRPCProtocolClientUsesNetworkDialerAgainstLocalHost(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpcgo.NewServer()
	server.RegisterService(&localAIBackendServiceDesc, networkBackendImpl{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	client := NewPinnedGRPCProtocolClient(platformgrpc.NetworkDialer{})
	response, err := client.Predict(
		WithInvocationEndpoint(context.Background(), listener.Addr().String()),
		PredictRequest{Prompt: "network proof"},
	)
	if err != nil {
		t.Fatalf("Predict() error = %v", err)
	}
	if response.Text != "network response" {
		t.Fatalf("Predict() text = %q, want network response", response.Text)
	}
}

func TestPinnedGRPCProtocolServerProvesProjectorLoadBeforeImagePredict(t *testing.T) {
	t.Parallel()

	imageBytes := controlledImageBytes(t)
	endpoint, backend := startControlledImageServer(t)
	modelRoot := t.TempDir()
	modelFile := filepath.Join(modelRoot, "gemma-4-E4B-it-Q4_K_M.gguf")
	mmprojFile := filepath.Join(modelRoot, "mmproj-F16.gguf")
	ctx, cancel := context.WithTimeout(context.Background(), testProtocolDeadline)
	defer cancel()

	negotiateControlledImageHost(t, ctx, endpoint, modelFile, mmprojFile)
	result := invokeControlledImage(t, ctx, endpoint, imageBytes)
	assertControlledImageResult(t, result)
	assertControlledImageObservation(t, backend.snapshot(), modelRoot, modelFile, mmprojFile, 1, controlledImagePNGHash)
}

func TestPinnedGRPCProtocolServerKeepsTextOnlyPredictImageFree(t *testing.T) {
	t.Parallel()

	endpoint, backend := startControlledImageServer(t)
	modelRoot := t.TempDir()
	modelFile := filepath.Join(modelRoot, "gemma-4-E4B-it-Q4_K_M.gguf")
	mmprojFile := filepath.Join(modelRoot, "mmproj-F16.gguf")
	ctx, cancel := context.WithTimeout(context.Background(), testProtocolDeadline)
	defer cancel()

	negotiateControlledImageHost(t, ctx, endpoint, modelFile, mmprojFile)
	result := invokeControlledTextOnly(t, ctx, endpoint)
	assertControlledImageResult(t, result)
	assertControlledImageObservation(t, backend.snapshot(), modelRoot, modelFile, mmprojFile, 0, "")
}

func controlledImageBytes(t *testing.T) []byte {
	t.Helper()
	imageBytes, err := base64.StdEncoding.DecodeString(controlledImagePNGBase64)
	if err != nil {
		t.Fatalf("DecodeString(controlled PNG) error = %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(imageBytes)); err != nil {
		t.Fatalf("controlled image is not a PNG: %v", err)
	}
	return imageBytes
}

func startControlledImageServer(t *testing.T) (string, *controlledImageBackend) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpcgo.NewServer()
	backend := &controlledImageBackend{}
	server.RegisterService(&localAIBackendServiceDesc, backend)
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		<-serveDone
	})
	return listener.Addr().String(), backend
}

func negotiateControlledImageHost(t *testing.T, ctx context.Context, endpoint, modelFile, mmprojFile string) {
	t.Helper()
	negotiator := NewPinnedGRPCHostProtocolNegotiator(platformgrpc.NetworkDialer{})
	negotiated, err := negotiator.Negotiate(ctx, endpoint, modelseffects.HostProtocolNegotiationRequest{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         "localai-llamacpp",
		ModelName:       models.BuiltInModelNameLLM,
		ModelPath:       modelFile,
		MMProjPath:      mmprojFile,
	})
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if !negotiated.Ready {
		t.Fatalf("Negotiate() result = %#v, want ready", negotiated)
	}
}

func invokeControlledImage(t *testing.T, ctx context.Context, endpoint string, imageBytes []byte) OmniInvocationResult {
	t.Helper()
	return invokeControlledOmni(t, ctx, endpoint, []models.InferenceInput{
		{Name: "prompt", Modality: models.ModalityText, Content: "describe this image"},
		{Name: "image", Modality: models.ModalityImage, ContentType: "image/png", MediaType: "image/png", Content: string(imageBytes)},
	})
}

func invokeControlledTextOnly(t *testing.T, ctx context.Context, endpoint string) OmniInvocationResult {
	t.Helper()
	return invokeControlledOmni(t, ctx, endpoint, []models.InferenceInput{
		{Name: "prompt", Modality: models.ModalityText, Content: "describe this image"},
	})
}

func invokeControlledOmni(t *testing.T, ctx context.Context, endpoint string, inputs []models.InferenceInput) OmniInvocationResult {
	t.Helper()
	codec := NewPinnedOmniCodec(NewPinnedGRPCProtocolClient(platformgrpc.NetworkDialer{}))
	scope, err := (models.RuntimeScopeRef{}).Parse("scope:controlled-image")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	result, err := codec.Invoke(WithInvocationEndpoint(ctx, endpoint), models.InvokeModelRequest{
		Scope:     scope,
		Holder:    "controlled-image-test",
		Model:     models.ModelReference{NameOrURI: models.BuiltInModelNameLLM},
		Operation: models.OperationOMNI,
		Inputs:    inputs,
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	return result
}

func assertControlledImageResult(t *testing.T, result OmniInvocationResult) {
	t.Helper()
	if len(result.Content) != 1 || result.Content[0].Content != "controlled image response" {
		t.Fatalf("Invoke() result = %#v, want controlled text output", result)
	}
}

func assertControlledImageObservation(
	t *testing.T,
	observation controlledImageObservation,
	modelRoot, modelFile, mmprojFile string,
	wantImageCount int,
	wantImageSHA256 string,
) {
	t.Helper()
	if !equalStrings(observation.calls, []string{localAIHealthMethod, localAILoadModelMethod, localAIPredictMethod}) {
		t.Fatalf("server calls = %#v, want Health, LoadModel, Predict", observation.calls)
	}
	if observation.load.ModelFile != modelFile || observation.load.ModelPath != modelRoot ||
		observation.load.MMProj != mmprojFile || observation.load.Model != models.BuiltInModelNameLLM {
		t.Fatalf("server LoadModel = %#v, want exact model/projector paths", observation.load)
	}
	if observation.imageCount != wantImageCount || observation.imageSHA256 != wantImageSHA256 {
		t.Fatalf("server image observation = count:%d sha256:%q, want count:%d sha256:%q", observation.imageCount, observation.imageSHA256, wantImageCount, wantImageSHA256)
	}
}

const (
	controlledImagePNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	controlledImagePNGHash   = "431ced6916a2a21a156e38701afe55bbd7f88969fbbfc56d7fe099d47f265460"
	testProtocolDeadline     = 10 * time.Second
)

type recordingGRPCDialer struct {
	connection *recordingGRPCConnection
}

func (dialer recordingGRPCDialer) Dial(context.Context, string) (platformgrpc.Connection, error) {
	return dialer.connection, nil
}

type recordingGRPCConnection struct {
	method           string
	methods          []string
	request          PredictOptions
	embeddingRequest PredictOptions
	loadRequest      ModelOptions
	loadPayload      []byte
	response         []byte
	invokeErr        error
	closed           int
}

func (connection *recordingGRPCConnection) Invoke(
	_ context.Context,
	method string,
	payload []byte,
) ([]byte, error) {
	connection.method = method
	connection.methods = append(connection.methods, method)
	if connection.invokeErr != nil {
		return nil, connection.invokeErr
	}
	if method == localAIPredictMethod {
		if err := proto.Unmarshal(payload, &connection.request); err != nil {
			return nil, err
		}
	}
	if method == localAIEmbeddingMethod {
		if err := proto.Unmarshal(payload, &connection.embeddingRequest); err != nil {
			return nil, err
		}
	}
	if method == localAILoadModelMethod {
		connection.loadPayload = append([]byte(nil), payload...)
		if err := proto.Unmarshal(payload, &connection.loadRequest); err != nil {
			return nil, err
		}
	}
	return connection.response, nil
}

func appendStringField(buffer []byte, number protowire.Number, value string) []byte {
	buffer = protowire.AppendTag(buffer, number, protowire.BytesType)
	return protowire.AppendString(buffer, value)
}

func appendVarintField(buffer []byte, number protowire.Number, value int32) []byte {
	buffer = protowire.AppendTag(buffer, number, protowire.VarintType)
	return protowire.AppendVarint(buffer, uint64(value))
}

type countingEmbeddingDialer struct {
	calls int
}

func (dialer *countingEmbeddingDialer) Dial(context.Context, string) (platformgrpc.Connection, error) {
	dialer.calls++
	return &recordingGRPCConnection{}, nil
}

func (connection *recordingGRPCConnection) Close() error {
	connection.closed++
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type controlledImageBackend struct {
	mu          sync.Mutex
	calls       []string
	load        controlledLoadObservation
	imageCount  int
	imageSHA256 string
}

func (backend *controlledImageBackend) Health(context.Context, *HealthMessage) (*Reply, error) {
	backend.record(localAIHealthMethod)
	return &Reply{}, nil
}

func (backend *controlledImageBackend) LoadModel(_ context.Context, request *ModelOptions) (*Result, error) {
	backend.mu.Lock()
	backend.calls = append(backend.calls, localAILoadModelMethod)
	backend.load = controlledLoadObservation{
		Model:     request.GetModel(),
		ModelFile: request.GetModelFile(),
		ModelPath: request.GetModelPath(),
		MMProj:    request.GetMMProj(),
		Options:   append([]string(nil), request.GetOptions()...),
	}
	backend.mu.Unlock()
	return &Result{Success: true}, nil
}

func (backend *controlledImageBackend) Predict(_ context.Context, request *PredictOptions) (*Reply, error) {
	var imageHash string
	if len(request.GetImages()) == 1 {
		imageBytes, err := base64.StdEncoding.DecodeString(request.GetImages()[0])
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "image is not base64")
		}
		digest := sha256.Sum256(imageBytes)
		imageHash = hex.EncodeToString(digest[:])
	}
	backend.mu.Lock()
	backend.calls = append(backend.calls, localAIPredictMethod)
	backend.imageCount = len(request.GetImages())
	backend.imageSHA256 = imageHash
	backend.mu.Unlock()
	return &Reply{Message: []byte("controlled image response")}, nil
}

func (backend *controlledImageBackend) record(method string) {
	backend.mu.Lock()
	backend.calls = append(backend.calls, method)
	backend.mu.Unlock()
}

type controlledImageObservation struct {
	calls       []string
	load        controlledLoadObservation
	imageCount  int
	imageSHA256 string
}

type controlledLoadObservation struct {
	Model     string
	ModelFile string
	ModelPath string
	MMProj    string
	Options   []string
}

func (backend *controlledImageBackend) snapshot() controlledImageObservation {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return controlledImageObservation{
		calls:       append([]string(nil), backend.calls...),
		load:        backend.load,
		imageCount:  backend.imageCount,
		imageSHA256: backend.imageSHA256,
	}
}

type networkBackend interface {
	Health(context.Context, *HealthMessage) (*Reply, error)
	LoadModel(context.Context, *ModelOptions) (*Result, error)
	Predict(context.Context, *PredictOptions) (*Reply, error)
}

type networkBackendImpl struct{}

func (networkBackendImpl) Health(context.Context, *HealthMessage) (*Reply, error) {
	return &Reply{}, nil
}

func (networkBackendImpl) LoadModel(context.Context, *ModelOptions) (*Result, error) {
	return &Result{Success: true}, nil
}

func (networkBackendImpl) Predict(context.Context, *PredictOptions) (*Reply, error) {
	return &Reply{Message: []byte("network response")}, nil
}

var localAIBackendServiceDesc = grpcgo.ServiceDesc{
	ServiceName: "backend.Backend",
	HandlerType: (*networkBackend)(nil),
	Methods: []grpcgo.MethodDesc{
		{MethodName: "Health", Handler: localAIHealthHandler},
		{MethodName: "LoadModel", Handler: localAILoadModelHandler},
		{MethodName: "Predict", Handler: localAIPredictHandler},
	},
}

func localAILoadModelHandler(
	srv any,
	ctx context.Context,
	decode func(any) error,
	interceptor grpcgo.UnaryServerInterceptor,
) (any, error) {
	request := new(ModelOptions)
	if err := decode(request); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(networkBackend).LoadModel(ctx, request)
	}
	info := &grpcgo.UnaryServerInfo{Server: srv, FullMethod: localAILoadModelMethod}
	handler := func(ctx context.Context, request any) (any, error) {
		return srv.(networkBackend).LoadModel(ctx, request.(*ModelOptions))
	}
	return interceptor(ctx, request, info, handler)
}

func localAIHealthHandler(
	srv any,
	ctx context.Context,
	decode func(any) error,
	interceptor grpcgo.UnaryServerInterceptor,
) (any, error) {
	request := new(HealthMessage)
	if err := decode(request); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(networkBackend).Health(ctx, request)
	}
	info := &grpcgo.UnaryServerInfo{Server: srv, FullMethod: localAIHealthMethod}
	handler := func(ctx context.Context, request any) (any, error) {
		return srv.(networkBackend).Health(ctx, request.(*HealthMessage))
	}
	return interceptor(ctx, request, info, handler)
}

func localAIPredictHandler(
	srv any,
	ctx context.Context,
	decode func(any) error,
	interceptor grpcgo.UnaryServerInterceptor,
) (any, error) {
	request := new(PredictOptions)
	if err := decode(request); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(networkBackend).Predict(ctx, request)
	}
	info := &grpcgo.UnaryServerInfo{Server: srv, FullMethod: localAIPredictMethod}
	handler := func(ctx context.Context, request any) (any, error) {
		return srv.(networkBackend).Predict(ctx, request.(*PredictOptions))
	}
	return interceptor(ctx, request, info, handler)
}
