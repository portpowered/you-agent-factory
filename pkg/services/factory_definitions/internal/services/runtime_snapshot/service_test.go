package runtimesnapshot_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	runtimesnapshotwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/runtime_snapshot/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestResolveRuntimeSnapshotReturnsDetachedEffectiveValues(t *testing.T) {
	t.Parallel()

	source := newTestLoadedSource()
	var receivedCanonical []byte
	resolver, err := runtimesnapshotwire.NewService(
		func(payload []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			receivedCanonical = payload
			return source, nil
		},
		func(_ string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return source, nil
		},
		func() factorydefinitions.WorkstationLoader { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := factorydefinitions.ResolveRuntimeSnapshotRequest{
		Canonical:        []byte(`{"name":"detached"}`),
		ExecutionBaseDir: "/execution/base",
		Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
			FactorySessionID: "session-1",
			WorkflowID:       "workflow-1",
		},
	}
	first, err := resolver.ResolveRuntimeSnapshot(context.Background(), request)
	if err != nil {
		t.Fatalf("ResolveRuntimeSnapshot(first) error = %v", err)
	}
	request.Canonical[0] = 'X'
	if string(receivedCanonical) != `{"name":"detached"}` {
		t.Fatalf("canonical loader received aliased request bytes %q", receivedCanonical)
	}

	assertInitialRuntimeSnapshot(t, first.Snapshot)
	mutateRuntimeSnapshot(first.Snapshot)

	second, err := resolver.ResolveRuntimeSnapshot(
		context.Background(),
		factorydefinitions.ResolveRuntimeSnapshotRequest{
			Canonical:        []byte(`{"name":"detached"}`),
			ExecutionBaseDir: "/execution/base",
			Invocation:       first.Snapshot.Invocation,
		},
	)
	if err != nil {
		t.Fatalf("ResolveRuntimeSnapshot(second) error = %v", err)
	}
	assertDetachedRuntimeSnapshot(t, second.Snapshot)
}

func TestResolveRuntimeSnapshotInterpolatesInvocationValuesBeforeDetaching(t *testing.T) {
	t.Parallel()

	source := newTestLoadedSource()
	source.config.Workers[0].ModelProvider = "${provider}"
	source.config.Workers[0].Body = "document=${document}"
	var readPath string
	resolver, err := runtimesnapshotwire.NewService(
		func(_ []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return source, nil
		},
		func(_ string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return source, nil
		},
		func() factorydefinitions.WorkstationLoader { return nil },
		factorydefinitions.FileReader(func(path string) ([]byte, error) {
			readPath = path
			return []byte("resolved document"), nil
		}),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	arguments := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"provider": {Values: []string{"codex"}},
		"document": {
			Values:    []string{"story.md"},
			ValueMode: work.InvocationParameterValueModeFileContents,
		},
	}}
	result, err := resolver.ResolveRuntimeSnapshot(context.Background(), factorydefinitions.ResolveRuntimeSnapshotRequest{
		Canonical: []byte(`{"name":"detached"}`),
		Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
			Arguments: arguments,
		},
	})
	if err != nil {
		t.Fatalf("ResolveRuntimeSnapshot() error = %v", err)
	}
	if readPath != "story.md" {
		t.Fatalf("FILE_CONTENTS reader path = %q, want story.md", readPath)
	}
	worker := result.Snapshot.EffectiveFactory.Workers[0]
	if worker.ModelProvider != "codex" || worker.Body != "document=resolved document" {
		t.Fatalf("resolved worker = %#v, want concrete provider and interpolated body", worker)
	}
	if source.config.Workers[0].ModelProvider != "${provider}" || source.config.Workers[0].Body != "document=${document}" {
		t.Fatalf("source worker was mutated: %#v", source.config.Workers[0])
	}
}

