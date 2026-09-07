package wire

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	managedbackend "github.com/portpowered/infinite-you/pkg/wire/internal/managedbackend"
)

func TestProvideProviderRegistryComposesBuiltIns(t *testing.T) {
	t.Parallel()

	providersService, err := provideProvidersService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	providers, err := provideProviderRegistry(serviceedges.Edges{}, providersService)
	if err != nil {
		t.Fatalf("provideProviderRegistry() error = %v", err)
	}
	canonical, err := providers.CanonicalIdentity("codex")
	if err != nil {
		t.Fatalf("CanonicalIdentity(codex) error = %v", err)
	}
	if canonical != "codex" {
		t.Fatalf("CanonicalIdentity(codex) = %q, want codex", canonical)
	}
}

func TestProvideResponsePresentationReturnsUsableInjectedService(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	presentationOutput := provideResponsePresentation().OpenLosslessOutput(&output)
	if err := presentationOutput.Enqueue([]byte("factory event")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := presentationOutput.CloseAndDrain(); err != nil {
		t.Fatalf("CloseAndDrain: %v", err)
	}
	if got, want := output.String(), "factory event\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestProvideWorkStopSummaryProjectorDelegatesToFactorySessions(t *testing.T) {
	t.Parallel()
	if got := provideWorkStopSummaryProjector()(factorysessions.WorkStopSummaryRequest{}); got != nil {
		t.Fatalf("empty Work stop summary = %#v, want nil", got)
	}
}

func TestFactoryRuntimeClockResolverPreservesOverrideAndSelectsPlatformDefault(t *testing.T) {
	t.Parallel()

	resolver := provideFactoryRuntimeClockResolver()
	override := &wireTestClock{}
	if got := resolver(override); got != override {
		t.Fatalf("resolved override = %#v, want original clock", got)
	}
	if _, ok := resolver(nil).(platformclock.Real); !ok {
		t.Fatalf("resolved default = %T, want platform clock", resolver(nil))
	}
}

type wireTestClock struct{}

func (*wireTestClock) Now() time.Time {
	return time.Time{}
}

func TestFactorySessionsAssemblyRequiresRuntimeClockBinding(t *testing.T) {
	t.Parallel()
	namedPathResolver, err := factorydefinitionswire.NewPathResolver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct named-path resolver: %v", err)
	}
	eventsService, err := eventswire.NewService()
	if err != nil {
		t.Fatalf("construct events service: %v", err)
	}

	assembly, err := provideFactorySessionsAssembly(
		factoryruntime.NewSessionResultProjectionOperation(),
		nil,
		nil,
		nil,
		func() string { return "response-event-test-id" },
		nil,
		func() string { return "session-test-id" },
		func() (string, error) { return t.TempDir(), nil },
		platformfilesystem.Local{},
		namedPathResolver,
		factorysessionwire.InvocationInputReader(func(string) ([]byte, error) { return nil, nil }),
		factorysessionwire.InitialWorkReader(func(string) ([]byte, error) { return nil, nil }),
		func(path string) (string, error) { return path, nil },
		eventsService,
		&wireTestClock{},
		factorysessionwire.NewLiveChangeCoordinator(),
		nil,
	)
	if err != nil {
		t.Fatalf("provide Factory Sessions assembly: %v", err)
	}
	if assembly == nil {
		t.Fatal("Factory Sessions assembly is nil")
	}
}

