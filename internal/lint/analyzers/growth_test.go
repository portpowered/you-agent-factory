package analyzers

import (
	"reflect"
	"testing"
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
