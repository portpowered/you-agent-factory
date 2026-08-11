package packagedfactorycatalog_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestValidateFirstPartyWorkStateRolesAcceptsEveryImplementedRouteRole(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name:             "task",
				HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "routed", Type: factorydefinitions.StateTypeProcessing},
					{Name: "classified", Type: factorydefinitions.StateTypeProcessing},
					{Name: "continued", Type: factorydefinitions.StateTypeProcessing},
					{Name: "rejected", Type: factorydefinitions.StateTypeProcessing},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
					{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				},
			},
		},
		InvocationReturn: &factorydefinitions.InvocationReturnConfig{
			Policy:        factorydefinitions.InvocationReturnPolicyExplicit,
			WorkTypeName:  "task",
			TerminalState: "complete",
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{
			{
				Name: "route",
				Inputs: []factorydefinitions.IOConfig{{
					WorkTypeName: "task", StateName: "init",
				}},
				Outputs: []factorydefinitions.IOConfig{{
					WorkTypeName: "task", StateName: "routed",
				}},
				OnContinue: []factorydefinitions.IOConfig{{
					WorkTypeName: "task", StateName: "continued",
				}},
				OnRejection: []factorydefinitions.IOConfig{{
					WorkTypeName: "task", StateName: "rejected",
				}},
				OnFailure: []factorydefinitions.IOConfig{{
					WorkTypeName: "task", StateName: "failed",
				}},
				ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{{
					Label: "classified",
					Outputs: []factorydefinitions.IOConfig{{
						WorkTypeName: "task", StateName: "classified",
					}},
				}},
			},
		},
	}

	if err := packagedfactorycatalog.ValidateFirstPartyWorkStateRoles("route-complete", cfg); err != nil {
		t.Fatalf("ValidateFirstPartyWorkStateRoles() error = %v", err)
	}
}

func TestValidateFirstPartyWorkStateRolesRejectsDisconnectedStateWithExactIdentity(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name:             "task",
			HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
				{Name: "orphan", Type: factorydefinitions.StateTypeProcessing},
				{Name: "terminal-orphan", Type: factorydefinitions.StateTypeTerminal},
			},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:    "execute",
			Inputs:  []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			OnFailure: []factorydefinitions.IOConfig{{
				WorkTypeName: "task", StateName: "failed",
			}},
		}},
	}

	err := packagedfactorycatalog.ValidateFirstPartyWorkStateRoles("synthetic", cfg)
	if err == nil {
		t.Fatal("ValidateFirstPartyWorkStateRoles() error = nil, want disconnected-state diagnostic")
	}
	if !strings.Contains(err.Error(), `"synthetic"`) ||
		!strings.Contains(err.Error(), "task:orphan") ||
		!strings.Contains(err.Error(), "task:terminal-orphan") {
		t.Fatalf("error = %q, want Factory slug and exact disconnected state identities", err)
	}
}

func TestValidateFirstPartyWorkStateRolesAcceptsInvocationScheduleOverlapBridge(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{
				Name:   "controller",
				States: []factorydefinitions.StateConfig{{Name: "active", Type: factorydefinitions.StateTypeInitial}},
			},
			{
				Name: "scheduled",
				States: []factorydefinitions.StateConfig{
					{Name: "init", Type: factorydefinitions.StateTypeInitial},
					{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
					{Name: "skipped", Type: factorydefinitions.StateTypeTerminal},
				},
			},
		},
		InvocationReturn: &factorydefinitions.InvocationReturnConfig{
			Policy:        factorydefinitions.InvocationReturnPolicyExplicit,
			WorkTypeName:  "scheduled",
			TerminalState: "complete",
		},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name: "schedule",
			Kind: factorydefinitions.WorkstationKindCron,
			Cron: &factorydefinitions.CronConfig{Every: "1m"},
			Inputs: []factorydefinitions.IOConfig{{
				WorkTypeName: "controller", StateName: "active",
			}},
			Outputs: []factorydefinitions.IOConfig{{
				WorkTypeName: "controller", StateName: "active",
			}, {
				WorkTypeName: "scheduled", StateName: "init",
			}},
		}},
	}

	if err := packagedfactorycatalog.ValidateFirstPartyWorkStateRoles("synthetic-loop", cfg); err != nil {
		t.Fatalf("ValidateFirstPartyWorkStateRoles() error = %v, want named scheduler bridge to accept scheduled:skipped: %v", err, err)
	}
}

func TestValidateFirstPartyWorkStateRolesSkipsJavaScriptFactories(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind: factorydefinitions.OrchestratorKindJavaScript,
		},
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name:   "runtime-owned",
			States: []factorydefinitions.StateConfig{{Name: "unlisted", Type: factorydefinitions.StateTypeProcessing}},
		}},
	}

	if err := packagedfactorycatalog.ValidateFirstPartyWorkStateRoles("javascript", cfg); err != nil {
		t.Fatalf("ValidateFirstPartyWorkStateRoles() error = %v, want JavaScript catalog skipped: %v", err, err)
	}
}
