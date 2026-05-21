package config

import "github.com/portpowered/infinite-you/pkg/interfaces"

type workerFrontmatter struct {
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
		Model:            def.Model,
		ModelProvider:    modelProvider,
		ExecutorProvider: executorProvider,
		Command:          def.Command,
		Args:             append([]string(nil), def.Args...),
		Resources:        append([]interfaces.ResourceConfig(nil), def.Resources...),
		Timeout:          def.Timeout,
		StopToken:        def.StopToken,
		SkipPermissions:  def.SkipPermissions,
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
