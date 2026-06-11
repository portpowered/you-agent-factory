package workflowvalidation

import (
	"fmt"

	"github.com/tdewolff/parse/v2/js"
)

func (a *sourceAnalyzer) validateSupportedPrimitiveShape(call *js.CallExpr, primitive string) {
	switch primitive {
	case "meta":
		a.validateMetaCall(call)
	case "phase", "log":
		a.validateSingleStringArgCall(call, primitive)
	case "workflow.log":
		a.validateSingleStringArgCall(call, "workflow.log")
	case "workflow.artifact":
		a.validateWorkflowArtifactCall(call)
	case "workflow.final":
		a.validateArity(call, "workflow.final", 1)
	case "agent.run":
		a.validateAgentRunCall(call)
	case "parallel", "pipeline":
		a.validateSingleArrayArgCall(call, primitive)
	}
}

func (a *sourceAnalyzer) validateMetaCall(call *js.CallExpr) {
	obj, ok := a.requireSingleObjectArg(call, "meta")
	if !ok || obj == nil {
		return
	}
	if !a.requireObjectStringProperty(call, obj, "meta", "name") {
		return
	}
	if value, found := objectProperty(obj, "version"); !found || !isNumberLiteral(value) {
		a.addIssue(CodeInvalidMetadata, `meta() requires an object argument with a numeric "version" property`, call)
	}
}

func (a *sourceAnalyzer) validateWorkflowArtifactCall(call *js.CallExpr) {
	obj, ok := a.requireSingleObjectArg(call, "workflow.artifact")
	if !ok || obj == nil {
		return
	}
	if !a.requireObjectStringProperty(call, obj, "workflow.artifact", "kind") {
		return
	}
	a.requireObjectStringProperty(call, obj, "workflow.artifact", "label")
}

func (a *sourceAnalyzer) validateAgentRunCall(call *js.CallExpr) {
	obj, ok := a.requireSingleObjectArg(call, "agent.run")
	if !ok || obj == nil {
		return
	}
	a.requireObjectStringProperty(call, obj, "agent.run", "prompt")
}

func (a *sourceAnalyzer) validateSingleStringArgCall(call *js.CallExpr, primitive string) {
	if !a.validateArity(call, primitive, 1) {
		return
	}
	arg, ok := firstCallArg(call)
	if !ok || !isLiteralExpr(arg) {
		return
	}
	if !isStringLiteral(arg) {
		a.addIssue(CodeUnsupportedPrimitive, fmt.Sprintf("%s() requires a string argument", primitive), call)
	}
}

func (a *sourceAnalyzer) validateSingleArrayArgCall(call *js.CallExpr, primitive string) {
	if !a.validateArity(call, primitive, 1) {
		return
	}
	arg, ok := firstCallArg(call)
	if !ok || !isLiteralExpr(arg) {
		return
	}
	if _, ok := isArrayLiteral(arg); !ok {
		a.addIssue(CodeUnsupportedPrimitive, fmt.Sprintf("%s() requires an array argument", primitive), call)
	}
}

func (a *sourceAnalyzer) validateArity(call *js.CallExpr, primitive string, want int) bool {
	if callArity(call) != want {
		a.addIssue(shapeIssueCode(primitive), fmt.Sprintf("%s() requires exactly %d argument(s)", primitive, want), call)
		return false
	}
	return true
}

func (a *sourceAnalyzer) requireSingleObjectArg(call *js.CallExpr, primitive string) (*js.ObjectExpr, bool) {
	if !a.validateArity(call, primitive, 1) {
		return nil, false
	}
	arg, ok := firstCallArg(call)
	if !ok {
		a.addIssue(shapeIssueCode(primitive), fmt.Sprintf("%s() requires an object argument", primitive), call)
		return nil, false
	}
	if !isLiteralExpr(arg) {
		return nil, true
	}
	obj, ok := isObjectLiteral(arg)
	if !ok {
		a.addIssue(shapeIssueCode(primitive), fmt.Sprintf("%s() requires an object argument", primitive), call)
		return nil, false
	}
	return obj, true
}

func (a *sourceAnalyzer) requireObjectStringProperty(call *js.CallExpr, obj *js.ObjectExpr, primitive, property string) bool {
	value, found := objectProperty(obj, property)
	if !found {
		a.addIssue(shapeIssueCode(primitive), fmt.Sprintf(`%s() requires an object argument with a string %q property`, primitive, property), call)
		return false
	}
	if !isStringLiteral(value) {
		a.addIssue(shapeIssueCode(primitive), fmt.Sprintf(`%s() requires %q to be a string literal`, primitive, property), call)
		return false
	}
	return true
}

func shapeIssueCode(primitive string) string {
	if primitive == "meta" {
		return CodeInvalidMetadata
	}
	return CodeUnsupportedPrimitive
}

func callArity(call *js.CallExpr) int {
	return len(call.Args.List)
}

func firstCallArg(call *js.CallExpr) (js.IExpr, bool) {
	if len(call.Args.List) == 0 {
		return nil, false
	}
	return call.Args.List[0].Value, true
}

func isLiteralExpr(expr js.IExpr) bool {
	switch expr.(type) {
	case js.LiteralExpr, *js.LiteralExpr, *js.ObjectExpr, *js.ArrayExpr:
		return true
	default:
		return false
	}
}

func isStringLiteral(expr js.IExpr) bool {
	switch node := expr.(type) {
	case js.LiteralExpr:
		return node.TokenType == js.StringToken
	case *js.LiteralExpr:
		return node.TokenType == js.StringToken
	default:
		return false
	}
}

func isNumberLiteral(expr js.IExpr) bool {
	switch node := expr.(type) {
	case js.LiteralExpr:
		return node.TokenType == js.IntegerToken || node.TokenType == js.DecimalToken
	case *js.LiteralExpr:
		return node.TokenType == js.IntegerToken || node.TokenType == js.DecimalToken
	default:
		return false
	}
}

func isObjectLiteral(expr js.IExpr) (*js.ObjectExpr, bool) {
	obj, ok := expr.(*js.ObjectExpr)
	return obj, ok
}

func isArrayLiteral(expr js.IExpr) (*js.ArrayExpr, bool) {
	arr, ok := expr.(*js.ArrayExpr)
	return arr, ok
}

func objectProperty(obj *js.ObjectExpr, name string) (js.IExpr, bool) {
	for _, prop := range obj.List {
		if prop.Spread || prop.Name == nil {
			continue
		}
		if prop.Name.IsIdent([]byte(name)) {
			return prop.Value, true
		}
	}
	return nil, false
}
