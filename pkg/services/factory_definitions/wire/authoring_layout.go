package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	internalauthoredlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/wire"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
)

// AuthoringLayoutDependencies are the exact owner-local ports required to
// construct the private authoring_layout subservice behind the CTR-DEF root
// authoring slice.
type AuthoringLayoutDependencies struct {
	Validator            factorydefinitions.Validator
	MapInput             factorydefinitions.FactoryLayoutPayloadMapper
	Loader               *compilationloading.Loader
	MaterializeFiles     factorydefinitions.PortableBundledFilesMaterializer
	ValidateWrites       factorydefinitions.PortableBundledFileWritesValidator
	PruneRemovedDocs     factorydefinitions.PortableBundledDocsPruner
	CopySupportedFiles   factorydefinitions.PortableBundledFilesCopier
	AuthoredWriterFS     factorydefinitions.AuthoredLayoutWriterFileSystem
	EnsureInbox          factorydefinitions.InputInboxSentinelEnsurer
	PersistenceFS        factorydefinitions.PersistenceFileSystem
	NamedPaths           factorydefinitions.NamedPathResolver
	Directories          factorydefinitions.DirectoryReplacementStore
}

// NewAuthoringLayoutService constructs the private authoring_layout subservice
// from owner-local composition ports.
func NewAuthoringLayoutService(
	deps AuthoringLayoutDependencies,
) (authoringlayout.Service, error) {
	mapper := factorymapping.NewFactoryConfigMapper()
	writer := internalauthoredlayout.NewWriter(
		authoredmapping.RenderWorkerAgentsMarkdown,
		authoredmapping.RenderWorkstationAgentsMarkdown,
		authoredmapping.RenderAgentsBody,
		internalauthoredlayout.NewAgentsFileWriter(deps.AuthoredWriterFS),
		authoredmapping.SafeFactoryLayoutSegment,
		authoredmapping.SafePromptFilePath,
		deps.AuthoredWriterFS,
		deps.EnsureInbox,
	)
	return authoringlayoutwire.NewService(authoringlayout.Dependencies{
		Validator:         deps.Validator,
		MapInput:          deps.MapInput,
		DecodeFactory:     mapper.Expand,
		NormalizeAuthored: authoredmapping.AuthoredFactoryConfigForExpandedLayout,
		EncodeFactory:     mapper.Flatten,
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
