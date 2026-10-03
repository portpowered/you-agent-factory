package service

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func TestBuiltInLLMResolvesPackagedGRPCHostStartSpec(t *testing.T) {
	t.Parallel()

	runtimeConfig := &models.RuntimeConfig{}
	worker, err := localWorkerForModel(runtimeConfig, nil, models.BuiltInModelNameLLM)
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
		CachePath: `C:\models\llm\revision`,
		ExpectedArtifacts: []models.AssetRequirement{
			{Name: builtInLLMProjectorArtifactName, Bytes: builtInLLMProjectorArtifactBytes, SHA256: builtInLLMProjectorArtifactSHA256},
			{Name: builtInLLMModelArtifactName, Bytes: builtInLLMModelArtifactBytes, SHA256: builtInLLMModelArtifactSHA256},
		},
		ObservedArtifacts: []models.AssetArtifact{
			{Name: builtInLLMProjectorArtifactName, Bytes: builtInLLMProjectorArtifactBytes, SHA256: builtInLLMProjectorArtifactSHA256},
			{Name: builtInLLMModelArtifactName, Bytes: builtInLLMModelArtifactBytes, SHA256: builtInLLMModelArtifactSHA256},
		},
		IntegrityVerified: true,
		BackendRequired:   true,
		BackendFiles:      []string{`C:\models\backend\llama.zip`},
	}
	configuration, err := resolvedHostConfigurationFromInspection(
		models.RuntimeScopeRef{}, identity, inspection, models.AssetHostPlatform{}, nil,
	)
	if err != nil {
		t.Fatalf("resolvedHostConfigurationFromInspection: %v", err)
	}
	spec, err := defaultGRPCServerStartBuilderWithSymlinkResolver(configuration, worker)
	if err != nil {
		t.Fatalf("defaultGRPCServerStartBuilderWithSymlinkResolver: %v", err)
	}
	assertBuiltInLLMStartSpec(t, spec, identity, inspection)
}

func TestOperatorOverlaySuppliesWorkerForStandalonePinnedHost(t *testing.T) {
	t.Parallel()

	const name = "index-tts2.5"
	source := "file:///models/index-tts2_5.gguf"
	backend := "localai-audio-cpp"
	overlays := map[string]models.ModelOverlay{
		" INDEX-TTS2.5 ": {Source: &source, Backend: &backend},
	}
	worker, err := localWorkerForModel(&models.RuntimeConfig{}, overlays, name)
	if err != nil {
		t.Fatalf("localWorkerForModel: %v", err)
	}
	if worker.Name != name || worker.Model != name || worker.Type != models.RuntimeWorkerTypeInference ||
		worker.ModelLocality != models.RuntimeModelLocalityLocal || worker.Command != "" {
		t.Fatalf("operator worker = %#v, want local inference worker using packaged backend", worker)
	}
	configuration := modelseffects.ResolvedHostConfiguration{
		ModelName: name, Backend: backend,
		ModelCachePath: "/models/cache", BackendFiles: []string{"/backends/audio-cpp"},
	}
	spec, err := defaultGRPCServerStartBuilderWithSymlinkResolver(configuration, worker)
	if err != nil || spec.Command != "" || spec.Configuration.Backend != backend {
		t.Fatalf("packaged backend start spec = %#v, error = %v", spec, err)
	}
	if _, err := localWorkerForModel(&models.RuntimeConfig{}, nil, name); err == nil {
		t.Fatal("unconfigured model unexpectedly received a worker")
	}

	authored := models.RuntimeWorker{
		Name: "authored", Type: models.RuntimeWorkerTypeInference,
		Model: name, ModelLocality: models.RuntimeModelLocalityLocal,
		Command: "custom-backend",
	}
	worker, err = localWorkerForModel(&models.RuntimeConfig{Workers: []models.RuntimeWorker{authored}}, overlays, name)
	if err != nil || worker.Command != authored.Command {
		t.Fatalf("authored worker = %#v, error = %v, want explicit command", worker, err)
	}
}

