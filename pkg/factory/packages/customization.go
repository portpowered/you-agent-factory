package packages

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ValidateCustomization validates supported customer edits to a packaged factory.
func ValidateCustomization(cfg *interfaces.FactoryConfig) error {
	if cfg == nil || (strings.TrimSpace(cfg.Name) != "@you/classifier" && strings.TrimSpace(cfg.Project) != "builtin-classifier") {
		return nil
	}

	classifier, ok := classifierWorkstation(cfg)
	if !ok {
		return fmt.Errorf("@you/classifier customization requires classifier workstation %q", "classify-complexity")
	}
	if len(classifier.ClassificationRoutes) != len(requiredClassifierLabels) {
		return fmt.Errorf("@you/classifier customization requires exactly the small, medium, and large classification routes")
	}

	seen := make(map[string]struct{}, len(classifier.ClassificationRoutes))
	for routeIndex, route := range classifier.ClassificationRoutes {
		label := strings.TrimSpace(route.Label)
		if _, required := requiredClassifierLabels[label]; !required {
			return fmt.Errorf("@you/classifier classificationRoutes[%d].label %q is unsupported: expected small, medium, or large", routeIndex, route.Label)
		}
		if _, duplicate := seen[label]; duplicate {
			return fmt.Errorf("@you/classifier classificationRoutes[%d].label %q is duplicated", routeIndex, label)
		}
		seen[label] = struct{}{}
		if len(route.Outputs) != 1 {
			return fmt.Errorf("@you/classifier classificationRoutes[%d] for label %q must declare exactly one target", routeIndex, label)
		}
		if err := validateClassifierRouteTarget(cfg, routeIndex, label, route.Outputs[0]); err != nil {
			return err
		}
	}

	for label := range requiredClassifierLabels {
		if _, exists := seen[label]; !exists {
			return fmt.Errorf("@you/classifier customization is missing classification route label %q", label)
		}
	}
	return nil
}

var requiredClassifierLabels = map[string]struct{}{"small": {}, "medium": {}, "large": {}}

func classifierWorkstation(cfg *interfaces.FactoryConfig) (interfaces.FactoryWorkstationConfig, bool) {
	for _, workstation := range cfg.Workstations {
		if workstation.Name == "classify-complexity" {
			return workstation, true
		}
	}
	return interfaces.FactoryWorkstationConfig{}, false
}

func validateClassifierRouteTarget(cfg *interfaces.FactoryConfig, routeIndex int, label string, output interfaces.IOConfig) error {
	targets := make([]interfaces.FactoryWorkstationConfig, 0, 1)
	for _, workstation := range cfg.Workstations {
		if workstation.Name != "classify-complexity" && classifierRouteHasInput(workstation, output) {
			targets = append(targets, workstation)
		}
	}
	if len(targets) != 1 {
		return fmt.Errorf("@you/classifier classificationRoutes[%d] for label %q targets %q/%q, which must be consumed by exactly one target workstation", routeIndex, label, output.WorkTypeName, output.StateName)
	}
	worker, ok := classifierWorkerByName(cfg, targets[0].WorkerTypeName)
	if !ok {
		return fmt.Errorf("@you/classifier classificationRoutes[%d] for label %q targets workstation %q with unknown worker %q", routeIndex, label, targets[0].Name, targets[0].WorkerTypeName)
	}
	if strings.TrimSpace(worker.Model) == "" {
		return fmt.Errorf("@you/classifier classificationRoutes[%d] for label %q targets worker %q without a model selection", routeIndex, label, worker.Name)
	}
	if !supportedClassifierModelProvider(worker.ModelProvider) {
		return fmt.Errorf("@you/classifier classificationRoutes[%d] for label %q targets worker %q with unsupported modelProvider %q: %s", routeIndex, label, worker.Name, worker.ModelProvider, interfaces.AcceptedPublicWorkerModelProviderSummary())
	}
	return nil
}

func classifierRouteHasInput(workstation interfaces.FactoryWorkstationConfig, output interfaces.IOConfig) bool {
	for _, input := range workstation.Inputs {
		if input.WorkTypeName == output.WorkTypeName && input.StateName == output.StateName {
			return true
		}
	}
	return false
}

func classifierWorkerByName(cfg *interfaces.FactoryConfig, name string) (interfaces.WorkerConfig, bool) {
	for _, worker := range cfg.Workers {
		if worker.Name == name {
			return worker, true
		}
	}
	return interfaces.WorkerConfig{}, false
}

func supportedClassifierModelProvider(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || interfaces.IsSymbolicWorkerModelProviderDefault(trimmed) {
		return false
	}
	if _, ok := interfaces.CanonicalizeOperatorWorkerModelProviderInput(trimmed); ok {
		return true
	}
	for _, provider := range interfaces.SupportedModelProviders() {
		if string(provider) == trimmed {
			return true
		}
	}
	return false
}
