package retiredboundary

import (
	"encoding/json"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type Field struct {
	Key         string
	Replacement string
}

var factoryFields = []Field{
	{Key: "project", Replacement: "use id"},
	{Key: "factoryDir", Replacement: "use factoryDirectory"},
	{Key: "factory_dir", Replacement: "use factoryDirectory"},
	{Key: "resourceManifest", Replacement: "use supportingFiles"},
	{Key: "resource_manifest", Replacement: "use supportingFiles"},
	{Key: "workflowId", Replacement: "remove workflowId"},
	{Key: "workflow_id", Replacement: "remove workflowId"},
}

var workerFields = []Field{
	{Key: "model_provider", Replacement: "use modelProvider"},
	{Key: "sessionId", Replacement: "remove sessionId; provider sessions are runtime-owned"},
	{Key: "session_id", Replacement: "remove sessionId; provider sessions are runtime-owned"},
	{Key: "concurrency", Replacement: "remove concurrency; use resources to limit concurrent work"},
}

var workstationFields = []Field{
	{Key: "kind", Replacement: "use behavior"},
	{Key: "runtimeType", Replacement: "use type"},
	{Key: "runtime_type", Replacement: "use type"},
	{Key: "resourceUsage", Replacement: "use resources"},
	{Key: "resource_usage", Replacement: "use resources"},
	{Key: "resource-usage", Replacement: "use resources"},
	{Key: "stopToken", Replacement: "use stopWords"},
	{Key: "stop_token", Replacement: "use stopWords"},
	{Key: "runtimeStopWords", Replacement: "use stopWords"},
	{Key: "runtime_stop_words", Replacement: "use stopWords"},
	{Key: "timeout", Replacement: "use limits.maxExecutionTime"},
}

func RejectFanInField(data []byte) error {
	var payload struct {
		Workstations []map[string]json.RawMessage `json:"workstations"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	for index, workstation := range payload.Workstations {
		if _, ok := workstation["join"]; ok {
			return fmt.Errorf("workstations[%d].join is not supported; use per-input guards", index)
		}
	}
	return nil
}

func RejectExhaustionRulesField(data []byte) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	if _, ok := payload["exhaustionRules"]; ok {
		return fmt.Errorf("exhaustion_rules is retired; use a guarded LOGICAL_MOVE workstation with a visit_count guard instead")
	}
	if _, ok := payload["exhaustion_rules"]; ok {
		return fmt.Errorf("exhaustion_rules is retired; use a guarded LOGICAL_MOVE workstation with a visit_count guard instead")
	}
	return nil
}

func RejectCronIntervalField(data []byte) error {
	var payload struct {
		Workstations []struct {
			Cron *interfaces.CronConfig `json:"cron"`
		} `json:"workstations"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	for index, workstation := range payload.Workstations {
		if workstation.Cron != nil && workstation.Cron.HasUnsupportedInterval() {
			return fmt.Errorf("workstations[%d].cron.interval is not supported; use cron.schedule", index)
		}
	}
	return nil
}

func RejectGeneratedBoundaryAliases(data []byte) error {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
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
	return RejectFields(cron, path, []Field{
		{Key: "trigger_at_start", Replacement: "use triggerAtStart"},
		{Key: "expiry_window", Replacement: "use expiryWindow"},
	})
}

func RejectFields(container map[string]any, path string, fields []Field) error {
	for _, field := range fields {
		if _, ok := container[field.Key]; ok {
			return fmt.Errorf("%s.%s is not supported; %s", path, field.Key, field.Replacement)
		}
	}
	return nil
}
