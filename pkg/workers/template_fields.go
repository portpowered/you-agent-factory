package workers

import (
	"bytes"
	"fmt"
	"text/template"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ResolvedFields holds the resolved values of parameterized workstation fields.
type ResolvedFields struct {
	WorkingDirectory string            // resolved working directory (empty if not templated)
	Worktree         string            // resolved worktree path passed as --worktree to CLI dispatchers
	Env              map[string]string // resolved environment variables
}

// ResolveTemplateFields resolves Go template strings in workstation config fields
// using data from input tokens and workflow context. This uses the same PromptData
// structure as prompt rendering for consistency.
//
// If a template string contains no template directives, it is returned as-is.
// If a required template variable is missing or a template fails to execute,
// an error is returned so the caller can route the token to the failure state.
func ResolveTemplateFields(
	workingDirTemplate string,
	envTemplates map[string]string,
	tokens []interfaces.Token,
	wfCtx *factory_context.FactoryContext,
	worktreeTemplate ...string,
) (*ResolvedFields, error) {
	data := buildPromptData(tokens, wfCtx)
	result := &ResolvedFields{
		Env: make(map[string]string),
	}

	// Resolve working directory template.
	if workingDirTemplate != "" {
		resolved, err := resolveTemplate("working_directory", workingDirTemplate, data)
		if err != nil {
			return nil, fmt.Errorf("working_directory: %w", err)
		}
		result.WorkingDirectory = resolved
	}

	// Resolve environment variable templates.
	for key, tmpl := range envTemplates {
		resolved, err := resolveTemplate(fmt.Sprintf("env[%s]", key), tmpl, data)
		if err != nil {
			return nil, fmt.Errorf("env[%s]: %w", key, err)
		}
		result.Env[key] = resolved
	}

	// Resolve worktree template (optional variadic parameter for backwards compatibility).
	if len(worktreeTemplate) > 0 && worktreeTemplate[0] != "" {
		resolved, err := resolveTemplate("worktree", worktreeTemplate[0], data)
		if err != nil {
			return nil, fmt.Errorf("worktree: %w", err)
		}
		result.Worktree = resolved
	}

	return result, nil
}

// resolveTemplate parses and executes a single Go template string against PromptData.
// Returns an error if the template is malformed or references missing fields.
func resolveTemplate(name, tmpl string, data PromptData) (string, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("invalid template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}

	return buf.String(), nil
}

// applyResolvedFields creates a copy of the FactoryContext with resolved field
// values applied. If the base context is nil, a new one is created. Only non-empty
// resolved values override existing context fields.
func applyResolvedFields(base *factory_context.FactoryContext, resolved *ResolvedFields) *factory_context.FactoryContext {
	if resolved == nil {
		return base
	}

	// Start from a copy of the base context (or a new empty one).
	var ctx factory_context.FactoryContext
	if base != nil {
		ctx = *base
		if base.EnvVars != nil {
			ctx.EnvVars = make(map[string]string, len(base.EnvVars))
			for k, v := range base.EnvVars {
				ctx.EnvVars[k] = v
			}
		}
	}

	if resolved.WorkingDirectory != "" {
		ctx.WorkDirectory = resolved.WorkingDirectory
	}

	if len(resolved.Env) > 0 {
		if ctx.EnvVars == nil {
			ctx.EnvVars = make(map[string]string)
		}
		// Merge resolved env vars over the base — resolved values take precedence.
		for k, v := range resolved.Env {
			ctx.EnvVars[k] = v
		}
	}

	return &ctx
}
