// Package workflowsource is a transitional compatibility shim for JavaScript
// orchestrator source loading.
//
// Deprecated: use pkg/orchestrators/javascript/source. This shim delegates to
// orchestrator ownership and is not the final package boundary.
package workflowsource

import jssource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"

type (
	ArtifactRootDecision = jssource.ArtifactRootDecision
	Context              = jssource.Context
	Diagnostic           = jssource.Diagnostic
	Kind                 = jssource.Kind
	LookupStage          = jssource.LookupStage
	Request              = jssource.Request
	Resolution           = jssource.Resolution
)

const (
	CodeArtifactRootInsideRepo = jssource.CodeArtifactRootInsideRepo
	CodeArtifactRootInvalid    = jssource.CodeArtifactRootInvalid
	CodeSourceConflict         = jssource.CodeSourceConflict
	CodeSourceNotFound         = jssource.CodeSourceNotFound
	ProjectClaudeWorkflowsDir  = jssource.ProjectClaudeWorkflowsDir

	KindFactoryID      = jssource.KindFactoryID
	KindFactoryInline  = jssource.KindFactoryInline
	KindInlineWorkflow = jssource.KindInlineWorkflow
	KindWorkflowFile   = jssource.KindWorkflowFile
	KindWorkflowName   = jssource.KindWorkflowName

	LookupStageExplicitFactory    = jssource.LookupStageExplicitFactory
	LookupStageExplicitSourceKind = jssource.LookupStageExplicitSourceKind
	LookupStageGlobalUser         = jssource.LookupStageGlobalUser
	LookupStageNamedJavaScript    = jssource.LookupStageNamedJavaScript
	LookupStagePackageRelative    = jssource.LookupStagePackageRelative
	LookupStageProjectClaude      = jssource.LookupStageProjectClaude
)

var (
	DefaultContext = jssource.DefaultContext
	Resolve        = jssource.Resolve
)
