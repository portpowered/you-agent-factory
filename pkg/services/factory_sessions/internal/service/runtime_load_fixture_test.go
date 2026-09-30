package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func runtimeLoadedFactorySnapshotCapturer() factorydefinitions.LoadedFactorySnapshotCapturer {
	return func(source factorydefinitions.FactorySnapshotSource, factoryDir string, _ map[string]string) (*factorydefinitions.FactorySnapshot, error) {
		config, err := factorydefinitions.CloneFactoryConfig(source.FactoryConfig())
		if err != nil {
			return nil, err
		}
		for index, worker := range config.Workers {
			if effective, ok := source.Worker(worker.Name); ok {
				config.Workers[index] = factorydefinitions.CloneWorkerConfig(*effective)
			}
		}
		payload, err := json.Marshal(config)
		if err != nil {
			return nil, err
		}
		var fields map[string]any
		if err := json.Unmarshal(payload, &fields); err != nil {
			return nil, err
		}
		fields["factoryDirectory"] = factoryDir
		hash := sha256.Sum256(payload)
		workers, err := json.Marshal(config.Workers)
		if err != nil {
			return nil, err
		}
		workersHash := sha256.Sum256(workers)
		fields["metadata"] = map[string]string{
			"factory_hash":        fmt.Sprintf("sha256:%x", hash),
			"workers_hash":        fmt.Sprintf("sha256:%x", workersHash),
			"runtime_config_hash": fmt.Sprintf("sha256:%x", hash),
		}
		payload, err = json.Marshal(fields)
		if err != nil {
			return nil, err
		}
		snapshot := factorydefinitions.FactorySnapshot(payload)
		return &snapshot, nil
	}
}

func runtimeLoadFactorySnapshot(t *testing.T) *factorydefinitions.FactorySnapshot {
	t.Helper()

	snapshot := factorydefinitions.FactorySnapshot([]byte(factoryfixtures.CrossPathValidAlphaFactoryJSON))
	return &snapshot
}
