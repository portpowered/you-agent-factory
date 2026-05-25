package prompting

import (
	"bytes"
	"fmt"
	"text/template"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ResolvedFields holds the resolved values of parameterized workstation fields.
type ResolvedFields struct {
	WorkingDirectory string
	Worktree         string
	Env              map[string]string
}

// ResolveTemplateFields resolves Go template strings in workstation config fields
// using data from input tokens and workflow context.
func ResolveTemplateFields(
	workingDirTemplate string,
	envTemplates map[string]string,
	tokens []interfaces.Token,
	wfCtx *factory_context.FactoryContext,
	worktreeTemplate string,
) (*ResolvedFields, error) {
	data := BuildPromptData(tokens, wfCtx)
	result := &ResolvedFields{
		Env: make(map[string]string),
	}

	if workingDirTemplate != "" {
		resolved, err := resolveTemplate("working_directory", workingDirTemplate, data)
		if err != nil {
			return nil, fmt.Errorf("working_directory: %w", err)
		}
		result.WorkingDirectory = resolved
	}

	for key, tmpl := range envTemplates {
		resolved, err := resolveTemplate(fmt.Sprintf("env[%s]", key), tmpl, data)
		if err != nil {
			return nil, fmt.Errorf("env[%s]: %w", key, err)
		}
		result.Env[key] = resolved
	}

	if worktreeTemplate != "" {
		resolved, err := resolveTemplate("worktree", worktreeTemplate, data)
		if err != nil {
			return nil, fmt.Errorf("worktree: %w", err)
		}
		result.Worktree = resolved
	}

	return result, nil
}

// ApplyResolvedFields creates a copy of the FactoryContext with resolved field
// values applied.
func ApplyResolvedFields(base *factory_context.FactoryContext, resolved *ResolvedFields) *factory_context.FactoryContext {
	if resolved == nil {
		return base
	}

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
		for k, v := range resolved.Env {
			ctx.EnvVars[k] = v
		}
	}

	return &ctx
}

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
