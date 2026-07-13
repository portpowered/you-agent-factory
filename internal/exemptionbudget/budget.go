package exemptionbudget

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

const (
	BaselinePath = "backend-exemption-budget.json"
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
			return fmt.Errorf("exemption baseline entry %s has empty owner", identity)
		}
		if strings.TrimSpace(entry.RemovalReason) == "" {
			return fmt.Errorf("exemption baseline entry %s has empty removalReason", identity)
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
		return rule + "/<empty-target>"
	}
	return rule + "/" + target
}

func compareIdentity(left, right Entry) int {
	if byRule := strings.Compare(left.Rule, right.Rule); byRule != 0 {
		return byRule
	}
	return strings.Compare(left.Target, right.Target)
}
