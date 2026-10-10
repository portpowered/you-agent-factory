package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/service"
)

type stubLoadedSource struct {
	factoryDir     string
	runtimeBaseDir string
	cfg            *factorydefinitions.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factorydefinitions.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                               { return s.factoryDir }
func (s stubLoadedSource) RuntimeBaseDir() string                           { return s.runtimeBaseDir }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                         {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*factorydefinitions.FactoryWorkerConfig) error) error {
	return nil
}
func (s stubLoadedSource) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}

func newCompilationService(
	t *testing.T,
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	loadFromFactoryDir factorydefinitions.LoadedFactoryLoader,
) compilationservice.Service {
	t.Helper()
	if loadCanonical == nil {
		loadCanonical = func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, factoryroot.ErrInvalidNamedFactory
		}
	}
	if loadFromFactoryDir == nil {
		loadFromFactoryDir = func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, factoryroot.ErrInvalidNamedFactory
		}
	}
	svc := compilationimpl.New(
		loadCanonical,
		loadFromFactoryDir,
		stubEncodeFactory,
	)
	return svc
}

func stubEncodeFactory(cfg *factorydefinitions.FactoryConfig) ([]byte, error) {
	if cfg == nil {
		return nil, errors.New("factory config is required")
	}
	return json.Marshal(cfg)
}

func TestCompileEffectiveFactorySource_CancellationPrecedesLoading(t *testing.T) {
	t.Parallel()
	loadCanonical := func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		t.Fatal("canceled compilation reached the canonical loader")
		return nil, nil
	}
	loadDirectory := func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		t.Fatal("canceled compilation reached the directory loader")
		return nil, nil
	}
	svc := compilationimpl.New(loadCanonical, loadDirectory, stubEncodeFactory)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := svc.CompileEffectiveFactorySource(ctx, factoryroot.CompileEffectiveFactorySourceRequest{
		Canonical: []byte(`{"name":"alpha"}`),
	})
	if !errors.Is(err, context.Canceled) || result != (factoryroot.CompileEffectiveFactorySourceResult{}) {
		t.Fatalf("canceled compilation = %#v, %v; want empty result and context.Canceled", result, err)
	}
}

func TestCompileEffectiveFactorySource_LoaderFailureRetainsCause(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"canonical", "directory"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("source read failed")
			loadCanonical := func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
				return nil, cause
			}
			loadDirectory := func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
				return nil, cause
			}
			svc := compilationimpl.New(loadCanonical, loadDirectory, stubEncodeFactory)
			request := factoryroot.CompileEffectiveFactorySourceRequest{FactoryDir: "/factories/alpha"}
			if source == "canonical" {
				request.Canonical = []byte(`{"name":"alpha"}`)
			}
			result, err := svc.CompileEffectiveFactorySource(context.Background(), request)
			if !errors.Is(err, cause) || !errors.Is(err, factoryroot.ErrInvalidAuthoredFactorySource) {
				t.Fatalf("compile failure = %v; want typed invalid source and loader cause", err)
			}
			if result != (factoryroot.CompileEffectiveFactorySourceResult{}) {
				t.Fatalf("failed compilation returned partial result: %#v", result)
			}
		})
	}
}

func stubLoadCanonical(payload []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	var cfg factorydefinitions.FactoryConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, factoryroot.ErrInvalidNamedFactory
	}
	return stubLoadedSource{
		factoryDir:     "/factories/alpha",
		runtimeBaseDir: "/factories/alpha",
		cfg:            &cfg,
	}, nil
}

func TestCompileEffectiveFactorySource_EquivalentCanonicalInputsShareIdentity(t *testing.T) {
	t.Parallel()

	svc := newCompilationService(t, stubLoadCanonical, nil)

	first, err := svc.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{
			Canonical:  []byte(`  {"name":"alpha"}  `),
			FactoryDir: "/factories/alpha",
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource first: %v", err)
	}

	second, err := svc.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{
			Canonical:  []byte(`{"name":"alpha"}`),
			FactoryDir: "/factories/alpha",
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource second: %v", err)
	}

	if first.Effective.ContentIdentity == "" {
		t.Fatal("CompileEffectiveFactorySource ContentIdentity is empty")
	}
	if first.Effective.ContentIdentity != second.Effective.ContentIdentity {
		t.Fatalf(
			"equivalent inputs produced different ContentIdentity: %q vs %q",
			first.Effective.ContentIdentity,
			second.Effective.ContentIdentity,
		)
	}
	if first.Effective.FactoryDir != "/factories/alpha" ||
		first.Effective.RuntimeBaseDir != "/factories/alpha" {
		t.Fatalf("CompileEffectiveFactorySource effective = %#v, want alpha identity facts", first.Effective)
	}
}

