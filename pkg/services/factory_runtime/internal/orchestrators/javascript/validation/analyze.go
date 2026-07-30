package workflowvalidation

import (
	"fmt"
	"strings"

	"github.com/tdewolff/parse/v2/js"
)

type sourceAnalyzer struct {
	source string
	ref    string
	path   string
	issues []Issue
	seen   map[string]struct{}
}

func analyzeJavaScriptSource(req Request) []Issue {
	analyzer := &sourceAnalyzer{
		source: req.Source,
		ref:    req.SourceRef,
		path:   req.ConfigPath,
		seen:   make(map[string]struct{}),
	}
	js.Walk(analyzer, req.AST)
	return analyzer.issues
}

func (a *sourceAnalyzer) Enter(n js.INode) js.IVisitor {
	switch node := n.(type) {
	case *js.ImportStmt:
		a.addIssue(CodeForbiddenHostAccess, "ES module import is not supported in MVP workflows", node)
	case *js.ImportMetaExpr:
		a.addIssue(CodeForbiddenHostAccess, "import.meta is not supported in MVP workflows", node)
	case *js.CallExpr:
		a.inspectCall(node)
	case *js.DotExpr:
		a.inspectDot(node)
	case *js.Var:
		a.inspectIdentifierUse(node)
	}
	return a
}

func (a *sourceAnalyzer) Exit(js.INode) {}

func (a *sourceAnalyzer) inspectCall(call *js.CallExpr) {
	switch callee := call.X.(type) {
	case *js.DotExpr:
		root, member, ok := memberAccess(callee)
		if !ok {
			return
		}
		if rootVar, isVar := callee.X.(*js.Var); isVar && isDeclaredLocalBinding(rootVar) {
			return
		}
		if msg, forbidden := forbiddenRootGlobals[root]; forbidden {
			a.addIssue(CodeForbiddenHostAccess, msg, call)
			return
		}
		switch root {
		case "workflow":
			if _, supported := supportedWorkflowMembers[member]; !supported {
				a.addIssue(CodeUnsupportedPrimitive, fmt.Sprintf("unsupported workflow primitive workflow.%s", member), call)
				return
			}
			a.validateSupportedPrimitiveShape(call, "workflow."+member)
		case "agent":
			if _, supported := supportedAgentMembers[member]; !supported {
				a.addIssue(CodeUnsupportedPrimitive, fmt.Sprintf("unsupported workflow primitive agent.%s", member), call)
				return
			}
			a.validateSupportedPrimitiveShape(call, "agent."+member)
		default:
			if _, supported := supportedRootGlobals[root]; supported {
				return
			}
			a.addIssue(CodeUnsupportedGlobal, fmt.Sprintf("unsupported workflow global %q", root), call)
		}
	default:
		name, ok := callCalleeName(call.X)
		if !ok {
			return
		}
		if msg, forbidden := forbiddenRootGlobals[name]; forbidden {
			a.addIssue(CodeForbiddenHostAccess, msg, call)
			return
		}
		if _, supported := supportedRootGlobals[name]; !supported {
			a.addIssue(CodeUnsupportedGlobal, fmt.Sprintf("unsupported workflow global %q", name), call)
			return
		}
		a.validateSupportedPrimitiveShape(call, name)
	}
}

func (a *sourceAnalyzer) inspectDot(dot *js.DotExpr) {
	root, member, ok := memberAccess(dot)
	if !ok {
		return
	}
	if rootVar, isVar := dot.X.(*js.Var); isVar && isDeclaredLocalBinding(rootVar) {
		return
	}
	if msg, forbidden := forbiddenRootGlobals[root]; forbidden {
		a.addIssue(CodeForbiddenHostAccess, msg+" via "+root+"."+member, dot)
		return
	}
	switch root {
	case "workflow", "agent":
		return
	default:
		if _, supported := supportedRootGlobals[root]; supported {
			return
		}
		a.addIssue(CodeForbiddenHostAccess, fmt.Sprintf("unsupported host member access %s.%s", root, member), dot)
	}
}

