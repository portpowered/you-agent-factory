package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func loadTestServiceImportBaseline(repoRoot string) (testServiceImportBaseline, error) {
	path := filepath.Join(repoRoot, testServiceImportBaselinePath)
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return testServiceImportBaseline{}, nil
		}
		return testServiceImportBaseline{}, fmt.Errorf("read test service import baseline: %w", err)
	}
	var baseline testServiceImportBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return testServiceImportBaseline{}, fmt.Errorf("decode test service import baseline: %w", err)
	}
	if baseline.Version != 1 {
		return testServiceImportBaseline{}, fmt.Errorf(
			"test service import baseline version = %d, want 1",
			baseline.Version,
		)
	}
	if err := requireNonEmptyMigrationBaseline(testServiceImportBaselinePath, len(baseline.Entries)); err != nil {
		return testServiceImportBaseline{}, err
	}
	return baseline, nil
}

func partitionTestServiceImportFindings(
	findings []testServiceImportFinding,
	baseline testServiceImportBaseline,
) ([]testServiceImportFinding, []testServiceImportBaselineEntry, error) {
	baselineByKey := make(map[string]testServiceImportBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		if err := validateTestServiceImportBaselineEntry(entry); err != nil {
			return nil, nil, err
		}
		class, err := sourceClassFromBaseline(entry.Class, entry.FilePath)
		if err != nil || class != testOnlySourceClass {
			if err != nil {
				return nil, nil, err
			}
			return nil, nil, fmt.Errorf("test service import baseline entry %s -> %s class = %q, want %q", entry.FilePath, entry.ImportPath, class, testOnlySourceClass)
		}
		key := testServiceImportKey(entry.FilePath, entry.ImportPath, class)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, fmt.Errorf(
				"duplicate test service import baseline entry: %s -> %s",
				entry.FilePath,
				entry.ImportPath,
			)
		}
		baselineByKey[key] = entry
	}

	var blocking []testServiceImportFinding
	seen := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		key := testServiceImportKey(finding.filePath, finding.importPath, finding.class)
		seen[key] = struct{}{}
		entry, recorded := baselineByKey[key]
		if !recorded {
			blocking = append(blocking, finding)
			continue
		}
		if entry.Owner != finding.owner {
			return nil, nil, fmt.Errorf(
				"test service import baseline edge %s -> %s declares owner %s; detected %s",
				entry.FilePath,
				entry.ImportPath,
				entry.Owner,
				finding.owner,
			)
		}
	}
	var stale []testServiceImportBaselineEntry
	for key, entry := range baselineByKey {
		if _, found := seen[key]; !found {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right testServiceImportBaselineEntry) int {
		if comparison := strings.Compare(left.FilePath, right.FilePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.ImportPath, right.ImportPath)
	})
	return blocking, stale, nil
}

func validateTestServiceImportBaselineEntry(entry testServiceImportBaselineEntry) error {
	if strings.TrimSpace(entry.Owner) == "" ||
		strings.TrimSpace(entry.ImportPath) == "" ||
		strings.TrimSpace(entry.FilePath) == "" ||
		strings.TrimSpace(entry.TargetRoot) == "" ||
		strings.TrimSpace(entry.Stage) == "" ||
		strings.TrimSpace(entry.DeletionGate) == "" {
		return fmt.Errorf("test service import baseline entry is incomplete: %#v", entry)
	}
	for _, value := range []string{entry.Owner, entry.ImportPath, entry.FilePath, entry.TargetRoot} {
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("test service import baseline entry must be exact and cannot contain wildcards: %#v", entry)
		}
	}
	class, err := sourceClassFromBaseline(entry.Class, entry.FilePath)
	if err != nil || class != testOnlySourceClass {
		if err != nil {
			return err
		}
		return fmt.Errorf("test service import baseline entry %s -> %s class = %q, want %q", entry.FilePath, entry.ImportPath, class, testOnlySourceClass)
	}
	if entry.Stage != testServiceImportBaselineStage {
		return fmt.Errorf(
			"test service import baseline entry %s -> %s stage = %q, want %q",
			entry.FilePath,
			entry.ImportPath,
			entry.Stage,
			testServiceImportBaselineStage,
		)
	}
	wantTarget := "pkg/services/" + entry.Owner
	if entry.TargetRoot != wantTarget {
		return fmt.Errorf(
			"test service import baseline entry %s -> %s target = %q, want %q",
			entry.FilePath,
			entry.ImportPath,
			entry.TargetRoot,
			wantTarget,
		)
	}
	if entry.DeletionGate != testServiceImportDeletionGate {
		return fmt.Errorf(
			"test service import baseline entry %s -> %s has an unrecognized deletion gate",
			entry.FilePath,
			entry.ImportPath,
		)
	}
	return nil
}

