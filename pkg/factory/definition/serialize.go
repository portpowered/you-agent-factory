package factorydefinition

import (
	"fmt"
	"os"
	"path/filepath"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/replay"
)

func (s *Service) serializeNamedFactory(
	name string,
	current *factoryconfig.LoadedFactoryConfig,
	inlineBundledFiles bool,
) (*interfaces.FactorySnapshot, error) {
	factoryCfg := current.FactoryConfig()
	if inlineBundledFiles && factoryCfg != nil {
		clonedFactoryCfg, err := factoryconfig.CloneFactoryConfig(factoryCfg)
		if err != nil {
			return nil, fmt.Errorf("clone named factory config: %w", err)
		}
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, true, false); err != nil {
			return nil, fmt.Errorf("inline named factory bundled files: %w", err)
		}
		if err := factoryconfig.ApplySharedFactoryStarterWork(current.FactoryDir(), clonedFactoryCfg); err != nil {
			return nil, fmt.Errorf("inline shared factory starter work: %w", err)
		}
		factoryCfg = clonedFactoryCfg
	}
	workflowID := ""
	if s != nil && s.host != nil {
		workflowID = s.host.WorkflowID()
	}
	snapshot, err := replay.FactorySnapshotFromRuntimeConfig(
		current.FactoryDir(),
		factoryCfg,
		current,
		replay.WithFactorySnapshotSourceDirectory(current.FactoryDir()),
		replay.WithFactorySnapshotWorkflowID(workflowID),
	)
	if err != nil {
		return nil, fmt.Errorf("serialize current factory: %w", err)
	}
	namedSnapshot, err := snapshot.WithName(name)
	if err != nil {
		return nil, fmt.Errorf("name current factory snapshot: %w", err)
	}
	return namedSnapshot, nil
}

func (s *Service) currentFactoryDefinitionVersionAtRoot(rootDir string, name string) (interfaces.FactoryVersion, error) {
	factoryDir := rootDir
	if name != interfaces.DefaultCurrentFactoryName {
		resolved, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
		if err != nil {
			return interfaces.FactoryVersion{}, err
		}
		factoryDir = resolved
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if s != nil && s.host != nil {
		workstationLoader = s.host.WorkstationLoader()
	}
	current, err := configload.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return interfaces.FactoryVersion{}, fmt.Errorf("load current factory definition: %w", err)
	}
	if current.FactoryConfig().Version != nil {
		version := current.FactoryConfig().Version
		return interfaces.FactoryVersion{
			Logical:  version.Logical,
			Physical: version.Physical.UTC(),
		}, nil
	}

	info, err := os.Stat(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		return interfaces.FactoryVersion{}, fmt.Errorf("stat current factory definition: %w", err)
	}
	modified := info.ModTime().UTC()
	logical := modified.UnixNano()
	if logical < 0 {
		logical = 0
	}
	return interfaces.FactoryVersion{
		Logical:  logical,
		Physical: modified,
	}, nil
}

// SerializeNamedFactoryUpsertResponse returns the PUT upsert read model with thin
// portable DOC/SCRIPT bundled files (disk-backed targets without inline content).
func (s *Service) SerializeNamedFactoryUpsertResponse(
	name string,
	current *factoryconfig.LoadedFactoryConfig,
) (*interfaces.FactorySnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	factoryCfg := current.FactoryConfig()
	if factoryCfg != nil {
		clonedFactoryCfg, err := factoryconfig.CloneFactoryConfig(factoryCfg)
		if err != nil {
			return nil, fmt.Errorf("clone named factory config: %w", err)
		}
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, false, false); err != nil {
			return nil, fmt.Errorf("merge named factory portable bundled files: %w", err)
		}
		if err := factoryconfig.ApplySharedFactoryStarterWork(current.FactoryDir(), clonedFactoryCfg); err != nil {
			return nil, fmt.Errorf("inline shared factory starter work: %w", err)
		}
		factoryCfg = clonedFactoryCfg
	}
	workflowID := ""
	if s.host != nil {
		workflowID = s.host.WorkflowID()
	}
	snapshot, err := replay.FactorySnapshotFromRuntimeConfig(
		current.FactoryDir(),
		factoryCfg,
		current,
		replay.WithFactorySnapshotSourceDirectory(current.FactoryDir()),
		replay.WithFactorySnapshotWorkflowID(workflowID),
	)
	if err != nil {
		return nil, fmt.Errorf("serialize upsert factory: %w", err)
	}
	namedSnapshot, err := snapshot.WithName(name)
	if err != nil {
		return nil, fmt.Errorf("name upsert factory snapshot: %w", err)
	}
	return namedSnapshot, nil
}
