package layouttests

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestFactoryConfig_SharedSurfaceRetiresExhaustionRules(t *testing.T) {
	factoryType := reflect.TypeOf(interfaces.FactoryConfig{})
	if _, ok := factoryType.FieldByName("ExhaustionRules"); ok {
		t.Fatal("interfaces.FactoryConfig must not expose ExhaustionRules")
	}
}

func TestCloneFactoryConfig_PreservesGuardedLogicalMoveLoopBreakerWorkstations(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "review", Type: interfaces.StateTypeProcessing},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "review-story",
				WorkerTypeName: "reviewer",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "review"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "review"}},
				OnRejection:    []interfaces.IOConfig{{WorkTypeName: "story", StateName: "review"}},
			},
			{
				Name:    "review-loop-breaker",
				Type:    interfaces.WorkstationTypeLogical,
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "story", StateName: "review"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
				Guards: []interfaces.GuardConfig{{
					Type:        interfaces.GuardTypeVisitCount,
					Workstation: "review-story",
					MaxVisits:   3,
				}},
			},
		},
	}

	cloned, err := interfaces.CloneFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	if len(cloned.Workstations) != 2 {
		t.Fatalf("expected 2 workstations, got %#v", cloned.Workstations)
	}
	loopBreaker := cloned.Workstations[1]
	if loopBreaker.Name != "review-loop-breaker" || loopBreaker.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("expected guarded logical move loop breaker to be preserved, got %#v", loopBreaker)
	}
	if len(loopBreaker.Guards) != 1 {
		t.Fatalf("expected one loop-breaker guard, got %#v", loopBreaker.Guards)
	}
	if guard := loopBreaker.Guards[0]; guard.Type != interfaces.GuardTypeVisitCount || guard.Workstation != "review-story" || guard.MaxVisits != 3 {
		t.Fatalf("expected visit_count guard details to survive clone, got %#v", guard)
	}

	cfg.Workstations[1].Guards[0].Workstation = "mutated"
	if cloned.Workstations[1].Guards[0].Workstation != "review-story" {
		t.Fatalf("expected cloned guard to be independent of source mutations, got %#v", cloned.Workstations[1].Guards[0])
	}
}

func TestCloneFactoryConfig_ClonesMatchesFieldsGuardMatchConfig(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "match-assets",
			Guards: []interfaces.GuardConfig{{
				Type:        interfaces.GuardTypeMatchesFields,
				MatchConfig: &interfaces.GuardMatchConfig{InputKey: ".Name"},
			}},
		}},
	}

	cloned, err := interfaces.CloneFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	cfg.Workstations[0].Guards[0].MatchConfig.InputKey = `.Tags["_last_output"]`
	if cloned.Workstations[0].Guards[0].MatchConfig == nil || cloned.Workstations[0].Guards[0].MatchConfig.InputKey != ".Name" {
		t.Fatalf("expected cloned matchConfig to be independent of source mutations, got %#v", cloned.Workstations[0].Guards[0].MatchConfig)
	}
}

func TestCloneFactoryConfig_PreservesFactoryGuards(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Guards: []interfaces.FactoryGuardConfig{{
			Type:          interfaces.GuardTypeInferenceThrottle,
			ModelProvider: "claude",
			Model:         "claude-sonnet",
			RefreshWindow: "15m",
		}},
	}

	cloned, err := interfaces.CloneFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	if len(cloned.Guards) != 1 {
		t.Fatalf("expected cloned factory guard, got %#v", cloned.Guards)
	}
	if cloned.Guards[0] != cfg.Guards[0] {
		t.Fatalf("cloned factory guard = %#v, want %#v", cloned.Guards[0], cfg.Guards[0])
	}

	cfg.Guards[0].ModelProvider = "codex"
	if cloned.Guards[0].ModelProvider != "claude" {
		t.Fatalf("expected cloned factory guard to be independent of source mutations, got %#v", cloned.Guards[0])
	}
}