func TestWirePackageExposesOnlyCanonicalApplicationInjector(t *testing.T) {
	t.Parallel()

	names := map[string]struct{}{}
	parseProductionGoFiles(t, ".", func(_ string, file *ast.File) {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && strings.HasPrefix(function.Name.Name, "Inject") {
				names[function.Name.Name] = struct{}{}
			}
		}
	})
	if len(names) != 1 {
		t.Fatalf("Wire injector names = %v, want only InjectBundle", names)
	}
	if _, ok := names["InjectBundle"]; !ok {
		t.Fatalf("Wire injector names = %v, want InjectBundle", names)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestLegacyRuntimeBuilderAndRuntimeBundleCannotReturn(t *testing.T) {
	t.Parallel()

	runtimeApplicationDir := filepath.Join("..", "initializer", "runtimeapplication")
	forbiddenDeclarations := map[string]struct{}{
		"RuntimeBuilder":               {},
		"RuntimeFactory":               {},
		"NewRuntimeFactory":            {},
		"NewRuntimeFactoryFromOpening": {},
		"BuildRuntimeScope":            {},
	}
	parseProductionGoFiles(t, runtimeApplicationDir, func(path string, file *ast.File) {
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if _, forbidden := forbiddenDeclarations[value.Name.Name]; forbidden {
					t.Errorf("%s declares legacy composition seam %s", path, value.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if _, forbidden := forbiddenDeclarations[typeSpec.Name.Name]; forbidden {
						t.Errorf("%s declares legacy composition seam %s", path, typeSpec.Name.Name)
					}
				}
			}
		}
	})

	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".claude" || entry.Name() == "vendor" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if importPath == "github.com/portpowered/infinite-you/pkg/wire/runtimebundle" ||
				strings.HasPrefix(importPath, "github.com/portpowered/infinite-you/pkg/wire/runtimebundle/") {
				t.Errorf("%s imports deleted secondary composition package %s", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan repository imports: %v", err)
	}
}

func TestFactoryRuntimeAssemblyCallbackCannotReturn(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".claude", "node_modules", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range general.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && typeSpec.Name.Name == "FactoryRuntimeAssemblyFactory" {
					t.Errorf("%s declares deleted Factory Runtime assembly callback", path)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan repository for deleted Factory Runtime assembly callback: %v", err)
	}
}

func TestRunTransportCannotRecreateAnApplicationBuilder(t *testing.T) {
	t.Parallel()

	forbiddenDeclarations := map[string]struct{}{
		"Application":        {},
		"ApplicationBuilder": {},
		"BuildApplication":   {},
	}
	parseProductionGoFiles(t, filepath.Join("..", "transports", "cli", "run"), func(path string, file *ast.File) {
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if _, forbidden := forbiddenDeclarations[value.Name.Name]; forbidden {
					t.Errorf("%s declares alternate application construction seam %s", path, value.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if _, forbidden := forbiddenDeclarations[typeSpec.Name.Name]; forbidden {
						t.Errorf("%s declares alternate application construction seam %s", path, typeSpec.Name.Name)
					}
				}
			}
		}
	})
}

// pss-cln-run-fold-engine-pipeline-007: root pkg/wire must reach Factory Runtime
// only through published root contracts and factory_runtime/wire assembly seams.
func TestRootWireImportsFactoryRuntimeThroughPublishedSeamsOnly(t *testing.T) {
	t.Parallel()

	const (
		factoryRuntimeRootImport = "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
		factoryRuntimeWireImport = factoryRuntimeRootImport + "/wire"
	)

	parseProductionGoFiles(t, ".", func(path string, file *ast.File) {
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			if !strings.HasPrefix(importPath, factoryRuntimeRootImport) {
				continue
			}
			if importPath == factoryRuntimeRootImport ||
				importPath == factoryRuntimeWireImport ||
				strings.HasPrefix(importPath, factoryRuntimeWireImport+"/") {
				continue
			}
			t.Fatalf(
				"%s imports forbidden Factory Runtime owner-private path %s; use factory_runtime root contracts and factory_runtime/wire only",
				path,
				importPath,
			)
		}
	})
}

func parseProductionGoFiles(
	t *testing.T,
	dir string,
	visit func(string, *ast.File),
) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		visit(path, file)
	}
}

