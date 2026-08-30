package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	historicalquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/historical_query"
)

// resumeSourceCanonicalSessionID extracts only the canonical identity that a
// resumed runtime may retain for metrics. Public selectors and historical
// aliases are deliberately not promoted to a canonical scope.
func resumeSourceCanonicalSessionID(input recordings.LoadReplayInputResult) (string, error) {
	if input.Portable != nil && input.Legacy != nil {
		return "", fmt.Errorf("replay input returned both portable and legacy identities")
	}

	if metadataID := strings.TrimSpace(replayInputMetadataID(input.Metadata)); isCanonicalSessionUUID(metadataID) {
		for _, candidate := range replayInputCanonicalIDs(input) {
			if candidate != metadataID {
				return "", fmt.Errorf("replay input contains conflicting canonical Factory Session identities")
			}
		}
		return metadataID, nil
	}

	canonicalIDs := replayInputCanonicalIDs(input)
	if len(canonicalIDs) > 1 {
		return "", fmt.Errorf("replay input contains conflicting Factory Session identities")
	}
	if len(canonicalIDs) == 1 {
		return canonicalIDs[0], nil
	}
	return "", nil
}

// resumeSourceCanonicalSessionIDForPath combines identities carried by the
// decoded input with the canonical name of a Recordings-owned automatic
// target. Legacy JSON artifacts can contain only the public ~default event
// scope, while the automatic dated target already preserves the canonical
// UUID in its filename. Arbitrary explicit paths are intentionally ignored.
func resumeSourceCanonicalSessionIDForPath(
	input recordings.LoadReplayInputResult,
	path string,
) (string, error) {
	inputID, err := resumeSourceCanonicalSessionID(input)
	if err != nil {
		return "", err
	}
	pathID := automaticRecordingPathCanonicalSessionID(path)
	if inputID != "" && pathID != "" && inputID != pathID {
		return "", fmt.Errorf("replay input contains conflicting canonical Factory Session identities")
	}
	if inputID != "" {
		return inputID, nil
	}
	return pathID, nil
}

func automaticRecordingPathCanonicalSessionID(path string) string {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "." || !strings.EqualFold(filepath.Ext(cleanPath), ".json") {
		return ""
	}
	day := filepath.Dir(cleanPath)
	month := filepath.Dir(day)
	year := filepath.Dir(month)
	recordingsRoot := filepath.Dir(year)
	if strings.ToLower(filepath.Base(recordingsRoot)) != "recordings" ||
		strings.ToLower(filepath.Base(filepath.Dir(recordingsRoot))) != ".you-agent-factory" ||
		!isDatePathComponent(filepath.Base(year), 4) ||
		!isDatePathComponent(filepath.Base(month), 2) ||
		!isDatePathComponent(filepath.Base(day), 2) {
		return ""
	}
	canonicalID := strings.TrimSuffix(filepath.Base(cleanPath), filepath.Ext(cleanPath))
	if !isCanonicalSessionUUID(canonicalID) {
		return ""
	}
	return canonicalID
}

func isDatePathComponent(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func replayInputCanonicalIDs(input recordings.LoadReplayInputResult) []string {
	ids := make([]string, 0, 1)
	appendID := func(value string) {
		value = strings.TrimSpace(value)
		if !isCanonicalSessionUUID(value) {
			return
		}
		for _, existing := range ids {
			if existing == value {
				return
			}
		}
		ids = append(ids, value)
	}
	if input.Portable != nil {
		appendID(input.Portable.Session.ID)
	}
	if input.Legacy != nil {
		for _, event := range input.Legacy.Events {
			if event.Context.SessionID != nil {
				appendID(*event.Context.SessionID)
			}
		}
	}
	return ids
}

func replayInputMetadataID(metadata *recordings.ReplayInputMetadata) string {
	if metadata == nil {
		return ""
	}
	return metadata.FactorySessionID
}

func isCanonicalSessionUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

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
