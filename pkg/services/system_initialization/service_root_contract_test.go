package systeminitialization_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

// rootServiceFake is a peer-shaped System Bootstrap root Service that uses only
// Bootstrap-owned request, result, value, and typed-error contracts. It never
// imports Operator Settings implementation packages, Factory Definitions
// implementation subpackages, concrete filesystem collaborator ports, or
// pkg/initializer lifecycle types.
type rootServiceFake struct {
	result systeminitialization.Result
	err    error

	lastRequest systeminitialization.Request
}

var _ systeminitialization.Service = (*rootServiceFake)(nil)

func (fake *rootServiceFake) Initialize(
	_ context.Context,
	request systeminitialization.Request,
) (systeminitialization.Result, error) {
	fake.lastRequest = request
	if fake.err != nil {
		return systeminitialization.Result{}, fake.err
	}
	return fake.result, nil
}

func TestRootService_Characterization_FakeImplementsSingularSeam(t *testing.T) {
	t.Parallel()

	fake := &rootServiceFake{
		result: systeminitialization.Result{
			HomeDir:             "/home/peer",
			ConfigPath:          "/home/peer/.you/config.json",
			NamedFactoriesRoot:  "/home/peer/.you-agent-factory/factories",
			SystemConfigOutcome: systeminitialization.SystemConfigCreated,
			PackagedFactories: []systeminitialization.PackagedFactoryResult{{
				Name:       "@you/goal",
				FactoryDir: "goal",
				Outcome:    systeminitialization.PackagedFactoryCreated,
			}},
		},
	}

	var service systeminitialization.Service = fake
	result, err := service.Initialize(context.Background(), systeminitialization.Request{
		HomeDir: "/home/peer",
	})
	if err != nil {
		t.Fatalf("Initialize() = %v", err)
	}
	if fake.lastRequest.HomeDir != "/home/peer" {
		t.Fatalf("fake recorded request = %#v", fake.lastRequest)
	}
	if result.HomeDir != "/home/peer" ||
		result.SystemConfigOutcome != systeminitialization.SystemConfigCreated ||
		len(result.PackagedFactories) != 1 ||
		result.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCreated {
		t.Fatalf("Initialize() result = %#v", result)
	}
}

func TestRootService_Characterization_InitializeSuccessWithCreatedAndSkippedOutcomes(t *testing.T) {
	t.Parallel()

	fake := &rootServiceFake{
		result: systeminitialization.Result{
			HomeDir:             "/home/peer",
			ConfigPath:          "/home/peer/.you/config.json",
			NamedFactoriesRoot:  "/home/peer/.you-agent-factory/factories",
			SystemConfigOutcome: systeminitialization.SystemConfigSkipped,
			PackagedFactories: []systeminitialization.PackagedFactoryResult{
				{
					Name:       "@you/goal",
					FactoryDir: "goal",
					Outcome:    systeminitialization.PackagedFactoryCreated,
				},
				{
					Name:       "@you/legacy",
					FactoryDir: "legacy",
					Outcome:    systeminitialization.PackagedFactorySkipped,
				},
			},
		},
	}

	var service systeminitialization.Service = fake
	result, err := service.Initialize(context.Background(), systeminitialization.Request{
		HomeDir: "/home/peer",
	})
	if err != nil {
		t.Fatalf("Initialize() = %v", err)
	}
	if result.SystemConfigOutcome != systeminitialization.SystemConfigSkipped {
		t.Fatalf("SystemConfigOutcome = %q, want skipped", result.SystemConfigOutcome)
	}
	if len(result.PackagedFactories) != 2 ||
		result.PackagedFactories[0].Outcome != systeminitialization.PackagedFactoryCreated ||
		result.PackagedFactories[1].Outcome != systeminitialization.PackagedFactorySkipped {
		t.Fatalf("PackagedFactories = %#v, want created then skipped", result.PackagedFactories)
	}
}

