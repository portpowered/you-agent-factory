package logicaltarget

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
	"go.uber.org/zap"
)

// Discover lists runnable factory targets under folderPath.
func Discover(folderPath string, probe factorysessions.TargetProbe, directories factorysessions.DirectoryInspection, resolveHome factorysessions.HomeDirectoryResolver) ([]factorysessions.Target, error) {
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
	targets, loadFailures, err := collect(resolvedFolder, probe, directories)
	if err != nil {
		return nil, err
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Ref.Kind != targets[j].Ref.Kind {
			return targets[i].Ref.Kind == factorysessions.TargetKindDefault
		}
		return targets[i].Ref.Name < targets[j].Ref.Name
	})
	if len(targets) == 0 {
		if len(loadFailures) > 0 {
			return nil, sessionvalidation.NewConfigLoadFailed(loadFailures)
		}
		return nil, sessionvalidation.New(factorysessions.ValidationReasonNotRunnable, "folderPath", fmt.Errorf("folder %q does not expose any runnable factory targets", resolvedFolder))
	}
	return targets, nil
}

// DiscoverConfigured loads runnable Factory targets with canonical diagnostics.
func DiscoverConfigured(folderPath string, workstationLoader factorydefinitions.WorkstationLoader, loadFactory factorydefinitions.LoadedFactoryLoader, logger *zap.Logger, directories factorysessions.DirectoryInspection, resolveHome factorysessions.HomeDirectoryResolver) ([]factorysessions.Target, error) {
	return Discover(folderPath, func(folderPath, factoryDir string, ref factorysessions.TargetRef) (factorysessions.Target, bool, *factorysessions.DiscoveryFailure) {
		return probeConfigured(folderPath, factoryDir, ref, workstationLoader, loadFactory, logger)
	}, directories, resolveHome)
}

func probeConfigured(folderPath, factoryDir string, ref factorysessions.TargetRef, workstationLoader factorydefinitions.WorkstationLoader, loadFactory factorydefinitions.LoadedFactoryLoader, logger *zap.Logger) (factorysessions.Target, bool, *factorysessions.DiscoveryFailure) {
	if loadFactory == nil {
		err := errors.New("loaded factory loader is required")
		logProbeFailure(logger, folderPath, factoryDir, ref, err)
		return factorysessions.Target{}, false, &factorysessions.DiscoveryFailure{FactoryDir: factoryDir, Ref: ref, Summary: err.Error()}
	}
	loaded, err := loadFactory(factoryDir, workstationLoader)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound) || errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
			return factorysessions.Target{}, false, nil
		}
		logProbeFailure(logger, folderPath, factoryDir, ref, err)
		return factorysessions.Target{}, false, &factorysessions.DiscoveryFailure{FactoryDir: factoryDir, Ref: ref, Summary: err.Error()}
	}
	project := ""
	if cfg := loaded.FactoryConfig(); cfg != nil {
		project = strings.TrimSpace(cfg.Project)
		if project == "" {
			project = strings.TrimSpace(cfg.Name)
		}
	}
	return Build(folderPath, factoryDir, ref, project), true, nil
}

func logProbeFailure(logger *zap.Logger, folderPath, factoryDir string, ref factorysessions.TargetRef, err error) {
	if err == nil {
		return
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	fields := []zap.Field{zap.String("submitted_folder_path", folderPath), zap.String("target_factory_dir", factoryDir), zap.String("target_kind", string(ref.Kind)), zap.String("target_display_name", DisplayName(ref)), zap.String("failure_summary", err.Error()), zap.Error(err)}
	if ref.Kind == factorysessions.TargetKindNamed && strings.TrimSpace(ref.Name) != "" {
		fields = append(fields, zap.String("target_name", strings.TrimSpace(ref.Name)))
	}
	logger.Error("factory session discovery target runtime config load failed", fields...)
}

func collect(folderPath string, probe factorysessions.TargetProbe, directories factorysessions.DirectoryInspection) ([]factorysessions.Target, []factorysessions.DiscoveryFailure, error) {
	targets := make([]factorysessions.Target, 0, 4)
	failures := make([]factorysessions.DiscoveryFailure, 0, 2)
	appendProbed(&targets, &failures, probe, folderPath, folderPath, factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault})
	entries, err := directories.ReadDir(folderPath)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, nil, sessionvalidation.New(factorysessions.ValidationReasonUnreadable, "folderPath", fmt.Errorf("read factory session folder %s: %w", folderPath, err))
		}
		return nil, nil, fmt.Errorf("read factory session folder %s: %w", folderPath, err)
	}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if !entry.IsDir() || name == "" {
			continue
		}
		if _, err := factorydefinitions.PathSegments(name); err != nil {
			continue
		}
		appendProbed(&targets, &failures, probe, folderPath, filepath.Join(folderPath, name), factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: name})
	}
	return targets, failures, nil
}

func appendProbed(targets *[]factorysessions.Target, failures *[]factorysessions.DiscoveryFailure, probe factorysessions.TargetProbe, folderPath, factoryDir string, ref factorysessions.TargetRef) {
	target, ok, failure := probe(folderPath, factoryDir, ref)
	if ok {
		*targets = append(*targets, target)
	} else if failure != nil {
		*failures = append(*failures, *failure)
	}
}

// Select chooses a discovered target from an optional explicit reference.
func Select(targets []factorysessions.Target, ref *factorysessions.TargetRef) (*factorysessions.Target, error) {
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
	normalized := factorysessions.TargetRef{Kind: ref.Kind, Name: strings.TrimSpace(ref.Name)}
	switch normalized.Kind {
	case factorysessions.TargetKindDefault:
		normalized.Name = ""
	case factorysessions.TargetKindNamed:
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
	return nil, sessionvalidation.New(factorysessions.ValidationReasonTargetNotFound, "target.name", fmt.Errorf("factory session target %q was not found", DisplayName(normalized)))
}

// DisplayName formats a target reference for operator-facing errors.
func DisplayName(ref factorysessions.TargetRef) string {
	if ref.Kind == factorysessions.TargetKindDefault {
		return "default"
	}
	return ref.Name
}

// Clone returns a defensive copy of discovered targets.
func Clone(targets []factorysessions.Target) []factorysessions.Target {
	if len(targets) == 0 {
		return nil
	}
	return append([]factorysessions.Target(nil), targets...)
}

// Build projects loaded factory config into a session target.
func Build(folderPath, factoryDir string, ref factorysessions.TargetRef, projectName string) factorysessions.Target {
	label := "default"
	if ref.Kind == factorysessions.TargetKindNamed {
		label = ref.Name
	}
	return factorysessions.Target{Ref: ref, Label: label, FolderPath: folderPath, FactoryDir: factoryDir, Project: projectName}
}
