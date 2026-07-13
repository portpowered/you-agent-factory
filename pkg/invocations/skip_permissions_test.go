package invocations

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
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
			name:               "PersistedTrueWinsWithoutOverride",
			persisted:          true,
			workerType:         interfaces.WorkerTypeAgent,
			want:               true,
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

func TestAgentWorkerSupportsSkipPermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		worker *interfaces.WorkerConfig
		want   bool
	}{
		{
			name: "SupportedCloudAgentProvider",
			worker: &interfaces.WorkerConfig{
				Type:          interfaces.WorkerTypeAgent,
				ModelProvider: string(interfaces.ModelProviderClaude),
			},
			want: true,
		},
		{
			name: "LocalManagedAgentWorker",
			worker: &interfaces.WorkerConfig{
				Type:          interfaces.WorkerTypeAgent,
				Model:         "OMNIVOICE_Q4_K_M",
				ModelProvider: string(interfaces.ModelProviderClaude),
				ModelLocality: interfaces.ModelLocalityLocal,
			},
			want: false,
		},
		{
			name: "UnsupportedModelProvider",
			worker: &interfaces.WorkerConfig{
				Type:          interfaces.WorkerTypeAgent,
				ModelProvider: "acme",
			},
			want: false,
		},
		{
			name: "ModelWorkerIgnored",
			worker: &interfaces.WorkerConfig{
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
	agent := &interfaces.WorkerConfig{
		Type:          interfaces.WorkerTypeAgent,
		ModelProvider: "acme",
	}

	if err := ValidateInvocationSkipPermissionsForWorker(agent, nil); err != nil {
		t.Fatalf("nil override: %v", err)
	}
	if err := ValidateInvocationSkipPermissionsForWorker(agent, &overrideTrue); err == nil {
		t.Fatal("expected unsupported provider to fail when override is set")
	}
}
