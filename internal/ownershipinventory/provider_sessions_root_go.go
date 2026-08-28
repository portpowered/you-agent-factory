package ownershipinventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	ProviderSessionsRootGoInventoryRelativePath = "docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json"

	ProviderSessionsRootGoThinContract           = "thin_root_contract"
	ProviderSessionsRootGoThinContractTest       = "thin_root_contract_test"
	ProviderSessionsRootGoFoldTargetConstruction = "fold_target_construction_port"
	ProviderSessionsRootGoFoldTargetImplTest     = "fold_target_implementation_test"
)

// ProviderSessionsThinRootContractFiles lists the committed peer-facing root
// .go sources that remain at pkg/services/provider_sessions. The list is the
// production classification source for the root-Go snapshot; it is not read
// back from the generated JSON artifact.
var ProviderSessionsThinRootContractFiles = []string{
	"contracts.go",
	"doc.go",
}

// ProviderSessionsRootContractFoldTarget names one future root contract fold
// cluster. An empty list is the current accepted policy after the completed
// Provider Sessions contract-root fold.
type ProviderSessionsRootContractFoldTarget struct {
	Cluster        string
	Files          []string
	Destination    string
	Classification string
}

var ProviderSessionsExcessRootContractFolds = []ProviderSessionsRootContractFoldTarget{}

// ClassifyProviderSessionsRootContractFile reports whether fileName belongs to
// the committed thin root contract or an explicitly classified fold target.
func ClassifyProviderSessionsRootContractFile(fileName string) (kind string, foldTarget ProviderSessionsRootContractFoldTarget, ok bool) {
	if slices.Contains(ProviderSessionsThinRootContractFiles, fileName) {
		return "thin_root_retain", ProviderSessionsRootContractFoldTarget{}, true
	}
	for _, target := range ProviderSessionsExcessRootContractFolds {
		if slices.Contains(target.Files, fileName) {
			return "excess_fold", target, true
		}
	}
	return "", ProviderSessionsRootContractFoldTarget{}, false
}

// ProviderSessionsRootContractInventory returns the stable closed inventory of
// live Provider Sessions root .go files from the production classification.
func ProviderSessionsRootContractInventory() []string {
	inventory := slices.Clone(ProviderSessionsThinRootContractFiles)
	for _, target := range ProviderSessionsExcessRootContractFolds {
		inventory = append(inventory, target.Files...)
	}
	slices.Sort(inventory)
	return inventory
}

// ProviderSessionsRootGoFile records one committed root-level .go file
// classification under pkg/services/provider_sessions/.
type ProviderSessionsRootGoFile struct {
	File            string `json:"file"`
	Classification  string `json:"classification"`
	FoldDestination string `json:"foldDestination,omitempty"`
	Note            string `json:"note,omitempty"`
}

// ProviderSessionsRootGoInventory is the INV-PSES-TOPLEVEL root .go freeze for
// Provider Sessions thin contract vs fold/consolidation targets.
type ProviderSessionsRootGoInventory struct {
	FormatVersion string                       `json:"formatVersion"`
	OwnerPackage  string                       `json:"ownerPackage"`
	SortKey       string                       `json:"sortKey"`
	Files         []ProviderSessionsRootGoFile `json:"files"`
}

