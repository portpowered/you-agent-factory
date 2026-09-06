package artifacts_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
)

func TestDefaultModelRoleManifestDeclaresExactTTSBundle(t *testing.T) {
	t.Parallel()

	manifest, err := artifacts.DefaultModelRoleManifest()
	if err != nil {
		t.Fatalf("DefaultModelRoleManifest: %v", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Kind != "localai-model-role-artifacts" {
		t.Fatalf("manifest identity = %#v, want schema 1 and private role kind", manifest)
	}
	model, ok := manifest.Model("tts")
	if !ok {
		t.Fatal("manifest is missing tts")
	}
	if model.Publication.Repository != "mudler/vibevoice.cpp-models" ||
		model.Publication.Revision != "a67807e65e3002e187179a856e96043f75060bc9" ||
		model.Publication.License != "MIT" ||
		model.Publication.BaseModel != "microsoft/VibeVoice-Realtime-0.5B" ||
		model.Source.URI != "hf://mudler/vibevoice.cpp-models/vibevoice-realtime-0.5B-q8_0.gguf@a67807e65e3002e187179a856e96043f75060bc9" {
		t.Fatalf("TTS publication/source = %#v, want exact immutable identity", model)
	}
	if model.Backend.ID != "localai-vibevoice" ||
		model.Backend.Repository != "https://github.com/mudler/vibevoice.cpp" ||
		model.Backend.Commit != "000e37282bc5bb09edc20f7047a47924122ba3a0" ||
		model.Backend.LocalAICommit != "b224c96db6f4b87306a33a808650bfce63b12588" ||
		model.Protocol.Path != "backend/backend.proto" ||
		model.Protocol.Revision != "ad62c6df07ae1169eb14411a565a689cd996b19c" ||
		strings.Join(model.Targets, ",") != "darwin-arm64,linux-amd64,windows-amd64" {
		t.Fatalf("TTS backend/protocol/targets = %#v, want exact private identity", model)
	}
	wantArtifacts := []struct {
		role, path, sha256 string
		bytes              int64
	}{
		{role: "model", path: "vibevoice-realtime-0.5B-q8_0.gguf", bytes: 1699832128, sha256: "5251e3f0386d1056a90c61b6c7359a4775da44dd19402499bef1989c4b5c653a"},
		{role: "tokenizer", path: "tokenizer.gguf", bytes: 5922368, sha256: "37dc3b722d5677e37e29a57df55aa05c485116eeb5459e57ff8dde616b4986f6"},
		{role: "voice", path: "voice-en-Carter_man.gguf", bytes: 8472448, sha256: "b15cd8b9cae6ee2c3d20b0ee6e7bfe93f13489f8b63b6834e9bbf0dfabf6505a"},
	}
	if len(model.Artifacts) != len(wantArtifacts) {
		t.Fatalf("TTS artifacts = %#v, want exact three-role bundle", model.Artifacts)
	}
	for index, want := range wantArtifacts {
		if got := model.Artifacts[index]; got.Role != want.role || got.Path != want.path || got.SizeBytes != want.bytes || got.SHA256 != want.sha256 {
			t.Fatalf("TTS artifact[%d] = %#v, want exact role identity %#v", index, got, want)
		}
	}
}

func TestDecodeModelRoleManifestRejectsMalformedPrivateFacts(t *testing.T) {
	t.Parallel()

	base, err := artifacts.DefaultModelRoleManifest()
	if err != nil {
		t.Fatalf("DefaultModelRoleManifest: %v", err)
	}
	baseBytes, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal base manifest: %v", err)
	}
	tests := []struct {
		name   string
		data   func() []byte
		assert func(t *testing.T, err error)
	}{
		{
			name: "invalid json",
			data: func() []byte { return []byte("{not-json") },
			assert: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, artifacts.ErrModelRoleManifestMalformed) {
					t.Fatalf("error = %v, want typed malformed sentinel", err)
				}
			},
		},
		{
			name: "unknown field",
			data: func() []byte {
				return append(append([]byte(nil), baseBytes[:len(baseBytes)-1]...), []byte(`,"unexpected":true}`)...)
			},
			assert: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, artifacts.ErrModelRoleManifestMalformed) {
					t.Fatalf("error = %v, want typed malformed sentinel", err)
				}
			},
		},
		{
			name: "traversal artifact",
			data: func() []byte {
				mutated := base
				mutated.Models[0].Artifacts[2].Path = "../voice.gguf"
				data, _ := json.Marshal(mutated)
				return data
			},
			assert: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, artifacts.ErrModelRoleManifestMalformed) {
					t.Fatalf("error = %v, want typed malformed sentinel", err)
				}
			},
		},
		{
			name: "source mismatch",
			data: func() []byte {
				mutated := base
				mutated.Models[0].Source.URI = "hf://mudler/vibevoice.cpp-models/other.gguf@a67807e65e3002e187179a856e96043f75060bc9"
				data, _ := json.Marshal(mutated)
				return data
			},
			assert: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, artifacts.ErrModelRoleManifestMalformed) {
					t.Fatalf("error = %v, want typed malformed sentinel", err)
				}
			},
		},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			_, err := artifacts.DecodeModelRoleManifest(testCase.data())
			if err == nil {
				t.Fatal("DecodeModelRoleManifest error = nil, want malformed manifest")
			}
			testCase.assert(t, err)
		})
	}
}

func TestModelRoleManifestReturnsDetachedModelSnapshots(t *testing.T) {
	t.Parallel()

	manifest, err := artifacts.DefaultModelRoleManifest()
	if err != nil {
		t.Fatalf("DefaultModelRoleManifest: %v", err)
	}
	first, ok := manifest.Model("tts")
	if !ok {
		t.Fatal("manifest is missing tts")
	}
	first.Targets[0] = "mutated"
	first.Artifacts[0].Path = "mutated.gguf"
	second, ok := manifest.Model("tts")
	if !ok {
		t.Fatal("manifest lost tts after detached mutation")
	}
	if second.Targets[0] == "mutated" || second.Artifacts[0].Path == "mutated.gguf" {
		t.Fatalf("manifest changed through detached model: %#v", second)
	}
}
