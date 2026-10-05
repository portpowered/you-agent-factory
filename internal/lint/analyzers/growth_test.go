package analyzers

import (
	"go/ast"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestBaselineGrowthSeedsOnlyNewRuleIDs(t *testing.T) {
	t.Parallel()
	seeds, err := CompareBaselineGrowth("# base\nold|a|b\n", "old|a|b\nnew|a|b\nnew|c|d\n")
	if err != nil || !reflect.DeepEqual(seeds, []string{"new"}) {
		t.Fatalf("seeds=%v err=%v", seeds, err)
	}
	for _, base := range []string{"", "# empty"} {
		if _, err := CompareBaselineGrowth(base, "new|a|b"); err != nil {
			t.Fatal(err)
		}
	}
}

// The reporter is shared by all migrated rules. Keep these pass-local cases
// serialized because useFixtures replaces the package's baseline provider.
func TestMigratedRuleBaselineIsolation(t *testing.T) {
	const unit = "pkg/transports/fixture"
	rules := []string{"service-construction", "transport-lifecycle", "petri-public"}
	for _, rule := range rules {
		t.Run(rule, func(t *testing.T) {
			listed := violation{rule: rule, importer: unit, importee: "Listed", pos: 2}
			unlisted := violation{rule: rule, importer: unit, importee: "Unlisted", pos: 3}
			if rule == "petri-public" {
				listed.importee += "|" + petriPackage + ".Marking"
				unlisted.importee += "|" + petriPackage + ".Marking"
			}
			entries := []string{listed.key()}
			for _, other := range rules {
				if other != rule {
					// Same unit and target must not suppress this rule's finding,
					// or be declared stale by this rule's pass.
					entries = append(entries, other+"|"+unit+"|"+unlisted.importee)
				}
			}
			entries = append(entries, rule+"|pkg/other|"+listed.importee)
			for _, tc := range []struct {
				name    string
				entries []string
				found   []violation
				want    string
			}{
				{"exact suppression and duplicate", entries, []violation{listed, listed}, ""},
				{"foreign rule cannot suppress new key", entries, []violation{listed, unlisted}, "exact key `" + unlisted.key() + "`"},
				{"duplicate new key reports once", entries, []violation{listed, unlisted, unlisted}, "exact key `" + unlisted.key() + "`"},
				{"only owned unit becomes stale", entries, nil, "stale baseline entry `" + listed.key() + "`"},
				{"deleting stale key restores success", entries[1:], nil, ""},
			} {
				t.Run(tc.name, func(t *testing.T) {
					useFixtures(t, tc.entries...)
					var diagnostics []analysis.Diagnostic
					pass := &analysis.Pass{
						Analyzer: &analysis.Analyzer{Name: "fixture"},
						Files:    []*ast.File{{Package: token.Pos(1)}},
						Report:   func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) },
					}
					reportAgainstBaseline(pass, unit, map[string]bool{rule: true}, tc.found, false)
					if tc.want == "" {
						if len(diagnostics) != 0 {
							t.Fatalf("unexpected diagnostics: %v", diagnostics)
						}
					} else if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, tc.want) {
						t.Fatalf("diagnostics = %v, want exactly one containing %q", diagnostics, tc.want)
					}
				})
			}
		})
	}
}
func TestBaselineGrowthRejectsEstablishedRuleKeys(t *testing.T) {
	t.Parallel()
	for _, head := range []string{"old|a|c", "old|a|b\nold|a|c", "new|x|y\nold|a|c"} {
		if _, err := CompareBaselineGrowth("old|a|b\nold|z|b", head); err == nil {
			t.Fatalf("accepted %q", head)
		}
	}
	for _, head := range []string{"old|a|b", "", "# removed"} {
		if _, err := CompareBaselineGrowth("old|a|b", head); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBaselineGrowthInterfaceMemberReplacement(t *testing.T) {
	t.Parallel()
	const prefix = "service-root-interface-count|pkg/services/factory_runtime|"
	const clock = "pkg/services/factory_runtime/clock.go:Clock"
	const opener = "pkg/services/factory_runtime/composition_contracts.go:WorkerAttemptOpener"
	const retained = "pkg/services/factory_runtime/clock.go:LogicalClock"
	base := prefix + clock + "," + retained
	for _, tc := range []struct {
		name      string
		base      string
		head      string
		wantError bool
	}{
		{"single member swap", base, prefix + retained + "," + opener, false},
		{"member growth", base, base + "," + opener, true},
		{"foreign package", base, "service-root-interface-count|pkg/services/other|" + retained + "," + opener, true},
		{"extra allowance", base, base + "\n" + prefix + retained + "," + opener, true},
		{"duplicate member hides growth", base, prefix + opener + "," + opener + "," + retained, true},
		{"empty member", base, prefix + opener + ",", true},
		{"multiple member replacements", base, prefix + opener + ",other.go:Other", true},
		{"other rule stays deletion only", "old|unit|a,b", "old|unit|a,c", true},
		{"swap cannot offset another violation", base + "\nold|unit|a", prefix + retained + "," + opener + "\nold|unit|b", true},
		{"ambiguous prior allowance", base + "\n" + prefix + retained + ",other.go:Other", prefix + retained + "," + opener, true},
		{"allowance deletion", base, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			seeds, err := CompareBaselineGrowth(tc.base, tc.head)
			if (err != nil) != tc.wantError {
				t.Fatalf("seeds=%v err=%v, wantError=%v", seeds, err, tc.wantError)
			}
			if err == nil && len(seeds) != 0 {
				t.Fatalf("replacement seeded new rule IDs: %v", seeds)
			}
		})
	}
}
