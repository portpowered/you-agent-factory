package wire

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
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
	canonical, err := providers.CanonicalIdentity("agent")
	if err != nil {
		t.Fatalf("CanonicalIdentity(agent) error = %v", err)
	}
	if canonical != "cursor" {
		t.Fatalf("CanonicalIdentity(agent) = %q, want cursor", canonical)
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

func TestFactorySessionsServiceRequiresRuntimeClockBinding(t *testing.T) {
	t.Parallel()
	namedPathResolver, err := factorynamedpaths.New(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct named-path resolver: %v", err)
	}

	service, err := provideFactorySessionsService(
		factoryruntime.NewSessionResultProjectionOperation(),
		nil,
		nil,
		nil,
		func() string { return "response-event-test-id" },
		func() string { return "session-test-id" },
		func() (string, error) { return t.TempDir(), nil },
		platformfilesystem.Local{},
		namedPathResolver,
		factorysessionwire.InvocationInputReader(func(string) ([]byte, error) { return nil, nil }),
		factorysessionwire.InitialWorkReader(func(string) ([]byte, error) { return nil, nil }),
		func(path string) (string, error) { return path, nil },
	)
	if err != nil {
		t.Fatalf("provide Factory Sessions service: %v", err)
	}
	if assembly, err := service.ForRuntime(factorysessions.RuntimeBinding{}); assembly != nil || err == nil {
		t.Fatalf("Factory Sessions assembly without clock = %#v, want nil", assembly)
	}
	if assembly, err := service.ForRuntime(factorysessions.RuntimeBinding{Clock: &wireTestClock{}}); assembly == nil || err != nil {
		t.Fatalf("Factory Sessions assembly with explicit Wire clock = %#v, error = %v", assembly, err)
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
			if entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == "node_modules" {
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
			case ".git", "node_modules", "vendor":
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
