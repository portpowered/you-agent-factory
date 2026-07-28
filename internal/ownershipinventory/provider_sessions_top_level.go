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
	ProviderSessionsTopLevelInventoryRelativePath = "docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json"
	ProviderSessionsOwnerPackagePath              = "pkg/services/provider_sessions"

	ProviderSessionsTopLevelCanonicalRetain            = "canonical_retain"
	ProviderSessionsTopLevelUnexpectedPublicSibling    = "unexpected_public_sibling"
	ProviderSessionsTopLevelINVUnexpectedPublicSibling = "inv_unexpected_public_sibling"
)

// ProviderSessionsTopLevelChild records one committed top-level directory
// classification under pkg/services/provider_sessions/.
type ProviderSessionsTopLevelChild struct {
	Directory      string `json:"directory"`
	Classification string `json:"classification"`
}

// ProviderSessionsTopLevelInventory is the INV-PSES-TOPLEVEL live-child freeze
// for Provider Sessions top-level directories.
type ProviderSessionsTopLevelInventory struct {
	FormatVersion                            string                          `json:"formatVersion"`
	OwnerPackage                             string                          `json:"ownerPackage"`
	SortKey                                  string                          `json:"sortKey"`
	HasUnexpectedPublicSiblingsBeyondService bool                            `json:"hasUnexpectedPublicSiblingsBeyondService"`
	UnexpectedPublicSiblingsBeyondService    []string                        `json:"unexpectedPublicSiblingsBeyondService"`
	Children                                 []ProviderSessionsTopLevelChild `json:"children"`
}

// LoadProviderSessionsTopLevelInventory reads the committed Provider Sessions
// top-level inventory artifact from the repository root.
func LoadProviderSessionsTopLevelInventory(root string) (ProviderSessionsTopLevelInventory, error) {
	path := filepath.Join(root, filepath.FromSlash(ProviderSessionsTopLevelInventoryRelativePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		return ProviderSessionsTopLevelInventory{}, fmt.Errorf("read provider sessions top-level inventory %s: %w", ProviderSessionsTopLevelInventoryRelativePath, err)
	}
	var inventory ProviderSessionsTopLevelInventory
	if err := json.Unmarshal(payload, &inventory); err != nil {
		return ProviderSessionsTopLevelInventory{}, fmt.Errorf("parse provider sessions top-level inventory %s: %w", ProviderSessionsTopLevelInventoryRelativePath, err)
	}
	return inventory, nil
}

// ListProviderSessionsTopLevelDirectories returns every live top-level directory
// under pkg/services/provider_sessions/, stable-sorted by directory name.
func ListProviderSessionsTopLevelDirectories(root string) ([]string, error) {
	serviceRoot := filepath.Join(root, filepath.FromSlash(ProviderSessionsOwnerPackagePath))
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		return nil, fmt.Errorf("read provider sessions root %s: %w", ProviderSessionsOwnerPackagePath, err)
	}
	var directories []string
	for _, entry := range entries {
		if !entry.IsDir() || ignoredProviderSessionsTopLevelDirectory(entry.Name()) {
			continue
		}
		directories = append(directories, entry.Name())
	}
	slices.Sort(directories)
	return directories, nil
}

// ProviderSessionsUnexpectedPublicSiblingPackagePaths returns stable-sorted
// production package paths for every unexpected public top-level sibling listed
// in the committed Provider Sessions top-level inventory.
func ProviderSessionsUnexpectedPublicSiblingPackagePaths(inventory ProviderSessionsTopLevelInventory) []string {
	var packagePaths []string
	for _, child := range inventory.Children {
		if !isProviderSessionsUnexpectedPublicSiblingClassification(child.Classification) {
			continue
		}
		packagePaths = append(packagePaths, ProviderSessionsOwnerPackagePath+"/"+child.Directory)
	}
	slices.Sort(packagePaths)
	return packagePaths
}

func isProviderSessionsUnexpectedPublicSiblingClassification(classification string) bool {
	return classification == ProviderSessionsTopLevelUnexpectedPublicSibling ||
		classification == ProviderSessionsTopLevelINVUnexpectedPublicSibling
}

