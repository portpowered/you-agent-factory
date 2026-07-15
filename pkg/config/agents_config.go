package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config/retiredboundary"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerrunner "github.com/portpowered/infinite-you/pkg/workers/runner"
	"gopkg.in/yaml.v3"
)

// WorkstationLoader loads workstation definitions by name.
// Implement this interface in tests to inject workstation configs without
// requiring AGENTS.md files on disk. Returning (nil, nil) signals no config is
// available and the caller should use its normal fallback behavior.
type WorkstationLoader interface {
	Load(name string) (*interfaces.FactoryWorkstationConfig, error)
}

// ErrInvalidNamedFactoryName reports that the requested canonical named-factory
// name failed validation before any on-disk lookup or persistence work began.
var ErrInvalidNamedFactoryName = errors.New("invalid named factory name")

// ErrNamedFactoryNotFound reports that a requested named factory was not found
// in the searched root or roots.
var ErrNamedFactoryNotFound = errors.New("named factory not found")

// NamedFactoryResolutionSource identifies the root that supplied a resolved
// named factory.
type NamedFactoryResolutionSource string

const (
	NamedFactoryResolutionSourceProjectLocal NamedFactoryResolutionSource = "project-local"
	NamedFactoryResolutionSourceGlobal       NamedFactoryResolutionSource = "global"
)

// NamedFactoryPrecedenceDecision reports whether cross-root resolution observed
// a local-over-global conflict before selecting a runnable directory.
type NamedFactoryPrecedenceDecision string

const (
	NamedFactoryPrecedenceDecisionNone              NamedFactoryPrecedenceDecision = "none"
	NamedFactoryPrecedenceDecisionProjectOverGlobal NamedFactoryPrecedenceDecision = "project-local-over-global"
)

// NamedFactoryResolution reports the selected runnable directory and the roots
// considered during cross-root named-factory resolution.
type NamedFactoryResolution struct {
	Name               string
	FactoryDir         string
	Source             NamedFactoryResolutionSource
	ProjectRoot        string
	GlobalRoot         string
	PrecedenceDecision NamedFactoryPrecedenceDecision
}

type invalidNamedFactoryNameError struct {
	name string
	err  error
}

func (e *invalidNamedFactoryNameError) Error() string {
	return fmt.Sprintf("invalid named factory name %q: %v", e.name, e.err)
}

func (e *invalidNamedFactoryNameError) Unwrap() []error {
	return []error{ErrInvalidNamedFactoryName, e.err}
}

type namedFactoryNotFoundError struct{ name string }

func (e *namedFactoryNotFoundError) Error() string {
	return fmt.Sprintf("named factory %q not found", e.name)
}

func (e *namedFactoryNotFoundError) Unwrap() []error {
	return []error{ErrNamedFactoryNotFound, os.ErrNotExist}
}

func wrapInvalidNamedFactoryName(name string, err error) error {
	if err == nil || errors.Is(err, ErrInvalidNamedFactoryName) {
		return err
	}
	return &invalidNamedFactoryNameError{name: strings.TrimSpace(name), err: err}
}

func namedFactoryResolution(
	name string,
	factoryDir string,
	source NamedFactoryResolutionSource,
	projectRoot string,
	globalRoot string,
	precedence NamedFactoryPrecedenceDecision,
) *NamedFactoryResolution {
	return &NamedFactoryResolution{
		Name:               name,
		FactoryDir:         factoryDir,
		Source:             source,
		ProjectRoot:        projectRoot,
		GlobalRoot:         globalRoot,
		PrecedenceDecision: precedence,
	}
}

func projectLocalPrecedenceDecision(globalRoot, canonicalName string) NamedFactoryPrecedenceDecision {
	if globalRoot == "" {
		return NamedFactoryPrecedenceDecisionNone
	}
	if hasNamedFactoryCandidate(globalRoot, canonicalName) {
		return NamedFactoryPrecedenceDecisionProjectOverGlobal
	}
	return NamedFactoryPrecedenceDecisionNone
}

func hasNamedFactoryCandidate(rootDir, canonicalName string) bool {
	_, found, err := resolveNamedFactoryCandidate(rootDir, canonicalName)
	return err == nil && found
}

func newNamedFactoryNotFoundError(name string) error {
	return &namedFactoryNotFoundError{name: strings.TrimSpace(name)}
}

// IsInvalidNamedFactoryName reports whether err wraps ErrInvalidNamedFactoryName.
func IsInvalidNamedFactoryName(err error) bool { return errors.Is(err, ErrInvalidNamedFactoryName) }

// IsNamedFactoryNotFound reports whether err wraps ErrNamedFactoryNotFound.
func IsNamedFactoryNotFound(err error) bool { return errors.Is(err, ErrNamedFactoryNotFound) }

