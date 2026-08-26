package skippermissions

import (
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
