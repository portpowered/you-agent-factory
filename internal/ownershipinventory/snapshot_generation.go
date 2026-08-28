package ownershipinventory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	operatorSettingsRootGoFormatVersion   = "pss-operator-settings-root-go-inventory/v1"
	operatorSettingsTopLevelFormatVersion = "pss-operator-settings-top-level-inventory/v1"
	providerSessionsRootGoFormatVersion   = "pss-provider-sessions-root-go-inventory/v1"
	providerSessionsTopLevelFormatVersion = "pss-provider-sessions-top-level-inventory/v1"
	rootGoFileSortKeyDescription          = "file name ascending byte order"
	topLevelDirectorySortKeyDescription   = "directory name ascending byte order"
)

// SnapshotCandidates contains the four S-05 through S-08 projections that are
// generated together by ownershipinventoryfreeze. Each builder validates its
// complete candidate before this value is returned, so the command can prepare
// every new artifact before replacing any of them.
type SnapshotCandidates struct {
	OperatorSettingsRootGo   OperatorSettingsRootGoInventory
	OperatorSettingsTopLevel OperatorSettingsTopLevelInventory
	ProviderSessionsRootGo   ProviderSessionsRootGoInventory
	ProviderSessionsTopLevel ProviderSessionsTopLevelInventory
}

// BuildSnapshotCandidates constructs and validates all four S-05 through S-08
// projections from the live owner roots and production classification helpers.
func BuildSnapshotCandidates(root string) (SnapshotCandidates, error) {
	operatorSettingsRootGo, err := BuildOperatorSettingsRootGoInventory(root)
	if err != nil {
		return SnapshotCandidates{}, fmt.Errorf("operator settings root go: %w", err)
	}
	operatorSettingsTopLevel, err := BuildOperatorSettingsTopLevelInventory(root)
	if err != nil {
		return SnapshotCandidates{}, fmt.Errorf("operator settings top level: %w", err)
	}
	providerSessionsRootGo, err := BuildProviderSessionsRootGoInventory(root)
	if err != nil {
		return SnapshotCandidates{}, fmt.Errorf("provider sessions root go: %w", err)
	}
	providerSessionsTopLevel, err := BuildProviderSessionsTopLevelInventory(root)
	if err != nil {
		return SnapshotCandidates{}, fmt.Errorf("provider sessions top level: %w", err)
	}
	return SnapshotCandidates{
		OperatorSettingsRootGo:   operatorSettingsRootGo,
		OperatorSettingsTopLevel: operatorSettingsTopLevel,
		ProviderSessionsRootGo:   providerSessionsRootGo,
		ProviderSessionsTopLevel: providerSessionsTopLevel,
	}, nil
}

// BuildOperatorSettingsRootGoInventory constructs the S-05 root-Go projection
// from the live filename list and the production root-contract classifier.
func BuildOperatorSettingsRootGoInventory(root string) (OperatorSettingsRootGoInventory, error) {
	files, err := ListOperatorSettingsRootGoFiles(root)
	if err != nil {
		return OperatorSettingsRootGoInventory{}, err
	}
	inventory := OperatorSettingsRootGoInventory{
		FormatVersion: operatorSettingsRootGoFormatVersion,
		OwnerPackage:  OperatorSettingsOwnerPackagePath,
		SortKey:       rootGoFileSortKeyDescription,
		Clusters:      make([]OperatorSettingsRootGoCluster, 0, len(OperatorSettingsExcessRootContractFolds)),
		Files:         make([]OperatorSettingsRootGoFile, 0, len(files)),
	}
	for _, fileName := range files {
		file, err := buildOperatorSettingsRootGoFile(fileName)
		if err != nil {
			return OperatorSettingsRootGoInventory{}, err
		}
		inventory.Files = append(inventory.Files, file)
	}
	for _, target := range OperatorSettingsExcessRootContractFolds {
		clusterFiles := slices.Clone(target.Files)
		slices.Sort(clusterFiles)
		inventory.Clusters = append(inventory.Clusters, OperatorSettingsRootGoCluster{
			Cluster:     target.Cluster,
			Destination: target.Destination,
			Files:       clusterFiles,
		})
	}
	slices.SortFunc(inventory.Clusters, func(left, right OperatorSettingsRootGoCluster) int {
		return strings.Compare(left.Cluster, right.Cluster)
	})
	if err := validateOperatorSettingsRootGoInventory(inventory); err != nil {
		return OperatorSettingsRootGoInventory{}, fmt.Errorf("validate candidate: %w", err)
	}
	return inventory, nil
}

