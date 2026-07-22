package exemptionbudget

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	BaselinePath = "docs/internal/baselines/backend-exemption-budget.json"
	Version      = 1

	RuleBackendFile       = "backendsizecheck:ignore-file"
	RuleBackendFunction   = "backendsizecheck:ignore-function"
	RulePackageFileLines  = "pkgmaintcheck:ignore-file-lines"
	RulePackageFuncLines  = "pkgmaintcheck:ignore-function-lines"
	RulePackageComplexity = "pkgmaintcheck:ignore-cyclomatic-complexity"
)

var supportedRules = []string{
	RuleBackendFile,
	RuleBackendFunction,
	RulePackageFileLines,
	RulePackageFuncLines,
	RulePackageComplexity,
}

const minimumRemovalReasonLength = 20

var removalActionWords = []string{
	"delete",
	"extract",
	"migrate",
	"move",
	"reduce",
	"refactor",
	"remove",
	"replace",
	"simplify",
	"split",
}

type Baseline struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

type Entry struct {
	Rule          string `json:"rule"`
	Target        string `json:"target"`
	Owner         string `json:"owner"`
	RemovalReason string `json:"removalReason"`
}

type Directive struct {
	Rule   string
	Target string
	Reason string
}

type DifferenceKind string

const (
	DifferenceUnregistered DifferenceKind = "unregistered"
	DifferenceStale        DifferenceKind = "stale"
	DifferenceDuplicate    DifferenceKind = "duplicate"
)

type Difference struct {
	Kind   DifferenceKind
	Rule   string
	Target string
}

func Reconcile(root string, scope Scope) ([]Difference, error) {
	data, err := os.ReadFile(filepath.Join(root, BaselinePath))
	if err != nil {
		return nil, fmt.Errorf("read exemption baseline %s: %w", BaselinePath, err)
	}
	baseline, err := Parse(data)
	if err != nil {
		return nil, err
	}
	directives, err := Discover(root, scope)
	if err != nil {
		return nil, err
	}
	baseline.Entries = slices.DeleteFunc(baseline.Entries, func(entry Entry) bool {
		return !includedRule(entry.Rule, scope)
	})
	return Compare(directives, baseline), nil
}

func FormatDifference(difference Difference) string {
	identity := fmt.Sprintf("rule=%s target=%s", difference.Rule, difference.Target)
	switch difference.Kind {
	case DifferenceUnregistered:
		return fmt.Sprintf("exemption budget %s is unregistered; add a sorted %s entry with a non-empty owner and actionable removalReason", identity, BaselinePath)
	case DifferenceStale:
		return fmt.Sprintf("exemption budget %s is stale; remove its %s entry or restore the directive", identity, BaselinePath)
	case DifferenceDuplicate:
		return fmt.Sprintf("exemption budget %s is duplicated; keep exactly one directive and one %s entry", identity, BaselinePath)
	default:
		return fmt.Sprintf("exemption budget %s has unknown status %q", identity, difference.Kind)
	}
}

func Parse(data []byte) (Baseline, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var baseline Baseline
	if err := decoder.Decode(&baseline); err != nil {
		return Baseline{}, fmt.Errorf("parse exemption baseline: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Baseline{}, fmt.Errorf("parse exemption baseline: unexpected trailing JSON value")
		}
		return Baseline{}, fmt.Errorf("parse exemption baseline: %w", err)
	}
	if err := Validate(baseline); err != nil {
		return Baseline{}, err
	}
	return baseline, nil
}