func TestAppendManagedBackendEnvironmentPreservesExplicitValuesAndReplacesKeys(t *testing.T) {
	t.Parallel()

	base := []string{
		"PATH=C:\\runtime",
		"VIBEVOICECPP_LIBRARY=C:\\stale\\library.dll",
		"MODEL=tts",
	}
	got := appendManagedBackendEnvironment(base, []string{
		"vibevoicecpp_library=C:\\managed\\library.dll",
		"MODEL_ROOT=C:\\models",
	})
	if len(got) != len(base)+1 {
		t.Fatalf("merged environment length = %d, want %d: %#v", len(got), len(base)+1, got)
	}
	if got[0] != base[0] || got[2] != base[2] {
		t.Fatalf("merged environment changed unrelated values: %#v", got)
	}
	if got[1] != "vibevoicecpp_library=C:\\managed\\library.dll" {
		t.Fatalf("merged environment did not replace case-insensitive library key: %#v", got)
	}
	if got[3] != "MODEL_ROOT=C:\\models" {
		t.Fatalf("merged environment omitted new value: %#v", got)
	}
}

func TestAppendManagedBackendEnvironmentCollapsesCaseInsensitiveDuplicates(t *testing.T) {
	t.Parallel()

	got := appendManagedBackendEnvironment(
		[]string{
			"PATH=C:\\runtime",
			"VIBEVOICECPP_LIBRARY=C:\\stale\\first.dll",
			"vibevoicecpp_library=C:\\stale\\second.dll",
			"TEMP=C:\\temp",
		},
		[]string{"VibeVoiceCpp_Library=C:\\managed\\library.dll"},
	)
	var libraries []string
	for _, entry := range got {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "VIBEVOICECPP_LIBRARY") {
			libraries = append(libraries, value)
		}
	}
	if len(libraries) != 1 || libraries[0] != `C:\managed\library.dll` {
		t.Fatalf("merged VibeVoice values = %#v, want one managed value", libraries)
	}
	if !containsEnvironmentValue(got, "PATH", `C:\runtime`) ||
		!containsEnvironmentValue(got, "TEMP", `C:\temp`) {
		t.Fatalf("merged environment dropped unrelated values: %#v", got)
	}
}

func TestManagedEnvironmentFactsUseOnlyAllowlistedValueDigests(t *testing.T) {
	t.Parallel()

	secretPath := `C:\isolated\private-model.gguf`
	secretToken := "token=private-value"
	managedPath := `C:\managed\libgovibevoicecpp.dll`
	facts := managedEnvironmentFacts([]string{
		"PATH=C:\\runtime",
		"TEMP=C:\\temp",
		"TMP=C:\\temp",
		"VIBEVOICECPP_LIBRARY=" + managedPath,
		"MODEL_SECRET=" + secretToken,
		"MODEL_PATH=" + secretPath,
	})
	body, err := json.Marshal(facts)
	if err != nil {
		t.Fatalf("marshal managed environment facts: %v", err)
	}
	serialized := string(body)
	for _, marker := range []string{secretPath, secretToken, "MODEL_SECRET", "MODEL_PATH"} {
		if strings.Contains(serialized, marker) {
			t.Fatalf("managed environment facts leaked %q: %s", marker, serialized)
		}
	}
	want := map[string]string{
		"PATH":                 environmentValueSHA256(`C:\runtime`),
		"TEMP":                 environmentValueSHA256(`C:\temp`),
		"TMP":                  environmentValueSHA256(`C:\temp`),
		"VIBEVOICECPP_LIBRARY": environmentValueSHA256(managedPath),
	}
	if len(facts) != len(want) {
		t.Fatalf("managed environment facts = %#v, want four allowlisted facts", facts)
	}
	for _, fact := range facts {
		if !fact.Present || fact.ValueSHA256 != want[fact.Name] {
			t.Fatalf("managed environment fact = %#v, want digest for %q", fact, fact.Name)
		}
	}
}

