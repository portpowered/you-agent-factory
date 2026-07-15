package contracttests

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestPublicWorkerTypeForFactoryUsage(t *testing.T) {
	t.Parallel()

	agentWorker := interfaces.WorkerConfig{Name: "executor", Type: interfaces.WorkerTypeModel}
	agentWorkstations := []interfaces.FactoryWorkstationConfig{{
		Name:           "execute-story",
		Type:           interfaces.WorkstationTypeModel,
		WorkerTypeName: "executor",
	}}

	if got := interfaces.PublicWorkerTypeForFactoryUsage(agentWorker, agentWorkstations); got != interfaces.WorkerTypeAgent {
		t.Fatalf("agent factory usage = %q, want %q", got, interfaces.WorkerTypeAgent)
	}

	inferenceWorker := interfaces.WorkerConfig{Name: "executor", Type: interfaces.WorkerTypeModel}
	inferenceWorkstations := []interfaces.FactoryWorkstationConfig{{
		Name:           "invoke-story",
		Type:           interfaces.WorkstationTypeInvoke,
		WorkerTypeName: "executor",
	}}
	if got := interfaces.PublicWorkerTypeForFactoryUsage(inferenceWorker, inferenceWorkstations); got != interfaces.WorkerTypeInference {
		t.Fatalf("inference factory usage = %q, want %q", got, interfaces.WorkerTypeInference)
	}

	mixedWorker := interfaces.WorkerConfig{Name: "executor", Type: interfaces.WorkerTypeModel}
	mixedWorkstations := []interfaces.FactoryWorkstationConfig{
		{
			Name:           "execute-story",
			Type:           interfaces.WorkstationTypeModel,
			WorkerTypeName: "executor",
		},
		{
			Name:           "invoke-story",
			Type:           interfaces.WorkstationTypeInvoke,
			WorkerTypeName: "executor",
		},
	}
	if got := interfaces.PublicWorkerTypeForFactoryUsage(mixedWorker, mixedWorkstations); got != interfaces.WorkerTypeModel {
		t.Fatalf("mixed legacy factory usage = %q, want %q", got, interfaces.WorkerTypeModel)
	}
}

type workerWorkstationBehaviorCase struct {
	name        string
	workerType  string
	workstation interfaces.FactoryWorkstationConfig
	want        bool
}

func workerMatchesWorkstationBehaviorCompatibleCases() []workerWorkstationBehaviorCase {
	return []workerWorkstationBehaviorCase{
		{
			name:       "inference run with inference worker",
			workerType: interfaces.WorkerTypeInference,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeInference,
			},
			want: true,
		},
		{
			name:       "legacy invoke with model worker",
			workerType: interfaces.WorkerTypeModel,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeInvoke,
			},
			want: true,
		},
		{
			name:       "legacy model workstation with model worker",
			workerType: interfaces.WorkerTypeModel,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeModel,
			},
			want: true,
		},
		{
			name:       "agent run with agent worker",
			workerType: interfaces.WorkerTypeAgent,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeAgent,
			},
			want: true,
		},
		{
			name:       "script run with script worker",
			workerType: interfaces.WorkerTypeScript,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeScript,
			},
			want: true,
		},
		{
			name:       "legacy model workstation with script worker",
			workerType: interfaces.WorkerTypeScript,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeModel,
			},
			want: true,
		},
		{
			name:       "poller run with poller worker",
			workerType: interfaces.WorkerTypePoller,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypePoller,
				Kind: interfaces.WorkstationKindPoller,
			},
			want: true,
		},
		{
			name:       "legacy hosted worker on poller workstation",
			workerType: interfaces.WorkerTypeHosted,
			workstation: interfaces.FactoryWorkstationConfig{
				Kind: interfaces.WorkstationKindPoller,
			},
			want: true,
		},
		{
			name:       "logical move exempt",
			workerType: interfaces.WorkerTypeInference,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeLogical,
			},
			want: true,
		},
	}
}

func workerMatchesWorkstationBehaviorIncompatibleCases() []workerWorkstationBehaviorCase {
	return []workerWorkstationBehaviorCase{
		{
			name:       "agent run with inference worker",
			workerType: interfaces.WorkerTypeInference,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeAgent,
			},
			want: false,
		},
		{
			name:       "agent run with legacy model worker",
			workerType: interfaces.WorkerTypeModel,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeAgent,
			},
			want: false,
		},
		{
			name:       "inference run with agent worker",
			workerType: interfaces.WorkerTypeAgent,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeInference,
			},
			want: false,
		},
		{
			name:       "poller run with inference worker",
			workerType: interfaces.WorkerTypeInference,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypePoller,
				Kind: interfaces.WorkstationKindPoller,
			},
			want: false,
		},
	}
}

func workerMatchesWorkstationBehaviorCases() []workerWorkstationBehaviorCase {
	cases := workerMatchesWorkstationBehaviorCompatibleCases()
	cases = append(cases, workerMatchesWorkstationBehaviorIncompatibleCases()...)
	return cases
}

