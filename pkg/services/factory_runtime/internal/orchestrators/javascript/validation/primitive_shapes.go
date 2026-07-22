package workflowvalidation

import (
	"fmt"
	"strconv"

	orchestratorcontract "github.com/portpowered/infinite-you/pkg/services/factory_runtime/orchestratorcontract"
	"github.com/tdewolff/parse/v2/js"
)

func (a *sourceAnalyzer) validateSupportedPrimitiveShape(call *js.CallExpr, primitive string) {
	switch primitive {
	case "meta":
		a.validateMetaCall(call)
	case "phase":
		a.validateSingleStringArgCall(call, primitive)
	case "log", "workflow.log":
		a.validateLogCall(call, primitive)
	case "workflow.artifact":
		a.validateWorkflowArtifactCall(call)
	case "workflow.checkpoint":
		a.validateWorkflowCheckpointCall(call)
	case "workflow.resumeState":
		a.validateWorkflowResumeStateCall(call)
	case "workflow.budget":
		a.validateWorkflowBudgetCall(call)
	case "workflow.final":
		a.validateArity(call, "workflow.final", 1)
	case "agent.run":
		a.validateAgentRunCall(call)
	case "parallel":
		a.validateSingleArrayArgCall(call, primitive)
	case "pipeline":
		a.validatePipelineCall(call)
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

func (a *sourceAnalyzer) validateLogCall(call *js.CallExpr, primitive string) {
	arity := callArity(call)
	if arity < 1 || arity > 2 {
		a.addIssue(shapeIssueCode(primitive), fmt.Sprintf("%s() requires 1 or 2 argument(s)", primitive), call)
		return
	}
	arg, ok := firstCallArg(call)
	if !ok || !isLiteralExpr(arg) {
		return
	}
	if !isStringLiteral(arg) {
		a.addIssue(CodeUnsupportedPrimitive, fmt.Sprintf("%s() requires a string message argument", primitive), call)
		return
	}
	if arity == 1 {
		return
	}
	second := call.Args.List[1].Value
	if !isLiteralExpr(second) {
		return
	}
	if _, ok := isObjectLiteral(second); !ok {
		a.addIssue(shapeIssueCode(primitive), fmt.Sprintf("%s() optional fields argument must be an object literal", primitive), call)
	}
}

func (a *sourceAnalyzer) validateWorkflowResumeStateCall(call *js.CallExpr) {
	a.validateArity(call, "workflow.resumeState", 0)
}

func (a *sourceAnalyzer) validateWorkflowCheckpointCall(call *js.CallExpr) {
	obj, ok := a.requireSingleObjectArg(call, "workflow.checkpoint")
	if !ok || obj == nil {
		return
	}
	a.requireObjectStringProperty(call, obj, "workflow.checkpoint", "label")
}

func (a *sourceAnalyzer) validateWorkflowBudgetCall(call *js.CallExpr) {
	a.validateArity(call, "workflow.budget", 0)
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
	for _, property := range obj.List {
		field, visible := staticPropertyName(property)
		if visible && !orchestratorcontract.IsJavaScriptChildSupportedField(field) {
			a.addIssue(
				shapeIssueCode("agent.run"),
				fmt.Sprintf(`agent.run() does not support field %q`, field),
				call,
			)
		}
	}
	for _, field := range orchestratorcontract.JavaScriptChildSupportedFields() {
		value, found := objectProperty(obj, field)
		if field == orchestratorcontract.FieldPrompt && !found {
			a.addIssue(shapeIssueCode("agent.run"), `agent.run() requires an object argument with a string "prompt" property`, call)
			continue
		}
		if found && !isAgentRunStringValue(value) {
			a.addIssue(shapeIssueCode("agent.run"), fmt.Sprintf(`agent.run() requires %q to be a string value`, field), call)
		}
	}
}

// Agent request fields may be computed from invocation arguments and completed
// child results. Runtime normalizes and policy-validates their resolved string
// values before dispatch; literal non-strings remain a static validation error.
func isAgentRunStringValue(value js.IExpr) bool {
	if isLiteralExpr(value) {
		return isStringLiteral(value)
	}
	return true
}

func staticPropertyName(property js.Property) (string, bool) {
	if property.Spread || property.Name == nil || property.Name.IsComputed() {
		return "", false
	}
	literal := property.Name.Literal
	switch literal.TokenType {
	case js.IdentifierToken:
		return string(literal.Data), true
	case js.StringToken:
		name, err := strconv.Unquote(string(literal.Data))
		return name, err == nil
	default:
		return "", false
	}
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

func (a *sourceAnalyzer) validatePipelineCall(call *js.CallExpr) {
	arity := callArity(call)
	if arity < 2 || arity > 3 {
		a.addIssue(shapeIssueCode("pipeline"), "pipeline() requires 2 or 3 argument(s)", call)
		return
	}
	arg, ok := firstCallArg(call)
	if !ok || !isLiteralExpr(arg) {
		return
	}
	if _, ok := isArrayLiteral(arg); !ok {
		a.addIssue(CodeUnsupportedPrimitive, "pipeline() requires an array items argument", call)
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
