package analyzers

import (
	"go/ast"
	"go/parser"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Layering enforces the package layering table below as from->to import-edge
// predicates against an exact baseline. It replaces the import rules of the
// retired pkgboundarycheck and ownershipboundarycheck. Rules that the Go
// compiler already enforces (internal/ visibility) are skipped, not listed.
var Layering = &analysis.Analyzer{
	Name: "layering",
	Doc:  "enforce pkg/ layering as import-edge predicates with an exact baseline",
	Run:  runLayering,
}

// edge is one import of importee by importer, observed in one source file.
type edge struct {
	importer string // module-relative package path, _test suffix removed
	importee string // module-relative import path
	test     bool
}

type layeringRule struct {
	name     string
	hint     string
	violates func(e edge) bool
}

// serviceRootPorts is the one allow-list of the service-subpackage rule: the
// permanent, documented cross-service contract imports (external-effect ports
// aggregated by pkg/services/edges and the wire-time provider hand-off). Each
// port lists the importer roots that may use it; tests may always use them.
var serviceRootPorts = map[string][]string{
	"pkg/services/providers/wire": {
		"pkg/services/edges", "pkg/services/factory_runtime", "pkg/services/factory_sessions", "pkg/services/recordings",
	},
	"pkg/services/models/wire": {"pkg/services/edges"},
}

// supportRoots are reusable production support packages held to the same
// service-subpackage rule as tests.
var supportRoots = []string{"internal/configcontractsmoke", "internal/testutil", "tests/functional/internal/support"}

var layeringRules = []layeringRule{
	{
		name:     "functional-provider-boundary",
		hint:     "use tests/functional/internal/support.BuildProcess and exact public external-effect ports",
		violates: violatesFunctionalProvider,
	},
	{
		name: "constructed-service-edges",
		hint: "inject exact external-effect ports from pkg/wire instead of the broad Edges bag",
		violates: func(e edge) bool {
			return !e.test && under(e.importer, "pkg/services") && !under(e.importer, "pkg/services/edges") && e.importee == "pkg/services/edges"
		},
	},
	{
		name: "application-graph",
		hint: "only pkg/root and pkg/wire may import pkg/wire; tests build the application through root.BuildProcess",
		violates: func(e edge) bool {
			return under(e.importee, "pkg/wire") && !under(e.importer, "pkg/root") && !under(e.importer, "pkg/wire") &&
				(e.test || under(e.importer, "pkg"))
		},
	},
	{
		name: "domain-transport",
		hint: "services must not import pkg/transports; map at the transport edge (service-owned transports/ packages are exempt)",
		violates: func(e edge) bool {
			return under(e.importer, "pkg/services") && strings.HasPrefix(e.importee, "pkg/transports/") && !serviceOwnedTransport(e.importer)
		},
	},
	{
		name:     "service-subpackage",
		hint:     "import the peer service root contract (pkg/services/<peer>), not one of its subpackages",
		violates: violatesServiceSubpackage,
	},
	{
		name: "initializer-transport",
		hint: "inject an already-constructed transport operation instead of importing pkg/transports from the initializer",
		violates: func(e edge) bool {
			return under(e.importer, "pkg/initializer") && strings.HasPrefix(e.importee, "pkg/transports/")
		},
	},
	{
		name: "initializer-service",
		hint: "the initializer may import only service roots; inject an already-constructed service-root operation",
		violates: func(e edge) bool {
			_, rest, ok := serviceSplit(e.importee)
			return under(e.importer, "pkg/initializer") && ok && rest != ""
		},
	},
	{
		name: "platform-services",
		hint: "platform implements exact external-effect ports and must not depend on services; move domain policy to the owning service",
		violates: func(e edge) bool {
			return under(e.importer, "pkg/platform") && strings.HasPrefix(e.importee, "pkg/services/")
		},
	},
}

// serviceSplit splits pkg/services/<owner>[/rest] into owner and rest.
func serviceSplit(path string) (owner, rest string, ok bool) {
	tail, found := strings.CutPrefix(path, "pkg/services/")
	if !found || tail == "" {
		return "", "", false
	}
	owner, rest, _ = strings.Cut(tail, "/")
	return owner, rest, true
}

// serviceOwnedTransport reports whether path is pkg/services/<owner>/transports/<protocol>[/...].
func serviceOwnedTransport(path string) bool {
	_, rest, ok := serviceSplit(path)
	if !ok {
		return false
	}
	parts := strings.Split(rest, "/")
	return len(parts) >= 2 && parts[0] == "transports" && parts[1] != ""
}

func violatesServiceSubpackage(e edge) bool {
	wire := under(e.importee, "pkg/wire")
	owner, rest, isService := serviceSplit(e.importee)
	subpackage := isService && rest != ""
	if !subpackage && !wire {
		return false
	}
	if subpackage && containsSegment(rest, "internal") {
		return false // compiler-enforced
	}
	importerOwner, _, importerIsService := serviceSplit(e.importer)
	inSupport := false
	for _, root := range supportRoots {
		inSupport = inSupport || under(e.importer, root)
	}
	if wire && !inSupport {
		return false // the application-graph rule owns pkg/wire edges
	}
	if !importerIsService && !inSupport && !(e.test && !under(e.importer, "pkg/wire")) {
		return false
	}
	if importerIsService && owner == importerOwner {
		return false
	}
	if e.test && subpackage && serviceOwnedTransport(e.importee) {
		return false // a test may consume a service's public transport adapter
	}
	if e.test && subpackage && matchesProtocol(e.importer, e.importee) {
		return false
	}
	if permitted, ok := serviceRootPorts[e.importee]; ok {
		if e.test {
			return false
		}
		for _, root := range permitted {
			if under(e.importer, root) {
				return false
			}
		}
	}
	return true
}

// matchesProtocol reports whether importer is pkg/transports/<p>[/...] and
// importee is pkg/services/<owner>/transports/<p>[/...].
func matchesProtocol(importer, importee string) bool {
	_, rest, _ := serviceSplit(importee)
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] != "transports" {
		return false
	}
	return under(importer, "pkg/transports/"+parts[1])
}

