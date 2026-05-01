package workers

import (
	"bytes"
	"strings"
	"text/template"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// PromptRenderer interpolates token color data into prompt templates using
// Go's text/template. The renderer builds PromptData from input tokens and
// workflow context, then executes the template against it.
type PromptRenderer interface {
	Render(tmpl string, tokens []interfaces.Token, wfCtx *factory_context.FactoryContext) (string, error)
}

// TokenData holds the per-token data extracted from a single input token's color and history.
// Available per-input via {{ (index .Inputs 0).FieldName }}.
type TokenData struct {
	Name       string                // {{ (index .Inputs 0).Name }} — human-readable identifier
	WorkID     string                // {{ (index .Inputs 0).WorkID }}
	WorkTypeID string                // {{ (index .Inputs 0).WorkTypeID }}
	DataType   string                // {{ (index .Inputs 0).DataType }} — "work" or "resource"
	TraceID    string                // {{ (index .Inputs 0).TraceID }}
	ParentID   string                // {{ (index .Inputs 0).ParentID }}
	Project    string                // {{ (index .Inputs 0).Project }} — token project tag, explicit context project, or neutral default
	Tags       map[string]string     // {{ index (index .Inputs 0).Tags "key" }}
	Payload    string                // {{ (index .Inputs 0).Payload }}
	Relations  []interfaces.Relation // {{ range (index .Inputs 0).Relations }}...{{ end }}

	PreviousOutput    string // {{ (index .Inputs 0).PreviousOutput }} — from Tags["_last_output"]
	RejectionFeedback string // {{ (index .Inputs 0).RejectionFeedback }} — from Tags["_rejection_feedback"]

	History PromptHistory // {{ (index .Inputs 0).History.LastError }}, {{ (index .Inputs 0).History.AttemptNumber }}
}

// PromptData is the data object passed to Go text/template execution.
type PromptData struct {
	Inputs []TokenData // {{ (index .Inputs 0).Payload }}, {{ (index .Inputs 1).Payload }}

	Context PromptContext // {{ .Context.WorkDir }}
}

// PromptHistory captures retry-aware execution history for prompt templates.
type PromptHistory struct {
	LastError     string                     // {{ (index .Inputs 0).History.LastError }}
	FailureCount  int                        // {{ (index .Inputs 0).History.FailureCount }}
	FailureLog    []interfaces.FailureRecord // {{ range (index .Inputs 0).History.FailureLog }}...{{ end }}
	TotalVisits   int                        // {{ (index .Inputs 0).History.TotalVisits }}
	AttemptNumber int                        // {{ (index .Inputs 0).History.AttemptNumber }} — 1-indexed
}

// PromptContext provides execution environment details to prompt templates.
type PromptContext struct {
	WorkDir     string            // {{ .Context.WorkDir }}
	ArtifactDir string            // {{ .Context.ArtifactDir }}
	Project     string            // {{ .Context.Project }} — explicit context project, first work-input project tag, or neutral default
	Env         map[string]string // {{ index .Context.Env "VAR_NAME" }}
}

// DefaultPromptRenderer is the standard PromptRenderer implementation.
type DefaultPromptRenderer struct{}

// Render parses the template string, builds PromptData from input tokens and
// workflow context, and returns the rendered prompt.
func (r *DefaultPromptRenderer) Render(tmpl string, tokens []interfaces.Token, wfCtx *factory_context.FactoryContext) (string, error) {
	if tmpl == "" {
		// just return the token payloads as the prompt.
		return r.getTokenPayloads(tokens)
	}

	data := buildPromptData(tokens, wfCtx)

	t, err := template.New("prompt").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// getTokenPayloads returns the payloads of non-resource tokens as a string.
// Resource tokens (semaphores) carry no meaningful payload and are skipped.
func (r *DefaultPromptRenderer) getTokenPayloads(tokens []interfaces.Token) (string, error) {
	payloads := []string{}
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		if token.Color.Payload == nil {
			continue
		}
		payloads = append(payloads, string(token.Color.Payload))
	}
	if len(payloads) == 0 {
		return "", nil
	}
	return strings.Join(payloads, "\n"), nil
}

// buildTokenData constructs a TokenData from a single petri.Token.
func buildTokenData(token interfaces.Token, wfCtx *factory_context.FactoryContext) TokenData {
	td := TokenData{
		Tags: make(map[string]string),
	}

	color := token.Color
	td.Name = color.Name
	td.WorkID = color.WorkID
	td.WorkTypeID = color.WorkTypeID
	td.DataType = string(color.DataType)
	td.TraceID = color.TraceID
	td.ParentID = color.ParentID
	td.Payload = string(color.Payload)
	td.Relations = color.Relations

	if color.Tags != nil {
		td.Tags = color.Tags
		td.PreviousOutput = color.Tags["_last_output"]
		td.RejectionFeedback = color.Tags["_rejection_feedback"]
	}
	td.Project = promptProject(td.Tags, wfCtx)

	// Build history from token history.
	history := token.History
	td.History = PromptHistory{
		LastError:  history.LastError,
		FailureLog: history.FailureLog,
	}
	td.History.FailureCount = len(history.FailureLog)

	totalVisits := 0
	for _, v := range history.TotalVisits {
		totalVisits += v
	}
	td.History.TotalVisits = totalVisits
	td.History.AttemptNumber = totalVisits + 1

	return td
}

// buildPromptData constructs PromptData from input tokens and workflow context.
// Each input token gets its own entry in Inputs with per-token context.
func buildPromptData(tokens []interfaces.Token, wfCtx *factory_context.FactoryContext) PromptData {
	var data PromptData

	for _, token := range tokens {
		data.Inputs = append(data.Inputs, buildTokenData(token, wfCtx))
	}

	if wfCtx != nil {
		data.Context = PromptContext{
			WorkDir:     wfCtx.WorkDirectory,
			ArtifactDir: wfCtx.ArtifactDir,
			Project:     promptContextProject(tokens, wfCtx),
			Env:         wfCtx.EnvVars,
		}
		if data.Context.Env == nil {
			data.Context.Env = make(map[string]string)
		}
	} else {
		data.Context.Project = promptContextProject(tokens, nil)
	}

	return data
}

func promptContextProject(tokens []interfaces.Token, wfCtx *factory_context.FactoryContext) string {
	if project := explicitContextProject(wfCtx); project != "" {
		return project
	}
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		if project := token.Color.Tags[factory_context.ProjectTagKey]; project != "" {
			return factory_context.ResolveProjectID(project, nil, nil)
		}
	}
	return factory_context.DefaultProjectID
}

func explicitContextProject(wfCtx *factory_context.FactoryContext) string {
	if wfCtx == nil {
		return ""
	}
	project := factory_context.ResolveProjectID(wfCtx.ProjectID, nil, nil)
	if project == factory_context.DefaultProjectID {
		return ""
	}
	return project
}

func promptProject(tags map[string]string, wfCtx *factory_context.FactoryContext) string {
	if tags != nil {
		if project := tags[factory_context.ProjectTagKey]; project != "" {
			return factory_context.ResolveProjectID(project, nil, nil)
		}
	}
	if wfCtx != nil && wfCtx.ProjectID != "" {
		return factory_context.ResolveProjectID(wfCtx.ProjectID, nil, nil)
	}
	return factory_context.DefaultProjectID
}

// Compile-time check.
var _ PromptRenderer = (*DefaultPromptRenderer)(nil)
