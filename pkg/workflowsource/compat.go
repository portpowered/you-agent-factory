// Package workflowsource is a Batch 001 compatibility shim for the legacy root
// workflow source import path.
//
// Deprecated: canonical ownership for JavaScript workflow source lives in
// github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source. Core
// runtime and API code must import pkg/orchestrators/javascript/source directly.
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
	CodeArtifactRootInvalid       = target.CodeArtifactRootInvalid
	CodeArtifactRootInsideRepo    = target.CodeArtifactRootInsideRepo
	KindFactoryID                 = target.KindFactoryID
	KindFactoryInline             = target.KindFactoryInline
	KindWorkflowFile              = target.KindWorkflowFile
	KindWorkflowName              = target.KindWorkflowName
	KindInlineWorkflow            = target.KindInlineWorkflow
	LookupStageProjectClaude      = target.LookupStageProjectClaude
	LookupStageGlobalUser         = target.LookupStageGlobalUser
	LookupStagePackageRelative    = target.LookupStagePackageRelative
	LookupStageNamedJavaScript    = target.LookupStageNamedJavaScript
	LookupStageExplicitFactory    = target.LookupStageExplicitFactory
	LookupStageExplicitSourceKind = target.LookupStageExplicitSourceKind
	ProjectClaudeWorkflowsDir     = target.ProjectClaudeWorkflowsDir
	GlobalWorkflowsDirName        = target.GlobalWorkflowsDirName
	CodeSourceNotFound            = target.CodeSourceNotFound
	CodeSourceConflict            = target.CodeSourceConflict
	CodeUnsupportedKind           = target.CodeUnsupportedKind
)
