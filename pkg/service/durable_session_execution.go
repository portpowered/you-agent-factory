package service

import (
	"context"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

var _ apisurface.DurableSessionExecutionAPI = (*FactoryService)(nil)

func (fs *FactoryService) StartDurableFactorySessionAsync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionExecutionResponse, error) {
	startReq, err := factorysession.StartRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	result, err := fs.durableExecutionService().StartAsync(ctx, startReq)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	return factorysession.AsyncStartResponseToAPI(result), nil
}

func (fs *FactoryService) StartDurableFactorySessionSync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionSyncExecutionResponse, error) {
	startReq, err := factorysession.StartRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	result, err := fs.durableExecutionService().StartSync(ctx, startReq)
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	return factorysession.SyncStartResponseToAPI(result), nil
}

func (fs *FactoryService) durableExecutionService() factorysessionexecution.Service {
	if fs == nil {
		return factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{})
	}
	fs.durableExecutionMu.Lock()
	defer fs.durableExecutionMu.Unlock()
	if fs.durableExecution == nil {
		fs.durableExecution = factorysessionexecution.NewRuntimeService(factorysessionexecution.StartPrepareContext{
			StartSourceContext: factorysessionexecution.StartSourceContext{
				ProjectRoot: fs.durableProjectRoot(),
			},
		})
	}
	return fs.durableExecution
}

func (fs *FactoryService) durableProjectRoot() string {
	if fs == nil {
		return ""
	}
	if fs.cfg != nil {
		if root := strings.TrimSpace(fs.cfg.ExecutionBaseDir); root != "" {
			return root
		}
		if root := strings.TrimSpace(fs.cfg.Dir); root != "" {
			return root
		}
	}
	return strings.TrimSpace(fs.factoryRootDir)
}