func TestCompilationDirectoryAndCanonicalInputsPreserveLoadedIdentity(t *testing.T) {
	t.Parallel()
	config := &factorydefinitions.FactoryConfig{Name: "alpha"}
	source := stubLoadedSource{cfg: config, factoryDir: "/factories/alpha", runtimeBaseDir: "/runtime/alpha"}
	canonicalCalls, directoryCalls := 0, 0
	svc := compilationimpl.New(
		func(payload []byte, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			canonicalCalls++
			if string(payload) != `{"name":"alpha"}` || loader != nil {
				t.Fatalf("canonical arguments = %q, %v", payload, loader)
			}
			return source, nil
		},
		func(dir string, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			directoryCalls++
			if dir != source.factoryDir || loader != nil {
				t.Fatalf("directory arguments = %q, %v", dir, loader)
			}
			return source, nil
		}, stubEncodeFactory,
	)
	fromDirectory, err := svc.CompileEffectiveFactorySource(t.Context(), factoryroot.CompileEffectiveFactorySourceRequest{FactoryDir: source.factoryDir})
	if err != nil {
		t.Fatal(err)
	}
	fromCanonical, err := svc.CompileEffectiveFactorySource(t.Context(), factoryroot.CompileEffectiveFactorySourceRequest{Canonical: []byte(`{"name":"alpha"}`), FactoryDir: source.factoryDir})
	if err != nil {
		t.Fatal(err)
	}
	if fromDirectory != fromCanonical || fromDirectory.Effective.ContentIdentity == "" || fromDirectory.Effective.FactoryDir != source.factoryDir || fromDirectory.Effective.RuntimeBaseDir != source.runtimeBaseDir {
		t.Fatalf("effective outcomes = %#v, %#v; want equal content and loaded directory identities", fromDirectory, fromCanonical)
	}
	if canonicalCalls != 1 || directoryCalls != 1 {
		t.Fatalf("loader calls = canonical %d, directory %d; want one selected load each", canonicalCalls, directoryCalls)
	}
}

func TestCompileEffectiveFactorySource_TypedInvalidSourceAndUnresolvedReference(t *testing.T) {
	t.Parallel()

	svc := newCompilationService(t, stubLoadCanonical, nil)

	_, invalidErr := svc.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factoryroot.ErrInvalidAuthoredFactorySource) {
		t.Fatalf(
			"CompileEffectiveFactorySource invalid-source error = %v, want %v",
			invalidErr,
			factoryroot.ErrInvalidAuthoredFactorySource,
		)
	}

	_, unresolvedErr := svc.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{
			Canonical: []byte(`{"worker":"$unresolved"}`),
		},
	)
	if !errors.Is(unresolvedErr, factoryroot.ErrUnresolvedDefinitionReference) {
		t.Fatalf(
			"CompileEffectiveFactorySource unresolved error = %v, want %v",
			unresolvedErr,
			factoryroot.ErrUnresolvedDefinitionReference,
		)
	}
	if errors.Is(unresolvedErr, factoryroot.ErrInvalidAuthoredFactorySource) {
		t.Fatal("unresolved definition reference must not also match ErrInvalidAuthoredFactorySource")
	}
}

func TestCompileEffectiveFactorySource_LoadsAuthoredFactoryDirectory(t *testing.T) {
	t.Parallel()

	loadFromFactoryDir := func(
		factoryDir string,
		_ factorydefinitions.WorkstationLoader,
	) (factorydefinitions.MutableLoadedFactorySource, error) {
		if factoryDir != "/factories/alpha" {
			t.Fatalf("factoryDir = %q, want /factories/alpha", factoryDir)
		}
		return stubLoadedSource{
			factoryDir:     factoryDir,
			runtimeBaseDir: factoryDir,
			cfg:            &factorydefinitions.FactoryConfig{Name: "alpha"},
		}, nil
	}
	svc := newCompilationService(t, nil, loadFromFactoryDir)

	got, err := svc.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{FactoryDir: "/factories/alpha"},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource: %v", err)
	}
	if got.Effective.FactoryDir != "/factories/alpha" ||
		got.Effective.RuntimeBaseDir != "/factories/alpha" {
		t.Fatalf("effective = %#v, want alpha directory facts", got.Effective)
	}
	if !strings.Contains(got.Effective.ContentIdentity, `"name":"alpha"`) {
		t.Fatalf("ContentIdentity = %q, want encoded alpha factory", got.Effective.ContentIdentity)
	}
}

func TestCompileEffectiveFactorySource_DoesNotStartFactorySessionOrRuntime(t *testing.T) {
	t.Parallel()

	loadCanonical := func(
		_ []byte,
		_ factorydefinitions.WorkstationLoader,
	) (factorydefinitions.MutableLoadedFactorySource, error) {
		return stubLoadedSource{
			factoryDir:     "/factories/alpha",
			runtimeBaseDir: "/factories/alpha",
			cfg:            &factorydefinitions.FactoryConfig{Name: "alpha"},
		}, nil
	}
	svc := newCompilationService(t, loadCanonical, nil)

	_, err := svc.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{
			Canonical: []byte(`{"name":"alpha"}`),
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource: %v", err)
	}
}

func TestCanonicalLoadingPreservesArgumentsAndOutcome(t *testing.T) {
	t.Parallel()
	cause := errors.New("canonical loader failure")
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "failure", err: fmt.Errorf("read canonical source: %w", cause)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload := []byte("  {canonical source}\n")
			workstationLoader := &canonicalWorkstationLoader{}
			expected := &stubLoadedSource{factoryDir: "/factories/alpha"}
			loadCanonical := func(got []byte, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
				if !bytes.Equal(got, payload) || &got[0] != &payload[0] || loader != workstationLoader {
					t.Fatalf("canonical arguments = %q, %v; want original payload and workstation loader", got, loader)
				}
				return expected, tc.err
			}
			svc := compilationimpl.New(loadCanonical, nil, nil)
			result, err := svc.LoadCanonicalFactorySource(payload, workstationLoader)
			if result != expected || err != tc.err {
				t.Fatalf("canonical outcome = %v, %v; want original source and error %v", result, err, tc.err)
			}
			if tc.err != nil && !errors.Is(err, cause) {
				t.Fatalf("canonical failure lost cause: %v", err)
			}
		})
	}
}

type canonicalWorkstationLoader struct{}

func (*canonicalWorkstationLoader) Load(string) (*factorydefinitions.FactoryWorkstationConfig, error) {
	panic("canonical delegation must pass the workstation loader without invoking it")
}
