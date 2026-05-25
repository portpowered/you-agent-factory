package prompting

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
	Name       string
	WorkID     string
	WorkTypeID string
	DataType   string
	TraceID    string
	ParentID   string
	Project    string
	Tags       map[string]string
	Payload    string
	Relations  []interfaces.Relation
	Content    []interfaces.WorkContentPart

	PreviousOutput    string
	RejectionFeedback string

	History PromptHistory
}

// PromptData is the data object passed to Go text/template execution.
type PromptData struct {
	Inputs  []TokenData
	Context PromptContext
}

// PromptHistory captures retry-aware execution history for prompt templates.
type PromptHistory struct {
	LastError     string
	FailureCount  int
	FailureLog    []interfaces.FailureRecord
	TotalVisits   int
	AttemptNumber int
}

// PromptContext provides execution environment details to prompt templates.
type PromptContext struct {
	WorkDir     string
	ArtifactDir string
	Project     string
	Env         map[string]string
}

// DefaultPromptRenderer is the standard PromptRenderer implementation.
type DefaultPromptRenderer struct{}

// Render parses the template string, builds PromptData from input tokens and
// workflow context, and returns the rendered prompt.
func (r *DefaultPromptRenderer) Render(tmpl string, tokens []interfaces.Token, wfCtx *factory_context.FactoryContext) (string, error) {
	if tmpl == "" {
		return r.getTokenPayloads(tokens)
	}

	data := BuildPromptData(tokens, wfCtx)

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

// BuildPromptData constructs PromptData from input tokens and workflow context.
func BuildPromptData(tokens []interfaces.Token, wfCtx *factory_context.FactoryContext) PromptData {
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
	td.Content = append([]interfaces.WorkContentPart(nil), color.Content...)

	if color.Tags != nil {
		td.Tags = color.Tags
		td.PreviousOutput = color.Tags["_last_output"]
		td.RejectionFeedback = color.Tags["_rejection_feedback"]
	}
	td.Project = promptProject(td.Tags, wfCtx)

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

var _ PromptRenderer = (*DefaultPromptRenderer)(nil)