func TestWorkerMatchesWorkstationBehavior(t *testing.T) {
	t.Parallel()

	for _, tt := range workerMatchesWorkstationBehaviorCases() {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := interfaces.WorkerMatchesWorkstationBehavior(tt.workerType, tt.workstation); got != tt.want {
				t.Fatalf("WorkerMatchesWorkstationBehavior(%q, %#v) = %v, want %v", tt.workerType, tt.workstation, got, tt.want)
			}
		})
	}
}

func TestWorkerWorkstationBehaviorHelpers(t *testing.T) {
	t.Parallel()

	t.Run("exempt workstation types", func(t *testing.T) {
		t.Parallel()
		if !interfaces.ExemptFromWorkerWorkstationCompatibility(interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeLogical}) {
			t.Fatal("logical move workstation should be exempt")
		}
		if !interfaces.ExemptFromWorkerWorkstationCompatibility(interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeClassify}) {
			t.Fatal("classifier workstation should be exempt")
		}
		if interfaces.ExemptFromWorkerWorkstationCompatibility(interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeAgent}) {
			t.Fatal("agent workstation should not be exempt")
		}
	})

	t.Run("effective workstation type defaults", func(t *testing.T) {
		t.Parallel()
		if got := interfaces.EffectiveWorkstationTypeForCompatibility(interfaces.FactoryWorkstationConfig{Type: "  "}); got != interfaces.WorkstationTypeModel {
			t.Fatalf("blank standard workstation type = %q, want %q", got, interfaces.WorkstationTypeModel)
		}
		if got := interfaces.EffectiveWorkstationTypeForCompatibility(interfaces.FactoryWorkstationConfig{Kind: interfaces.WorkstationKindPoller}); got != "" {
			t.Fatalf("blank poller workstation type = %q, want empty", got)
		}
		if got := interfaces.EffectiveWorkstationTypeForCompatibility(interfaces.FactoryWorkstationConfig{Type: "  " + interfaces.WorkstationTypeScript + "  "}); got != interfaces.WorkstationTypeScript {
			t.Fatalf("trimmed workstation type = %q, want %q", got, interfaces.WorkstationTypeScript)
		}
	})

	t.Run("worker behavior class mapping", func(t *testing.T) {
		t.Parallel()
		cases := map[string]interfaces.WorkerWorkstationBehaviorClass{
			interfaces.WorkerTypeInference: interfaces.WorkerWorkstationBehaviorInference,
			interfaces.WorkerTypeModel:     interfaces.WorkerWorkstationBehaviorInference,
			interfaces.WorkerTypeAgent:     interfaces.WorkerWorkstationBehaviorAgent,
			interfaces.WorkerTypeScript:    interfaces.WorkerWorkstationBehaviorScript,
			interfaces.WorkerTypePoller:    interfaces.WorkerWorkstationBehaviorPoller,
			interfaces.WorkerTypeHosted:    interfaces.WorkerWorkstationBehaviorPoller,
		}
		for workerType, want := range cases {
			got, ok := interfaces.WorkerBehaviorClass(workerType)
			if !ok || got != want {
				t.Fatalf("WorkerBehaviorClass(%q) = (%q, %v), want (%q, true)", workerType, got, ok, want)
			}
		}
		if got, ok := interfaces.WorkerBehaviorClass("CUSTOM"); ok || got != "" {
			t.Fatalf("WorkerBehaviorClass(custom) = (%q, %v), want empty false", got, ok)
		}
	})
}

