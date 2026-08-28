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
	OperatorSettingsRootGoInventoryRelativePath = "docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json"

	OperatorSettingsRootGoThinContract                 = "thin_root_contract"
	OperatorSettingsRootGoThinContractTest             = "thin_root_contract_test"
	OperatorSettingsRootGoFoldTargetConstruction       = "fold_target_construction_port"
	OperatorSettingsRootGoFoldTargetDocument           = "fold_target_document"
	OperatorSettingsRootGoFoldTargetResolution         = "fold_target_resolution"
	OperatorSettingsRootGoFoldTargetIdentity           = "fold_target_identity"
	OperatorSettingsRootGoFoldTargetProvidersConstruct = "fold_target_providers_construct"
	OperatorSettingsRootGoFoldTargetImplementation     = "fold_target_implementation"
	OperatorSettingsRootGoFoldTargetImplTest           = "fold_target_implementation_test"
)

// OperatorSettingsRootGoCluster records one excess root contract/helper cluster
// for CLN-SET-CONTRACT-ROOTS without performing the fold in INV-SET-TOPLEVEL.
type OperatorSettingsRootGoCluster struct {
	Cluster     string   `json:"cluster"`
	Destination string   `json:"destination"`
	Files       []string `json:"files"`
}

// OperatorSettingsRootGoFile records one committed root-level .go file
// classification under pkg/services/operator_settings/.
type OperatorSettingsRootGoFile struct {
	File            string `json:"file"`
	Classification  string `json:"classification"`
	FoldDestination string `json:"foldDestination,omitempty"`
	Cluster         string `json:"cluster,omitempty"`
	Note            string `json:"note,omitempty"`
}

// OperatorSettingsRootGoInventory is the INV-SET-TOPLEVEL root .go freeze for
// Operator Settings thin contract vs fold/consolidation targets.
type OperatorSettingsRootGoInventory struct {
	FormatVersion string                          `json:"formatVersion"`
	OwnerPackage  string                          `json:"ownerPackage"`
	SortKey       string                          `json:"sortKey"`
	Clusters      []OperatorSettingsRootGoCluster `json:"clusters"`
	Files         []OperatorSettingsRootGoFile    `json:"files"`
}