// LoadWorkerConfig loads a worker configuration from the given directory.
// It reads AGENTS.md, parses YAML frontmatter into WorkerConfig, and sets
// Body to the remaining markdown content.
func LoadWorkerConfig(dir string) (*workerconfig.Config, error) {
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

	cfg := workerconfig.Config{
		Type:             parsed.Type,
		Provider:         parsed.Provider,
		Model:            parsed.Model,
		ModelProvider:    parsed.ModelProvider,
		ExecutorProvider: parsed.ExecutorProvider,
		Command:          parsed.Command,
		Args:             append([]string(nil), parsed.Args...),
		Resources:        append([]factoryresource.Config(nil), parsed.Resources...),
		Timeout:          parsed.Timeout,
		StopToken:        parsed.StopToken,
		SkipPermissions:  parsed.SkipPermissions,
		OpenCodeAgent:    parsed.OpenCodeAgent,
		Auth:             cloneHostedWorkerAuthConfig(parsed.Auth),
		Linear:           cloneHostedLinearWorkerConfig(parsed.Linear),
		Body:             body,
	}
	if err := validateOpenCodeAgentInFrontmatter(rawFrontmatter, "frontmatter"); err != nil {
		return nil, fmt.Errorf("validate worker frontmatter in %s: %w", agentsPath, err)
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
	Type             string                                 `yaml:"type"`
	Provider         string                                 `yaml:"provider,omitempty"`
	Model            string                                 `yaml:"model,omitempty"`
	ModelProvider    string                                 `yaml:"modelProvider,omitempty"`
	ExecutorProvider string                                 `yaml:"executorProvider,omitempty"`
	Command          string                                 `yaml:"command,omitempty"`
	Args             []string                               `yaml:"args,omitempty"`
	Resources        []factoryresource.Config               `yaml:"resources,omitempty"`
	Timeout          string                                 `yaml:"timeout,omitempty"`
	StopToken        string                                 `yaml:"stopToken,omitempty"`
	SkipPermissions  bool                                   `yaml:"skipPermissions,omitempty"`
	OpenCodeAgent    string                                 `yaml:"openCodeAgent,omitempty"`
	Auth             *workerconfig.HostedWorkerAuthConfig   `yaml:"auth,omitempty"`
	Linear           *workerconfig.HostedLinearWorkerConfig `yaml:"linear,omitempty"`
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

func cloneHostedWorkerAuthConfig(cfg *workerconfig.HostedWorkerAuthConfig) *workerconfig.HostedWorkerAuthConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	return &cloned
}

func cloneHostedLinearWorkerConfig(cfg *workerconfig.HostedLinearWorkerConfig) *workerconfig.HostedLinearWorkerConfig {
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
	if err := validateOpenCodeAgentInFrontmatter(rawFrontmatter, "frontmatter"); err != nil {
		return nil, fmt.Errorf("validate workstation frontmatter in %s: %w", agentsPath, err)
	}

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

	return content, nil
}

func normalizeWorkstationPublicEnums(cfg *interfaces.FactoryWorkstationConfig) {
	if cfg == nil {
		return
	}
	cfg.Runner = workerrunner.NormalizeRunnerID(cfg.Runner)
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
	if err := retiredboundary.RejectHostedProviderAlias(frontmatter, "frontmatter"); err != nil {
		return err
	}
	return retiredboundary.RejectFields(frontmatter, "frontmatter", []retiredboundary.Field{
		{Key: "model_provider", Replacement: "use modelProvider"},
		{Key: "sessionId", Replacement: "remove sessionId; provider sessions are runtime-owned"},
		{Key: "session_id", Replacement: "remove sessionId; provider sessions are runtime-owned"},
		{Key: "concurrency", Replacement: "remove concurrency; use resources to limit concurrent work"},
		{Key: "stop_token", Replacement: "use stopToken"},
		{Key: "skip_permissions", Replacement: "use skipPermissions"},
		{Key: "open_code_agent", Replacement: "use openCodeAgent"},
	})
}

func rejectRetiredWorkstationFrontmatterAliases(frontmatter map[string]any) error {
	if err := retiredboundary.RejectFields(frontmatter, "frontmatter", []retiredboundary.Field{
		{Key: "kind", Replacement: "use behavior"},
		{Key: "runtimeType", Replacement: "use type"},
		{Key: "runtime_type", Replacement: "use type"},
		{Key: "prompt_file", Replacement: "use promptFile"},
		{Key: "output_schema", Replacement: "use outputSchema"},
		{Key: "on_continue", Replacement: "use onContinue"},
		{Key: "on_rejection", Replacement: "use onRejection"},
		{Key: "on_failure", Replacement: "use onFailure"},
		{Key: "resourceUsage", Replacement: "use resources"},
		{Key: "resource_usage", Replacement: "use resources"},
		{Key: "stopToken", Replacement: "use stopWords"},
		{Key: "stop_token", Replacement: "use stopWords"},
		{Key: "stop_words", Replacement: "use stopWords"},
		{Key: "runtimeStopWords", Replacement: "use stopWords"},
		{Key: "runtime_stop_words", Replacement: "use stopWords"},
		{Key: "timeout", Replacement: "use limits.maxExecutionTime"},
		{Key: "working_directory", Replacement: "use workingDirectory"},
		{Key: "open_code_agent", Replacement: "use openCodeAgent"},
	}); err != nil {
		return err
	}
	if err := retiredboundary.RejectFields(frontmatterMap(frontmatter["limits"]), "frontmatter.limits", []retiredboundary.Field{
		{Key: "max_retries", Replacement: "use maxRetries"},
		{Key: "max_execution_time", Replacement: "use maxExecutionTime"},
	}); err != nil {
		return err
	}
	if err := retiredboundary.RejectCronBoundaryAliases(frontmatter["cron"], "frontmatter.cron"); err != nil {
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
	if err := retiredboundary.RejectFields(entry, path, []retiredboundary.Field{
		{Key: "work_type", Replacement: "use workType"},
	}); err != nil {
		return err
	}
	return rejectRetiredInputGuardFrontmatterAliases(entry["guard"], path+".guard")
}

func rejectRetiredInputGuardFrontmatterAliases(raw any, path string) error {
	return retiredboundary.RejectFields(frontmatterMap(raw), path, []retiredboundary.Field{
		{Key: "match_input", Replacement: "use matchInput"},
		{Key: "parent_input", Replacement: "use parentInput"},
		{Key: "spawned_by", Replacement: "use spawnedBy"},
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
		if err := retiredboundary.RejectFields(entry, entryPath, []retiredboundary.Field{
			{Key: "max_visits", Replacement: "use maxVisits"},
		}); err != nil {
			return err
		}
		if err := retiredboundary.RejectFields(frontmatterMap(entry["matchConfig"]), entryPath+".matchConfig", []retiredboundary.Field{
			{Key: "input_key", Replacement: "use inputKey"},
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
	Type             string                                 `yaml:"type"`
	Provider         string                                 `yaml:"provider,omitempty"`
	Model            string                                 `yaml:"model,omitempty"`
	ModelProvider    string                                 `yaml:"modelProvider,omitempty"`
	ExecutorProvider string                                 `yaml:"executorProvider,omitempty"`
	Command          string                                 `yaml:"command,omitempty"`
	Args             []string                               `yaml:"args,omitempty"`
	Resources        []factoryresource.Config               `yaml:"resources,omitempty"`
	Timeout          string                                 `yaml:"timeout,omitempty"`
	StopToken        string                                 `yaml:"stopToken,omitempty"`
	SkipPermissions  bool                                   `yaml:"skipPermissions,omitempty"`
	OpenCodeAgent    string                                 `yaml:"openCodeAgent,omitempty"`
	Auth             *workerconfig.HostedWorkerAuthConfig   `yaml:"auth,omitempty"`
	Linear           *workerconfig.HostedLinearWorkerConfig `yaml:"linear,omitempty"`
}

type workstationFrontmatter struct {
	ID               string                       `yaml:"id,omitempty"`
	Name             string                       `yaml:"name,omitempty"`
	Kind             interfaces.WorkstationKind   `yaml:"behavior,omitempty"`
	Type             string                       `yaml:"type,omitempty"`
	Worker           string                       `yaml:"worker,omitempty"`
	Runner           string                       `yaml:"runner,omitempty"`
	OpenCodeAgent    string                       `yaml:"openCodeAgent,omitempty"`
	PromptFile       string                       `yaml:"promptFile,omitempty"`
	OutputSchema     string                       `yaml:"outputSchema,omitempty"`
	Limits           workstationLimitsFrontmatter `yaml:"limits,omitempty"`
	Cron             *cronFrontmatter             `yaml:"cron,omitempty"`
	Inputs           []ioFrontmatter              `yaml:"inputs,omitempty"`
	Outputs          []ioFrontmatter              `yaml:"outputs,omitempty"`
	OnContinue       []ioFrontmatter              `yaml:"onContinue,omitempty"`
	OnRejection      []ioFrontmatter              `yaml:"onRejection,omitempty"`
	OnFailure        []ioFrontmatter              `yaml:"onFailure,omitempty"`
	Resources        []factoryresource.Config     `yaml:"resources,omitempty"`
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
		Resources:        append([]factoryresource.Config(nil), def.Resources...),
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

func workerFrontmatterForExpansion(def workerconfig.Config) workerFrontmatter {
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
		Resources:        append([]factoryresource.Config(nil), def.Resources...),
		Timeout:          def.Timeout,
		StopToken:        def.StopToken,
		SkipPermissions:  def.SkipPermissions,
		OpenCodeAgent:    def.OpenCodeAgent,
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
	return []byte(body)
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

	return content, true, nil
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