func buildOperatorSettingsRootGoFile(fileName string) (OperatorSettingsRootGoFile, error) {
	kind, foldTarget, ok := ClassifyOperatorSettingsRootContractFile(fileName)
	if !ok {
		return OperatorSettingsRootGoFile{}, fmt.Errorf("operator settings root go file %q is unclassified by production ownership policy", fileName)
	}
	classification, err := operatorSettingsRootGoSnapshotClassification(fileName, kind, foldTarget)
	if err != nil {
		return OperatorSettingsRootGoFile{}, err
	}
	file := OperatorSettingsRootGoFile{
		File:           fileName,
		Classification: classification,
		Note:           operatorSettingsRootGoNote(fileName),
	}
	if kind == "excess_fold" {
		file.FoldDestination = foldTarget.Destination
		file.Cluster = foldTarget.Cluster
	}
	return file, nil
}

func operatorSettingsRootGoSnapshotClassification(fileName, kind string, foldTarget OperatorSettingsRootContractFoldTarget) (string, error) {
	switch kind {
	case "thin_root_retain":
		if strings.HasSuffix(fileName, "_test.go") {
			return OperatorSettingsRootGoThinContractTest, nil
		}
		return OperatorSettingsRootGoThinContract, nil
	case "excess_fold":
		if strings.TrimSpace(foldTarget.Classification) == "" {
			return "", fmt.Errorf("operator settings root go fold target %q has no production snapshot classification", fileName)
		}
		if !isOperatorSettingsRootGoClassification(foldTarget.Classification) || !isOperatorSettingsRootGoFoldTargetClassification(foldTarget.Classification) {
			return "", fmt.Errorf("operator settings root go fold target %q has unknown production snapshot classification %q", fileName, foldTarget.Classification)
		}
		return foldTarget.Classification, nil
	default:
		return "", fmt.Errorf("operator settings root go file %q has unknown production classification kind %q", fileName, kind)
	}
}

// BuildOperatorSettingsTopLevelInventory constructs the S-06 immediate-child
// projection from the live directory list and owner top-level classification.
func BuildOperatorSettingsTopLevelInventory(root string) (OperatorSettingsTopLevelInventory, error) {
	directories, err := ListOperatorSettingsTopLevelDirectories(root)
	if err != nil {
		return OperatorSettingsTopLevelInventory{}, err
	}
	inventory := OperatorSettingsTopLevelInventory{
		FormatVersion:            operatorSettingsTopLevelFormatVersion,
		OwnerPackage:             OperatorSettingsOwnerPackagePath,
		SortKey:                  topLevelDirectorySortKeyDescription,
		UnexpectedPublicSiblings: make([]string, 0),
		Children:                 make([]OperatorSettingsTopLevelChild, 0, len(directories)),
	}
	for _, directory := range directories {
		classification, ok := operatorSettingsTopLevelSnapshotClassification(directory)
		if !ok {
			return OperatorSettingsTopLevelInventory{}, fmt.Errorf("operator settings top-level directory %q is unclassified by production ownership policy", directory)
		}
		child := OperatorSettingsTopLevelChild{
			Directory:      directory,
			Classification: classification,
			Note:           operatorSettingsTopLevelNote(directory, classification),
		}
		inventory.Children = append(inventory.Children, child)
		if classification == OperatorSettingsTopLevelUnexpectedPublicSibling {
			inventory.UnexpectedPublicSiblings = append(inventory.UnexpectedPublicSiblings, directory)
		}
	}
	if err := validateOperatorSettingsTopLevelInventory(inventory); err != nil {
		return OperatorSettingsTopLevelInventory{}, fmt.Errorf("validate candidate: %w", err)
	}
	return inventory, nil
}

func operatorSettingsTopLevelSnapshotClassification(directory string) (string, bool) {
	kind, ok := ClassifyOwnerTopLevelChild(operatorSettingsOwner, directory)
	if !ok {
		return "", false
	}
	switch kind {
	case "expected_retain":
		return OperatorSettingsTopLevelCanonicalRetain, true
	case "unexpected_move":
		if directory == "testdata" {
			return OperatorSettingsTopLevelTestOnlyRetain, true
		}
		return OperatorSettingsTopLevelUnexpectedPublicSibling, true
	default:
		return "", false
	}
}

