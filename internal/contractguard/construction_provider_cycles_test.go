package contractguard

import "testing"

func TestConstructionRecursiveFocusedProvider(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body, helpers string
		want                int
	}{
		{"direct recursion", `Provide(p); selected.New(p)`, "", 1},
		{"conditional recursion", `if flag { Provide(p) }; selected.New(p)`, "", 1},
		{"provider alias", `again := Provide; again(p); selected.New(p)`, "", 1},
		{"mutual recursion", `step(p); selected.New(p)`, `func step(p selected.Port) { Provide(p) }`, 1},
		{"helper alias recursion", `step(p); selected.New(p)`, `func step(p selected.Port) { again := Provide; again(p) }`, 1},
		{"long cycle", `step(p); selected.New(p)`, `func step(p selected.Port) { next(p) }; func next(p selected.Port) { Provide(p) }`, 1},
		{"deferred recursion", `defer Provide(p); selected.New(p)`, "", 1},
		{"asynchronous recursion", `go Provide(p); selected.New(p)`, "", 1},
		{"generic helper recursion", `step[int](p); selected.New(p)`, `func step[T any](p selected.Port) { Provide(p) }`, 1},
		{"method helper recursion", `helper{}.step(p); selected.New(p)`, `type helper struct{}; func (helper) step(p selected.Port) { Provide(p) }`, 1},
		{"acyclic helper", `step(p); selected.New(p)`, `func step(p selected.Port) { consume(p) }; func consume(selected.Port) {}`, 0},
		{"unrelated helper cycle", `step(p); selected.New(p)`, `func step(p selected.Port) { step(p) }`, 0},
		{"shadowed provider", `Provide := func(selected.Port) {}; Provide(p); selected.New(p)`, "", 0},
		{"uninvoked helper", `selected.New(p)`, `func step(p selected.Port) { Provide(p) }`, 0},
		{"uninvoked closure", `_ = func() { Provide(p) }; selected.New(p)`, "", 0},
		{"shadowed helper", `step := func(selected.Port) {}; step(p); selected.New(p)`, `func step(p selected.Port) { Provide(p) }`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
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
					if finding.Rule != "registered-construction" || finding.Caller != registry.Allowances[0].Caller || finding.Callee != registry.Constructors[0].Symbol || finding.FilePath != "pkg/wire/provider.go" || finding.Line != 3 {
						t.Fatalf("unexpected qualified finding: %+v", finding)
					}
				}
			}
		})
	}
}
