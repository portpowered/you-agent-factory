package factory

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestCloneRuntimeMutableFields_PreservesNilInputs(t *testing.T) {
	if CloneRuntimeTags(nil) != nil {
		t.Fatal("CloneRuntimeTags(nil) = non-nil, want nil")
	}
	if CloneRuntimeRelations(nil) != nil {
		t.Fatal("CloneRuntimeRelations(nil) = non-nil, want nil")
	}
	if CloneRuntimePayload(nil) != nil {
		t.Fatal("CloneRuntimePayload(nil) = non-nil, want nil")
	}
}

func TestCloneRuntimeMutableFields_DetachesNonEmptyInputs(t *testing.T) {
	tags := map[string]string{"priority": "high"}
	relations := []interfaces.Relation{{
		Type:          interfaces.RelationDependsOn,
		TargetWorkID:  "work-upstream",
		RequiredState: "done",
	}}
	payload := []byte("payload")

	clonedTags := CloneRuntimeTags(tags)
	clonedRelations := CloneRuntimeRelations(relations)
	clonedPayload := CloneRuntimePayload(payload)

	clonedTags["priority"] = "low"
	clonedRelations[0].TargetWorkID = "work-mutated"
	clonedPayload[0] = 'P'

	if got := tags["priority"]; got != "high" {
		t.Fatalf("source tags mutated to %q, want high", got)
	}
	if got := relations[0].TargetWorkID; got != "work-upstream" {
		t.Fatalf("source relation target = %q, want work-upstream", got)
	}
	if got := string(payload); got != "payload" {
		t.Fatalf("source payload = %q, want payload", got)
	}
}

func TestCloneRuntimeMutableFields_NormalizesEmptyInputsToNil(t *testing.T) {
	tags := map[string]string{}
	relations := []interfaces.Relation{}
	payload := []byte{}

	if CloneRuntimeTags(tags) != nil {
		t.Fatal("CloneRuntimeTags(empty) = non-nil, want nil")
	}
	if CloneRuntimeRelations(relations) != nil {
		t.Fatal("CloneRuntimeRelations(empty) = non-nil, want nil")
	}
	if CloneRuntimePayload(payload) != nil {
		t.Fatal("CloneRuntimePayload(empty) = non-nil, want nil")
	}
}
