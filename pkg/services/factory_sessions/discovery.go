package factorysessions

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

// TargetProbe loads metadata for one discovered factory session target.
type TargetProbe func(folderPath string, factoryDir string, ref TargetRef) (Target, bool, *DiscoveryFailure)

// DiscoveryFailure captures one target that looked like a factory but failed to load.
type DiscoveryFailure struct {
	FactoryDir string
	Ref        TargetRef
	Summary    string
}

// DiscoverTargets lists runnable factory targets under folderPath.
func DiscoverTargets(
	folderPath string,
	probe TargetProbe,
	directories DirectoryInspection,
	resolveHome HomeDirectoryResolver,
) ([]Target, error) {
	resolvedFolder, err := ResolveSessionFolder(folderPath, resolveHome, directories)
	if err != nil {
		return nil, err
	}
	if probe == nil {
		return nil, fmt.Errorf("factory session target probe is required")
	}
	if directories == nil {
		return nil, fmt.Errorf("factory session directory inspection is required")
	}

	targets, loadFailures, err := collectDiscoveredTargets(resolvedFolder, probe, directories)
	if err != nil {
		return nil, err
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
		if len(loadFailures) > 0 {
			return nil, NewConfigLoadFailedError(loadFailures)
		}
		return nil, NewValidationError(
			validationReasonNotRunnable,
			"folderPath",
			fmt.Errorf("folder %q does not expose any runnable factory targets", resolvedFolder),
		)
	}
	return targets, nil
}

// DiscoverConfiguredTargets loads runnable Factory targets with the canonical
// config-error classification and structured diagnostics.
func DiscoverConfiguredTargets(
	folderPath string,
	workstationLoader interfaces.WorkstationLoader,
	loadFactory interfaces.LoadedFactoryLoader,
	logger *zap.Logger,
	directories DirectoryInspection,
	resolveHome HomeDirectoryResolver,
) ([]Target, error) {
	return DiscoverTargets(folderPath, func(folderPath, factoryDir string, ref TargetRef) (Target, bool, *DiscoveryFailure) {
		return ProbeConfiguredTarget(folderPath, factoryDir, ref, workstationLoader, loadFactory, logger)
	}, directories, resolveHome)
}

// ProbeConfiguredTarget loads one candidate target using the canonical
// discovery error and diagnostic policy.
func ProbeConfiguredTarget(
	folderPath, factoryDir string,
	ref TargetRef,
	workstationLoader interfaces.WorkstationLoader,
	loadFactory interfaces.LoadedFactoryLoader,
	logger *zap.Logger,
) (Target, bool, *DiscoveryFailure) {
	if loadFactory == nil {
		err := errors.New("loaded factory loader is required")
		logTargetProbeFailure(logger, folderPath, factoryDir, ref, err)
		return Target{}, false, &DiscoveryFailure{
			FactoryDir: factoryDir, Ref: ref, Summary: err.Error(),
		}
	}
	loaded, err := loadFactory(factoryDir, workstationLoader)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) ||
			errors.Is(err, interfaces.ErrFactoryLayoutNotFound) ||
			errors.Is(err, interfaces.ErrNamedFactoryNotFound) {
			return Target{}, false, nil
		}
		logTargetProbeFailure(logger, folderPath, factoryDir, ref, err)
		return Target{}, false, &DiscoveryFailure{
			FactoryDir: factoryDir, Ref: ref, Summary: err.Error(),
		}
	}
	project := ""
	if cfg := loaded.FactoryConfig(); cfg != nil {
		project = strings.TrimSpace(cfg.Project)
		if project == "" {
			project = strings.TrimSpace(cfg.Name)
		}
	}
	return BuildTargetFromConfig(folderPath, factoryDir, ref, project), true, nil
}

func logTargetProbeFailure(logger *zap.Logger, folderPath, factoryDir string, ref TargetRef, err error) {
	if err == nil {
		return
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	fields := []zap.Field{
		zap.String("submitted_folder_path", folderPath),
		zap.String("target_factory_dir", factoryDir),
		zap.String("target_kind", string(ref.Kind)),
		zap.String("target_display_name", TargetDisplayName(ref)),
		zap.String("failure_summary", err.Error()),
		zap.Error(err),
	}
	if ref.Kind == TargetKindNamed && strings.TrimSpace(ref.Name) != "" {
		fields = append(fields, zap.String("target_name", strings.TrimSpace(ref.Name)))
	}
	logger.Error("factory session discovery target runtime config load failed", fields...)
}

func collectDiscoveredTargets(
	folderPath string,
	probe TargetProbe,
	directories DirectoryInspection,
) ([]Target, []DiscoveryFailure, error) {
	targets := make([]Target, 0, 4)
	loadFailures := make([]DiscoveryFailure, 0, 2)
	appendProbedTarget(&targets, &loadFailures, probe, folderPath, folderPath, TargetRef{
		Kind: TargetKindDefault,
	})

	childEntries, err := directories.ReadDir(folderPath)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, nil, NewValidationError(
				validationReasonUnreadable,
				"folderPath",
				fmt.Errorf("read factory session folder %s: %w", folderPath, err),
			)
		}
		return nil, nil, fmt.Errorf("read factory session folder %s: %w", folderPath, err)
	}
	for _, entry := range childEntries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if _, err := interfaces.PathSegments(name); err != nil {
			continue
		}
		appendProbedTarget(&targets, &loadFailures, probe, folderPath, filepath.Join(folderPath, name), TargetRef{
			Kind: TargetKindNamed,
			Name: name,
		})
	}
	return targets, loadFailures, nil
}

func appendProbedTarget(targets *[]Target, failures *[]DiscoveryFailure, probe TargetProbe, folderPath string, factoryDir string, ref TargetRef) {
	target, ok, failure := probe(folderPath, factoryDir, ref)
	if ok {
		*targets = append(*targets, target)
		return
	}
	if failure != nil {
		*failures = append(*failures, *failure)
	}
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
