package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/initializer"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// prepareCanonicalSessionIDForRun allocates the identity used by the
// automatic recording target and by explicit append-only JSONL recordings.
// Keeping allocation at the CLI boundary means the recording path can be
// reserved before runtime construction without deriving a second identity.
func prepareCanonicalSessionIDForRun(cfg RunConfig) (RunConfig, error) {
	if !usesCanonicalRecording(cfg) || strings.TrimSpace(cfg.CanonicalSessionID) != "" {
		return cfg, nil
	}
	generator := cfg.CanonicalSessionIDGenerator
	if generator == nil {
		return RunConfig{}, errors.New("prepare recording: canonical Factory Session ID generator is required")
	}
	canonicalID := strings.TrimSpace(generator())
	if canonicalID == "" {
		return RunConfig{}, fmt.Errorf("canonical Factory Session ID generator returned an empty identity")
	}
	cfg.CanonicalSessionID = canonicalID
	return cfg, nil
}

func usesCanonicalRecording(cfg RunConfig) bool {
	if strings.TrimSpace(cfg.ReplayPath) != "" || cfg.DisableDefaultRecording {
		return false
	}
	if strings.TrimSpace(cfg.RecordPath) == "" {
		return true
	}
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(cfg.RecordPath)), ".jsonl")
}

// InvocationOperation is the exact Factory invocation capability consumed by
// the run transport.
type InvocationOperation interface {
	InvokeModel(context.Context, factorysessions.InvocationTarget, string, models.Request) (models.Result, error)
	ResolveModelInvocationFactoryDir(string) (string, error)
	ExportModelInvocationArtifact(string, string) error
	InvokeFactory(context.Context, factorysessions.InvocationTarget, factorysessions.InvocationRequest) (factorysessions.FactoryInvocationOutcome, error)
}

// DirectJavaScriptRunOperation is the exact direct-workflow capability
// consumed by CLI selection.
type DirectJavaScriptRunOperation interface {
	Supports(string) bool
	Run(
		context.Context,
		factorysessions.DirectJavaScriptRunRequest,
		initializer.InvocationCancellation,
		func(),
	) error
}

// SelectionFactory binds one parsed CLI RunConfig to the run operations
// already selected by Wire.
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
	buildRunner RuntimeRunnerBuilder,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	directJavaScript DirectJavaScriptRunOperation,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	loadMockWorkers workers.MockWorkersConfigDiagnosticsLoader,
	buildRuntimeRequest SessionStartRequestFactory,
	presentations factorysessions.OpeningPresentationOwner,
	visualizations factoryvisualization.RuntimeSinkOwner,
) (SelectionFactory, error) {
	if buildRunner == nil || invocation == nil || presentation == nil ||
		directJavaScript == nil {
		return nil, fmt.Errorf("run transport operations are required")
	}
	return func(cfg RunConfig) processcontract.RunSelection {
		return &selection{
			cfg: cfg, buildRunner: buildRunner, invocation: invocation,
			presentation: presentation, directJavaScript: directJavaScript,
			prepareWorkTarget: prepareWorkTarget,
			loadMockWorkers:   loadMockWorkers, buildRuntimeRequest: buildRuntimeRequest,
			presentations: presentations, visualizations: visualizations,
		}
	}, nil
}

type selection struct {
	cfg                 RunConfig
	buildRunner         RuntimeRunnerBuilder
	invocation          InvocationOperation
	presentation        factoryvisualization.ResponsePresentation
	directJavaScript    DirectJavaScriptRunOperation
	prepareWorkTarget   work.SingleWorkTargetPreparation
	loadMockWorkers     workers.MockWorkersConfigDiagnosticsLoader
	buildRuntimeRequest SessionStartRequestFactory
	presentations       factorysessions.OpeningPresentationOwner
	visualizations      factoryvisualization.RuntimeSinkOwner
}

func (s *selection) Run(
	ctx context.Context,
	intent processcontract.RunIntent,
) error {
	if s == nil {
		return fmt.Errorf("run selection is required")
	}
	cfg, err := applyRunIntent(s.cfg, intent)
	if err != nil {
		return err
	}
	if s.directJavaScript.Supports(cfg.FactoryConfigPath) {
		return s.runDirectJavaScript(ctx, cfg, intent)
	}
	return RunSelected(ctx, cfg, s.buildRunner, s.invocation, s.presentation,
		s.prepareWorkTarget, s.loadMockWorkers, s.buildRuntimeRequest,
		s.presentations, s.visualizations)
}

func (s *selection) runDirectJavaScript(
	ctx context.Context,
	cfg RunConfig,
	intent processcontract.RunIntent,
) error {
	startupDisclosure, err := prepareStartupBeforeRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	request := factorysessions.DirectJavaScriptRunRequest{
		SourcePath: cfg.FactoryConfigPath, MockWorkersEnabled: cfg.MockWorkersEnabled,
		JSONOutput: cfg.JSONOutput,
	}
	var observer factorysessions.RuntimeHostObserver
	if intent.APIEnabled {
		request.Host = &factorysessions.RuntimeHostRequest{
			Directory: cfg.Dir, Host: cfg.BindHost, Port: cfg.Port, AutoPort: cfg.AutoPort,
			Pprof: cfg.Pprof,
		}
		observer = newRuntimeHostObserver(
			ctx, cfg, resolvedRunRecordPath{}, cfg.Port,
			func() runtimeartifact.Diagnostics { return runtimeartifact.Diagnostics{} },
			startupDisclosure,
		)
	}
	var scopeID factorysessions.OpeningScopeID
	if s.presentations != nil {
		scopeID, err = s.presentations.RegisterDirectJavaScript(factorysessions.DirectJavaScriptRunScope{
			Output: cfg.Output, RuntimeHostObserver: observer,
		})
		if err != nil {
			return fmt.Errorf("register direct JavaScript presentation: %w", err)
		}
		request.ScopeID = scopeID
		defer s.presentations.Close(scopeID)
	}
	var discloseStartup func()
	if !intent.APIEnabled || s.presentations == nil {
		discloseStartup = startupDisclosure.commit
	}
	return s.directJavaScript.Run(ctx, request, cfg.Cancellation, discloseStartup)
}

func applyRunIntent(cfg RunConfig, intent processcontract.RunIntent) (RunConfig, error) {
	cfg.Cancellation = intent.Cancellation
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
