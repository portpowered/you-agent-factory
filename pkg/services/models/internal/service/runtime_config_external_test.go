package service_test

import models "github.com/portpowered/infinite-you/pkg/services/models"

type modelRuntimeConfig = models.RuntimeConfig
type modelRuntimeWorker = models.RuntimeWorker
type modelRuntimeResource = models.RuntimeResource

type testFactoryConfig struct {
	Name             string
	Workers          []modelRuntimeWorker
	Resources        []modelRuntimeResource
	ResourceManifest *testResourceManifest
}

type testResourceManifest struct{ RequiredTools []testRequiredTool }
type testRequiredTool struct{ Name, Command string }

func projectTestModelsRuntimeConfig(factoryDir string, cfg *testFactoryConfig) *modelRuntimeConfig {
	if cfg == nil {
		return nil
	}
	return &modelRuntimeConfig{
		FactoryDirectory: factoryDir,
		Workers:          append([]modelRuntimeWorker(nil), cfg.Workers...),
		Resources:        append([]modelRuntimeResource(nil), cfg.Resources...),
	}
}
