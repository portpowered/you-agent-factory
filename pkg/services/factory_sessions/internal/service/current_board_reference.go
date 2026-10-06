package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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

func (opening *sessionRuntimeOpening) selectCurrentBoardReference(ctx context.Context) error {
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
		if probe.hasDurableState {
			return currentBoardHistoryFailure("", opening.sessionID,
				"MISSING_HISTORY: a durable board requires a matching retained recording before an implicit reference can be published", nil)
		}
		return nil
	}
	opening.configured.Recordings.RecordPath = path
	opening.sessionSelection.Recording.RecordPath = path
	opening.hasCurrentBoardReference = true
	// The durable probe still validates the snapshot; an existing reference must
	// never silently become an empty board when its selected artifact is gone.
	return nil
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