// LoadProviderSessionsRootGoInventory reads the committed Provider Sessions
// root .go inventory artifact from the repository root.
func LoadProviderSessionsRootGoInventory(root string) (ProviderSessionsRootGoInventory, error) {
	path := filepath.Join(root, filepath.FromSlash(ProviderSessionsRootGoInventoryRelativePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		return ProviderSessionsRootGoInventory{}, fmt.Errorf("read provider sessions root go inventory %s: %w", ProviderSessionsRootGoInventoryRelativePath, err)
	}
	var inventory ProviderSessionsRootGoInventory
	if err := json.Unmarshal(payload, &inventory); err != nil {
		return ProviderSessionsRootGoInventory{}, fmt.Errorf("parse provider sessions root go inventory %s: %w", ProviderSessionsRootGoInventoryRelativePath, err)
	}
	return inventory, nil
}

// ListProviderSessionsRootGoFiles returns every live root-level .go file under
// pkg/services/provider_sessions/, stable-sorted by file name.
func ListProviderSessionsRootGoFiles(root string) ([]string, error) {
	serviceRoot := filepath.Join(root, filepath.FromSlash(ProviderSessionsOwnerPackagePath))
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		return nil, fmt.Errorf("read provider sessions root %s: %w", ProviderSessionsOwnerPackagePath, err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		files = append(files, name)
	}
	slices.Sort(files)
	return files, nil
}

// ProviderSessionsRootGoFoldTargets returns stable-sorted fold/consolidation
// target rows from the committed inventory for CLN-PSES-CONTRACT-ROOTS.
func ProviderSessionsRootGoFoldTargets(inventory ProviderSessionsRootGoInventory) []ProviderSessionsRootGoFile {
	var targets []ProviderSessionsRootGoFile
	for _, file := range inventory.Files {
		if !isProviderSessionsRootGoFoldTargetClassification(file.Classification) {
			continue
		}
		targets = append(targets, file)
	}
	slices.SortFunc(targets, func(left, right ProviderSessionsRootGoFile) int {
		return strings.Compare(left.File, right.File)
	})
	return targets
}

// VerifyProviderSessionsRootGoInventory proves the live filesystem matches the
// committed Provider Sessions root .go inventory rows.
func VerifyProviderSessionsRootGoInventory(root string) error {
	inventory, err := LoadProviderSessionsRootGoInventory(root)
	if err != nil {
		return err
	}
	if err := validateProviderSessionsRootGoInventory(inventory); err != nil {
		return err
	}
	live, err := ListProviderSessionsRootGoFiles(root)
	if err != nil {
		return err
	}
	committed := providerSessionsRootGoFileNames(inventory.Files)
	if !slices.Equal(live, committed) {
		return fmt.Errorf("provider sessions root .go files drift: live=%v committed=%v", live, committed)
	}
	if err := verifyProviderSessionsRootGoProductionPolicy(inventory); err != nil {
		return err
	}
	return nil
}

func verifyProviderSessionsRootGoProductionPolicy(inventory ProviderSessionsRootGoInventory) error {
	for _, file := range inventory.Files {
		kind, foldTarget, ok := ClassifyProviderSessionsRootContractFile(file.File)
		if !ok {
			return fmt.Errorf("provider sessions root go inventory file %q is unclassified by production ownership policy", file.File)
		}
		wantClassification, err := providerSessionsRootGoSnapshotClassification(file.File, kind, foldTarget)
		if err != nil {
			return err
		}
		if file.Classification != wantClassification {
			return fmt.Errorf("provider sessions root go inventory file %q classification %q disagrees with production ownership policy classification %q", file.File, file.Classification, wantClassification)
		}
		if kind == "excess_fold" && file.FoldDestination != foldTarget.Destination {
			return fmt.Errorf("provider sessions root go inventory file %q foldDestination = %q, want production policy destination %q", file.File, file.FoldDestination, foldTarget.Destination)
		}
	}
	committed := providerSessionsRootGoFileNames(inventory.Files)
	want := ProviderSessionsRootContractInventory()
	if !slices.Equal(committed, want) {
		return fmt.Errorf("provider sessions root contract inventory drift: json=%v go=%v", committed, want)
	}
	return nil
}

func validateProviderSessionsRootGoInventory(inventory ProviderSessionsRootGoInventory) error {
	if inventory.FormatVersion != "pss-provider-sessions-root-go-inventory/v1" {
		return fmt.Errorf("provider sessions root go inventory formatVersion = %q, want pss-provider-sessions-root-go-inventory/v1", inventory.FormatVersion)
	}
	if inventory.OwnerPackage != ProviderSessionsOwnerPackagePath {
		return fmt.Errorf("provider sessions root go inventory ownerPackage = %q, want %s", inventory.OwnerPackage, ProviderSessionsOwnerPackagePath)
	}
	if inventory.SortKey != rootGoFileSortKeyDescription {
		return fmt.Errorf("provider sessions root go inventory sortKey = %q, want %s", inventory.SortKey, rootGoFileSortKeyDescription)
	}
	if len(inventory.Files) == 0 {
		return fmt.Errorf("provider sessions root go inventory has no files")
	}
	seen := make(map[string]struct{}, len(inventory.Files))
	for index, file := range inventory.Files {
		if strings.TrimSpace(file.File) == "" {
			return fmt.Errorf("provider sessions root go inventory file %d has empty file name", index)
		}
		if _, duplicate := seen[file.File]; duplicate {
			return fmt.Errorf("provider sessions root go inventory duplicate file %q", file.File)
		}
		seen[file.File] = struct{}{}
		if !isProviderSessionsRootGoClassification(file.Classification) {
			return fmt.Errorf("provider sessions root go inventory file %q has unknown classification %q", file.File, file.Classification)
		}
		if isProviderSessionsRootGoFoldTargetClassification(file.Classification) {
			if strings.TrimSpace(file.FoldDestination) == "" {
				return fmt.Errorf("provider sessions root go inventory fold target %q missing foldDestination", file.File)
			}
			if !isProviderSessionsPrivateSuccessor(file.FoldDestination) {
				return fmt.Errorf("provider sessions root go inventory fold target %q destination %q outside provider_sessions private destinations", file.File, file.FoldDestination)
			}
			continue
		}
		if strings.TrimSpace(file.FoldDestination) != "" {
			return fmt.Errorf("provider sessions root go inventory thin surface %q must not set foldDestination", file.File)
		}
	}
	if !slices.IsSorted(providerSessionsRootGoFileNames(inventory.Files)) {
		return fmt.Errorf("provider sessions root go inventory files are not stable-sorted")
	}
	return nil
}

func providerSessionsRootGoFileNames(files []ProviderSessionsRootGoFile) []string {
	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.File)
	}
	return names
}

func isProviderSessionsRootGoClassification(classification string) bool {
	switch classification {
	case ProviderSessionsRootGoThinContract,
		ProviderSessionsRootGoThinContractTest,
		ProviderSessionsRootGoFoldTargetConstruction,
		ProviderSessionsRootGoFoldTargetImplTest:
		return true
	default:
		return false
	}
}

func isProviderSessionsRootGoFoldTargetClassification(classification string) bool {
	switch classification {
	case ProviderSessionsRootGoFoldTargetConstruction,
		ProviderSessionsRootGoFoldTargetImplTest:
		return true
	default:
		return false
	}
}
