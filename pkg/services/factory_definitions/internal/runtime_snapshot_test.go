package internal_test

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

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
