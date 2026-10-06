package factorysessionexecution

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
)

// LoadCurrentBoard reads the reference through this opening's persistence
// owner. It does not consult the process-global Current Factory route.
func (s *JavaScriptRuntimeService) LoadCurrentBoard(ctx context.Context, factoryDirectory string) (string, error) {
	store, err := s.currentBoardStore()
	if err != nil {
		return "", err
	}
	return store.LoadCurrentBoard(ctx, factoryDirectory)
}

// SaveCurrentBoard publishes after reconstruction and before activation.
func (s *JavaScriptRuntimeService) SaveCurrentBoard(ctx context.Context, factoryDirectory, artifact string) error {
	store, err := s.currentBoardStore()
	if err != nil {
		return err
	}
	return store.SaveCurrentBoard(ctx, factoryDirectory, artifact)
}

func (s *JavaScriptRuntimeService) currentBoardStore() (runtimepersist.CurrentBoardStore, error) {
	if s == nil {
		return nil, ErrSessionNotFound
	}
	store, ok := s.persistence.(runtimepersist.CurrentBoardStore)
	if !ok {
		return nil, errors.New("current board reference persistence is unavailable")
	}
	return store, nil
}
