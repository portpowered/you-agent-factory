package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type durableSessionLister func(context.Context, fse.ListSessionsRequest) (fse.ListSessionsResult, error)

var (
	defaultDurableServiceOnce sync.Once
	defaultDurableService     *fse.FakeService
	defaultDurableServiceErr  error
)

func defaultDurableSessionLister(ctx context.Context, req fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
	defaultDurableServiceOnce.Do(func() {
		catalogPath, err := resolveContractFixtureCatalogPath()
		if err != nil {
			defaultDurableServiceErr = err
			return
		}
		defaultDurableService, defaultDurableServiceErr = fse.NewFakeServiceFromContractFixtures(catalogPath)
	})
	if defaultDurableServiceErr != nil {
		return fse.ListSessionsResult{}, defaultDurableServiceErr
	}
	return defaultDurableService.ListSessions(ctx, req)
}

func resolveContractFixtureCatalogPath() (string, error) {
	candidates := make([]string, 0, 2)
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if execPath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(execPath))
	}
	relativeCatalog := filepath.FromSlash(fixtures.ContractFixtureCatalogRelativePath)
	for _, start := range candidates {
		root, err := findRepoRoot(start)
		if err != nil {
			continue
		}
		catalogPath := filepath.Join(root, relativeCatalog)
		if _, err := os.Stat(catalogPath); err == nil {
			return catalogPath, nil
		}
	}
	return "", fmt.Errorf(
		"durable session fixture catalog not found at %s",
		fixtures.ContractFixtureCatalogRelativePath,
	)
}

func findRepoRoot(startDir string) (string, error) {
	current := filepath.Clean(startDir)
	for {
		goModPath := filepath.Join(current, "go.mod")
		if info, err := os.Stat(goModPath); err == nil && !info.IsDir() {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}

func listDurableSessions(
	ctx context.Context,
	lister durableSessionLister,
	req fse.ListSessionsRequest,
) (fse.ListSessionsResult, error) {
	if lister == nil {
		lister = defaultDurableSessionLister
	}
	return lister(ctx, req)
}

func mergeScopedListResult(
	ctx context.Context,
	cfg ListConfig,
	normalized fse.ListSessionsRequest,
	liveSessions []fse.LiveSessionSummary,
) (fse.ListSessionsResult, error) {
	needsDurable := normalized.Scope == fse.SessionListScopePersisted || normalized.Scope == fse.SessionListScopeAll
	if !needsDurable {
		return fse.ApplySessionListScope(fse.ListSessionsResult{
			Scope:        normalized.Scope,
			LiveSessions: liveSessions,
		}, normalized), nil
	}

	durableResult, err := listDurableSessions(ctx, cfg.DurableLister, fse.ListSessionsRequest{
		Scope: fse.SessionListScopeAll,
	})
	if err != nil {
		return fse.ListSessionsResult{}, fmt.Errorf("list durable factory sessions failed: %w", err)
	}

	return fse.ApplySessionListScope(fse.ListSessionsResult{
		Scope:           normalized.Scope,
		LiveSessions:    liveSessions,
		DurableSessions: durableResult.DurableSessions,
	}, normalized), nil
}

func listResponseFromScopedResult(result fse.ListSessionsResult) factoryapi.ListFactorySessionsResponse {
	return factorysession.ListSessionsResponseToAPI(result)
}
