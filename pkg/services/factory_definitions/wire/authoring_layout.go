package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
)

// AuthoringLayoutDependencies are the exact owner-local ports required to
// construct the private authoring_layout subservice behind the CTR-DEF root
// authoring slice.
type AuthoringLayoutDependencies struct {
	Validator          factorydefinitions.Validator
	MapInput           factorydefinitions.FactoryLayoutPayloadMapper
	Representation     Representation
	Loader             *compilationwire.Loader
	MaterializeFiles   factorydefinitions.PortableBundledFilesMaterializer
	ValidateWrites     factorydefinitions.PortableBundledFileWritesValidator
	PruneRemovedDocs   factorydefinitions.PortableBundledDocsPruner
	CopySupportedFiles factorydefinitions.PortableBundledFilesCopier
	AuthoredWriterFS   factoryeffect.AuthoredLayoutWriterFileSystem
	EnsureInbox        factoryeffect.InputInboxSentinelEnsurer
	PersistenceFS      factoryeffect.PersistenceFileSystem
	NamedPaths         factoryeffect.NamedPathResolver
	Directories        factoryeffect.DirectoryReplacementStore
}

// NewAuthoringLayoutService constructs the private authoring_layout subservice
// from owner-local composition ports.
func NewAuthoringLayoutService(
	deps AuthoringLayoutDependencies,
) (authoringlayout.Service, error) {
	writer := authoringlayoutwire.NewWriter(
		deps.Representation.RenderWorker,
		deps.Representation.RenderWorkstation,
		deps.Representation.RenderAgentsBody,
		authoringlayoutwire.NewAgentsFileWriter(deps.AuthoredWriterFS),
		deps.Representation.SafeLayoutSegment,
		deps.Representation.SafePromptPath,
		deps.AuthoredWriterFS,
		deps.EnsureInbox,
		compilationwire.NormalizeCanonicalWorkstationRuntime,
	)
	return authoringlayoutwire.NewService(authoringlayout.Dependencies{
		Validator:         deps.Validator,
		MapInput:          deps.MapInput,
		DecodeFactory:     deps.Representation.DecodeFactory,
		NormalizeAuthored: deps.Representation.NormalizeAuthored,
		EncodeFactory:     deps.Representation.EncodeFactory,
		Write: func(
			targetDir string,
			prepared *factorydefinitions.PreparedFactoryLayoutPayload,
			sourcePath string,
		) error {
			return writer.WritePrepared(
				targetDir,
				prepared,
				sourcePath,
				deps.MaterializeFiles,
				deps.PruneRemovedDocs,
			)
		},
		Validate: func(targetDir string) error {
			return deps.Loader.ValidateFactoryDirReadOnly(
				targetDir,
				nil,
				deps.ValidateWrites,
			)
		},
		Flatten: deps.Loader.FlattenFactoryConfig,
		Expand: func(path string) (string, factorydefinitions.LayoutExpansionReport, error) {
			targetDir, sourceDir, sourcePath, factoryConfig, canonical, err :=
				deps.Loader.PrepareFactoryLayoutExpansion(path)
			if err != nil {
				return "", factorydefinitions.LayoutExpansionReport{}, err
			}
			report, err := writer.Expand(
				targetDir,
				sourceDir,
				sourcePath,
				factoryConfig,
				canonical,
				deps.ValidateWrites,
				deps.MaterializeFiles,
				deps.CopySupportedFiles,
			)
			return targetDir, report, err
		},
		FileSystem:           deps.PersistenceFS,
		RequireDefinitionDir: deps.NamedPaths.RequireDefinitionDir,
		Directories:          deps.Directories,
	})
}

// AuthoredFactorySourceLoader supplies the Factory Definitions-owned authored
// source resolver to transport operations without exposing its implementation.
func AuthoredFactorySourceLoader(
	fileSystem factoryeffect.AuthoredLayoutReaderFileSystem,
) factorydefinitions.AuthoredFactorySourceLoader {
	return authoringlayoutwire.NewFactorySourceLoader(fileSystem)
}