func Validate(baseline Baseline) error {
	if baseline.Version != Version {
		return fmt.Errorf("exemption baseline version must be %d, got %d", Version, baseline.Version)
	}

	for index, entry := range baseline.Entries {
		identity := entryIdentity(entry.Rule, entry.Target)
		if !slices.Contains(supportedRules, entry.Rule) {
			return fmt.Errorf("exemption baseline entry %s has unsupported rule", identity)
		}
		if strings.TrimSpace(entry.Target) == "" {
			return fmt.Errorf("exemption baseline entry %d has empty target", index)
		}
		if strings.TrimSpace(entry.Owner) == "" {
			return fmt.Errorf("exemption baseline entry %s has empty owner; set a non-empty owner", identity)
		}
		if strings.TrimSpace(entry.RemovalReason) == "" {
			return fmt.Errorf("exemption baseline entry %s has empty removalReason; describe the concrete refactor or threshold-removal condition", identity)
		}
		if !actionableRemovalReason(entry.RemovalReason) {
			return fmt.Errorf("exemption baseline entry %s has non-actionable removalReason; use at least %d characters and name a concrete removal action (%s)", identity, minimumRemovalReasonLength, strings.Join(removalActionWords, ", "))
		}
		if index == 0 {
			continue
		}
		comparison := compareIdentity(baseline.Entries[index-1], entry)
		if comparison == 0 {
			return fmt.Errorf("exemption baseline entry %s is duplicated", identity)
		}
		if comparison > 0 {
			return fmt.Errorf("exemption baseline entries must be sorted by rule and target: %s appears out of order", identity)
		}
	}
	return nil
}

func actionableRemovalReason(reason string) bool {
	trimmed := strings.TrimSpace(reason)
	if len([]rune(trimmed)) < minimumRemovalReasonLength {
		return false
	}
	words := strings.FieldsFunc(strings.ToLower(trimmed), func(character rune) bool {
		return character < 'a' || character > 'z'
	})
	return slices.ContainsFunc(words, func(word string) bool {
		return slices.Contains(removalActionWords, word)
	})
}

func Compare(directives []Directive, baseline Baseline) []Difference {
	directiveCounts := make(map[string]int, len(directives))
	entryCounts := make(map[string]int, len(baseline.Entries))
	identities := make(map[string]Directive, len(directives)+len(baseline.Entries))

	for _, directive := range directives {
		key := identityKey(directive.Rule, directive.Target)
		directiveCounts[key]++
		identities[key] = directive
	}
	for _, entry := range baseline.Entries {
		key := identityKey(entry.Rule, entry.Target)
		entryCounts[key]++
		identities[key] = Directive{Rule: entry.Rule, Target: entry.Target}
	}

	differences := make([]Difference, 0)
	for key, identity := range identities {
		directiveCount, entryCount := directiveCounts[key], entryCounts[key]
		switch {
		case directiveCount > 1 || entryCount > 1:
			differences = append(differences, Difference{Kind: DifferenceDuplicate, Rule: identity.Rule, Target: identity.Target})
		case directiveCount == 0:
			differences = append(differences, Difference{Kind: DifferenceStale, Rule: identity.Rule, Target: identity.Target})
		case entryCount == 0:
			differences = append(differences, Difference{Kind: DifferenceUnregistered, Rule: identity.Rule, Target: identity.Target})
		}
	}
	slices.SortFunc(differences, func(left, right Difference) int {
		if byRule := strings.Compare(left.Rule, right.Rule); byRule != 0 {
			return byRule
		}
		if byTarget := strings.Compare(left.Target, right.Target); byTarget != 0 {
			return byTarget
		}
		return strings.Compare(string(left.Kind), string(right.Kind))
	})
	return differences
}

func identityKey(rule, target string) string {
	return rule + "\x00" + target
}

func entryIdentity(rule, target string) string {
	if strings.TrimSpace(target) == "" {
		return fmt.Sprintf("rule=%s target=<empty-target>", rule)
	}
	return fmt.Sprintf("rule=%s target=%s", rule, target)
}

func compareIdentity(left, right Entry) int {
	if byRule := strings.Compare(left.Rule, right.Rule); byRule != 0 {
		return byRule
	}
	return strings.Compare(left.Target, right.Target)
}
