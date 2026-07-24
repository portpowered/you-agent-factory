package service_test

import (
	"context"
	"errors"
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

	_, err := service.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical:  []byte(`{"name":"alpha"}`),
			FactoryDir: "/factories/alpha",
		},
	)
	if !errors.Is(err, factorydefinitions.ErrInvalidAuthoredFactorySource) {
		t.Fatalf(
			"shell CompileEffectiveFactorySource error = %v, want %v",
			err,
			factorydefinitions.ErrInvalidAuthoredFactorySource,
		)
	}
}
