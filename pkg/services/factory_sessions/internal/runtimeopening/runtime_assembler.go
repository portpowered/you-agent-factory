package runtimeopening

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// newOrderlyRecordingFlush adapts the already-composed Recordings root to the
// initializer lifecycle boundary. A missing record path means that this
// runtime has no live recording to flush, so the orderly-stop phase remains a
// no-op.
func newOrderlyRecordingFlush(
	service recordings.Service,
	recordingID string,
	recordPath string,
) lifecycle.OrderlyStopOperation {
	if service == nil || strings.TrimSpace(recordingID) == "" || strings.TrimSpace(recordPath) == "" {
		return nil
	}
	return func(ctx context.Context) error {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if _, err := service.FlushRecording(recordings.FlushRecordingRequest{
			RecordingID: recordings.RecordingID(recordingID),
		}); err != nil {
			return fmt.Errorf("flush live recording during orderly shutdown: %w", err)
		}
		return nil
	}
}

// FactoryRuntimeAssembler is the session-owned runtime assembly operation
// constructed once by Wire. Assemble receives only invocation/session values;
// its product-policy dependencies are already bound.
type FactoryRuntimeAssembler interface {
	Assemble(
		context.Context,
		string,
		string,
		bool,
		string,
		string,
		string,
		string,
		factorydefinitions.WorkstationLoader,
		factoryruntime.LoadedFactoryLoader,
		providers.Service,
		platformprocess.CommandRunner,
		platformprocess.CommandRunner,
		*workers.MockWorkersConfig,
		factorydefinitions.RuntimeMode,
		factoryruntime.Scheduler,
		bool,
		recordings.SubmissionRecorder,
		recordings.DispatchRecorder,
		string,
		factoryruntime.RuntimeLogStorageConfig,
		factoryruntime.RuntimeFileLoggingPolicy,
		factoryruntime.RuntimeMetricsPolicy,
		string,
		factoryruntime.RuntimeMetricsStorageConfig,
		time.Duration,
		string,
		string,
		bool,
		bool,
		*bool,
		factoryruntime.Clock,
		*zap.Logger,
		factoryruntime.WorkersMockCommandRunnerFactory,
		func(string) workers.ProgressPublisher,
		func(string) func(string),
		factoryruntime.PetriMutationRecorder,
		factoryruntime.WorldStateProjector,
		recordings.RuntimeOpening,
		factorydefinitions.InitialFactorySnapshotFactory,
		string,
		string,
		string,
		factorydefinitions.MutableLoadedFactorySource,
		string,
		*factorydefinitions.ReplayArtifact,
		*recordings.LoadResumeInputResult,
		*factorydefinitions.FactoryWorldState,
		[]factorydefinitions.FactoryEvent,
		automations.Service,
		bool,
	) (
		runtimeports.RuntimeReplacementBuilder,
		runtimeports.RuntimeInstance,
		factoryruntime.SessionBuildSpec,
		runtimeports.RuntimeLifecycle,
		runtimeports.RuntimeSidecarService,
		error,
	)
}
