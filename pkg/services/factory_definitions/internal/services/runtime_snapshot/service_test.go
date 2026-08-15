package runtimesnapshot_test

import (
	"context"
	"errors"
	"reflect"
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
