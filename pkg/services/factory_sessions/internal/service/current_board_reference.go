package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type currentBoardReferencePersistence interface {
	LoadCurrentBoard(context.Context, string) (string, error)
	SaveCurrentBoard(context.Context, string, string) error
}

// The reference scopes selection, but is not authority for the bytes at its
// target. Validate the canonical recording's own repository identity before
// any runtime or writer opens it.
func validateCurrentBoardFactoryDirectory(events []factorydefinitions.FactoryEvent, directory string) error {
	found := false
	for _, event := range events {
		if event.Type != factorydefinitions.FactoryEventTypeRunRequest {
			continue
		}
		var payload struct {
			Factory struct {
				Directory string `json:"factoryDirectory"`
			} `json:"factory"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil ||
			!filepath.IsAbs(payload.Factory.Directory) ||
			filepath.Clean(payload.Factory.Directory) != filepath.Clean(directory) {
			return fmt.Errorf("selected recording belongs to another Factory directory or has no repository identity")
		}
		found = true
	}
	if found {
		return nil
	}
	return fmt.Errorf("selected recording has no canonical Factory identity")
}

func (opening *sessionRuntimeOpening) usesImplicitCurrentBoard() bool {
	selection := opening.sessionSelection
	return selection != nil && selection.Recording.ImplicitCurrentBoard &&
		opening.sessionID == factorysessions.DefaultSessionID &&
		selection.Mode == factorysessions.SessionRuntimeModeService && selection.Host.Port > 0 &&
		strings.TrimSpace(opening.configured.Recordings.ReplayPath) == "" &&
		strings.TrimSpace(opening.configured.Recordings.ResumePath) == ""
}

func (opening *sessionRuntimeOpening) currentBoardReferenceStore() (currentBoardReferencePersistence, error) {
	store, ok := opening.durableExecution.Service.(currentBoardReferencePersistence)
	if !ok {
		return nil, fmt.Errorf("current board reference persistence is unavailable")
	}
	return store, nil
}

func (r *Root) selectCurrentBoardReference(ctx context.Context, opening *sessionRuntimeOpening) error {
	if !opening.usesImplicitCurrentBoard() {
		return nil
	}
	store, err := opening.currentBoardReferenceStore()
	if err != nil {
		return err
	}
	path, err := store.LoadCurrentBoard(ctx, opening.load.LoadedFactoryCfg.FactoryDir())
	if err != nil {
		var local interface {
			CurrentBoardReferenceFailure()
			SnapshotFailureCause() string
		}
		if errors.As(err, &local) {
			return r.quarantineCurrentBoardArtifact(ctx, opening, "", local.SnapshotFailureCause())
		}
		return err
	}
	probe, err := inspectCurrentBoardHistory(ctx, opening.durableExecution.Service, opening.sessionID)
	if err != nil {
		return r.quarantineUnreadableCurrentBoard(ctx, opening, err)
	}
	opening.boardHistoryOpening = probe
	if !probe.hasDurableState {
		// A reference alone is not a durable board. Reserve a new recording
		// rather than replaying or overwriting the stale selected history.
		opening.startEmptyCurrentBoard()
		return nil
	}
	if path == "" {
		path, err = r.discoverLegacyCurrentBoard(ctx, opening)
		if err != nil {
			return err
		}
	}
	opening.configured.Recordings.RecordPath = path
	opening.sessionSelection.Recording.RecordPath = path
	opening.hasCurrentBoardReference = true
	// The durable probe still validates the snapshot; an existing reference must
	// never silently become an empty board when its selected artifact is gone.
	return nil
}

type currentBoardQuarantinePersistence interface {
	QuarantineCurrentBoard(context.Context, time.Time, string) (string, string, error)
}

type currentBoardStartupRecovery struct {
	file, quarantinedFile, cause string
}

func (r *Root) quarantineUnreadableCurrentBoard(ctx context.Context, opening *sessionRuntimeOpening, failure error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var classified interface{ SnapshotFailureCause() string }
	if !errors.As(failure, &classified) {
		return failure
	}
	cause := classified.SnapshotFailureCause()
	switch cause {
	case "INVALID_JSON", "INVALID_SCHEMA", "SIZE_LIMIT", "READ_FAILED":
	default:
		return failure
	}
	store, ok := opening.durableExecution.Service.(currentBoardQuarantinePersistence)
	if !ok || opening.clock == nil || r.generateSessionID == nil {
		return fmt.Errorf("current board quarantine persistence, clock and identity are required")
	}
	file, archive, err := store.QuarantineCurrentBoard(ctx, opening.clock.Now(), r.generateSessionID())
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	opening.startupRecovery = &currentBoardStartupRecovery{file: file, quarantinedFile: archive, cause: cause}
	opening.startEmptyCurrentBoard()
	return nil
}

func (opening *sessionRuntimeOpening) startEmptyCurrentBoard() {
	opening.emptyCurrentBoard = true
	opening.configured.Recordings.RecordPath = ""
	opening.sessionSelection.Recording.RecordPath = ""
}

type currentBoardFactsReader interface {
	LoadCurrentBoardFacts(context.Context, string) ([]factorydefinitions.FactoryEvent, error)
	MatchCurrentBoardWork(context.Context, string, *factorydefinitions.FactoryWorldState) (bool, error)
}

// selectUniqueCurrentBoard rejects every ambiguous match, including prefixes.
func selectUniqueCurrentBoard(histories map[string][]factorydefinitions.FactoryEvent) (string, error) {
	if len(histories) > 1 {
		return "", fmt.Errorf("AMBIGUOUS_HISTORY: multiple retained recordings match durable facts")
	}
	for path := range histories {
		return path, nil
	}
	return "", fmt.Errorf("MISSING_HISTORY: no retained recording matches durable board facts")
}

func currentBoardContainsFacts(events, facts []factorydefinitions.FactoryEvent) bool {
	if len(facts) == 0 {
		return false
	}
	index := 0
	for _, event := range events {
		if index < len(facts) && equalCurrentBoardEvent(event, facts[index]) {
			index++
		}
	}
	return index == len(facts)
}

func equalCurrentBoardEvent(left, right factorydefinitions.FactoryEvent) bool {
	var a, b any
	leftDecoder, rightDecoder := json.NewDecoder(bytes.NewReader(left.Payload)), json.NewDecoder(bytes.NewReader(right.Payload))
	leftDecoder.UseNumber()
	rightDecoder.UseNumber()
	if leftDecoder.Decode(&a) != nil || rightDecoder.Decode(&b) != nil {
		return false
	}
	left.Payload, right.Payload = nil, nil
	return reflect.DeepEqual(left, right) && reflect.DeepEqual(a, b)
}

func (r *Root) discoverLegacyCurrentBoard(ctx context.Context, opening *sessionRuntimeOpening) (string, error) {
	reader, ok := opening.durableExecution.Service.(currentBoardFactsReader)
	if !ok {
		return "", fmt.Errorf("current board durable witness reader is unavailable")
	}
	witness, err := reader.LoadCurrentBoardFacts(ctx, opening.sessionID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(opening.sessionSelection.SystemConfigHome) == "" {
		return "", fmt.Errorf("current board recording profile is required")
	}
	root := filepath.Join(opening.sessionSelection.SystemConfigHome, ".you-agent-factory", "recordings")
	listed, err := r.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root})
	if err != nil {
		return "", currentBoardHistoryFailure("", opening.sessionID, "UNREADABLE_RECORDING: legacy recording inventory could not be read", err)
	}
	histories := make(map[string][]factorydefinitions.FactoryEvent)
	for _, candidate := range listed.Sessions {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if candidate.FactorySessionID != opening.sessionID {
			continue
		}
		path, events, err := r.matchLegacyCurrentBoard(ctx, opening, reader, root, candidate.ArtifactReference, witness)
		if err != nil {
			return "", err
		}
		if events != nil {
			histories[path] = events
		}
	}
	path, err := selectUniqueCurrentBoard(histories)
	if err != nil {
		return "", currentBoardHistoryFailure("", opening.sessionID, err.Error(), nil)
	}
	for _, warning := range listed.Warnings {
		opening.skippedBoardRecordings = append(opening.skippedBoardRecordings, warning.ArtifactReference)
	}
	return path, nil
}

func (opening *sessionRuntimeOpening) publishCurrentBoardReference(ctx context.Context) error {
	if !opening.usesImplicitCurrentBoard() {
		if !opening.restoresExplicitCurrentBoard() {
			return nil
		}
		store, ok := opening.durableExecution.Service.(interface {
			SaveCurrentBoardIfAbsent(context.Context, string, string) error
		})
		if !ok {
			return fmt.Errorf("current board absent-reference persistence is unavailable")
		}
		path, err := filepath.Abs(opening.configured.Recordings.RecordPath)
		if err != nil {
			return err
		}
		return store.SaveCurrentBoardIfAbsent(ctx, opening.load.LoadedFactoryCfg.FactoryDir(), path)
	}
	store, err := opening.currentBoardReferenceStore()
	if err != nil {
		return err
	}
	return store.SaveCurrentBoard(ctx, opening.load.LoadedFactoryCfg.FactoryDir(), opening.configured.Recordings.RecordPath)
}

// Eligibility depends on confirmed reconstruction, never a fresh explicit target.
func (opening *sessionRuntimeOpening) restoresExplicitCurrentBoard() bool {
	selection := opening.sessionSelection
	return selection != nil && !selection.Recording.ImplicitCurrentBoard &&
		selection.Mode == factorysessions.SessionRuntimeModeService && selection.Host.Port > 0 &&
		opening.sessionID == factorysessions.DefaultSessionID &&
		strings.TrimSpace(selection.Recording.RecordPath) != "" &&
		strings.TrimSpace(opening.configured.Recordings.ResumePath) == "" &&
		strings.TrimSpace(opening.configured.Recordings.ReplayPath) == "" &&
		opening.restoredWorldState != nil &&
		currentBoardHistoryBelongsToFactory(opening.restoredEventHistory, opening.load.LoadedFactoryCfg.FactoryDir())
}

func (r *Root) reserveFreshCurrentBoard(ctx context.Context, opening *sessionRuntimeOpening) error {
	if !opening.usesImplicitCurrentBoard() || strings.TrimSpace(opening.configured.Recordings.RecordPath) != "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.recordingsRuntime == nil {
		return fmt.Errorf("current board recording target planner is unavailable")
	}
	selection := opening.sessionSelection
	target, err := r.recordingsRuntime.PlanLiveRecordingTarget(recordings.LiveRecordingTargetRequest{
		HomeDir: selection.SystemConfigHome, CanonicalSessionID: selection.CanonicalSessionID,
		ReportedSessionID: opening.sessionID,
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(target.ServicePath) == "" {
		return fmt.Errorf("current board recording target planner returned an empty path")
	}
	opening.configured.Recordings.RecordPath = target.ServicePath
	selection.Recording.RecordPath = target.ServicePath
	return nil
}

func (r *Root) matchLegacyCurrentBoard(ctx context.Context, opening *sessionRuntimeOpening, reader currentBoardFactsReader, root, artifact string, witness []factorydefinitions.FactoryEvent) (string, []factorydefinitions.FactoryEvent, error) {
	relative := filepath.FromSlash(artifact)
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(filepath.Clean(relative), ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("legacy inventory returned a foreign artifact reference")
	}
	path := filepath.Join(root, relative)
	history, err := restoreCurrentBoardHistory(r.recordingsService, path, opening.sessionID, false)
	if err != nil {
		return "", nil, err
	}
	if !currentBoardHistoryBelongsToFactory(history.events, opening.load.LoadedFactoryCfg.FactoryDir()) {
		return "", nil, nil
	}
	matched, err := reader.MatchCurrentBoardWork(ctx, opening.sessionID, history.state)
	if err != nil {
		return "", nil, err
	}
	if len(witness) > 0 && !currentBoardContainsFacts(history.events, witness) {
		return "", nil, nil
	}
	if matched || len(witness) > 0 {
		return path, history.events, nil
	}
	return "", nil, nil
}

func currentBoardHistoryBelongsToFactory(events []factorydefinitions.FactoryEvent, directory string) bool {
	return validateCurrentBoardFactoryDirectory(events, directory) == nil
}

func (r *Root) quarantineCurrentBoardArtifact(ctx context.Context, opening *sessionRuntimeOpening, artifact, cause string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store, ok := opening.durableExecution.Service.(interface {
		QuarantineCurrentBoardArtifact(context.Context, time.Time, string, string) (string, string, error)
	})
	if !ok || opening.clock == nil || r.generateSessionID == nil {
		return fmt.Errorf("current board artifact quarantine persistence, clock and identity are required")
	}
	file, archive, err := store.QuarantineCurrentBoardArtifact(ctx, opening.clock.Now(), r.generateSessionID(), artifact)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	opening.startupRecovery = &currentBoardStartupRecovery{file: file, quarantinedFile: archive, cause: cause}
	opening.startEmptyCurrentBoard()
	return nil
}

// A corrupt recording can be selected without parsing its damaged body only
// through a validated repository reference to this profile's recording tree.
// Explicit and foreign paths retain their rejecting behavior.
func (r *Root) quarantineSelectedCurrentBoardRecording(ctx context.Context, opening *sessionRuntimeOpening, failure error) error {
	if !opening.usesImplicitCurrentBoard() || !opening.hasCurrentBoardReference {
		return failure
	}
	var typed *currentBoardHistoryRestoreError
	if !errors.As(failure, &typed) || typed.code != currentBoardRecordingCorruptCode {
		return failure
	}
	root := filepath.Join(opening.sessionSelection.SystemConfigHome, ".you-agent-factory", "recordings")
	path := filepath.Clean(opening.configured.Recordings.RecordPath)
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return failure
	}
	return r.quarantineCurrentBoardArtifact(ctx, opening, path, "INVALID_SCHEMA")
}
