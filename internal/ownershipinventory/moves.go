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
	// UnfinishedMovesRelativePath is the single consolidated ledger of packages
	// that still carry unfinished Packaged Service Structure migration intent.
	// It replaces the per-package row lists that ownership-inventory.json and
	// package-target-manifest.json each used to carry separately.
	UnfinishedMovesRelativePath = "docs/internal/baselines/unfinished-package-moves.json"

	// UnfinishedMovesStage identifies the consolidated move ledger artifact.
	UnfinishedMovesStage = "pss-unfinished-package-moves"
)

// UnfinishedMoveRow records one package whose migration has not landed yet.
//
// There is deliberately no disposition field: every row in this ledger is an
// open move. A package that already lives where it belongs carries no row at
// all, because OwnerForPackage derives that from the tree.
type UnfinishedMoveRow struct {
	// PackagePath is the repository-relative package that still has to move.
	PackagePath string `json:"packagePath"`
	// Destination is the committed destination bucket, owner-relative:
	// <owner>, <owner>/internal, or <owner>/internal/services/<subservice>.
	Destination string `json:"destination"`
	// Successor is the repository-relative package path that replaces
	// PackagePath once the move lands.
	Successor string `json:"successor"`
	// DeletionCondition names the cutover proof that closes this row when the
	// migration packet named one. It is optional: rows that only ever existed
	// in the package-target manifest never carried a condition.
	DeletionCondition string `json:"deletionCondition,omitempty"`
}

// UnfinishedMoves is the consolidated open-move ledger artifact.
//
// The ledger only shrinks: landing a move deletes its row. When Moves is empty
// the file, its loaders, and its checks are deleted outright.
type UnfinishedMoves struct {
	Version  int                 `json:"version"`
	Stage    string              `json:"stage"`
	Purpose  string              `json:"purpose"`
	EndState string              `json:"endState"`
	SortKey  string              `json:"sortKey"`
	Moves    []UnfinishedMoveRow `json:"moves"`
}

// LoadUnfinishedMoves reads the consolidated open-move ledger from root. A
// missing file means every migration has landed, which is the ledger's intended
// end state, so it loads as an empty set rather than an error.
func LoadUnfinishedMoves(root string) (UnfinishedMoves, error) {
	path := filepath.Join(root, filepath.FromSlash(UnfinishedMovesRelativePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return UnfinishedMoves{}, nil
		}
		return UnfinishedMoves{}, fmt.Errorf("read unfinished package moves %s: %w", UnfinishedMovesRelativePath, err)
	}
	var moves UnfinishedMoves
	if err := json.Unmarshal(payload, &moves); err != nil {
		return UnfinishedMoves{}, fmt.Errorf("parse unfinished package moves %s: %w", UnfinishedMovesRelativePath, err)
	}
	return moves, nil
}

// PackageRows projects the consolidated ledger onto the ownership-inventory row
// shape. Owner and destination kind are derived from the destination bucket
// rather than restated per row.
func (m UnfinishedMoves) PackageRows() []PackageRow {
	rows := make([]PackageRow, 0, len(m.Moves))
	for _, move := range m.Moves {
		rows = append(rows, PackageRow{
			PackagePath:       move.PackagePath,
			Disposition:       DispositionMove,
			Destination:       DestinationOwnerOf(move.Destination),
			DestinationKind:   DestinationKindOwner,
			Successor:         move.Successor,
			DeletionCondition: move.DeletionCondition,
		})
	}
	return rows
}

// DestinationOwnerOf returns the owner root of an owner-relative destination
// bucket: "recordings/internal/services/replay" is owned by "recordings".
func DestinationOwnerOf(destination string) string {
	return firstPathSegment(strings.TrimSpace(destination))
}

// ValidateUnfinishedMoves checks the consolidated ledger's own schema: required
// fields, a closed destination owner, a successor under that owner, no
// duplicate package paths, and a stable sort. It deliberately does not check
// the live tree; ValidateInventory owns proving each row still names a package
// that exists.
func ValidateUnfinishedMoves(moves UnfinishedMoves) []string {
	var problems []string
	if len(moves.Moves) == 0 {
		return nil
	}
	if moves.Version != 1 {
		problems = append(problems, fmt.Sprintf("unfinished package moves version %d is unsupported; want 1", moves.Version))
	}
	if moves.Stage != UnfinishedMovesStage {
		problems = append(problems, fmt.Sprintf("unfinished package moves stage %q is unsupported; want %q", moves.Stage, UnfinishedMovesStage))
	}
	if moves.SortKey != SortKeyDescription {
		problems = append(problems, "unfinished package moves sortKey must document packagePath ascending byte order")
	}
	if strings.TrimSpace(moves.EndState) == "" {
		problems = append(problems, "unfinished package moves endState must state that the ledger shrinks to zero and is then deleted")
	}

	closed := closedDestinationSet()
	seen := map[string]int{}
	for _, move := range moves.Moves {
		seen[move.PackagePath]++
		problems = append(problems, validateUnfinishedMoveRow(move, closed)...)
	}
	for packagePath, count := range seen {
		if count > 1 {
			problems = append(problems, fmt.Sprintf("%s: duplicate unfinished move row", packagePath))
		}
	}
	if !slices.IsSortedFunc(moves.Moves, func(a, b UnfinishedMoveRow) int {
		return strings.Compare(a.PackagePath, b.PackagePath)
	}) {
		problems = append(problems, "unfinished package moves must be stable-sorted by packagePath")
	}
	slices.Sort(problems)
	return problems
}

func validateUnfinishedMoveRow(move UnfinishedMoveRow, closed map[string]string) []string {
	packagePath := strings.TrimSpace(move.PackagePath)
	if packagePath == "" {
		return []string{"unfinished move row missing packagePath"}
	}
	var problems []string
	if !strings.HasPrefix(packagePath, "pkg/") {
		problems = append(problems, fmt.Sprintf("%s: packagePath must be repository-relative under pkg/", packagePath))
	}
	owner := DestinationOwnerOf(move.Destination)
	if owner == "" {
		problems = append(problems, fmt.Sprintf("%s: missing destination", packagePath))
		return problems
	}
	if closed[owner] != DestinationKindOwner {
		problems = append(problems, fmt.Sprintf("%s: destination %q is outside the closed owner vocabulary", packagePath, move.Destination))
	}
	successor := strings.TrimSpace(move.Successor)
	if successor == "" {
		problems = append(problems, fmt.Sprintf("%s: move requires a successor package path", packagePath))
		return problems
	}
	// A successor may equal its own packagePath: the transitional top-level
	// fold rows record that a package folds into the tree it already sits in,
	// and the deletionCondition is what closes them.
	ownerRoot := "pkg/services/" + owner
	if successor != ownerRoot && !strings.HasPrefix(successor, ownerRoot+"/") {
		problems = append(problems, fmt.Sprintf("%s: successor %q must sit under %s for destination %q", packagePath, successor, ownerRoot, move.Destination))
	}
	return problems
}
