package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"gopkg.in/yaml.v3"
)

// WorkstationLoader loads workstation definitions by name.
// Implement this interface in tests to inject workstation configs without
// requiring AGENTS.md files on disk. Returning (nil, nil) signals no config is
// available and the caller should use its normal fallback behavior.
type WorkstationLoader interface {
	Load(name string) (*interfaces.FactoryWorkstationConfig, error)
}

// LoadWorkerConfig loads a worker configuration from the given directory.
// It reads AGENTS.md, parses YAML frontmatter into WorkerConfig, and sets
// Body to the remaining markdown content.
func LoadWorkerConfig(dir string) (*interfaces.WorkerConfig, error) {
	agentsPath := filepath.Join(dir, interfaces.FactoryAgentsFileName)
	frontmatter, body, err := parseAgentsMD(agentsPath)
	if err != nil {
		return nil, fmt.Errorf("load worker config from %s: %w", dir, err)
	}

	rawFrontmatter, err := parseAgentsFrontmatterMap(frontmatter)
	if err != nil {
		return nil, fmt.Errorf("parse worker frontmatter in %s: %w", agentsPath, err)
	}
	if err := rejectRetiredWorkerFrontmatterAliases(rawFrontmatter); err != nil {
		return nil, fmt.Errorf("reject retired worker frontmatter fields in %s: %w", agentsPath, err)
	}
	normalizeAgentsRuntimeResources(rawFrontmatter)
	frontmatter, err = yaml.Marshal(rawFrontmatter)
	if err != nil {
		return nil, fmt.Errorf("normalize worker frontmatter in %s: %w", agentsPath, err)
	}

	var parsed workerFrontmatterInput
	if err := yaml.Unmarshal(frontmatter, &parsed); err != nil {
		return nil, fmt.Errorf("parse worker frontmatter in %s: %w", agentsPath, err)
	}

	cfg := interfaces.WorkerConfig{
		Type:             parsed.Type,
		Model:            parsed.Model,
		ModelProvider:    parsed.ModelProvider,
		ExecutorProvider: parsed.ExecutorProvider,
		Command:          parsed.Command,
		Args:             append([]string(nil), parsed.Args...),
		Resources:        append([]interfaces.ResourceConfig(nil), parsed.Resources...),
		Timeout:          parsed.Timeout,
		StopToken:        parsed.StopToken,
		SkipPermissions:  parsed.SkipPermissions,
		Body:             body,
	}
	if cfg.ModelProvider != "" {
		modelProvider := factoryapi.WorkerModelProvider(cfg.ModelProvider)
		cfg.ModelProvider = internalFactoryWorkerModelProviderFromPublic(&modelProvider)
	}
	if cfg.ExecutorProvider != "" {
		provider := factoryapi.WorkerProvider(cfg.ExecutorProvider)
		cfg.ExecutorProvider = internalFactoryWorkerProviderFromPublic(&provider)
	}
	return &cfg, nil
}

type workerFrontmatterInput struct {
	Type             string                      `yaml:"type"`
	Model            string                      `yaml:"model,omitempty"`
	ModelProvider    string                      `yaml:"modelProvider,omitempty"`
	ExecutorProvider string                      `yaml:"executorProvider,omitempty"`
	Command          string                      `yaml:"command,omitempty"`
	Args             []string                    `yaml:"args,omitempty"`
	Resources        []interfaces.ResourceConfig `yaml:"resources,omitempty"`
	Timeout          string                      `yaml:"timeout,omitempty"`
	StopToken        string                      `yaml:"stopToken,omitempty"`
	SkipPermissions  bool                        `yaml:"skipPermissions,omitempty"`
}

