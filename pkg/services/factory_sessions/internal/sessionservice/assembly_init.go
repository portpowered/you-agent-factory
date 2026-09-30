package service

import (
	"errors"
	"fmt"
	"path/filepath"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
)

// PrepareNewFactoryScaffold creates the portable scaffold before runtime
// opening tries to resolve a Current Factory. Existing scaffolds are passed
// through the idempotent initializer without replacing customer edits.
func (a *Assembly) PrepareNewFactoryScaffold(folderPath string, initialize factorysessions.FactoryScaffoldInitializer) (string, error) {
	if a == nil || initialize == nil || a.namedPaths == nil {
		return "", fmt.Errorf("initialize Factory scaffold: required dependencies are unavailable")
	}
	resolvedFolder, err := logicaltarget.ResolveSessionFolder(folderPath, a.resolveHome, a.directoryInspection)
	if err != nil {
		return "", err
	}
	factoryDir, err := a.namedPaths.ResolveCurrentDir(resolvedFolder)
	if err == nil {
		return factoryDir, initialize(factoryDir)
	}
	if !errors.Is(err, factorydefinitions.ErrLayoutNotFound) {
		return "", err
	}
	nestedFactoryDir := filepath.Join(resolvedFolder, factorydefinitions.FactoryDir)
	if nestedCurrent, nestedErr := a.namedPaths.ResolveCurrentDir(nestedFactoryDir); nestedErr == nil {
		return nestedCurrent, initialize(nestedCurrent)
	} else if !errors.Is(nestedErr, factorydefinitions.ErrLayoutNotFound) {
		return "", nestedErr
	}
	if err := logicaltarget.ValidateInitNewFactoryNestedDir(resolvedFolder, a.directoryInspection); err != nil {
		return "", err
	}
	return nestedFactoryDir, initialize(nestedFactoryDir)
}
