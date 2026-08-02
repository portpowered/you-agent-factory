// Package replayconfig owns replay runtime-config reconstruction inside the
// snapshots_portability subservice.
package replayconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotscontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/contracts"
)

type runtimeConfig struct {
	factory          *factorydefinitions.FactoryConfig
	factoryDir       string
	workers          map[string]*factorydefinitions.FactoryWorkerConfig
	workstations     map[string]*factorydefinitions.FactoryWorkstationConfig
	workstationsByID map[string]*factorydefinitions.FactoryWorkstationConfig
}

var _ snapshotscontracts.ReplayRuntimeConfig = (*runtimeConfig)(nil)

// Decode reconstructs a detached runtime lookup using the representation
// decoder selected by Wire.
func Decode(
	snapshot *factorydefinitions.FactorySnapshot,
	decodeFactoryConfig snapshotscontracts.FactoryConfigJSONDecoder,
) (snapshotscontracts.ReplayRuntimeConfig, error) {
	if snapshot == nil {
		return nil, errors.New("replay artifact factory is required")
	}
	if decodeFactoryConfig == nil {
		return nil, errors.New("Factory config JSON decoder is required")
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode replay artifact factory: %w", err)
	}
	factoryConfig, err := decodeFactoryConfig(payload)
	if err != nil {
		return nil, err
	}
	if factoryConfig == nil {
		return nil, errors.New("decoded Factory config is required")
	}
	var envelope struct {
		FactoryDirectory string `json:"factoryDirectory"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode replay artifact metadata: %w", err)
	}
	config := &runtimeConfig{
		factory:          factoryConfig,
		factoryDir:       envelope.FactoryDirectory,
		workers:          make(map[string]*factorydefinitions.FactoryWorkerConfig),
		workstations:     make(map[string]*factorydefinitions.FactoryWorkstationConfig),
		workstationsByID: make(map[string]*factorydefinitions.FactoryWorkstationConfig),
	}
	for _, worker := range factoryConfig.Workers {
		worker.ExecutorProvider = factorydefinitions.PermissivePublicFactoryWorkerProvider(
			strings.TrimSpace(worker.ExecutorProvider),
		)
		definition := factorydefinitions.CloneWorkerConfig(worker)
		config.workers[worker.Name] = &definition
	}
	for _, workstation := range factoryConfig.Workstations {
		definition := factorydefinitions.CloneWorkstationConfig(workstation)
		config.workstations[workstation.Name] = &definition
		if workstation.ID != "" {
			config.workstationsByID[workstation.ID] = &definition
		}
	}
	return config, nil
}

func (c *runtimeConfig) FactoryConfig() *factorydefinitions.FactoryConfig {
	if c == nil {
		return nil
	}
	return c.factory
}

func (c *runtimeConfig) FactoryDir() string {
	if c == nil {
		return ""
	}
	return c.factoryDir
}

func (c *runtimeConfig) RuntimeBaseDir() string {
	return c.FactoryDir()
}

func (c *runtimeConfig) Worker(
	name string,
) (*factorydefinitions.FactoryWorkerConfig, bool) {
	if c == nil {
		return nil, false
	}
	definition, ok := c.workers[name]
	return definition, ok
}

func (c *runtimeConfig) Workstation(
	name string,
) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	if c == nil {
		return nil, false
	}
	definition, ok := c.workstations[name]
	return definition, ok
}

func (c *runtimeConfig) WorkstationByID(
	id string,
) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	if c == nil {
		return nil, false
	}
	definition, ok := c.workstationsByID[id]
	return definition, ok
}