func containsSegment(path, segment string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == segment {
			return true
		}
	}
	return false
}

var layeringRuleNames = func() map[string]bool {
	names := map[string]bool{}
	for _, rule := range layeringRules {
		names[rule.name] = true
		names[rule.name+"-test"] = true
	}
	return names
}()

func runLayering(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	importer := strings.TrimSuffix(unit, "_test")
	// Preserve the legacy eight-family predicate; this does not authorize
	// recreating the retired config/internal families in product code.
	if tail, ok := strings.CutPrefix(importer, "pkg/"); ok {
		family, _, _ := strings.Cut(tail, "/")
		if !slices.Contains([]string{"config", "initializer", "internal", "platform", "root", "services", "transports", "wire"}, family) {
			pass.Reportf(pass.Files[0].Package, "package-family: unapproved package family pkg/%s; use the owning service, platform or transport package", family)
		}
	}
	var found []violation
	if providerLocalSupport(importer) {
		pass.Reportf(pass.Files[0].Package, "functional-provider-support: keep reusable process support in tests/functional/internal/support")
	}
	hasTests := false
	visit := func(file *ast.File, filename string) {
		if (importer == "pkg/transports/http/client" || importer == "pkg/transports/http/generated") && !ast.IsGenerated(file) {
			pass.Reportf(file.Package, "generated-only: handwritten Go file in generated-only package %s; generate source with the standard Code generated ... DO NOT EDIT. marker", importer)
		}
		if ast.IsGenerated(file) {
			return
		}
		test := strings.HasSuffix(filename, "_test.go")
		hasTests = hasTests || test
		found = append(found, layeringImports(file, importer, unit, test)...)
	}
	for _, file := range pass.Files {
		visit(file, pass.Fset.Position(file.Pos()).Filename)
	}
	// Files excluded by build constraints are invisible to the type checker;
	// their imports are read with the one allowed parse (ImportsOnly) so the
	// edge rules do not depend on the GOOS or tags of the vet run.
	visited := map[string]bool{}
	for _, name := range slices.Concat(pass.IgnoredFiles, pass.OtherFiles) {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		name = filepath.Clean(name)
		if visited[name] {
			continue
		}
		visited[name] = true
		contents, err := pass.ReadFile(name)
		if err != nil {
			return nil, err
		}
		file, err := parser.ParseFile(pass.Fset, name, contents, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return nil, err
		}
		visit(file, name)
	}
	reportAgainstBaseline(pass, unit, layeringRuleNames, found, hasTests)
	return nil, nil
}

func layeringImports(file *ast.File, importer, unit string, test bool) []violation {
	var found []violation
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !strings.HasPrefix(path, modulePrefix) {
			continue
		}
		e := edge{importer: importer, importee: strings.TrimPrefix(path, modulePrefix), test: test}
		for _, rule := range layeringRules {
			if !rule.violates(e) {
				continue
			}
			name := rule.name
			if test {
				name += "-test"
			}
			found = append(found, violation{rule: name, importer: unit, importee: e.importee, pos: spec.Pos(), hint: rule.hint})
		}
	}
	return found
}
