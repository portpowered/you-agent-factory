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
	backendSizeBaselinePath    = "docs/internal/baselines/backend-size-baseline.json"
	backendSizeBaselineVersion = 1
	backendSizeBaselineStage   = "backend-size"
	backendSizeFileRule        = "file-line-count"
	backendSizeFunctionRule    = "function-line-count"
)

type backendSizeBaseline struct {
	Version int                        `json:"version"`
	Stage   string                     `json:"stage"`
	Entries []backendSizeBaselineEntry `json:"entries"`
}

type backendSizeBaselineEntry struct {
	Rule   string `json:"rule"`
	Target string `json:"target"`
	Actual int    `json:"actual"`
	Limit  int    `json:"limit"`
}

type backendSizeBaselineComparison struct {
	New       []violation
	Regressed []violation
	Stale     []backendSizeBaselineEntry
}

func loadBackendSizeBaseline(root string) (backendSizeBaseline, error) {
	path := filepath.Join(root, filepath.FromSlash(backendSizeBaselinePath))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return backendSizeBaseline{Version: backendSizeBaselineVersion, Stage: backendSizeBaselineStage}, nil
	}
	if err != nil {
		return backendSizeBaseline{}, fmt.Errorf("read backend-size baseline %s: %w", backendSizeBaselinePath, err)
	}

	var baseline backendSizeBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return backendSizeBaseline{}, fmt.Errorf("parse backend-size baseline %s: %w", backendSizeBaselinePath, err)
	}
	if baseline.Version != backendSizeBaselineVersion {
		return backendSizeBaseline{}, fmt.Errorf("backend-size baseline %s has version %d, want %d", backendSizeBaselinePath, baseline.Version, backendSizeBaselineVersion)
	}
	if baseline.Stage != backendSizeBaselineStage {
		return backendSizeBaseline{}, fmt.Errorf("backend-size baseline %s has stage %q, want %q", backendSizeBaselinePath, baseline.Stage, backendSizeBaselineStage)
	}
	if err := validateBackendSizeBaselineEntries(baseline.Entries); err != nil {
		return backendSizeBaseline{}, err
	}
	return baseline, nil
}

func validateBackendSizeBaselineEntries(entries []backendSizeBaselineEntry) error {
	previous := ""
	for index, entry := range entries {
		if entry.Rule != backendSizeFileRule && entry.Rule != backendSizeFunctionRule {
			return fmt.Errorf("backend-size baseline entry %d has unsupported rule %q", index, entry.Rule)
		}
		if strings.TrimSpace(entry.Target) == "" {
			return fmt.Errorf("backend-size baseline entry %d has an empty target", index)
		}
		if entry.Actual <= entry.Limit || entry.Limit <= 0 {
			return fmt.Errorf("backend-size baseline entry %d has invalid actual/limit %d/%d", index, entry.Actual, entry.Limit)
		}
		key := backendSizeBaselineKey(entry.Rule, entry.Target)
		if index > 0 && key <= previous {
			return fmt.Errorf("backend-size baseline entries must be sorted and unique at %q", key)
		}
		previous = key
	}
	return nil
}

func compareBackendSizeBaseline(violations []violation, baseline backendSizeBaseline) backendSizeBaselineComparison {
	recorded := make(map[string]backendSizeBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		recorded[backendSizeBaselineKey(entry.Rule, entry.Target)] = entry
	}

	comparison := backendSizeBaselineComparison{}
	active := make(map[string]struct{}, len(violations))
	for _, finding := range violations {
		entry := backendSizeBaselineEntryForViolation(finding)
		key := backendSizeBaselineKey(entry.Rule, entry.Target)
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
		if _, ok := active[backendSizeBaselineKey(entry.Rule, entry.Target)]; !ok {
			comparison.Stale = append(comparison.Stale, entry)
		}
	}
	slices.SortFunc(comparison.Stale, func(left, right backendSizeBaselineEntry) int {
		return strings.Compare(backendSizeBaselineKey(left.Rule, left.Target), backendSizeBaselineKey(right.Rule, right.Target))
	})
	return comparison
}

func backendSizeBaselineEntryForViolation(finding violation) backendSizeBaselineEntry {
	rule := backendSizeFunctionRule
	target := finding.filePath + "#" + finding.function
	if finding.function == "" {
		rule = backendSizeFileRule
		target = finding.filePath
	}
	return backendSizeBaselineEntry{Rule: rule, Target: target, Actual: finding.actual, Limit: finding.limit}
}

func backendSizeBaselineKey(rule, target string) string {
	return rule + "\x00" + target
}
