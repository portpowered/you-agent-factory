package skippermissions

import (
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
			workerType: interfaces.WorkerTypeAgent,
			want:       false,
		},
		{
			name:               "OverrideTrueAgentWorker",
			persisted:          false,
			workerType:         interfaces.WorkerTypeAgent,
			invocationOverride: &overrideTrue,
			want:               true,
		},
		{
			name:               "OverrideTrueModelWorkerIgnored",
			persisted:          false,
			workerType:         interfaces.WorkerTypeModel,
			invocationOverride: &overrideTrue,
			want:               false,
		},
		{
			name:               "OverrideFalseAgentWorkerUsesPersistedFalse",
			persisted:          false,
			workerType:         interfaces.WorkerTypeAgent,
			invocationOverride: &overrideFalse,
			want:               false,
		},
		{
			name:       "PersistedTrueWinsWithoutOverride",
			persisted:  true,
			workerType: interfaces.WorkerTypeAgent,
			want:       true,
		},
		{
			name:               "PersistedTrueWinsWithOverrideFalse",
			persisted:          true,
			workerType:         interfaces.WorkerTypeAgent,
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
			workerType: interfaces.WorkerTypeAgent,
			want:       false,
		},
		{
			name:               "OverrideTrueWithPersistedFalse",
			persisted:          false,
			workerType:         interfaces.WorkerTypeAgent,
			invocationOverride: &overrideTrue,
			want:               true,
		},
		{
			name:               "OverrideTrueIgnoredForModelWorker",
			persisted:          false,
			workerType:         interfaces.WorkerTypeModel,
			invocationOverride: &overrideTrue,
			want:               false,
		},
		{
			name:               "OverrideFalseWithPersistedFalse",
			persisted:          false,
			workerType:         interfaces.WorkerTypeAgent,
			invocationOverride: &overrideFalse,
			want:               false,
		},
		{
			name:       "PersistedTrueWithoutOverride",
			persisted:  true,
			workerType: interfaces.WorkerTypeAgent,
			want:       true,
		},
		{
			name:               "PersistedTrueWithOverrideFalse",
			persisted:          true,
			workerType:         interfaces.WorkerTypeAgent,
			invocationOverride: &overrideFalse,
			want:               true,
		},
		{
			name:               "PersistedTrueWithOverrideTrue",
			persisted:          true,
			workerType:         interfaces.WorkerTypeAgent,
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
		worker *interfaces.FactoryWorkerConfig
		want   bool
	}{
		{
			name: "SupportedCloudAgentProvider",
			worker: &interfaces.FactoryWorkerConfig{
				Type:          interfaces.WorkerTypeAgent,
				ModelProvider: string(modelprovider.ProviderClaude),
			},
			want: true,
		},
		{
			name: "LocalManagedAgentWorker",
			worker: &interfaces.FactoryWorkerConfig{
				Type:          interfaces.WorkerTypeAgent,
				Model:         "OMNIVOICE_Q4_K_M",
				ModelProvider: string(modelprovider.ProviderClaude),
				ModelLocality: interfaces.ModelLocalityLocal,
			},
			want: false,
		},
		{
			name: "UnsupportedModelProvider",
			worker: &interfaces.FactoryWorkerConfig{
				Type:          interfaces.WorkerTypeAgent,
				ModelProvider: "acme",
			},
			want: false,
		},
		{
			name: "ModelWorkerIgnored",
			worker: &interfaces.FactoryWorkerConfig{
				Type:          interfaces.WorkerTypeModel,
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
	agent := &interfaces.FactoryWorkerConfig{
		Type:          interfaces.WorkerTypeAgent,
		ModelProvider: "acme",
	}
	supportedAgent := &interfaces.FactoryWorkerConfig{
		Type:          interfaces.WorkerTypeAgent,
		ModelProvider: string(modelprovider.ProviderClaude),
	}
	localManagedAgent := &interfaces.FactoryWorkerConfig{
		Type:          interfaces.WorkerTypeAgent,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelProvider: string(modelprovider.ProviderClaude),
		ModelLocality: interfaces.ModelLocalityLocal,
	}
	modelWorker := &interfaces.FactoryWorkerConfig{
		Type:          interfaces.WorkerTypeModel,
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
	workers map[string]*interfaces.FactoryWorkerConfig
}

func (s skipPermissionsRuntimeLookup) Worker(name string) (*interfaces.FactoryWorkerConfig, bool) {
	worker, ok := s.workers[name]
	return worker, ok
}

func (skipPermissionsRuntimeLookup) Workstation(string) (*interfaces.FactoryWorkstationConfig, bool) {
	return nil, false
}

func (skipPermissionsRuntimeLookup) Guard(string) (*interfaces.GuardConfig, bool) {
	return nil, false
}

func (skipPermissionsRuntimeLookup) Resource(string) (*interfaces.ResourceConfig, bool) {
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
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "supported-agent"},
			{Name: "unsupported-agent"},
		},
	}
	runtimeCfg := skipPermissionsRuntimeLookup{
		workers: map[string]*interfaces.FactoryWorkerConfig{
			"supported-agent": {
				Type:          interfaces.WorkerTypeAgent,
				ModelProvider: string(modelprovider.ProviderClaude),
			},
			"unsupported-agent": {
				Type:          interfaces.WorkerTypeAgent,
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
	if !AgentWorkerSupportsSkipPermissions(&interfaces.FactoryWorkerConfig{
		Type:          interfaces.WorkerTypeAgent,
		ModelProvider: "  ",
	}) {
		t.Fatal("empty provider should be treated as supported")
	}
}
