package contractguard

import (
	"bytes"
	"strings"
	"testing"
)

func TestConstructionImmutableValues(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, operation string
		calls, debts    int
	}{
		{"short declaration", `alias := New; alias(p)`, 1, 0},
		{"var declaration", `var alias = New; alias(p)`, 1, 0},
		{"chain", `first := New; second := first; second(p)`, 1, 0},
		{"parenthesized chain", `first := (New); second := (first); (second)(p)`, 1, 0},
		{"parallel assignment", `first, second := New, New; first(p); second(p)`, 2, 0},
		{"closure", `alias := New; defer func() { alias(p) }()`, 1, 0},
		{"defer", `alias := New; defer alias(p)`, 1, 0},
		{"go", `alias := New; go alias(p)`, 1, 0},
		{"shadowed", `alias := New; { alias := func(Port) {}; alias(p) }; alias(p)`, 1, 0},
		{"escape", `alias := New; _ = alias`, 0, 2},
		{"escape through chain", `first := New; second := first; _ = second`, 0, 3},
		{"reassignment", `alias := New; alias = func(Port) (*Service, error) { return nil, nil }; alias(p)`, 0, 1},
		{"closure write", `alias := New; func() { alias = New }(); alias(p)`, 0, 2},
		{"range write", `alias := New; for _, alias = range []func(Port) (*Service, error){New} { alias(p) }`, 0, 2},
		{"unused", `alias := New`, 0, 1},
		{"domain value", `alias := func(Port) {}; alias(p)`, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
func New(p Port) (*Service, error) { return &Service{port: p}, nil }
func Run(p Port) { `+tc.operation+` }
`)
			findings, err := ScanConstruction(root, registry)
			if err != nil {
				t.Fatal(err)
			}
			calls, debts := 0, 0
			for _, finding := range findings {
				if finding.Caller != (ConstructionSymbol{ImportPath: fixtureOwner, Name: "Run"}) || finding.Callee != registry.Constructors[0].Symbol || finding.FilePath != "pkg/owner/service.go" || finding.Line != 5 {
					t.Fatalf("qualified value finding = %+v", finding)
				}
				switch finding.Rule {
				case "registered-construction":
					calls++
				case "unresolved-construction-reference":
					debts++
				default:
					t.Fatalf("unexpected rule: %+v", finding)
				}
			}
			if calls != tc.calls || debts != tc.debts || CountBlockingConstructionFindings(findings) != calls+debts {
				t.Fatalf("calls/debts = %d/%d, want %d/%d: %+v", calls, debts, tc.calls, tc.debts, findings)
			}
		})
	}
}

func TestConstructionQualifiedMethods(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, operation string
		calls, debts    int
	}{
		{"parameter receiver", `func Run(b selected.Builder, p selected.Port) { b.Build(p) }`, 1, 0},
		{"pointer receiver", `func Run(b *selected.Builder, p selected.Port) { b.Build(p) }`, 1, 0},
		{"allocated receiver", `func Run(p selected.Port) { b := &selected.Builder{}; b.Build(p) }`, 1, 0},
		{"new receiver", `func Run(p selected.Port) { b := new(selected.Builder); b.Build(p) }`, 1, 0},
		{"receiver alias", `func Run(b selected.Builder, p selected.Port) { alias := b; alias.Build(p) }`, 1, 0},
		{"method expression", `func Run(b selected.Builder, p selected.Port) { selected.Builder.Build(b, p) }`, 1, 0},
		{"pointer expression", `func Run(b *selected.Builder, p selected.Port) { (*selected.Builder).Build(b, p) }`, 1, 0},
		{"method value", `func Run(b selected.Builder, p selected.Port) { alias := b.Build; alias(p) }`, 1, 0},
		{"escaped method value", `func Run(b selected.Builder) { _ = b.Build }`, 0, 1},
		{"imported type alias", `func Run(b exported.Alias, p selected.Port) { b.Build(p) }`, 1, 0},
		{"local type alias", `type Local = exported.Alias; func Run(b Local, p selected.Port) { b.Build(p) }`, 1, 0},
		{"alias expression", `func Run(b exported.Alias, p selected.Port) { exported.Alias.Build(b, p) }`, 1, 0},
		{"same method other type", `func Run(b selected.Other, p selected.Port) { b.Build(p) }`, 0, 0},
		{"defined type is not alias", `type Local selected.Builder; func Run(b Local, p selected.Port) { b.Build(p) }`, 0, 0},
		{"import shadow", `func Run(selected struct { Builder struct { Build func() } }) { selected.Builder.Build() }`, 0, 0},
		{"type name shadow", `func Run(Builder struct { Build func() }) { Builder.Build() }`, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, registry := constructionFixture(t)
			writeConstructionFixture(t, root, "pkg/owner/service.go", `package owner
type Port interface { Execute() }
type Service struct { port Port }
type Builder struct{}
type Other struct{}
func New(p Port) (*Service, error) { return &Service{port: p}, nil }
func (b Builder) Build(p Port) (*Service, error) { return &Service{port: p}, nil }
func (b Other) Build(p Port) (*Service, error) { return &Service{port: p}, nil }
`)
			writeConstructionFixture(t, root, "pkg/exported/alias.go", `package exported
import selected "example.test/factory/pkg/owner"
type Alias = selected.Builder
`)
			writeConstructionFixture(t, root, "pkg/consumer/operation.go", `package consumer
import selected "example.test/factory/pkg/owner"
import exported "example.test/factory/pkg/exported"
`+tc.operation+"\n")
			method := ConstructionSymbol{ImportPath: fixtureOwner, Receiver: "Builder", Name: "Build"}
			registry.Constructors = append(registry.Constructors, ConstructionConstructor{Symbol: method, CapabilitySet: "owner", Results: registry.Constructors[0].Results})
			registry.CapabilitySets[0].Mode = ConstructionEnforce
			findings, err := ScanConstruction(root, registry)
			if err != nil {
				t.Fatal(err)
			}
			calls, debts := 0, 0
			for _, finding := range findings {
				if finding.Callee != method || finding.Caller != (ConstructionSymbol{ImportPath: "example.test/factory/pkg/consumer", Name: "Run"}) || finding.FilePath != "pkg/consumer/operation.go" || finding.Line != 4 {
					t.Fatalf("qualified method finding = %+v", finding)
				}
				if finding.Rule == "registered-construction" {
					calls++
				} else if finding.Rule == "unresolved-construction-reference" {
					debts++
				} else {
					t.Fatalf("unexpected rule: %+v", finding)
				}
			}
			if calls != tc.calls || debts != tc.debts || CountBlockingConstructionFindings(findings) != calls+debts {
				t.Fatalf("method calls/debts = %d/%d, want %d/%d: %+v", calls, debts, tc.calls, tc.debts, findings)
			}
			var output bytes.Buffer
			WriteConstructionFindings(&output, findings)
			if strings.Contains(output.String(), tc.operation) {
				t.Fatal("diagnostic exposed source text")
			}
		})
	}
}
