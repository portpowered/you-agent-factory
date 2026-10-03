package contractguard

import (
	"go/ast"
	"go/token"
)

// Guard provenance is deliberately bounded to declared required parameters,
// local aliases, and storage on the registered result. It does not infer
// requiredness from pointer/interface types or dependency-looking names.
func (index constructionIndex) scanRequiredConstructionGuards(registry ConstructionRegistry) []ConstructionFinding {
	var findings []ConstructionFinding
	for _, constructor := range registry.Constructors {
		decl := index.declarations[constructor.Symbol]
		required := constructionRequiredObjects(decl.function, constructor.RequiredParameters)
		fields := index.constructionRequiredFields(decl, constructor, required)
		var requiredReceivers []ConstructionSymbol
		if constructionProhibitedKind(constructor, registry.Types) {
			requiredReceivers = constructor.Results
		}
		helperRequired := index.constructionHelperRequired(decl, required, fields, requiredReceivers)
		for _, source := range index.sources {
			for _, declaration := range source.file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil {
					continue
				}
				caller := ConstructionSymbol{ImportPath: source.importPath, Name: function.Name.Name}
				var receiver *ast.Object
				if function.Recv != nil {
					field := function.Recv.List[0]
					caller.Receiver = constructionReceiver(field.Type)
					if len(field.Names) == 1 {
						receiver = field.Names[0].Obj
					}
				}
				origins := helperRequired[function]
				stored := fields[ConstructionSymbol{ImportPath: caller.ImportPath, Name: caller.Receiver}]
				provenance := constructionGuardProvenance{source: source, required: origins, receiver: receiver, fields: stored,
					mutations: constructionGuardMutations(function.Body), fieldMutations: constructionGuardFieldMutations(function.Body, receiver)}
				provenance.requiredReceiver = constructionProhibitedKind(constructor, registry.Types) &&
					constructionHasResult(constructor, ConstructionSymbol{ImportPath: caller.ImportPath, Name: caller.Receiver})
				ast.Inspect(function.Body, func(node ast.Node) bool {
					var condition ast.Expr
					switch statement := node.(type) {
					case *ast.IfStmt:
						condition = statement.Cond
					case *ast.ForStmt:
						condition = statement.Cond
					}
					findings = append(findings, constructionAssertionGuards(condition, provenance, caller, constructor, registry)...)
					comparison, ok := node.(*ast.BinaryExpr)
					if !ok || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
						return true
					}
					dependency := constructionNilOperand(comparison)
					rule := provenance.origin(dependency, map[*ast.Object]bool{})
					if rule == "" {
						return true
					}
					mode := ConstructionReport
					for _, set := range registry.CapabilitySets {
						if set.Name == constructor.CapabilitySet {
							mode = set.Mode
						}
					}
					findings = append(findings, ConstructionFinding{FilePath: source.path,
						Line: source.set.Position(comparison.Pos()).Line, Caller: caller,
						Callee: constructor.Symbol, CapabilitySet: constructor.CapabilitySet,
						Mode: mode, Rule: rule})
					return true
				})
			}
		}
	}
	return findings
}

func constructionRequiredObjects(function *ast.FuncDecl, parameters []ConstructionParameter) map[*ast.Object]string {
	required := make(map[*ast.Object]string)
	position := 0
	for _, field := range function.Type.Params.List {
		for offset := range max(1, len(field.Names)) {
			for _, parameter := range parameters {
				if parameter.Index == position && offset < len(field.Names) {
					required[field.Names[offset].Obj] = "required-dependency-guard"
				}
			}
			position++
		}
	}
	return required
}

func constructionNilOperand(comparison *ast.BinaryExpr) ast.Expr {
	for _, pair := range [][2]ast.Expr{{comparison.X, comparison.Y}, {comparison.Y, comparison.X}} {
		if ident, ok := pair[0].(*ast.Ident); ok && ident.Name == "nil" && ident.Obj == nil {
			return pair[1]
		}
	}
	return nil
}

type constructionGuardProvenance struct {
	source           *constructionSource
	helpers          map[*ast.FuncDecl]bool
	required         map[*ast.Object]string
	receiver         *ast.Object
	requiredReceiver bool
	fields           map[string]string
	mutations        map[*ast.Object]bool
	fieldMutations   map[string]bool
}

func (p constructionGuardProvenance) origin(expr ast.Expr, visited map[*ast.Object]bool) string {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return p.origin(value.X, visited)
	case *ast.TypeAssertExpr:
		return p.origin(value.X, visited)
	case *ast.CallExpr:
		return p.helperOrigin(value)
	case *ast.SelectorExpr:
		if constructionReceiverAlias(value.X, p.receiver, map[*ast.Object]bool{}) {
			if p.fields[value.Sel.Name] != "" && (p.fieldMutations[value.Sel.Name] || constructionStorageMutated(value.X, p.mutations, map[*ast.Object]bool{})) {
				return "unresolved-required-dependency-guard"
			}
			return p.fields[value.Sel.Name]
		}
	case *ast.Ident:
		if value.Obj == nil || visited[value.Obj] {
			return ""
		}
		if p.requiredReceiver && value.Obj == p.receiver {
			if p.mutations[value.Obj] {
				return "unresolved-required-dependency-guard"
			}
			return "required-receiver-guard"
		}
		visited[value.Obj] = true
		rule := ""
		if p.required[value.Obj] != "" {
			rule = p.required[value.Obj]
		} else {
			switch declaration := value.Obj.Decl.(type) {
			case *ast.AssignStmt:
				rule = p.origin(constructionAssignedValue(value.Obj, declaration.Lhs, declaration.Rhs), visited)
			case *ast.ValueSpec:
				if len(declaration.Names) == 1 && len(declaration.Values) == 1 {
					rule = p.origin(declaration.Values[0], visited)
				}
			}
		}
		if rule != "" && p.mutations[value.Obj] {
			return "unresolved-required-dependency-guard"
		}
		return rule
	}
	return ""
}

func constructionGuardFieldMutations(body ast.Node, receiver *ast.Object) map[string]bool {
	mutations := make(map[string]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, left := range assignment.Lhs {
			if selector, ok := left.(*ast.SelectorExpr); ok {
				if constructionReceiverAlias(selector.X, receiver, map[*ast.Object]bool{}) {
					mutations[selector.Sel.Name] = true
				}
			}
		}
		return true
	})
	return mutations
}

// Reassignment prevents a syntactic alias from proving the current value's
// origin. Report coverage debt instead of pretending to have flow analysis.
func constructionGuardMutations(body ast.Node) map[*ast.Object]bool {
	mutations := make(map[*ast.Object]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, left := range assignment.Lhs {
			if ident, ok := left.(*ast.Ident); ok && ident.Obj != nil && ident.Obj.Decl != assignment {
				mutations[ident.Obj] = true
			}
		}
		return true
	})
	return mutations
}