func TestResolveRuntimeSnapshotAllowsLogicalWorkstationsDuringOneShotResolution(t *testing.T) {
	t.Parallel()

	source := newTestLoadedSource()
	source.config.Workers = nil
	source.config.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name: "logical-failure",
		Type: factorydefinitions.WorkstationTypeLogical,
	}}
	resolver, err := runtimesnapshotwire.NewService(
		func(_ []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return source, nil
		},
		func(_ string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return source, nil
		},
		func() factorydefinitions.WorkstationLoader { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := resolver.ResolveRuntimeSnapshot(context.Background(), factorydefinitions.ResolveRuntimeSnapshotRequest{
		Canonical: []byte(`{"name":"logical"}`),
		Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
			Arguments: &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{}},
		},
	})
	if err != nil {
		t.Fatalf("ResolveRuntimeSnapshot() error = %v", err)
	}
	if got := result.Snapshot.EffectiveFactory.Workstations[0].Type; got != factorydefinitions.WorkstationTypeLogical {
		t.Fatalf("logical workstation type = %q, want %q", got, factorydefinitions.WorkstationTypeLogical)
	}
}

func assertInitialRuntimeSnapshot(t *testing.T, snapshot factorydefinitions.RuntimeSnapshot) {
	t.Helper()
	if snapshot.FactoryDir != "/factories/alpha" {
		t.Fatalf("FactoryDir = %q, want /factories/alpha", snapshot.FactoryDir)
	}
	if snapshot.RuntimeBaseDir != "/execution/base" {
		t.Fatalf("RuntimeBaseDir = %q, want /execution/base", snapshot.RuntimeBaseDir)
	}
	if snapshot.DefinitionVersion == nil || snapshot.DefinitionVersion.Logical != 7 {
		t.Fatalf("DefinitionVersion = %#v, want logical version 7", snapshot.DefinitionVersion)
	}
	if snapshot.EffectiveFactory.Workers[0].Concurrency != 3 || snapshot.Workers[0].RuntimeDefaultModel != "runtime-model" {
		t.Fatalf("runtime worker facts = %#v / %#v, want runtime metadata preserved", snapshot.EffectiveFactory.Workers[0], snapshot.Workers[0])
	}
	if len(snapshot.AutomationSources) != 1 || snapshot.AutomationSources[0].Kind != factorydefinitions.RuntimeAutomationSourceKindCron {
		t.Fatalf("AutomationSources = %#v, want one cron source", snapshot.AutomationSources)
	}
	if len(snapshot.PromptSources) != 2 {
		t.Fatalf("PromptSources = %#v, want worker and workstation sources", snapshot.PromptSources)
	}
}

func mutateRuntimeSnapshot(snapshot factorydefinitions.RuntimeSnapshot) {
	snapshot.EffectiveFactory.Workers[0].Auth.SecretRef = "mutated"
	snapshot.Workers[0].Args[0] = "mutated"
	snapshot.Workstations[0].Env["TOKEN"] = "mutated"
	snapshot.AutomationSources[0].Workstation.Env["TOKEN"] = "mutated"
	snapshot.PromptSources[0].Path = "mutated"
}

func assertDetachedRuntimeSnapshot(t *testing.T, snapshot factorydefinitions.RuntimeSnapshot) {
	t.Helper()
	if snapshot.EffectiveFactory.Workers[0].Auth.SecretRef != "secret-ref" {
		t.Fatalf("second effective worker Auth = %#v, was affected by first result", snapshot.EffectiveFactory.Workers[0].Auth)
	}
	if snapshot.Workers[0].Args[0] != "--safe" {
		t.Fatalf("second worker Args = %#v, was affected by first result", snapshot.Workers[0].Args)
	}
	if snapshot.Workstations[0].Env["TOKEN"] != "value" {
		t.Fatalf("second workstation Env = %#v, was affected by first result", snapshot.Workstations[0].Env)
	}
	if snapshot.AutomationSources[0].Workstation.Env["TOKEN"] != "value" {
		t.Fatalf("second automation workstation Env = %#v, was affected by second result", snapshot.AutomationSources[0].Workstation.Env)
	}
	if snapshot.PromptSources[0].Path != "workers/agent.md" {
		t.Fatalf("second prompt source = %#v, was affected by first result", snapshot.PromptSources[0])
	}
}

