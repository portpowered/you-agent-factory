package config

import (
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type retiredBoundaryField struct {
	key         string
	replacement string
}

var retiredFactoryBoundaryFields = []retiredBoundaryField{
	{key: "project", replacement: "use id"},
	{key: "factoryDir", replacement: "use factoryDirectory"},
	{key: "factory_dir", replacement: "use factoryDirectory"},
	{key: "resourceManifest", replacement: "use supportingFiles"},
	{key: "resource_manifest", replacement: "use supportingFiles"},
	{key: "workflowId", replacement: "remove workflowId"},
	{key: "workflow_id", replacement: "remove workflowId"},
}

var retiredWorkerBoundaryFields = []retiredBoundaryField{
	{key: "provider", replacement: "use executorProvider"},
	{key: "model_provider", replacement: "use modelProvider"},
	{key: "sessionId", replacement: "remove sessionId; provider sessions are runtime-owned"},
	{key: "session_id", replacement: "remove sessionId; provider sessions are runtime-owned"},
	{key: "concurrency", replacement: "remove concurrency; use resources to limit concurrent work"},
}

var retiredWorkstationBoundaryFields = []retiredBoundaryField{
	{key: "kind", replacement: "use behavior"},
	{key: "runtimeType", replacement: "use type"},
	{key: "runtime_type", replacement: "use type"},
	{key: "resourceUsage", replacement: "use resources"},
	{key: "resource_usage", replacement: "use resources"},
	{key: "resource-usage", replacement: "use resources"},
	{key: "stopToken", replacement: "use stopWords"},
	{key: "stop_token", replacement: "use stopWords"},
	{key: "runtimeStopWords", replacement: "use stopWords"},
	{key: "runtime_stop_words", replacement: "use stopWords"},
	{key: "timeout", replacement: "use limits.maxExecutionTime"},
}

func rejectRetiredFanInField(data []byte) error {
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

func rejectRetiredExhaustionRulesField(data []byte) error {
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

func rejectRetiredCronIntervalField(data []byte) error {
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

func rejectRetiredGeneratedBoundaryAliases(data []byte) error {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	if err := rejectRetiredBoundaryFields(payload, "factory", retiredFactoryBoundaryFields); err != nil {
		return err
	}
	if err := rejectRetiredWorkerBoundaryAliases(payload); err != nil {
		return err
	}
	if err := rejectRetiredWorkstationBoundaryAliases(payload); err != nil {
		return err
	}
	return nil
}

func rejectRetiredWorkerBoundaryAliases(root map[string]any) error {
	workers, ok := root["workers"].([]any)
	if !ok {
		return nil
	}
	for index, item := range workers {
		worker, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if err := rejectRetiredWorkerBoundaryObject(worker, fmt.Sprintf("workers[%d]", index), true); err != nil {
			return err
		}
	}
	return nil
}

func rejectRetiredWorkerBoundaryObject(worker map[string]any, path string, includeDefinition bool) error {
	if err := rejectRetiredBoundaryFields(worker, path, retiredWorkerBoundaryFields); err != nil {
		return err
	}
	if !includeDefinition {
		return nil
	}
	definition, ok := worker["definition"].(map[string]any)
	if !ok {
		return nil
	}
	return rejectRetiredWorkerBoundaryObject(definition, path+".definition", false)
}

func rejectRetiredWorkstationBoundaryAliases(root map[string]any) error {
	workstations, ok := root["workstations"].([]any)
	if !ok {
		return nil
	}
	for index, item := range workstations {
		workstation, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if err := rejectRetiredWorkstationBoundaryObject(workstation, fmt.Sprintf("workstations[%d]", index), true); err != nil {
			return err
		}
	}
	return nil
}

func rejectRetiredWorkstationBoundaryObject(workstation map[string]any, path string, includeDefinition bool) error {
	if err := rejectRetiredBoundaryFields(workstation, path, retiredWorkstationBoundaryFields); err != nil {
		return err
	}
	if err := rejectRetiredCronBoundaryAliases(workstation["cron"], path+".cron"); err != nil {
		return err
	}
	if !includeDefinition {
		return nil
	}
	definition, ok := workstation["definition"].(map[string]any)
	if !ok {
		return nil
	}
	return rejectRetiredWorkstationBoundaryObject(definition, path+".definition", false)
}

func rejectRetiredCronBoundaryAliases(raw any, path string) error {
	cron, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return rejectRetiredBoundaryFields(cron, path, []retiredBoundaryField{
		{key: "trigger_at_start", replacement: "use triggerAtStart"},
		{key: "expiry_window", replacement: "use expiryWindow"},
	})
}

func rejectRetiredBoundaryFields(container map[string]any, path string, fields []retiredBoundaryField) error {
	for _, field := range fields {
		if _, ok := container[field.key]; ok {
			return fmt.Errorf("%s.%s is not supported; %s", path, field.key, field.replacement)
		}
	}
	return nil
}
