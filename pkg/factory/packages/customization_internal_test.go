package packages

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestValidateCustomizationClassifierBranches(t *testing.T) {
	if err := ValidateCustomization(nil); err != nil {
		t.Fatalf("nil customization: %v", err)
	}
	if err := ValidateCustomization(&interfaces.FactoryConfig{Name: "other"}); err != nil {
		t.Fatalf("non-classifier customization: %v", err)
	}
	if err := ValidateCustomization(&interfaces.FactoryConfig{Project: "builtin-classifier"}); err == nil {
		t.Fatal("classifier project without topology was accepted")
	}

	tests := []struct {
		name string
		edit func(*interfaces.FactoryConfig)
		want string
	}{
		{"missing classifier", func(cfg *interfaces.FactoryConfig) { cfg.Workstations = nil }, "requires classifier"},
		{"missing route", func(cfg *interfaces.FactoryConfig) {
			cfg.Workstations[0].ClassificationRoutes = cfg.Workstations[0].ClassificationRoutes[:2]
		}, "exactly"},
		{"unknown label", func(cfg *interfaces.FactoryConfig) { cfg.Workstations[0].ClassificationRoutes[0].Label = "unknown" }, "unsupported"},
		{"duplicate label", func(cfg *interfaces.FactoryConfig) { cfg.Workstations[0].ClassificationRoutes[1].Label = "small" }, "duplicated"},
		{"multiple outputs", func(cfg *interfaces.FactoryConfig) {
			cfg.Workstations[0].ClassificationRoutes[0].Outputs = append(cfg.Workstations[0].ClassificationRoutes[0].Outputs, routeOutput("small"))
		}, "exactly one target"},
		{"missing target", func(cfg *interfaces.FactoryConfig) {
			cfg.Workstations[0].ClassificationRoutes[0].Outputs[0] = routeOutput("missing")
		}, "exactly one target workstation"},
		{"unknown worker", func(cfg *interfaces.FactoryConfig) { cfg.Workstations[1].WorkerTypeName = "missing" }, "unknown worker"},
		{"missing selection", func(cfg *interfaces.FactoryConfig) { cfg.Workers[0].Model = ""; cfg.Workers[0].Preset = "" }, "without a model selection"},
		{"unsupported provider", func(cfg *interfaces.FactoryConfig) {
			cfg.Workers[0].Preset = ""
			cfg.Workers[0].ModelProvider = "unknown"
		}, "unsupported modelProvider"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := classifierConfigForValidation()
			test.edit(cfg)
			err := ValidateCustomization(cfg)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateCustomization() error = %v, want %q", err, test.want)
			}
		})
	}

	if err := ValidateCustomization(classifierConfigForValidation()); err != nil {
		t.Fatalf("valid customization: %v", err)
	}
}

func TestValidateResolvedCustomizationClassifierBranches(t *testing.T) {
	if err := ValidateResolvedCustomization(nil); err != nil {
		t.Fatalf("nil customization: %v", err)
	}
	if err := ValidateResolvedCustomization(&interfaces.FactoryConfig{Name: "other"}); err != nil {
		t.Fatalf("non-classifier customization: %v", err)
	}
	if err := ValidateResolvedCustomization(&interfaces.FactoryConfig{Name: "@you/classifier"}); err != nil {
		t.Fatalf("missing classifier workstation: %v", err)
	}
	for _, test := range []struct {
		name string
		edit func(*interfaces.FactoryConfig)
	}{
		{"multiple outputs ignored after authored validation", func(cfg *interfaces.FactoryConfig) {
			cfg.Workstations[0].ClassificationRoutes[0].Outputs = append(cfg.Workstations[0].ClassificationRoutes[0].Outputs, routeOutput("small"))
		}},
		{"missing target ignored after authored validation", func(cfg *interfaces.FactoryConfig) {
			cfg.Workstations[0].ClassificationRoutes[0].Outputs[0] = routeOutput("missing")
		}},
		{"missing target worker ignored after authored validation", func(cfg *interfaces.FactoryConfig) { cfg.Workstations[1].WorkerTypeName = "missing" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := classifierConfigForValidation()
			test.edit(cfg)
			if err := ValidateResolvedCustomization(cfg); err != nil {
				t.Fatalf("ValidateResolvedCustomization() = %v", err)
			}
		})
	}

	for _, test := range []struct {
		name string
		edit func(*interfaces.FactoryConfig)
		want string
	}{
		{"empty model", func(cfg *interfaces.FactoryConfig) { cfg.Workers[0].Model = "" }, "resolved model selection is empty"},
		{"unsupported provider", func(cfg *interfaces.FactoryConfig) { cfg.Workers[0].ModelProvider = "unknown" }, "unsupported resolved modelProvider"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := classifierConfigForValidation()
			test.edit(cfg)
			err := ValidateResolvedCustomization(cfg)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResolvedCustomization() error = %v, want %q", err, test.want)
			}
		})
	}

	if err := ValidateResolvedCustomization(classifierConfigForValidation()); err != nil {
		t.Fatalf("valid resolved customization: %v", err)
	}
}

func TestClassifierCustomizationHelpers(t *testing.T) {
	cfg := classifierConfigForValidation()
	if _, ok := classifierWorkerByName(cfg, "missing"); ok {
		t.Fatal("missing worker found")
	}
	if classifierRouteHasInput(cfg.Workstations[1], routeOutput("other")) {
		t.Fatal("unexpected route input match")
	}
	if supportedClassifierModelProvider("") || supportedClassifierModelProvider("DEFAULT") || supportedClassifierModelProvider("unknown") {
		t.Fatal("invalid provider accepted")
	}
	if !supportedClassifierModelProvider("CODEX") || !supportedClassifierModelProvider("codex") {
		t.Fatal("supported provider rejected")
	}
}

func classifierConfigForValidation() *interfaces.FactoryConfig {
	routes := make([]interfaces.ClassificationRouteConfig, 0, len(requiredClassifierLabels))
	workstations := []interfaces.FactoryWorkstationConfig{{Name: "classify-complexity"}}
	workers := make([]interfaces.WorkerConfig, 0, len(requiredClassifierLabels))
	for _, label := range []string{"small", "medium", "large"} {
		routes = append(routes, interfaces.ClassificationRouteConfig{Label: label, Outputs: []interfaces.IOConfig{routeOutput(label)}})
		workstations = append(workstations, interfaces.FactoryWorkstationConfig{Name: label + "-target", WorkerTypeName: label + "-worker", Inputs: []interfaces.IOConfig{routeOutput(label)}})
		workers = append(workers, interfaces.WorkerConfig{Name: label + "-worker", Type: interfaces.WorkerTypeModel, ModelProvider: "CODEX", Model: "gpt-5"})
	}
	workstations[0].ClassificationRoutes = routes
	return &interfaces.FactoryConfig{Name: "@you/classifier", Workstations: workstations, Workers: workers}
}

func routeOutput(state string) interfaces.IOConfig {
	return interfaces.IOConfig{WorkTypeName: "task", StateName: state}
}
