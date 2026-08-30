package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strings"
)

func loadBaseline(path string) (baselineDocument, error) {
	data, err := readJSONFile(path, "baseline")
	if err != nil {
		return baselineDocument{}, err
	}
	var baseline baselineDocument
	if err := decodeJSON(data, &baseline); err != nil {
		return baselineDocument{}, fmt.Errorf("parse baseline %s: %w", filepath.ToSlash(path), err)
	}
	findings := validateBaseline(baseline)
	if len(findings) > 0 {
		return baselineDocument{}, formatValidationError("baseline", findings)
	}
	return baseline, nil
}

func loadInventory(path string) (inventoryDocument, error) {
	data, err := readJSONFile(path, "inventory")
	if err != nil {
		return inventoryDocument{}, err
	}
	var inventory inventoryDocument
	if err := decodeJSON(data, &inventory); err != nil {
		return inventoryDocument{}, fmt.Errorf("parse inventory %s: %w", filepath.ToSlash(path), err)
	}
	findings := validateInventory(inventory)
	if len(findings) > 0 {
		return inventoryDocument{}, formatValidationError("inventory", findings)
	}
	return inventory, nil
}

func readJSONFile(path, noun string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s %s: %w", noun, filepath.ToSlash(path), err)
	}
	return data, nil
}

func decodeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("contains more than one JSON value")
		}
		return fmt.Errorf("contains trailing JSON: %w", err)
	}
	return nil
}

func validateBaseline(baseline baselineDocument) []string {
	var findings []string
	if baseline.Version != baselineFormatVersion {
		findings = append(findings, fmt.Sprintf("version must be %d, got %d", baselineFormatVersion, baseline.Version))
	}
	if baseline.CountUnit != spawnCountUnit {
		findings = append(findings, fmt.Sprintf("countUnit must be %q, got %q", spawnCountUnit, baseline.CountUnit))
	}
	if baseline.Packages == nil {
		findings = append(findings, "packages must be an array")
		return findings
	}
	previous := ""
	for index, row := range baseline.Packages {
		label := fmt.Sprintf("packages[%d]", index)
		if err := validatePackagePath(row.PackagePath); err != nil {
			findings = append(findings, fmt.Sprintf("%s: %v", label, err))
		}
		if row.PackagePath <= previous && row.PackagePath != "" {
			findings = append(findings, fmt.Sprintf("%s packagePath must be unique and sorted", label))
		}
		if row.Count < 0 {
			findings = append(findings, fmt.Sprintf("%s count must be non-negative", label))
		}
		previous = row.PackagePath
	}
	return findings
}

func validateInventory(inventory inventoryDocument) []string {
	var findings []string
	if inventory.FormatVersion != inventoryFormatVersion {
		findings = append(findings, fmt.Sprintf("formatVersion must be %d, got %d", inventoryFormatVersion, inventory.FormatVersion))
	}
	if inventory.TestRows == nil {
		findings = append(findings, "testRows must be an array")
	}
	if inventory.OSSpawnSites == nil {
		findings = append(findings, "osSpawnSites must be an array")
	}
	findings = append(findings, validateTestRows(inventory.TestRows)...)
	findings = append(findings, validateSpawnRecords(inventory.OSSpawnSites)...)
	return findings
}

func validateTestRows(rows []inventoryTestRow) []string {
	var findings []string
	for index, row := range rows {
		switch row.Classification {
		case "shareable", "shareable-with-mock":
			if row.OSBoundaryIntentionality != nil {
				findings = append(findings, fmt.Sprintf("testRows[%d] non-isolated row must not carry osBoundaryIntentionality", index))
			}
		case "isolated-with-reason":
			label := fmt.Sprintf("testRows[%d]", index)
			findings = append(findings, validateIntentionality(row.OSBoundaryIntentionality, label)...)
		default:
			findings = append(findings, fmt.Sprintf("testRows[%d] has unknown classification %q", index, row.Classification))
		}
	}
	return findings
}

func validateSpawnRecords(sites []inventorySpawnSite) []string {
	var findings []string
	seen := map[string]struct{}{}
	for index, site := range sites {
		label := fmt.Sprintf("osSpawnSites[%d]", index)
		if site.SiteID == "" {
			findings = append(findings, fmt.Sprintf("%s siteId must not be empty", label))
		} else if _, exists := seen[site.SiteID]; exists {
			findings = append(findings, fmt.Sprintf("%s duplicates siteId %q", label, site.SiteID))
		} else {
			seen[site.SiteID] = struct{}{}
		}
		if err := validatePackagePath(site.PackagePath); err != nil {
			findings = append(findings, fmt.Sprintf("%s packagePath: %v", label, err))
		}
		if err := validateSourcePath(site.SourcePath); err != nil {
			findings = append(findings, fmt.Sprintf("%s sourcePath: %v", label, err))
		}
		if site.SourceLine <= 0 {
			findings = append(findings, fmt.Sprintf("%s sourceLine must be positive", label))
		}
		if strings.TrimSpace(site.EnclosingIdentity) == "" {
			findings = append(findings, fmt.Sprintf("%s enclosingIdentity must not be empty", label))
		}
		if strings.TrimSpace(site.LauncherKind) == "" {
			findings = append(findings, fmt.Sprintf("%s launcherKind must not be empty", label))
		}
		if site.Occurrence <= 0 {
			findings = append(findings, fmt.Sprintf("%s occurrence must be positive", label))
		}
		findings = append(findings, validateVerdictFields(site.Verdict, site.RequiredProperty, site.AssertionEvidence, site.ConversionObligation, label)...)
	}
	return findings
}

func validatePackagePath(path string) error {
	if err := validateFunctionalPath(path); err != nil {
		return fmt.Errorf("must be a slash-normalized path under tests/functional")
	}
	return nil
}

func validateSourcePath(path string) error {
	if err := validateFunctionalPath(path); err != nil || !strings.HasSuffix(path, ".go") {
		return fmt.Errorf("must be a slash-normalized Go path under tests/functional")
	}
	return nil
}

func validateFunctionalPath(value string) error {
	if value == "" || strings.Contains(value, "\\") || !strings.HasPrefix(value, "tests/functional/") {
		return fmt.Errorf("invalid path")
	}
	if strings.Contains(value, "//") || pathpkg.Clean(value) != value {
		return fmt.Errorf("invalid path")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid path")
		}
	}
	return nil
}

func formatValidationError(noun string, findings []string) error {
	slices.Sort(findings)
	return fmt.Errorf("[agent-factory:functional-os-boundary] %s validation failed: %s", noun, strings.Join(findings, "; "))
}

func validationFindings(err error) []string {
	if err == nil {
		return nil
	}
	lines := strings.Split(fmt.Sprintf("[agent-factory:functional-os-boundary] %v", err), "\n")
	for index := range lines {
		lines[index] = strings.TrimSpace(lines[index])
	}
	slices.Sort(lines)
	return lines
}
