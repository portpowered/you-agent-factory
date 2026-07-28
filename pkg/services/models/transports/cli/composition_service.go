package cli

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type compositionService struct {
	owned  Service
	legacy *httpService
}

func bindCompositionService(httpProtocol clihttp.Protocol, invocation InvocationOperation) Service {
	if httpProtocol == nil || invocation == nil {
		return nil
	}
	legacy := &httpService{http: httpProtocol, invocation: invocation}
	owned := NewService(ConfigFromComposition(httpProtocol, invocation))
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
	// Owned invoke currently supports JSON metadata only. Audio export still
	// routes through the bootstrap-owned factory invocation path until the
	// scoped inference runtime streams artifacts for AUDIO_STREAM.
	if !cfg.JSON {
		return false
	}
	root, ok := service.owned.(*rootService)
	return ok && root != nil && root.openInvokeScope != nil
}
