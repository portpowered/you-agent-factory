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

// TestRootContractInvariants_AllSlicesThroughSingularService seals the System
// Bootstrap root-contract packet: initialize success with outcome vocabulary,
// skipped/already-initialized results, typed validation and cancellation
// failures, and partial-failure rollback facts are all reachable through one
// named systeminitialization.Service, a peer-shaped fake exercises every path
// using only the root package, and no second peer-facing Bootstrap authority
// is required.
func TestRootContractInvariants_AllSlicesThroughSingularService(t *testing.T) {
	t.Parallel()

	fake := &rootServiceFake{}
	var service systeminitialization.Service = fake

	assertSealInitializeSuccessWithCreatedOutcomes(t, service, fake)
	assertSealInitializeSuccessWithSkippedOutcomes(t, service, fake)
	assertSealInitializeValidationFailure(t, service, fake)
	assertSealInitializeCancellationFailure(t, service, fake)
	assertSealInitializePartialFailureWithRollbackFacts(t, service, fake)
}

func assertSealInitializeSuccessWithCreatedOutcomes(
	t *testing.T,
	service systeminitialization.Service,
	fake *rootServiceFake,
) {
	t.Helper()

	fake.result = systeminitialization.Result{
		HomeDir:             "/home/peer",
		ConfigPath:          "/home/peer/.you/config.json",
		NamedFactoriesRoot:  "/home/peer/.you-agent-factory/factories",
		SystemConfigOutcome: systeminitialization.SystemConfigCreated,
		PackagedFactories: []systeminitialization.PackagedFactoryResult{{
			Name:       "@you/goal",
			FactoryDir: "goal",
			Outcome:    systeminitialization.PackagedFactoryCreated,
		}},
	}
	fake.err = nil

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

func assertSealInitializeSuccessWithSkippedOutcomes(
	t *testing.T,
	service systeminitialization.Service,
	fake *rootServiceFake,
) {
	t.Helper()

	fake.result = systeminitialization.Result{
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
	}
	fake.err = nil

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

func assertSealInitializeValidationFailure(
	t *testing.T,
	service systeminitialization.Service,
	fake *rootServiceFake,
) {
	t.Helper()

	fake.result = systeminitialization.Result{}
	fake.err = fmt.Errorf("peer validation: %w", systeminitialization.ErrMissingHomeDir)

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

func assertSealInitializeCancellationFailure(
	t *testing.T,
	service systeminitialization.Service,
	fake *rootServiceFake,
) {
	t.Helper()

	fake.result = systeminitialization.Result{}
	fake.err = fmt.Errorf("peer cancellation: %w", systeminitialization.ErrInitializeCancelled)

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

func assertSealInitializePartialFailureWithRollbackFacts(
	t *testing.T,
	service systeminitialization.Service,
	fake *rootServiceFake,
) {
	t.Helper()

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
	fake.result = systeminitialization.Result{}
	fake.err = fmt.Errorf("peer partial failure: %w", partialFailure)

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

func TestRootContract_ContractValuesStayInertWhenHeld(t *testing.T) {
	t.Parallel()

	request := systeminitialization.Request{HomeDir: "/home/peer"}
	result := systeminitialization.Result{
		HomeDir:             "/home/peer",
		SystemConfigOutcome: systeminitialization.SystemConfigCreated,
		PackagedFactories: []systeminitialization.PackagedFactoryResult{{
			Name:    "@you/goal",
			Outcome: systeminitialization.PackagedFactoryCreated,
		}},
	}

	fact := systeminitialization.RollbackFact{
		Step:    systeminitialization.InitializeStepSystemConfig,
		Outcome: systeminitialization.RollbackStepCompleted,
	}
	clonedFact := fact.Clone()
	fact.Outcome = systeminitialization.RollbackStepUnresolved
	if clonedFact.Outcome == systeminitialization.RollbackStepUnresolved {
		t.Fatal("RollbackFact.Clone() shares mutable outcome state")
	}

	partialFailure := systeminitialization.InitializePartialFailure{
		Message: "install failed",
		Facts: []systeminitialization.RollbackFact{{
			Step:    systeminitialization.InitializeStepPackagedFactories,
			Outcome: systeminitialization.RollbackStepUnresolved,
		}},
	}
	clonedFailure := partialFailure.Clone()
	partialFailure.Facts[0].Outcome = systeminitialization.RollbackStepCompleted
	if clonedFailure.Facts[0].Outcome == systeminitialization.RollbackStepCompleted {
		t.Fatal("InitializePartialFailure.Clone() shares mutable rollback facts")
	}

	// Holding contract values must not require a Service implementation or
	// perform Settings persist, packaged install, legacy migration, Wire, or
	// process-initializer work.
	var (
		_ systeminitialization.Request = request
		_ systeminitialization.Result  = result
		_ systeminitialization.PackagedFactoryResult
		_ systeminitialization.RollbackFact             = fact
		_ systeminitialization.InitializePartialFailure = partialFailure
		_ systeminitialization.SystemConfigOutcome
		_ systeminitialization.PackagedFactoryOutcome
		_ systeminitialization.InitializeStepID
		_ systeminitialization.RollbackStepOutcome
	)
}

func TestRootContract_FakePeerConstructionIsInert(t *testing.T) {
	t.Parallel()

	fake := &rootServiceFake{}
	var service systeminitialization.Service = fake
	if service == nil {
		t.Fatal("constructed Service is nil")
	}
	if !reflect.DeepEqual(fake.result, systeminitialization.Result{}) {
		t.Fatalf("fake peer construction result = %#v, want zero value", fake.result)
	}
	if fake.err != nil {
		t.Fatalf("fake peer construction err = %v, want nil", fake.err)
	}
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
