// Deprecated: use github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source instead.
// This package is a Batch 001 compatibility shim; core runtime and API code must import the orchestrator-owned path directly.
package workflowsource

import target "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"

type (
	Request              = target.Request
	Context              = target.Context
	Diagnostic           = target.Diagnostic
	ArtifactRootDecision = target.ArtifactRootDecision
	Resolution           = target.Resolution
	Kind                 = target.Kind
	LookupStage          = target.LookupStage
)

const (
	CodeArtifactRootInvalid     = target.CodeArtifactRootInvalid
	CodeArtifactRootInsideRepo  = target.CodeArtifactRootInsideRepo
	KindFactoryID               = target.KindFactoryID
	KindFactoryInline           = target.KindFactoryInline
	KindWorkflowFile            = target.KindWorkflowFile
	KindWorkflowName            = target.KindWorkflowName
	KindInlineWorkflow          = target.KindInlineWorkflow
	LookupStageProjectClaude    = target.LookupStageProjectClaude
	LookupStageGlobalUser       = target.LookupStageGlobalUser
	LookupStagePackageRelative  = target.LookupStagePackageRelative
	LookupStageNamedJavaScript  = target.LookupStageNamedJavaScript
	LookupStageExplicitFactory  = target.LookupStageExplicitFactory
	LookupStageExplicitSourceKind = target.LookupStageExplicitSourceKind
	ProjectClaudeWorkflowsDir   = target.ProjectClaudeWorkflowsDir
	GlobalWorkflowsDirName      = target.GlobalWorkflowsDirName
	CodeSourceNotFound          = target.CodeSourceNotFound
	CodeSourceConflict          = target.CodeSourceConflict
	CodeUnsupportedKind         = target.CodeUnsupportedKind
)

func DefaultContext(projectRoot string) (Context, error) { return target.DefaultContext(projectRoot) }

func Resolve(req Request, ctx Context) Resolution { return target.Resolve(req, ctx) }
