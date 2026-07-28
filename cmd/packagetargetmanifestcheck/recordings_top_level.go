package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const recordingsRootRelative = "pkg/services/recordings"

// recordingsTopLevelExpectedRetain lists canonical public top-level children
// under pkg/services/recordings/ that retain at the Recordings owner root.
var recordingsTopLevelExpectedRetain = []string{
	"internal",
	"transports",
	"wire",
}

// recordingsTopLevelUnexpected lists transitional public top-level siblings
// that INV-REC-TOPLEVEL classifies as move targets rather than durable retain
// debt at the Recordings owner root.
var recordingsTopLevelUnexpected = []string{
	"artifacts",
	"events",
	"projections",
	"replay",
	"service",
}

func listRecordingsTopLevelChildren(root string) ([]string, error) {
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

func recordingsTopLevelInventory() []string {
	inventory := make([]string, 0, len(recordingsTopLevelExpectedRetain)+len(recordingsTopLevelUnexpected))
	inventory = append(inventory, recordingsTopLevelExpectedRetain...)
	inventory = append(inventory, recordingsTopLevelUnexpected...)
	slices.Sort(inventory)
	return inventory
}

func recordingsCanonicalRetainRest(rest string) bool {
	top, _, _ := strings.Cut(rest, "/")
	if top == "" {
		return false
	}
	return slices.Contains(recordingsTopLevelExpectedRetain, top)
}

func recordingsUnexpectedTopLevelRest(rest string) bool {
	top, _, _ := strings.Cut(rest, "/")
	return slices.Contains(recordingsTopLevelUnexpected, top)
}
