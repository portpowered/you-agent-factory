package interfaces

import "testing"

func TestCanonicalFactoryGraphEntityIDFallsBackToLegacyName(t *testing.T) {
	t.Parallel()

	if got := CanonicalFactoryGraphEntityID("", "legacy-resource"); got != "legacy-resource" {
		t.Fatalf("CanonicalFactoryGraphEntityID fallback = %q, want legacy-resource", got)
	}
	if got := CanonicalFactoryGraphNodeID("resource", CanonicalFactoryGraphEntityID("", "legacy-resource")); got != "resource:legacy-resource" {
		t.Fatalf("CanonicalFactoryGraphNodeID fallback = %q, want resource:legacy-resource", got)
	}
}

func TestCanonicalFactoryGraphIDsStayStableAcrossRenamesWhenExplicitIDsExist(t *testing.T) {
	t.Parallel()

	beforeRenameWorkType := WorkTypeConfig{ID: "work-type-story", Name: "story"}
	beforeRenameState := StateConfig{ID: "state-queued", Name: "queued"}
	beforeRenameWorkstation := FactoryWorkstationConfig{ID: "workstation-review", Name: "review"}
	beforeNodeID := CanonicalFactoryGraphNodeID(
		"work-state",
		CanonicalFactoryGraphWorkStateID(beforeRenameWorkType, beforeRenameState),
	)
	beforeEdgeID := CanonicalFactoryGraphEdgeID(
		"workstation-input",
		beforeNodeID,
		CanonicalFactoryGraphNodeID(
			"workstation",
			CanonicalFactoryGraphWorkstationID(beforeRenameWorkstation),
		),
	)

	afterRenameWorkType := WorkTypeConfig{ID: "work-type-story", Name: "renamed-story"}
	afterRenameState := StateConfig{ID: "state-queued", Name: "renamed-queued"}
	afterRenameWorkstation := FactoryWorkstationConfig{ID: "workstation-review", Name: "renamed-review"}
	afterNodeID := CanonicalFactoryGraphNodeID(
		"work-state",
		CanonicalFactoryGraphWorkStateID(afterRenameWorkType, afterRenameState),
	)
	afterEdgeID := CanonicalFactoryGraphEdgeID(
		"workstation-input",
		afterNodeID,
		CanonicalFactoryGraphNodeID(
			"workstation",
			CanonicalFactoryGraphWorkstationID(afterRenameWorkstation),
		),
	)

	if beforeNodeID != afterNodeID {
		t.Fatalf("work-state node id changed across rename: before=%q after=%q", beforeNodeID, afterNodeID)
	}
	if beforeEdgeID != afterEdgeID {
		t.Fatalf("edge id changed across rename: before=%q after=%q", beforeEdgeID, afterEdgeID)
	}
}
