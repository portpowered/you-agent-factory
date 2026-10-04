package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func loadServiceConstructionBaseline(repoRoot string) (serviceConstructionBaseline, error) {
	payload, err := os.ReadFile(filepath.Join(repoRoot, serviceConstructionBaselinePath))
	if err != nil {
		if os.IsNotExist(err) {
			return serviceConstructionBaseline{}, nil
		}
		return serviceConstructionBaseline{}, fmt.Errorf("read service construction baseline: %w", err)
	}
	var baseline serviceConstructionBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return serviceConstructionBaseline{}, fmt.Errorf("decode service construction baseline: %w", err)
	}
	if baseline.Version != 1 {
		return serviceConstructionBaseline{}, fmt.Errorf("service construction baseline version = %d, want 1", baseline.Version)
	}
	if err := requireNonEmptyMigrationBaseline(serviceConstructionBaselinePath, len(baseline.Entries)); err != nil {
		return serviceConstructionBaseline{}, err
	}
	return baseline, nil
}

func partitionServiceConstructionFindings(
	findings []serviceConstructionFinding,
	baseline serviceConstructionBaseline,
) ([]serviceConstructionFinding, []serviceConstructionBaselineEntry, error) {
	baselineByKey := make(map[string]serviceConstructionBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		if err := validateServiceConstructionBaselineEntry(entry); err != nil {
			return nil, nil, err
		}
		class, err := sourceClassFromBaseline(entry.Class, entry.FilePath)
		if err != nil {
			return nil, nil, err
		}
		key := serviceConstructionKey(entry.FilePath, entry.ImportPath, entry.Symbol, class)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, fmt.Errorf("duplicate service construction baseline entry: %s -> %s.%s", entry.FilePath, entry.ImportPath, entry.Symbol)
		}
		baselineByKey[key] = entry
	}
	var blocking []serviceConstructionFinding
	seen := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		key := serviceConstructionKey(finding.filePath, finding.importPath, finding.symbol, finding.class)
		seen[key] = struct{}{}
		entry, recorded := baselineByKey[key]
		if !recorded || entry.Count != finding.count {
			blocking = append(blocking, finding)
			continue
		}
		if entry.Owner != finding.owner {
			return nil, nil, fmt.Errorf("service construction baseline %s -> %s.%s declares owner %s; detected %s", entry.FilePath, entry.ImportPath, entry.Symbol, entry.Owner, finding.owner)
		}
	}
	var stale []serviceConstructionBaselineEntry
	for key, entry := range baselineByKey {
		if _, found := seen[key]; !found {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right serviceConstructionBaselineEntry) int {
		if comparison := strings.Compare(left.FilePath, right.FilePath); comparison != 0 {
			return comparison
		}
		if comparison := strings.Compare(left.ImportPath, right.ImportPath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.Symbol, right.Symbol)
	})
	return blocking, stale, nil
}

func validateServiceConstructionBaselineEntry(entry serviceConstructionBaselineEntry) error {
	if strings.TrimSpace(entry.Owner) == "" ||
		strings.TrimSpace(entry.ImportPath) == "" ||
		strings.TrimSpace(entry.Symbol) == "" ||
		strings.TrimSpace(entry.FilePath) == "" ||
		strings.TrimSpace(entry.Stage) == "" ||
		strings.TrimSpace(entry.DeletionGate) == "" ||
		entry.Count < 1 {
		return fmt.Errorf("service construction baseline entry is incomplete: %#v", entry)
	}
	for _, value := range []string{entry.Owner, entry.ImportPath, entry.Symbol, entry.FilePath} {
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("service construction baseline entry must be exact and cannot contain wildcards: %#v", entry)
		}
	}
	if _, err := sourceClassFromBaseline(entry.Class, entry.FilePath); err != nil {
		return err
	}
	wantOwner, servicePackage := serviceRootOwner(entry.ImportPath)
	if !servicePackage {
		repositoryPath := strings.TrimPrefix(entry.ImportPath, repositoryImportPrefix)
		wantOwner, servicePackage = serviceSubpackageOwner(repositoryPath)
	}
	if !servicePackage {
		return fmt.Errorf("service construction baseline entry %s names a non-service import %s", entry.FilePath, entry.ImportPath)
	}
	if !isProhibitedServiceConstructionSymbol(entry.ImportPath, entry.Symbol) {
		return fmt.Errorf("service construction baseline entry %s names an allowed or non-construction symbol %s.%s", entry.FilePath, entry.ImportPath, entry.Symbol)
	}
	if entry.Owner != wantOwner {
		return fmt.Errorf("service construction baseline entry %s owner = %q, want %q", entry.FilePath, entry.Owner, wantOwner)
	}
	if entry.Stage != serviceConstructionBaselineStage || entry.DeletionGate != serviceConstructionDeletionGate {
		return fmt.Errorf("service construction baseline entry %s has an unrecognized migration stage or deletion gate", entry.FilePath)
	}
	return nil
}

func serviceConstructionKey(filePath, importPath, symbol string, classes ...boundarySourceClass) string {
	class := classifyBoundarySource(filePath)
	if len(classes) > 0 {
		class = effectiveBoundarySourceClass(classes[0], filePath)
	}
	return filepath.ToSlash(filePath) + "\x00" + string(class) + "\x00" + importPath + "\x00" + symbol
}

func requireNonEmptyMigrationBaseline(path string, entryCount int) error {
	if entryCount == 0 {
		return fmt.Errorf("migration baseline %s is empty; delete the file to record zero debt", path)
	}
	return nil
}

func isApprovedPeerServiceContractImport(packagePath string, importPath string) bool {
	_, approved := approvedPeerServiceContractImports[packagePath+"\x00"+importPath]
	return approved
}
