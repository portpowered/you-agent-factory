package authored

import (
	"fmt"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"gopkg.in/yaml.v3"
)

var (
	internalFactoryHostedWorkerProviderFromPublic = factorymapping.InternalFactoryHostedWorkerProviderFromPublic
	internalFactoryWorkerModelProviderFromPublic  = factorymapping.InternalFactoryWorkerModelProviderFromPublic
	internalFactoryWorkerProviderFromPublic       = factorymapping.InternalFactoryWorkerProviderFromPublic
	internalFactoryWorkstationKindFromPublic      = factorymapping.InternalFactoryWorkstationKindFromPublic
	internalFactoryGuardTypeFromPublic            = factorymapping.InternalFactoryGuardTypeFromPublic
	publicFactoryHostedWorkerProviderFromInternal = factorymapping.PublicFactoryHostedWorkerProviderFromInternal
	publicFactoryWorkerModelProviderFromInternal  = factorymapping.PublicFactoryWorkerModelProviderFromInternal
	publicFactoryWorkerProviderFromInternal       = factorymapping.PublicFactoryWorkerProviderFromInternal
	publicFactoryWorkstationKindFromInternal      = factorymapping.PublicFactoryWorkstationKindFromInternal
	publicFactoryGuardTypeStringFromInternal      = factorymapping.PublicFactoryGuardTypeStringFromInternal
	runtimeResourceRequirementsFromBoundaryValue  = factorymapping.RuntimeResourceRequirementsFromBoundaryValue
	NormalizeWorkstationExecutionLimit            = factorymapping.NormalizeWorkstationExecutionLimit
	cloneStringMap                                = factorymapping.CloneStringMap
)

// ParseWorkerConfig maps one authored AGENTS.md representation into its
// canonical Worker configuration. Filesystem ownership remains with Factory
// Definitions.
func ParseWorkerConfig(data []byte, sourcePath string) (*factorydefinitions.FactoryWorkerConfig, error) {
	frontmatter, body, err := parseAgentsMarkdown(data, sourcePath)
	if err != nil {
		return nil, err
	}

	rawFrontmatter, err := parseAgentsFrontmatterMap(frontmatter)
	if err != nil {
		return nil, fmt.Errorf("parse worker frontmatter in %s: %w", sourcePath, err)
	}
	normalizeAgentsRuntimeResources(rawFrontmatter)
	frontmatter, err = yaml.Marshal(rawFrontmatter)
	if err != nil {
		return nil, fmt.Errorf("normalize worker frontmatter in %s: %w", sourcePath, err)
	}

	var parsed workerFrontmatterInput
	if err := yaml.Unmarshal(frontmatter, &parsed); err != nil {
		return nil, fmt.Errorf("parse worker frontmatter in %s: %w", sourcePath, err)
	}

	cfg := factorydefinitions.FactoryWorkerConfig{
		Type:             parsed.Type,
		Provider:         parsed.Provider,
		Model:            parsed.Model,
		ModelProvider:    parsed.ModelProvider,
		ExecutorProvider: parsed.ExecutorProvider,
		Command:          parsed.Command,
		Args:             append([]string(nil), parsed.Args...),
		Resources:        append([]factorydefinitions.ResourceConfig(nil), parsed.Resources...),
		Timeout:          parsed.Timeout,
		StopToken:        parsed.StopToken,
		SkipPermissions:  parsed.SkipPermissions,
		OpenCodeAgent:    parsed.OpenCodeAgent,
		Auth:             cloneHostedWorkerAuthConfig(parsed.Auth),
		Linear:           cloneHostedLinearWorkerConfig(parsed.Linear),
		Body:             body,
	}
	if err := validateOpenCodeAgentInFrontmatter(rawFrontmatter, "frontmatter"); err != nil {
		return nil, fmt.Errorf("validate worker frontmatter in %s: %w", sourcePath, err)
	}
	if cfg.Provider != "" {
		cfg.Provider = internalFactoryHostedWorkerProviderFromPublic(cfg.Provider)
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
	Type             string                                       `yaml:"type"`
	Provider         string                                       `yaml:"provider,omitempty"`
	Model            string                                       `yaml:"model,omitempty"`
	ModelProvider    string                                       `yaml:"modelProvider,omitempty"`
	ExecutorProvider string                                       `yaml:"executorProvider,omitempty"`
	Command          string                                       `yaml:"command,omitempty"`
	Args             []string                                     `yaml:"args,omitempty"`
	Resources        []factorydefinitions.ResourceConfig          `yaml:"resources,omitempty"`
	Timeout          string                                       `yaml:"timeout,omitempty"`
	StopToken        string                                       `yaml:"stopToken,omitempty"`
	SkipPermissions  bool                                         `yaml:"skipPermissions,omitempty"`
	OpenCodeAgent    string                                       `yaml:"openCodeAgent,omitempty"`
	Auth             *factorydefinitions.HostedWorkerAuthConfig   `yaml:"auth,omitempty"`
	Linear           *factorydefinitions.HostedLinearWorkerConfig `yaml:"linear,omitempty"`
}

func validateOpenCodeAgentInFrontmatter(frontmatter map[string]any, path string) error {
	raw, ok := frontmatter["openCodeAgent"]
	if !ok {
		return nil
	}
	agent, ok := raw.(string)
	if !ok {
		return fmt.Errorf("%s.openCodeAgent must be a string", path)
	}
	return validateOpenCodeAgentField(path, agent)
}

func validateOpenCodeAgentField(path, agent string) error {
	if strings.TrimSpace(agent) == "" {
		return fmt.Errorf("%s.openCodeAgent must be a non-empty string", path)
	}
	return nil
}

func cloneHostedWorkerAuthConfig(cfg *factorydefinitions.HostedWorkerAuthConfig) *factorydefinitions.HostedWorkerAuthConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	return &cloned
}

