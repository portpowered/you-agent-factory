package config

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// BuiltInNamedFactoryInitOutcome reports whether init created or skipped one
// packaged default factory.
type BuiltInNamedFactoryInitOutcome string

const (
	BuiltInNamedFactoryCreated BuiltInNamedFactoryInitOutcome = "created"
	BuiltInNamedFactorySkipped BuiltInNamedFactoryInitOutcome = "skipped"
)

// BuiltInNamedFactoryInitResult summarizes one packaged default factory ensure
// operation under a global named-factories root.
type BuiltInNamedFactoryInitResult struct {
	Name       string
	FactoryDir string
	Outcome    BuiltInNamedFactoryInitOutcome
}

// BuiltInNamedFactoryNames returns the sorted canonical names for packaged
// default factories materialized during you config init.
func BuiltInNamedFactoryNames() []string {
	return builtInNamedFactoryNamesSorted()
}

// EnsureBuiltInNamedFactories materializes each packaged default factory under
// globalRoot using the hierarchical named-factories layout. Existing factory
// directories are left unchanged.
func EnsureBuiltInNamedFactories(globalRoot string) ([]BuiltInNamedFactoryInitResult, error) {
	globalRoot = strings.TrimSpace(globalRoot)
	if globalRoot == "" {
		return nil, fmt.Errorf("global factory root is required")
	}

	names := builtInNamedFactoryNamesSorted()
	results := make([]BuiltInNamedFactoryInitResult, 0, len(names))
	for _, name := range names {
		factoryDir, outcome, err := ensureBuiltInNamedFactoryWithOutcome(globalRoot, name)
		if err != nil {
			return nil, err
		}
		results = append(results, BuiltInNamedFactoryInitResult{
			Name:       name,
			FactoryDir: factoryDir,
			Outcome:    outcome,
		})
	}
	return results, nil
}

func builtInNamedFactoryNamesSorted() []string {
	names := make([]string, 0, len(builtInNamedFactoryCatalog))
	for name := range builtInNamedFactoryCatalog {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ensureBuiltInNamedFactoryWithOutcome(
	globalRoot, canonicalName string,
) (string, BuiltInNamedFactoryInitOutcome, error) {
	payload, ok := builtInNamedFactoryCatalog[canonicalName]
	if !ok {
		return "", "", fmt.Errorf("unknown built-in named factory %q", canonicalName)
	}

	targetDir, err := MapNamedFactoryDir(globalRoot, canonicalName)
	if err != nil {
		return "", "", fmt.Errorf("materialize packaged default factory %q in global root %s: %w", canonicalName, globalRoot, err)
	}
	if _, err := os.Stat(targetDir); err == nil {
		if err := requireFactoryConfig(targetDir); err != nil {
			return "", "", fmt.Errorf(
				"materialize packaged default factory %q in global root %s: existing target invalid: %w",
				canonicalName,
				globalRoot,
				err,
			)
		}
		upgradedDir, err := upgradeMaterializedBuiltInNamedFactoryIfNeeded(globalRoot, canonicalName, targetDir)
		if err != nil {
			return "", "", err
		}
		return upgradedDir, BuiltInNamedFactorySkipped, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf(
			"materialize packaged default factory %q in global root %s: check existing target: %w",
			canonicalName,
			globalRoot,
			err,
		)
	}

	factoryDir, err := PersistNamedFactory(globalRoot, canonicalName, payload)
	if err != nil {
		return "", "", fmt.Errorf("materialize packaged default factory %q in global root %s: %w", canonicalName, globalRoot, err)
	}
	return factoryDir, BuiltInNamedFactoryCreated, nil
}
