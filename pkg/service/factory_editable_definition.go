package service

import (
	"context"
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// GetEditableFactoryDefinition returns the complete current factory definition
// with persisted version metadata for graph-editor draft saves.
func (fs *FactoryService) GetEditableFactoryDefinition(ctx context.Context) (factoryapi.EditableFactoryDefinition, error) {
	current, err := fs.GetCurrentNamedFactory(ctx)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	version, err := fs.currentFactoryDefinitionVersion(current.Name)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	return factoryapi.EditableFactoryDefinition{
		FactoryDefinition: current,
		Version:           version,
	}, nil
}

// SaveEditableFactoryDefinition replaces the current named-factory definition
// with a complete submitted Factory payload and activates the resulting runtime.
func (fs *FactoryService) SaveEditableFactoryDefinition(ctx context.Context, request factoryapi.SaveEditableFactoryDefinitionRequest) (factoryapi.EditableFactoryDefinition, error) {
	if fs == nil {
		return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("factory service is required")
	}
	current, payload, err := fs.validateEditableFactorySave(ctx, request)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}

	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntime(ctx); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	if err := fs.requireFreshEditableFactoryVersion(request.BaseVersion, current.Name); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	factoryDir, err := fs.replaceEditableFactoryDefinition(rootDir, request.FactoryDefinition.Name, payload)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}

	replacement, err := fs.buildEditableFactoryReplacement(ctx, rootDir, factoryDir)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, request.FactoryDefinition.Name, err)
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	if err := fs.activateReplacementRuntime(ctx, rootDir, string(request.FactoryDefinition.Name), replacement); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}

	return fs.GetEditableFactoryDefinition(ctx)
}

func (fs *FactoryService) validateEditableFactorySave(
	ctx context.Context,
	request factoryapi.SaveEditableFactoryDefinitionRequest,
) (factoryapi.Factory, []byte, error) {
	current, err := fs.GetCurrentNamedFactory(ctx)
	if err != nil {
		return factoryapi.Factory{}, nil, err
	}
	_, payload, err := fs.prepareEditableFactoryDefinitionSave("", current, request)
	if err != nil {
		return factoryapi.Factory{}, nil, err
	}
	return current, payload, nil
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
