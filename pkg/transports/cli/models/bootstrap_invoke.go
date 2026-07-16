package models

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"go.uber.org/zap"
)

// InvocationRequest carries normalized CLI inputs to the application
// composition boundary. It contains no service or runtime construction policy.
type InvocationRequest struct {
	FactoryDir       string
	HomeDir          string
	OperatorDefaults operatorconfig.ResolvedDefaults
	Logger           *zap.Logger
	Verbose          bool
}

// InvocationRunner is the already-constructed model invocation collaborator
// consumed by the CLI transport.
type InvocationRunner interface {
	Run(context.Context) error
	InvokeModel(context.Context, string, factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error)
	GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error)
	CloseFactorySession(context.Context, string) error
}

// InvocationBuilder constructs the model collaborator outside the transport.
type InvocationBuilder func(context.Context, InvocationRequest) (InvocationRunner, error)

func invokeModelThroughBootstrap(
	cfg invokeOptions,
	responseMode *factoryapi.ModelInvocationResponseMode,
) (apisurface.ModelInvocationResult, error) {
	request := factoryapi.ModelInvocationRequest{
		Operation: cfg.Operation,
		Content: &factoryapi.WorkContent{
			mustGeneratedTextContentPart(cfg.Text),
		},
	}
	if responseMode != nil {
		request.Options = &factoryapi.ModelInvocationOptions{
			ResponseMode: responseMode,
		}
	}

	bootstrapCfg, err := resolveBootstrapInvokeConfig(cfg)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}

	logBootstrapInvokeRequest(cfg, bootstrapCfg.FactoryDir)

	ctx, cancel := context.WithTimeout(context.Background(), modelsRequestTimeout)
	defer cancel()

	result, err := runBootstrapModelInvocation(ctx, cfg.BuildInvocation, bootstrapCfg, strings.TrimSpace(cfg.ModelName), request)
	if err != nil {
		return apisurface.ModelInvocationResult{}, mapBootstrapModelInvokeError(err)
	}
	return result, nil
}

func resolveBootstrapInvokeConfig(cfg invokeOptions) (InvocationRequest, error) {
	factoryDir, err := resolveModelsInvokeFactoryDir(cfg.FactoryDir)
	if err != nil {
		return InvocationRequest{}, err
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return InvocationRequest{
		FactoryDir:       factoryDir,
		HomeDir:          cfg.HomeDir,
		OperatorDefaults: cfg.OperatorDefaults,
		Logger:           logger,
		Verbose:          cfg.Verbose,
	}, nil
}

func resolveModelsInvokeFactoryDir(factoryDir string) (string, error) {
	if root := strings.TrimSpace(factoryDir); root != "" {
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve models invoke factory root: %w", err)
	}
	return factoryconfig.ResolveCurrentFactoryDir(filepath.Join(cwd, defaultcmd.FactoryDir))
}

func runBootstrapModelInvocation(
	ctx context.Context,
	build InvocationBuilder,
	cfg InvocationRequest,
	modelName string,
	request factoryapi.ModelInvocationRequest,
) (apisurface.ModelInvocationResult, error) {
	if build == nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("models invoke collaborator builder is required")
	}
	invoker, err := build(ctx, cfg)
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}
	if invoker == nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("models invoke collaborator builder returned nil runner")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- invoker.Run(runCtx)
	}()

	if err := waitForModelBootstrapSessionReady(runCtx, invoker, runErrCh); err != nil {
		return apisurface.ModelInvocationResult{}, err
	}

	result, err := invoker.InvokeModel(runCtx, modelName, request)
	if releaseErr := releaseModelBootstrapSession(runCtx, invoker, factorysessions.DefaultSessionID); releaseErr != nil && err == nil {
		err = releaseErr
	}
	cancel()
	runErr := <-runErrCh
	if err != nil {
		return apisurface.ModelInvocationResult{}, err
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return apisurface.ModelInvocationResult{}, runErr
	}
	return result, nil
}

