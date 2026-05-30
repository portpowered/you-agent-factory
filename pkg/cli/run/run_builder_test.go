package run

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
)

func TestSetBuildFactoryService_RegistersBuilder(t *testing.T) {
	original := buildFactoryService
	t.Cleanup(func() {
		buildFactoryService = original
	})

	builderErr := errors.New("registered builder")
	SetBuildFactoryService(func(
		_ context.Context,
		_ *service.FactoryServiceConfig,
	) (factoryServiceRunner, error) {
		return nil, builderErr
	})

	_, err := buildFactoryService(context.Background(), &service.FactoryServiceConfig{})
	if !errors.Is(err, builderErr) {
		t.Fatalf("buildFactoryService err = %v, want %v", err, builderErr)
	}
}

func TestSetBuildFactoryService_NilRestoresDefault(t *testing.T) {
	original := buildFactoryService
	t.Cleanup(func() {
		buildFactoryService = original
	})

	customErr := errors.New("custom builder")
	SetBuildFactoryService(func(
		_ context.Context,
		_ *service.FactoryServiceConfig,
	) (factoryServiceRunner, error) {
		return nil, customErr
	})
	SetBuildFactoryService(nil)

	secondErr := errors.New("second builder")
	SetBuildFactoryService(func(
		_ context.Context,
		_ *service.FactoryServiceConfig,
	) (factoryServiceRunner, error) {
		return nil, secondErr
	})

	_, err := buildFactoryService(context.Background(), &service.FactoryServiceConfig{})
	if !errors.Is(err, secondErr) {
		t.Fatalf("buildFactoryService err = %v, want %v", err, secondErr)
	}
}

func TestBuildFactoryService_DefaultMatchesServiceBuilder(t *testing.T) {
	original := buildFactoryService
	t.Cleanup(func() {
		buildFactoryService = original
	})
	buildFactoryService = defaultBuildFactoryService

	_, err := buildFactoryService(context.Background(), nil)
	_, defaultErr := service.BuildFactoryService(context.Background(), nil)
	if (err == nil) != (defaultErr == nil) {
		t.Fatalf("default builder error presence = %v, service.BuildFactoryService = %v", err, defaultErr)
	}
	if err != nil && defaultErr != nil && err.Error() != defaultErr.Error() {
		t.Fatalf("default builder err = %q, service.BuildFactoryService err = %q", err, defaultErr)
	}
}
