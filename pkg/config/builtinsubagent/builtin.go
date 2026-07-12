package builtinsubagent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed factory.json
var factoryJSON []byte

//go:embed prompts/worker.md
var workerPrompt string

//go:embed prompts/run-subagent.md
var runSubagentPrompt string

var workerPromptBodies = map[string]string{
	"subagent-worker": workerPrompt,
}

var workstationPromptBodies = map[string]string{
	"run-subagent": runSubagentPrompt,
}

// BuiltInSubagentFactoryJSON is the canonical runnable @you/subagent packaged
// factory payload assembled from authored factory.json and prompt files.
var BuiltInSubagentFactoryJSON = mustAssembleBuiltInSubagentFactoryJSON()

// FactoryJSON returns the authored factory scaffold without assembled prompt bodies.
func FactoryJSON() []byte {
	return append([]byte(nil), factoryJSON...)
}

func mustAssembleBuiltInSubagentFactoryJSON() []byte {
	payload, err := assembleBuiltInSubagentFactoryJSON()
	if err != nil {
		panic(fmt.Sprintf("assemble built-in @you/subagent factory json: %v", err))
	}
	return payload
}

func assembleBuiltInSubagentFactoryJSON() ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(factoryJSON, &root); err != nil {
		return nil, fmt.Errorf("unmarshal factory.json: %w", err)
	}
	return assembleBuiltInSubagentFactoryJSONFromRoot(root)
}

func assembleBuiltInSubagentFactoryJSONFromRoot(root map[string]any) ([]byte, error) {
	workers, ok := root["workers"].([]any)
	if !ok {
		return nil, fmt.Errorf("factory.json workers must be an array")
	}
	for _, entry := range workers {
		worker, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("factory.json worker entry must be an object")
		}
		name, _ := worker["name"].(string)
		promptBody, ok := workerPromptBodies[name]
		if !ok {
			continue
		}
		worker["body"] = strings.TrimSpace(promptBody)
		delete(worker, "promptFile")
	}

	workstations, ok := root["workstations"].([]any)
	if !ok {
		return nil, fmt.Errorf("factory.json workstations must be an array")
	}
	for _, entry := range workstations {
		workstation, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("factory.json workstation entry must be an object")
		}
		name, _ := workstation["name"].(string)
		promptBody, ok := workstationPromptBodies[name]
		if !ok {
			continue
		}
		workstation["body"] = strings.TrimSpace(promptBody)
		delete(workstation, "promptFile")
	}

	payload, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal assembled factory json: %w", err)
	}
	return payload, nil
}
