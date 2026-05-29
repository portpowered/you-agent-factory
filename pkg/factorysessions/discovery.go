package factorysessions

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

// TargetProbe loads metadata for one discovered factory session target.
type TargetProbe func(folderPath string, factoryDir string, ref TargetRef) (Target, bool)

// DiscoverTargets lists runnable factory targets under folderPath.
func DiscoverTargets(folderPath string, probe TargetProbe) ([]Target, error) {
	resolvedFolder, err := ResolveSessionFolder(folderPath)
	if err != nil {
		return nil, err
	}
	if probe == nil {
		return nil, fmt.Errorf("factory session target probe is required")
	}

	targets := make([]Target, 0, 4)
	if target, ok := probe(resolvedFolder, resolvedFolder, TargetRef{
		Kind: TargetKindDefault,
	}); ok {
		targets = append(targets, target)
	}

	childEntries, err := os.ReadDir(resolvedFolder)
	if err != nil {
		if os.IsPermission(err) {
			return nil, NewValidationError(
				validationReasonUnreadable,
				"folderPath",
				fmt.Errorf("read factory session folder %s: %w", resolvedFolder, err),
			)
		}
		return nil, fmt.Errorf("read factory session folder %s: %w", resolvedFolder, err)
	}
	for _, entry := range childEntries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if err := factoryconfig.ValidateNamedFactoryName(name); err != nil {
			continue
		}
		targetDir := filepath.Join(resolvedFolder, name)
		target, ok := probe(resolvedFolder, targetDir, TargetRef{
			Kind: TargetKindNamed,
			Name: name,
		})
		if ok {
			targets = append(targets, target)
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		left := targets[i]
		right := targets[j]
		if left.Ref.Kind != right.Ref.Kind {
			return left.Ref.Kind == TargetKindDefault
		}
		return left.Ref.Name < right.Ref.Name
	})
	if len(targets) == 0 {
		return nil, NewValidationError(
			validationReasonNotRunnable,
			"folderPath",
			fmt.Errorf("folder %q does not expose any runnable factory targets", resolvedFolder),
		)
	}
	return targets, nil
}

// SelectTarget chooses a discovered target from an optional explicit ref.
func SelectTarget(targets []Target, ref *TargetRef) (*Target, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("factory session target list is empty")
	}
	if ref == nil {
		if len(targets) == 1 {
			target := targets[0]
			return &target, nil
		}
		return nil, nil
	}

	normalized := TargetRef{
		Kind: ref.Kind,
		Name: strings.TrimSpace(ref.Name),
	}
	switch normalized.Kind {
	case TargetKindDefault:
		normalized.Name = ""
	case TargetKindNamed:
		if normalized.Name == "" {
			return nil, fmt.Errorf("named factory session target requires a name")
		}
	default:
		return nil, fmt.Errorf("unsupported factory session target kind %q", normalized.Kind)
	}

	for i := range targets {
		if targets[i].Ref == normalized {
			target := targets[i]
			return &target, nil
		}
	}
	return nil, NewValidationError(
		validationReasonTargetNotFound,
		"target.name",
		fmt.Errorf("factory session target %q was not found", TargetDisplayName(normalized)),
	)
}

// TargetDisplayName formats a target ref for operator-facing errors.
func TargetDisplayName(ref TargetRef) string {
	if ref.Kind == TargetKindDefault {
		return "default"
	}
	return ref.Name
}

// CloneTargets returns a defensive copy of discovered targets.
func CloneTargets(targets []Target) []Target {
	if len(targets) == 0 {
		return nil
	}
	cloned := make([]Target, len(targets))
	copy(cloned, targets)
	return cloned
}

// BuildTargetFromConfig projects loaded factory config into a session target.
func BuildTargetFromConfig(
	folderPath string,
	factoryDir string,
	ref TargetRef,
	projectName string,
) Target {
	label := "default"
	if ref.Kind == TargetKindNamed {
		label = ref.Name
	}
	return Target{
		Ref:        ref,
		Label:      label,
		FolderPath: folderPath,
		FactoryDir: factoryDir,
		Project:    projectName,
	}
}