func TestResolveRuntimeSnapshotEquivalentRequestsProduceEquivalentValues(t *testing.T) {
	t.Parallel()

	source := newTestLoadedSource()
	resolver, err := runtimesnapshotwire.NewService(
		func(_ []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return source, nil
		},
		func(_ string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return source, nil
		},
		func() factorydefinitions.WorkstationLoader { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first, err := resolver.ResolveRuntimeSnapshot(context.Background(), factorydefinitions.ResolveRuntimeSnapshotRequest{
		FactoryDir:       "/factories/alpha",
		ExecutionBaseDir: "/execution/base",
	})
	if err != nil {
		t.Fatalf("ResolveRuntimeSnapshot(FactoryDir) error = %v", err)
	}
	second, err := resolver.ResolveRuntimeSnapshot(context.Background(), factorydefinitions.ResolveRuntimeSnapshotRequest{
		SourcePath:       "/factories/alpha",
		ExecutionBaseDir: "/execution/base",
	})
	if err != nil {
		t.Fatalf("ResolveRuntimeSnapshot(SourcePath) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("equivalent requests differ:\nfirst: %#v\nsecond: %#v", first, second)
	}
}

func TestResolveRuntimeSnapshotRejectsInvalidRequestBeforeLoading(t *testing.T) {
	t.Parallel()

	called := false
	resolver, err := runtimesnapshotwire.NewService(
		func(_ []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			called = true
			return nil, nil
		},
		func(_ string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			called = true
			return nil, nil
		},
		func() factorydefinitions.WorkstationLoader { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = resolver.ResolveRuntimeSnapshot(context.Background(), factorydefinitions.ResolveRuntimeSnapshotRequest{
		FactoryDir: "/factories/alpha",
		Canonical:  []byte(`{"name":"conflict"}`),
	})
	if !errors.Is(err, factorydefinitions.ErrInvalidRuntimeSnapshotRequest) {
		t.Fatalf("error = %v, want invalid request", err)
	}
	if called {
		t.Fatal("loader was called for invalid request")
	}
}

func TestResolveRuntimeSnapshotPreservesTypedLoaderFailure(t *testing.T) {
	t.Parallel()

	cause := factorydefinitions.ErrInvalidNamedFactory
	resolver, err := runtimesnapshotwire.NewService(
		func(_ []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, cause
		},
		func(_ string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, cause
		},
		func() factorydefinitions.WorkstationLoader { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = resolver.ResolveRuntimeSnapshot(context.Background(), factorydefinitions.ResolveRuntimeSnapshotRequest{
		Canonical: []byte(`{"name":"invalid"}`),
	})
	if !errors.Is(err, factorydefinitions.ErrInvalidRuntimeSnapshotDefinition) {
		t.Fatalf("error = %v, want invalid definition", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want underlying typed loader failure %v", err, cause)
	}
}

func TestResolveRuntimeSnapshotCarriesInvocationProvenanceAndDetachesInputs(t *testing.T) {
	t.Parallel()

	source := newTestLoadedSource()
	source.config.Workers[0].ModelProvider = "codex"
	source.config.Workers[0].Args = []string{"--token=${apiKey}", "--label=${label}"}
	source.config.Workstations[0].Env["secret/name~value"] = "${apiKey}"
	arguments := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"apiKey": {Values: []string{"super-secret"}, Sensitive: true},
		"label":  {Values: []string{"public"}},
	}}
	workstationLoader := &testWorkstationLoader{}
	var receivedPath string
	var receivedWorkstationLoader factorydefinitions.WorkstationLoader
	resolver, err := runtimesnapshotwire.NewService(
		func(_ []byte, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			receivedWorkstationLoader = loader
			return source, nil
		},
		func(path string, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			receivedPath = path
			receivedWorkstationLoader = loader
			return source, nil
		},
		func() factorydefinitions.WorkstationLoader { return workstationLoader },
		factorydefinitions.FileReader(func(string) ([]byte, error) { return nil, nil }),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := resolver.ResolveRuntimeSnapshot(context.Background(), factorydefinitions.ResolveRuntimeSnapshotRequest{
		SourcePath:       "  /factories/alpha  ",
		ExecutionBaseDir: "  /execution/base  ",
		Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
			FactorySessionID: "session-1",
			WorkflowID:       "workflow-1",
			Arguments:        arguments,
		},
	})
	if err != nil {
		t.Fatalf("ResolveRuntimeSnapshot() error = %v", err)
	}
	if receivedPath != "/factories/alpha" {
		t.Fatalf("loaded Factory path = %q, want trimmed source path", receivedPath)
	}
	if receivedWorkstationLoader != workstationLoader {
		t.Fatalf("workstation loader = %T, want injected loader %T", receivedWorkstationLoader, workstationLoader)
	}

	snapshot := result.Snapshot
	if snapshot.FactoryDir != "/factories/alpha" || snapshot.RuntimeBaseDir != "/execution/base" {
		t.Fatalf("snapshot paths = %q/%q, want Factory and execution roots", snapshot.FactoryDir, snapshot.RuntimeBaseDir)
	}
	if snapshot.Invocation.FactorySessionID != "session-1" || snapshot.Invocation.WorkflowID != "workflow-1" {
		t.Fatalf("invocation context = %#v, want session and workflow identity", snapshot.Invocation)
	}
	if got := snapshot.EffectiveFactory.Workers[0].Args; !reflect.DeepEqual(got, []string{"--token=super-secret", "--label=public"}) {
		t.Fatalf("effective worker args = %#v, want interpolated arguments", got)
	}
	if got := snapshot.EffectiveFactory.Workstations[0].Env["secret/name~value"]; got != "super-secret" {
		t.Fatalf("effective sensitive environment value = %q, want interpolated value", got)
	}
	wantPointers := []string{
		"/workers/0/args/0",
		"/workstations/0/env/secret~1name~0value",
	}
	if !reflect.DeepEqual(snapshot.InvocationSensitiveJSONPointers, wantPointers) {
		t.Fatalf("sensitive JSON pointers = %#v, want %#v", snapshot.InvocationSensitiveJSONPointers, wantPointers)
	}
	if strings.Contains(strings.Join(snapshot.InvocationSensitiveJSONPointers, "\n"), "super-secret") {
		t.Fatal("sensitive invocation value was exposed in JSON pointer provenance")
	}
	if len(snapshot.BundledFiles) != 1 || snapshot.BundledFiles[0].TargetPath != "docs/README.md" {
		t.Fatalf("bundled files = %#v, want loaded portable replacement", snapshot.BundledFiles)
	}

	arguments.Arguments["apiKey"] = work.InvocationArgument{Values: []string{"changed"}, Sensitive: true}
	arguments.Arguments["label"].Values[0] = "changed"
	source.config.Workers[0].Args[0] = "source-mutated"
	source.config.Workstations[0].Env["secret/name~value"] = "source-mutated"
	source.replacements[0].TargetPath = "source-mutated"
	if snapshot.Invocation.Arguments.Arguments["apiKey"].Values[0] != "super-secret" ||
		snapshot.Invocation.Arguments.Arguments["label"].Values[0] != "public" {
		t.Fatalf("snapshot invocation arguments were not detached: %#v", snapshot.Invocation.Arguments)
	}
	if snapshot.EffectiveFactory.Workers[0].Args[0] != "--token=super-secret" ||
		snapshot.EffectiveFactory.Workstations[0].Env["secret/name~value"] != "super-secret" {
		t.Fatalf("snapshot effective values were affected by source mutation: %#v", snapshot.EffectiveFactory)
	}
	if snapshot.BundledFiles[0].TargetPath != "docs/README.md" {
		t.Fatalf("snapshot bundled file was affected by source mutation: %#v", snapshot.BundledFiles)
	}
}

func TestResolveRuntimeSnapshotClassifiesEveryAutomationSourceKind(t *testing.T) {
	t.Parallel()

	source := newTestLoadedSource()
	source.config.Workstations = append(source.config.Workstations,
		factorydefinitions.FactoryWorkstationConfig{
			ID:   "script-source",
			Name: "script-source",
			Type: factorydefinitions.WorkstationTypeScript,
		},
		factorydefinitions.FactoryWorkstationConfig{
			ID:   "poller-source",
			Name: "poller-source",
			Type: factorydefinitions.WorkstationTypePoller,
		},
		factorydefinitions.FactoryWorkstationConfig{
			ID:             "hosted-source",
			Name:           "hosted-source",
			WorkerTypeName: "agent",
		},
		factorydefinitions.FactoryWorkstationConfig{
			ID:   "ordinary-source",
			Name: "ordinary-source",
			Type: factorydefinitions.WorkstationTypeHumanApproval,
		},
	)
	resolver, err := runtimesnapshotwire.NewService(
		func(_ []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return source, nil
		},
		func(_ string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return source, nil
		},
		func() factorydefinitions.WorkstationLoader { return nil },
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := resolver.ResolveRuntimeSnapshot(context.Background(), factorydefinitions.ResolveRuntimeSnapshotRequest{
		Canonical: []byte(`{"name":"automation"}`),
	})
	if err != nil {
		t.Fatalf("ResolveRuntimeSnapshot() error = %v", err)
	}
	got := make(map[string]factorydefinitions.RuntimeAutomationSource)
	for _, automation := range result.Snapshot.AutomationSources {
		got[automation.WorkstationName] = automation
	}
	wantKinds := map[string]factorydefinitions.RuntimeAutomationSourceKind{
		"cron-agent":    factorydefinitions.RuntimeAutomationSourceKindCron,
		"script-source": factorydefinitions.RuntimeAutomationSourceKindScript,
		"poller-source": factorydefinitions.RuntimeAutomationSourceKindPoller,
		"hosted-source": factorydefinitions.RuntimeAutomationSourceKindHosted,
	}
	if len(got) != len(wantKinds) {
		t.Fatalf("automation sources = %#v, want exactly %d classified sources", got, len(wantKinds))
	}
	for name, wantKind := range wantKinds {
		automation, ok := got[name]
		if !ok || automation.Kind != wantKind {
			t.Fatalf("automation source %q = %#v, want kind %q", name, automation, wantKind)
		}
		if name == "hosted-source" && (automation.Worker == nil || automation.Worker.Name != "agent") {
			t.Fatalf("hosted automation worker = %#v, want detached agent worker", automation.Worker)
		}
	}
	if _, ok := got["ordinary-source"]; ok {
		t.Fatalf("ordinary workstation unexpectedly became an automation source: %#v", got["ordinary-source"])
	}
}

func TestResolveRuntimeSnapshotClassifiesContextAndDefinitionFailures(t *testing.T) {
	t.Parallel()

	loaderCause := errors.New("loader failed")
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name           string
		context        context.Context
		loaded         factorydefinitions.MutableLoadedFactorySource
		loaderError    error
		cancelOnLoad   bool
		wantCode       factorydefinitions.RuntimeSnapshotDiagnosticCode
		wantUnderlying error
		wantCause      bool
	}{
		{
			name:     "nil context",
			context:  nil,
			wantCode: factorydefinitions.RuntimeSnapshotDiagnosticInvalidRequest,
		},
		{
			name:           "canceled before loading",
			context:        canceledContext,
			wantCode:       factorydefinitions.RuntimeSnapshotDiagnosticCanceled,
			wantUnderlying: context.Canceled,
		},
		{
			name:           "canceled after loading",
			context:        context.Background(),
			loaded:         newTestLoadedSource(),
			cancelOnLoad:   true,
			wantCode:       factorydefinitions.RuntimeSnapshotDiagnosticCanceled,
			wantUnderlying: context.Canceled,
		},
		{
			name:           "loader failure",
			context:        context.Background(),
			loaderError:    loaderCause,
			wantCode:       factorydefinitions.RuntimeSnapshotDiagnosticInvalidDefinition,
			wantUnderlying: loaderCause,
		},
		{
			name:     "nil loaded source",
			context:  context.Background(),
			wantCode: factorydefinitions.RuntimeSnapshotDiagnosticInvalidDefinition,
		},
		{
			name:    "und detachable definition",
			context: context.Background(),
			loaded: func() factorydefinitions.MutableLoadedFactorySource {
				source := newTestLoadedSource()
				source.config = nil
				return source
			}(),
			wantCode:  factorydefinitions.RuntimeSnapshotDiagnosticInvalidDefinition,
			wantCause: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx := test.context
			if test.cancelOnLoad {
				var cancelLoad context.CancelFunc
				ctx, cancelLoad = context.WithCancel(context.Background())
				defer cancelLoad()
				loader := test.loaded
				resolver, err := runtimesnapshotwire.NewService(
					func(_ []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
						cancelLoad()
						return loader, nil
					},
					func(_ string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
						cancelLoad()
						return loader, nil
					},
					func() factorydefinitions.WorkstationLoader { return nil },
					nil,
				)
				if err != nil {
					t.Fatalf("New() error = %v", err)
				}
				assertRuntimeSnapshotFailure(t, resolver, ctx, test.wantCode, test.wantUnderlying, test.wantCause)
				return
			}

			resolver, err := runtimesnapshotwire.NewService(
				func(_ []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
					return test.loaded, test.loaderError
				},
				func(_ string, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
					return test.loaded, test.loaderError
				},
				func() factorydefinitions.WorkstationLoader { return nil },
				nil,
			)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			assertRuntimeSnapshotFailure(t, resolver, ctx, test.wantCode, test.wantUnderlying, test.wantCause)
		})
	}
}

func TestNewRuntimeSnapshotServiceRejectsMissingSourceLoaders(t *testing.T) {
	t.Parallel()

	validCanonical := factorydefinitions.CanonicalFactoryJSONLoader(func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return newTestLoadedSource(), nil
	})
	validFactory := factorydefinitions.LoadedFactoryLoader(func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return newTestLoadedSource(), nil
	})
	if _, err := runtimesnapshotwire.NewService(nil, validFactory, nil, nil); err == nil {
		t.Fatal("NewService(nil canonical loader) succeeded, want construction failure")
	}
	if _, err := runtimesnapshotwire.NewService(validCanonical, nil, nil, nil); err == nil {
		t.Fatal("NewService(nil Factory loader) succeeded, want construction failure")
	}
}

func assertRuntimeSnapshotFailure(
	t *testing.T,
	resolver interface {
		ResolveRuntimeSnapshot(context.Context, factorydefinitions.ResolveRuntimeSnapshotRequest) (factorydefinitions.ResolveRuntimeSnapshotResult, error)
	},
	ctx context.Context,
	wantCode factorydefinitions.RuntimeSnapshotDiagnosticCode,
	wantUnderlying error,
	wantCause bool,
) {
	t.Helper()
	_, err := resolver.ResolveRuntimeSnapshot(ctx, factorydefinitions.ResolveRuntimeSnapshotRequest{
		Canonical: []byte(`{"name":"failure"}`),
	})
	if err == nil {
		t.Fatal("ResolveRuntimeSnapshot() succeeded, want typed failure")
	}
	var diagnostic *factorydefinitions.RuntimeSnapshotResolutionError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error = %v, want RuntimeSnapshotResolutionError", err)
	}
	if diagnostic.Diagnostic.Code != wantCode {
		t.Fatalf("diagnostic code = %q, want %q", diagnostic.Diagnostic.Code, wantCode)
	}
	if wantUnderlying != nil && !errors.Is(err, wantUnderlying) {
		t.Fatalf("error = %v, want underlying %v", err, wantUnderlying)
	}
	if wantCause && diagnostic.Cause == nil {
		t.Fatal("invalid definition diagnostic has no inspectable cause")
	}
}

type testWorkstationLoader struct{}

func (*testWorkstationLoader) Load(string) (*factorydefinitions.FactoryWorkstationConfig, error) {
	return nil, nil
}

type fakeLoadedSource struct {
	config       *factorydefinitions.FactoryConfig
	factoryDir   string
	runtimeBase  string
	replacements []factorydefinitions.PortableBundledFileReplacement
	workers      map[string]*factorydefinitions.FactoryWorkerConfig
	workstations map[string]*factorydefinitions.FactoryWorkstationConfig
	prompts      map[string]factorydefinitions.PromptSource
}

var _ factorydefinitions.MutableLoadedFactorySource = (*fakeLoadedSource)(nil)

func newTestLoadedSource() *fakeLoadedSource {
	config := &factorydefinitions.FactoryConfig{
		Name: "alpha",
		Version: &factorydefinitions.FactoryVersion{
			Logical:  7,
			Physical: time.Unix(10, 0).UTC(),
		},
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name:                        "agent",
			Type:                        factorydefinitions.WorkerTypeHosted,
			Args:                        []string{"--safe"},
			Auth:                        &factorydefinitions.HostedWorkerAuthConfig{SecretRef: "secret-ref"},
			Body:                        "worker body",
			Concurrency:                 3,
			RuntimeDefaultModel:         "runtime-model",
			RuntimeDefaultModelProvider: "runtime-provider",
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			ID:             "cron-agent",
			Name:           "cron-agent",
			Type:           "POLLER_RUN",
			WorkerTypeName: "agent",
			Cron:           &factorydefinitions.CronConfig{Schedule: "*/5 * * * *", TriggerAtStart: true},
			Env:            map[string]string{"TOKEN": "value"},
		}},
	}
	return &fakeLoadedSource{
		config:     config,
		factoryDir: "/factories/alpha",
		workers: map[string]*factorydefinitions.FactoryWorkerConfig{
			"agent": &config.Workers[0],
		},
		workstations: map[string]*factorydefinitions.FactoryWorkstationConfig{
			"cron-agent": &config.Workstations[0],
		},
		prompts: map[string]factorydefinitions.PromptSource{
			"worker:agent":           {Path: "workers/agent.md"},
			"workstation:cron-agent": {Path: "workstations/cron.md", IsTemplate: true},
		},
		replacements: []factorydefinitions.PortableBundledFileReplacement{{TargetPath: "docs/README.md"}},
	}
}