// BuildProviderSessionsRootGoInventory constructs the S-07 root-Go projection
// from the live filename list and the production root-contract classifier.
func BuildProviderSessionsRootGoInventory(root string) (ProviderSessionsRootGoInventory, error) {
	files, err := ListProviderSessionsRootGoFiles(root)
	if err != nil {
		return ProviderSessionsRootGoInventory{}, err
	}
	inventory := ProviderSessionsRootGoInventory{
		FormatVersion: providerSessionsRootGoFormatVersion,
		OwnerPackage:  ProviderSessionsOwnerPackagePath,
		SortKey:       rootGoFileSortKeyDescription,
		Files:         make([]ProviderSessionsRootGoFile, 0, len(files)),
	}
	for _, fileName := range files {
		kind, foldTarget, ok := ClassifyProviderSessionsRootContractFile(fileName)
		if !ok {
			return ProviderSessionsRootGoInventory{}, fmt.Errorf("provider sessions root go file %q is unclassified by production ownership policy", fileName)
		}
		classification, err := providerSessionsRootGoSnapshotClassification(fileName, kind, foldTarget)
		if err != nil {
			return ProviderSessionsRootGoInventory{}, err
		}
		inventory.Files = append(inventory.Files, ProviderSessionsRootGoFile{
			File:            fileName,
			Classification:  classification,
			FoldDestination: foldTarget.Destination,
			Note:            providerSessionsRootGoNote(fileName),
		})
	}
	if err := validateProviderSessionsRootGoInventory(inventory); err != nil {
		return ProviderSessionsRootGoInventory{}, fmt.Errorf("validate candidate: %w", err)
	}
	return inventory, nil
}

func providerSessionsRootGoSnapshotClassification(fileName, kind string, foldTarget ProviderSessionsRootContractFoldTarget) (string, error) {
	switch kind {
	case "thin_root_retain":
		if strings.HasSuffix(fileName, "_test.go") {
			return ProviderSessionsRootGoThinContractTest, nil
		}
		return ProviderSessionsRootGoThinContract, nil
	case "excess_fold":
		if strings.TrimSpace(foldTarget.Classification) == "" {
			return "", fmt.Errorf("provider sessions root go fold target %q has no production snapshot classification", fileName)
		}
		if !isProviderSessionsRootGoClassification(foldTarget.Classification) || !isProviderSessionsRootGoFoldTargetClassification(foldTarget.Classification) {
			return "", fmt.Errorf("provider sessions root go fold target %q has unknown production snapshot classification %q", fileName, foldTarget.Classification)
		}
		return foldTarget.Classification, nil
	default:
		return "", fmt.Errorf("provider sessions root go file %q has unknown production classification kind %q", fileName, kind)
	}
}

// BuildProviderSessionsTopLevelInventory constructs the S-08 immediate-child
// projection from the live directory list and owner top-level classification.
func BuildProviderSessionsTopLevelInventory(root string) (ProviderSessionsTopLevelInventory, error) {
	directories, err := ListProviderSessionsTopLevelDirectories(root)
	if err != nil {
		return ProviderSessionsTopLevelInventory{}, err
	}
	inventory := ProviderSessionsTopLevelInventory{
		FormatVersion:                            providerSessionsTopLevelFormatVersion,
		OwnerPackage:                             ProviderSessionsOwnerPackagePath,
		SortKey:                                  topLevelDirectorySortKeyDescription,
		HasUnexpectedPublicSiblingsBeyondService: false,
		UnexpectedPublicSiblingsBeyondService:    make([]string, 0),
		Children:                                 make([]ProviderSessionsTopLevelChild, 0, len(directories)),
	}
	for _, directory := range directories {
		classification, ok := providerSessionsTopLevelSnapshotClassification(directory)
		if !ok {
			return ProviderSessionsTopLevelInventory{}, fmt.Errorf("provider sessions top-level directory %q is unclassified by production ownership policy", directory)
		}
		inventory.Children = append(inventory.Children, ProviderSessionsTopLevelChild{
			Directory:      directory,
			Classification: classification,
		})
		if isProviderSessionsUnexpectedPublicSiblingClassification(classification) {
			inventory.UnexpectedPublicSiblingsBeyondService = append(inventory.UnexpectedPublicSiblingsBeyondService, directory)
		}
	}
	inventory.HasUnexpectedPublicSiblingsBeyondService = len(inventory.UnexpectedPublicSiblingsBeyondService) > 0
	if err := validateProviderSessionsTopLevelInventory(inventory); err != nil {
		return ProviderSessionsTopLevelInventory{}, fmt.Errorf("validate candidate: %w", err)
	}
	return inventory, nil
}

func providerSessionsTopLevelSnapshotClassification(directory string) (string, bool) {
	kind, ok := ClassifyOwnerTopLevelChild("provider_sessions", directory)
	if !ok {
		return "", false
	}
	switch kind {
	case "expected_retain":
		return ProviderSessionsTopLevelCanonicalRetain, true
	case "unexpected_move":
		if directory == "service" {
			return ProviderSessionsTopLevelUnexpectedPublicSibling, true
		}
		return ProviderSessionsTopLevelINVUnexpectedPublicSibling, true
	default:
		return "", false
	}
}

