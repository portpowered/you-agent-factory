package factory

import workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript/source"

type (
	WorkflowSourceKind                 = workflowsource.Kind
	WorkflowSourceLookupStage          = workflowsource.LookupStage
	WorkflowSourceRequest              = workflowsource.Request
	WorkflowSourceContext              = workflowsource.Context
	WorkflowSourceDiagnostic           = workflowsource.Diagnostic
	WorkflowSourceArtifactRootDecision = workflowsource.ArtifactRootDecision
	WorkflowSourceResolution           = workflowsource.Resolution
)

const (
	WorkflowSourceKindFactoryID      = workflowsource.KindFactoryID
	WorkflowSourceKindFactoryInline  = workflowsource.KindFactoryInline
	WorkflowSourceKindWorkflowFile   = workflowsource.KindWorkflowFile
	WorkflowSourceKindWorkflowName   = workflowsource.KindWorkflowName
	WorkflowSourceKindInlineWorkflow = workflowsource.KindInlineWorkflow

	WorkflowSourceLookupStageProjectClaude      = workflowsource.LookupStageProjectClaude
	WorkflowSourceLookupStageGlobalUser         = workflowsource.LookupStageGlobalUser
	WorkflowSourceLookupStagePackageRelative    = workflowsource.LookupStagePackageRelative
	WorkflowSourceLookupStageNamedJavaScript    = workflowsource.LookupStageNamedJavaScript
	WorkflowSourceLookupStageExplicitFactory    = workflowsource.LookupStageExplicitFactory
	WorkflowSourceLookupStageExplicitSourceKind = workflowsource.LookupStageExplicitSourceKind

	WorkflowSourceProjectClaudeWorkflowsDir = workflowsource.ProjectClaudeWorkflowsDir
	WorkflowSourceGlobalWorkflowsDirName    = workflowsource.GlobalWorkflowsDirName

	WorkflowSourceCodeArtifactRootInvalid    = workflowsource.CodeArtifactRootInvalid
	WorkflowSourceCodeArtifactRootInsideRepo = workflowsource.CodeArtifactRootInsideRepo
	WorkflowSourceCodeNotFound               = workflowsource.CodeSourceNotFound
	WorkflowSourceCodeConflict               = workflowsource.CodeSourceConflict
	WorkflowSourceCodeUnsupportedKind        = workflowsource.CodeUnsupportedKind
)
