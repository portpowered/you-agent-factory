package skippermissions

import (
	"strings"
	"testing"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestEffectiveSkipPermissions(t *testing.T) {
	t.Parallel()

	overrideTrue := true
	overrideFalse := false

	tests := []struct {
		name               string
		persisted          bool
		workerType         string
		invocationOverride *bool
		want               bool
	}{
		{
			name:       "AbsentOverrideUsesPersistedFalse",
			persisted:  false,
			workerType: workertaxonomy.WorkerTypeAgent,
			want:       false,
		},
		{
			name:               "OverrideTrueAgentWorker",
			persisted:          false,
			workerType:         workertaxonomy.WorkerTypeAgent,
			invocationOverride: &overrideTrue,
			want:               true,
		},
		{
			name:               "OverrideTrueModelWorkerIgnored",
			persisted:          false,
			workerType:         workertaxonomy.WorkerTypeModel,
			invocationOverride: &overrideTrue,
			want:               false,
		},
		{
			name:               "OverrideFalseAgentWorkerUsesPersistedFalse",
			persisted:          false,
			workerType:         workertaxonomy.WorkerTypeAgent,
			invocationOverride: &overrideFalse,
			want:               false,
		},
		{
			name:       "PersistedTrueWinsWithoutOverride",
			persisted:  true,
			workerType: workertaxonomy.WorkerTypeAgent,
			want:       true,
		},
		{
			name:               "PersistedTrueWinsWithOverrideFalse",
			persisted:          true,
			workerType:         workertaxonomy.WorkerTypeAgent,
			invocationOverride: &overrideFalse,
			want:               true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := EffectiveSkipPermissions(tc.persisted, tc.workerType, tc.invocationOverride)
			if got != tc.want {
				t.Fatalf("EffectiveSkipPermissions() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestS14SkipPermissionsPrecedenceEvidence(t *testing.T) {
	t.Parallel()

	overrideTrue := true
	overrideFalse := false

	tests := []struct {
		name               string
		persisted          bool
		workerType         string
		invocationOverride *bool
		want               bool
	}{
		{
			name:       "AbsentOverrideUsesPersistedFalse",
			persisted:  false,
			workerType: workertaxonomy.WorkerTypeAgent,
			want:       false,
		},
		{
			name:               "OverrideTrueWithPersistedFalse",
			persisted:          false,
			workerType:         workertaxonomy.WorkerTypeAgent,
			invocationOverride: &overrideTrue,
			want:               true,
		},
		{
			name:               "OverrideTrueIgnoredForModelWorker",
			persisted:          false,
			workerType:         workertaxonomy.WorkerTypeModel,
			invocationOverride: &overrideTrue,
			want:               false,
		},
		{
			name:               "OverrideFalseWithPersistedFalse",
			persisted:          false,
			workerType:         workertaxonomy.WorkerTypeAgent,
			invocationOverride: &overrideFalse,
			want:               false,
		},
		{
			name:       "PersistedTrueWithoutOverride",
			persisted:  true,
			workerType: workertaxonomy.WorkerTypeAgent,
			want:       true,
		},
		{
			name:               "PersistedTrueWithOverrideFalse",
			persisted:          true,
			workerType:         workertaxonomy.WorkerTypeAgent,
			invocationOverride: &overrideFalse,
			want:               true,
		},
		{
			name:               "PersistedTrueWithOverrideTrue",
			persisted:          true,
			workerType:         workertaxonomy.WorkerTypeAgent,
			invocationOverride: &overrideTrue,
			want:               true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := EffectiveSkipPermissions(tc.persisted, tc.workerType, tc.invocationOverride)
			if got != tc.want {
				t.Fatalf("EffectiveSkipPermissions() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAgentWorkerSupportsSkipPermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		worker *workerconfig.Config
		want   bool
	}{
		{
			name: "SupportedCloudAgentProvider",
			worker: &workerconfig.Config{
				Type:          workertaxonomy.WorkerTypeAgent,
				ModelProvider: string(modelprovider.Claude),
			},
			want: true,
		},
		{
			name: "LocalManagedAgentWorker",
			worker: &workerconfig.Config{
				Type:          workertaxonomy.WorkerTypeAgent,
				Model:         "OMNIVOICE_Q4_K_M",
				ModelProvider: string(modelprovider.Claude),
				ModelLocality: workerconfig.ModelLocalityLocal,
			},
			want: false,
		},
		{
			name: "UnsupportedModelProvider",
			worker: &workerconfig.Config{
				Type:          workertaxonomy.WorkerTypeAgent,
				ModelProvider: "acme",
			},
			want: false,
		},
		{
			name: "ModelWorkerIgnored",
			worker: &workerconfig.Config{
				Type:          workertaxonomy.WorkerTypeModel,
				ModelProvider: "acme",
			},
			want: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := AgentWorkerSupportsSkipPermissions(tc.worker)
			if got != tc.want {
				t.Fatalf("AgentWorkerSupportsSkipPermissions() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateInvocationSkipPermissionsForWorker(t *testing.T) {
	t.Parallel()

	overrideTrue := true
	agent := &workerconfig.Config{
		Type:          workertaxonomy.WorkerTypeAgent,
		ModelProvider: "acme",
	}
	supportedAgent := &workerconfig.Config{
		Type:          workertaxonomy.WorkerTypeAgent,
		ModelProvider: string(modelprovider.Claude),
	}
	localManagedAgent := &workerconfig.Config{
		Type:          workertaxonomy.WorkerTypeAgent,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelProvider: string(modelprovider.Claude),
		ModelLocality: workerconfig.ModelLocalityLocal,
	}
	modelWorker := &workerconfig.Config{
		Type:          workertaxonomy.WorkerTypeModel,
		ModelProvider: "acme",
	}

	if err := ValidateInvocationSkipPermissionsForWorker(agent, nil); err != nil {
		t.Fatalf("nil override: %v", err)
	}
	if err := ValidateInvocationSkipPermissionsForWorker(nil, &overrideTrue); err != nil {
		t.Fatalf("nil worker: %v", err)
	}
	if err := ValidateInvocationSkipPermissionsForWorker(modelWorker, &overrideTrue); err != nil {
		t.Fatalf("model worker: %v", err)
	}
	if err := ValidateInvocationSkipPermissionsForWorker(supportedAgent, &overrideTrue); err != nil {
		t.Fatalf("supported agent: %v", err)
	}
	if err := ValidateInvocationSkipPermissionsForWorker(agent, &overrideTrue); err == nil {
		t.Fatal("expected unsupported provider to fail when override is set")
	}
	if err := ValidateInvocationSkipPermissionsForWorker(localManagedAgent, &overrideTrue); err == nil {
		t.Fatal("expected local managed agent to fail when override is set")
	} else if !strings.Contains(err.Error(), "local managed model workers cannot honor CLI skip-permissions") {
		t.Fatalf("local managed error = %q, want locality detail", err.Error())
	}
}

type skipPermissionsRuntimeLookup struct {
	workers map[string]*workerconfig.Config
}

func (s skipPermissionsRuntimeLookup) Worker(name string) (*workerconfig.Config, bool) {
	worker, ok := s.workers[name]
	return worker, ok
}

func (skipPermissionsRuntimeLookup) Workstation(string) (*interfaces.FactoryWorkstationConfig, bool) {
	return nil, false
}

func (skipPermissionsRuntimeLookup) Guard(string) (*interfaces.GuardConfig, bool) {
	return nil, false
}

func (skipPermissionsRuntimeLookup) Resource(string) (*factoryresource.Config, bool) {
	return nil, false
}

func (skipPermissionsRuntimeLookup) FactoryConfig() *interfaces.FactoryConfig {
	return nil
}

func (skipPermissionsRuntimeLookup) FactoryDir() string {
	return ""
}

func (skipPermissionsRuntimeLookup) RuntimeBaseDir() string {
	return ""
}

func TestValidateInvocationSkipPermissionsWorkers(t *testing.T) {
	t.Parallel()

	overrideTrue := true
	factoryCfg := &interfaces.FactoryConfig{
		Workers: []workerconfig.Config{
			{Name: "supported-agent"},
			{Name: "unsupported-agent"},
		},
	}
	runtimeCfg := skipPermissionsRuntimeLookup{
		workers: map[string]*workerconfig.Config{
			"supported-agent": {
				Type:          workertaxonomy.WorkerTypeAgent,
				ModelProvider: string(modelprovider.Claude),
			},
			"unsupported-agent": {
				Type:          workertaxonomy.WorkerTypeAgent,
				ModelProvider: "acme",
			},
		},
	}

	if err := ValidateInvocationSkipPermissionsWorkers(factoryCfg, runtimeCfg, nil); err != nil {
		t.Fatalf("nil override: %v", err)
	}
	if err := ValidateInvocationSkipPermissionsWorkers(nil, runtimeCfg, &overrideTrue); err != nil {
		t.Fatalf("nil factory config: %v", err)
	}
	if err := ValidateInvocationSkipPermissionsWorkers(factoryCfg, nil, &overrideTrue); err != nil {
		t.Fatalf("nil runtime lookup: %v", err)
	}
	if err := ValidateInvocationSkipPermissionsWorkers(factoryCfg, runtimeCfg, &overrideTrue); err == nil {
		t.Fatal("expected unsupported worker to fail before dispatch")
	} else if !strings.Contains(err.Error(), `worker "unsupported-agent"`) {
		t.Fatalf("error = %q, want worker name", err.Error())
	}
}

func TestAgentWorkerSupportsSkipPermissionsNilAndEmptyProvider(t *testing.T) {
	t.Parallel()

	if !AgentWorkerSupportsSkipPermissions(nil) {
		t.Fatal("nil worker should be treated as supported")
	}
	if !AgentWorkerSupportsSkipPermissions(&workerconfig.Config{
		Type:          workertaxonomy.WorkerTypeAgent,
		ModelProvider: "  ",
	}) {
		t.Fatal("empty provider should be treated as supported")
	}
}
