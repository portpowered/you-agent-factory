package factorydefinitions

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	sharedFactoryHomeDirName = ".you-agent-factory"
	namedFactoriesDirName    = "you-agent-factories"
	projectFactoriesDirName  = "factory"
)

// NamedFactoryRoots contains the ordered project and global catalog roots used
// for one named-Factory lookup.
type NamedFactoryRoots struct {
	Project string
	Global  string
}

// NamedFactoriesRoot returns the Factory Definitions-owned global catalog root.
func NamedFactoriesRoot(homeDir string) string {
	return filepath.Join(homeDir, sharedFactoryHomeDirName, namedFactoriesDirName)
}

func NamedFactoriesRootForHome(homeDir string) (string, error) {
	trimmed := strings.TrimSpace(homeDir)
	if trimmed == "" {
		return "", fmt.Errorf("user home directory is required")
	}
	return NamedFactoriesRoot(trimmed), nil
}

func ProjectFactoriesRoot(workingDir string) string {
	return filepath.Join(workingDir, projectFactoriesDirName)
}

func ProjectFactoriesRootForWorkingDir(workingDir string) (string, error) {
	trimmed := strings.TrimSpace(workingDir)
	if trimmed == "" {
		return "", fmt.Errorf("working directory is required")
	}
	return ProjectFactoriesRoot(trimmed), nil
}

// ResolveNamedFactoryRoots derives both catalog roots from explicit process
// edges. The project root remains first in lookup order.
func ResolveNamedFactoryRoots(homeDir, workingDir string) (NamedFactoryRoots, error) {
	global, err := NamedFactoriesRootForHome(homeDir)
	if err != nil {
		return NamedFactoryRoots{}, err
	}
	project, err := ProjectFactoriesRootForWorkingDir(workingDir)
	if err != nil {
		return NamedFactoryRoots{}, err
	}
	return NamedFactoryRoots{Project: project, Global: global}, nil
}
