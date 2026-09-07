package wire

import (
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
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
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