func TestCloneFactoryConfig_PreservesPortableLayout(t *testing.T) {
	locked := true
	parentGroupID := "group-root"
	cfg := &interfaces.FactoryConfig{
		Layout: &interfaces.FactoryLayoutConfig{
			SchemaVersion: 1,
			Nodes: []interfaces.FactoryLayoutNodeConfig{{
				ID:       "workstation:plan-task",
				Position: interfaces.FactoryLayoutPointConfig{X: 144, Y: 288},
				Size:     &interfaces.FactoryLayoutSizeConfig{Width: 320, Height: 180},
				Locked:   &locked,
				EmptyState: &interfaces.FactoryLayoutEmptyStateConfig{Image: &interfaces.FactoryLayoutImageConfig{
					Source:          interfaces.FactoryLayoutImageSourceConfig{Kind: "EMBEDDED", MediaType: "image/png", Data: "AQID"},
					AlternativeText: "Planning queue is empty",
				}},
			}},
			Edges: []interfaces.FactoryLayoutEdgeConfig{{
				ID: "output:workstation:plan-task->work-type:story",
				Waypoints: []interfaces.FactoryLayoutPointConfig{{
					X: 200,
					Y: 300,
				}},
				LabelPosition: &interfaces.FactoryLayoutPointConfig{X: 220, Y: 280},
			}},
			Groups: []interfaces.FactoryLayoutGroupConfig{{
				ID:            "group-1",
				Label:         "Planning",
				NodeIDs:       []string{"workstation:plan-task"},
				Bounds:        interfaces.FactoryLayoutBoundsConfig{X: 100, Y: 220, Width: 420, Height: 240},
				ParentGroupID: &parentGroupID,
				Color:         "#ddeeff",
				Locked:        &locked,
			}},
			Viewport:    &interfaces.FactoryLayoutViewportConfig{X: 40, Y: 60, Zoom: 0.85},
			Preferences: &interfaces.FactoryLayoutPreferencesConfig{Direction: "RIGHT"},
		},
	}

	cloned, err := interfaces.CloneFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	if cloned.Layout == nil || cloned.Layout.SchemaVersion != 1 {
		t.Fatalf("cloned layout = %#v, want preserved layout", cloned.Layout)
	}
	if !reflect.DeepEqual(cloned.Layout, cfg.Layout) {
		t.Fatalf("cloned layout = %#v, want %#v", cloned.Layout, cfg.Layout)
	}

	cfg.Layout.Nodes[0].Position.X = 999
	cfg.Layout.Nodes[0].EmptyState.Image.AlternativeText = "mutated empty state"
	cfg.Layout.Edges[0].Waypoints[0].X = 777
	cfg.Layout.Groups[0].NodeIDs[0] = "mutated"
	cfg.Layout.Viewport.Zoom = 0.5
	cfg.Layout.Preferences.Direction = "LEFT"

	if cloned.Layout.Nodes[0].Position.X != 144 {
		t.Fatalf("cloned layout node position mutated with source: %#v", cloned.Layout.Nodes[0].Position)
	}
	if cloned.Layout.Nodes[0].EmptyState.Image.AlternativeText != "Planning queue is empty" {
		t.Fatalf("cloned node empty state mutated with source: %#v", cloned.Layout.Nodes[0].EmptyState)
	}
	if cloned.Layout.Edges[0].Waypoints[0].X != 200 {
		t.Fatalf("cloned layout edge waypoint mutated with source: %#v", cloned.Layout.Edges[0].Waypoints)
	}
	if cloned.Layout.Groups[0].NodeIDs[0] != "workstation:plan-task" {
		t.Fatalf("cloned layout group node ids mutated with source: %#v", cloned.Layout.Groups[0].NodeIDs)
	}
	if cloned.Layout.Viewport.Zoom != 0.85 {
		t.Fatalf("cloned layout viewport mutated with source: %#v", cloned.Layout.Viewport)
	}
	if cloned.Layout.Preferences.Direction != "RIGHT" {
		t.Fatalf("cloned layout preferences mutated with source: %#v", cloned.Layout.Preferences)
	}
}

func TestCloneFactoryConfig_ClonesModelOperationBindingWorkContent(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:      "writer",
			Operation: "MODEL_INVOKE",
			OperationBindings: []interfaces.ModelOperationBinding{{
				Slot: "draft",
				Config: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "configured prompt",
				}, {
					Type:  work.WorkContentPartTypeImage,
					File:  "configured-diagram.png",
					Label: "Configured diagram",
				}},
				DefaultContent: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "fallback prompt",
				}, {
					Type:  work.WorkContentPartTypeImage,
					File:  "fallback-diagram.png",
					Label: "Fallback diagram",
				}},
			}, {
				Slot:           "empty",
				Config:         []work.WorkContentPart{},
				DefaultContent: []work.WorkContentPart{},
			}},
		}},
	}

	cloned, err := interfaces.CloneFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("CloneFactoryConfig: %v", err)
	}
	bindings := cloned.Workstations[0].OperationBindings
	if len(bindings) != 2 {
		t.Fatalf("cloned bindings = %#v, want two bindings", bindings)
	}
	assertBindingMultiPartContent(t, bindings[0].Config, "configured prompt", "configured-diagram.png", "Configured diagram", "config")
	assertBindingMultiPartContent(t, bindings[0].DefaultContent, "fallback prompt", "fallback-diagram.png", "Fallback diagram", "default")
	if bindings[1].Config != nil {
		t.Fatalf("empty config content = %#v, want nil", bindings[1].Config)
	}
	if bindings[1].DefaultContent != nil {
		t.Fatalf("empty default content = %#v, want nil", bindings[1].DefaultContent)
	}

	cfg.Workstations[0].OperationBindings[0].Config[0].Text = "mutated config"
	cfg.Workstations[0].OperationBindings[0].Config[1].File = "mutated-configured-diagram.png"
	cfg.Workstations[0].OperationBindings[0].DefaultContent[0].Text = "mutated default"
	cfg.Workstations[0].OperationBindings[0].DefaultContent[1].Label = "Mutated fallback diagram"
	assertBindingMultiPartContent(t, bindings[0].Config, "configured prompt", "configured-diagram.png", "Configured diagram", "config after source mutation")
	assertBindingMultiPartContent(t, bindings[0].DefaultContent, "fallback prompt", "fallback-diagram.png", "Fallback diagram", "default after source mutation")
}