func assertBuiltInLLMStartSpec(
	t *testing.T,
	spec modelseffects.HostProcessStartSpec,
	identity supervisedIdentity,
	inspection cacheInspection,
) {
	t.Helper()
	if spec.Command != "" || len(spec.Args) != 0 || spec.HealthEndpoint != "" {
		t.Fatalf("packaged backend start spec = %#v, want no authored process or endpoint", spec)
	}
	if spec.Configuration.Backend != identity.Backend {
		t.Fatalf("spec backend = %q, want %q", spec.Configuration.Backend, identity.Backend)
	}
	wantModelPath := filepath.Join(inspection.CachePath, builtInLLMModelArtifactName)
	wantMMProjPath := filepath.Join(inspection.CachePath, builtInLLMProjectorArtifactName)
	if spec.Configuration.ModelPath != wantModelPath {
		t.Fatalf("spec model path = %q, want %q", spec.Configuration.ModelPath, wantModelPath)
	}
	if spec.Configuration.MMProjPath != wantMMProjPath {
		t.Fatalf("spec mmproj path = %q, want %q", spec.Configuration.MMProjPath, wantMMProjPath)
	}
	wantFiles := []string{wantModelPath, wantMMProjPath}
	if len(spec.Configuration.ModelFiles) != len(wantFiles) || spec.Configuration.ModelFiles[0] != wantFiles[0] || spec.Configuration.ModelFiles[1] != wantFiles[1] {
		t.Fatalf("spec model files = %#v, want %#v", spec.Configuration.ModelFiles, wantFiles)
	}
	if len(spec.Configuration.BackendFiles) != 1 || spec.Configuration.BackendFiles[0] != inspection.BackendFiles[0] {
		t.Fatalf("spec backend files = %#v, want %#v", spec.Configuration.BackendFiles, inspection.BackendFiles)
	}
}

func TestBuiltInLLMRejectsInvalidProjectorMetadataBeforeHostStart(t *testing.T) {
	t.Parallel()

	base := cacheInspection{
		CachePath: `C:\models\llm\revision`,
		ExpectedArtifacts: []models.AssetRequirement{
			{Name: builtInLLMModelArtifactName, Bytes: builtInLLMModelArtifactBytes, SHA256: builtInLLMModelArtifactSHA256},
			{Name: builtInLLMProjectorArtifactName, Bytes: builtInLLMProjectorArtifactBytes, SHA256: builtInLLMProjectorArtifactSHA256},
		},
		ObservedArtifacts: []models.AssetArtifact{
			{Name: builtInLLMModelArtifactName, Bytes: builtInLLMModelArtifactBytes, SHA256: builtInLLMModelArtifactSHA256},
			{Name: builtInLLMProjectorArtifactName, Bytes: builtInLLMProjectorArtifactBytes, SHA256: builtInLLMProjectorArtifactSHA256},
		},
		IntegrityVerified: true,
	}
	definition, ok := models.BuiltInCatalog{}.ModelDefinitionFor(models.BuiltInModelNameLLM)
	if !ok {
		t.Fatal("built-in llm definition is missing")
	}
	identity := supervisedIdentity{
		Name: models.BuiltInModelNameLLM, Backend: "localai-llamacpp",
		Source: definition.Source,
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*cacheInspection)
	}{
		{name: "missing projector", mutate: func(inspection *cacheInspection) {
			inspection.ObservedArtifacts = inspection.ObservedArtifacts[:1]
		}},
		{name: "corrupt projector metadata", mutate: func(inspection *cacheInspection) {
			inspection.ObservedArtifacts[1].Bytes++
		}},
		{name: "duplicate projector", mutate: func(inspection *cacheInspection) {
			inspection.ObservedArtifacts = append(inspection.ObservedArtifacts, inspection.ObservedArtifacts[1])
		}},
		{name: "escaping projector", mutate: func(inspection *cacheInspection) {
			inspection.ObservedArtifacts[1].Name = "../" + builtInLLMProjectorArtifactName
		}},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			inspection := base
			inspection.ExpectedArtifacts = append([]models.AssetRequirement(nil), base.ExpectedArtifacts...)
			inspection.ObservedArtifacts = append([]models.AssetArtifact(nil), base.ObservedArtifacts...)
			testCase.mutate(&inspection)
			_, err := resolvedHostConfigurationFromInspection(
				models.RuntimeScopeRef{}, identity, inspection, models.AssetHostPlatform{}, nil,
			)
			if !errors.Is(err, models.ErrHostMissingAssets) {
				t.Fatalf("builder error = %v, want ErrHostMissingAssets", err)
			}
		})
	}
}

