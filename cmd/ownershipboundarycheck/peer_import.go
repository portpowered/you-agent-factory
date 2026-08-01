package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Exact documented leaf-effect ports that may be imported across owners.
// This is not a private-root catalog; each entry is an approved contract path.
var approvedPeerServiceContractImports = map[string]struct{}{
	"pkg/services/edges\x00github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty":                                      {},
	"pkg/platform/pty\x00github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty":                                        {},
	"pkg/services/edges\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                                                                          {},
	"pkg/services/edges\x00github.com/portpowered/infinite-you/pkg/services/automations":                                                                                             {},
	"pkg/services/factory_runtime\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                                                                {},
	"pkg/services/factory_runtime/internal/services/instance_host/build\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                          {},
	"pkg/services/factory_sessions\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                                                               {},
	"pkg/services/factory_sessions/internal/execution\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                                            {},
	"pkg/services/factory_sessions/internal/runtimeopening\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                                       {},
	"pkg/services/factory_sessions/internal/executionopening\x00github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty": {},
	"pkg/services/factory_sessions/internal/runtimeopening\x00github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty":   {},
	"pkg/services/recordings\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                                                                     {},
	"pkg/services/recordings/internal/services/artifacts_export/artifacts\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                        {},
	"pkg/services/recordings/internal/services/replay/replay\x00github.com/portpowered/infinite-you/pkg/services/providers/wire":                                                     {},
}

func isApprovedPeerServiceContractImport(packagePath, importPath string) bool {
	_, approved := approvedPeerServiceContractImports[packagePath+"\x00"+importPath]
	return approved
}

// peerOwnerFromImport returns the inventoried owner segment for a services import.
func peerOwnerFromImport(importPath string) string {
	if !strings.HasPrefix(importPath, servicesImport) {
		return ""
	}
	remainder := strings.TrimPrefix(importPath, servicesImport)
	owner, _, _ := strings.Cut(remainder, "/")
	return owner
}

// peerViolationMessage names the importer and peer owners and directs
// remediation to the peer root contract.
func peerViolationMessage(item finding) string {
	importer := serviceOwnerForFile(item.FilePath)
	peer := peerOwnerFromImport(item.Target)
	if importer == "" || peer == "" {
		return deletionGates[rulePeerServiceImplementation]
	}
	return fmt.Sprintf(
		`importer owner %q must not import peer owner %q non-root path; import only the peer service root contract (pkg/services/%s)`,
		importer,
		peer,
		peer,
	)
}

// crossOwnerPeerImplementationImport reports whether importPath is a prohibited
// peer non-root import for importerOwner. Peer roots and same-owner paths are
// allowed by this cross-owner rule.
func crossOwnerPeerImplementationImport(
	importerOwner string,
	importPath string,
	inventory ownerInventory,
) bool {
	if importerOwner == "" || !strings.HasPrefix(importPath, servicesImport) {
		return false
	}
	repositoryPath := "pkg/services/" + strings.TrimPrefix(importPath, servicesImport)
	peerOwner, surface := inventory.classify(repositoryPath)
	if peerOwner == "" || surface == surfaceNone {
		// Without an inventoried peer, keep the path-shape heuristic so fixtures
		// and incomplete trees still classify deeper imports as non-root.
		remainder := strings.TrimPrefix(importPath, servicesImport)
		parts := strings.Split(remainder, "/")
		if len(parts) < 2 || parts[0] == "" || parts[0] == importerOwner {
			return false
		}
		return true
	}
	if peerOwner == importerOwner {
		return false
	}
	return surface == surfaceNonRoot
}

func serviceOwnerForFile(relative string) string {
	relative = filepath.ToSlash(relative)
	prefix := servicesRootRelative + "/"
	if !strings.HasPrefix(relative, prefix) {
		return ""
	}
	remainder := strings.TrimPrefix(relative, prefix)
	owner, _, _ := strings.Cut(remainder, "/")
	return owner
}

func scanPeerServiceImports(
	repoRoot string,
	inventory ownerInventory,
) ([]finding, error) {
	servicesRoot := filepath.Join(repoRoot, filepath.FromSlash(servicesRootRelative))
	if _, err := os.Stat(servicesRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var findings []finding
	err := filepath.WalkDir(servicesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		owner := serviceOwnerForFile(relative)
		if owner == "" || !inventory.hasOwner(owner) {
			return nil
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		packagePath := filepath.ToSlash(filepath.Dir(relative))
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if isApprovedPeerServiceContractImport(packagePath, importPath) {
				continue
			}
			if !crossOwnerPeerImplementationImport(owner, importPath, inventory) {
				continue
			}
			findings = append(findings, finding{
				Rule:     rulePeerServiceImplementation,
				FilePath: relative,
				Target:   importPath,
				Line:     fileSet.Position(spec.Pos()).Line,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}