func createTestServiceImportBaseline(cfg config) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	path := filepath.Join(repoRoot, testServiceImportBaselinePath)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing test service import baseline: %s", testServiceImportBaselinePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat test service import baseline: %w", err)
	}
	findings, err := scanTestServiceSubpackageImports(repoRoot)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		return fmt.Errorf("refusing to create empty test service import baseline: no migration debt exists")
	}
	baseline := testServiceImportBaseline{
		Version: 1,
		Entries: make([]testServiceImportBaselineEntry, 0, len(findings)),
	}
	for _, finding := range findings {
		baseline.Entries = append(baseline.Entries, testServiceImportBaselineEntry{
			Owner:        finding.owner,
			ImportPath:   finding.importPath,
			FilePath:     finding.filePath,
			TargetRoot:   "pkg/services/" + finding.owner,
			Class:        string(finding.class),
			Stage:        testServiceImportBaselineStage,
			DeletionGate: testServiceImportDeletionGate,
		})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create test service import baseline directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create test service import baseline: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(baseline); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode test service import baseline: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close test service import baseline: %w", err)
	}
	fmt.Fprintf(
		stdoutWriter,
		"[agent-factory:pkg-boundary] created %s with %d deletion-only edge(s)\n",
		testServiceImportBaselinePath,
		len(baseline.Entries),
	)
	return nil
}

func testServiceImportKey(filePath, importPath string, classes ...boundarySourceClass) string {
	class := classifyBoundarySource(filePath)
	if len(classes) > 0 {
		class = effectiveBoundarySourceClass(classes[0], filePath)
	}
	return filepath.ToSlash(filePath) + "\x00" + string(class) + "\x00" + importPath
}

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

func isApprovedPeerServiceContractImport(packagePath string, importPath string) bool {
	_, approved := approvedPeerServiceContractImports[packagePath+"\x00"+importPath]
	return approved
}

func loadPeerServiceImportBaseline(repoRoot string) (peerServiceImportBaseline, error) {
	path := filepath.Join(repoRoot, peerServiceImportBaselinePath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return peerServiceImportBaseline{}, nil
		}
		return peerServiceImportBaseline{}, fmt.Errorf("read peer service import baseline: %w", err)
	}
	var baseline peerServiceImportBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return peerServiceImportBaseline{}, fmt.Errorf("decode peer service import baseline: %w", err)
	}
	if baseline.Version != 1 {
		return peerServiceImportBaseline{}, fmt.Errorf(
			"decode peer service import baseline: version %d is unsupported; want 1",
			baseline.Version,
		)
	}
	if err := requireNonEmptyMigrationBaseline(peerServiceImportBaselinePath, len(baseline.Entries)); err != nil {
		return peerServiceImportBaseline{}, err
	}
	return baseline, nil
}

func requireNonEmptyMigrationBaseline(path string, entryCount int) error {
	if entryCount == 0 {
		return fmt.Errorf("migration baseline %s is empty; delete the file to record zero debt", path)
	}
	return nil
}

