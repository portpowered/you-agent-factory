package internal

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// CaptureInitialSnapshot captures the portable Factory Definition stored with
// a newly created runtime recording.
func CaptureInitialSnapshot(
	loaded factorydefinitions.LoadedFactorySource,
	preparePortableFactoryConfig factorydefinitions.PortableFactoryConfigPreparer,
	captureFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
) (*factorydefinitions.FactorySnapshot, error) {
	if loaded == nil || loaded.FactoryConfig() == nil {
		return nil, fmt.Errorf("loaded factory config is unavailable")
	}
	if preparePortableFactoryConfig == nil || captureFactorySnapshot == nil {
		return nil, fmt.Errorf("Factory snapshot adapters are required")
	}
	factoryCfg, err := preparePortableFactoryConfig(
		loaded.FactoryDir(),
		loaded.FactoryConfig(),
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("prepare initial Factory snapshot: %w", err)
	}
	return captureFactorySnapshot(
		preparedInitialFactorySnapshotSource{loaded: loaded, config: factoryCfg},
		loaded.FactoryDir(),
		nil,
	)
}

// preparedInitialFactorySnapshotSource keeps the portable-prepared config
// while forwarding value-free provenance from the effective loaded source.
// Initial snapshot capture must not convert this source through the explicit
// config adapter: that adapter cannot carry invocation-sensitive spans.
type preparedInitialFactorySnapshotSource struct {
	loaded factorydefinitions.LoadedFactorySource
	config *factorydefinitions.FactoryConfig
}

func (source preparedInitialFactorySnapshotSource) FactoryDir() string {
	if source.loaded == nil {
		return ""
	}
	return source.loaded.FactoryDir()
}

func (source preparedInitialFactorySnapshotSource) FactoryConfig() *factorydefinitions.FactoryConfig {
	return source.config
}

func (source preparedInitialFactorySnapshotSource) Worker(
	name string,
) (*factorydefinitions.FactoryWorkerConfig, bool) {
	if source.loaded == nil {
		return nil, false
	}
	return source.loaded.Worker(name)
}

func (source preparedInitialFactorySnapshotSource) Workstation(
	name string,
) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	if source.loaded == nil {
		return nil, false
	}
	return source.loaded.Workstation(name)
}

func (source preparedInitialFactorySnapshotSource) InvocationSensitiveJSONSpans() []factorydefinitions.InvocationSensitiveJSONSpan {
	provenance, ok := source.loaded.(factorydefinitions.InvocationSensitiveJSONSpanSource)
	if !ok {
		return nil
	}
	return append([]factorydefinitions.InvocationSensitiveJSONSpan(nil), provenance.InvocationSensitiveJSONSpans()...)
}

func (source preparedInitialFactorySnapshotSource) InvocationSensitiveJSONPointers() []string {
	provenance, ok := source.loaded.(interface {
		InvocationSensitiveJSONPointers() []string
	})
	if !ok {
		return nil
	}
	return append([]string(nil), provenance.InvocationSensitiveJSONPointers()...)
}
