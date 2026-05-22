package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"gopkg.in/yaml.v3"
)

func safeFactoryLayoutSegment(kind, name string) (string, error) {
	segment := strings.TrimSpace(name)
	if segment == "" {
		return "", fmt.Errorf("%s name is required for factory config layout", kind)
	}
	if filepath.IsAbs(segment) || filepath.VolumeName(segment) != "" || strings.ContainsAny(segment, `/\`) {
		return "", fmt.Errorf("%s name %q cannot contain path separators", kind, name)
	}
	if segment == "." || segment == ".." {
		return "", fmt.Errorf("%s name %q is not a valid directory name", kind, name)
	}
	return segment, nil
}

func safePromptFilePath(workstationDir, promptFile string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(promptFile))
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("prompt file path is required")
	}
	if filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return "", fmt.Errorf("prompt file %q must be relative to the workstation directory", promptFile)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("prompt file %q cannot escape the workstation directory", promptFile)
	}
	return filepath.Join(workstationDir, cleaned), nil
}

func splitRuntimeEntityDirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

type workerFrontmatter struct {
	Type             string                               `yaml:"type"`
	Provider         string                               `yaml:"provider,omitempty"`
	Model            string                               `yaml:"model,omitempty"`
	ModelProvider    string                               `yaml:"modelProvider,omitempty"`
	ExecutorProvider string                               `yaml:"executorProvider,omitempty"`
	Command          string                               `yaml:"command,omitempty"`
	Args             []string                             `yaml:"args,omitempty"`
	Resources        []interfaces.ResourceConfig          `yaml:"resources,omitempty"`
	Timeout          string                               `yaml:"timeout,omitempty"`
	StopToken        string                               `yaml:"stopToken,omitempty"`
	SkipPermissions  bool                                 `yaml:"skipPermissions,omitempty"`
	Auth             *interfaces.HostedWorkerAuthConfig   `yaml:"auth,omitempty"`
	Linear           *interfaces.HostedLinearWorkerConfig `yaml:"linear,omitempty"`
}

type workstationFrontmatter struct {
	ID               string                       `yaml:"id,omitempty"`
	Name             string                       `yaml:"name,omitempty"`
	Kind             interfaces.WorkstationKind   `yaml:"behavior,omitempty"`
	Type             string                       `yaml:"type,omitempty"`
	Worker           string                       `yaml:"worker,omitempty"`
	PromptFile       string                       `yaml:"promptFile,omitempty"`
	OutputSchema     string                       `yaml:"outputSchema,omitempty"`
	Limits           workstationLimitsFrontmatter `yaml:"limits,omitempty"`
	Cron             *cronFrontmatter             `yaml:"cron,omitempty"`
	Inputs           []ioFrontmatter              `yaml:"inputs,omitempty"`
	Outputs          []ioFrontmatter              `yaml:"outputs,omitempty"`
	OnContinue       []ioFrontmatter              `yaml:"onContinue,omitempty"`
	OnRejection      []ioFrontmatter              `yaml:"onRejection,omitempty"`
	OnFailure        []ioFrontmatter              `yaml:"onFailure,omitempty"`
	Resources        []interfaces.ResourceConfig  `yaml:"resources,omitempty"`
	Guards           []guardFrontmatter           `yaml:"guards,omitempty"`
	StopWords        []string                     `yaml:"stopWords,omitempty"`
	WorkingDirectory string                       `yaml:"workingDirectory,omitempty"`
	Worktree         string                       `yaml:"worktree,omitempty"`
	Env              map[string]string            `yaml:"env,omitempty"`
}

type workstationLimitsFrontmatter struct {
	MaxRetries       int    `yaml:"maxRetries,omitempty"`
	MaxExecutionTime string `yaml:"maxExecutionTime,omitempty"`
}

type cronFrontmatter struct {
	Schedule       string `yaml:"schedule,omitempty"`
	TriggerAtStart bool   `yaml:"triggerAtStart,omitempty"`
	Jitter         string `yaml:"jitter,omitempty"`
	ExpiryWindow   string `yaml:"expiryWindow,omitempty"`
}

type ioFrontmatter struct {
	WorkType string                 `yaml:"workType"`
	State    string                 `yaml:"state"`
	Guard    *inputGuardFrontmatter `yaml:"guard,omitempty"`
}

type inputGuardFrontmatter struct {
	Type        interfaces.GuardType `yaml:"type"`
	MatchInput  string               `yaml:"matchInput,omitempty"`
	ParentInput string               `yaml:"parentInput,omitempty"`
	SpawnedBy   string               `yaml:"spawnedBy,omitempty"`
}

type guardFrontmatter struct {
	Type        interfaces.GuardType         `yaml:"type"`
	Workstation string                       `yaml:"workstation,omitempty"`
	MaxVisits   int                          `yaml:"maxVisits,omitempty"`
	MatchConfig *interfaces.GuardMatchConfig `yaml:"matchConfig,omitempty"`
}

func workstationFrontmatterForExpansion(def interfaces.FactoryWorkstationConfig) workstationFrontmatter {
	behavior := def.Kind
	if def.Kind != "" {
		behavior = interfaces.WorkstationKind(publicFactoryWorkstationKindFromInternal(def.Kind))
	}
	rendered := workstationFrontmatter{
		ID:               def.ID,
		Name:             def.Name,
		Kind:             behavior,
		Type:             def.Type,
		Worker:           def.WorkerTypeName,
		PromptFile:       def.PromptFile,
		OutputSchema:     def.OutputSchema,
		Limits:           workstationLimitsFrontmatter{MaxRetries: def.Limits.MaxRetries, MaxExecutionTime: def.Limits.MaxExecutionTime},
		Inputs:           ioFrontmatterSlice(def.Inputs),
		Outputs:          ioFrontmatterSlice(def.Outputs),
		OnContinue:       ioFrontmatterSlice(def.OnContinue),
		OnRejection:      ioFrontmatterSlice(def.OnRejection),
		OnFailure:        ioFrontmatterSlice(def.OnFailure),
		Resources:        append([]interfaces.ResourceConfig(nil), def.Resources...),
		Guards:           guardFrontmatterSlice(def.Guards),
		StopWords:        append([]string(nil), def.StopWords...),
		WorkingDirectory: def.WorkingDirectory,
		Worktree:         def.Worktree,
		Env:              cloneStringMap(def.Env),
	}
	if def.Cron != nil {
		rendered.Cron = &cronFrontmatter{
			Schedule:       def.Cron.Schedule,
			TriggerAtStart: def.Cron.TriggerAtStart,
			Jitter:         def.Cron.Jitter,
			ExpiryWindow:   def.Cron.ExpiryWindow,
		}
	}
	return rendered
}

func ioFrontmatterSlice(configs []interfaces.IOConfig) []ioFrontmatter {
	if len(configs) == 0 {
		return nil
	}
	out := make([]ioFrontmatter, len(configs))
	for i := range configs {
		out[i] = ioFrontmatter{
			WorkType: configs[i].WorkTypeName,
			State:    configs[i].StateName,
			Guard:    inputGuardFrontmatterPtr(configs[i].Guard),
		}
	}
	return out
}

func workerFrontmatterForExpansion(def interfaces.WorkerConfig) workerFrontmatter {
	modelProvider := def.ModelProvider
	if def.ModelProvider != "" {
		modelProvider = string(publicFactoryWorkerModelProviderFromInternal(def.ModelProvider))
	}
	executorProvider := def.ExecutorProvider
	if def.ExecutorProvider != "" {
		executorProvider = string(publicFactoryWorkerProviderFromInternal(def.ExecutorProvider))
	}
	return workerFrontmatter{
		Type:             def.Type,
		Provider:         publicFactoryHostedWorkerProviderFromInternal(def.Provider),
		Model:            def.Model,
		ModelProvider:    modelProvider,
		ExecutorProvider: executorProvider,
		Command:          def.Command,
		Args:             append([]string(nil), def.Args...),
		Resources:        append([]interfaces.ResourceConfig(nil), def.Resources...),
		Timeout:          def.Timeout,
		StopToken:        def.StopToken,
		SkipPermissions:  def.SkipPermissions,
		Auth:             cloneHostedWorkerAuthConfig(def.Auth),
		Linear:           cloneHostedLinearWorkerConfig(def.Linear),
	}
}

func inputGuardFrontmatterPtr(cfg *interfaces.InputGuardConfig) *inputGuardFrontmatter {
	if cfg == nil {
		return nil
	}
	return &inputGuardFrontmatter{
		Type:        interfaces.GuardType(publicFactoryGuardTypeStringFromInternal(cfg.Type)),
		MatchInput:  cfg.MatchInput,
		ParentInput: cfg.ParentInput,
		SpawnedBy:   cfg.SpawnedBy,
	}
}

func guardFrontmatterSlice(configs []interfaces.GuardConfig) []guardFrontmatter {
	if len(configs) == 0 {
		return nil
	}
	out := make([]guardFrontmatter, len(configs))
	for i := range configs {
		out[i] = guardFrontmatter{
			Type:        interfaces.GuardType(publicFactoryGuardTypeStringFromInternal(configs[i].Type)),
			Workstation: configs[i].Workstation,
			MaxVisits:   configs[i].MaxVisits,
			MatchConfig: cloneGuardMatchConfigPtr(configs[i].MatchConfig),
		}
	}
	return out
}

func authoredFactoryConfigForExpandedLayout(cfg *interfaces.FactoryConfig) (*interfaces.FactoryConfig, error) {
	authored, err := CloneFactoryConfig(cfg)
	if err != nil {
		return nil, err
	}
	for i := range authored.Workers {
		authored.Workers[i].Body = ""
	}
	for i := range authored.Workstations {
		authored.Workstations[i].Body = ""
		authored.Workstations[i].PromptTemplate = ""
	}
	if authored.ResourceManifest != nil {
		for i := range authored.ResourceManifest.BundledFiles {
			if !shouldOmitSupportedPortableBundledInline(authored.ResourceManifest.BundledFiles[i]) {
				continue
			}
			authored.ResourceManifest.BundledFiles[i].Content.Inline = ""
		}
	}
	return authored, nil
}

func renderAgentsMarkdown(frontmatter any, body string) ([]byte, error) {
	frontmatterBytes, err := yaml.Marshal(frontmatter)
	if err != nil {
		return nil, err
	}

	var rendered strings.Builder
	rendered.WriteString("---\n")
	rendered.Write(frontmatterBytes)
	rendered.WriteString("---\n")
	if body != "" {
		rendered.WriteString("\n")
		rendered.WriteString(strings.TrimSpace(body))
		rendered.WriteString("\n")
	}
	return []byte(rendered.String()), nil
}

func renderAgentsBody(body string) []byte {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return []byte{}
	}
	return []byte(trimmed + "\n")
}

func loadWorkerBody(dir string) (string, bool, error) {
	agentsPath := filepath.Join(dir, interfaces.FactoryAgentsFileName)
	body, err := loadAgentsBody(agentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("load worker body from %s: %w", dir, err)
	}
	return body, true, nil
}

func loadWorkstationBody(dir string) (string, bool, error) {
	agentsPath := filepath.Join(dir, interfaces.FactoryAgentsFileName)
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("load workstation body from %s: %w", dir, err)
	}

	content := string(data)
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		return "", false, nil
	}

	return strings.TrimSpace(content), true, nil
}

func writeAgentsFile(dir string, content []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, interfaces.FactoryAgentsFileName)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
