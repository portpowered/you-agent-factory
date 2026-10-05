package analyzers

import (
	"go/ast"
	"go/types"
	"strings"

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
	petriReferenceFindings(pass, unit)
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

// References to retired root contracts are prohibited even inside private
// declarations. Resolve the referenced object, not its spelling: aliases and
// dot imports cannot hide a reference, and local shadows are unrelated.
func petriReferenceFindings(pass *analysis.Pass, unit string) {
	var found []violation
	selected := map[string]bool{}
	hasTests := false
	for _, file := range pass.Files {
		path := serviceSource(pass, unit, file)
		selected[path] = true
		hasTests = hasTests || strings.HasSuffix(path, "_test.go")
		if ast.IsGenerated(file) || strings.Contains("/"+path, "/testdata/") {
			continue
		}
		found = append(found, inspectPetriReferences(pass, file, unit, path)...)
	}
	reportWithBaseline(pass, unit, setOf("petri-reference"), countedTestPolicy(found), hasTests, petriReferenceBaseline(pass, unit, selected, hasTests))
}

func inspectPetriReferences(pass *analysis.Pass, file *ast.File, unit, path string) []violation {
	var found []violation
	ast.Inspect(file, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pass.TypesInfo.Uses[id]
		if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
			return true
		}
		owner := strings.TrimPrefix(obj.Pkg().Path(), modulePrefix)
		if obj.Pkg().Path() == modulePrefix+owner && petriReferenceOwners[owner] && petriReferenceSymbols[obj.Name()] {
			found = append(found, violation{rule: "petri-reference", importer: unit,
				importee: path + "#" + owner + "." + obj.Name(), pos: id.Pos(),
				hint: "keep live engine references inside Factory Runtime internals; authored PETRI configuration remains allowed"})
		}
		return true
	})
	return found
}

func petriReferenceBaseline(pass *analysis.Pass, unit string, selected map[string]bool, hasTests bool) map[string]struct{} {
	ignored := map[string]bool{}
	for _, name := range pass.IgnoredFiles {
		ignored[sourceName(unit, name)] = true
	}
	listed := baseline()
	for key := range listed {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) != 3 || parts[0] != "petri-reference" || parts[1] != unit {
			continue
		}
		path, _, _ := strings.Cut(parts[2], "#")
		if !selected[path] && (ignored[path] || (!hasTests && strings.HasSuffix(path, "_test.go"))) {
			delete(listed, key)
		}
	}
	return listed
}

var petriReferenceOwners = setOf("pkg/services/factory_runtime", "pkg/services/factory_definitions", "pkg/services/factory_definitions/internal/contracts")

var petriReferenceSymbols = setOf("Net", "RuntimeNet", "PetriMarking", "PetriMarkingSnapshot", "RuntimeToken", "RuntimeTokenColor", "PetriTransition", "EnabledTransition", "EngineStateSnapshot", "StateSnapshot", "NewEngineStateSnapshot")

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
