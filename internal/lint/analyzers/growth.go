package analyzers

import (
	"fmt"
	"sort"
	"strings"
)

// CompareBaselineGrowth allows the first seed of a rule absent from base.
// Every rule already present remains deletion-only, even after key removals.
func CompareBaselineGrowth(baseText, headText string) ([]string, error) {
	base, head := parseBaseline(baseText), parseBaseline(headText)
	rules := map[string]bool{}
	for key := range base {
		rules[strings.SplitN(key, "|", 2)[0]] = true
	}
	seeded := map[string]bool{}
	var added []string
	for key := range head {
		if _, exists := base[key]; exists {
			continue
		}
		rule := strings.SplitN(key, "|", 2)[0]
		if rules[rule] {
			added = append(added, key)
		} else {
			seeded[rule] = true
		}
	}
	sort.Strings(added)
	if len(added) > 0 {
		return nil, fmt.Errorf("established baseline rules gained keys; fix the violations instead:\n%s", strings.Join(added, "\n"))
	}
	var names []string
	for rule := range seeded {
		names = append(names, rule)
	}
	sort.Strings(names)
	return names, nil
}

// CheckBaselineGrowth compares a single merge-base object with this build's
// embedded baseline; it does not load source packages.
func CheckBaselineGrowth(baseText string) ([]string, error) {
	return CompareBaselineGrowth(baseText, baselineText)
}