func cloneHostedLinearWorkerConfig(cfg *factorydefinitions.HostedLinearWorkerConfig) *factorydefinitions.HostedLinearWorkerConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.TeamIDs = append([]string(nil), cfg.TeamIDs...)
	cloned.StateIDs = append([]string(nil), cfg.StateIDs...)
	if cfg.Claim != nil {
		claim := *cfg.Claim
		cloned.Claim = &claim
	}
	return &cloned
}

// ParseWorkstationConfig maps one authored AGENTS.md representation into its
// canonical Workstation configuration. A referenced prompt is loaded later by
// the Factory Definitions authored-layout reader.
func ParseWorkstationConfig(data []byte, sourcePath string) (*factorydefinitions.FactoryWorkstationConfig, error) {
	frontmatter, body, err := parseAgentsMarkdown(data, sourcePath)
	if err != nil {
		return nil, err
	}

	rawFrontmatter, err := parseAgentsFrontmatterMap(frontmatter)
	if err != nil {
		return nil, fmt.Errorf("parse workstation frontmatter in %s: %w", sourcePath, err)
	}
	normalizeAgentsRuntimeResources(rawFrontmatter)
	frontmatter, err = yaml.Marshal(rawFrontmatter)
	if err != nil {
		return nil, fmt.Errorf("normalize workstation frontmatter in %s: %w", sourcePath, err)
	}

	var cfg factorydefinitions.FactoryWorkstationConfig
	if err := yaml.Unmarshal(frontmatter, &cfg); err != nil {
		return nil, fmt.Errorf("parse workstation frontmatter in %s: %w", sourcePath, err)
	}
	normalizeWorkstationPublicEnums(&cfg)
	NormalizeWorkstationExecutionLimit(&cfg)
	if err := validateOpenCodeAgentInFrontmatter(rawFrontmatter, "frontmatter"); err != nil {
		return nil, fmt.Errorf("validate workstation frontmatter in %s: %w", sourcePath, err)
	}

	cfg.Body = body
	if cfg.PromptFile == "" {
		cfg.PromptTemplate = body
	}

	return &cfg, nil
}

// ParseAgentsBody returns the markdown body from an authored AGENTS.md
// representation, or the complete contents for a body-only representation.
func ParseAgentsBody(data []byte, sourcePath string) (string, error) {
	content := string(data)
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		_, body, err := parseAgentsMarkdown(data, sourcePath)
		if err != nil {
			return "", err
		}
		return body, nil
	}

	return content, nil
}

