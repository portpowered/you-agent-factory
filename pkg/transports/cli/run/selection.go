package run

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

// SelectionFactory binds one parsed CLI RunConfig to the exact run operations
// already selected by Wire. It does not construct services or lifecycle state.
type SelectionFactory func(RunConfig) processcontract.RunSelection

func NewSelectionFactory(
	open Opener,
	buildRunner RuntimeRunnerBuilder,
	invocation factorysessions.InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	directJavaScript factorysessions.DirectJavaScriptRunOperation,
) (SelectionFactory, error) {
	if open == nil || buildRunner == nil || invocation == nil || presentation == nil || directJavaScript == nil {
		return nil, fmt.Errorf("run transport operations are required")
	}
	return func(cfg RunConfig) processcontract.RunSelection {
		return &selection{
			cfg: cfg, open: open, buildRunner: buildRunner, invocation: invocation,
			presentation: presentation, directJavaScript: directJavaScript,
		}
	}, nil
}

type selection struct {
	cfg              RunConfig
	open             Opener
	buildRunner      RuntimeRunnerBuilder
	invocation       factorysessions.InvocationOperation
	presentation     factoryvisualization.ResponsePresentation
	directJavaScript factorysessions.DirectJavaScriptRunOperation
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
		return directJavaScriptApplication{
			operation: s.directJavaScript,
			request: factorysessions.DirectJavaScriptRunRequest{
				SourcePath: cfg.FactoryConfigPath, MockWorkersEnabled: cfg.MockWorkersEnabled,
				JSONOutput: cfg.JSONOutput, Output: cfg.Output,
			},
		}, nil
	}
	return s.open(ctx, cfg, s.buildRunner, s.invocation, s.presentation)
}

type directJavaScriptApplication struct {
	operation factorysessions.DirectJavaScriptRunOperation
	request   factorysessions.DirectJavaScriptRunRequest
}

func (application directJavaScriptApplication) Run(ctx context.Context) error {
	if err := application.operation.Run(ctx, application.request); err != nil {
		return fmt.Errorf("initialize run service: %w", err)
	}
	return nil
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
	cfg.SuppressDashboardRendering = !intent.DashboardEnabled
	return cfg, nil
}
