package service_test

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/service"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
)

func TestCompilationShell_ImplementsSubserviceRoot(t *testing.T) {
	t.Parallel()

	var service compilation.Service = compilationwire.NewService()
	if service == nil {
		t.Fatal("compilation wire returned nil Service")
	}

	direct := compilationservice.New()
	if direct == nil {
		t.Fatal("compilation New returned nil implementation")
	}
}

func TestCompilation_CompileEquivalentInputsSameEffectiveIdentity(t *testing.T) {
	t.Parallel()

	var service compilation.Service = compilationwire.NewService()

	first, err := service.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical:  []byte(`  {"name":"alpha"}  `),
			FactoryDir: "/factories/alpha",
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource first: %v", err)
	}

	second, err := service.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{
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
	if second.Effective.FactoryDir != first.Effective.FactoryDir ||
		second.Effective.RuntimeBaseDir != first.Effective.RuntimeBaseDir {
		t.Fatalf(
			"equivalent inputs produced different directory facts: first=%#v second=%#v",
			first.Effective,
			second.Effective,
		)
	}
}
