package factorydefinition

import (
	"fmt"
	"os"
	"path/filepath"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/replay"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

func (s *Service) serializeNamedFactory(
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
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, true, false); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline named factory bundled files: %w", err)
		}
		if err := factoryconfig.ApplySharedFactoryStarterWork(current.FactoryDir(), clonedFactoryCfg); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline shared factory starter work: %w", err)
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
		return factoryapi.Factory{}, fmt.Errorf("serialize current factory: %w", err)
	}
	generatedFactory, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("map current factory snapshot: %w", err)
	}
	generatedFactory.Name = factoryapi.FactoryName(name)
	return *generatedFactory, nil
}

func (s *Service) currentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	factoryDir := rootDir
	if name != apisurface.DefaultCurrentFactoryName {
		resolved, err := factoryconfig.ResolveNamedFactoryDir(rootDir, string(name))
		if err != nil {
			return factoryapi.HybridLogicalTimestamp{}, err
		}
		factoryDir = resolved
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if s != nil && s.host != nil {
		workstationLoader = s.host.WorkstationLoader()
	}
	current, err := configload.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("load current factory definition: %w", err)
	}
	if current.FactoryConfig().Version != nil {
		version := current.FactoryConfig().Version
		return factoryapi.HybridLogicalTimestamp{
			Logical:  apitypes.Int64String(version.Logical),
			Physical: version.Physical.UTC(),
		}, nil
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
		Logical:  apitypes.Int64String(logical),
		Physical: modified,
	}, nil
}

func (s *Service) withCurrentFactoryVersion(
	rootDir string,
	name factoryapi.FactoryName,
	serialized factoryapi.Factory,
) (factoryapi.Factory, error) {
	version, err := s.currentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized.Version = &version
	return serialized, nil
}

// SerializeNamedFactoryUpsertResponse returns the PUT upsert read model with thin
// portable DOC/SCRIPT bundled files (disk-backed targets without inline content).
func (s *Service) SerializeNamedFactoryUpsertResponse(
	name factoryapi.FactoryName,
	current *factoryconfig.LoadedFactoryConfig,
) (factoryapi.Factory, error) {
	if s == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	factoryCfg := current.FactoryConfig()
	if factoryCfg != nil {
		clonedFactoryCfg, err := factoryconfig.CloneFactoryConfig(factoryCfg)
		if err != nil {
			return factoryapi.Factory{}, fmt.Errorf("clone named factory config: %w", err)
		}
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, false, false); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("merge named factory portable bundled files: %w", err)
		}
		if err := factoryconfig.ApplySharedFactoryStarterWork(current.FactoryDir(), clonedFactoryCfg); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline shared factory starter work: %w", err)
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
		return factoryapi.Factory{}, fmt.Errorf("serialize upsert factory: %w", err)
	}
	generatedFactory, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("map upsert factory snapshot: %w", err)
	}
	generatedFactory.Name = factoryapi.FactoryName(name)
	return *generatedFactory, nil
}
