package baseline_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/baseline"
)

const docsTopicIndexFixture = "testdata/docs_topic_index.txt"

func TestDocsTopicIndexBaseline_MatchesFixture(t *testing.T) {
	got := baseline.SerializeDocsTopicIndex()

	want, err := baseline.ReadFixtureText(fixtureSourceStore(), docsTopicIndexFixture)
	if err != nil {
		t.Fatalf("read docs topic index baseline fixture: %v", err)
	}

	if got == want {
		return
	}

	t.Fatalf(
		"docs topic index baseline drift detected; update %s when intentional\n%s",
		docsTopicIndexFixture,
		formatDocsTopicIndexDiff(want, got),
	)
}

func TestDocsTopicIndexBaseline_IsStableAcrossRuns(t *testing.T) {
	first := baseline.SerializeDocsTopicIndex()
	second := baseline.SerializeDocsTopicIndex()
	if first != second {
		t.Fatalf(
			"docs topic index serialization is not stable across repeated runs\n%s",
			formatDocsTopicIndexDiff(first, second),
		)
	}
}

func formatDocsTopicIndexDiff(want, got string) string {
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
	b.WriteString("removed topics:\n")
	if len(removed) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, line := range removed {
			b.WriteString("  - ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	b.WriteString("added topics:\n")
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
