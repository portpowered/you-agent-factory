package retiredboundary

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

var factoryFields = RetiredFactoryFieldAliases()
var workerFields = RetiredWorkerFieldAliases()
var workstationFields = RetiredWorkstationFieldAliases()

func RejectFanInField(payload map[string]any) error {
	workstations, _ := decodedObjectField(payload, "workstations").([]any)
	for index, value := range workstations {
		workstation, _ := value.(map[string]any)
		if _, ok := workstation["join"]; ok {
			return fmt.Errorf("workstations[%d].join is not supported; use per-input guards", index)
		}
	}
	return nil
}

func RejectExhaustionRulesField(payload map[string]any) error {
	if _, ok := payload["exhaustionRules"]; ok {
		return fmt.Errorf("exhaustion_rules is retired; use a guarded LOGICAL_MOVE workstation with a visit_count guard instead")
	}
	if _, ok := payload["exhaustion_rules"]; ok {
		return fmt.Errorf("exhaustion_rules is retired; use a guarded LOGICAL_MOVE workstation with a visit_count guard instead")
	}
	return nil
}

func RejectCronIntervalField(payload map[string]any) error {
	workstations, _ := decodedObjectField(payload, "workstations").([]any)
	for index, value := range workstations {
		workstation, _ := value.(map[string]any)
		cron, _ := decodedObjectField(workstation, "cron").(map[string]any)
		if _, supplied := cron["interval"]; supplied {
			return fmt.Errorf("workstations[%d].cron.interval is not supported; use cron.schedule", index)
		}
	}
	return nil
}

// The generated-model decoder accepts case-insensitive field names. After
// normalization marshals sorted keys, the last matching spelling wins.
func decodedObjectField(object map[string]any, field string) any {
	var selected string
	for key := range object {
		if strings.EqualFold(key, field) && key > selected {
			selected = key
		}
	}
	if selected == "" {
		return nil
	}
	return object[selected]
}

func RejectGeneratedBoundaryAliases(payload map[string]any) error {
	if err := RejectFields(payload, "factory", factoryFields); err != nil {
		return err
	}
	if err := rejectWorkerBoundaryAliases(payload); err != nil {
		return err
	}
	if err := rejectWorkstationBoundaryAliases(payload); err != nil {
		return err
	}
	return nil
}

func rejectWorkerBoundaryAliases(root map[string]any) error {
	workers, ok := root["workers"].([]any)
	if !ok {
		return nil
	}
	for index, item := range workers {
		worker, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if err := rejectWorkerBoundaryObject(worker, fmt.Sprintf("workers[%d]", index), true); err != nil {
			return err
		}
	}
	return nil
}

func rejectWorkerBoundaryObject(worker map[string]any, path string, includeDefinition bool) error {
	if err := RejectHostedProviderAlias(worker, path); err != nil {
		return err
	}
	if err := RejectFields(worker, path, workerFields); err != nil {
		return err
	}
	if !includeDefinition {
		return nil
	}
	definition, ok := worker["definition"].(map[string]any)
	if !ok {
		return nil
	}
	return rejectWorkerBoundaryObject(definition, path+".definition", false)
}

func RejectHostedProviderAlias(worker map[string]any, path string) error {
	rawProvider, hasProvider := worker["provider"]
	if !hasProvider {
		return nil
	}
	provider, _ := rawProvider.(string)
	workerType, _ := worker["type"].(string)
	if interfaces.IsPollerWorkerPublicType(interfaces.StrictPublicFactoryWorkerType(workerType)) &&
		interfaces.StrictPublicFactoryHostedWorkerProvider(provider) != "" {
		return nil
	}
	return fmt.Errorf("%s.provider is not supported; use executorProvider", path)
}

func rejectWorkstationBoundaryAliases(root map[string]any) error {
	workstations, ok := root["workstations"].([]any)
	if !ok {
		return nil
	}
	for index, item := range workstations {
		workstation, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if err := rejectWorkstationBoundaryObject(workstation, fmt.Sprintf("workstations[%d]", index), true); err != nil {
			return err
		}
	}
	return nil
}

func rejectWorkstationBoundaryObject(workstation map[string]any, path string, includeDefinition bool) error {
	if err := RejectFields(workstation, path, workstationFields); err != nil {
		return err
	}
	if err := RejectCronBoundaryAliases(workstation["cron"], path+".cron"); err != nil {
		return err
	}
	if !includeDefinition {
		return nil
	}
	definition, ok := workstation["definition"].(map[string]any)
	if !ok {
		return nil
	}
	return rejectWorkstationBoundaryObject(definition, path+".definition", false)
}

func RejectCronBoundaryAliases(raw any, path string) error {
	cron, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return RejectFields(cron, path, RetiredCronFieldAliases())
}

func RejectFields(container map[string]any, path string, fields []Field) error {
	for _, field := range fields {
		if _, ok := container[field.Key]; ok {
			return fmt.Errorf("%s.%s is not supported; %s", path, field.Key, field.Replacement)
		}
	}
	return nil
}
