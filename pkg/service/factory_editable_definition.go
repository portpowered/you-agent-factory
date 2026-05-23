package service

import (
	"context"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
)

// GetCurrentFactory returns the canonical current factory definition together
// with durable optimistic-concurrency metadata.
func (fs *FactoryService) GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error) {
	return fs.GetCurrentNamedFactory(ctx)
}

// SaveCurrentFactory replaces the current named-factory definition with one
// complete canonical Factory payload and activates the resulting runtime.
func (fs *FactoryService) SaveCurrentFactory(ctx context.Context, request factoryapi.Factory) (factoryapi.Factory, error) {
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}
	current, sanitized, err := fs.validateEditableFactorySave(ctx, request)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntime(ctx); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.requireFreshEditableFactoryVersion(request.Version, current.Name); err != nil {
		return factoryapi.Factory{}, err
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	nextVersion := nextEditableFactoryVersion(
		current.Version,
		factory.EnsureClock(fs.clock).Now().UTC(),
	)
	payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	factoryDir, err := fs.replaceEditableFactoryDefinition(rootDir, request.Name, payload)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	replacement, err := fs.buildEditableFactoryReplacement(ctx, rootDir, factoryDir)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, request.Name, err)
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.activateReplacementRuntime(ctx, rootDir, string(request.Name), replacement); err != nil {
		return factoryapi.Factory{}, err
	}

	return fs.GetCurrentFactory(ctx)
}

func (fs *FactoryService) validateEditableFactorySave(
	ctx context.Context,
	request factoryapi.Factory,
) (factoryapi.Factory, factoryapi.Factory, error) {
	current, err := fs.GetCurrentFactory(ctx)
	if err != nil {
		return factoryapi.Factory{}, factoryapi.Factory{}, err
	}
	_, sanitized, err := fs.prepareEditableFactoryDefinitionSave("", current, request)
	if err != nil {
		return factoryapi.Factory{}, factoryapi.Factory{}, err
	}
	return current, sanitized, nil
}

func (fs *FactoryService) buildEditableFactoryReplacement(
	ctx context.Context,
	rootDir string,
	factoryDir string,
) (*replacementFactoryRuntime, error) {
	sessionID := defaultFactorySessionID
	if runState := fs.currentRunState(); runState != nil && strings.TrimSpace(runState.sessionID) != "" {
		sessionID = runState.sessionID
	}
	return fs.buildReplacementFactoryRuntime(ctx, rootDir, factoryDir, sessionID)
}
