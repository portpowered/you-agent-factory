package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	internalauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

// AuthoringLayout is the completed private authoring owner consumed by Definitions.
type AuthoringLayout = authoringlayout.Service

// AuthoredLayoutWriter is the completed split-layout writer.
type AuthoredLayoutWriter = internalauthoredlayout.Writer

// NewAuthoringLayout constructs only the authoring owner from completed ports.
func NewAuthoringLayout(
	validator factorydefinitions.Validator,
	mapInput factorydefinitions.FactoryLayoutPayloadMapper,
	decodeFactory factorydefinitions.FactoryConfigJSONDecoder,
	normalizeAuthored func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error),
	encodeFactory func(*factorydefinitions.FactoryConfig) ([]byte, error),
	write func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error,
	validate func(string) error,
	flatten factorydefinitions.FactoryLayoutFlattener,
	expand factorydefinitions.FactoryLayoutExpander,
	fileSystem factorydefinitions.PersistenceFileSystem,
	requireDefinitionDir factorydefinitions.DefinitionDirectoryRequirer,
	directories factorydefinitions.DirectoryReplacementStore,
) (AuthoringLayout, error) {
	return authoringlayoutwire.NewService(validator, mapInput, decodeFactory,
		normalizeAuthored, encodeFactory, write, validate, flatten, expand,
		fileSystem, requireDefinitionDir, directories)
}

// NewAuthoredLayoutWriter constructs one writer with its supplied filesystem effects.
func NewAuthoredLayoutWriter(
	fileSystem factorydefinitions.AuthoredLayoutWriterFileSystem,
	ensureInbox factorydefinitions.InputInboxSentinelEnsurer,
	writeAgents func(string, []byte) error,
) *AuthoredLayoutWriter {
	return internalauthoredlayout.NewWriter(
		authoredmapping.RenderWorkerAgentsMarkdown,
		authoredmapping.RenderWorkstationAgentsMarkdown,
		authoredmapping.RenderAgentsBody,
		writeAgents,
		authoredmapping.SafeFactoryLayoutSegment,
		authoredmapping.SafePromptFilePath,
		fileSystem,
		ensureInbox,
	)
}

// AuthoredAgentsFileWriter binds the filesystem effect without performing writes.
func AuthoredAgentsFileWriter(fileSystem factorydefinitions.AuthoredLayoutWriterFileSystem) func(string, []byte) error {
	return internalauthoredlayout.NewAgentsFileWriter(fileSystem)
}

// PreparedAuthoredLayoutWriter binds completed portable operations to one writer.
func PreparedAuthoredLayoutWriter(
	writer *AuthoredLayoutWriter,
	materialize factorydefinitions.PortableBundledFilesMaterializer,
	prune factorydefinitions.PortableBundledDocsPruner,
) func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error {
	return func(targetDir string, prepared *factorydefinitions.PreparedFactoryLayoutPayload, sourcePath string) error {
		return writer.WritePrepared(targetDir, prepared, sourcePath, materialize, prune)
	}
}

// AuthoredLayoutValidator binds read-only validation to the completed loader.
func AuthoredLayoutValidator(
	loader *compilationloading.Loader,
	validateWrites factorydefinitions.PortableBundledFileWritesValidator,
) func(string) error {
	return func(targetDir string) error {
		return loader.ValidateFactoryDirReadOnly(targetDir, nil, validateWrites)
	}
}

// AuthoredLayoutExpander binds expansion to the completed loader and writer.
func AuthoredLayoutExpander(
	loader *compilationloading.Loader,
	writer *AuthoredLayoutWriter,
	validateWrites factorydefinitions.PortableBundledFileWritesValidator,
	materialize factorydefinitions.PortableBundledFilesMaterializer,
	copyFiles factorydefinitions.PortableBundledFilesCopier,
) factorydefinitions.FactoryLayoutExpander {
	return func(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
		targetDir, sourceDir, sourcePath, config, canonical, err := loader.PrepareFactoryLayoutExpansion(path)
		if err != nil {
			return "", factorydefinitions.LayoutExpansionReport{}, err
		}
		report, err := writer.Expand(targetDir, sourceDir, sourcePath, config, canonical,
			validateWrites, materialize, copyFiles)
		return targetDir, report, err
	}
}

// AuthoredFactorySourceLoader supplies the Factory Definitions-owned authored
// source resolver to transport operations without exposing its implementation.
func AuthoredFactorySourceLoader(
	fileSystem factorydefinitions.AuthoredLayoutReaderFileSystem,
) factorydefinitions.AuthoredFactorySourceLoader {
	return internalauthoredlayout.NewFactorySourceLoader(fileSystem)
}
