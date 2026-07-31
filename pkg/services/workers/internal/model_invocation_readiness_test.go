package internal

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

func TestEnsureInvocationReadyClassifiesScopedReadinessProjection(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:readiness-test")
	if err != nil {
		t.Fatalf("parse Models scope: %v", err)
	}
	service := &Service{
		models: scopedReadinessModelsService{
			testModelsService: testModelsService{},
			readiness: models.Runtime{
				Identity:       "OMNIVOICE_Q4_K_M",
				ReadinessState: models.ReadinessStateMissing,
				LifecycleState: models.LifecycleStateNotInstalled,
			},
		},
		modelsScope: scope,
	}

	readiness, err := service.ensureInvocationReady(
		context.Background(),
		&runtimefixtures.RuntimeConfigLookupFixture{},
		"OMNIVOICE_Q4_K_M",
		"TTS",
	)
	if !errors.Is(err, models.ErrMissing) {
		t.Fatalf("ensureInvocationReady() error = %v, want errors.Is ErrMissing", err)
	}
	if readiness.Identity != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("ensureInvocationReady() readiness = %#v, want named projection", readiness)
	}
}

type scopedReadinessModelsService struct {
	testModelsService
	readiness models.Runtime
}

func (service scopedReadinessModelsService) GetModelReadiness(
	context.Context,
	models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	return models.GetModelReadinessResult{
		ModelName: service.readiness.Identity,
		Readiness: service.readiness,
	}, nil
}
