package cli

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type compositionService struct {
	owned  Service
	legacy *httpService
}

func bindCompositionService(
	httpProtocol clihttp.Protocol,
	pullHTTPProtocol clihttp.Protocol,
	invocation InvocationOperation,
	outputFileSystem OutputFileSystem,
	providers ...CompositionScopeProvider,
) Service {
	if httpProtocol == nil || invocation == nil {
		return nil
	}
	if pullHTTPProtocol == nil {
		pullHTTPProtocol = httpProtocol
	}
	legacy := &httpService{
		http: httpProtocol, pullHTTP: pullHTTPProtocol, invocation: invocation,
	}
	cfg := ConfigFromComposition(httpProtocol, invocation, providers...)
	cfg.PullHTTP = pullHTTPProtocol
	cfg.OutputFileSystem = outputFileSystem
	owned := NewService(cfg)
	if owned == nil {
		return legacy
	}
	return &compositionService{owned: owned, legacy: legacy}
}

func (service *compositionService) List(cfg ListConfig) error {
	if service.owned != nil {
		return service.owned.List(cfg)
	}
	return service.legacy.List(cfg)
}

func (service *compositionService) Inspect(cfg InspectConfig) error {
	if service.owned != nil {
		return service.owned.Inspect(cfg)
	}
	return service.legacy.Inspect(cfg)
}

func (service *compositionService) Pull(cfg PullConfig) error {
	if service.owned != nil {
		return service.owned.Pull(cfg)
	}
	return service.legacy.Pull(cfg)
}

func (service *compositionService) Remove(cfg RemoveConfig) error {
	if service.owned != nil {
		return service.owned.Remove(cfg)
	}
	return service.legacy.Remove(cfg)
}

func (service *compositionService) Invoke(cfg InvokeConfig) error {
	if service.owned != nil && service.canInvokeThroughOwned(cfg) {
		return service.owned.Invoke(cfg)
	}
	return service.legacy.Invoke(cfg)
}

func (service *compositionService) canInvokeThroughOwned(cfg InvokeConfig) bool {
	if strings.TrimSpace(cfg.Server) != "" {
		return false
	}
	// Preserve the bootstrap-owned audio export contract. The joined Models
	// path owns inline generic output and JSON projection; legacy audio export
	// still needs the bootstrap stream-file response and artifact exporter.
	if !cfg.JSON && strings.TrimSpace(cfg.OutputPath) != "" {
		return false
	}
	root, ok := service.owned.(*rootService)
	return ok && root != nil && root.openInvokeScope != nil
}
