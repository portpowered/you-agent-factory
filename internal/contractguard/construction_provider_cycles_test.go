package contractguard

import "testing"

type constructionProviderCycleCase struct {
	name, body, helpers string
	want                int
}

func constructionProviderCycleCases() []constructionProviderCycleCase {
	return []constructionProviderCycleCase{
		{"direct recursion", `Provide(p); selected.New(p)`, "", 1},
		{"recursion beside opaque dispatch", `var next func(); next(); Provide(p); selected.New(p)`, "", 1},
		{"conditional recursion", `if flag { Provide(p) }; selected.New(p)`, "", 1},
		{"provider alias", `again := Provide; again(p); selected.New(p)`, "", 1},
		{"mutual recursion", `step(p); selected.New(p)`, `func step(p selected.Port) { Provide(p) }`, 1},
		{"helper alias recursion", `step(p); selected.New(p)`, `func step(p selected.Port) { again := Provide; again(p) }`, 1},
		{"long cycle", `step(p); selected.New(p)`, `func step(p selected.Port) { next(p) }; func next(p selected.Port) { Provide(p) }`, 1},
		{"deferred recursion", `defer Provide(p); selected.New(p)`, "", 1},
		{"asynchronous recursion", `go Provide(p); selected.New(p)`, "", 1},
		{"generic helper recursion", `step[int](p); selected.New(p)`, `func step[T any](p selected.Port) { Provide(p) }`, 1},
		{"method helper recursion", `helper{}.step(p); selected.New(p)`, `type helper struct{}; func (helper) step(p selected.Port) { Provide(p) }`, 1},
		{"invoked literal", `func() { Provide(p) }(); selected.New(p)`, "", 1},
		{"parenthesized literal", `(func() { Provide(p) })(); selected.New(p)`, "", 1},
		{"invoked local closure", `again := func() { Provide(p) }; again(); selected.New(p)`, "", 1},
		{"closure alias", `again := func() { Provide(p) }; next := again; next(); selected.New(p)`, "", 1},
		{"declared closure", `var again = func() { Provide(p) }; again(); selected.New(p)`, "", 1},
		{"nested invoked closure", `func() { func() { Provide(p) }() }(); selected.New(p)`, "", 1},
		{"helper invokes closure", `step(p); selected.New(p)`, `func step(p selected.Port) { again := func() { Provide(p) }; again() }`, 1},
		{"closure calls helper", `func() { step(p) }(); selected.New(p)`, `func step(p selected.Port) { Provide(p) }`, 1},
		{"deferred closure", `defer func() { Provide(p) }(); selected.New(p)`, "", 1},
		{"asynchronous closure alias", `again := func() { Provide(p) }; go again(); selected.New(p)`, "", 1},
		{"uncalled nested closure", `func() { _ = func() { Provide(p) } }(); selected.New(p)`, "", 0},
		{"acyclic invoked closure", `func() { consume(p) }(); selected.New(p)`, `func consume(selected.Port) {}`, 0},
		{"shadowed closure binding", `again := func() { Provide(p) }; _ = again; { again := func() {}; again() }; selected.New(p)`, "", 0},
		{"closure argument without invocation", `consume(func() { Provide(p) }); selected.New(p)`, `func consume(func()) {}`, 0},
		{"package closure", `again(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 1},
		{"package closure alias", `again(p); selected.New(p)`, `var again = next; var next = func(p selected.Port) { Provide(p) }`, 1},
		{"local alias of package closure", `local := again; local(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 1},
		{"package closure helper", `again(p); selected.New(p)`, `var again = func(p selected.Port) { step(p) }; func step(p selected.Port) { Provide(p) }`, 1},
		{"deferred package closure", `defer again(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 1},
		{"asynchronous package closure", `go again(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 1},
		{"uncalled package closure", `selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 0},
		{"shadowed package closure", `again := func(selected.Port) {}; again(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 0},
		{"acyclic package closure", `again(p); selected.New(p)`, `var again = func(selected.Port) {}`, 0},
		{"package closure called in declaration file", `step(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }; func step(p selected.Port) { again(p) }`, 1},
		{"package closure local shadow write", `{ again := func(selected.Port) {}; again = func(selected.Port) {}; _ = again }; again(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 1},
		{"package closure local shadow address", `{ again := func(selected.Port) {}; _ = &again }; again(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 1},
		{"acyclic helper", `step(p); selected.New(p)`, `func step(p selected.Port) { consume(p) }; func consume(selected.Port) {}`, 0},
		{"unrelated helper cycle", `step(p); selected.New(p)`, `func step(p selected.Port) { step(p) }`, 0},
		{"shadowed provider", `Provide := func(selected.Port) {}; Provide(p); selected.New(p)`, "", 0},
		{"uninvoked helper", `selected.New(p)`, `func step(p selected.Port) { Provide(p) }`, 0},
		{"uninvoked closure", `_ = func() { Provide(p) }; selected.New(p)`, "", 0},
		{"shadowed helper", `step := func(selected.Port) {}; step(p); selected.New(p)`, `func step(p selected.Port) { Provide(p) }`, 0},
	}
}

func TestConstructionRecursiveFocusedProvider(t *testing.T) {
	t.Parallel()
	for _, tc := range constructionProviderCycleCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkConstructionProviderCase(t, tc, "registered-construction")
		})
	}
}

func checkConstructionProviderCase(t *testing.T, tc constructionProviderCycleCase, wantRule string) {
	t.Helper()
	root, registry := constructionFixture(t)
	writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New(p Port) (*Service, error) { return &Service{port: p}, nil }
`)
	writeConstructionFixture(t, root, "pkg/wire/provider.go", `package wire
import selected "example.test/factory/pkg/owner"
func Provide(p selected.Port) { `+tc.body+` }
var flag bool
`)
	writeConstructionFixture(t, root, "pkg/wire/helpers.go", "package wire\nimport selected \"example.test/factory/pkg/owner\"\n"+tc.helpers)
	for _, mode := range []ConstructionMode{ConstructionReport, ConstructionEnforce} {
		registry.CapabilitySets[0].Mode = mode
		findings, err := ScanConstruction(root, registry)
		if err != nil || len(findings) != tc.want {
			t.Fatalf("mode %s: findings = %+v, error = %v, want %d", mode, findings, err, tc.want)
		}
		blocking := tc.want
		if mode == ConstructionReport {
			blocking = 0
		}
		if CountBlockingConstructionFindings(findings) != blocking {
			t.Fatalf("unexpected blocking findings: %+v", findings)
		}
		for _, finding := range findings {
			if finding.Rule != wantRule || finding.Caller != registry.Allowances[0].Caller || finding.Callee != registry.Constructors[0].Symbol || finding.FilePath != "pkg/wire/provider.go" || finding.Line != 3 {
				t.Fatalf("unexpected qualified finding: %+v", finding)
			}
		}
	}
}
