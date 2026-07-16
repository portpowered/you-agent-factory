package factorysave

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func TestValidateEditableFactoryTopology_AllowsRecoverableLayoutWarnings(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	factory.Layout = &factoryapi.FactoryLayout{
		SchemaVersion: interfaces.SupportedFactoryLayoutSchemaVersion,
		Nodes: &[]factoryapi.FactoryLayoutNode{{
			Id:       "workstation:stale-node",
			Position: factoryapi.FactoryLayoutPoint{X: 10, Y: 20},
			Size:     &factoryapi.FactoryLayoutSize{Width: 100, Height: 80},
		}},
		Viewport: &factoryapi.FactoryLayoutViewport{Zoom: 1},
	}

	apiResult, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfilePrePersist,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	if !apiResult.HasTargets() {
		t.Fatal("expected recoverable layout validation targets")
	}
	if apiResult.HasBlockingTargets() {
		t.Fatalf("blocking targets = %#v, want only layout warnings", apiResult.BlockingTargets())
	}

	if err := validateEditableFactoryTopology(factory, nil); err != nil {
		t.Fatalf("validateEditableFactoryTopology: %v, want save allowed with recoverable layout warnings", err)
	}

	cfg, err := factoryconfig.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	blocking := factoryvalidation.ValidateBlockingLoad(&cfg)
	if blocking.HasTargets() {
		t.Fatalf("blocking load targets = %#v, want runtime load to remain non-fatal for layout defects", blocking.Targets)
	}
}
