package run

import (
	"bytes"
	"context"
	"testing"

	runtimehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestEmitReplayMetadataWarningsUsesConciseDeterministicComponents(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	warnings := []recordings.MetadataMismatchWarning{
		{Key: "workstations_hash"},
		{Key: "workers_hash"},
		{Key: "factory_hash"},
		{Key: "workers_hash"},
		{Key: "runtime_config_hash"},
	}
	if err := emitReplayMetadataWarnings(&output, warnings); err != nil {
		t.Fatalf("emitReplayMetadataWarnings() error = %v", err)
	}
	want := "Replay warning: current Factory Definition differs from the recording; affected components: Factory Definition, runtime configuration, workers, workstations. Replay continues with recorded inputs.\n"
	if output.String() != want {
		t.Fatalf("replay metadata warning = %q, want %q", output.String(), want)
	}
}

func TestOperationRunDisclosesReplayDriftAfterSuccessfulReplay(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	operation := &Operation{
		cfg:                    RunConfig{Output: &output, SuppressDashboardRendering: true},
		runner:                 stubFactoryService{run: func(context.Context) error { return nil }},
		replayMetadataWarnings: []recordings.MetadataMismatchWarning{{Key: "workers_hash"}},
	}
	if err := operation.Run(context.Background()); err != nil {
		t.Fatalf("Operation.Run() error = %v, want successful replay", err)
	}
	want := "Replay warning: current Factory Definition differs from the recording; affected components: workers. Replay continues with recorded inputs.\n"
	if output.String() != want {
		t.Fatalf("replay output = %q, want %q", output.String(), want)
	}
}

func TestOperationRunDoesNotDiscloseWhenReplayMetadataMatches(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	operation := &Operation{
		cfg:    RunConfig{Output: &output, SuppressDashboardRendering: true},
		runner: stubFactoryService{run: func(context.Context) error { return nil }},
	}
	if err := operation.Run(context.Background()); err != nil {
		t.Fatalf("Operation.Run() error = %v, want successful replay", err)
	}
	if output.Len() != 0 {
		t.Fatalf("replay output = %q, want no drift warning", output.String())
	}
}

var _ runtimehost.Service = stubFactoryService{}
