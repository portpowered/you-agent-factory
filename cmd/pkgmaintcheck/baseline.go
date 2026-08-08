package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	pkgMaintBaselinePath    = "docs/internal/baselines/pkg-maint-baseline.json"
	pkgMaintBaselineVersion = 1
	pkgMaintBaselineStage   = "pkg-maint"
)

type pkgMaintBaseline struct {
	Version int                     `json:"version"`
	Stage   string                  `json:"stage"`
	Entries []pkgMaintBaselineEntry `json:"entries"`
}

type pkgMaintBaselineEntry struct {
	Rule   string `json:"rule"`
	Target string `json:"target"`
	Actual int    `json:"actual"`
	Limit  int    `json:"limit"`
}

type pkgMaintBaselineComparison struct {
	New       []violation
	Regressed []violation
	Stale     []pkgMaintBaselineEntry
}

func loadPkgMaintBaseline(root string) (pkgMaintBaseline, error) {
	path := filepath.Join(root, filepath.FromSlash(pkgMaintBaselinePath))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return pkgMaintBaseline{Version: pkgMaintBaselineVersion, Stage: pkgMaintBaselineStage}, nil
	}
	if err != nil {
		return pkgMaintBaseline{}, fmt.Errorf("read pkg-maint baseline %s: %w", pkgMaintBaselinePath, err)
	}

	var baseline pkgMaintBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return pkgMaintBaseline{}, fmt.Errorf("parse pkg-maint baseline %s: %w", pkgMaintBaselinePath, err)
	}
	if baseline.Version != pkgMaintBaselineVersion {
		return pkgMaintBaseline{}, fmt.Errorf("pkg-maint baseline %s has version %d, want %d", pkgMaintBaselinePath, baseline.Version, pkgMaintBaselineVersion)
	}
	if baseline.Stage != pkgMaintBaselineStage {
		return pkgMaintBaseline{}, fmt.Errorf("pkg-maint baseline %s has stage %q, want %q", pkgMaintBaselinePath, baseline.Stage, pkgMaintBaselineStage)
	}
	if err := validatePkgMaintBaselineEntries(baseline.Entries); err != nil {
		return pkgMaintBaseline{}, err
	}
	return baseline, nil
}

func validatePkgMaintBaselineEntries(entries []pkgMaintBaselineEntry) error {
	previous := ""
	for index, entry := range entries {
		if strings.TrimSpace(entry.Rule) == "" || strings.TrimSpace(entry.Target) == "" {
			return fmt.Errorf("pkg-maint baseline entry %d has an empty rule or target", index)
		}
		if entry.Actual <= entry.Limit || entry.Limit <= 0 {
			return fmt.Errorf("pkg-maint baseline entry %d has invalid actual/limit %d/%d", index, entry.Actual, entry.Limit)
		}
		key := pkgMaintBaselineKey(entry.Rule, entry.Target)
		if index > 0 && key <= previous {
			return fmt.Errorf("pkg-maint baseline entries must be sorted and unique at %q", key)
		}
		previous = key
	}
	return nil
}

func comparePkgMaintBaseline(violations []violation, baseline pkgMaintBaseline) pkgMaintBaselineComparison {
	recorded := make(map[string]pkgMaintBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		recorded[pkgMaintBaselineKey(entry.Rule, entry.Target)] = entry
	}

	comparison := pkgMaintBaselineComparison{}
	active := make(map[string]struct{}, len(violations))
	for _, finding := range violations {
		entry := pkgMaintBaselineEntryForViolation(finding)
		key := pkgMaintBaselineKey(entry.Rule, entry.Target)
		active[key] = struct{}{}
		previous, ok := recorded[key]
		if !ok {
			comparison.New = append(comparison.New, finding)
			continue
		}
		if entry.Actual > previous.Actual {
			comparison.Regressed = append(comparison.Regressed, finding)
		}
	}
	for _, entry := range baseline.Entries {
		if _, ok := active[pkgMaintBaselineKey(entry.Rule, entry.Target)]; !ok {
			comparison.Stale = append(comparison.Stale, entry)
		}
	}
	slices.SortFunc(comparison.Stale, func(left, right pkgMaintBaselineEntry) int {
		return strings.Compare(pkgMaintBaselineKey(left.Rule, left.Target), pkgMaintBaselineKey(right.Rule, right.Target))
	})
	return comparison
}

func pkgMaintBaselineEntryForViolation(finding violation) pkgMaintBaselineEntry {
	target := finding.filePath
	if finding.function != "" {
		target += "#" + finding.function
	}
	return pkgMaintBaselineEntry{Rule: finding.rule, Target: target, Actual: finding.actual, Limit: finding.limit}
}

func pkgMaintBaselineKey(rule, target string) string {
	return rule + "\x00" + target
}
