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
	OperatorSettingsTopLevelInventoryRelativePath = "docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json"
	OperatorSettingsOwnerPackagePath              = "pkg/services/operator_settings"

	OperatorSettingsTopLevelCanonicalRetain         = "canonical_retain"
	OperatorSettingsTopLevelTestOnlyRetain          = "test_only_retain"
	OperatorSettingsTopLevelUnexpectedPublicSibling = "unexpected_public_sibling"
)

// OperatorSettingsTopLevelChild records one committed top-level directory
// classification under pkg/services/operator_settings/.
type OperatorSettingsTopLevelChild struct {
	Directory      string `json:"directory"`
	Classification string `json:"classification"`
	Note           string `json:"note,omitempty"`
}

// OperatorSettingsTopLevelInventory is the INV-SET-TOPLEVEL live-child freeze
// for Operator Settings top-level directories.
type OperatorSettingsTopLevelInventory struct {
	FormatVersion            string                          `json:"formatVersion"`
	OwnerPackage             string                          `json:"ownerPackage"`
	SortKey                  string                          `json:"sortKey"`
	UnexpectedPublicSiblings []string                        `json:"unexpectedPublicSiblings"`
	Children                 []OperatorSettingsTopLevelChild `json:"children"`
}

// LoadOperatorSettingsTopLevelInventory reads the committed Operator Settings
// top-level inventory artifact from the repository root.
func LoadOperatorSettingsTopLevelInventory(root string) (OperatorSettingsTopLevelInventory, error) {
	path := filepath.Join(root, filepath.FromSlash(OperatorSettingsTopLevelInventoryRelativePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		return OperatorSettingsTopLevelInventory{}, fmt.Errorf("read operator settings top-level inventory %s: %w", OperatorSettingsTopLevelInventoryRelativePath, err)
	}
	var inventory OperatorSettingsTopLevelInventory
	if err := json.Unmarshal(payload, &inventory); err != nil {
		return OperatorSettingsTopLevelInventory{}, fmt.Errorf("parse operator settings top-level inventory %s: %w", OperatorSettingsTopLevelInventoryRelativePath, err)
	}
	return inventory, nil
}

// ListOperatorSettingsTopLevelDirectories returns every live top-level directory
// under pkg/services/operator_settings/, stable-sorted by directory name.
func ListOperatorSettingsTopLevelDirectories(root string) ([]string, error) {
	serviceRoot := filepath.Join(root, filepath.FromSlash(OperatorSettingsOwnerPackagePath))
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		return nil, fmt.Errorf("read operator settings root %s: %w", OperatorSettingsOwnerPackagePath, err)
	}
	var directories []string
	for _, entry := range entries {
		if !entry.IsDir() || ignoredOperatorSettingsTopLevelDirectory(entry.Name()) {
			continue
		}
		directories = append(directories, entry.Name())
	}
	slices.Sort(directories)
	return directories, nil
}

// OperatorSettingsUnexpectedPublicSiblingPackagePaths returns stable-sorted
// production package paths for every unexpected public top-level sibling listed
// in the committed Operator Settings top-level inventory.
func OperatorSettingsUnexpectedPublicSiblingPackagePaths(inventory OperatorSettingsTopLevelInventory) []string {
	var packagePaths []string
	for _, child := range inventory.Children {
		if child.Classification != OperatorSettingsTopLevelUnexpectedPublicSibling {
			continue
		}
		packagePaths = append(packagePaths, OperatorSettingsOwnerPackagePath+"/"+child.Directory)
	}
	slices.Sort(packagePaths)
	return packagePaths
}

// VerifyOperatorSettingsUnexpectedPublicSiblingRemaps proves live top-level
// children match the committed inventory and every unexpected public sibling
// is remapped move/delete with an explicit private destination rather than
// retain→operator_settings.
func VerifyOperatorSettingsUnexpectedPublicSiblingRemaps(root string) error {
	if err := VerifyOperatorSettingsTopLevelInventory(root); err != nil {
		return err
	}
	inventory, err := LoadOperatorSettingsTopLevelInventory(root)
	if err != nil {
		return err
	}
	for _, packagePath := range OperatorSettingsUnexpectedPublicSiblingPackagePaths(inventory) {
		row, err := MapPackage(packagePath)
		if err != nil {
			return fmt.Errorf("map unexpected public sibling %q: %w", packagePath, err)
		}
		if err := validateOperatorSettingsUnexpectedPublicSiblingRow(packagePath, row); err != nil {
			return err
		}
	}
	return nil
}

