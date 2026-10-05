package analyzers

import (
	"fmt"
	"sort"
	"strings"
)

// CompareBaselineGrowth allows the first seed of a rule absent from base.
// Established rules remain deletion-only except a single count-neutral
// interface member replacement within the same package's existing allowance.
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
		if isInterfaceMemberReplacement(key, base, head) {
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

func isInterfaceMemberReplacement(key string, base, head map[string]struct{}) bool {
	parts := strings.Split(key, "|")
	if len(parts) != 3 || parts[0] != "service-root-interface-count" {
		return false
	}
	prefix := parts[0] + "|" + parts[1] + "|"
	previous, baseCount := interfaceAllowanceMembers(prefix, base)
	current, headCount := interfaceAllowanceMembers(prefix, head)
	if baseCount != 1 || headCount != 1 || len(previous) != len(current) {
		return false
	}
	removed := 0
	for member := range previous {
		if _, exists := current[member]; !exists {
			removed++
		}
	}
	return removed == 1
}

func interfaceAllowanceMembers(prefix string, entries map[string]struct{}) (map[string]struct{}, int) {
	var members map[string]struct{}
	count := 0
	for key := range entries {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		count++
		members = make(map[string]struct{})
		for _, member := range strings.Split(strings.TrimPrefix(key, prefix), ",") {
			if member == "" {
				return nil, 0
			}
			if _, duplicate := members[member]; duplicate {
				return nil, 0
			}
			members[member] = struct{}{}
		}
	}
	return members, count
}

// CheckBaselineGrowth compares a single merge-base object with this build's
// embedded baseline; it does not load source packages.
func CheckBaselineGrowth(baseText string) ([]string, error) {
	return CompareBaselineGrowth(baseText, baselineText)
}
