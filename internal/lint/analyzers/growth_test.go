package analyzers

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestBaselineGrowthHistoryFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, failedCommand, head, want string
		readFailure                     bool
	}{
		{name: "unchanged", head: "old|a|b"},
		{name: "new rule seed", head: "old|a|b\nnew|a|b"},
		{name: "deletion", head: ""},
		{name: "growth", head: "old|a|b\nold|a|c", want: "old|a|c"},
		{name: "missing git repository", failedCommand: "rev-parse", want: "rev-parse"},
		{name: "missing origin", failedCommand: "merge-base", want: "merge-base"},
		{name: "missing baseline object", failedCommand: "show", want: "read merge-base baseline"},
		{name: "missing current baseline", readFailure: true, want: "read current baseline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			readGit := func(args ...string) (string, error) {
				if args[0] == tc.failedCommand {
					return "", errors.New(tc.failedCommand)
				}
				switch args[0] {
				case "rev-parse":
					return "/fixture\n", nil
				case "merge-base":
					return "abc123\n", nil
				case "show":
					if args[1] != "abc123:"+baselinePath {
						t.Fatalf("wrong object: %v", args)
					}
					return "old|a|b", nil
				default:
					t.Fatalf("unexpected git operation: %v", args)
					return "", nil
				}
			}
			readHead := func(root string) (string, error) {
				if root != "/fixture" {
					t.Fatalf("root=%q", root)
				}
				if tc.readFailure {
					return "", errors.New("absent")
				}
				return tc.head, nil
			}
			base, head, err := loadBaselineHistory(readGit, readHead)
			if err == nil {
				_, err = CompareBaselineGrowth(base, head)
			}
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestBaselineGrowthSnapshotCacheIdentity(t *testing.T) {
	t.Parallel()
	clean := baselineGrowthSnapshot("old|a|b", "old|a|b", nil, "ordinary")
	for _, candidate := range []*analysis.Analyzer{
		baselineGrowthSnapshot("old|a|b", "old|a|c", nil, "ordinary"),
		baselineGrowthSnapshot("old|a|c", "old|a|b", nil, "ordinary"),
		baselineGrowthSnapshot("old|a|b", "old|a|b", errors.New("missing origin/main"), "ordinary"),
		baselineGrowthSnapshot("old|a|b", "old|a|b", nil, "strict"),
	} {
		if candidate.Name == clean.Name {
			t.Fatal("external input/configuration change reused issue-cache identity")
		}
	}
	if clean.Name != baselineGrowthSnapshot("old|a|b", "old|a|b", nil, "ordinary").Name {
		t.Fatal("unchanged snapshot must retain its cache identity")
	}
}

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
	for _, head := range []string{"old|a|b\nold|z|b\nold|a|c", "new|x|y\nold|a|b\nold|z|b\nold|a|c"} {
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
		{"multiple member replacements", base, prefix + opener + ",other.go:Other", false},
		{"other rule replacement", "old|unit|a,b", "old|unit|a,c", false},
		{"independent rule replacements", base + "\nold|unit|a", prefix + retained + "," + opener + "\nold|unit|b", false},
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

func TestBaselineGrowthMultipleKeyReplacements(t *testing.T) {
	t.Parallel()
	var base, replacement []string
	for i := range 22 {
		base = append(base, fmt.Sprintf("petri-public|old|Member%02d|Type", i))
		replacement = append(replacement, fmt.Sprintf("petri-public|new|Member%02d|Type", i))
	}
	const extra = "petri-public|z|Extra|Type"
	for _, tc := range []struct {
		name string
		head []string
		want string
	}{
		{"22 replacements", replacement, ""},
		{"22 replacements and net growth", append(append([]string{}, replacement...), extra), extra},
		{"pure addition", append(append([]string{}, base...), extra), extra},
		{"different rule cannot offset growth", append(append([]string{}, replacement...), "other|unit|Retained", "other|unit|Extra"), "other|unit|Extra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			seeds, err := CompareBaselineGrowth(strings.Join(append(append([]string{}, base...), "other|unit|Retained"), "\n"), strings.Join(tc.head, "\n"))
			if tc.want == "" {
				if err != nil || len(seeds) != 0 {
					t.Fatalf("seeds=%v err=%v", seeds, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v, want added key %q", err, tc.want)
			}
		})
	}
}
