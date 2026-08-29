package cli

import (
	"strings"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
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
	inputFileReader InputFileReader,
	now func() time.Time,
	providers ...CompositionScopeProvider,
) Service {
	if httpProtocol == nil || invocation == nil {
		return nil
	}
	if pullHTTPProtocol == nil {
		pullHTTPProtocol = httpProtocol
	}
	cfg := ConfigFromComposition(httpProtocol, invocation, providers...)
	cfg.PullHTTP = pullHTTPProtocol
	cfg.OutputFileSystem = outputFileSystem
	cfg.InputFileReader = inputFileReader
	cfg.Clock = now
	legacy := &httpService{
		http:             httpProtocol,
		pullHTTP:         pullHTTPProtocol,
		invocation:       invocation,
		now:              now,
		models:           cfg.Models,
		openCatalogScope: cfg.OpenCatalogScope,
		openInvokeScope:  cfg.OpenInvokeScope,
		inputFileReader:  inputFileReader,
	}
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
	if len(cfg.InputMappings) > 0 || len(cfg.InputSpecs) > 0 || len(cfg.ParameterSpecs) > 0 {
		return true
	}
	// Existing named-worker models retain the bootstrap audio artifact contract.
	// The effective built-in tts alias is a generic operation, so its legacy
	// --text/--output spelling must share the same owned path as --input text=.
	if !cfg.JSON && strings.TrimSpace(cfg.OutputPath) != "" && !isDirectTTSAlias(cfg) {
		return false
	}
	root, ok := service.owned.(*rootService)
	return ok && root != nil && root.openInvokeScope != nil
}

func isDirectTTSAlias(cfg InvokeConfig) bool {
	return strings.EqualFold(strings.TrimSpace(cfg.ModelName), modelinference.BuiltInModelNameTTS) &&
		strings.EqualFold(strings.TrimSpace(cfg.Operation), modelinference.OperationTTS)
}
