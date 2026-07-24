package factorydefinition

import (
	"context"
	"errors"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestService_CompileEffectiveFactorySource_DelegatesEquivalentInputs(t *testing.T) {
	t.Parallel()

	var service factoryroot.Service = New(stubDefinitionHost{})

	first, err := service.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{
			Canonical:  []byte(`  {"name":"alpha"}  `),
			FactoryDir: "/factories/alpha",
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource first: %v", err)
	}

	second, err := service.CompileEffectiveFactorySource(
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

func TestService_CompileEffectiveFactorySource_TypedFailures(t *testing.T) {
	t.Parallel()

	var service factoryroot.Service = New(stubDefinitionHost{})

	_, invalidErr := service.CompileEffectiveFactorySource(
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

	_, unresolvedErr := service.CompileEffectiveFactorySource(
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
