package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

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
		return err
	}
	if path == "" {
		probe, err := inspectCurrentBoardHistory(ctx, opening.durableExecution.Service, opening.sessionID)
		if err != nil {
			return err
		}
		if !probe.hasDurableState {
			return nil
		}
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

type currentBoardFactsReader interface {
	LoadCurrentBoardFacts(context.Context, string) ([]factorydefinitions.FactoryEvent, error)
	MatchCurrentBoardWork(context.Context, string, *factorydefinitions.FactoryWorldState) (bool, error)
}

// selectMaximalCurrentBoard rejects ties and incomparable matching histories.
// Canonical event prefixes determine continuation, never filenames.
func selectMaximalCurrentBoard(histories map[string][]factorydefinitions.FactoryEvent) (string, error) {
	selected := ""
	for path, events := range histories {
		maximal := true
		for other, continuation := range histories {
			if other != path && len(continuation) > len(events) && currentBoardEventPrefix(events, continuation) {
				maximal = false
				break
			}
		}
		if !maximal {
			continue
		}
		if selected != "" {
			return "", fmt.Errorf("AMBIGUOUS_HISTORY: multiple retained board continuations match durable facts")
		}
		selected = path
	}
	if selected == "" {
		return "", fmt.Errorf("MISSING_HISTORY: no retained recording matches durable board facts")
	}
	return selected, nil
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

func currentBoardEventPrefix(prefix, events []factorydefinitions.FactoryEvent) bool {
	if len(prefix) > len(events) {
		return false
	}
	for index, event := range prefix {
		if !equalCurrentBoardEvent(event, events[index]) {
			return false
		}
	}
	return true
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
	if len(listed.Warnings) != 0 {
		return "", currentBoardHistoryFailure("", opening.sessionID, "UNREADABLE_RECORDING: legacy inventory contains unreadable histories; preserve them before retrying", nil)
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
	path, err := selectMaximalCurrentBoard(histories)
	if err != nil {
		return "", currentBoardHistoryFailure("", opening.sessionID, err.Error(), nil)
	}
	return path, nil
}

func (opening *sessionRuntimeOpening) publishCurrentBoardReference(ctx context.Context) error {
	if !opening.usesImplicitCurrentBoard() {
		return nil
	}
	store, err := opening.currentBoardReferenceStore()
	if err != nil {
		return err
	}
	return store.SaveCurrentBoard(ctx, opening.load.LoadedFactoryCfg.FactoryDir(), opening.configured.Recordings.RecordPath)
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
