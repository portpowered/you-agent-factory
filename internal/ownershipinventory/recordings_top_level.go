package ownershipinventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const recordingsRootRelative = "pkg/services/recordings"

// RecordingsTopLevelExpectedRetain lists canonical public top-level children
// under pkg/services/recordings/ that retain at the Recordings owner root.
// The service root package itself (pkg/services/recordings) is also expected
// retain and is handled separately by recordingsMapping.
var RecordingsTopLevelExpectedRetain = []string{
	"internal",
	"transports",
	"wire",
}

// RecordingsTopLevelUnexpected lists transitional public top-level siblings
// that INV-REC-TOPLEVEL classifies as move targets rather than durable retain
// debt at the Recordings owner root. DEL-REC deleted the last transitional
// siblings; the slice stays for closed-inventory helpers and historical tests.
var RecordingsTopLevelUnexpected = []string{}

// ListRecordingsTopLevelChildren returns every live directory name immediately
// under pkg/services/recordings/.
func ListRecordingsTopLevelChildren(root string) ([]string, error) {
	recordingsRoot := filepath.Join(root, filepath.FromSlash(recordingsRootRelative))
	entries, err := os.ReadDir(recordingsRoot)
	if err != nil {
		return nil, fmt.Errorf("read recordings root: %w", err)
	}
	children := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		children = append(children, entry.Name())
	}
	slices.Sort(children)
	return children, nil
}

// ClassifyRecordingsTopLevelChild reports whether name is an expected retain
// child or an inventoried unexpected sibling.
func ClassifyRecordingsTopLevelChild(name string) (kind string, ok bool) {
	if slices.Contains(RecordingsTopLevelExpectedRetain, name) {
		return "expected_retain", true
	}
	if slices.Contains(RecordingsTopLevelUnexpected, name) {
		return "unexpected_move", true
	}
	return "", false
}

// IsRecordingsCanonicalRetainRest reports whether rest (path after recordings/)
// lives under a canonical retain top-level child.
func IsRecordingsCanonicalRetainRest(rest string) bool {
	top, _, _ := strings.Cut(rest, "/")
	if top == "" {
		return false
	}
	return slices.Contains(RecordingsTopLevelExpectedRetain, top)
}

// IsRecordingsUnexpectedTopLevelRest reports whether rest names an unexpected
// public top-level sibling (exact or nested).
func IsRecordingsUnexpectedTopLevelRest(rest string) bool {
	top, _, _ := strings.Cut(rest, "/")
	return slices.Contains(RecordingsTopLevelUnexpected, top)
}

// RecordingsTopLevelInventory returns the closed committed inventory of live
// top-level children: expected retain plus unexpected move siblings.
func RecordingsTopLevelInventory() []string {
	inventory := make([]string, 0, len(RecordingsTopLevelExpectedRetain)+len(RecordingsTopLevelUnexpected))
	inventory = append(inventory, RecordingsTopLevelExpectedRetain...)
	inventory = append(inventory, RecordingsTopLevelUnexpected...)
	slices.Sort(inventory)
	return inventory
}
