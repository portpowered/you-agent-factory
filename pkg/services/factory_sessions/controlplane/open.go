package controlplane

import (
	"context"
	"fmt"
	"path/filepath"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/dataplane"
)

// ScaffoldHost materializes a new factory scaffold for init-new-factory opens.
type ScaffoldHost interface {
	InitializeFactoryScaffold(factoryDir string) error
}

// NestedFactoryDirectoryValidator checks whether init-new-factory can safely
// populate its canonical nested directory.
type NestedFactoryDirectoryValidator interface {
	ValidateInitNewFactoryNestedDir(resolvedFolder string) error
}

// OpenControlHost exposes discovery and scaffold seams owned by the composition root.
type OpenControlHost interface {
	DiscoveryHost
	ScaffoldHost
	NestedFactoryDirectoryValidator
	ResolveSessionFolder(string) (string, error)
}

// OpenFromFolder applies session open policy: target discovery, selection,
// validate-only responses, init-new-factory routing, and live runtime open.
func OpenFromFolder(
	ctx context.Context,
	host OpenControlHost,
	liveOpener *dataplane.LiveOpener,
	folderPath string,
	target *factorysessions.TargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*factorysessions.OpenResult, error) {
	if host == nil {
		return nil, fmt.Errorf("factory session open control host is required")
	}
	if initNewFactory {
		return initNewFactoryAndOpenSession(ctx, host, liveOpener, folderPath)
	}

	targets, err := host.DiscoverTargets(folderPath)
	if err != nil {
		if validateOnly {
			if reason, _, ok := factorysessions.ValidationReasonFromError(err); ok && reason == factorysessions.ValidationReasonNotRunnable {
				resolved, resolveErr := host.ResolveSessionFolder(folderPath)
				if resolveErr != nil {
					return nil, resolveErr
				}
				return &factorysessions.OpenResult{
					InitsNewFactory: true,
					FolderPath:      resolved,
				}, nil
			}
		}
		return nil, err
	}

	selectedTarget, err := factorysessions.SelectTarget(targets, target)
	if err != nil {
		return nil, err
	}
	if selectedTarget == nil {
		return &factorysessions.OpenResult{Targets: factorysessions.CloneTargets(targets)}, nil
	}
	if validateOnly {
		return &factorysessions.OpenResult{Targets: factorysessions.CloneTargets(targets)}, nil
	}
	if liveOpener == nil {
		return nil, fmt.Errorf("live session dataplane opener is required")
	}

	sessionID, err := liveOpener.OpenForTarget(ctx, *selectedTarget)
	if err != nil {
		return nil, err
	}
	return &factorysessions.OpenResult{SessionID: sessionID}, nil
}

func initNewFactoryAndOpenSession(
	ctx context.Context,
	host OpenControlHost,
	liveOpener *dataplane.LiveOpener,
	folderPath string,
) (*factorysessions.OpenResult, error) {
	resolvedFolder, err := host.ResolveSessionFolder(folderPath)
	if err != nil {
		return nil, err
	}

	targets, discoverErr := host.DiscoverTargets(folderPath)
	if discoverErr == nil {
		return nil, factorysessions.NewValidationError(
			factorysessions.ValidationReasonNotRunnable,
			"folderPath",
			fmt.Errorf("folder %q already exposes runnable factory targets", resolvedFolder),
		)
	}
	reason, _, ok := factorysessions.ValidationReasonFromError(discoverErr)
	if !ok || reason != factorysessions.ValidationReasonNotRunnable {
		return nil, discoverErr
	}

	if err := host.ValidateInitNewFactoryNestedDir(resolvedFolder); err != nil {
		return nil, err
	}

	nestedFactoryDir := filepath.Join(resolvedFolder, interfaces.FactoryDir)
	if err := host.InitializeFactoryScaffold(nestedFactoryDir); err != nil {
		return nil, err
	}

	targets, err = host.DiscoverTargets(resolvedFolder)
	if err != nil {
		return nil, fmt.Errorf("discover initialized factory targets: %w", err)
	}
	selectedTarget, err := factorysessions.SelectTarget(targets, nil)
	if err != nil {
		return nil, err
	}
	if selectedTarget == nil {
		return nil, fmt.Errorf("initialized factory folder %q did not resolve to a runnable target", resolvedFolder)
	}
	if liveOpener == nil {
		return nil, fmt.Errorf("live session dataplane opener is required")
	}

	sessionID, err := liveOpener.OpenForTarget(ctx, *selectedTarget)
	if err != nil {
		return nil, err
	}
	return &factorysessions.OpenResult{SessionID: sessionID}, nil
}