func TestExpectedWorkerBehaviorClassForWorkstation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		workstation interfaces.FactoryWorkstationConfig
		workerType  string
		want        interfaces.WorkerWorkstationBehaviorClass
		wantOK      bool
	}{
		{
			name:        "inference workstation",
			workstation: interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeInference},
			workerType:  interfaces.WorkerTypeInference,
			want:        interfaces.WorkerWorkstationBehaviorInference,
			wantOK:      true,
		},
		{
			name:        "legacy model workstation with script worker projects to script",
			workstation: interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeModel},
			workerType:  interfaces.WorkerTypeScript,
			want:        interfaces.WorkerWorkstationBehaviorScript,
			wantOK:      true,
		},
		{
			name:        "poller kind blank type",
			workstation: interfaces.FactoryWorkstationConfig{Kind: interfaces.WorkstationKindPoller},
			workerType:  interfaces.WorkerTypeHosted,
			want:        interfaces.WorkerWorkstationBehaviorPoller,
			wantOK:      true,
		},
		{
			name:        "logical move exempt",
			workstation: interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeLogical},
			workerType:  interfaces.WorkerTypeInference,
			wantOK:      false,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := interfaces.ExpectedWorkerBehaviorClassForWorkstation(tt.workstation, tt.workerType)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("ExpectedWorkerBehaviorClassForWorkstation(%#v, %q) = (%q, %v), want (%q, %v)", tt.workstation, tt.workerType, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestCompatibilityProjectionHelpers(t *testing.T) {
	t.Parallel()

	if got := interfaces.EffectiveWorkstationBehaviorClass("", interfaces.WorkstationKindStandard, true); got != interfaces.WorkstationTypeAgent {
		t.Fatalf("EffectiveWorkstationBehaviorClass blank standard = %q, want %q", got, interfaces.WorkstationTypeAgent)
	}
	if got := interfaces.EffectiveWorkstationBehaviorClass("", interfaces.WorkstationKindPoller, true); got != interfaces.WorkstationTypePoller {
		t.Fatalf("EffectiveWorkstationBehaviorClass blank poller = %q, want %q", got, interfaces.WorkstationTypePoller)
	}
	if got := interfaces.EffectiveWorkstationBehaviorClass("", interfaces.WorkstationKindStandard, false); got != "" {
		t.Fatalf("EffectiveWorkstationBehaviorClass without worker = %q, want empty", got)
	}

	legacyPairs := []struct {
		workerType      string
		workstationType string
		kind            interfaces.WorkstationKind
	}{
		{workerType: interfaces.WorkerTypeModel, workstationType: interfaces.WorkstationTypeModel},
		{workerType: interfaces.WorkerTypeScript, workstationType: interfaces.WorkstationTypeModel},
		{workerType: interfaces.WorkerTypeInference, workstationType: "", kind: interfaces.WorkstationKindStandard},
		{workerType: interfaces.WorkerTypeHosted, workstationType: "", kind: interfaces.WorkstationKindPoller},
	}
	for _, tt := range legacyPairs {
		if !interfaces.IsLegacyGrandfatheredWorkerWorkstationPair(tt.workerType, tt.workstationType, tt.kind) {
			t.Fatalf("IsLegacyGrandfatheredWorkerWorkstationPair(%q, %q, %q) = false, want true", tt.workerType, tt.workstationType, tt.kind)
		}
	}
	if interfaces.IsLegacyGrandfatheredWorkerWorkstationPair(interfaces.WorkerTypeAgent, interfaces.WorkstationTypeInference, "") {
		t.Fatal("agent worker and inference workstation should not be grandfathered")
	}

	if !interfaces.RequiresWorkerWorkstationBehaviorCompatibility(interfaces.WorkstationTypeAgent, "", "worker-name") {
		t.Fatal("agent workstation with bound worker should require compatibility")
	}
	if interfaces.RequiresWorkerWorkstationBehaviorCompatibility(interfaces.WorkstationTypeLogical, "", "worker-name") {
		t.Fatal("logical workstation should not require compatibility")
	}
	if interfaces.RequiresWorkerWorkstationBehaviorCompatibility(interfaces.WorkstationTypeAgent, "", "") {
		t.Fatal("workstation without bound worker should not require compatibility")
	}
}

func TestCompatibleWorkerWorkstationBehaviorAndMismatchMessage(t *testing.T) {
	t.Parallel()

	if !interfaces.CompatibleWorkerWorkstationBehavior(interfaces.WorkerTypeModel, interfaces.WorkstationTypeModel, "") {
		t.Fatal("legacy model worker/model workstation should be compatible")
	}
	if !interfaces.CompatibleWorkerWorkstationBehavior("", interfaces.WorkstationTypeAgent, "") {
		t.Fatal("empty worker type should be treated as compatible")
	}
	if interfaces.CompatibleWorkerWorkstationBehavior(interfaces.WorkerTypeAgent, interfaces.WorkstationTypeInference, "") {
		t.Fatal("agent worker and inference workstation should not be compatible")
	}

	if got := interfaces.RuntimeBehaviorClassLabel(interfaces.WorkerTypeInference); got != "inference" {
		t.Fatalf("RuntimeBehaviorClassLabel inference = %q, want inference", got)
	}
	if got := interfaces.RuntimeBehaviorClassLabel("  CUSTOM_BEHAVIOR  "); got != "custom_behavior" {
		t.Fatalf("RuntimeBehaviorClassLabel custom = %q, want custom_behavior", got)
	}

	message := interfaces.WorkerWorkstationBehaviorMismatchMessage(
		"review",
		"",
		interfaces.WorkstationKindPoller,
		"planner",
		"",
	)
	want := `workstation "review" (legacy poller kind) is a poller-run workstation but worker "planner" (unspecified worker type) is a  worker; bind a compatible poller worker or change the workstation type`
	if message != want {
		t.Fatalf("WorkerWorkstationBehaviorMismatchMessage = %q, want %q", message, want)
	}
}

func TestPublicWorkerTypeForFactoryUsage_Fallbacks(t *testing.T) {
	t.Parallel()

	if got := interfaces.PublicWorkerTypeForFactoryUsage(
		interfaces.WorkerConfig{Name: "", Type: interfaces.WorkerTypeModel},
		[]interfaces.FactoryWorkstationConfig{{Type: interfaces.WorkstationTypeAgent, WorkerTypeName: "executor"}},
	); got != interfaces.WorkerTypeInference {
		t.Fatalf("model worker without name = %q, want %q", got, interfaces.WorkerTypeInference)
	}

	if got := interfaces.PublicWorkerTypeForFactoryUsage(
		interfaces.WorkerConfig{Name: "executor", Type: interfaces.WorkerTypeAgent},
		nil,
	); got != interfaces.WorkerTypeAgent {
		t.Fatalf("non-model worker type projection = %q, want %q", got, interfaces.WorkerTypeAgent)
	}
}
