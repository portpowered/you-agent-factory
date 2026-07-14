package baseline_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/baseline"
	"github.com/spf13/cobra"
)

const runFlagsFixture = "testdata/run_flags.txt"

func TestRunFlagsBaseline_MatchesFixture(t *testing.T) {
	runCmd := productionRunCommand(t)
	got := baseline.SerializeRunFlags(runCmd)

	want, err := baseline.ReadFixtureText(runFlagsFixture)
	if err != nil {
		t.Fatalf("read run flags baseline fixture: %v", err)
	}

	if got == want {
		return
	}

	t.Fatalf(
		"run flags baseline drift detected; update %s when intentional\n%s",
		runFlagsFixture,
		formatLineDiff(want, got),
	)
}

func TestRunFlagsBaseline_IsStableAcrossRuns(t *testing.T) {
	runCmd := productionRunCommand(t)

	first := baseline.SerializeRunFlags(runCmd)
	second := baseline.SerializeRunFlags(runCmd)
	if first != second {
		t.Fatalf("run flags serialization is not stable across repeated runs\n%s", formatLineDiff(first, second))
	}
}

func productionRunCommand(t *testing.T) *cobra.Command {
	t.Helper()

	root := cli.NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run command: %v", err)
	}
	return runCmd
}

func formatLineDiff(want, got string) string {
	wantLines := nonEmptyLines(want)
	gotLines := nonEmptyLines(got)

	wantSet := make(map[string]struct{}, len(wantLines))
	for _, line := range wantLines {
		wantSet[line] = struct{}{}
	}
	gotSet := make(map[string]struct{}, len(gotLines))
	for _, line := range gotLines {
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
	b.WriteString("removed flags:\n")
	if len(removed) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, line := range removed {
			b.WriteString("  - ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	b.WriteString("added flags:\n")
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

func nonEmptyLines(text string) []string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
