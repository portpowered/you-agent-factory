package baseline_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli"
	"github.com/portpowered/infinite-you/pkg/cli/baseline"
)

const commandTreeFixture = "testdata/command_tree.txt"

func TestCommandTreeBaseline_MatchesFixture(t *testing.T) {
	root := cli.NewRootCommand()
	got := baseline.SerializeCommandTree(root)

	want, err := baseline.ReadFixtureText(commandTreeFixture)
	if err != nil {
		t.Fatalf("read command tree baseline fixture: %v", err)
	}

	if got == want {
		return
	}

	t.Fatalf(
		"command tree baseline drift detected; update %s when intentional\n%s",
		commandTreeFixture,
		formatCommandTreeDiff(want, got),
	)
}

func TestCommandTreeBaseline_IsStableAcrossRuns(t *testing.T) {
	root := cli.NewRootCommand()

	first := baseline.SerializeCommandTree(root)
	second := baseline.SerializeCommandTree(root)
	if first != second {
		t.Fatalf("command tree serialization is not stable across repeated runs\n%s", formatCommandTreeDiff(first, second))
	}
}

func formatCommandTreeDiff(want, got string) string {
	wantLines := strings.Split(strings.TrimSuffix(want, "\n"), "\n")
	gotLines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")

	wantSet := make(map[string]struct{}, len(wantLines))
	for _, line := range wantLines {
		if line == "" {
			continue
		}
		wantSet[line] = struct{}{}
	}
	gotSet := make(map[string]struct{}, len(gotLines))
	for _, line := range gotLines {
		if line == "" {
			continue
		}
		gotSet[line] = struct{}{}
	}

	var removed []string
	for line := range wantSet {
		if _, ok := gotSet[line]; !ok {
			removed = append(removed, line)
		}
	}
	sort.Strings(removed)

	var added []string
	for line := range gotSet {
		if _, ok := wantSet[line]; !ok {
			added = append(added, line)
		}
	}
	sort.Strings(added)

	var b strings.Builder
	b.WriteString("removed commands:\n")
	if len(removed) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, line := range removed {
			b.WriteString("  - ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	b.WriteString("added commands:\n")
	if len(added) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, line := range added {
			b.WriteString("  + ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
