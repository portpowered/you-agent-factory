package quorumpolicy

import (
	"reflect"
	"testing"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestQuorumIdentityUsesDeclaredNameOrProject(t *testing.T) {
	policy := Service{}
	if policy.IsPackagedQuorumFactory(nil) || policy.IsPackagedQuorumFactory(&definitions.FactoryConfig{Name: "other"}) {
		t.Fatal("unrelated definition identified as Quorum")
	}
	for _, cfg := range []*definitions.FactoryConfig{{Name: " " + definitions.PackagedQuorumFactoryName + " "}, {Project: " " + definitions.PackagedQuorumFactoryProject + " "}} {
		if !policy.IsPackagedQuorumFactory(cfg) {
			t.Fatal("declared Quorum identity rejected")
		}
	}
}

func TestQuorumRelationsKeepOnlyRelevantLineage(t *testing.T) {
	policy := Service{}
	for _, args := range [][3]string{{"other", "parent", "branch"}, {definitions.PackagedQuorumSplitWorkstationName, "", "branch"}, {definitions.PackagedQuorumSplitWorkstationName, "parent", "task"}, {definitions.PackagedQuorumMergeWorkstationName, "parent", "task"}} {
		if got := policy.WorkRelations(args[0], args[1], args[2], nil); len(got) != 0 {
			t.Fatalf("irrelevant lineage = %#v", got)
		}
	}
	got := policy.WorkRelations(definitions.PackagedQuorumSplitWorkstationName, "parent", "branch", nil)
	want := []work.Relation{{Type: work.RelationParentChild, TargetWorkID: "parent"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("split lineage = %#v", got)
	}
	inputs := []definitions.QuorumLineageInput{{WorkTypeID: "quorum-branch-a"}, {WorkID: "ignored", WorkTypeID: "task"}, {WorkID: "a", WorkTypeID: "quorum-branch-a"}, {WorkID: "b", WorkTypeID: "quorum-branch-b"}}
	got = policy.WorkRelations(definitions.PackagedQuorumMergeWorkstationName, "parent", "quorum-merge", inputs)
	want = []work.Relation{{Type: work.RelationDependsOn, TargetWorkID: "a", RequiredState: "complete"}, {Type: work.RelationDependsOn, TargetWorkID: "b", RequiredState: "complete"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge lineage = %#v", got)
	}
}

func TestPolicy_QuorumPolicy(t *testing.T) {
	t.Parallel()

	quorum := NewService()
	if !quorum.IsPackagedQuorumFactory(&factorydefinitions.FactoryConfig{
		Name: factorydefinitions.PackagedQuorumFactoryName,
	}) {
		t.Fatal("IsPackagedQuorumFactory() = false, want true for packaged quorum factory")
	}

	relations := quorum.WorkRelations(
		factorydefinitions.PackagedQuorumSplitWorkstationName,
		"task-1",
		"quorum-branch-a",
		nil,
	)
	if len(relations) != 1 || relations[0].Type != work.RelationParentChild || relations[0].TargetWorkID != "task-1" {
		t.Fatalf("split WorkRelations = %#v, want parent-child to task-1", relations)
	}

	branches := []factorydefinitions.QuorumLineageInput{
		{WorkID: "branch-a", WorkTypeID: "quorum-branch-a"},
		{WorkID: "branch-b", WorkTypeID: "quorum-branch-b"},
	}
	mergeRelations := quorum.WorkRelations(
		factorydefinitions.PackagedQuorumMergeWorkstationName,
		"",
		"quorum-merge",
		branches,
	)
	if len(mergeRelations) != 2 {
		t.Fatalf("merge WorkRelations = %#v, want two branch dependencies", mergeRelations)
	}
}
