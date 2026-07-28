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

	ProviderSessionsRootGoThinContract            = "thin_root_contract"
	ProviderSessionsRootGoThinContractTest      = "thin_root_contract_test"
	ProviderSessionsRootGoFoldTargetConstruction  = "fold_target_construction_port"
	ProviderSessionsRootGoFoldTargetImplTest      = "fold_target_implementation_test"
)

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
	FormatVersion string                     `json:"formatVersion"`
	OwnerPackage  string                     `json:"ownerPackage"`
	SortKey       string                     `json:"sortKey"`
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
	return nil
}

func validateProviderSessionsRootGoInventory(inventory ProviderSessionsRootGoInventory) error {
	if inventory.FormatVersion != "pss-provider-sessions-root-go-inventory/v1" {
		return fmt.Errorf("provider sessions root go inventory formatVersion = %q, want pss-provider-sessions-root-go-inventory/v1", inventory.FormatVersion)
	}
	if inventory.OwnerPackage != ProviderSessionsOwnerPackagePath {
		return fmt.Errorf("provider sessions root go inventory ownerPackage = %q, want %s", inventory.OwnerPackage, ProviderSessionsOwnerPackagePath)
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