// VerifyProviderSessionsUnexpectedPublicSiblingRemaps proves live top-level
// children match the committed inventory and every unexpected public sibling
// is remapped move/delete with an explicit private destination rather than
// retain→provider_sessions.
func VerifyProviderSessionsUnexpectedPublicSiblingRemaps(root string) error {
	if err := VerifyProviderSessionsTopLevelInventory(root); err != nil {
		return err
	}
	inventory, err := LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		return err
	}
	for _, packagePath := range ProviderSessionsUnexpectedPublicSiblingPackagePaths(inventory) {
		row, err := MapPackage(packagePath)
		if err != nil {
			return fmt.Errorf("map unexpected public sibling %q: %w", packagePath, err)
		}
		if err := validateProviderSessionsUnexpectedPublicSiblingRow(packagePath, row); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderSessionsUnexpectedPublicSiblingRow(packagePath string, row PackageRow) error {
	if row.Disposition == DispositionRetain && row.Destination == "provider_sessions" {
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
		if !isProviderSessionsPrivateSuccessor(row.Successor) {
			return fmt.Errorf("unexpected public sibling %q successor %q outside provider_sessions private destinations", packagePath, row.Successor)
		}
	}
	return nil
}

func isProviderSessionsPrivateSuccessor(successor string) bool {
	switch successor {
	case ProviderSessionsOwnerPackagePath + "/internal",
		ProviderSessionsOwnerPackagePath + "/internal/services/codex_reader",
		ProviderSessionsOwnerPackagePath + "/internal/services/cursor_reader":
		return true
	default:
		return false
	}
}

// VerifyProviderSessionsTopLevelInventory proves the live filesystem matches the
// committed Provider Sessions top-level inventory rows.
func VerifyProviderSessionsTopLevelInventory(root string) error {
	inventory, err := LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		return err
	}
	if err := validateProviderSessionsTopLevelInventory(inventory); err != nil {
		return err
	}
	live, err := ListProviderSessionsTopLevelDirectories(root)
	if err != nil {
		return err
	}
	committed := providerSessionsTopLevelDirectoryNames(inventory.Children)
	if !slices.Equal(live, committed) {
		return fmt.Errorf("provider sessions top-level directories drift: live=%v committed=%v", live, committed)
	}
	return nil
}

func validateProviderSessionsTopLevelInventory(inventory ProviderSessionsTopLevelInventory) error {
	if inventory.FormatVersion != "pss-provider-sessions-top-level-inventory/v1" {
		return fmt.Errorf("provider sessions top-level inventory formatVersion = %q, want pss-provider-sessions-top-level-inventory/v1", inventory.FormatVersion)
	}
	if inventory.OwnerPackage != ProviderSessionsOwnerPackagePath {
		return fmt.Errorf("provider sessions top-level inventory ownerPackage = %q, want %s", inventory.OwnerPackage, ProviderSessionsOwnerPackagePath)
	}
	if len(inventory.Children) == 0 {
		return fmt.Errorf("provider sessions top-level inventory has no children")
	}
	seen := make(map[string]struct{}, len(inventory.Children))
	var unexpectedBeyondService []string
	for index, child := range inventory.Children {
		if strings.TrimSpace(child.Directory) == "" {
			return fmt.Errorf("provider sessions top-level inventory child %d has empty directory", index)
		}
		if _, duplicate := seen[child.Directory]; duplicate {
			return fmt.Errorf("provider sessions top-level inventory duplicate directory %q", child.Directory)
		}
		seen[child.Directory] = struct{}{}
		if !isProviderSessionsTopLevelClassification(child.Classification) {
			return fmt.Errorf("provider sessions top-level inventory child %q has unknown classification %q", child.Directory, child.Classification)
		}
		if isProviderSessionsUnexpectedPublicSiblingClassification(child.Classification) {
			unexpectedBeyondService = append(unexpectedBeyondService, child.Directory)
		}
	}
	if !slices.IsSorted(providerSessionsTopLevelDirectoryNames(inventory.Children)) {
		return fmt.Errorf("provider sessions top-level inventory children are not stable-sorted")
	}
	if inventory.HasUnexpectedPublicSiblingsBeyondService != (len(unexpectedBeyondService) > 0) {
		return fmt.Errorf("provider sessions top-level inventory hasUnexpectedPublicSiblingsBeyondService = %t, want %t", inventory.HasUnexpectedPublicSiblingsBeyondService, len(unexpectedBeyondService) > 0)
	}
	if !slices.Equal(inventory.UnexpectedPublicSiblingsBeyondService, unexpectedBeyondService) {
		return fmt.Errorf("provider sessions top-level inventory unexpectedPublicSiblingsBeyondService = %v, want %v", inventory.UnexpectedPublicSiblingsBeyondService, unexpectedBeyondService)
	}
	return nil
}

func providerSessionsTopLevelDirectoryNames(children []ProviderSessionsTopLevelChild) []string {
	names := make([]string, 0, len(children))
	for _, child := range children {
		names = append(names, child.Directory)
	}
	return names
}

func isProviderSessionsTopLevelClassification(classification string) bool {
	switch classification {
	case ProviderSessionsTopLevelCanonicalRetain:
		return true
	case ProviderSessionsTopLevelUnexpectedPublicSibling,
		ProviderSessionsTopLevelINVUnexpectedPublicSibling:
		return true
	default:
		return false
	}
}

func ignoredProviderSessionsTopLevelDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	default:
		return false
	}
}