func (s *fakeLoadedSource) FactoryDir() string { return s.factoryDir }
func (s *fakeLoadedSource) RuntimeBaseDir() string {
	if s.runtimeBase != "" {
		return s.runtimeBase
	}
	return s.factoryDir
}
func (s *fakeLoadedSource) SetRuntimeBaseDir(value string)                   { s.runtimeBase = value }
func (s *fakeLoadedSource) FactoryConfig() *factorydefinitions.FactoryConfig { return s.config }
func (s *fakeLoadedSource) Worker(name string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	worker, ok := s.workers[name]
	return worker, ok
}
func (s *fakeLoadedSource) Workstation(name string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	workstation, ok := s.workstations[name]
	return workstation, ok
}
func (s *fakeLoadedSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return append([]factorydefinitions.PortableBundledFileReplacement(nil), s.replacements...)
}
func (s *fakeLoadedSource) MutateWorkers(mutate func(*factorydefinitions.FactoryWorkerConfig) error) error {
	for _, worker := range s.workers {
		if err := mutate(worker); err != nil {
			return err
		}
	}
	return nil
}
func (s *fakeLoadedSource) WorkerPromptSource(name string) (factorydefinitions.PromptSource, bool) {
	source, ok := s.prompts["worker:"+name]
	return source, ok
}
func (s *fakeLoadedSource) WorkstationPromptSource(name string) (factorydefinitions.PromptSource, bool) {
	source, ok := s.prompts["workstation:"+name]
	return source, ok
}