func normalizeWorkstationPublicEnums(cfg *factorydefinitions.FactoryWorkstationConfig) {
	if cfg == nil {
		return
	}
	cfg.Runner = workers.NormalizeRunnerID(cfg.Runner)
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

// parseAgentsMarkdown splits AGENTS.md bytes into YAML frontmatter and markdown
// body. Frontmatter is delimited by --- on its own lines.
func parseAgentsMarkdown(data []byte, sourcePath string) (frontmatter []byte, body string, err error) {
	content := string(data)

	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return nil, "", fmt.Errorf("AGENTS.md missing frontmatter delimiter at %s", sourcePath)
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
			return nil, "", fmt.Errorf("AGENTS.md missing closing frontmatter delimiter at %s", sourcePath)
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

type workerFrontmatter struct {
	Type             string                                       `yaml:"type"`
	Provider         string                                       `yaml:"provider,omitempty"`
	Model            string                                       `yaml:"model,omitempty"`
	ModelProvider    string                                       `yaml:"modelProvider,omitempty"`
	ExecutorProvider string                                       `yaml:"executorProvider,omitempty"`
	Command          string                                       `yaml:"command,omitempty"`
	Args             []string                                     `yaml:"args,omitempty"`
	Resources        []factorydefinitions.ResourceConfig          `yaml:"resources,omitempty"`
	Timeout          string                                       `yaml:"timeout,omitempty"`
	StopToken        string                                       `yaml:"stopToken,omitempty"`
	SkipPermissions  bool                                         `yaml:"skipPermissions,omitempty"`
	OpenCodeAgent    string                                       `yaml:"openCodeAgent,omitempty"`
	Auth             *factorydefinitions.HostedWorkerAuthConfig   `yaml:"auth,omitempty"`
	Linear           *factorydefinitions.HostedLinearWorkerConfig `yaml:"linear,omitempty"`
}

type workstationFrontmatter struct {
	ID               string                              `yaml:"id,omitempty"`
	Name             string                              `yaml:"name,omitempty"`
	Kind             factorydefinitions.WorkstationKind  `yaml:"behavior,omitempty"`
	Type             string                              `yaml:"type,omitempty"`
	Worker           string                              `yaml:"worker,omitempty"`
	Runner           string                              `yaml:"runner,omitempty"`
	OpenCodeAgent    string                              `yaml:"openCodeAgent,omitempty"`
	PromptFile       string                              `yaml:"promptFile,omitempty"`
	OutputSchema     string                              `yaml:"outputSchema,omitempty"`
	Limits           workstationLimitsFrontmatter        `yaml:"limits,omitempty"`
	Cron             *cronFrontmatter                    `yaml:"cron,omitempty"`
	Inputs           []ioFrontmatter                     `yaml:"inputs,omitempty"`
	Outputs          []ioFrontmatter                     `yaml:"outputs,omitempty"`
	OnContinue       []ioFrontmatter                     `yaml:"onContinue,omitempty"`
	OnRejection      []ioFrontmatter                     `yaml:"onRejection,omitempty"`
	OnFailure        []ioFrontmatter                     `yaml:"onFailure,omitempty"`
	Resources        []factorydefinitions.ResourceConfig `yaml:"resources,omitempty"`
	Guards           []guardFrontmatter                  `yaml:"guards,omitempty"`
	StopWords        []string                            `yaml:"stopWords,omitempty"`
	WorkingDirectory string                              `yaml:"workingDirectory,omitempty"`
	Worktree         string                              `yaml:"worktree,omitempty"`
	Env              map[string]string                   `yaml:"env,omitempty"`
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
	Type        factorydefinitions.GuardType `yaml:"type"`
	MatchInput  string                       `yaml:"matchInput,omitempty"`
	ParentInput string                       `yaml:"parentInput,omitempty"`
	SpawnedBy   string                       `yaml:"spawnedBy,omitempty"`
}

type guardFrontmatter struct {
	Type        factorydefinitions.GuardType         `yaml:"type"`
	Workstation string                               `yaml:"workstation,omitempty"`
	MaxVisits   int                                  `yaml:"maxVisits,omitempty"`
	MatchConfig *factorydefinitions.GuardMatchConfig `yaml:"matchConfig,omitempty"`
}

func workstationFrontmatterForExpansion(def factorydefinitions.FactoryWorkstationConfig) workstationFrontmatter {
	behavior := def.Kind
	if def.Kind != "" {
		behavior = factorydefinitions.WorkstationKind(publicFactoryWorkstationKindFromInternal(def.Kind))
	}
	rendered := workstationFrontmatter{
		ID:               def.ID,
		Name:             def.Name,
		Kind:             behavior,
		Type:             def.Type,
		Worker:           def.WorkerTypeName,
		Runner:           def.Runner,
		OpenCodeAgent:    def.OpenCodeAgent,
		PromptFile:       def.PromptFile,
		OutputSchema:     def.OutputSchema,
		Limits:           workstationLimitsFrontmatter{MaxRetries: def.Limits.MaxRetries, MaxExecutionTime: def.Limits.MaxExecutionTime},
		Inputs:           ioFrontmatterSlice(def.Inputs),
		Outputs:          ioFrontmatterSlice(def.Outputs),
		OnContinue:       ioFrontmatterSlice(def.OnContinue),
		OnRejection:      ioFrontmatterSlice(def.OnRejection),
		OnFailure:        ioFrontmatterSlice(def.OnFailure),
		Resources:        append([]factorydefinitions.ResourceConfig(nil), def.Resources...),
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

func ioFrontmatterSlice(configs []factorydefinitions.IOConfig) []ioFrontmatter {
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

func workerFrontmatterForExpansion(def factorydefinitions.FactoryWorkerConfig) workerFrontmatter {
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
		Resources:        append([]factorydefinitions.ResourceConfig(nil), def.Resources...),
		Timeout:          def.Timeout,
		StopToken:        def.StopToken,
		SkipPermissions:  def.SkipPermissions,
		OpenCodeAgent:    def.OpenCodeAgent,
		Auth:             cloneHostedWorkerAuthConfig(def.Auth),
		Linear:           cloneHostedLinearWorkerConfig(def.Linear),
	}
}

func inputGuardFrontmatterPtr(cfg *factorydefinitions.InputGuardConfig) *inputGuardFrontmatter {
	if cfg == nil {
		return nil
	}
	return &inputGuardFrontmatter{
		Type:        factorydefinitions.GuardType(publicFactoryGuardTypeStringFromInternal(cfg.Type)),
		MatchInput:  cfg.MatchInput,
		ParentInput: cfg.ParentInput,
		SpawnedBy:   cfg.SpawnedBy,
	}
}

func guardFrontmatterSlice(configs []factorydefinitions.GuardConfig) []guardFrontmatter {
	if len(configs) == 0 {
		return nil
	}
	out := make([]guardFrontmatter, len(configs))
	for i := range configs {
		out[i] = guardFrontmatter{
			Type:        factorydefinitions.GuardType(publicFactoryGuardTypeStringFromInternal(configs[i].Type)),
			Workstation: configs[i].Workstation,
			MaxVisits:   configs[i].MaxVisits,
			MatchConfig: factorydefinitions.CloneGuardMatchConfig(configs[i].MatchConfig),
		}
	}
	return out
}

func authoredFactoryConfigForExpandedLayout(cfg *factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error) {
	authored, err := factorydefinitions.CloneFactoryConfig(cfg)
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
			if !factorydefinitions.ShouldOmitSupportedPortableBundledInline(authored.ResourceManifest.BundledFiles[i]) {
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
	return []byte(body)
}

func AuthoredFactoryConfigForExpandedLayout(
	cfg *factorydefinitions.FactoryConfig,
) (*factorydefinitions.FactoryConfig, error) {
	return authoredFactoryConfigForExpandedLayout(cfg)
}

func CloneHostedWorkerAuthConfig(
	cfg *factorydefinitions.HostedWorkerAuthConfig,
) *factorydefinitions.HostedWorkerAuthConfig {
	return cloneHostedWorkerAuthConfig(cfg)
}

func CloneHostedLinearWorkerConfig(
	cfg *factorydefinitions.HostedLinearWorkerConfig,
) *factorydefinitions.HostedLinearWorkerConfig {
	return cloneHostedLinearWorkerConfig(cfg)
}

func SafeFactoryLayoutSegment(kind, name string) (string, error) {
	return safeFactoryLayoutSegment(kind, name)
}

func SafePromptFilePath(workstationDir, promptFile string) (string, error) {
	return safePromptFilePath(workstationDir, promptFile)
}

func RenderWorkerAgentsMarkdown(
	def factorydefinitions.FactoryWorkerConfig,
) ([]byte, error) {
	return renderAgentsMarkdown(workerFrontmatterForExpansion(def), def.Body)
}

func RenderWorkstationAgentsMarkdown(
	def factorydefinitions.FactoryWorkstationConfig,
) ([]byte, error) {
	return renderAgentsMarkdown(workstationFrontmatterForExpansion(def), def.Body)
}

func RenderAgentsBody(body string) []byte {
	return renderAgentsBody(body)
}
