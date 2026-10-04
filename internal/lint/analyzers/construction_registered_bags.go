package analyzers

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Go preserves record fields in export data, but erases the declared ancestry
// of a defined type (type Derived Collaborator). Carry only that missing kind
// through dependency facts; explicit registry classifications remain authoritative.
type registeredConstructionKindFact struct {
	Kind ConstructionKind
}

func (*registeredConstructionKindFact) AFact() {}
func (f *registeredConstructionKindFact) String() string {
	return "construction-kind=" + string(f.Kind)
}

type registeredBagTypes struct {
	pass     *analysis.Pass
	registry ConstructionRegistry
	declared map[*types.TypeName]types.Type
}

func scanRegisteredConstructionBags(pass *analysis.Pass, registry ConstructionRegistry,
	add func(ConstructionSymbol, ConstructionSymbol, ConstructionConstructor, string, token.Pos),
) registeredBagTypes {
	typeset := registeredBagTypes{pass: pass, registry: registry, declared: map[*types.TypeName]types.Type{}}
	authored := typeset.collectDeclarations()
	for obj, rhs := range typeset.declared {
		if obj.Parent() != pass.Pkg.Scope() || obj.IsAlias() || typeset.explicitKind(obj) != "" {
			continue
		}
		if kind := typeset.kind(rhs, map[types.Type]bool{}); kind != "" {
			pass.ExportObjectFact(obj, &registeredConstructionKindFact{Kind: kind})
		}
	}
	for _, constructor := range registry.Constructors {
		if constructor.Symbol.ImportPath != pass.Pkg.Path() {
			continue
		}
		fn := registeredConstructorDeclaration(pass.Pkg, constructor.Symbol)
		if fn == nil || !authored[fn] {
			continue // Declaration/signature errors are owned by metadata validation.
		}
		sig := fn.Type().(*types.Signature)
		for _, required := range constructor.RequiredParameters {
			param := sig.Params().At(required.Index)
			if typeset.bag(param.Type(), map[types.Type]bool{}) {
				add(constructor.Symbol, constructor.Symbol, constructor, "required-dependency-bag", param.Pos())
			}
		}
	}
	return typeset
}

func (s registeredBagTypes) collectDeclarations() map[*types.Func]bool {
	authored := map[*types.Func]bool{}
	for _, file := range s.pass.Files {
		if ast.IsGenerated(file) || strings.HasSuffix(s.pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if obj, ok := s.pass.TypesInfo.Defs[fn.Name].(*types.Func); ok {
					authored[obj] = true
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if spec, ok := node.(*ast.TypeSpec); ok {
				if obj, ok := s.pass.TypesInfo.Defs[spec.Name].(*types.TypeName); ok {
					s.declared[obj] = s.pass.TypesInfo.TypeOf(spec.Type)
				}
			}
			return true
		})
	}
	return authored
}

func (s registeredBagTypes) explicitKind(obj *types.TypeName) ConstructionKind {
	symbol := registeredConstructionSymbol(obj)
	for _, classified := range s.registry.Types {
		if classified.Symbol == symbol {
			return classified.Kind
		}
	}
	return ""
}

func registeredTypeName(typ types.Type) *types.TypeName {
	switch typ := typ.(type) {
	case *types.Named:
		return typ.Origin().Obj()
	case *types.Alias:
		return typ.Origin().Obj()
	}
	return nil
}

// Kind follows named declaration ancestry only. Containers and unclassified
// records are inspected by bag/collaborator instead of inheriting a field's kind.
func (s registeredBagTypes) kind(typ types.Type, visited map[types.Type]bool) ConstructionKind {
	obj := registeredTypeName(typ)
	if obj == nil || visited[typ] {
		return ""
	}
	if kind := s.explicitKind(obj); kind != "" {
		return kind
	}
	visited[typ] = true
	defer delete(visited, typ)
	if rhs := s.declared[obj]; rhs != nil {
		return s.kind(rhs, visited)
	}
	if alias, ok := typ.(*types.Alias); ok {
		return s.kind(alias.Rhs(), visited)
	}
	var fact registeredConstructionKindFact
	if obj.Pkg() != s.pass.Pkg && s.pass.ImportObjectFact(obj, &fact) {
		return fact.Kind
	}
	return ""
}

func (s registeredBagTypes) bag(typ types.Type, visited map[types.Type]bool) bool {
	if typ == nil || visited[typ] || s.kind(typ, map[types.Type]bool{}) != "" {
		return false // A classified collaborator or domain/state/resource is never a bag.
	}
	visited[typ] = true
	defer delete(visited, typ)
	switch typ := typ.(type) {
	case *types.Alias:
		return s.bag(typ.Rhs(), visited)
	case *types.Named:
		return s.bag(typ.Underlying(), visited)
	case *types.Pointer:
		return s.bag(typ.Elem(), visited)
	case *types.Slice:
		return s.bag(typ.Elem(), visited)
	case *types.Array:
		return s.bag(typ.Elem(), visited)
	case *types.Chan:
		return s.bag(typ.Elem(), visited)
	case *types.Map:
		return s.bag(typ.Key(), visited) || s.bag(typ.Elem(), visited)
	case *types.Struct:
		return s.structBag(typ, visited)
	}
	return false
}

func (s registeredBagTypes) collaborator(typ types.Type, visited map[types.Type]bool) bool {
	if typ == nil || visited[typ] {
		return false
	}
	if kind := s.kind(typ, map[types.Type]bool{}); kind != "" {
		return kind == ConstructionBehavior || kind == ConstructionEffect
	}
	visited[typ] = true
	defer delete(visited, typ)
	switch typ := typ.(type) {
	case *types.Alias:
		return s.collaborator(typ.Rhs(), visited)
	case *types.Named:
		return s.collaborator(typ.Underlying(), visited)
	case *types.Pointer:
		return s.collaborator(typ.Elem(), visited)
	case *types.Slice:
		return s.collaborator(typ.Elem(), visited)
	case *types.Array:
		return s.collaborator(typ.Elem(), visited)
	case *types.Chan:
		return s.collaborator(typ.Elem(), visited)
	case *types.Map:
		return s.collaborator(typ.Key(), visited) || s.collaborator(typ.Elem(), visited)
	}
	return false
}

func (s registeredBagTypes) structBag(typ *types.Struct, visited map[types.Type]bool) bool {
	for i := 0; i < typ.NumFields(); i++ {
		field := typ.Field(i).Type()
		if s.collaborator(field, map[types.Type]bool{}) || s.bag(field, visited) {
			return true
		}
	}
	return false
}
