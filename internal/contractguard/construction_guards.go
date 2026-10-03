package contractguard

import (
	"go/ast"
	"go/token"
)

// Guard provenance is deliberately bounded to declared required parameters,
// local aliases, and keyed storage on the registered result. It does not infer
// requiredness from pointer/interface types or dependency-looking names.
func (index constructionIndex) scanRequiredConstructionGuards(registry ConstructionRegistry) []ConstructionFinding {
	var findings []ConstructionFinding
	for _, constructor := range registry.Constructors {
		decl := index.declarations[constructor.Symbol]
		required := constructionRequiredObjects(decl.function, constructor.RequiredParameters)
		fields := constructionRequiredFields(decl, constructor, required)
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
				origins := map[*ast.Object]bool{}
				if function == decl.function {
					origins = required
				}
				stored := fields[ConstructionSymbol{ImportPath: caller.ImportPath, Name: caller.Receiver}]
				provenance := constructionGuardProvenance{required: origins, receiver: receiver, fields: stored,
					mutations: constructionGuardMutations(function.Body), fieldMutations: constructionGuardFieldMutations(function.Body, receiver)}
				ast.Inspect(function.Body, func(node ast.Node) bool {
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

func constructionRequiredObjects(function *ast.FuncDecl, parameters []ConstructionParameter) map[*ast.Object]bool {
	required := make(map[*ast.Object]bool)
	position := 0
	for _, field := range function.Type.Params.List {
		for offset := range max(1, len(field.Names)) {
			for _, parameter := range parameters {
				if parameter.Index == position && offset < len(field.Names) {
					required[field.Names[offset].Obj] = true
				}
			}
			position++
		}
	}
	return required
}

func constructionRequiredFields(decl constructionDeclaration, constructor ConstructionConstructor, required map[*ast.Object]bool) map[ConstructionSymbol]map[string]string {
	fields := make(map[ConstructionSymbol]map[string]string)
	if decl.function.Body == nil {
		return fields
	}
	provenance := constructionGuardProvenance{required: required, mutations: constructionGuardMutations(decl.function.Body)}
	ast.Inspect(decl.function.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		typ, resolved := constructionResultSymbol(literal.Type, decl.source)
		if !resolved {
			return true
		}
		for _, result := range constructor.Results {
			if typ != result {
				continue
			}
			for _, element := range literal.Elts {
				entry, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				name, ok := entry.Key.(*ast.Ident)
				rule := provenance.origin(entry.Value, map[*ast.Object]bool{})
				if !ok || rule == "" {
					continue
				}
				if fields[typ] == nil {
					fields[typ] = make(map[string]string)
				}
				fields[typ][name.Name] = rule
			}
		}
		return true
	})
	return fields
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
	required       map[*ast.Object]bool
	receiver       *ast.Object
	fields         map[string]string
	mutations      map[*ast.Object]bool
	fieldMutations map[string]bool
}

func (p constructionGuardProvenance) origin(expr ast.Expr, visited map[*ast.Object]bool) string {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return p.origin(value.X, visited)
	case *ast.SelectorExpr:
		ident, ok := value.X.(*ast.Ident)
		if ok && p.receiver != nil && ident.Obj == p.receiver {
			if p.fields[value.Sel.Name] != "" && p.fieldMutations[value.Sel.Name] {
				return "unresolved-required-dependency-guard"
			}
			return p.fields[value.Sel.Name]
		}
	case *ast.Ident:
		if value.Obj == nil || visited[value.Obj] {
			return ""
		}
		visited[value.Obj] = true
		rule := ""
		if p.required[value.Obj] {
			rule = "required-dependency-guard"
		} else {
			switch declaration := value.Obj.Decl.(type) {
			case *ast.AssignStmt:
				if len(declaration.Lhs) == 1 && len(declaration.Rhs) == 1 {
					rule = p.origin(declaration.Rhs[0], visited)
				}
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
				if ident, ok := selector.X.(*ast.Ident); ok && receiver != nil && ident.Obj == receiver {
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
