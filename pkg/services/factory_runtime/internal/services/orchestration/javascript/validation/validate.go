package workflowvalidation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

var syntaxLocationPattern = regexp.MustCompile(`on line (\d+) and column (\d+)`)

const (
	workflowScriptWrapperPrefix = "(function(){\n"
	workflowScriptWrapperSuffix = "\n})()"
)

// Request carries workflow source text and orchestrator config fields to validate.
type Request struct {
	Source     string
	SourceRef  string
	ConfigPath string
	Metadata   map[string]string
	ArgsSchema []byte
	AST        *js.AST
}

// ValidateLoaded checks loaded workflow source and remaps diagnostics to authored line numbers.
func ValidateLoaded(loaded LoadedSource, req Request) Result {
	req.Source = loaded.ExecutableSource
	if strings.TrimSpace(req.SourceRef) == "" {
		req.SourceRef = loaded.SourceRef
	}
	result := Validate(req)
	result.Issues = remapIssues(result.Issues, loaded.RemapLine)
	return result
}

// Validate checks workflow orchestrator config fields and JavaScript source without executing it.
func Validate(req Request) Result {
	var issues []Issue
	issues = append(issues, validateConfig(req)...)
	source := strings.TrimSpace(req.Source)
	if source == "" {
		return Result{Issues: issues}
	}

	parseSource, wrapped := workflowScriptSourceForValidation(source)
	req.Source = parseSource

	ast, err := js.Parse(parse.NewInputString(parseSource), js.Options{})
	if err != nil {
		issue := syntaxIssue(err, req)
		if wrapped {
			issue.Line = remapWrappedWorkflowIssueLine(issue.Line)
		}
		issues = append(issues, issue)
		return Result{Issues: issues}
	}
	req.AST = ast
	analyzed := analyzeJavaScriptSource(req)
	if wrapped {
		analyzed = remapWrappedWorkflowIssues(analyzed)
	}
	issues = append(issues, analyzed...)
	return Result{Issues: issues}
}

func workflowScriptSourceForValidation(source string) (string, bool) {
	if isWrappedWorkflowScriptSource(source) {
		return source, true
	}
	if _, err := js.Parse(parse.NewInputString(source), js.Options{}); err == nil {
		return source, false
	}
	wrapped := wrapWorkflowScriptSource(source)
	if _, err := js.Parse(parse.NewInputString(wrapped), js.Options{}); err == nil {
		return wrapped, true
	}
	return source, false
}

func isWrappedWorkflowScriptSource(source string) bool {
	trimmed := strings.TrimSpace(source)
	return strings.HasPrefix(trimmed, workflowScriptWrapperPrefix) ||
		strings.HasPrefix(trimmed, "(async function(){")
}

func wrapWorkflowScriptSource(source string) string {
	return workflowScriptWrapperPrefix + source + workflowScriptWrapperSuffix
}

func remapWrappedWorkflowIssueLine(line int) int {
	if line > 1 {
		return line - 1
	}
	return line
}

func remapWrappedWorkflowIssues(issues []Issue) []Issue {
	if len(issues) == 0 {
		return issues
	}
	out := make([]Issue, len(issues))
	for i, issue := range issues {
		out[i] = issue
		if issue.Line > 0 {
			out[i].Line = remapWrappedWorkflowIssueLine(issue.Line)
		}
	}
	return out
}

func syntaxIssue(err error, req Request) Issue {
	issue := Issue{
		Code:    CodeSyntaxError,
		Message: "workflow source has a JavaScript syntax error: " + err.Error(),
		Path:    sourcePath(req.SourceRef, req.ConfigPath),
	}
	if matches := syntaxLocationPattern.FindStringSubmatch(err.Error()); len(matches) == 3 {
		issue.Line, _ = strconv.Atoi(matches[1])
		issue.Column, _ = strconv.Atoi(matches[2])
		issue.Message = fmt.Sprintf(
			"workflow source has a JavaScript syntax error%s: %s",
			issue.LocationSuffix(),
			strings.TrimSpace(strings.Split(err.Error(), "\n")[0]),
		)
	}
	return issue
}