func validateOperatorSettingsUnexpectedPublicSiblingRow(packagePath string, row PackageRow) error {
	if row.Disposition == DispositionRetain && row.Destination == operatorSettingsOwner {
		return fmt.Errorf("unexpected public sibling %q retains under owner root", packagePath)
	}
	switch row.Disposition {
	case DispositionMove, DispositionDelete:
	default:
		return fmt.Errorf("unexpected public sibling %q disposition %q must be move or delete", packagePath, row.Disposition)
	}
	if row.Disposition == DispositionMove {
		if strings.TrimSpace(row.Successor) == "" || strings.TrimSpace(row.DeletionCondition) == "" {
			return fmt.Errorf("unexpected public sibling %q missing successor/deletionCondition", packagePath)
		}
		if !isOperatorSettingsPrivateSuccessor(row.Successor) {
			return fmt.Errorf("unexpected public sibling %q successor %q outside operator_settings private destinations", packagePath, row.Successor)
		}
	}
	return nil
}

// VerifyOperatorSettingsTopLevelInventory proves the live filesystem matches the
// committed Operator Settings top-level inventory rows.
func VerifyOperatorSettingsTopLevelInventory(root string) error {
	inventory, err := LoadOperatorSettingsTopLevelInventory(root)
	if err != nil {
		return err
	}
	if err := validateOperatorSettingsTopLevelInventory(inventory); err != nil {
		return err
	}
	live, err := ListOperatorSettingsTopLevelDirectories(root)
	if err != nil {
		return err
	}
	committed := operatorSettingsTopLevelDirectoryNames(inventory.Children)
	if !slices.Equal(live, committed) {
		return fmt.Errorf("operator settings top-level directories drift: live=%v committed=%v", live, committed)
	}
	return nil
}

func validateOperatorSettingsTopLevelInventory(inventory OperatorSettingsTopLevelInventory) error {
	if inventory.FormatVersion != "pss-operator-settings-top-level-inventory/v1" {
		return fmt.Errorf("operator settings top-level inventory formatVersion = %q, want pss-operator-settings-top-level-inventory/v1", inventory.FormatVersion)
	}
	if inventory.OwnerPackage != OperatorSettingsOwnerPackagePath {
		return fmt.Errorf("operator settings top-level inventory ownerPackage = %q, want %s", inventory.OwnerPackage, OperatorSettingsOwnerPackagePath)
	}
	if inventory.SortKey != topLevelDirectorySortKeyDescription {
		return fmt.Errorf("operator settings top-level inventory sortKey = %q, want %s", inventory.SortKey, topLevelDirectorySortKeyDescription)
	}
	if len(inventory.Children) == 0 {
		return fmt.Errorf("operator settings top-level inventory has no children")
	}
	seen := make(map[string]struct{}, len(inventory.Children))
	var unexpectedPublicSiblings []string
	for index, child := range inventory.Children {
		if strings.TrimSpace(child.Directory) == "" {
			return fmt.Errorf("operator settings top-level inventory child %d has empty directory", index)
		}
		if _, duplicate := seen[child.Directory]; duplicate {
			return fmt.Errorf("operator settings top-level inventory duplicate directory %q", child.Directory)
		}
		seen[child.Directory] = struct{}{}
		if !isOperatorSettingsTopLevelClassification(child.Classification) {
			return fmt.Errorf("operator settings top-level inventory child %q has unknown classification %q", child.Directory, child.Classification)
		}
		if child.Classification == OperatorSettingsTopLevelTestOnlyRetain && strings.TrimSpace(child.Note) == "" {
			return fmt.Errorf("operator settings top-level inventory child %q requires an INV note for test_only_retain", child.Directory)
		}
		if child.Classification == OperatorSettingsTopLevelUnexpectedPublicSibling {
			unexpectedPublicSiblings = append(unexpectedPublicSiblings, child.Directory)
		}
	}
	if !slices.IsSorted(operatorSettingsTopLevelDirectoryNames(inventory.Children)) {
		return fmt.Errorf("operator settings top-level inventory children are not stable-sorted")
	}
	if !slices.Equal(inventory.UnexpectedPublicSiblings, unexpectedPublicSiblings) {
		return fmt.Errorf("operator settings top-level inventory unexpectedPublicSiblings = %v, want %v", inventory.UnexpectedPublicSiblings, unexpectedPublicSiblings)
	}
	return nil
}

func operatorSettingsTopLevelDirectoryNames(children []OperatorSettingsTopLevelChild) []string {
	names := make([]string, 0, len(children))
	for _, child := range children {
		names = append(names, child.Directory)
	}
	return names
}

func isOperatorSettingsTopLevelClassification(classification string) bool {
	switch classification {
	case OperatorSettingsTopLevelCanonicalRetain,
		OperatorSettingsTopLevelTestOnlyRetain,
		OperatorSettingsTopLevelUnexpectedPublicSibling:
		return true
	default:
		return false
	}
}

func ignoredOperatorSettingsTopLevelDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	default:
		return false
	}
}
