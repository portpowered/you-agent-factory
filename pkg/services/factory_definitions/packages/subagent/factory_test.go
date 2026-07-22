package subagent

import (
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

func TestBuiltInFactoryJSON_LoadsRunnablePackagedSubagentFactory(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Name != PackagedFactoryName {
		t.Fatalf("factory name = %q, want %s", cfg.Name, PackagedFactoryName)
	}
	if cfg.Project != PackagedFactoryProject {
		t.Fatalf("factory project = %q, want %s", cfg.Project, PackagedFactoryProject)
	}
	if cfg.InvocationSignature == nil {
		t.Fatal("InvocationSignature = nil, want packaged signature with request input")
	}

	assertOnePassWorkType(t, cfg)
	assertOnePassTopology(t, cfg)
	assertRequestContentPropagation(t, cfg)
	assertSharedAgentToolsPolicy(t, cfg)
	assertTopologyValidationPasses(t, cfg)
}

func assertOnePassWorkType(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	if len(cfg.WorkTypes) != 1 {
		t.Fatalf("workTypes = %#v, want one DEFAULT-handled work type", cfg.WorkTypes)
	}
	workType := cfg.WorkTypes[0]
	if workType.Name != PackagedWorkTypeName {
		t.Fatalf("work type name = %q, want %s", workType.Name, PackagedWorkTypeName)
	}
	if len(workType.HandlingBehavior) != 1 || workType.HandlingBehavior[0] != interfaces.WorkTypeHandlingBehaviorDefault {
		t.Fatalf("handlingBehavior = %#v, want [DEFAULT]", workType.HandlingBehavior)
	}

	stateNames := make([]string, 0, len(workType.States))
	stateTypes := map[string]interfaces.StateType{}
	for _, state := range workType.States {
		stateNames = append(stateNames, state.Name)
		stateTypes[state.Name] = state.Type
	}
	for _, want := range []string{"init", "complete", "failed"} {
		if !containsString(stateNames, want) {
			t.Fatalf("states = %#v, want lifecycle state %q", stateNames, want)
		}
	}
	if stateTypes["init"] != interfaces.StateTypeInitial {
		t.Fatalf("init state type = %q, want %s", stateTypes["init"], interfaces.StateTypeInitial)
	}
	if stateTypes["complete"] != interfaces.StateTypeTerminal {
		t.Fatalf("complete state type = %q, want %s", stateTypes["complete"], interfaces.StateTypeTerminal)
	}
	if stateTypes["failed"] != interfaces.StateTypeFailed {
		t.Fatalf("failed state type = %q, want %s", stateTypes["failed"], interfaces.StateTypeFailed)
	}
}

func assertOnePassTopology(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	if len(cfg.Workers) != 1 {
		t.Fatalf("workers = %#v, want exactly one AGENT_WORKER", cfg.Workers)
	}
	worker := cfg.Workers[0]
	if worker.Name != PackagedWorkerName {
		t.Fatalf("worker name = %q, want %s", worker.Name, PackagedWorkerName)
	}
	publicWorkerType := interfaces.PublicWorkerTypeFromInternalRuntime(worker.Type)
	if publicWorkerType != interfaces.WorkerTypeAgent {
		t.Fatalf("worker %q public type = %q, want %s", worker.Name, publicWorkerType, interfaces.WorkerTypeAgent)
	}

	if len(cfg.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want exactly one AGENT_RUN workstation", cfg.Workstations)
	}
	workstation := cfg.Workstations[0]
	if workstation.Name != PackagedRunWorkstationName {
		t.Fatalf("workstation name = %q, want %s", workstation.Name, PackagedRunWorkstationName)
	}
	publicWorkstationType := interfaces.PublicWorkstationTypeFromInternalRuntime(workstation.Type, worker.Type, workstation.Kind)
	if publicWorkstationType != interfaces.WorkstationTypeAgent {
		t.Fatalf("workstation %q public type = %q, want %s", workstation.Name, publicWorkstationType, interfaces.WorkstationTypeAgent)
	}
	if workstation.WorkerTypeName != PackagedWorkerName {
		t.Fatalf("workstation worker = %q, want %s", workstation.WorkerTypeName, PackagedWorkerName)
	}
	assertIORoute(t, workstation.Inputs, PackagedWorkTypeName, "init")
	assertIORoute(t, workstation.Outputs, PackagedWorkTypeName, "complete")
	assertIORoute(t, workstation.OnFailure, PackagedWorkTypeName, "failed")
}

func assertRequestContentPropagation(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	for _, workstation := range cfg.Workstations {
		if workstation.Name != PackagedRunWorkstationName {
			continue
		}
		if !strings.Contains(workstation.Body, "${input}") {
			t.Fatalf("run-subagent body = %q, want submitted invocation request interpolation", workstation.Body)
		}
		return
	}
	t.Fatal("run-subagent workstation not found")
}

func assertSharedAgentToolsPolicy(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	for _, worker := range cfg.Workers {
		if worker.Name != PackagedWorkerName {
			continue
		}
		if worker.AgentTools == nil {
			t.Fatal("subagent-worker agentTools = nil, want explicit shared policy")
		}
		if worker.AgentTools.Policy != workerconfig.AgentToolPolicyReadOnly {
			t.Fatalf("agentTools.policy = %q, want %s", worker.AgentTools.Policy, workerconfig.AgentToolPolicyReadOnly)
		}
		return
	}
	t.Fatal("subagent-worker not found")
}

func assertTopologyValidationPasses(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	for _, target := range factoryvalidation.Validate(cfg).Targets {
		t.Fatalf("validation target = %#v, want valid one-pass subagent topology", target)
	}
	findings := factoryvalidation.New(nil).
		ValidateTopology(t.Context(), cfg, nil).
		Findings
	if len(findings) != 0 {
		t.Fatalf("canonical structural findings = %#v, want none", findings)
	}
}

func assertIORoute(t *testing.T, routes []interfaces.IOConfig, workTypeName, stateName string) {
	t.Helper()
	for _, route := range routes {
		if route.WorkTypeName == workTypeName && route.StateName == stateName {
			return
		}
	}
	t.Fatalf("routes = %#v, want %s:%s", routes, workTypeName, stateName)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestIsPackagedFactory_MatchesBuiltInSubagentIdentity(t *testing.T) {
	if !IsPackagedFactory(&interfaces.FactoryConfig{Name: PackagedFactoryName}) {
		t.Fatal("expected packaged factory name match")
	}
	if !IsPackagedFactory(&interfaces.FactoryConfig{Project: PackagedFactoryProject}) {
		t.Fatal("expected packaged factory project match")
	}
	if IsPackagedFactory(&interfaces.FactoryConfig{Name: "customer-subagent"}) {
		t.Fatal("unexpected packaged factory match for unrelated factory")
	}
	if IsPackagedFactory(nil) {
		t.Fatal("expected nil factory config not to match")
	}
}
