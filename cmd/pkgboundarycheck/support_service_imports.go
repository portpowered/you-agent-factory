package main

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type supportServiceImportFinding struct {
	owner      string
	importPath string
	filePath   string
}

type supportServiceImportBaseline struct {
	Version int                                 `json:"version"`
	Entries []supportServiceImportBaselineEntry `json:"entries"`
}

type supportServiceImportBaselineEntry struct {
	Owner        string `json:"owner"`
	ImportPath   string `json:"importPath"`
	FilePath     string `json:"filePath"`
	TargetRoot   string `json:"targetRoot"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

func scanSupportServiceSubpackageImports(repoRoot string) ([]supportServiceImportFinding, error) {
	findingsByKey := map[string]supportServiceImportFinding{}
	supportRoots := []string{
		filepath.Join(repoRoot, "internal", "configcontractsmoke"),
		filepath.Join(repoRoot, "internal", "testutil"),
		filepath.Join(repoRoot, "tests", "functional", "internal", "support"),
	}
	for _, supportRoot := range supportRoots {
		err := scanSupportRootServiceSubpackageImports(
			repoRoot,
			supportRoot,
			findingsByKey,
		)
		if err != nil {
			return nil, err
		}
	}
	findings := make([]supportServiceImportFinding, 0, len(findingsByKey))
	for _, finding := range findingsByKey {
		findings = append(findings, finding)
	}
	slices.SortFunc(findings, func(left, right supportServiceImportFinding) int {
		if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.importPath, right.importPath)
	})
	return findings, nil
}

func scanSupportRootServiceSubpackageImports(
	repoRoot string,
	supportRoot string,
	findingsByKey map[string]supportServiceImportFinding,
) error {
	err := filepath.WalkDir(supportRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "testdata", "vendor":
				if path != supportRoot {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
		parsedFile, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, importSpec := range parsedFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			repositoryPath := strings.TrimPrefix(importPath, repositoryImportPrefix)
			importedOwner, isServiceSubpackage := serviceSubpackageOwner(repositoryPath)
			if importPath == applicationGraphImportPath || strings.HasPrefix(importPath, applicationGraphImportPath+"/") {
				importedOwner, isServiceSubpackage = "wire", true
			}
			if !isServiceSubpackage {
				continue
			}
			if _, publicEffectContract := publicExternalEffectContractImports[importPath]; publicEffectContract {
				continue
			}
			finding := supportServiceImportFinding{
				owner:      importedOwner,
				importPath: importPath,
				filePath:   filePath,
			}
			findingsByKey[supportServiceImportKey(filePath, importPath)] = finding
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("scan reusable support service subpackage imports: %w", err)
	}
	return nil
}

func loadSupportServiceImportBaseline(repoRoot string) (supportServiceImportBaseline, error) {
	payload, err := os.ReadFile(filepath.Join(repoRoot, supportServiceImportBaselinePath))
	if err != nil {
		if os.IsNotExist(err) {
			return supportServiceImportBaseline{}, nil
		}
		return supportServiceImportBaseline{}, fmt.Errorf("read reusable support service import baseline: %w", err)
	}
	var baseline supportServiceImportBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return supportServiceImportBaseline{}, fmt.Errorf("decode reusable support service import baseline: %w", err)
	}
	if baseline.Version != 1 {
		return supportServiceImportBaseline{}, fmt.Errorf(
			"reusable support service import baseline version = %d, want 1",
			baseline.Version,
		)
	}
	if err := requireNonEmptyMigrationBaseline(supportServiceImportBaselinePath, len(baseline.Entries)); err != nil {
		return supportServiceImportBaseline{}, err
	}
	return baseline, nil
}

func partitionSupportServiceImportFindings(
	findings []supportServiceImportFinding,
	baseline supportServiceImportBaseline,
) ([]supportServiceImportFinding, []supportServiceImportBaselineEntry, error) {
	baselineByKey := make(map[string]supportServiceImportBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		if err := validateSupportServiceImportBaselineEntry(entry); err != nil {
			return nil, nil, err
		}
		key := supportServiceImportKey(entry.FilePath, entry.ImportPath)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, fmt.Errorf(
				"duplicate reusable support service import baseline entry: %s -> %s",
				entry.FilePath,
				entry.ImportPath,
			)
		}
		baselineByKey[key] = entry
	}

	var blocking []supportServiceImportFinding
	seen := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		key := supportServiceImportKey(finding.filePath, finding.importPath)
		seen[key] = struct{}{}
		entry, recorded := baselineByKey[key]
		if !recorded {
			blocking = append(blocking, finding)
			continue
		}
		if entry.Owner != finding.owner {
			return nil, nil, fmt.Errorf(
				"reusable support service import baseline edge %s -> %s declares owner %s; detected %s",
				entry.FilePath,
				entry.ImportPath,
				entry.Owner,
				finding.owner,
			)
		}
	}
	var stale []supportServiceImportBaselineEntry
	for key, entry := range baselineByKey {
		if _, found := seen[key]; !found {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right supportServiceImportBaselineEntry) int {
		if comparison := strings.Compare(left.FilePath, right.FilePath); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.ImportPath, right.ImportPath)
	})
	return blocking, stale, nil
}

func validateSupportServiceImportBaselineEntry(entry supportServiceImportBaselineEntry) error {
	if strings.TrimSpace(entry.Owner) == "" ||
		strings.TrimSpace(entry.ImportPath) == "" ||
		strings.TrimSpace(entry.FilePath) == "" {
		return fmt.Errorf("reusable support service import baseline entry requires owner, importPath, and filePath")
	}
	for _, value := range []string{entry.Owner, entry.ImportPath, entry.FilePath, entry.TargetRoot} {
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("reusable support service import baseline entry must be exact and cannot contain wildcards: %#v", entry)
		}
	}
	wantTargetRoot := "pkg/services/" + entry.Owner
	if entry.Owner == "wire" {
		wantTargetRoot = "pkg/wire"
	}
	if entry.TargetRoot != wantTargetRoot {
		return fmt.Errorf(
			"reusable support service import baseline entry %s -> %s targetRoot = %q, want %q",
			entry.FilePath,
			entry.ImportPath,
			entry.TargetRoot,
			wantTargetRoot,
		)
	}
	if entry.Stage != supportServiceImportBaselineStage {
		return fmt.Errorf(
			"reusable support service import baseline entry %s -> %s stage = %q, want %q",
			entry.FilePath,
			entry.ImportPath,
			entry.Stage,
			supportServiceImportBaselineStage,
		)
	}
	if entry.DeletionGate != supportServiceImportDeletionGate {
		return fmt.Errorf(
			"reusable support service import baseline entry %s -> %s has an invalid deletion gate",
			entry.FilePath,
			entry.ImportPath,
		)
	}
	return nil
}

func createSupportServiceImportBaseline(cfg config) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	path := filepath.Join(repoRoot, supportServiceImportBaselinePath)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing reusable support service import baseline: %s", supportServiceImportBaselinePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat reusable support service import baseline: %w", err)
	}
	findings, err := scanSupportServiceSubpackageImports(repoRoot)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		return fmt.Errorf("refusing to create empty reusable support service import baseline: no migration debt exists")
	}
	baseline := supportServiceImportBaseline{
		Version: 1,
		Entries: make([]supportServiceImportBaselineEntry, 0, len(findings)),
	}
	for _, finding := range findings {
		baseline.Entries = append(baseline.Entries, supportServiceImportBaselineEntry{
			Owner:      finding.owner,
			ImportPath: finding.importPath,
			FilePath:   finding.filePath,
			TargetRoot: func() string {
				if finding.owner == "wire" {
					return "pkg/wire"
				}
				return "pkg/services/" + finding.owner
			}(),
			Stage:        supportServiceImportBaselineStage,
			DeletionGate: supportServiceImportDeletionGate,
		})
	}
	payload, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("encode reusable support service import baseline: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write reusable support service import baseline: %w", err)
	}
	fmt.Fprintf(stdoutWriter, "[agent-factory:pkg-boundary] created %s with %d deletion-only edge(s)\n", supportServiceImportBaselinePath, len(baseline.Entries))
	return nil
}

func supportServiceImportKey(filePath, importPath string) string {
	return filePath + "\x00" + importPath
}

func writeSupportServiceImportFindings(writer io.Writer, findings []supportServiceImportFinding) {
	for _, finding := range findings {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] prohibited reusable support service implementation import: %s (%s)\n",
			finding.importPath,
			finding.filePath,
		)
		fmt.Fprintln(writer, "  reason: repository-wide reusable test support is not an application composition root.")
		fmt.Fprintln(writer, "  remediation: use the owning service root contract, a typed edge fake, package-local owner coverage, or root.BuildProcess.")
	}
}

func writeStaleSupportServiceBaselineEntries(writer io.Writer, entries []supportServiceImportBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(
			writer,
			"[agent-factory:pkg-boundary] stale reusable support service import baseline entry: %s -> %s\n",
			entry.FilePath,
			entry.ImportPath,
		)
		fmt.Fprintln(writer, "  reason: the concrete reusable-support import no longer exists.")
		fmt.Fprintf(writer, "  remediation: remove this entry from %s in the same change.\n", supportServiceImportBaselinePath)
	}
}

func writeSupportServiceBaselineSummary(writer io.Writer, count int) {
	if count == 0 {
		return
	}
	fmt.Fprintf(writer, "[agent-factory:pkg-boundary] active reusable support service-import migration baseline: %d edge(s)\n", count)
	fmt.Fprintln(writer, "  deletion gate: replace each helper dependency with a service-root contract, typed edge fake, owner-local fixture, or root.BuildProcess; then delete the exact baseline entry.")
}
