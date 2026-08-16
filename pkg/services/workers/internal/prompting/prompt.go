package prompting

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// PromptRenderer interpolates token color data into prompt templates using
// Go's text/template. The renderer builds PromptData from input tokens and
// workflow context, then executes the template against it.
type PromptRenderer interface {
	Render(tmpl string, tokens []workerexecution.Token, wfCtx *workerexecution.Context) (string, error)
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
	Relations  []work.Relation
	Content    []work.WorkContentPart

	// StructuredResult is the detached native JSON value produced by an
	// upstream schema-bound workstation. It is optional: a missing value is
	// rejected when a template attempts to use it rather than falling back to
	// Payload or artifact content.
	StructuredResult        any
	structuredResultPresent bool

	PreviousOutput    string
	RejectionFeedback string

	History PromptHistory
}

// PromptData is the data object passed to Go text/template execution.
type PromptData struct {
	Docs    map[string]string
	Inputs  []TokenData
	Context PromptContext
}

// PromptHistory captures retry-aware execution history for prompt templates.
type PromptHistory struct {
	LastError     string
	FailureCount  int
	FailureLog    []workerexecution.Failure
	TotalVisits   int
	AttemptNumber int
}

// PromptContext provides execution environment details to prompt templates.
type PromptContext struct {
	WorkDir     string
	ArtifactDir string
	Project     string
	SessionID   string
	Env         map[string]string
}

// DefaultPromptRenderer is the standard PromptRenderer implementation.
type DefaultPromptRenderer struct {
	FactoryDocs workerexecution.FactoryDocsLoader
}

// Render parses the template string, builds PromptData from input tokens and
// workflow context, and returns the rendered prompt.
func (r *DefaultPromptRenderer) Render(tmpl string, tokens []workerexecution.Token, wfCtx *workerexecution.Context) (string, error) {
	if tmpl == "" {
		return r.getTokenPayloads(tokens)
	}

	data, err := BuildPromptDataWithFactoryDocs(tokens, wfCtx, r.FactoryDocs)
	if err != nil {
		return "", err
	}

	t, err := template.New("prompt").Parse(tmpl)
	if err != nil {
		return "", err
	}
	if diagnostics := validateStructuredResultAvailability(t, structuredResultPresence(data.Inputs)); len(diagnostics) > 0 {
		return "", fmt.Errorf("render Worker prompt: %s", diagnostics[0].Message)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// BuildPromptData constructs PromptData from input tokens and workflow context.
func BuildPromptData(tokens []workerexecution.Token, wfCtx *workerexecution.Context) PromptData {
	data, _ := BuildPromptDataWithFactoryDocs(tokens, wfCtx, nil)
	return data
}

// BuildPromptDataWithFactoryDocs constructs prompt data using the injected
// Factory documentation operation.
func BuildPromptDataWithFactoryDocs(
	tokens []workerexecution.Token,
	wfCtx *workerexecution.Context,
	loader workerexecution.FactoryDocsLoader,
) (PromptData, error) {
	var data PromptData

	for _, token := range tokens {
		data.Inputs = append(data.Inputs, buildTokenData(token, wfCtx))
	}

	if wfCtx != nil {
		if strings.TrimSpace(wfCtx.FactoryDirectory) != "" {
			if loader == nil {
				return PromptData{}, fmt.Errorf("build Worker prompt data: Factory docs loader is required")
			}
			var err error
			data.Docs, err = loader(wfCtx.FactoryDirectory)
			if err != nil {
				return PromptData{}, fmt.Errorf("build Worker prompt data: load Factory docs: %w", err)
			}
		}
		data.Context = PromptContext{
			WorkDir:     wfCtx.WorkDirectory,
			ArtifactDir: wfCtx.ArtifactDir,
			Project:     promptContextProject(tokens, wfCtx),
			SessionID:   promptContextSessionID(wfCtx),
			Env:         wfCtx.EnvVars,
		}
		if data.Context.Env == nil {
			data.Context.Env = make(map[string]string)
		}
	} else {
		data.Context.Project = promptContextProject(tokens, nil)
	}
	if data.Docs == nil {
		data.Docs = map[string]string{}
	}

	return data, nil
}

func (r *DefaultPromptRenderer) getTokenPayloads(tokens []workerexecution.Token) (string, error) {
	payloads := []string{}
	for _, token := range tokens {
		if token.Color.DataType == workerexecution.DataTypeResource {
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

func buildTokenData(token workerexecution.Token, wfCtx *workerexecution.Context) TokenData {
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
	td.Content = append([]work.WorkContentPart(nil), color.Content...)
	td.StructuredResult = jsonvalue.Clone(color.StructuredResult)
	td.structuredResultPresent = jsonvalue.Present(color.StructuredResult, color.StructuredResultPresent)

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

func structuredResultPresence(inputs []TokenData) []bool {
	present := make([]bool, len(inputs))
	for index, input := range inputs {
		present[index] = input.structuredResultPresent
	}
	return present
}

func promptContextProject(tokens []workerexecution.Token, wfCtx *workerexecution.Context) string {
	if project := explicitContextProject(wfCtx); project != "" {
		return project
	}
	for _, token := range tokens {
		if token.Color.DataType == workerexecution.DataTypeResource {
			continue
		}
		if project := token.Color.Tags[workerexecution.ProjectTagKey]; project != "" {
			return workerexecution.ResolveProjectID(project)
		}
	}
	return workerexecution.DefaultProjectID
}

func promptContextSessionID(wfCtx *workerexecution.Context) string {
	if wfCtx == nil || strings.TrimSpace(wfCtx.SessionID) == "" {
		return workerexecution.DefaultSessionID
	}
	return strings.TrimSpace(wfCtx.SessionID)
}

func explicitContextProject(wfCtx *workerexecution.Context) string {
	if wfCtx == nil {
		return ""
	}
	project := workerexecution.ResolveProjectID(wfCtx.ProjectID)
	if project == workerexecution.DefaultProjectID {
		return ""
	}
	return project
}

func promptProject(tags map[string]string, wfCtx *workerexecution.Context) string {
	if tags != nil {
		if project := tags[workerexecution.ProjectTagKey]; project != "" {
			return workerexecution.ResolveProjectID(project)
		}
	}
	if wfCtx != nil && wfCtx.ProjectID != "" {
		return workerexecution.ResolveProjectID(wfCtx.ProjectID)
	}
	return workerexecution.DefaultProjectID
}

var _ PromptRenderer = (*DefaultPromptRenderer)(nil)