func waitForModelBootstrapSessionReady(
	ctx context.Context,
	invoker InvocationRunner,
	runErrCh <-chan error,
) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if _, err := invoker.GetCurrentFactoryForSession(ctx, factorysessions.DefaultSessionID); err == nil {
			return nil
		} else if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			return err
		}

		select {
		case err := <-runErrCh:
			if err == nil || errors.Is(err, context.Canceled) {
				return fmt.Errorf("models invoke bootstrap stopped before session became ready")
			}
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func releaseModelBootstrapSession(ctx context.Context, invoker InvocationRunner, sessionID string) error {
	if invoker == nil {
		return fmt.Errorf("models invoke bootstrap runner is required")
	}
	if err := invoker.CloseFactorySession(ctx, sessionID); err != nil {
		if errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			return nil
		}
		return fmt.Errorf("release models invoke bootstrap session: %w", err)
	}
	return nil
}

func modelInvocationResponseFromResult(result apisurface.ModelInvocationResult) factoryapi.ModelInvocationResponse {
	return factoryapi.ModelInvocationResponse{
		ModelName:        result.ModelName,
		Worker:           result.Worker,
		Operation:        result.Operation,
		ProviderLocality: factoryapi.WorkerModelLocality(result.ProviderLocality),
		Content:          derefGeneratedWorkContent(contentcontract.GeneratedPtrFromParts(result.Content)),
		Bindings:         generatedResolvedModelInvocationBindings(result.Bindings),
	}
}

func derefGeneratedWorkContent(content *factoryapi.WorkContent) factoryapi.WorkContent {
	if content == nil {
		return nil
	}
	return *content
}

func generatedResolvedModelInvocationBindings(values []workerexecution.ResolvedModelOperationBinding) []factoryapi.ResolvedModelOperationBinding {
	if len(values) == 0 {
		return nil
	}
	bindings := make([]factoryapi.ResolvedModelOperationBinding, 0, len(values))
	for _, binding := range values {
		content := contentcontract.GeneratedPtrFromParts(binding.Content)
		bindings = append(bindings, factoryapi.ResolvedModelOperationBinding{
			Slot:    binding.Slot,
			Source:  factoryapi.ResolvedModelOperationBindingSource(binding.Source),
			Content: derefGeneratedWorkContent(content),
		})
	}
	return bindings
}

func copyModelInvocationStreamFile(streamFile, outputPath string) error {
	input, err := os.Open(streamFile)
	if err != nil {
		return fmt.Errorf("open streamed invocation output: %w", err)
	}
	defer input.Close()

	output, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

func mapBootstrapModelInvokeError(err error) error {
	if err == nil {
		return nil
	}
	if failure, ok := apisurface.AsInferenceFailure(err); ok {
		return failure
	}
	switch {
	case errors.Is(err, apisurface.ErrModelNotFound):
		return fmt.Errorf("%w: model not found", ErrModelNotFound)
	case apisurface.IsManagedRuntimeInvocationBlocked(err),
		apisurface.IsManagedRuntimeMissing(err),
		errors.Is(err, apisurface.ErrModelNotAvailable):
		return err
	case errors.Is(err, apisurface.ErrModelInvocationUnsupportedOperation),
		errors.Is(err, apisurface.ErrModelInvocationUnsupportedMode):
		return err
	default:
		return err
	}
}

func logBootstrapInvokeRequest(cfg invokeOptions, factoryDir string) {
	requestBytes := len(strings.TrimSpace(cfg.Text))
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"models invoke bootstrap request factoryDir=%s modelName=%q operation=%q outputPath=%s requestBytes=%d",
		factoryDir,
		strings.TrimSpace(cfg.ModelName),
		cfg.Operation,
		cfg.OutputPath,
		requestBytes,
	)
}

func logBootstrapInvokeResponse(cfg invokeOptions, summary string) {
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"models invoke bootstrap response modelName=%q operation=%q %s",
		strings.TrimSpace(cfg.ModelName),
		cfg.Operation,
		strings.TrimSpace(summary),
	)
}
