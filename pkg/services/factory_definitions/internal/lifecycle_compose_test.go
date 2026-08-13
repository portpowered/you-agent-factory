package internal_test

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
)

func TestComposedLifecycleHostExercisesVersionSurface(t *testing.T) {
	t.Parallel()

	service := lifecycle.New(nil, lifecycle.StubActivationGateway())
	next := service.NextEditableFactoryVersion(nil, time.Unix(42, 0).UTC())
	if next.Logical != 1 {
		t.Fatalf("NextEditableFactoryVersion logical = %d, want 1", next.Logical)
	}
	if !next.Physical.Equal(time.Unix(42, 0).UTC()) {
		t.Fatalf("NextEditableFactoryVersion physical = %v, want unix 42", next.Physical)
	}

	current := factorydefinitions.FactoryVersion{Logical: 3, Physical: time.Unix(100, 0).UTC()}
	base := factorydefinitions.FactoryVersion{Logical: 4, Physical: time.Unix(101, 0).UTC()}
	if err := service.RequireFreshEditableFactoryVersion(&base, current); err != nil {
		t.Fatalf("RequireFreshEditableFactoryVersion() error = %v, want success", err)
	}
}

func TestAttachRuntimeSnapshotDelegatesThroughDefinitionsRoot(t *testing.T) {
	t.Parallel()

	called := false
	base := embeddedDefinitionsService{}
	attached, err := factoryinternal.AttachRuntimeSnapshot(
		base,
		func(
			_ context.Context,
			request factorydefinitions.ResolveRuntimeSnapshotRequest,
		) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
			called = true
			return factorydefinitions.ResolveRuntimeSnapshotResult{
				Snapshot: factorydefinitions.RuntimeSnapshot{
					FactoryDir: request.FactoryDir,
				},
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("AttachRuntimeSnapshot() error = %v", err)
	}

	result, err := attached.ResolveRuntimeSnapshot(
		context.Background(),
		factorydefinitions.ResolveRuntimeSnapshotRequest{FactoryDir: "/factories/alpha"},
	)
	if err != nil {
		t.Fatalf("ResolveRuntimeSnapshot() error = %v", err)
	}
	if !called || result.Snapshot.FactoryDir != "/factories/alpha" {
		t.Fatalf("delegation = called %t, result %#v; want attached operation", called, result)
	}
}

type embeddedDefinitionsService struct {
	factorydefinitions.Service
}

var _ factorydefinitions.Service = embeddedDefinitionsService{}
