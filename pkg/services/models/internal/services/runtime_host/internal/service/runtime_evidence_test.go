package service_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
)

func TestManagedLocalAIPreflightRecordsProtocolStageEvidence(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "runtime-evidence-preflight")
	ref := openScope(t, scopes, cacheDirectory, managedLocalAIConfig(models.LoadPolicyOnDemand))
	sink := &runtimeEvidenceSink{}
	host := internalservice.NewWithHostTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		&recordingProcessLauncher{},
		http.DefaultClient,
		realHostClock{},
		nil,
		nil,
		internalservice.SupervisorTestConfig{},
		internalservice.HostPolicyTestConfig{},
		runtimehost.Options{
			Platform:             managedHostPlatform(),
			CompatibilityChecker: &testCompatibilityChecker{},
			RuntimeEvidence:      modelseffects.NewOrderedRuntimeEvidenceRecorder(sink),
		},
	)

	_, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  managedModelName,
	})
	if !errors.Is(err, models.ErrHostProtocolIncompatible) {
		t.Fatalf("EnsureModelHost error = %v, want protocol incompatibility", err)
	}
	stage, class, ok := modelseffects.ClassifyRuntimeFailure(err)
	if !ok || stage != modelseffects.RuntimeStageProtocolLoad ||
		class != modelseffects.RuntimeFailureProtocolIncompatible {
		t.Fatalf("runtime classification = (%q, %q, %t), want PROTOCOL_LOAD/PROTOCOL_INCOMPATIBLE", stage, class, ok)
	}
	records := sink.snapshot()
	if len(records) != 1 {
		t.Fatalf("runtime evidence records = %d, want one host stage", len(records))
	}
	if records[0].Sequence != 1 || records[0].Kind != modelseffects.RuntimeEvidenceKindStage ||
		records[0].Stage != modelseffects.RuntimeStageProtocolLoad ||
		records[0].Class != modelseffects.RuntimeFailureProtocolIncompatible ||
		records[0].Outcome != modelseffects.RuntimeEvidenceOutcomeFailed {
		t.Fatalf("runtime evidence record = %#v, want bounded protocol-load failure", records[0])
	}
}

type runtimeEvidenceSink struct {
	mu      sync.Mutex
	records []modelseffects.RuntimeEvidenceRecord
}

func (sink *runtimeEvidenceSink) RecordRuntimeEvidence(
	record modelseffects.RuntimeEvidenceRecord,
) {
	if sink == nil {
		return
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.records = append(sink.records, record)
}

func (sink *runtimeEvidenceSink) snapshot() []modelseffects.RuntimeEvidenceRecord {
	if sink == nil {
		return nil
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]modelseffects.RuntimeEvidenceRecord(nil), sink.records...)
}