func TestRootService_Characterization_InitializeValidationFailure(t *testing.T) {
	t.Parallel()

	fake := &rootServiceFake{
		err: fmt.Errorf("peer validation: %w", systeminitialization.ErrMissingHomeDir),
	}

	var service systeminitialization.Service = fake
	result, err := service.Initialize(context.Background(), systeminitialization.Request{})
	if !errors.Is(err, systeminitialization.ErrMissingHomeDir) {
		t.Fatalf("Initialize() error = %v, want ErrMissingHomeDir", err)
	}
	if !reflect.DeepEqual(result, systeminitialization.Result{}) {
		t.Fatalf("Initialize() result = %#v, want zero result", result)
	}
	var partialFailure systeminitialization.InitializePartialFailure
	if errors.As(err, &partialFailure) {
		t.Fatalf("validation failure error = %v, want no rollback facts", err)
	}
}

func TestRootService_Characterization_InitializeCancellationFailureHasNoRollbackFacts(t *testing.T) {
	t.Parallel()

	fake := &rootServiceFake{
		err: fmt.Errorf("peer cancellation: %w", systeminitialization.ErrInitializeCancelled),
	}

	var service systeminitialization.Service = fake
	result, err := service.Initialize(context.Background(), systeminitialization.Request{
		HomeDir: "/home/peer",
	})
	if !errors.Is(err, systeminitialization.ErrInitializeCancelled) {
		t.Fatalf("Initialize() error = %v, want ErrInitializeCancelled", err)
	}
	if !reflect.DeepEqual(result, systeminitialization.Result{}) {
		t.Fatalf("Initialize() result = %#v, want zero result", result)
	}
	var partialFailure systeminitialization.InitializePartialFailure
	if errors.As(err, &partialFailure) {
		t.Fatalf("cancellation failure error = %v, want no rollback facts", err)
	}
}

func TestRootService_Characterization_InitializePartialFailureWithRollbackFacts(t *testing.T) {
	t.Parallel()

	partialFailure := systeminitialization.InitializePartialFailure{
		Message: "packaged factory install failed",
		Facts: []systeminitialization.RollbackFact{
			{
				Step:    systeminitialization.InitializeStepLegacyMigration,
				Outcome: systeminitialization.RollbackStepCompleted,
			},
			{
				Step:    systeminitialization.InitializeStepSystemConfig,
				Outcome: systeminitialization.RollbackStepRolledBackOrPreserved,
			},
			{
				Step:    systeminitialization.InitializeStepPackagedFactories,
				Outcome: systeminitialization.RollbackStepUnresolved,
			},
		},
	}
	fake := &rootServiceFake{
		err: fmt.Errorf("peer partial failure: %w", partialFailure),
	}

	var service systeminitialization.Service = fake
	result, err := service.Initialize(context.Background(), systeminitialization.Request{
		HomeDir: "/home/peer",
	})
	if !errors.Is(err, systeminitialization.ErrInitializePartialFailure) {
		t.Fatalf("Initialize() error = %v, want ErrInitializePartialFailure", err)
	}
	if !reflect.DeepEqual(result, systeminitialization.Result{}) {
		t.Fatalf("Initialize() result = %#v, want zero result", result)
	}
	var got systeminitialization.InitializePartialFailure
	if !errors.As(err, &got) {
		t.Fatalf("Initialize() error = %T(%v), want InitializePartialFailure", err, err)
	}
	if len(got.Facts) != 3 ||
		got.Facts[0].Step != systeminitialization.InitializeStepLegacyMigration ||
		got.Facts[0].Outcome != systeminitialization.RollbackStepCompleted ||
		got.Facts[1].Outcome != systeminitialization.RollbackStepRolledBackOrPreserved ||
		got.Facts[2].Outcome != systeminitialization.RollbackStepUnresolved {
		t.Fatalf("Initialize() rollback facts = %#v", got.Facts)
	}
}