// LoadOperatorSettingsRootGoInventory reads the committed Operator Settings
// root .go inventory artifact from the repository root.
func LoadOperatorSettingsRootGoInventory(root string) (OperatorSettingsRootGoInventory, error) {
	path := filepath.Join(root, filepath.FromSlash(OperatorSettingsRootGoInventoryRelativePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		return OperatorSettingsRootGoInventory{}, fmt.Errorf("read operator settings root go inventory %s: %w", OperatorSettingsRootGoInventoryRelativePath, err)
	}
	var inventory OperatorSettingsRootGoInventory
	if err := json.Unmarshal(payload, &inventory); err != nil {
		return OperatorSettingsRootGoInventory{}, fmt.Errorf("parse operator settings root go inventory %s: %w", OperatorSettingsRootGoInventoryRelativePath, err)
	}
	return inventory, nil
}

// ListOperatorSettingsRootGoFiles returns every live root-level .go file under
// pkg/services/operator_settings/, stable-sorted by file name.
func ListOperatorSettingsRootGoFiles(root string) ([]string, error) {
	serviceRoot := filepath.Join(root, filepath.FromSlash(OperatorSettingsOwnerPackagePath))
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		return nil, fmt.Errorf("read operator settings root %s: %w", OperatorSettingsOwnerPackagePath, err)
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

// OperatorSettingsRootGoFoldTargets returns stable-sorted fold/consolidation
// target rows from the committed inventory for CLN-SET-CONTRACT-ROOTS.
func OperatorSettingsRootGoFoldTargets(inventory OperatorSettingsRootGoInventory) []OperatorSettingsRootGoFile {
	var targets []OperatorSettingsRootGoFile
	for _, file := range inventory.Files {
		if !isOperatorSettingsRootGoFoldTargetClassification(file.Classification) {
			continue
		}
		targets = append(targets, file)
	}
	slices.SortFunc(targets, func(left, right OperatorSettingsRootGoFile) int {
		return strings.Compare(left.File, right.File)
	})
	return targets
}

// VerifyOperatorSettingsRootGoInventory proves the live filesystem matches the
// committed Operator Settings root .go inventory rows.
func VerifyOperatorSettingsRootGoInventory(root string) error {
	inventory, err := LoadOperatorSettingsRootGoInventory(root)
	if err != nil {
		return err
	}
	if err := validateOperatorSettingsRootGoInventory(inventory); err != nil {
		return err
	}
	live, err := ListOperatorSettingsRootGoFiles(root)
	if err != nil {
		return err
	}
	committed := operatorSettingsRootGoFileNames(inventory.Files)
	if !slices.Equal(live, committed) {
		return fmt.Errorf("operator settings root .go files drift: live=%v committed=%v", live, committed)
	}
	if err := verifyOperatorSettingsRootGoProductionPolicy(inventory); err != nil {
		return err
	}
	return nil
}

func verifyOperatorSettingsRootGoProductionPolicy(inventory OperatorSettingsRootGoInventory) error {
	for _, file := range inventory.Files {
		kind, foldTarget, ok := ClassifyOperatorSettingsRootContractFile(file.File)
		if !ok {
			return fmt.Errorf("operator settings root go inventory file %q is unclassified by production ownership policy", file.File)
		}
		wantClassification, err := operatorSettingsRootGoSnapshotClassification(file.File, kind, foldTarget)
		if err != nil {
			return err
		}
		if file.Classification != wantClassification {
			return fmt.Errorf("operator settings root go inventory file %q classification %q disagrees with production ownership policy classification %q", file.File, file.Classification, wantClassification)
		}
		if kind == "excess_fold" {
			if file.FoldDestination != foldTarget.Destination {
				return fmt.Errorf("operator settings root go inventory file %q foldDestination = %q, want production policy destination %q", file.File, file.FoldDestination, foldTarget.Destination)
			}
			if file.Cluster != foldTarget.Cluster {
				return fmt.Errorf("operator settings root go inventory file %q cluster = %q, want production policy cluster %q", file.File, file.Cluster, foldTarget.Cluster)
			}
		}
	}
	committed := operatorSettingsRootGoFileNames(inventory.Files)
	want := OperatorSettingsRootContractInventory()
	if !slices.Equal(committed, want) {
		return fmt.Errorf("operator settings root contract inventory drift: json=%v go=%v", committed, want)
	}
	return nil
}

func validateOperatorSettingsRootGoInventory(inventory OperatorSettingsRootGoInventory) error {
	if inventory.FormatVersion != "pss-operator-settings-root-go-inventory/v1" {
		return fmt.Errorf("operator settings root go inventory formatVersion = %q, want pss-operator-settings-root-go-inventory/v1", inventory.FormatVersion)
	}
	if inventory.OwnerPackage != OperatorSettingsOwnerPackagePath {
		return fmt.Errorf("operator settings root go inventory ownerPackage = %q, want %s", inventory.OwnerPackage, OperatorSettingsOwnerPackagePath)
	}
	if inventory.SortKey != rootGoFileSortKeyDescription {
		return fmt.Errorf("operator settings root go inventory sortKey = %q, want %s", inventory.SortKey, rootGoFileSortKeyDescription)
	}
	if len(inventory.Files) == 0 {
		return fmt.Errorf("operator settings root go inventory has no files")
	}
	if err := validateOperatorSettingsRootGoClusters(inventory); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(inventory.Files))
	for index, file := range inventory.Files {
		if strings.TrimSpace(file.File) == "" {
			return fmt.Errorf("operator settings root go inventory file %d has empty file name", index)
		}
		if _, duplicate := seen[file.File]; duplicate {
			return fmt.Errorf("operator settings root go inventory duplicate file %q", file.File)
		}
		seen[file.File] = struct{}{}
		if !isOperatorSettingsRootGoClassification(file.Classification) {
			return fmt.Errorf("operator settings root go inventory file %q has unknown classification %q", file.File, file.Classification)
		}
		if isOperatorSettingsRootGoFoldTargetClassification(file.Classification) {
			if strings.TrimSpace(file.FoldDestination) == "" {
				return fmt.Errorf("operator settings root go inventory fold target %q missing foldDestination", file.File)
			}
			if file.FoldDestination == OperatorSettingsOwnerPackagePath {
				return fmt.Errorf("operator settings root go inventory fold target %q regressed to owner root retain destination", file.File)
			}
			if !isOperatorSettingsPrivateSuccessor(file.FoldDestination) {
				return fmt.Errorf("operator settings root go inventory fold target %q destination %q outside operator_settings private destinations", file.File, file.FoldDestination)
			}
			continue
		}
		if strings.TrimSpace(file.FoldDestination) != "" {
			return fmt.Errorf("operator settings root go inventory thin surface %q must not set foldDestination", file.File)
		}
		if strings.TrimSpace(file.Cluster) != "" {
			return fmt.Errorf("operator settings root go inventory thin surface %q must not set cluster", file.File)
		}
	}
	if !slices.IsSorted(operatorSettingsRootGoFileNames(inventory.Files)) {
		return fmt.Errorf("operator settings root go inventory files are not stable-sorted")
	}
	return nil
}

func validateOperatorSettingsRootGoClusters(inventory OperatorSettingsRootGoInventory) error {
	clusterFiles := make(map[string]string, len(inventory.Files))
	for _, file := range inventory.Files {
		if !isOperatorSettingsRootGoFoldTargetClassification(file.Classification) {
			continue
		}
		if strings.TrimSpace(file.Cluster) == "" {
			return fmt.Errorf("operator settings root go inventory fold target %q missing cluster", file.File)
		}
		clusterFiles[file.File] = file.Cluster
	}
	for index, cluster := range inventory.Clusters {
		if strings.TrimSpace(cluster.Cluster) == "" {
			return fmt.Errorf("operator settings root go inventory cluster %d has empty cluster name", index)
		}
		if strings.TrimSpace(cluster.Destination) == "" {
			return fmt.Errorf("operator settings root go inventory cluster %q missing destination", cluster.Cluster)
		}
		if cluster.Destination == OperatorSettingsOwnerPackagePath {
			return fmt.Errorf("operator settings root go inventory cluster %q regressed to owner root retain destination", cluster.Cluster)
		}
		if !isOperatorSettingsPrivateSuccessor(cluster.Destination) {
			return fmt.Errorf("operator settings root go inventory cluster %q destination %q outside operator_settings private destinations", cluster.Cluster, cluster.Destination)
		}
		if len(cluster.Files) == 0 {
			return fmt.Errorf("operator settings root go inventory cluster %q has no files", cluster.Cluster)
		}
		if !slices.IsSorted(cluster.Files) {
			return fmt.Errorf("operator settings root go inventory cluster %q files are not stable-sorted", cluster.Cluster)
		}
		for _, fileName := range cluster.Files {
			wantCluster, ok := clusterFiles[fileName]
			if !ok {
				return fmt.Errorf("operator settings root go inventory cluster %q lists non-fold file %q", cluster.Cluster, fileName)
			}
			if wantCluster != cluster.Cluster {
				return fmt.Errorf("operator settings root go inventory cluster %q lists file %q classified under cluster %q", cluster.Cluster, fileName, wantCluster)
			}
		}
	}
	for fileName, clusterName := range clusterFiles {
		found := false
		for _, cluster := range inventory.Clusters {
			if cluster.Cluster != clusterName {
				continue
			}
			if slices.Contains(cluster.Files, fileName) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("operator settings root go inventory fold target %q cluster %q missing from clusters inventory", fileName, clusterName)
		}
	}
	return nil
}

func operatorSettingsRootGoFileNames(files []OperatorSettingsRootGoFile) []string {
	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.File)
	}
	return names
}

func isOperatorSettingsRootGoClassification(classification string) bool {
	switch classification {
	case OperatorSettingsRootGoThinContract,
		OperatorSettingsRootGoThinContractTest,
		OperatorSettingsRootGoFoldTargetConstruction,
		OperatorSettingsRootGoFoldTargetDocument,
		OperatorSettingsRootGoFoldTargetResolution,
		OperatorSettingsRootGoFoldTargetIdentity,
		OperatorSettingsRootGoFoldTargetProvidersConstruct,
		OperatorSettingsRootGoFoldTargetImplementation,
		OperatorSettingsRootGoFoldTargetImplTest:
		return true
	default:
		return false
	}
}

func isOperatorSettingsRootGoFoldTargetClassification(classification string) bool {
	switch classification {
	case OperatorSettingsRootGoFoldTargetConstruction,
		OperatorSettingsRootGoFoldTargetDocument,
		OperatorSettingsRootGoFoldTargetResolution,
		OperatorSettingsRootGoFoldTargetIdentity,
		OperatorSettingsRootGoFoldTargetProvidersConstruct,
		OperatorSettingsRootGoFoldTargetImplementation,
		OperatorSettingsRootGoFoldTargetImplTest:
		return true
	default:
		return false
	}
}