// LoadWorkstationConfig loads a workstation configuration from the given directory.
// It reads AGENTS.md, parses YAML frontmatter into interfaces.FactoryWorkstationConfig, sets Body to the
// remaining markdown content, and loads PromptFile if specified.
func LoadWorkstationConfig(dir string) (*interfaces.FactoryWorkstationConfig, error) {
	agentsPath := filepath.Join(dir, interfaces.FactoryAgentsFileName)
	frontmatter, body, err := parseAgentsMD(agentsPath)
	if err != nil {
		return nil, fmt.Errorf("load workstation config from %s: %w", dir, err)
	}

	rawFrontmatter, err := parseAgentsFrontmatterMap(frontmatter)
	if err != nil {
		return nil, fmt.Errorf("parse workstation frontmatter in %s: %w", agentsPath, err)
	}
	if err := rejectRetiredWorkstationFrontmatterAliases(rawFrontmatter); err != nil {
		return nil, fmt.Errorf("reject retired workstation frontmatter fields in %s: %w", agentsPath, err)
	}
	normalizeAgentsRuntimeResources(rawFrontmatter)
	frontmatter, err = yaml.Marshal(rawFrontmatter)
	if err != nil {
		return nil, fmt.Errorf("normalize workstation frontmatter in %s: %w", agentsPath, err)
	}

	var cfg interfaces.FactoryWorkstationConfig
	if err := yaml.Unmarshal(frontmatter, &cfg); err != nil {
		return nil, fmt.Errorf("parse workstation frontmatter in %s: %w", agentsPath, err)
	}
	normalizeWorkstationPublicEnums(&cfg)
	NormalizeWorkstationExecutionLimit(&cfg)

	cfg.Body = body

	if cfg.PromptFile != "" {
		cfg.PromptTemplate, err = loadWorkstationPromptTemplate(dir, cfg.PromptFile)
		if err != nil {
			return nil, err
		}
	} else {
		cfg.PromptTemplate = body
	}

	return &cfg, nil
}

func loadWorkstationPromptTemplate(dir, promptFile string) (string, error) {
	promptPath := filepath.Join(dir, promptFile)
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("load prompt file %s: %w", promptPath, err)
	}
	return string(data), nil
}

func loadAgentsBody(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	content := string(data)
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		_, body, err := parseAgentsMD(path)
		if err != nil {
			return "", err
		}
		return body, nil
	}

	return strings.TrimSpace(content), nil
}

func normalizeWorkstationPublicEnums(cfg *interfaces.FactoryWorkstationConfig) {
	if cfg == nil {
		return
	}
	if cfg.Kind != "" {
		behavior := factoryapi.WorkstationKind(cfg.Kind)
		cfg.Kind = internalFactoryWorkstationKindFromPublic(&behavior)
	}
	for i := range cfg.Guards {
		cfg.Guards[i].Type = internalFactoryGuardTypeFromPublic(factoryapi.GuardType(cfg.Guards[i].Type))
	}
	for i := range cfg.Inputs {
		if cfg.Inputs[i].Guard == nil {
			continue
		}
		cfg.Inputs[i].Guard.Type = internalFactoryGuardTypeFromPublic(factoryapi.GuardType(cfg.Inputs[i].Guard.Type))
	}
}

// parseAgentsMD reads an AGENTS.md file and splits it into YAML frontmatter
// and markdown body. Frontmatter is delimited by --- on its own lines.
func parseAgentsMD(path string) (frontmatter []byte, body string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	content := string(data)

	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return nil, "", fmt.Errorf("AGENTS.md missing frontmatter delimiter at %s", path)
	}

	rest := content[4:]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		idx = strings.Index(rest, "\r\n---\r\n")
		if idx < 0 {
			if strings.HasSuffix(strings.TrimSpace(rest), "---") {
				trimmed := strings.TrimSpace(rest)
				fm := trimmed[:len(trimmed)-3]
				return []byte(fm), "", nil
			}
			return nil, "", fmt.Errorf("AGENTS.md missing closing frontmatter delimiter at %s", path)
		}
		frontmatter = []byte(rest[:idx])
		body = strings.TrimSpace(rest[idx+len("\r\n---\r\n"):])
	} else {
		frontmatter = []byte(rest[:idx])
		body = strings.TrimSpace(rest[idx+len("\n---\n"):])
	}

	return frontmatter, body, nil
}

func parseAgentsFrontmatterMap(frontmatter []byte) (map[string]any, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(frontmatter, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		raw = make(map[string]any)
	}
	return raw, nil
}

func rejectRetiredWorkerFrontmatterAliases(frontmatter map[string]any) error {
	return rejectRetiredBoundaryFields(frontmatter, "frontmatter", []retiredBoundaryField{
		{key: "provider", replacement: "use executorProvider"},
		{key: "model_provider", replacement: "use modelProvider"},
		{key: "sessionId", replacement: "remove sessionId; provider sessions are runtime-owned"},
		{key: "session_id", replacement: "remove sessionId; provider sessions are runtime-owned"},
		{key: "concurrency", replacement: "remove concurrency; use resources to limit concurrent work"},
		{key: "stop_token", replacement: "use stopToken"},
		{key: "skip_permissions", replacement: "use skipPermissions"},
	})
}

