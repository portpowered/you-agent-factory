package localai

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

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
	assertWindowsCPUProjectorRequest(t, connection.loadRequest, modelFile, mmprojFile)
	assertWindowsCPUProjectorWire(t, connection.loadPayload, modelFile, mmprojFile)
}

func assertWindowsCPUProjectorResult(t *testing.T, result modelseffects.HostProtocolNegotiationResult, connection *recordingGRPCConnection) {
	t.Helper()
	if !result.Ready || !equalStrings(connection.methods, []string{localAIHealthMethod, localAILoadModelMethod}) || connection.closed != 1 {
		t.Fatalf("load transport facts = methods %#v, ready %t, closed %d, want Health/LoadModel/ready/one close", connection.methods, result.Ready, connection.closed)
	}
}

func assertWindowsCPUProjectorRequest(t *testing.T, request ModelOptions, modelFile, mmprojFile string) {
	t.Helper()
	if request.GetModel() != models.BuiltInModelNameLLM || request.GetEmbeddings() ||
		request.GetModelFile() != modelFile || request.GetMMProj() != mmprojFile ||
		request.GetModelPath() != filepath.Dir(modelFile) || request.GetNBatch() != localAIModelBatchSize {
		t.Fatalf("load request model=%q modelFile=%q mmproj=%q modelPath=%q nBatch=%d options=%v, want unchanged model, paths, directory, and batch size", request.GetModel(), request.GetModelFile(), request.GetMMProj(), request.GetModelPath(), request.GetNBatch(), request.GetOptions())
	}
	assertProjectorOption(t, request.GetOptions())
}

func assertWindowsCPUProjectorWire(t *testing.T, payload []byte, modelFile, mmprojFile string) {
	t.Helper()
	expected := appendStringField(nil, 1, models.BuiltInModelNameLLM)
	expected = appendVarintField(expected, 4, localAIModelBatchSize)
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
		t.Fatalf("projector options = %q, want exactly one option", options)
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
	if len(decoded.GetOptions()) != 0 {
		t.Fatalf("%s decoded LoadModel options = %q, want none", test.name, decoded.GetOptions())
	}
	if containsWireField(t, payload, 62) {
		t.Fatalf("%s LoadModel wire unexpectedly contains private field 62: %x", test.name, payload)
	}
}

func decodeLoadModelPayload(t *testing.T, payload []byte) ModelOptions {
	t.Helper()
	decoded := ModelOptions{}
	if err := proto.Unmarshal(payload, &decoded); err != nil {
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
