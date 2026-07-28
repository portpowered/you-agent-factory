package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	workerinferencemapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerinference"
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

// InvocationOperation is the already-constructed service operation consumed by
// the CLI transport.
type InvocationOperation interface {
	InvokeModel(context.Context, factorysessions.InvocationTarget, string, modelinference.Request) (modelinference.Result, error)
	ResolveModelInvocationFactoryDir(string) (string, error)
	ExportModelInvocationArtifact(string, string) error
}

func invokeModelThroughBootstrap(
	cfg invokeOptions,
	responseMode *factoryapi.ModelInvocationResponseMode,
) (modelinference.Result, error) {
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
		return modelinference.Result{}, err
	}

	logBootstrapInvokeRequest(cfg, bootstrapCfg.FactoryDir)

	result, err := runBootstrapModelInvocation(
		cfg.Context, cfg.Invocation, bootstrapCfg, strings.TrimSpace(cfg.ModelName), request,
	)
	if err != nil {
		return modelinference.Result{}, mapBootstrapModelInvokeError(err)
	}
	return result, nil
}

func resolveBootstrapInvokeConfig(cfg invokeOptions) (InvocationRequest, error) {
	if cfg.Invocation == nil {
		return InvocationRequest{}, fmt.Errorf("models invoke operation is required")
	}
	factoryDir, err := cfg.Invocation.ResolveModelInvocationFactoryDir(cfg.FactoryDir)
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

func runBootstrapModelInvocation(
	ctx context.Context,
	invoke InvocationOperation,
	cfg InvocationRequest,
	modelName string,
	request factoryapi.ModelInvocationRequest,
) (modelinference.Result, error) {
	if ctx == nil {
		return modelinference.Result{}, fmt.Errorf("context is required")
	}
	if invoke == nil {
		return modelinference.Result{}, fmt.Errorf("models invoke operation is required")
	}
	result, err := invoke.InvokeModel(
		ctx,
		factorysessions.InvocationTarget{
			FactoryDir:       cfg.FactoryDir,
			HomeDir:          cfg.HomeDir,
			OperatorDefaults: cfg.OperatorDefaults,
			Logger:           cfg.Logger,
			Verbose:          cfg.Verbose,
		},
		modelName,
		invocationRequestFromGenerated(request),
	)
	if err != nil {
		return modelinference.Result{}, err
	}
	return result, nil
}

func modelInvocationResponseFromResult(result modelinference.Result) factoryapi.ModelInvocationResponse {
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

func generatedResolvedModelInvocationBindings(values []modelinference.ResolvedModelOperationBinding) []factoryapi.ResolvedModelOperationBinding {
	if len(values) == 0 {
		return []factoryapi.ResolvedModelOperationBinding{}
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

func mapBootstrapModelInvokeError(err error) error {
	if err == nil {
		return nil
	}
	if failure, ok := workers.AsInferenceFailure(err); ok {
		return failure
	}
	switch {
	case errors.Is(err, modelinference.ErrNotFound):
		return fmt.Errorf("%w: model not found", ErrModelNotFound)
	case errors.Is(err, modelinference.ErrMissing),
		errors.Is(err, modelinference.ErrLoading),
		errors.Is(err, modelinference.ErrFailed),
		errors.Is(err, modelinference.ErrUnsupported),
		errors.Is(err, modelinference.ErrNotAvailable):
		return err
	case errors.Is(err, modelinference.ErrUnsupportedOperation),
		errors.Is(err, modelinference.ErrUnsupportedResponseMode):
		return err
	default:
		return err
	}
}

func invocationRequestFromGenerated(request factoryapi.ModelInvocationRequest) modelinference.Request {
	domain := modelinference.Request{
		Operation: request.Operation,
		Content:   contentcontract.PartsFromGenerated(request.Content),
		Bindings:  modelBindingsFromGenerated(request.Bindings),
	}
	if request.Options != nil {
		domain.Options = &modelinference.Options{}
		if request.Options.ResponseMode != nil {
			domain.Options.ResponseMode = modelinference.ResponseMode(*request.Options.ResponseMode)
		}
	}
	return domain
}

func modelBindingsFromGenerated(bindings *[]factoryapi.WorkstationOperationBinding) []modelinference.ModelOperationBinding {
	authored := workerinferencemapping.OperationBindingsFromGenerated(bindings)
	result := make([]modelinference.ModelOperationBinding, len(authored))
	for i := range authored {
		result[i] = modelinference.ModelOperationBinding{
			Slot: authored[i].Slot, Config: work.CloneWorkContentParts(authored[i].Config),
			DefaultContent: work.CloneWorkContentParts(authored[i].DefaultContent),
		}
		if authored[i].Selector != nil {
			result[i].Selector = &modelinference.ModelOperationBindingSelector{
				Slot: authored[i].Selector.Slot, Label: authored[i].Selector.Label,
				Type: authored[i].Selector.Type, Role: authored[i].Selector.Role,
			}
		}
	}
	return result
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
