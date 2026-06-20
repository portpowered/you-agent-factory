package builtingoal

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// SummarizerPromptTargetPath is the materialized path for the authored summarizer prompt.
	SummarizerPromptTargetPath = "factory/docs/summarizer.md"
)

//go:embed factory.json
var factoryJSON []byte

//go:embed prompts/planner.md
var plannerPrompt string

//go:embed prompts/executor.md
var executorPrompt string

//go:embed prompts/checker.md
var checkerPrompt string

//go:embed prompts/reviewer.md
var reviewerPrompt string

//go:embed prompts/summarizer.md
var summarizerPrompt string

var workstationPromptBodies = map[string]string{
	"plan-goal":    plannerPrompt,
	"execute-goal": executorPrompt,
	"check-goal":   checkerPrompt,
	"review-goal":  reviewerPrompt,
}

// AuthoredRolePrompts maps each goal role to its authored prompt source file content.
var AuthoredRolePrompts = map[string]string{
	"planner":    plannerPrompt,
	"executor":   executorPrompt,
	"checker":    checkerPrompt,
	"reviewer":   reviewerPrompt,
	"summarizer": summarizerPrompt,
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

	if err := injectSummarizerBundledPrompt(root); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal assembled factory json: %w", err)
	}
	return payload, nil
}

func injectSummarizerBundledPrompt(root map[string]any) error {
	supportingFiles, ok := root["supportingFiles"].(map[string]any)
	if !ok {
		return fmt.Errorf("factory.json supportingFiles must be an object")
	}
	bundledFiles, ok := supportingFiles["bundledFiles"].([]any)
	if !ok || len(bundledFiles) != 1 {
		return fmt.Errorf("factory.json must declare exactly one summarizer bundled file")
	}
	bundledFile, ok := bundledFiles[0].(map[string]any)
	if !ok {
		return fmt.Errorf("summarizer bundled file entry must be an object")
	}
	targetPath, _ := bundledFile["targetPath"].(string)
	if targetPath != SummarizerPromptTargetPath {
		return fmt.Errorf("summarizer bundled file targetPath = %q, want %q", targetPath, SummarizerPromptTargetPath)
	}
	content, ok := bundledFile["content"].(map[string]any)
	if !ok {
		return fmt.Errorf("summarizer bundled file content must be an object")
	}
	content["inline"] = strings.TrimSpace(summarizerPrompt)
	return nil
}
