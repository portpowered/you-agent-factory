package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
)

type stubLoadedSource struct {
	factoryDir     string
	runtimeBaseDir string
	cfg            *factorycontracts.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factorycontracts.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                             { return s.factoryDir }
func (s stubLoadedSource) RuntimeBaseDir() string                         { return s.runtimeBaseDir }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                       {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factorycontracts.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*factorycontracts.FactoryWorkerConfig) error) error {
	return nil
}
func (s stubLoadedSource) Workstation(string) (*factorycontracts.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*factorycontracts.FactoryWorkerConfig, bool) {
	return nil, false
}

func newCompilationService(
	t *testing.T,
	loadCanonical factorycontracts.CanonicalFactoryJSONLoader,
	loadFromFactoryDir factorycontracts.LoadedFactoryLoader,
) compilationservice.Service {
	t.Helper()
	if loadCanonical == nil {
		loadCanonical = func([]byte, factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
			return nil, factoryroot.ErrInvalidNamedFactory
		}
	}
	if loadFromFactoryDir == nil {
		loadFromFactoryDir = func(string, factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
			return nil, factoryroot.ErrInvalidNamedFactory
		}
	}
	svc, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      loadCanonical,
		LoadFromFactoryDir: loadFromFactoryDir,
		EncodeFactory:      stubEncodeFactory,
	})
	if err != nil {
		t.Fatalf("compilationwire.NewService: %v", err)
	}
	return svc
}

func stubEncodeFactory(cfg *factorycontracts.FactoryConfig) ([]byte, error) {
	if cfg == nil {
		return nil, errors.New("factory config is required")
	}
	return json.Marshal(cfg)
}

func stubLoadCanonical(payload []byte, _ factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
	var cfg factorycontracts.FactoryConfig
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
		_ factorycontracts.WorkstationLoader,
	) (factorycontracts.MutableLoadedFactorySource, error) {
		if factoryDir != "/factories/alpha" {
			t.Fatalf("factoryDir = %q, want /factories/alpha", factoryDir)
		}
		return stubLoadedSource{
			factoryDir:     factoryDir,
			runtimeBaseDir: factoryDir,
			cfg:            &factorycontracts.FactoryConfig{Name: "alpha"},
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
		_ factorycontracts.WorkstationLoader,
	) (factorycontracts.MutableLoadedFactorySource, error) {
		return stubLoadedSource{
			factoryDir:     "/factories/alpha",
			runtimeBaseDir: "/factories/alpha",
			cfg:            &factorycontracts.FactoryConfig{Name: "alpha"},
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
