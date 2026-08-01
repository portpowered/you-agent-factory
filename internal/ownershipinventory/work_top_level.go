package ownershipinventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const WorkOwnerPackagePath = workPackagePrefix

// VerifyWorkTopLevelInventory proves the live filesystem matches the committed
// unified owner top-level inventory for Work.
func VerifyWorkTopLevelInventory(root string) error {
	live, err := ListOwnerTopLevelChildren(root, workOwner)
	if err != nil {
		return err
	}
	want, ok := OwnerTopLevelInventory(workOwner)
	if !ok {
		return fmt.Errorf("work owner top-level inventory missing")
	}
	if !slices.Equal(live, want) {
		return fmt.Errorf("work top-level directories drift: live=%v committed=%v", live, want)
	}
	return nil
}

// VerifyWorkDeletedTransitionalPublicPathsAbsent locks DEL-WORK story 002:
// emptied transitional service/ and stateaccessrecordings/ public paths must
// not reappear on disk after leased deletion.
func VerifyWorkDeletedTransitionalPublicPathsAbsent(root string) error {
	serviceRoot := filepath.Join(root, filepath.FromSlash(workPackagePrefix))
	for _, dir := range []string{"service", "stateaccessrecordings"} {
		path := filepath.Join(serviceRoot, dir)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return fmt.Errorf("deleted transitional public path %q still present: %v", dir, err)
		}
	}
	return nil
}

// VerifyWorkUnexpectedPublicSiblingRemaps proves every committed unexpected
// public top-level sibling is remapped move/delete with an explicit private
// destination rather than retain→work.
func VerifyWorkUnexpectedPublicSiblingRemaps(root string) error {
	if err := VerifyWorkTopLevelInventory(root); err != nil {
		return err
	}
	spec, ok := OwnerTopLevelSpecFor(workOwner)
	if !ok {
		return fmt.Errorf("work owner top-level spec missing")
	}
	for _, child := range spec.Unexpected {
		packagePath := workPackagePrefix + "/" + child
		row, err := MapPackage(packagePath)
		if err != nil {
			return fmt.Errorf("map unexpected public sibling %q: %w", packagePath, err)
		}
		if err := validateWorkUnexpectedPublicSiblingRow(packagePath, row); err != nil {
			return err
		}
	}
	return nil
}

// VerifyWorkPostDelTransitionalDebtBurnDown proves DEL-WORK deleted transitional
// paths are absent from the committed owner top-level inventory and no
// unexpected public siblings remain recorded.
func VerifyWorkPostDelTransitionalDebtBurnDown(root string) error {
	if err := VerifyWorkDeletedTransitionalPublicPathsAbsent(root); err != nil {
		return err
	}
	spec, ok := OwnerTopLevelSpecFor(workOwner)
	if !ok {
		return fmt.Errorf("work owner top-level spec missing")
	}
	for _, deleted := range []string{"service", "stateaccessrecordings"} {
		if slices.Contains(spec.ExpectedRetain, deleted) || slices.Contains(spec.Unexpected, deleted) {
			return fmt.Errorf("deleted transitional directory %q still listed in work top-level inventory", deleted)
		}
	}
	wantUnexpected := []string{}
	if !slices.Equal(spec.Unexpected, wantUnexpected) {
		return fmt.Errorf("work unexpected top-level siblings = %v, want %v after DEL-WORK", spec.Unexpected, wantUnexpected)
	}
	return nil
}

func validateWorkUnexpectedPublicSiblingRow(packagePath string, row PackageRow) error {
	if row.Disposition == DispositionRetain && row.Destination == workOwner {
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
		if !isWorkPrivateSuccessor(row.Successor) {
			return fmt.Errorf("unexpected public sibling %q successor %q outside work private destinations", packagePath, row.Successor)
		}
	}
	return nil
}

func isWorkPrivateSuccessor(successor string) bool {
	if successor == workPackagePrefix+"/internal" {
		return true
	}
	return strings.HasPrefix(successor, workPackagePrefix+"/internal/")
}