func TestManagedChildEvidenceUsesBoundedIdentityAndSharedSequence(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "runtime.jsonl")
	recorder := &modelRuntimeEvidenceFileRecorder{path: path}
	recorder.RecordRuntimeEvidence(modelswire.RuntimeEvidenceRecord{
		Kind:           "STAGE",
		Stage:          "PROTOCOL_LOAD",
		Outcome:        "COMPLETED",
		DurationMillis: 1,
	})
	recorder.RecordManagedChildEnvironment(managedChildEnvironmentEvidence{
		Kind:      managedChildEvidenceKind,
		Backend:   boundedManagedBackendID(`C:\private\backend.exe`),
		ProcessID: 42,
		Phase:     managedChildPhaseStarted,
	})
	recorder.RecordManagedChildEnvironment(managedChildEnvironmentEvidence{
		Kind:      managedChildEvidenceKind,
		Backend:   "localai-vibevoice",
		ProcessID: 42,
		Phase:     managedChildPhaseExited,
		ExitClass: managedChildExitClassNonzero,
	})

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shared runtime evidence: %v", err)
	}
	records := decodeManagedChildEvidence(t, body)
	if len(records) != 3 || records[0].Sequence != 1 || records[1].Sequence != 2 || records[2].Sequence != 3 {
		t.Fatalf("shared runtime evidence records = %#v, want three ordered lines", records)
	}
	if records[1].Backend != "UNKNOWN" {
		t.Fatalf("unbounded backend identity = %q, want UNKNOWN", records[1].Backend)
	}
	if strings.Contains(string(body), `C:\private\backend.exe`) {
		t.Fatalf("shared runtime evidence leaked raw backend identity: %s", body)
	}
}

type managedChildEvidenceLine struct {
	Sequence uint64 `json:"sequence"`
	Kind     string `json:"kind"`
	Backend  string `json:"backend"`
	Phase    string `json:"phase"`
}

func decodeManagedChildEvidence(t *testing.T, body []byte) []managedChildEvidenceLine {
	t.Helper()
	var records []managedChildEvidenceLine
	decoder := json.NewDecoder(bytes.NewReader(body))
	for {
		var record managedChildEvidenceLine
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode shared runtime evidence: %v", err)
		}
		records = append(records, record)
	}
	return records
}

func TestModelsProcessLauncherStartFailureDoesNotEmitChildEvidence(t *testing.T) {
	t.Parallel()

	evidencePath := filepath.Join(t.TempDir(), "runtime.jsonl")
	recorder := &modelRuntimeEvidenceFileRecorder{path: evidencePath}
	missingCommand := filepath.Join(t.TempDir(), "missing-model-backend.exe")
	_, err := (modelsProcessLauncher{recorder: recorder}).Start(
		context.Background(),
		serviceedges.HostProcessStartSpec{
			Command:        missingCommand,
			Backend:        "localai-vibevoice",
			HealthEndpoint: "grpc://127.0.0.1:1",
		},
	)
	if err == nil {
		t.Fatal("missing managed backend start error = nil, want typed start failure")
	}
	var classifier interface {
		ModelRuntimeStage() string
		ModelRuntimeFailureClass() string
	}
	if !errors.As(err, &classifier) || classifier == nil ||
		classifier.ModelRuntimeStage() != "BACKEND_START" ||
		classifier.ModelRuntimeFailureClass() != "PROCESS_START_FAILED" {
		t.Fatalf("start failure classification = %v, want BACKEND_START/PROCESS_START_FAILED", err)
	}
	if strings.Contains(err.Error(), missingCommand) {
		t.Fatalf("start failure leaked command path: %q", err.Error())
	}
	body, readErr := os.ReadFile(evidencePath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read start failure evidence: %v", readErr)
	}
	if len(bytes.TrimSpace(body)) != 0 {
		t.Fatalf("start failure emitted false child evidence: %s", body)
	}
}