func partitionPeerServiceImportFindings(
	findings []peerServiceImportFinding,
	baseline peerServiceImportBaseline,
) ([]peerServiceImportFinding, []peerServiceImportBaselineEntry, error) {
	baselineByKey := make(map[string]peerServiceImportBaselineEntry, len(baseline.Entries))
	for index, entry := range baseline.Entries {
		if err := validatePeerServiceImportBaselineEntry(entry); err != nil {
			return nil, nil, fmt.Errorf("peer service import baseline entry %d: %w", index, err)
		}
		class, err := sourceClassFromBaseline(entry.Class, entry.FilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("peer service import baseline entry %d: %w", index, err)
		}
		key := peerServiceImportKey(entry.FilePath, entry.ImportPath, class)
		if _, exists := baselineByKey[key]; exists {
			return nil, nil, fmt.Errorf(
				"peer service import baseline entry %d: duplicate file/import edge %s -> %s",
				index,
				entry.FilePath,
				entry.ImportPath,
			)
		}
		baselineByKey[key] = entry
	}

	remaining := make(map[string]struct{}, len(findings))
	var violations []peerServiceImportFinding
	for _, finding := range findings {
		key := peerServiceImportKey(finding.filePath, finding.importPath, finding.class)
		entry, approved := baselineByKey[key]
		if !approved {
			violations = append(violations, finding)
			continue
		}
		if entry.Owner != finding.owner || entry.Peer != finding.peer {
			return nil, nil, fmt.Errorf(
				"peer service import baseline edge %s -> %s declares owner/peer %s/%s; detected %s/%s",
				entry.FilePath,
				entry.ImportPath,
				entry.Owner,
				entry.Peer,
				finding.owner,
				finding.peer,
			)
		}
		remaining[key] = struct{}{}
	}

	var stale []peerServiceImportBaselineEntry
	for key, entry := range baselineByKey {
		if _, exists := remaining[key]; !exists {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right peerServiceImportBaselineEntry) int {
		if comparison := strings.Compare(left.FilePath, right.FilePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.ImportPath, right.ImportPath)
	})
	return violations, stale, nil
}

func validatePeerServiceImportBaselineEntry(entry peerServiceImportBaselineEntry) error {
	for field, value := range map[string]string{
		"owner": entry.Owner, "peer": entry.Peer, "importPath": entry.ImportPath,
		"filePath": entry.FilePath, "targetRoot": entry.TargetRoot, "stage": entry.Stage,
		"deletionGate": entry.DeletionGate,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field)
		}
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("%s must be exact and cannot contain wildcards", field)
		}
	}
	if _, err := sourceClassFromBaseline(entry.Class, entry.FilePath); err != nil {
		return err
	}
	if entry.Stage != peerServiceImportBaselineStage || entry.DeletionGate != peerServiceImportDeletionGate {
		return fmt.Errorf("stage or deletionGate is not the recognized peer-service migration contract")
	}
	if entry.Owner == entry.Peer {
		return fmt.Errorf("owner and peer must differ")
	}
	expectedImportPrefix := repositoryImportPrefix + "pkg/services/" + entry.Peer + "/"
	if !strings.HasPrefix(entry.ImportPath, expectedImportPrefix) {
		return fmt.Errorf("importPath %q is not a subpackage of peer %q", entry.ImportPath, entry.Peer)
	}
	expectedTargetRoot := "pkg/services/" + entry.Peer
	if entry.TargetRoot != expectedTargetRoot {
		return fmt.Errorf("targetRoot %q must be %q", entry.TargetRoot, expectedTargetRoot)
	}
	expectedOwnerPrefix := "pkg/services/" + entry.Owner
	if entry.FilePath != expectedOwnerPrefix && !strings.HasPrefix(entry.FilePath, expectedOwnerPrefix+"/") {
		return fmt.Errorf("filePath %q is not owned by %q", entry.FilePath, entry.Owner)
	}
	return nil
}

func peerServiceImportKey(filePath, importPath string, classes ...boundarySourceClass) string {
	class := classifyBoundarySource(filePath)
	if len(classes) > 0 {
		class = effectiveBoundarySourceClass(classes[0], filePath)
	}
	return filepath.ToSlash(filePath) + "\x00" + string(class) + "\x00" + importPath
}
