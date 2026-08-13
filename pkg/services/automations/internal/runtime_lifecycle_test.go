package internal

import (
	"context"
	"errors"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func TestRuntimeLifecycle_IsolatesOwnersAndClassifiesDuplicates(t *testing.T) {
	service := New(zap.NewNop(), nil, nil, "", "", nil, nil, nil)
	root := service.Root()
	ctx := context.Background()

	alpha := runtimeActivationRequestForTest("runtime-alpha", "alpha")
	beta := runtimeActivationRequestForTest("runtime-beta", "beta")
	if _, err := root.ActivateRuntime(ctx, alpha); err != nil {
		t.Fatalf("ActivateRuntime(alpha) error = %v", err)
	}
	if _, err := root.ActivateRuntime(ctx, beta); err != nil {
		t.Fatalf("ActivateRuntime(beta) error = %v", err)
	}

	if service.runtimes[alpha.RuntimeID] == service.runtimes[beta.RuntimeID] {
		t.Fatal("runtime activations share an Automations owner")
	}
	alpha.Snapshot.EffectiveFactory.Name = "mutated-after-activation"
	duplicate, err := root.ActivateRuntime(ctx, runtimeActivationRequestForTest("runtime-alpha", "alpha"))
	if err != nil {
		t.Fatalf("ActivateRuntime(duplicate) error = %v", err)
	}
	if !duplicate.Idempotent || duplicate.State != automations.RuntimeLifecycleActivated {
		t.Fatalf("duplicate result = %#v, want idempotent activated result", duplicate)
	}

	_, err = root.ActivateRuntime(ctx, runtimeActivationRequestForTest("runtime-alpha", "changed"))
	if !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("ActivateRuntime(conflict) error = %v, want conflict", err)
	}

	stopped, err := root.DeactivateRuntime(ctx, automations.RuntimeDeactivationRequest{RuntimeID: alpha.RuntimeID})
	if err != nil || stopped.State != automations.RuntimeLifecycleStopped {
		t.Fatalf("DeactivateRuntime(alpha) = %#v, %v", stopped, err)
	}
	if _, ok := service.runtimes[beta.RuntimeID]; !ok {
		t.Fatal("deactivating alpha removed beta runtime state")
	}
	idempotent, err := root.DeactivateRuntime(ctx, automations.RuntimeDeactivationRequest{RuntimeID: alpha.RuntimeID})
	if err != nil || !idempotent.Idempotent {
		t.Fatalf("DeactivateRuntime(alpha again) = %#v, %v, want idempotent", idempotent, err)
	}
	if _, err := root.DeactivateRuntime(ctx, automations.RuntimeDeactivationRequest{RuntimeID: beta.RuntimeID}); err != nil {
		t.Fatalf("DeactivateRuntime(beta) error = %v", err)
	}
}

func TestRuntimeLifecycle_RejectsMissingIdentityWithTypedError(t *testing.T) {
	service := New(zap.NewNop(), nil, nil, "", "", nil, nil, nil)
	_, err := service.Root().ActivateRuntime(context.Background(), automations.RuntimeActivationRequest{})
	var typed *automations.Error
	if !errors.As(err, &typed) || typed.Code != automations.ErrorCodeInvalid {
		t.Fatalf("ActivateRuntime(empty) error = %v, want typed invalid error", err)
	}
}

func TestRuntimeLifecycle_RejectsBehavioralInputConflictsWithSameSnapshot(t *testing.T) {
	service := New(zap.NewNop(), nil, nil, "", "", nil, nil, nil)
	base := runtimeActivationRequestForTest("runtime-input-conflict", "same")
	if _, err := service.ActivateRuntime(context.Background(), base); err != nil {
		t.Fatalf("ActivateRuntime(base) error = %v", err)
	}

	schedulerConflict := base
	schedulerConflict.Inputs.StartSchedulers = true
	if _, err := service.ActivateRuntime(context.Background(), schedulerConflict); !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("ActivateRuntime(scheduler conflict) error = %v, want conflict", err)
	}

	filesystemConflict := base
	filesystemConflict.Inputs.Filesystem.KnownWorkTypes = []string{"different-work-type"}
	if _, err := service.ActivateRuntime(context.Background(), filesystemConflict); !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("ActivateRuntime(filesystem conflict) error = %v, want conflict", err)
	}

	if _, err := service.ActivateRuntime(context.Background(), base); err != nil {
		t.Fatalf("ActivateRuntime(unchanged duplicate) error = %v, want idempotent success", err)
	}
}

func TestRuntimeLifecycle_StartsAndStopsSchedulerOwnership(t *testing.T) {
	service := New(zap.NewNop(), nil, nil, "", "", nil, nil, nil)
	request := runtimeActivationRequestForTest("runtime-scheduler", "scheduler")
	request.Inputs.StartSchedulers = true
	request.Inputs.Submitter = func(context.Context, work.WorkRequest) error { return nil }
	if _, err := service.ActivateRuntime(context.Background(), request); err != nil {
		t.Fatalf("ActivateRuntime() error = %v", err)
	}
	if err := service.StartRuntime(context.Background(), request.RuntimeID); err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	if err := service.StartRuntime(context.Background(), request.RuntimeID); err != nil {
		t.Fatalf("StartRuntime() duplicate error = %v", err)
	}
	if _, err := service.DeactivateRuntime(context.Background(), automations.RuntimeDeactivationRequest{RuntimeID: request.RuntimeID}); err != nil {
		t.Fatalf("DeactivateRuntime() error = %v", err)
	}
}

func runtimeActivationRequestForTest(runtimeID, factoryName string) automations.RuntimeActivationRequest {
	return automations.RuntimeActivationRequest{
		RuntimeID:        runtimeID,
		FactorySessionID: "session-1",
		Snapshot: factorydefinitions.RuntimeSnapshot{
			FactoryDir:       "/factories/" + factoryName,
			RuntimeBaseDir:   "/factories/" + factoryName,
			Invocation:       factorydefinitions.RuntimeSnapshotInvocationContext{FactorySessionID: "session-1"},
			EffectiveFactory: factorydefinitions.FactoryConfig{Name: factoryName},
		},
	}
}