func rejectRetiredWorkstationFrontmatterAliases(frontmatter map[string]any) error {
	if err := rejectRetiredBoundaryFields(frontmatter, "frontmatter", []retiredBoundaryField{
		{key: "kind", replacement: "use behavior"},
		{key: "runtimeType", replacement: "use type"},
		{key: "runtime_type", replacement: "use type"},
		{key: "prompt_file", replacement: "use promptFile"},
		{key: "output_schema", replacement: "use outputSchema"},
		{key: "on_continue", replacement: "use onContinue"},
		{key: "on_rejection", replacement: "use onRejection"},
		{key: "on_failure", replacement: "use onFailure"},
		{key: "resourceUsage", replacement: "use resources"},
		{key: "resource_usage", replacement: "use resources"},
		{key: "stopToken", replacement: "use stopWords"},
		{key: "stop_token", replacement: "use stopWords"},
		{key: "stop_words", replacement: "use stopWords"},
		{key: "runtimeStopWords", replacement: "use stopWords"},
		{key: "runtime_stop_words", replacement: "use stopWords"},
		{key: "timeout", replacement: "use limits.maxExecutionTime"},
		{key: "working_directory", replacement: "use workingDirectory"},
	}); err != nil {
		return err
	}
	if err := rejectRetiredBoundaryFields(frontmatterMap(frontmatter["limits"]), "frontmatter.limits", []retiredBoundaryField{
		{key: "max_retries", replacement: "use maxRetries"},
		{key: "max_execution_time", replacement: "use maxExecutionTime"},
	}); err != nil {
		return err
	}
	if err := rejectRetiredCronBoundaryAliases(frontmatter["cron"], "frontmatter.cron"); err != nil {
		return err
	}
	if err := rejectRetiredIOFrontmatterListAliases(frontmatter["inputs"], "frontmatter.inputs"); err != nil {
		return err
	}
	if err := rejectRetiredIOFrontmatterListAliases(frontmatter["outputs"], "frontmatter.outputs"); err != nil {
		return err
	}
	if err := rejectRetiredIOFrontmatterListAliases(frontmatter["onContinue"], "frontmatter.onContinue"); err != nil {
		return err
	}
	if err := rejectRetiredIOFrontmatterListAliases(frontmatter["onRejection"], "frontmatter.onRejection"); err != nil {
		return err
	}
	if err := rejectRetiredIOFrontmatterListAliases(frontmatter["onFailure"], "frontmatter.onFailure"); err != nil {
		return err
	}
	return rejectRetiredGuardFrontmatterAliases(frontmatter["guards"], "frontmatter.guards")
}

func rejectRetiredIOFrontmatterListAliases(raw any, path string) error {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	for index, item := range items {
		if err := rejectRetiredIOFrontmatterAliases(item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func rejectRetiredIOFrontmatterAliases(raw any, path string) error {
	entry := frontmatterMap(raw)
	if err := rejectRetiredBoundaryFields(entry, path, []retiredBoundaryField{
		{key: "work_type", replacement: "use workType"},
	}); err != nil {
		return err
	}
	return rejectRetiredInputGuardFrontmatterAliases(entry["guard"], path+".guard")
}

func rejectRetiredInputGuardFrontmatterAliases(raw any, path string) error {
	return rejectRetiredBoundaryFields(frontmatterMap(raw), path, []retiredBoundaryField{
		{key: "match_input", replacement: "use matchInput"},
		{key: "parent_input", replacement: "use parentInput"},
		{key: "spawned_by", replacement: "use spawnedBy"},
	})
}

func rejectRetiredGuardFrontmatterAliases(raw any, path string) error {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	for index, item := range items {
		entryPath := fmt.Sprintf("%s[%d]", path, index)
		entry := frontmatterMap(item)
		if err := rejectRetiredBoundaryFields(entry, entryPath, []retiredBoundaryField{
			{key: "max_visits", replacement: "use maxVisits"},
		}); err != nil {
			return err
		}
		if err := rejectRetiredBoundaryFields(frontmatterMap(entry["matchConfig"]), entryPath+".matchConfig", []retiredBoundaryField{
			{key: "input_key", replacement: "use inputKey"},
		}); err != nil {
			return err
		}
	}
	return nil
}

func frontmatterMap(raw any) map[string]any {
	typed, _ := raw.(map[string]any)
	return typed
}

func normalizeAgentsRuntimeResources(container map[string]any) {
	resources, ok := container["resources"]
	if !ok {
		return
	}
	container["resources"] = runtimeResourceRequirementsFromBoundaryValue(resources)
}
