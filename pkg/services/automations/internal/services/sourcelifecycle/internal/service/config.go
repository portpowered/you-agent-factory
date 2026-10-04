package service

import (
	"fmt"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type runtimeSnapshotConfig struct {
	factoryDir     string
	runtimeBaseDir string
	runtimeID      string
	config         interfaces.FactoryConfig
}

func (c *runtimeSnapshotConfig) FactoryDir() string {
	if c == nil {
		return ""
	}
	return c.factoryDir
}

func (c *runtimeSnapshotConfig) RuntimeBaseDir() string {
	if c == nil {
		return ""
	}
	return c.runtimeBaseDir
}

// RuntimeInstanceID is an optional private lookup used by durable source
// effects that must keep checkpoints isolated even when two runtimes share a
// Factory directory and source names.
func (c *runtimeSnapshotConfig) RuntimeInstanceID() string {
	if c == nil {
		return ""
	}
	return c.runtimeID
}

func (c *runtimeSnapshotConfig) FactoryConfig() *interfaces.FactoryConfig {
	if c == nil {
		return nil
	}
	return &c.config
}

func (c *runtimeSnapshotConfig) Worker(name string) (*interfaces.FactoryWorkerConfig, bool) {
	if c == nil {
		return nil, false
	}
	for index := range c.config.Workers {
		if c.config.Workers[index].Name == name {
			return &c.config.Workers[index], true
		}
	}
	return nil, false
}

func (c *runtimeSnapshotConfig) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	if c == nil {
		return nil, false
	}
	for index := range c.config.Workstations {
		if c.config.Workstations[index].Name == name {
			return &c.config.Workstations[index], true
		}
	}
	return nil, false
}

var _ interfaces.RuntimeConfigLookup = (*runtimeSnapshotConfig)(nil)

func newRuntimeSnapshotConfig(runtimeID string, snapshot interfaces.RuntimeSnapshot) (*runtimeSnapshotConfig, error) {
	config, err := interfaces.CloneFactoryConfig(&snapshot.EffectiveFactory)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, fmt.Errorf("Factory Definition configuration is required")
	}
	if len(snapshot.Workers) > 0 {
		config.Workers = make([]interfaces.FactoryWorkerConfig, len(snapshot.Workers))
		for index, worker := range snapshot.Workers {
			config.Workers[index] = interfaces.CloneWorkerConfig(worker)
		}
	}
	if len(snapshot.Workstations) > 0 {
		config.Workstations = make([]interfaces.FactoryWorkstationConfig, len(snapshot.Workstations))
		for index, workstation := range snapshot.Workstations {
			config.Workstations[index] = interfaces.CloneWorkstationConfig(workstation)
		}
	}
	return &runtimeSnapshotConfig{
		factoryDir:     snapshot.FactoryDir,
		runtimeBaseDir: snapshot.RuntimeBaseDir,
		runtimeID:      runtimeID,
		config:         *config,
	}, nil
}
