package internal_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

type materializedSnapshotSource struct {
	factorydefinitions.MutableLoadedFactorySource
	baseDir string
}

func (source *materializedSnapshotSource) SetRuntimeBaseDir(dir string) { source.baseDir = dir }

func TestRuntimeSnapshotMaterializationDetachesSelectionsAndPreservesProvenance(t *testing.T) {
	t.Parallel()
	snapshot := factorydefinitions.RuntimeSnapshot{
		FactoryDir: "/factory", RuntimeBaseDir: "/runtime",
		EffectiveFactory:                factorydefinitions.FactoryConfig{Name: "factory", Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker"}}},
		Workers:                         []factorydefinitions.FactoryWorkerConfig{{Name: "worker", ReasoningEffort: "high"}},
		PromptSources:                   []factorydefinitions.RuntimePromptSource{{Role: "worker", Name: "worker", Path: "worker.md"}},
		PromptProvenance:                []factorydefinitions.RuntimePromptProvenance{{Name: "worker"}},
		InvocationSensitiveJSONPointers: []string{"/workers/0/prompt"},
	}
	var configs []*factorydefinitions.FactoryConfig
	var lookups []factorydefinitions.RuntimeDefinitionLookup
	var sources []*materializedSnapshotSource
	owner := factoryinternal.NewRuntimeSnapshotMaterializer(func(dir string, config *factorydefinitions.FactoryConfig,
		lookup factorydefinitions.RuntimeDefinitionLookup, _ []factorydefinitions.PortableBundledFileReplacement,
	) (factorydefinitions.MutableLoadedFactorySource, error) {
		if dir != snapshot.FactoryDir {
			t.Fatalf("materialized directory = %s", dir)
		}
		source := &materializedSnapshotSource{}
		configs, lookups, sources = append(configs, config), append(lookups, lookup), append(sources, source)
		return source, nil
	})
	first, err := owner.Materialize(&snapshot, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := owner.Materialize(&snapshot, "/selected")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Workers[0].ReasoningEffort = "low"
	snapshot.EffectiveFactory.Workers[0].Name = "caller changed"
	snapshot.InvocationSensitiveJSONPointers[0] = "caller changed"
	for index, loaded := range []factorydefinitions.MutableLoadedFactorySource{first, second} {
		worker, ok := lookups[index].Worker("worker")
		if !ok || worker.ReasoningEffort != "high" || configs[index].Workers[0].PromptSourcePath != "worker.md" {
			t.Fatalf("opening %d lost detached prompt/source selections", index)
		}
		provenance := loaded.(factorydefinitions.RuntimePromptProvenanceLookup)
		if selected, ok := provenance.WorkerPromptProvenance("worker"); !ok || selected.Name != "worker" {
			t.Fatalf("opening %d lost prompt provenance", index)
		}
		pointers := loaded.(interface{ InvocationSensitiveJSONPointers() []string }).InvocationSensitiveJSONPointers()
		if len(pointers) != 1 || pointers[0] != "/workers/0/prompt" {
			t.Fatalf("opening %d lost redaction provenance: %v", index, pointers)
		}
	}
	if sources[0].baseDir != "/runtime" || sources[1].baseDir != "/selected" || configs[0] == configs[1] {
		t.Fatal("materialization retargeted an earlier opening's data")
	}
}

func TestRuntimeSnapshotMaterializationReturnsSelectedFailureWithoutPublication(t *testing.T) {
	t.Parallel()
	failure := errors.New("selected materialization failed")
	owner := factoryinternal.NewRuntimeSnapshotMaterializer(func(string, *factorydefinitions.FactoryConfig,
		factorydefinitions.RuntimeDefinitionLookup, []factorydefinitions.PortableBundledFileReplacement,
	) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, failure
	})
	snapshot := factorydefinitions.RuntimeSnapshot{FactoryDir: "/factory", EffectiveFactory: factorydefinitions.FactoryConfig{Name: "factory"}}
	loaded, err := owner.Materialize(&snapshot, "")
	if loaded != nil || !errors.Is(err, failure) {
		t.Fatalf("failed materialization = %v, %v", loaded, err)
	}
	if loaded, err := owner.Materialize(nil, ""); loaded != nil || err == nil {
		t.Fatal("missing snapshot was accepted")
	}
}

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
