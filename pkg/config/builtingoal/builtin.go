package builtingoal

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed factory.json
var factoryJSON []byte

//go:embed prompts/executor.md
var executorPrompt string

var workstationPromptBodies = map[string]string{
	"execute-goal": executorPrompt,
}

// AuthoredRolePrompts maps each goal role to its authored prompt source file content.
var AuthoredRolePrompts = map[string]string{
	"executor": executorPrompt,
}

// BuiltInGoalFactoryJSON is the canonical runnable @you/goal packaged factory payload
// assembled from authored factory.json and role prompt files.
var BuiltInGoalFactoryJSON = mustAssembleBuiltInGoalFactoryJSON()

// AuthoredRolePrompt returns the authored prompt content for a goal role.
func AuthoredRolePrompt(role string) (string, bool) {
	prompt, ok := AuthoredRolePrompts[role]
	return strings.TrimSpace(prompt), ok
}

// FactoryJSON returns the authored factory scaffold without assembled prompt bodies.
func FactoryJSON() []byte {
	return append([]byte(nil), factoryJSON...)
}

func mustAssembleBuiltInGoalFactoryJSON() []byte {
	payload, err := assembleBuiltInGoalFactoryJSON()
	if err != nil {
		panic(fmt.Sprintf("assemble built-in @you/goal factory json: %v", err))
	}
	return payload
}

func assembleBuiltInGoalFactoryJSON() ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(factoryJSON, &root); err != nil {
		return nil, fmt.Errorf("unmarshal factory.json: %w", err)
	}
	return assembleBuiltInGoalFactoryJSONFromRoot(root)
}

func assembleBuiltInGoalFactoryJSONFromRoot(root map[string]any) ([]byte, error) {
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
		if name == "goal-executor" {
			worker["body"] = strings.TrimSpace(executorPrompt)
		}
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
	}

	payload, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal assembled factory json: %w", err)
	}
	return payload, nil
}