// WriteSnapshotCandidates validates and writes all four new artifacts. All
// candidate payloads are serialized before the first new file is replaced.
func WriteSnapshotCandidates(root string, candidates SnapshotCandidates) error {
	writes, err := snapshotWrites(candidates)
	if err != nil {
		return err
	}
	for _, write := range writes {
		if err := writeSnapshot(root, write.relativePath, write.payload); err != nil {
			return fmt.Errorf("%s: %w", write.relativePath, err)
		}
	}
	return nil
}

// WriteOperatorSettingsRootGoInventory writes one validated S-05 candidate.
func WriteOperatorSettingsRootGoInventory(root string, inventory OperatorSettingsRootGoInventory) error {
	payload, err := marshalSnapshot("operator settings root go inventory", inventory, func() error {
		return validateOperatorSettingsRootGoInventory(inventory)
	})
	if err != nil {
		return err
	}
	return writeSnapshot(root, OperatorSettingsRootGoInventoryRelativePath, payload)
}

// WriteOperatorSettingsTopLevelInventory writes one validated S-06 candidate.
func WriteOperatorSettingsTopLevelInventory(root string, inventory OperatorSettingsTopLevelInventory) error {
	payload, err := marshalSnapshot("operator settings top-level inventory", inventory, func() error {
		return validateOperatorSettingsTopLevelInventory(inventory)
	})
	if err != nil {
		return err
	}
	return writeSnapshot(root, OperatorSettingsTopLevelInventoryRelativePath, payload)
}

// WriteProviderSessionsRootGoInventory writes one validated S-07 candidate.
func WriteProviderSessionsRootGoInventory(root string, inventory ProviderSessionsRootGoInventory) error {
	payload, err := marshalSnapshot("provider sessions root go inventory", inventory, func() error {
		return validateProviderSessionsRootGoInventory(inventory)
	})
	if err != nil {
		return err
	}
	return writeSnapshot(root, ProviderSessionsRootGoInventoryRelativePath, payload)
}

// WriteProviderSessionsTopLevelInventory writes one validated S-08 candidate.
func WriteProviderSessionsTopLevelInventory(root string, inventory ProviderSessionsTopLevelInventory) error {
	payload, err := marshalSnapshot("provider sessions top-level inventory", inventory, func() error {
		return validateProviderSessionsTopLevelInventory(inventory)
	})
	if err != nil {
		return err
	}
	return writeSnapshot(root, ProviderSessionsTopLevelInventoryRelativePath, payload)
}

type snapshotWrite struct {
	relativePath string
	payload      []byte
}

func snapshotWrites(candidates SnapshotCandidates) ([]snapshotWrite, error) {
	operatorSettingsRootGo, err := marshalSnapshot("operator settings root go inventory", candidates.OperatorSettingsRootGo, func() error {
		return validateOperatorSettingsRootGoInventory(candidates.OperatorSettingsRootGo)
	})
	if err != nil {
		return nil, err
	}
	operatorSettingsTopLevel, err := marshalSnapshot("operator settings top-level inventory", candidates.OperatorSettingsTopLevel, func() error {
		return validateOperatorSettingsTopLevelInventory(candidates.OperatorSettingsTopLevel)
	})
	if err != nil {
		return nil, err
	}
	providerSessionsRootGo, err := marshalSnapshot("provider sessions root go inventory", candidates.ProviderSessionsRootGo, func() error {
		return validateProviderSessionsRootGoInventory(candidates.ProviderSessionsRootGo)
	})
	if err != nil {
		return nil, err
	}
	providerSessionsTopLevel, err := marshalSnapshot("provider sessions top-level inventory", candidates.ProviderSessionsTopLevel, func() error {
		return validateProviderSessionsTopLevelInventory(candidates.ProviderSessionsTopLevel)
	})
	if err != nil {
		return nil, err
	}
	return []snapshotWrite{
		{relativePath: OperatorSettingsRootGoInventoryRelativePath, payload: operatorSettingsRootGo},
		{relativePath: OperatorSettingsTopLevelInventoryRelativePath, payload: operatorSettingsTopLevel},
		{relativePath: ProviderSessionsRootGoInventoryRelativePath, payload: providerSessionsRootGo},
		{relativePath: ProviderSessionsTopLevelInventoryRelativePath, payload: providerSessionsTopLevel},
	}, nil
}

func marshalSnapshot(label string, value any, validate func() error) ([]byte, error) {
	if err := validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", label, err)
	}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("marshal %s: %w", label, err)
	}
	return payload.Bytes(), nil
}

func writeSnapshot(root, relativePath string, payload []byte) error {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir snapshot directory: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	return nil
}
