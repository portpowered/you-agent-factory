package quorumpolicy

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestPackagedQuorumIdentityMatrix(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		cfg  *factorydefinitions.FactoryConfig
		want bool
	}{
		{name: "nil"},
		{
			name: "packaged name",
			cfg:  &factorydefinitions.FactoryConfig{Name: factorydefinitions.PackagedQuorumFactoryName},
			want: true,
		},
		{
			name: "packaged project",
			cfg:  &factorydefinitions.FactoryConfig{Project: factorydefinitions.PackagedQuorumFactoryProject},
			want: true,
		},
		{
			name: "customer name",
			cfg:  &factorydefinitions.FactoryConfig{Name: "customer-quorum"},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPackagedQuorumFactory(testCase.cfg); got != testCase.want {
				t.Fatalf("IsPackagedQuorumFactory() = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestPackagedQuorumWorkRelationsMatrix(t *testing.T) {
	t.Parallel()

	branches := []factorydefinitions.QuorumLineageInput{
		{WorkID: "branch-a", WorkTypeID: "quorum-branch-a"},
		{WorkID: "ignored", WorkTypeID: "task"},
		{WorkID: "branch-b", WorkTypeID: "quorum-branch-b"},
	}
	tests := []struct {
		name        string
		workstation string
		parentID    string
		workTypeID  string
		inputs      []factorydefinitions.QuorumLineageInput
		want        []work.Relation
	}{
		{
			name:        "split branch",
			workstation: factorydefinitions.PackagedQuorumSplitWorkstationName,
			parentID:    "task-1",
			workTypeID:  "quorum-branch-a",
			want: []work.Relation{{
				Type: work.RelationParentChild, TargetWorkID: "task-1",
			}},
		},
		{
			name:        "merge branches",
			workstation: factorydefinitions.PackagedQuorumMergeWorkstationName,
			workTypeID:  "quorum-merge",
			inputs:      branches,
			want: []work.Relation{
				{Type: work.RelationDependsOn, TargetWorkID: "branch-a", RequiredState: "complete"},
				{Type: work.RelationDependsOn, TargetWorkID: "branch-b", RequiredState: "complete"},
			},
		},
		{
			name:        "customer workstation",
			workstation: "merge-customer",
			workTypeID:  "quorum-merge",
			inputs:      branches,
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := WorkRelations(
				testCase.workstation,
				testCase.parentID,
				testCase.workTypeID,
				testCase.inputs,
			)
			if len(got) != len(testCase.want) {
				t.Fatalf("relations = %#v, want %#v", got, testCase.want)
			}
			for index := range got {
				if got[index] != testCase.want[index] {
					t.Fatalf("relations[%d] = %#v, want %#v", index, got[index], testCase.want[index])
				}
			}
		})
	}
}
