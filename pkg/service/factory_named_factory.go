package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
)

// CreateNamedFactory persists one named-factory payload under the canonical
// layout and activates it through the idle-only runtime swap path.
func (fs *FactoryService) CreateNamedFactory(ctx context.Context, namedFactory factoryapi.Factory) (factoryapi.Factory, error) {
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}
	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	if err := apisurface.ValidateWritableNamedFactoryName(namedFactory.Name); err != nil {
		return factoryapi.Factory{}, err
	}

	payload, err := json.Marshal(namedFactory)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("marshal factory payload: %w", err)
	}

	factoryDir, err := factoryconfig.PersistNamedFactory(rootDir, string(namedFactory.Name), payload)
	if err != nil {
		switch {
		case errors.Is(err, factoryconfig.ErrNamedFactoryAlreadyExists):
			return factoryapi.Factory{}, factoryconfig.ErrNamedFactoryAlreadyExists
		case errors.Is(err, factoryconfig.ErrInvalidNamedFactory):
			return factoryapi.Factory{}, fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
		default:
			return factoryapi.Factory{}, err
		}
	}

	if err := fs.ActivateNamedFactory(ctx, string(namedFactory.Name)); err != nil {
		return factoryapi.Factory{}, err
	}

	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	created, err := factoryconfig.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("load created named factory %q: %w", namedFactory.Name, err)
	}
	return fs.serializeNamedFactory(namedFactory.Name, created, false)
}

// GetCurrentNamedFactory returns the durable current named-factory read model
// resolved entirely from the persisted pointer and canonical on-disk layout.
func (fs *FactoryService) GetCurrentNamedFactory(_ context.Context) (factoryapi.Factory, error) {
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	name, err := factoryconfig.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			currentRuntime := fs.currentRuntimeConfig()
			if currentRuntime != nil && sameFactoryDir(currentRuntime.FactoryDir(), rootDir) {
				return fs.serializeNamedFactory(apisurface.DefaultCurrentFactoryName, currentRuntime, true)
			}
			return factoryapi.Factory{}, ErrCurrentNamedFactoryNotFound
		}
		return factoryapi.Factory{}, fmt.Errorf("read current factory pointer: %w", err)
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("resolve current named factory %q: %w", name, err)
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	current, err := factoryconfig.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("load current named factory %q: %w", name, err)
	}

	return fs.serializeNamedFactory(factoryapi.FactoryName(name), current, true)
}

func (fs *FactoryService) currentFactoryDefinitionVersion(name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	return fs.currentFactoryDefinitionVersionAtRoot(rootDir, name)
}

func (fs *FactoryService) currentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	factoryDir := rootDir
	if name != apisurface.DefaultCurrentFactoryName {
		resolved, err := factoryconfig.ResolveNamedFactoryDir(rootDir, string(name))
		if err != nil {
			return factoryapi.HybridLogicalTimestamp{}, err
		}
		factoryDir = resolved
	}

	info, err := os.Stat(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("stat current factory definition: %w", err)
	}
	modified := info.ModTime().UTC()
	logical := modified.UnixNano()
	if logical < 0 {
		logical = 0
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  logical,
		Physical: modified,
	}, nil
}

func (fs *FactoryService) serializeNamedFactory(
	name factoryapi.FactoryName,
	current *factoryconfig.LoadedFactoryConfig,
	inlineBundledFiles bool,
) (factoryapi.Factory, error) {
	factoryCfg := current.FactoryConfig()
	if inlineBundledFiles && factoryCfg != nil {
		clonedFactoryCfg, err := factoryconfig.CloneFactoryConfig(factoryCfg)
		if err != nil {
			return factoryapi.Factory{}, fmt.Errorf("clone named factory config: %w", err)
		}
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, true); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline named factory bundled files: %w", err)
		}
		if err := factoryconfig.ApplySharedFactoryStarterWork(current.FactoryDir(), clonedFactoryCfg); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline shared factory starter work: %w", err)
		}
		factoryCfg = clonedFactoryCfg
	}
	generatedFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		current.FactoryDir(),
		factoryCfg,
		current,
		replay.WithGeneratedFactorySourceDirectory(current.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(fs.workflowID()),
	)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("serialize current named factory: %w", err)
	}
	generatedFactory.Name = factoryapi.FactoryName(name)
	return generatedFactory, nil
}

func sameFactoryDir(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
