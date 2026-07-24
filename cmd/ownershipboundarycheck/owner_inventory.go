package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	servicesRootRelative = "pkg/services"
	processEdgesOwner    = "edges"
)

type ownerKind string

const (
	ownerKindService      ownerKind = "service"
	ownerKindProcessEdges ownerKind = "process-edges"
)

type packageSurface string

const (
	surfaceNone    packageSurface = ""
	surfaceRoot    packageSurface = "root"
	surfaceNonRoot packageSurface = "non-root"
)

// ownerInventory is the committed Packaged Service Structure service-tree
// identity used for cross-owner import classification. Owners are derived from
// direct children of pkg/services; Process Edges is classified specially.
type ownerInventory struct {
	owners map[string]ownerKind
}

func loadOwnerInventory(repoRoot string) (ownerInventory, error) {
	inventory := ownerInventory{owners: map[string]ownerKind{}}
	servicesRoot := filepath.Join(repoRoot, filepath.FromSlash(servicesRootRelative))
	entries, err := os.ReadDir(servicesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return inventory, nil
		}
		return ownerInventory{}, fmt.Errorf("read service owner inventory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		kind := ownerKindService
		if name == processEdgesOwner {
			kind = ownerKindProcessEdges
		}
		inventory.owners[name] = kind
	}
	return inventory, nil
}

func (inventory ownerInventory) hasOwner(name string) bool {
	_, ok := inventory.owners[name]
	return ok
}

func (inventory ownerInventory) kind(name string) ownerKind {
	return inventory.owners[name]
}

// classify reports the owning service and whether repositoryPath is that
// owner's root contract surface or a deeper non-root path. Paths outside an
// inventoried owner return surfaceNone.
func (inventory ownerInventory) classify(repositoryPath string) (string, packageSurface) {
	repositoryPath = filepath.ToSlash(repositoryPath)
	prefix := servicesRootRelative + "/"
	if repositoryPath == servicesRootRelative || !strings.HasPrefix(repositoryPath, prefix) {
		return "", surfaceNone
	}
	remainder := strings.TrimPrefix(repositoryPath, prefix)
	if remainder == "" {
		return "", surfaceNone
	}
	parts := strings.Split(remainder, "/")
	owner := parts[0]
	if owner == "" || !inventory.hasOwner(owner) {
		return "", surfaceNone
	}
	if len(parts) == 1 {
		return owner, surfaceRoot
	}
	return owner, surfaceNonRoot
}
