package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/factory/sessions/invocation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	contentcontract "github.com/portpowered/infinite-you/pkg/work/content/contract"
	workinvocation "github.com/portpowered/infinite-you/pkg/work/invocation"
)

// invokeJavaScriptFactorySession adapts the public invocation contract to the
// canonical JavaScript execution owner. JavaScript sessions do not submit Petri
// Work, so they must not pass through the Petri invocation waiter.
func (fs *FactoryService) invokeJavaScriptFactorySession(
	ctx context.Context,
	sessionID string,
	cfg *interfaces.FactoryConfig,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	resolved, err := sessioninvocation.ResolveSessionInvocationInput(cfg, request)
	if err != nil {
		return apisurface.FactoryInvocationResult{}, err
	}
	args, err := javascriptInvocationArgs(cfg, resolved.NormalizedArguments)
	if err != nil {
		return apisurface.FactoryInvocationResult{}, err
	}
	source, err := fs.javaScriptInvocationSource(sessionID, cfg)
	if err != nil {
		return apisurface.FactoryInvocationResult{}, err
	}
	requestID := "invocation-" + factorysessions.NewSessionID()
	if request.RequestId != nil && strings.TrimSpace(*request.RequestId) != "" {
		requestID = strings.TrimSpace(*request.RequestId)
	}
	started, err := fs.durableExecutionService().StartSync(ctx, factorysessionexecution.StartRequest{
		RequestID: requestID,
		Source:    source,
		Args:      args,
	})
	if err != nil {
		return apisurface.FactoryInvocationResult{}, &interfaces.RequestValidationError{Message: err.Error()}
	}
	return javaScriptInvocationResult(requestID, started)
}

func (fs *FactoryService) javaScriptInvocationSource(sessionID string, cfg *interfaces.FactoryConfig) (factorysessionexecution.Source, error) {
	if cfg == nil || cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil {
		return factorysessionexecution.Source{}, fmt.Errorf("JavaScript factory configuration is required")
	}
	jsCfg := cfg.Orchestrator.JavaScript
	content := ""
	if jsCfg.InlineSource != nil {
		content = strings.TrimSpace(jsCfg.InlineSource.Inline)
	}
	sourceRef := strings.TrimSpace(jsCfg.SourceRef)
	if content == "" && sourceRef == "" {
		return factorysessionexecution.Source{}, fmt.Errorf("JavaScript factory has no workflow source")
	}
	var factoryDir string
	if content == "" {
		runtimeCfg, err := fs.sessionRuntimeConfig(sessionID)
		if err != nil {
			return factorysessionexecution.Source{}, err
		}
		factoryDir = runtimeCfg.FactoryDir()
		content, err = workflowvalidation.FileSourceReader(factoryDir).ReadWorkflowSource(sourceRef)
		if err != nil {
			return factorysessionexecution.Source{}, fmt.Errorf("read JavaScript factory workflow %q: %w", sourceRef, err)
		}
	}
	if sourceRef == "" {
		sourceRef = "inline"
	}
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = filepath.Base(factoryDir)
	}
	return factorysessionexecution.Source{
		Kind: workflowsource.KindInlineWorkflow,
		InlineWorkflow: &factorysessionexecution.InlineWorkflowSource{
			InlineSource: content,
			Metadata: map[string]string{
				"sourceRef": fmt.Sprintf("factory:%s:%s", name, filepath.ToSlash(sourceRef)),
			},
			Agents:        jsCfg.Agents,
			ArgsSchema:    jsCfg.ArgsSchema,
			DefaultPolicy: jsCfg.DefaultPolicy,
		},
	}, nil
}

func javascriptInvocationArgs(cfg *interfaces.FactoryConfig, normalized *workinvocation.NormalizedArguments) (map[string]any, error) {
	if normalized == nil || len(normalized.Arguments) == 0 {
		return nil, nil
	}
	types, err := javaScriptArgumentTypes(cfg)
	if err != nil {
		return nil, err
	}
	args := make(map[string]any, len(normalized.Arguments))
	for name, argument := range normalized.Arguments {
		if len(argument.Values) != 1 {
			return nil, &interfaces.RequestValidationError{Message: fmt.Sprintf("argument %q must have exactly one value", name)}
		}
		value, err := coerceJavaScriptArgument(name, argument.Values[0], types[name])
		if err != nil {
			return nil, err
		}
		args[name] = value
	}
	return args, nil
}

func javaScriptArgumentTypes(cfg *interfaces.FactoryConfig) (map[string]string, error) {
	if cfg == nil || cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil || len(cfg.Orchestrator.JavaScript.ArgsSchema) == 0 {
		return nil, nil
	}
	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(cfg.Orchestrator.JavaScript.ArgsSchema, &schema); err != nil {
		return nil, &interfaces.RequestValidationError{Message: fmt.Sprintf("invalid JavaScript args schema: %v", err)}
	}
	types := make(map[string]string, len(schema.Properties))
	for name, property := range schema.Properties {
		types[name] = property.Type
	}
	return types, nil
}

func coerceJavaScriptArgument(name, value, schemaType string) (any, error) {
	switch schemaType {
	case "integer":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, &interfaces.RequestValidationError{Message: fmt.Sprintf("argument %q must be an integer", name)}
		}
		return parsed, nil
	case "number":
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, &interfaces.RequestValidationError{Message: fmt.Sprintf("argument %q must be a number", name)}
		}
		return parsed, nil
	case "boolean":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, &interfaces.RequestValidationError{Message: fmt.Sprintf("argument %q must be a boolean", name)}
		}
		return parsed, nil
	default:
		return value, nil
	}
}

func javaScriptInvocationResult(requestID string, started factorysessionexecution.SyncStartResult) (apisurface.FactoryInvocationResult, error) {
	result := apisurface.FactoryInvocationResult{RequestID: requestID, SessionID: started.SessionID}
	if started.SyncOutcome != factorysessionexecution.SyncOutcomeCompleted {
		result.Status = factoryapi.InvocationTerminalStatusTimedOut
		result.ErrorCode = string(factoryapi.INVOCATIONTIMEDOUT)
		result.Message = "invocation timed out while waiting for JavaScript workflow result"
		return result, nil
	}
	var sessionResult factoryapi.FactorySessionResult
	if err := json.Unmarshal(started.Result, &sessionResult); err != nil {
		return apisurface.FactoryInvocationResult{}, fmt.Errorf("decode JavaScript workflow result: %w", err)
	}
	if sessionResult.PrimaryResult != nil {
		result.PrimaryResult = contentcontract.PartsFromGenerated(sessionResult.PrimaryResult)
	}
	if len(result.PrimaryResult) > 0 {
		result.Status = factoryapi.InvocationTerminalStatusCompleted
		return result, nil
	}
	result.Status = factoryapi.InvocationTerminalStatusFailed
	result.ErrorCode = "INVOCATION_RESULT_UNAVAILABLE"
	result.Message = "JavaScript workflow completed without a primary result"
	if sessionResult.FailureDetail != nil {
		result.ErrorCode = string(sessionResult.FailureDetail.Reason)
		result.Message = sessionResult.FailureDetail.Message
	}
	return result, nil
}
