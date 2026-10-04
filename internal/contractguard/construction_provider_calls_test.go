package contractguard

import (
	"bytes"
	"strings"
	"testing"
)

func TestConstructionFocusedProviderExecution(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want int
	}{
		{"direct", `selected.New(p)`, 0},
		{"immutable alias", `build := selected.New; build(p)`, 0},
		{"conditional synchronous", `if flag { selected.New(p) }`, 0},
		{"deferred constructor", `defer selected.New(p)`, 1},
		{"asynchronous constructor", `go selected.New(p)`, 1},
		{"deferred alias", `build := selected.New; defer build(p)`, 1},
		{"asynchronous alias", `build := selected.New; go build(p)`, 1},
		{"returned factory", `factory = func() { selected.New(p) }`, 1},
		{"immediate closure", `func() { selected.New(p) }()`, 1},
		{"nested closure", `factory = func() { factory = func() { selected.New(p) } }`, 1},
		{"deferred argument evaluated now", `defer consume(selected.New(p))`, 0},
		{"asynchronous argument evaluated now", `go consume(selected.New(p))`, 0},
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
var factory func()
func consume(*selected.Service, error) {}
`)
			for _, mode := range []ConstructionMode{ConstructionEnforce, ConstructionReport} {
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
				var output bytes.Buffer
				WriteConstructionFindings(&output, findings)
				if strings.Contains(output.String(), tc.body) {
					t.Fatalf("source disclosed: %s", output.String())
				}
			}
		})
	}
}