func TestProvideModelRuntimeEvidenceRecorderIsOptionalAndOwnerOnlyJSONL(t *testing.T) {
	t.Setenv(modelRuntimeEvidenceEnvironment, "")
	if recorder, err := provideModelRuntimeEvidenceRecorder(); err != nil || recorder != nil {
		t.Fatalf("absent runtime evidence recorder = (%v, %v), want (nil, nil)", recorder, err)
	}

	path := filepath.Join(t.TempDir(), "runtime.jsonl")
	t.Setenv(modelRuntimeEvidenceEnvironment, path)
	recorder, err := provideModelRuntimeEvidenceRecorder()
	if err != nil {
		t.Fatalf("provide runtime evidence recorder: %v", err)
	}
	if recorder == nil {
		t.Fatal("configured runtime evidence recorder is nil")
	}
	recorder.RecordRuntimeEvidence(modelswire.RuntimeEvidenceRecord{
		Kind:           "STAGE",
		Stage:          "PROTOCOL_LOAD",
		Outcome:        "FAILED",
		Class:          "PROTOCOL_INCOMPATIBLE",
		CauseSHA256:    strings.Repeat("a", 64),
		DurationMillis: 7,
	})
	assertRuntimeEvidenceFile(t, path)
}

func assertRuntimeEvidenceFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat runtime evidence file: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		got := info.Mode().Perm()
		t.Fatalf("runtime evidence permissions = %o, want owner-only 600", got)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime evidence file: %v", err)
	}
	if bytes.Count(body, []byte{'\n'}) != 1 {
		t.Fatalf("runtime evidence lines = %d, want one JSONL record", bytes.Count(body, []byte{'\n'}))
	}
	var got modelswire.RuntimeEvidenceRecord
	if err := json.Unmarshal(bytes.TrimSpace(body), &got); err != nil {
		t.Fatalf("decode runtime evidence record: %v", err)
	}
	if got.Sequence != 1 || got.Kind != "STAGE" || got.Stage != "PROTOCOL_LOAD" ||
		got.Outcome != "FAILED" || got.Class != "PROTOCOL_INCOMPATIBLE" {
		t.Fatalf("runtime evidence record = %#v, want ordered bounded record", got)
	}
}

func TestProvideModelRuntimeEvidenceRecorderRejectsRelativePath(t *testing.T) {
	t.Setenv(modelRuntimeEvidenceEnvironment, "runtime.jsonl")
	if recorder, err := provideModelRuntimeEvidenceRecorder(); recorder != nil || err == nil {
		t.Fatalf("relative runtime evidence path = (%v, %v), want error and nil", recorder, err)
	}
}

func containsEnvironmentValue(environment []string, name, want string) bool {
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, name) && value == want {
			return true
		}
	}
	return false
}

func environmentValueSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

const (
	exactVibeVoiceArchiveEnvironment = "INFINITE_YOU_PINNED_TTS_BACKEND_ARCHIVE"
	exactVibeVoiceArchiveBytes       = int64(10757902)
	exactVibeVoiceArchiveSHA256      = "8f3c14212948be34c930e9a790af7757460cb2f6bb6a0de80d5b9f95b71e8646"
	exactVibeVoiceProtocolVersion    = "localai-backend-v1"
)

// TestVerifiedVibeVoiceArchiveSurvivesCacheRuntimeMaterializationHandoff
// crosses the production Models composition and replaces only the OS process
// edge after the real managedbackend resolver. The exact published archive is
// supplied by the bounded local-real evidence environment.
func TestVerifiedVibeVoiceArchiveSurvivesCacheRuntimeMaterializationHandoff(t *testing.T) {
	t.Parallel()

	archivePath := strings.TrimSpace(os.Getenv(exactVibeVoiceArchiveEnvironment))
	if archivePath == "" {
		t.Skip("exact published VibeVoice archive is not configured")
	}
	archiveFacts := verifyExactVibeVoiceArchive(t, archivePath)
	modelPath, modelBody := writeControlledArchiveModel(t)
	service, scope, launcher := openVerifiedArchiveScope(t)
	prepareVerifiedArchiveAssets(t, service, scope, archivePath, archiveFacts, modelPath, modelBody)
	ensureVerifiedArchiveHost(t, service, scope)
	observation := launcher.snapshot()
	assertVerifiedArchiveLaunch(t, observation, archiveFacts)
	stopVerifiedArchiveHost(t, service, scope, observation.launch.WorkDir)
}