func isDeclaredLocalBinding(v *js.Var) bool {
	for v != nil {
		if v.Decl != js.NoDecl {
			return true
		}
		v = v.Link
	}
	return false
}

func (a *sourceAnalyzer) inspectIdentifierUse(v *js.Var) {
	if isDeclaredLocalBinding(v) {
		return
	}
	name := string(v.Name())
	if _, seen := a.seen["id:"+name]; seen {
		return
	}
	if msg, forbidden := forbiddenRootGlobals[name]; forbidden {
		a.addIssue(CodeForbiddenHostAccess, msg, v)
		a.seen["id:"+name] = struct{}{}
	}
}

func callCalleeName(expr js.IExpr) (string, bool) {
	switch callee := expr.(type) {
	case *js.Var:
		if callee.Decl != js.NoDecl {
			return "", false
		}
		return string(callee.Name()), true
	case js.LiteralExpr:
		if len(callee.Data) == 0 {
			return "", false
		}
		return string(callee.Data), true
	case *js.LiteralExpr:
		if callee == nil || len(callee.Data) == 0 {
			return "", false
		}
		return string(callee.Data), true
	default:
		return "", false
	}
}

func memberAccess(dot *js.DotExpr) (root string, member string, ok bool) {
	rootVar, isRoot := dot.X.(*js.Var)
	if !isRoot {
		return "", "", false
	}
	member, ok = identifierName(dot.Y)
	if !ok {
		return "", "", false
	}
	return string(rootVar.Name()), member, true
}

func identifierName(expr js.IExpr) (string, bool) {
	switch node := expr.(type) {
	case *js.Var:
		return string(node.Name()), true
	case js.LiteralExpr:
		if node.TokenType != js.IdentifierToken {
			return "", false
		}
		return string(node.Data), true
	case *js.LiteralExpr:
		if node.TokenType != js.IdentifierToken {
			return "", false
		}
		return string(node.Data), true
	default:
		return "", false
	}
}

func (a *sourceAnalyzer) addIssue(code, message string, node js.INode) {
	line, column := sourceLocation(a.source, nodeString(node))
	key := code + "|" + message + "|" + fmt.Sprintf("%d:%d", line, column)
	if _, exists := a.seen[key]; exists {
		return
	}
	a.seen[key] = struct{}{}
	a.issues = append(a.issues, Issue{
		Code:    code,
		Message: message,
		Line:    line,
		Column:  column,
		Path:    sourcePath(a.ref, a.path),
	})
}

func nodeString(node js.INode) string {
	if node == nil {
		return ""
	}
	switch n := node.(type) {
	case *js.Var:
		return string(n.Name())
	case *js.CallExpr:
		return n.String()
	case *js.DotExpr:
		return n.String()
	case *js.ImportStmt:
		return "import"
	default:
		return fmt.Sprint(node)
	}
}

func sourceLocation(source, needle string) (int, int) {
	needle = strings.TrimSpace(needle)
	if needle == "" || source == "" {
		return 0, 0
	}
	index := strings.Index(source, needle)
	if index < 0 {
		short := strings.TrimPrefix(strings.TrimPrefix(needle, "("), " ")
		index = strings.Index(source, short)
		if index < 0 {
			return 0, 0
		}
	}
	prefix := source[:index]
	line := strings.Count(prefix, "\n") + 1
	lastNewline := strings.LastIndex(prefix, "\n")
	column := index - lastNewline
	if lastNewline >= 0 {
		column = index - lastNewline
	} else {
		column = index + 1
	}
	return line, column
}

func sourcePath(sourceRef, configPath string) string {
	ref := strings.TrimSpace(sourceRef)
	if ref != "" {
		return ref
	}
	if strings.TrimSpace(configPath) != "" {
		return configPath
	}
	return "orchestrator.javascript"
}
