package wire

import (
	factoryeffect "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
)

type (
	DefinitionActivationGateway        = factoryeffect.DefinitionActivationGateway
	AuthoredLayoutReaderFileSystem     = factoryeffect.AuthoredLayoutReaderFileSystem
	AuthoredLayoutWriterFileSystem     = factoryeffect.AuthoredLayoutWriterFileSystem
	InputInboxSentinelEnsurer          = factoryeffect.InputInboxSentinelEnsurer
	FactoryConfigPathSource            = factoryeffect.FactoryConfigPathSource
	DecisionEnvelopeService            = factoryeffect.DecisionEnvelopeService
	InvocationInterpolationService     = factoryeffect.InvocationInterpolationService
	InvocationOutputShapingService     = factoryeffect.InvocationOutputShapingService
	InvocationWorkTypeService          = factoryeffect.InvocationWorkTypeService
	Clock                              = factoryeffect.Clock
	VersionFileSystem                  = factoryeffect.VersionFileSystem
	LoadingFileSystem                  = factoryeffect.LoadingFileSystem
	RequiredToolPathLookup             = factoryeffect.RequiredToolPathLookup
	RequiredToolVersionProbe           = factoryeffect.RequiredToolVersionProbe
	NamedFactoryCatalogFileSystem      = factoryeffect.NamedFactoryCatalogFileSystem
	NamedPathFileSystem                = factoryeffect.NamedPathFileSystem
	NamedPathResolver                  = factoryeffect.NamedPathResolver
	NamedFactoryCandidatePathsResolver = factoryeffect.NamedFactoryCandidatePathsResolver
	NamedFactoryCandidatePaths         = factoryeffect.NamedFactoryCandidatePaths
	DefinitionDirectoryRequirer        = factoryeffect.DefinitionDirectoryRequirer
	PackagedFactoryInstaller           = factoryeffect.PackagedFactoryInstaller
	PackagedFactoryInstallOutcome      = factoryeffect.PackagedFactoryInstallOutcome
	PackagedFactoryInstallResult       = factoryeffect.PackagedFactoryInstallResult
	PackagedFactoryInstallParams       = factoryeffect.PackagedFactoryInstallParams
	PackagedInstallationFileSystem     = factoryeffect.PackagedInstallationFileSystem
	PackagedGoalPromptFileSystem       = factoryeffect.PackagedGoalPromptFileSystem
	DirectoryReplacementStore          = factoryeffect.DirectoryReplacementStore
	PersistenceFileSystem              = factoryeffect.PersistenceFileSystem
	PortableBundledFileInspection      = factoryeffect.PortableBundledFileInspection
	QuorumPolicyService                = factoryeffect.QuorumPolicyService
	ReplayRuntimeConfig                = factoryeffect.ReplayRuntimeConfig
	ReplayRuntimeConfigDecoder         = factoryeffect.ReplayRuntimeConfigDecoder
	FactorySnapshotJSONDecoder         = factoryeffect.FactorySnapshotJSONDecoder
	FactorySnapshotDirectoryLoader     = factoryeffect.FactorySnapshotDirectoryLoader
	ScaffoldFileSystem                 = factoryeffect.ScaffoldFileSystem
	ScaffoldOutput                     = factoryeffect.ScaffoldOutput
	TTSObservabilityService            = factoryeffect.TTSObservabilityService
	QuorumLineageInput                 = factoryeffect.QuorumLineageInput
	TTSInvocationWaitOutcome           = factoryeffect.TTSInvocationWaitOutcome
	TTSInvocationFailure               = factoryeffect.TTSInvocationFailure
	ValidationOperations               = factoryeffect.ValidationOperations
	WorkPropagationPolicyService       = factoryeffect.WorkPropagationPolicyService
	WorkPropagationPolicyFunc          = factoryeffect.WorkPropagationPolicyFunc
	WorkstationExecutionPolicyService  = factoryeffect.WorkstationExecutionPolicyService
	FileReader                         = factoryeffect.FileReader
	CurrentFactoryPointerReader        = factoryeffect.CurrentFactoryPointerReader
	CurrentFactoryPointerWriter        = factoryeffect.CurrentFactoryPointerWriter
	FactoryLayoutPayloadMapper         = factoryeffect.FactoryLayoutPayloadMapper
	FactoryLayoutPayloadPreparer       = factoryeffect.FactoryLayoutPayloadPreparer
	NamedFactoryPersister              = factoryeffect.NamedFactoryPersister
	FactoryLayoutReplacer              = factoryeffect.FactoryLayoutReplacer
	FactoryLayoutFlattener             = factoryeffect.FactoryLayoutFlattener
	FactoryLayoutExpander              = factoryeffect.FactoryLayoutExpander
	FactoryConfigJSONDecoder           = factoryeffect.FactoryConfigJSONDecoder
	FactoryConfigJSONEncoder           = factoryeffect.FactoryConfigJSONEncoder
	CanonicalFactoryJSONLoader         = factoryeffect.CanonicalFactoryJSONLoader
	FactoryConfigCloner                = factoryeffect.FactoryConfigCloner
	PortableBundledFilesApplier        = factoryeffect.PortableBundledFilesApplier
	FactoryStarterWorkApplier          = factoryeffect.FactoryStarterWorkApplier
	PortableBundledFilesMaterializer   = factoryeffect.PortableBundledFilesMaterializer
	PortableBundledFileWritesValidator = factoryeffect.PortableBundledFileWritesValidator
	PortableBundledFilesCopier         = factoryeffect.PortableBundledFilesCopier
	PortableBundledDocsPruner          = factoryeffect.PortableBundledDocsPruner
	PortableBundledFileSourceResolver  = factoryeffect.PortableBundledFileSourceResolver
	FactorySnapshotObjectMapper        = factoryeffect.FactorySnapshotObjectMapper
)