func TestCloneWorkerConfig_PreservesNilHostedConfig(t *testing.T) {
	cloned := interfaces.CloneWorkerConfig(interfaces.FactoryWorkerConfig{
		Name:     "hosted-linear",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
	})

	if cloned.Auth != nil {
		t.Fatalf("cloned auth = %#v, want nil", cloned.Auth)
	}
	if cloned.Linear != nil {
		t.Fatalf("cloned linear = %#v, want nil", cloned.Linear)
	}
}

func TestCloneWorkerConfig_DetachesHostedNestedConfig(t *testing.T) {
	source := interfaces.FactoryWorkerConfig{
		Name:     "hosted-linear",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Auth: &interfaces.HostedWorkerAuthConfig{
			SecretRef: "linear-secret",
		},
		Linear: &interfaces.HostedLinearWorkerConfig{
			PollInterval: "30s",
			TeamIDs:      []string{"team-1", "team-2"},
			StateIDs:     []string{"state-1", "state-2"},
			Claim: &interfaces.HostedLinearWorkerClaimConfig{
				AssigneeField: "owner",
			},
		},
	}

	cloned := interfaces.CloneWorkerConfig(source)
	if cloned.Auth == nil || cloned.Auth.SecretRef != "linear-secret" {
		t.Fatalf("cloned auth = %#v, want preserved hosted auth", cloned.Auth)
	}
	if cloned.Linear == nil {
		t.Fatal("cloned linear = nil, want detached hosted linear config")
	}
	if !reflect.DeepEqual(cloned.Linear.TeamIDs, []string{"team-1", "team-2"}) {
		t.Fatalf("cloned TeamIDs = %#v, want preserved values", cloned.Linear.TeamIDs)
	}
	if !reflect.DeepEqual(cloned.Linear.StateIDs, []string{"state-1", "state-2"}) {
		t.Fatalf("cloned StateIDs = %#v, want preserved values", cloned.Linear.StateIDs)
	}
	if cloned.Linear.Claim == nil || cloned.Linear.Claim.AssigneeField != "owner" {
		t.Fatalf("cloned claim = %#v, want preserved claim config", cloned.Linear.Claim)
	}

	source.Auth.SecretRef = "mutated-secret"
	source.Linear.TeamIDs[0] = "mutated-team"
	source.Linear.StateIDs[0] = "mutated-state"
	source.Linear.Claim.AssigneeField = "mutated-owner"

	if cloned.Auth.SecretRef != "linear-secret" {
		t.Fatalf("cloned auth after source mutation = %#v, want detached auth", cloned.Auth)
	}
	if !reflect.DeepEqual(cloned.Linear.TeamIDs, []string{"team-1", "team-2"}) {
		t.Fatalf("cloned TeamIDs after source mutation = %#v, want detached values", cloned.Linear.TeamIDs)
	}
	if !reflect.DeepEqual(cloned.Linear.StateIDs, []string{"state-1", "state-2"}) {
		t.Fatalf("cloned StateIDs after source mutation = %#v, want detached values", cloned.Linear.StateIDs)
	}
	if cloned.Linear.Claim == nil || cloned.Linear.Claim.AssigneeField != "owner" {
		t.Fatalf("cloned claim after source mutation = %#v, want detached claim config", cloned.Linear.Claim)
	}
}

func assertBindingMultiPartContent(
	t *testing.T,
	parts []work.WorkContentPart,
	wantText string,
	wantFile string,
	wantLabel string,
	label string,
) {
	t.Helper()
	if parts == nil || len(parts) != 2 {
		t.Fatalf("%s content = %#v, want two detached parts", label, parts)
	}
	if parts[0].Text != wantText || parts[1].File != wantFile || parts[1].Label != wantLabel {
		t.Fatalf("%s content = %#v, want preserved multi-part content", label, parts)
	}
}