func writeControlledArchiveModel(t *testing.T) (string, []byte) {
	t.Helper()
	modelRoot := t.TempDir()
	modelPath := filepath.Join(modelRoot, "model.gguf")
	modelBody := []byte("controlled model payload")
	if err := os.WriteFile(modelPath, modelBody, 0o600); err != nil {
		t.Fatalf("write controlled model artifact: %v", err)
	}
	return modelPath, modelBody
}

func openVerifiedArchiveScope(t *testing.T) (models.Service, models.RuntimeScopeRef, *verifiedArchiveProcessLauncher) {
	t.Helper()
	launcher := &verifiedArchiveProcessLauncher{}
	service, err := provideModelsService(serviceedges.Edges{
		ModelAssetHostPlatform: models.AssetHostPlatform{
			OperatingSystem: "windows",
			Architecture:    "amd64",
		},
		ModelHostProcessLauncher:      launcher,
		ModelHostProtocolNegotiator:   verifiedArchiveProtocolNegotiator{},
		ModelHostCompatibilityChecker: verifiedArchiveCompatibilityChecker{},
	})
	if err != nil {
		t.Fatalf("provideModelsService: %v", err)
	}
	cacheDirectory := t.TempDir()
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			CacheDirectory: cacheDirectory,
			Runtime: models.RuntimeConfig{
				Workers: []models.RuntimeWorker{{
					Name:          "vibevoice-worker",
					Type:          models.RuntimeWorkerTypeInference,
					Model:         "joined-model",
					ModelLocality: models.RuntimeModelLocalityLocal,
				}},
				Resources: []models.RuntimeResource{{
					Type:       models.RuntimeResourceTypeModel,
					Model:      "joined-model",
					Backend:    "localai-vibevoice",
					Capacity:   1,
					LoadPolicy: string(models.LoadPolicyKeepWarm),
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	t.Cleanup(func() {
		_, _ = service.StopModelHost(context.Background(), models.StopModelHostRequest{
			Scope: opened.Scope, Name: "joined-model",
		})
		_, _ = service.CloseRuntimeScope(context.Background(), models.CloseRuntimeScopeRequest{Scope: opened.Scope})
	})
	return service, opened.Scope, launcher
}

func prepareVerifiedArchiveAssets(
	t *testing.T,
	service models.Service,
	scope models.RuntimeScopeRef,
	archivePath string,
	archiveFacts exactArchiveFacts,
	modelPath string,
	modelBody []byte,
) {
	t.Helper()
	prepared, err := service.PrepareModelAssets(context.Background(), models.PrepareModelAssetsRequest{
		Scope:     scope,
		Name:      "joined-model",
		Reference: models.ModelReference{NameOrURI: modelPath},
		Artifacts: []models.AssetRequirement{{
			Name: "model.gguf", Bytes: int64(len(modelBody)), SHA256: sha256HexForArchiveTest(modelBody),
		}},
		Backend:          "localai-vibevoice",
		BackendReference: models.ModelReference{NameOrURI: archivePath},
		BackendArtifacts: []models.AssetRequirement{{
			Name: filepath.Base(archivePath), Bytes: archiveFacts.bytes, SHA256: archiveFacts.sha256,
		}},
	})
	if err != nil {
		t.Fatalf("PrepareModelAssets: %v", err)
	}
	if prepared.Asset.Readiness != models.AssetReadinessAvailable ||
		prepared.Asset.Integrity != models.AssetIntegrityVerified ||
		len(prepared.Asset.BackendArtifacts) != 1 {
		t.Fatalf("prepared asset = %#v, want verified model and backend snapshot", prepared.Asset)
	}
}

func ensureVerifiedArchiveHost(t *testing.T, service models.Service, scope models.RuntimeScopeRef) {
	t.Helper()
	ensured, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: scope, Name: "joined-model",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	if ensured.Host.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("host readiness = %q, want READY", ensured.Host.ReadinessState)
	}
}

func assertVerifiedArchiveLaunch(t *testing.T, observation struct {
	launch       managedbackend.ManagedBackendLaunch
	backendFiles []string
}, archiveFacts exactArchiveFacts) {
	t.Helper()
	if len(observation.backendFiles) == 0 {
		t.Fatal("managed backend launcher observed no BackendFiles")
	}
	for _, path := range observation.backendFiles {
		if !filepath.IsAbs(path) || strings.Contains(path, ".partial") {
			t.Fatalf("BackendFiles path = %q, want absolute committed path", path)
		}
		info, statErr := os.Stat(path)
		if statErr != nil || info == nil || !info.Mode().IsRegular() {
			t.Fatalf("BackendFiles path stat = (%#v, %v), want live regular file", info, statErr)
		}
	}
	if observation.launch.Command == "" || !filepath.IsAbs(observation.launch.Command) {
		t.Fatalf("materialized command = %q, want absolute executable", observation.launch.Command)
	}
	for _, entry := range []string{
		"vibevoice-cpp.exe", "libgomp-1.dll", "libgovibevoicecpp.dll", "libwinpthread-1.dll",
	} {
		if !archiveFacts.entries[entry] {
			t.Fatalf("exact archive omitted required entry %q", entry)
		}
		path := filepath.Join(observation.launch.WorkDir, entry)
		info, statErr := os.Stat(path)
		if statErr != nil || info == nil || !info.Mode().IsRegular() {
			t.Fatalf("materialized %q stat = (%#v, %v), want live regular file", entry, info, statErr)
		}
	}
}

func stopVerifiedArchiveHost(t *testing.T, service models.Service, scope models.RuntimeScopeRef, workDir string) {
	t.Helper()
	peer := filepath.Join(filepath.Dir(workDir), "archive-materialization-peer-sentinel")
	if err := os.WriteFile(peer, []byte("peer"), 0o600); err != nil {
		t.Fatalf("write peer sentinel: %v", err)
	}
	stopped, err := service.StopModelHost(context.Background(), models.StopModelHostRequest{
		Scope: scope, Name: "joined-model",
	})
	if err != nil {
		t.Fatalf("StopModelHost: %v", err)
	}
	if stopped.Host.LifecycleState == models.LifecycleStateLoaded {
		t.Fatalf("stopped host lifecycle = %q, want unloaded", stopped.Host.LifecycleState)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("owned extraction root stat after stop = %v, want removed", err)
	}
	if body, err := os.ReadFile(peer); err != nil || string(body) != "peer" {
		t.Fatalf("peer sentinel = (%q, %v), want unchanged", body, err)
	}
}

type exactArchiveFacts struct {
	bytes   int64
	sha256  string
	entries map[string]bool
}

func verifyExactVibeVoiceArchive(t *testing.T, path string) exactArchiveFacts {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info == nil || !info.Mode().IsRegular() {
		t.Fatalf("exact archive stat = (%#v, %v), want regular file", info, err)
	}
	if info.Size() != exactVibeVoiceArchiveBytes {
		t.Fatalf("exact archive bytes = %d, want %d", info.Size(), exactVibeVoiceArchiveBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open exact archive: %v", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		_ = file.Close()
		t.Fatalf("hash exact archive: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close exact archive: %v", err)
	}
	digest := hex.EncodeToString(hasher.Sum(nil))
	if digest != exactVibeVoiceArchiveSHA256 {
		t.Fatalf("exact archive SHA-256 = %s, want %s", digest, exactVibeVoiceArchiveSHA256)
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open exact archive as ZIP: %v", err)
	}
	defer reader.Close()
	entries := make(map[string]bool, len(reader.File))
	for _, file := range reader.File {
		entries[strings.ToLower(filepath.Base(file.Name))] = true
	}
	expectedEntries := map[string]bool{
		"build-metadata.json":   true,
		"libgomp-1.dll":         true,
		"libgovibevoicecpp.dll": true,
		"libwinpthread-1.dll":   true,
		"vibevoice-cpp.exe":     true,
	}
	if len(entries) != len(expectedEntries) {
		t.Fatalf("exact archive entry count = %d, want %d (%#v)", len(entries), len(expectedEntries), entries)
	}
	for name := range expectedEntries {
		if !entries[name] {
			t.Fatalf("exact archive entry set omitted %q: %#v", name, entries)
		}
	}
	return exactArchiveFacts{bytes: info.Size(), sha256: digest, entries: entries}
}

func sha256HexForArchiveTest(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

type verifiedArchiveProcessLauncher struct {
	mu           sync.Mutex
	launch       managedbackend.ManagedBackendLaunch
	backendFiles []string
	process      *verifiedArchiveManagedProcess
}

func (launcher *verifiedArchiveProcessLauncher) Start(
	ctx context.Context,
	spec serviceedges.HostProcessStartSpec,
) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launch, err := managedbackend.ResolveManagedBackendLaunch(ctx, spec)
	if err != nil {
		return nil, err
	}
	process := &verifiedArchiveManagedProcess{
		endpoint: launch.Endpoint,
		cleanup:  launch.Cleanup,
		done:     make(chan struct{}),
	}
	launcher.mu.Lock()
	launcher.launch = launch
	launcher.backendFiles = append([]string(nil), spec.BackendFiles...)
	launcher.process = process
	launcher.mu.Unlock()
	return process, nil
}

func (launcher *verifiedArchiveProcessLauncher) snapshot() struct {
	launch       managedbackend.ManagedBackendLaunch
	backendFiles []string
} {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return struct {
		launch       managedbackend.ManagedBackendLaunch
		backendFiles []string
	}{launch: launcher.launch, backendFiles: append([]string(nil), launcher.backendFiles...)}
}

type verifiedArchiveManagedProcess struct {
	mu       sync.Mutex
	endpoint string
	cleanup  func() error
	done     chan struct{}
	stopOnce sync.Once
	stopErr  error
}

func (process *verifiedArchiveManagedProcess) HealthEndpoint() string {
	return process.endpoint
}

func (process *verifiedArchiveManagedProcess) Wait() error {
	<-process.done
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.stopErr
}

func (process *verifiedArchiveManagedProcess) Stop(context.Context) error {
	process.stopOnce.Do(func() {
		process.mu.Lock()
		process.stopErr = process.cleanup()
		process.mu.Unlock()
		close(process.done)
	})
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.stopErr
}

type verifiedArchiveProtocolNegotiator struct{}

func (verifiedArchiveProtocolNegotiator) Negotiate(
	context.Context,
	string,
	serviceedges.ModelHostProtocolNegotiationRequest,
) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	return serviceedges.ModelHostProtocolNegotiationResult{
		ProtocolVersion: exactVibeVoiceProtocolVersion,
		Backend:         "localai-vibevoice",
		Ready:           true,
	}, nil
}

type verifiedArchiveCompatibilityChecker struct{}

func (verifiedArchiveCompatibilityChecker) Check(context.Context, serviceedges.ModelHostCompatibilityRequest) error {
	return nil
}

func TestModelsManagedProcessRetainsCleanupErrorOnce(t *testing.T) {
	t.Parallel()

	cleanupErr := errors.New("bounded cleanup failure")
	cleanupCalls := 0
	process := &modelsManagedProcess{
		cleanup: func() error {
			cleanupCalls++
			return cleanupErr
		},
		finished: make(chan struct{}),
	}
	close(process.finished)

	process.cleanupResources()
	process.cleanupResources()
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want once", cleanupCalls)
	}
	if err := process.Wait(); !errors.Is(err, cleanupErr) {
		t.Fatalf("process wait error = %v, want retained cleanup error", err)
	}
}