func TestBuiltInTTSResolvesAllVerifiedModelFilesForPrivateHostNegotiation(t *testing.T) {
	t.Parallel()

	runtimeConfig := &models.RuntimeConfig{}
	worker, err := localWorkerForModel(runtimeConfig, nil, models.BuiltInModelNameTTS)
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
	configuration, err := resolvedHostConfigurationFromInspection(
		models.RuntimeScopeRef{}, identity, inspection, models.AssetHostPlatform{}, nil,
	)
	if err != nil {
		t.Fatalf("resolvedHostConfigurationFromInspection: %v", err)
	}
	spec, err := defaultGRPCServerStartBuilderWithSymlinkResolver(configuration, worker)
	if err != nil {
		t.Fatalf("defaultGRPCServerStartBuilderWithSymlinkResolver: %v", err)
	}
	wantFiles := []string{
		filepath.Join(inspection.CachePath, "model.gguf"),
		filepath.Join(inspection.CachePath, "tokenizer.gguf"),
		filepath.Join(inspection.CachePath, "voice-en-Carter_man.gguf"),
	}
	if spec.Configuration.ModelPath != wantFiles[0] || len(spec.Configuration.ModelFiles) != len(wantFiles) ||
		spec.Configuration.ModelFiles[0] != wantFiles[0] || spec.Configuration.ModelFiles[1] != wantFiles[1] || spec.Configuration.ModelFiles[2] != wantFiles[2] {
		t.Fatalf("TTS model layout = path %q files %#v, want path %q files %#v", spec.Configuration.ModelPath, spec.Configuration.ModelFiles, wantFiles[0], wantFiles)
	}
}

func TestSupervisedIdentityCanonicalizesPinnedBackendIdentifiers(t *testing.T) {
	t.Parallel()

	runtimeConfig := &models.RuntimeConfig{
		Resources: []models.RuntimeResource{{
			Type:       models.RuntimeResourceTypeModel,
			Model:      models.BuiltInModelNameTTS,
			Backend:    " LOCALAI-VIBEVOICE ",
			LoadPolicy: string(models.LoadPolicyOnDemand),
		}},
	}

	identity := supervisedIdentityForModel(runtimeConfig, nil, models.BuiltInModelNameTTS)
	if identity.Backend != "localai-vibevoice" {
		t.Fatalf("supervised identity backend = %q, want canonical pinned identifier", identity.Backend)
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

func TestNewInertRuntimeHostAppliesDefaultReadinessTimeout(t *testing.T) {
	t.Parallel()

	hostService, ok := New(nil, nil, nil, NewSlotState(), nil, nil, nil, nil, nil, models.AssetHostPlatform{}, nil, nil, nil, nil, 0, 0).(*service)
	if !ok {
		t.Fatal("New did not return the internal runtime host service implementation")
	}
	if hostService.supervisor.ReadinessTimeout != 5*time.Minute {
		t.Fatalf(
			"supervised readiness timeout = %s, want 5m so large cold model loads are not killed at 30s",
			hostService.supervisor.ReadinessTimeout,
		)
	}
	if hostService.supervisor.HealthCheckInterval != DefaultHealthCheckInterval {
		t.Fatalf(
			"health check interval = %s, want unchanged default %s",
			hostService.supervisor.HealthCheckInterval,
			DefaultHealthCheckInterval,
		)
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
