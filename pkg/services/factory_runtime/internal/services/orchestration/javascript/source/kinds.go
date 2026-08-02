package workflowsource

// Kind identifies how a workflow source request should be interpreted.
type Kind string

const (
	KindFactoryID      Kind = "FACTORY_ID"
	KindFactoryInline  Kind = "FACTORY_INLINE"
	KindWorkflowFile   Kind = "WORKFLOW_FILE"
	KindWorkflowName   Kind = "WORKFLOW_NAME"
	KindInlineWorkflow Kind = "INLINE_WORKFLOW"
)

// LookupStage records which ordered lookup stage supplied a resolved workflow file.
type LookupStage string

const (
	LookupStageProjectClaude      LookupStage = "project-claude-workflows"
	LookupStageGlobalUser         LookupStage = "global-user-workflows"
	LookupStagePackageRelative    LookupStage = "package-relative-workflows"
	LookupStageNamedJavaScript    LookupStage = "named-javascript-factory"
	LookupStageExplicitFactory    LookupStage = "explicit-factory"
	LookupStageExplicitSourceKind LookupStage = "explicit-source-kind"
)

const (
	ProjectClaudeWorkflowsDir = ".claude/workflows"
	GlobalWorkflowsDirName    = "workflows"
	defaultDialect            = "javascript"
)
