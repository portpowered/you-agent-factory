package loading

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loadedsource"
)

func (l *Loader) configureLoadedSourceMetadata(
	loadedSource factorydefinitions.MutableLoadedFactorySource,
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	sourcePath string,
	workstationLoader factorydefinitions.WorkstationLoader,
) {
	setter, ok := loadedSource.(factorydefinitions.LoadedFactorySourceMetadataSetter)
	if !ok {
		return
	}
	setter.SetLoadedFactoryVersion(l.loadedFactoryVersion(factoryConfig, sourcePath))

	activationSource, ok := loadedSource.(factorydefinitions.LoadedFactoryActivationSource)
	if !ok {
		return
	}
	activation := activationSource.FactoryActivationProvenance()
	if activation == nil || strings.TrimSpace(activation.LoadedSourceDigest) == "" {
		return
	}
	expectedDigest := activation.LoadedSourceDigest
	setter.SetAuthoredSourceComparison(func() (factorydefinitions.FactoryActivationState, error) {
		if strings.TrimSpace(factoryDir) == "" {
			return factorydefinitions.FactoryActivationStateNotActivated, nil
		}
		currentDigest, err := l.authoredSourceDigest(factoryDir, workstationLoader)
		if err != nil {
			return "", err
		}
		if currentDigest != expectedDigest {
			return factorydefinitions.FactoryActivationStateAuthoredChanged, nil
		}
		return factorydefinitions.FactoryActivationStateActive, nil
	})
}

func (l *Loader) loadedFactoryVersion(
	factoryConfig *factorydefinitions.FactoryConfig,
	sourcePath string,
) *factorydefinitions.FactoryVersion {
	if factoryConfig != nil && factoryConfig.Version != nil {
		version := *factoryConfig.Version
		version.Physical = version.Physical.UTC()
		return &version
	}
	if l == nil || l.fileSystem == nil || strings.TrimSpace(sourcePath) == "" {
		return nil
	}
	info, err := l.fileSystem.Stat(sourcePath)
	if err != nil {
		return nil
	}
	modified := info.ModTime().UTC()
	logical := modified.UnixNano()
	if logical < 0 {
		logical = 0
	}
	return &factorydefinitions.FactoryVersion{Logical: logical, Physical: modified}
}

// authoredSourceDigest reloads only the selected authored definition and its
// split runtime instructions. It deliberately skips materialization, repair,
// and persistence effects because inspection must never alter the source it is
// comparing.
func (l *Loader) authoredSourceDigest(
	factoryDir string,
	workstationLoader factorydefinitions.WorkstationLoader,
) (string, error) {
	source, resolvedFactoryDir, _, err := l.readFactoryConfigSource(factoryDir)
	if err != nil {
		return "", err
	}
	factoryConfig, err := l.decodeFactory(source.Data)
	if err != nil {
		return "", sourceContextError(source, "parse factory config", err)
	}
	if err := l.blockingLoadError(factoryConfig); err != nil {
		return "", sourceContextError(source, "validate factory config", err)
	}
	runtimeDefinitions, err := l.discoverRuntimeDefinitions(
		resolvedFactoryDir,
		factoryConfig,
		hasInlineRuntimeDefinitions(factoryConfig),
		workstationLoader,
	)
	if err != nil {
		return "", sourceContextError(source, "load runtime definitions", err)
	}
	digest, err := loadedsource.EffectiveLoadedSourceDigest(factoryConfig, runtimeDefinitions)
	if err != nil {
		return "", sourceContextError(source, "compute loaded Factory source digest", err)
	}
	return digest, nil
}
