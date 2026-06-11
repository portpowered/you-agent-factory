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

	ast, err := js.Parse(parse.NewInputString(source), js.Options{})
	if err != nil {
		issues = append(issues, syntaxIssue(err, req))
		return Result{Issues: issues}
	}
	req.AST = ast
	issues = append(issues, analyzeJavaScriptSource(req)...)
	return Result{Issues: issues}
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
