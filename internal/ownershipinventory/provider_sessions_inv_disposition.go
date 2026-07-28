package ownershipinventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ProviderSessionsUnexpectedPublicSiblingBeyondServicePackagePaths returns
// stable-sorted production package paths for unexpected public top-level siblings
// beyond service/ listed in the committed Provider Sessions top-level inventory.
func ProviderSessionsUnexpectedPublicSiblingBeyondServicePackagePaths(inventory ProviderSessionsTopLevelInventory) []string {
	var packagePaths []string
	for _, child := range inventory.Children {
		if child.Directory == "service" {
			continue
		}
		if !isProviderSessionsUnexpectedPublicSiblingClassification(child.Classification) {
			continue
		}
		packagePaths = append(packagePaths, ProviderSessionsOwnerPackagePath+"/"+child.Directory)
	}
	slices.Sort(packagePaths)
	return packagePaths
}

// VerifyProviderSessionsZeroExtraPublicSiblingAbsence locks INV-PSES-TOPLEVEL's
// zero-extra path: the live top-level tree must match the committed inventory,
// hasUnexpectedPublicSiblingsBeyondService must remain false, and every child
// beyond service/ must stay canonical_retain rather than an unremapped public
// sibling.
func VerifyProviderSessionsZeroExtraPublicSiblingAbsence(root string) error {
	if err := VerifyProviderSessionsTopLevelInventory(root); err != nil {
		return err
	}
	inventory, err := LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		return err
	}
	if inventory.HasUnexpectedPublicSiblingsBeyondService {
		return fmt.Errorf("provider sessions zero-extra public-sibling absence lock requires hasUnexpectedPublicSiblingsBeyondService = false, got true with %v", inventory.UnexpectedPublicSiblingsBeyondService)
	}
	return verifyProviderSessionsZeroExtraBeyondServiceDisposition(inventory)
}

// VerifyProviderSessionsINVDispositionBeyondService consumes INV-PSES-TOPLEVEL's
// disposition for unexpected public siblings beyond service/. When INV records
// zero extras, the live tree must match that absence. When INV names siblings,
// each must be remapped with its real implementation under the recorded private
// successor rather than retain→provider_sessions at the public path.
func VerifyProviderSessionsINVDispositionBeyondService(root string) error {
	if err := VerifyProviderSessionsTopLevelInventory(root); err != nil {
		return err
	}
	inventory, err := LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		return err
	}
	if inventory.HasUnexpectedPublicSiblingsBeyondService {
		return verifyProviderSessionsNamedSiblingBeyondServiceDisposition(root, inventory)
	}
	return verifyProviderSessionsZeroExtraBeyondServiceDisposition(inventory)
}

func verifyProviderSessionsZeroExtraBeyondServiceDisposition(inventory ProviderSessionsTopLevelInventory) error {
	if len(inventory.UnexpectedPublicSiblingsBeyondService) > 0 {
		return fmt.Errorf("provider sessions top-level inventory hasUnexpectedPublicSiblingsBeyondService = false but unexpectedPublicSiblingsBeyondService = %v", inventory.UnexpectedPublicSiblingsBeyondService)
	}
	beyondService := ProviderSessionsUnexpectedPublicSiblingBeyondServicePackagePaths(inventory)
	if len(beyondService) > 0 {
		return fmt.Errorf("provider sessions zero-extra INV disposition still lists unexpected public siblings beyond service/: %v", beyondService)
	}
	for _, child := range inventory.Children {
		if child.Directory == "service" {
			if child.Classification != ProviderSessionsTopLevelUnexpectedPublicSibling {
				return fmt.Errorf("provider sessions top-level inventory child service must remain %q for CLN-PSES-FOLD-SERVICE ownership, got %q", ProviderSessionsTopLevelUnexpectedPublicSibling, child.Classification)
			}
			continue
		}
		if child.Classification != ProviderSessionsTopLevelCanonicalRetain {
			return fmt.Errorf("provider sessions zero-extra path child %q classification = %q, want %q", child.Directory, child.Classification, ProviderSessionsTopLevelCanonicalRetain)
		}
	}
	return nil
}

func verifyProviderSessionsNamedSiblingBeyondServiceDisposition(root string, inventory ProviderSessionsTopLevelInventory) error {
	if !slices.Equal(inventory.UnexpectedPublicSiblingsBeyondService, providerSessionsUnexpectedBeyondServiceDirectoryNames(inventory)) {
		return fmt.Errorf("provider sessions top-level inventory unexpectedPublicSiblingsBeyondService = %v, want %v", inventory.UnexpectedPublicSiblingsBeyondService, providerSessionsUnexpectedBeyondServiceDirectoryNames(inventory))
	}
	if err := VerifyProviderSessionsUnexpectedPublicSiblingRemaps(root); err != nil {
		return err
	}
	for _, packagePath := range ProviderSessionsUnexpectedPublicSiblingBeyondServicePackagePaths(inventory) {
		row, err := MapPackage(packagePath)
		if err != nil {
			return fmt.Errorf("map unexpected public sibling beyond service %q: %w", packagePath, err)
		}
		if err := validateProviderSessionsUnexpectedPublicSiblingRow(packagePath, row); err != nil {
			return err
		}
		if row.Disposition != DispositionMove {
			continue
		}
		if err := verifyProviderSessionsSuccessorHasImplementation(root, row.Successor); err != nil {
			return fmt.Errorf("unexpected public sibling beyond service %q: %w", packagePath, err)
		}
	}
	return nil
}

func providerSessionsUnexpectedBeyondServiceDirectoryNames(inventory ProviderSessionsTopLevelInventory) []string {
	names := make([]string, 0, len(inventory.UnexpectedPublicSiblingsBeyondService))
	for _, child := range inventory.Children {
		if child.Directory == "service" {
			continue
		}
		if isProviderSessionsUnexpectedPublicSiblingClassification(child.Classification) {
			names = append(names, child.Directory)
		}
	}
	slices.Sort(names)
	return names
}

func verifyProviderSessionsSuccessorHasImplementation(root, successorPackagePath string) error {
	dir := filepath.Join(root, filepath.FromSlash(successorPackagePath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read successor package %q: %w", successorPackagePath, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return nil
		}
	}
	return fmt.Errorf("successor package %q has no non-test Go implementation files", successorPackagePath)
}
