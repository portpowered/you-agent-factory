package internal_test

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

type snapshotPortabilityStub struct {
	captureCalled     bool
	prepareCalled     bool
	materializeCalled bool
}

func (s *snapshotPortabilityStub) CaptureFactorySnapshot(
	context.Context,
	factorydefinitions.CaptureFactorySnapshotRequest,
) (factorydefinitions.CaptureFactorySnapshotResult, error) {
	s.captureCalled = true
	return factorydefinitions.CaptureFactorySnapshotResult{}, nil
}

func (s *snapshotPortabilityStub) PrepareFactorySnapshotImport(
	context.Context,
	factorydefinitions.PrepareFactorySnapshotImportRequest,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	s.prepareCalled = true
	return factorydefinitions.PrepareFactorySnapshotImportResult{}, nil
}

func (s *snapshotPortabilityStub) MaterializeFactorySnapshot(
	context.Context,
	factorydefinitions.MaterializeFactorySnapshotRequest,
) (factorydefinitions.MaterializeFactorySnapshotResult, error) {
	s.materializeCalled = true
	return factorydefinitions.MaterializeFactorySnapshotResult{}, nil
}

type snapshotRootStub struct {
	factorydefinitions.Service
}

func TestAttachSnapshotsPortabilityDelegatesRootSnapshotSlice(t *testing.T) {
	t.Parallel()

	stub := &snapshotPortabilityStub{}
	attached, err := factoryinternal.AttachSnapshotsPortability(snapshotRootStub{}, stub)
	if err != nil {
		t.Fatalf("AttachSnapshotsPortability() error = %v", err)
	}
	ctx := context.Background()

	if _, err := attached.CaptureFactorySnapshot(ctx, factorydefinitions.CaptureFactorySnapshotRequest{}); err != nil {
		t.Fatalf("CaptureFactorySnapshot() error = %v", err)
	}
	if _, err := attached.PrepareFactorySnapshotImport(ctx, factorydefinitions.PrepareFactorySnapshotImportRequest{}); err != nil {
		t.Fatalf("PrepareFactorySnapshotImport() error = %v", err)
	}
	if _, err := attached.MaterializeFactorySnapshot(ctx, factorydefinitions.MaterializeFactorySnapshotRequest{}); err != nil {
		t.Fatalf("MaterializeFactorySnapshot() error = %v", err)
	}
	if !stub.captureCalled || !stub.prepareCalled || !stub.materializeCalled {
		t.Fatalf("snapshot delegation flags = %#v, want all true", stub)
	}
}

func TestAttachSnapshotsPortabilityRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	if _, err := factoryinternal.AttachSnapshotsPortability(nil, &snapshotPortabilityStub{}); err == nil {
		t.Fatal("AttachSnapshotsPortability(nil service) expected error")
	}
	if _, err := factoryinternal.AttachSnapshotsPortability(snapshotRootStub{}, nil); err == nil {
		t.Fatal("AttachSnapshotsPortability(nil snapshots) expected error")
	}
}
