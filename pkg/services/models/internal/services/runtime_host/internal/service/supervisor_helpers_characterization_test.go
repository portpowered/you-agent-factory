package service

import (
	"path/filepath"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestBuiltInLLMResolvesPackagedGRPCHostStartSpec(t *testing.T) {
	t.Parallel()

	runtimeConfig := &models.RuntimeConfig{}
	worker, err := localWorkerForModel(runtimeConfig, models.BuiltInModelNameLLM)
	if err != nil {
		t.Fatalf("localWorkerForModel: %v", err)
	}
	if worker.Command != "" {
		t.Fatalf("built-in worker command = %q, want empty for packaged backend resolution", worker.Command)
	}

	identity := supervisedIdentityForModel(runtimeConfig, nil, models.BuiltInModelNameLLM)
	if identity.Backend != "localai-llamacpp" {
		t.Fatalf("built-in backend = %q, want localai-llamacpp", identity.Backend)
	}
	inspection := cacheInspection{
		CachePath:         `C:\models\llm\revision`,
		ObservedArtifacts: []models.AssetArtifact{{Name: "model.gguf"}},
		BackendRequired:   true,
		BackendFiles:      []string{`C:\models\backend\llama.zip`},
	}
	spec, err := defaultGRPCServerStartBuilderWithSymlinkResolver(identity, inspection, worker, nil)
	if err != nil {
		t.Fatalf("defaultGRPCServerStartBuilderWithSymlinkResolver: %v", err)
	}
	if spec.Command != "" || len(spec.Args) != 0 || spec.HealthEndpoint != "" {
		t.Fatalf("packaged backend start spec = %#v, want no authored process or endpoint", spec)
	}
	if spec.Backend != identity.Backend {
		t.Fatalf("spec backend = %q, want %q", spec.Backend, identity.Backend)
	}
	if spec.ModelPath != filepath.Join(inspection.CachePath, "model.gguf") {
		t.Fatalf("spec model path = %q, want %q", spec.ModelPath, filepath.Join(inspection.CachePath, "model.gguf"))
	}
	if len(spec.ModelFiles) != 1 || spec.ModelFiles[0] != spec.ModelPath {
		t.Fatalf("spec model files = %#v, want the verified model layout", spec.ModelFiles)
	}
	if len(spec.BackendFiles) != 1 || spec.BackendFiles[0] != inspection.BackendFiles[0] {
		t.Fatalf("spec backend files = %#v, want %#v", spec.BackendFiles, inspection.BackendFiles)
	}
}

func TestBuiltInTTSResolvesAllVerifiedModelFilesForPrivateHostNegotiation(t *testing.T) {
	t.Parallel()

	runtimeConfig := &models.RuntimeConfig{}
	worker, err := localWorkerForModel(runtimeConfig, models.BuiltInModelNameTTS)
	if err != nil {
		t.Fatalf("localWorkerForModel: %v", err)
	}
	identity := supervisedIdentityForModel(runtimeConfig, nil, models.BuiltInModelNameTTS)
	if identity.Backend != "localai-vibevoice" {
		t.Fatalf("built-in TTS backend = %q, want localai-vibevoice", identity.Backend)
	}
	inspection := cacheInspection{
		CachePath: `C:\models\tts\revision`,
		ObservedArtifacts: []models.AssetArtifact{
			{Name: "model.gguf"}, {Name: "tokenizer.gguf"}, {Name: "voice-en-Carter_man.gguf"},
		},
		BackendRequired: true,
		BackendFiles:    []string{`C:\models\backend\vibevoice.zip`},
	}
	spec, err := defaultGRPCServerStartBuilderWithSymlinkResolver(identity, inspection, worker, nil)
	if err != nil {
		t.Fatalf("defaultGRPCServerStartBuilderWithSymlinkResolver: %v", err)
	}
	wantFiles := []string{
		filepath.Join(inspection.CachePath, "model.gguf"),
		filepath.Join(inspection.CachePath, "tokenizer.gguf"),
		filepath.Join(inspection.CachePath, "voice-en-Carter_man.gguf"),
	}
	if spec.ModelPath != wantFiles[0] || len(spec.ModelFiles) != len(wantFiles) ||
		spec.ModelFiles[0] != wantFiles[0] || spec.ModelFiles[1] != wantFiles[1] || spec.ModelFiles[2] != wantFiles[2] {
		t.Fatalf("TTS model layout = path %q files %#v, want path %q files %#v", spec.ModelPath, spec.ModelFiles, wantFiles[0], wantFiles)
	}
}

func TestRequiresSupervisedBackend_CharacterizesCurrentMembership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		backend string
		want    bool
	}{
		{name: "canonical", backend: "LLAMACPP", want: true},
		{name: "case and surrounding whitespace", backend: "  llamaCpp \t", want: true},
		{name: "blank", backend: "", want: false},
		{name: "only whitespace", backend: " \t\n", want: false},
		{name: "unknown", backend: "GGUF", want: false},
		{name: "llamacpp artifact identifier", backend: "localai-llamacpp", want: false},
		{name: "whisper artifact identifier", backend: "localai-whisper", want: false},
		{name: "vibevoice artifact identifier", backend: "localai-vibevoice", want: false},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := requiresSupervisedBackend(testCase.backend)
			if got != testCase.want {
				t.Fatalf("requiresSupervisedBackend(%q) = %t, want %t", testCase.backend, got, testCase.want)
			}
		})
	}
}

func TestRequiresRuntimeHostBackend_PreservesPinnedBackendMembership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		backend string
		want    bool
	}{
		{name: "managed runtime alias", backend: "  llamaCpp ", want: true},
		{name: "pinned llama cpp artifact backend", backend: "localai-llamacpp", want: true},
		{name: "pinned whisper artifact backend", backend: "localai-whisper", want: true},
		{name: "pinned vibevoice artifact backend", backend: "localai-vibevoice", want: true},
		{name: "blank", backend: "", want: false},
		{name: "unknown", backend: "GGUF", want: false},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := requiresRuntimeHostBackend(testCase.backend)
			if got != testCase.want {
				t.Fatalf("requiresRuntimeHostBackend(%q) = %t, want %t", testCase.backend, got, testCase.want)
			}
		})
	}
}
