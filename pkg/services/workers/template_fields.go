package workers

// ResolvedTemplateFields contains the invocation-scoped runtime fields produced
// by Worker prompt/template resolution.
type ResolvedTemplateFields struct {
	WorkingDirectory string
	Worktree         string
	Env              map[string]string
}

// TemplateFieldResolver resolves Worker-owned runtime templates without
// exposing the prompting implementation package to peer services.
type TemplateFieldResolver func(
	workingDirectoryTemplate string,
	environmentTemplates map[string]string,
	tokens []Token,
	context *Context,
	worktreeTemplate string,
) (*ResolvedTemplateFields, error)
