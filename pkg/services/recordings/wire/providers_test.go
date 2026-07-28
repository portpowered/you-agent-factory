package wire_test

import (
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

func TestProviderBridgesConstructPublishedSurfaces(t *testing.T) {
	t.Parallel()

	if recordingswire.NewProjectionService() == nil {
		t.Fatal("NewProjectionService() returned nil")
	}

	ledger := recordingswire.NewRuntimeLedger(nil, time.Now, "wire-provider-gen", nil)
	if ledger == nil {
		t.Fatal("NewRuntimeLedger() returned nil")
	}

	if recordingswire.NewReplayClock(&recordings.ReplayArtifact{}) == nil {
		t.Fatal("NewReplayClock() returned nil")
	}

	_, _ = recordingswire.NewLifecycleRuntimeRecorder(
		time.Second,
		nil,
		time.Now,
		"",
		nil,
	)

	_, _, _, _, _ = recordingswire.NewReplayExecution(&recordings.ReplayArtifact{}, nil, nil)
}

func TestNewServiceWithProjectionRejectsMissingProjection(t *testing.T) {
	t.Parallel()

	service, err := recordingswire.NewServiceWithProjection(
		stubLedger{},
		nil,
		nil,
		func(string, []byte) error { return nil },
	)
	if err == nil {
		t.Fatal("NewServiceWithProjection() error = nil, want missing projection dependency")
	}
	if err.Error() != "construct Recordings: projection is required" {
		t.Fatalf("NewServiceWithProjection() error = %q, want projection required", err.Error())
	}
	if service != nil {
		t.Fatalf("NewServiceWithProjection() = %#v, want nil service", service)
	}
}
