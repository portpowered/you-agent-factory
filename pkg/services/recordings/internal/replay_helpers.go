package internal

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	historicalquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/historical_query"
)

// NewRuntimeRootWithHistoricalQueryAndAppender constructs the process-scoped
// Recordings root with separate replacement and append effects. The optional
// append effect is used only for new .jsonl replay recordings; v1 readers and
// explicit .json replacement flows remain on writeFile.
func NewRuntimeRootWithHistoricalQueryAndAppender(
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	appendFile func(string, []byte) error,
	readFile recordings.RecordingReadFile,
	publication interface {
		Publish(context.Context, string, []byte) error
		Read(context.Context, string) ([]byte, error)
	},
	captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
	historicalQuery historicalquery.Service,
	clocks ...recordings.RecordingClock,
) recordings.Service {
	router := newRuntimeLedgerRouter(recordingClockNow(clocks...))
	projection := NewProjectionService()
	var writer recordings.RecordingSnapshotWriter
	var tickers recordings.RecordingFlushTickerFactory
	if writeFile != nil {
		writer = newReplayRecordingSnapshotWriter(
			writeFile,
			appendFile,
			replayV2TargetPreparation(readFile),
		)
		tickers = NewRecordingFlushTickerFactory()
	}
	service := NewServiceWithLifecycleEffectsAndHistoricalQueryAndLoggerAndReplaySource(
		router,
		projection,
		targets,
		writer,
		tickers,
		publication,
		historicalQuery,
		readFile,
		decodeSnapshot,
		logger,
		clocks...,
	)
	root, ok := service.(*combinedService)
	if !ok || root == nil {
		return nil
	}
	root.runtimeRouter = router
	root.runtimeSnapshotCapture = captureSnapshot
	root.replaySnapshotDecoder = decodeSnapshot
	root.replayConfigDecoder = decodeRuntimeConfig
	root.replayInputs = replayInputs
	return root
}

func replayV2TargetPreparation(readFile recordings.RecordingReadFile) func(string) error {
	if readFile == nil {
		return nil
	}
	return func(target string) error {
		data, err := readFile(target)
		if err == nil {
			if len(data) != 0 {
				return fmt.Errorf("replay v2 target already contains data")
			}
			return nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read replay v2 target: %w", err)
	}
}

// NewServiceWithLifecycleEffectsAndHistoricalQueryAndLoggerAndReplaySource
// constructs the process-scoped root with the selected logger, historical
// reader, and explicit resume source.
func NewServiceWithLifecycleEffectsAndHistoricalQueryAndLoggerAndReplaySource(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
	targetPlanner recordings.LiveRecordingTargetPlanner,
	writer recordings.RecordingSnapshotWriter,
	tickers recordings.RecordingFlushTickerFactory,
	publication portableArtifactPublication,
	historicalQuery historicalquery.Service,
	readFile recordings.RecordingReadFile,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	logger logging.Logger,
	clocks ...recordings.RecordingClock,
) recordings.Service {
	return newServiceWithLifecycleEffects(
		ledger,
		projection,
		targetPlanner,
		writer,
		tickers,
		publication,
		logger,
		historicalQuery,
		readFile,
		decodeFactorySnapshot,
		clocks...,
	)
}

func NewReplayClock(artifact *recordings.ReplayArtifact) recordings.Clock {
	if artifact == nil {
		return nil
	}
	return replayimpl.NewArtifactClock(artifact)
}

func NewReplayExecution(
	artifact *recordings.ReplayArtifact,
	decodeFactorySnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
) (
	providers.Service,
	platformprocess.CommandRunner,
	[]recordings.ReplayHook,
	recordings.CompletionDeliveryPlanner,
	error,
) {
	if artifact == nil {
		return nil, nil, nil, nil, nil
	}
	sideEffects, err := replayimpl.NewSideEffects(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay side effects: %w", err)
	}
	submissionHook, err := replayimpl.NewSubmissionHook(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay submission hook: %w", err)
	}
	workStateChangeHook, err := replayimpl.NewWorkStateChangeHook(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay work state change hook: %w", err)
	}
	deliveryPlan, err := replayimpl.NewCompletionDeliveryPlan(
		decodeFactorySnapshot,
		decodeRuntimeConfig,
		artifact,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("build replay completion delivery plan: %w", err)
	}
	return sideEffects,
		sideEffects,
		[]recordings.ReplayHook{submissionHook, workStateChangeHook},
		deliveryPlan,
		nil
}
