package analyzers

import (
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Petripublic keeps live engine types behind the Factory Runtime boundary.
var Petripublic = &analysis.Analyzer{
	Name: "petripublic",
	Doc:  "reject exported surfaces containing internal Petri engine types",
	Run:  runPetriPublic,
}

const petriPackage = "pkg/services/factory_runtime/internal/orchestrators/petri"

func runPetriPublic(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok || under(unit, "pkg/services/factory_runtime/internal") {
		return nil, nil
	}
	var found []violation
	scope := pass.Pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		walk := petriTypeWalk{seen: map[types.Type]bool{}, record: func(leak string) {
			found = append(found, violation{
				rule: "petri-public", importer: unit, importee: name + "|" + leak,
				pos: obj.Pos(), hint: "keep live engine types inside Factory Runtime internals",
			})
		}}
		walk.visit(obj.Type())
	}
	reportAgainstBaseline(pass, unit, map[string]bool{"petri-public": true}, found, false)
	return nil, nil
}

// Each exported root owns its visited set: cycles terminate without hiding a
// second root's leak. The baseline reporter deduplicates repeated leaked types.
type petriTypeWalk struct {
	seen   map[types.Type]bool
	record func(string)
}

func (w *petriTypeWalk) visit(t types.Type) {
	if t == nil || w.seen[t] {
		return
	}
	w.seen[t] = true
	switch t := t.(type) {
	case *types.Alias:
		w.identity(t.Obj())
		w.parameters(t.TypeParams(), t.TypeArgs())
		w.visit(types.Unalias(t))
	case *types.Named:
		w.identity(t.Obj())
		w.parameters(t.TypeParams(), t.TypeArgs())
		w.visit(t.Underlying())
		for i := 0; i < t.NumMethods(); i++ {
			if method := t.Method(i); method.Exported() {
				w.visit(method.Type())
			}
		}
	case *types.Pointer:
		w.visit(t.Elem())
	case *types.Slice:
		w.visit(t.Elem())
	case *types.Array:
		w.visit(t.Elem())
	case *types.Map:
		w.visit(t.Key())
		w.visit(t.Elem())
	case *types.Chan:
		w.visit(t.Elem())
	case *types.Signature:
		w.parameters(t.TypeParams(), nil)
		w.parameters(t.RecvTypeParams(), nil)
		w.visit(t.Params())
		w.visit(t.Results())
	case *types.Tuple:
		for i := 0; i < t.Len(); i++ {
			w.visit(t.At(i).Type())
		}
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			w.visit(t.Field(i).Type())
		}
	case *types.Interface:
		for i := 0; i < t.NumExplicitMethods(); i++ {
			w.visit(t.ExplicitMethod(i).Type())
		}
		for i := 0; i < t.NumEmbeddeds(); i++ {
			w.visit(t.EmbeddedType(i))
		}
	case *types.TypeParam:
		w.visit(t.Constraint())
	case *types.Union:
		for i := 0; i < t.Len(); i++ {
			w.visit(t.Term(i).Type())
		}
	}
}

func (w *petriTypeWalk) identity(obj *types.TypeName) {
	if obj.Pkg() != nil && obj.Pkg().Path() == modulePrefix+petriPackage {
		w.record(petriPackage + "." + obj.Name())
	}
}

func (w *petriTypeWalk) parameters(params *types.TypeParamList, args *types.TypeList) {
	for i := 0; i < params.Len(); i++ {
		w.visit(params.At(i))
	}
	for i := 0; i < args.Len(); i++ {
		w.visit(args.At(i))
	}
}
