package run

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

// InvocationOperation is the exact Factory invocation capability consumed by
// the run transport.
type InvocationOperation interface {
	InvokeModel(context.Context, factorysessions.InvocationTarget, string, models.Request) (models.Result, error)
	ResolveModelInvocationFactoryDir(string) (string, error)
	ExportModelInvocationArtifact(string, string) error
	InvokeFactory(context.Context, factorysessions.InvocationTarget, factorysessions.InvocationRequest, factorysessions.FactoryEventConsumer) (factorysessions.FactoryInvocationOutcome, error)
}

// DirectJavaScriptRunOperation is the exact direct-workflow capability
// consumed by CLI selection.
type DirectJavaScriptRunOperation interface {
	Supports(string) bool
	Open(context.Context, factorysessions.DirectJavaScriptRunRequest) (factorysessions.DirectJavaScriptApplication, error)
}

// SessionInvoker is the exact live invocation role consumed by mapping tests.
type SessionInvoker interface {
	InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorydefinitions.FactoryInvocationResult, error)
}

// SelectionFactory binds one parsed CLI RunConfig to the exact run operations
// already selected by Wire. It does not construct services or lifecycle state.
type SelectionFactory func(RunConfig) processcontract.RunSelection

// SplitFlagTerminator separates tokens parsed as run selectors and flags from
// positional input protected by the canonical "--" terminator.
func SplitFlagTerminator(args []string) (flagArgs []string, positional []string, terminated bool) {
	for index, token := range args {
		if token == "--" {
			return args[:index], args[index+1:], true
		}
	}
	return args, nil, false
}

func NewSelectionFactory(
	open Opener,
	buildRunner RuntimeRunnerBuilder,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	directJavaScript DirectJavaScriptRunOperation,
	buildApplication initializer.RuntimeRunnerBuilder,
) (SelectionFactory, error) {
	if open == nil || buildRunner == nil || invocation == nil || presentation == nil ||
		directJavaScript == nil || buildApplication == nil {
		return nil, fmt.Errorf("run transport operations are required")
	}
	return func(cfg RunConfig) processcontract.RunSelection {
		return &selection{
			cfg: cfg, open: open, buildRunner: buildRunner, invocation: invocation,
			presentation: presentation, directJavaScript: directJavaScript,
			buildApplication: buildApplication,
		}
	}, nil
}

type selection struct {
	cfg              RunConfig
	open             Opener
	buildRunner      RuntimeRunnerBuilder
	invocation       InvocationOperation
	presentation     factoryvisualization.ResponsePresentation
	directJavaScript DirectJavaScriptRunOperation
	buildApplication initializer.RuntimeRunnerBuilder
}

func (s *selection) Open(
	ctx context.Context,
	intent processcontract.RunIntent,
) (initializer.RunApplication, error) {
	if s == nil {
		return nil, fmt.Errorf("run selection is required")
	}
	cfg, err := applyRunIntent(s.cfg, intent)
	if err != nil {
		return nil, err
	}
	if s.directJavaScript.Supports(cfg.FactoryConfigPath) {
		request := factorysessions.DirectJavaScriptRunRequest{
			SourcePath: cfg.FactoryConfigPath, MockWorkersEnabled: cfg.MockWorkersEnabled,
			JSONOutput: cfg.JSONOutput, Output: cfg.Output, Logger: cfg.Logger,
		}
		if intent.APIEnabled {
			request.Host = &factorysessions.RuntimeHostRequest{
				Directory: cfg.Dir, Host: cfg.BindHost, Port: cfg.Port, AutoPort: cfg.AutoPort,
			}
			request.RuntimeHostObserver = newRuntimeHostObserver(
				ctx, cfg, resolvedRunRecordPath{}, cfg.Port,
				func() runtimeartifact.Diagnostics { return runtimeartifact.Diagnostics{} },
			)
		}
		return s.buildApplication(ctx, func(openCtx context.Context) (initializer.OpenedApplication, error) {
			opened, err := s.directJavaScript.Open(openCtx, request)
			return initializer.OpenedApplication{Plan: opened.Plan}, err
		})
	}
	return s.open(ctx, cfg, s.buildRunner, s.invocation, s.presentation)
}

func applyRunIntent(cfg RunConfig, intent processcontract.RunIntent) (RunConfig, error) {
	if intent.DashboardEnabled && !intent.APIEnabled {
		return RunConfig{}, fmt.Errorf("dashboard sidecar requires API transport")
	}
	switch {
	case intent.DefaultInvocation || intent.Continuous:
		if !intent.WorkerSidecarsEnabled || !intent.Continuous {
			return RunConfig{}, fmt.Errorf("continuous run requires worker scheduler and watchers")
		}
		cfg.Continuously = true
	default:
		if !intent.WorkerSidecarsEnabled || intent.Continuous {
			return RunConfig{}, fmt.Errorf("local-run policy requires worker scheduler with watchers disabled")
		}
		cfg.Continuously = false
	}
	if !intent.APIEnabled {
		cfg.Port = 0
	}
	return cfg, nil
}
