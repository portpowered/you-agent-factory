package compatibility

import (
	"testing"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"
)

func TestWorkerWorkstationCompatibilityPreservesLegacyAndStrictPairings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		worker      string
		workstation Workstation
		want        bool
	}{
		{workertaxonomy.WorkerTypeAgent, Workstation{Type: workertaxonomy.WorkstationTypeAgent}, true},
		{workertaxonomy.WorkerTypeInference, Workstation{Type: workertaxonomy.WorkstationTypeAgent}, false},
		{workertaxonomy.WorkerTypeModel, Workstation{Type: workertaxonomy.WorkstationTypeModel}, true},
		{workertaxonomy.WorkerTypeHosted, Workstation{Kind: workertaxonomy.WorkstationKindPoller}, true},
	}
	for _, tc := range cases {
		if got := WorkerMatchesWorkstationBehavior(tc.worker, tc.workstation); got != tc.want {
			t.Fatalf("WorkerMatchesWorkstationBehavior(%q, %#v) = %v, want %v", tc.worker, tc.workstation, got, tc.want)
		}
	}
}

func TestPublicWorkerTypeForFactoryUsagePreservesMixedLegacyAlias(t *testing.T) {
	t.Parallel()
	worker := workerconfig.Config{Name: "executor", Type: workertaxonomy.WorkerTypeModel}
	workstations := []Workstation{{Type: workertaxonomy.WorkstationTypeModel, WorkerTypeName: "executor"}, {Type: workertaxonomy.WorkstationTypeInvoke, WorkerTypeName: "executor"}}
	if got := PublicWorkerTypeForFactoryUsage(worker, workstations); got != workertaxonomy.WorkerTypeModel {
		t.Fatalf("mixed usage = %q", got)
	}
}
