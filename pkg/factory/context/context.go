package factory_context

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	TIMESTAMP_FORMAT = "20060102T150405"
	ProjectTagKey    = "project"
	DefaultProjectID = "default-project"
)

// FactoryContext is the shared execution environment passed to workers.
// It provides filesystem paths, environment variables, and identifiers
// that workers need to interact with the execution environment.
type FactoryContext struct {
	FactoryDirectory string            `json:"workflow_id"`
	WorkDirectory    string            `json:"work_directory"`
	EnvVars          map[string]string `json:"env_vars"`
	ArtifactDir      string            `json:"artifact_directory"`
	ProjectID        string            `json:"project_id,omitempty"`
}

// WorkflowConfig holds workflow-level configuration used when creating
// a WorkflowContext. It captures settings from the workflow definition
// that affect the execution environment.
type WorkflowConfig struct {
	EnvVars map[string]string `json:"env_vars" yaml:"env_vars"`
	Project string            `json:"project,omitempty" yaml:"project,omitempty"`
}

// SubmitParams holds per-submission overrides provided when submitting
// work to the factory. These are merged last (highest priority) into
// the WorkflowContext.
type SubmitParams struct {
	EnvVars map[string]string `json:"env_vars"`
	Project string            `json:"project,omitempty" yaml:"project,omitempty"`
}

// ContextOption configures NewWorkflowContext.
type ContextOption func(*contextOptions)

type contextOptions struct {
	baseDir   string
	timestamp time.Time
}

// WithBaseDir sets the root directory under which run directories are created.
// Defaults to "factory/runs".
func WithBaseDir(dir string) ContextOption {
	return func(o *contextOptions) {
		o.baseDir = dir
	}
}

// WithTimestamp overrides the timestamp used for the run directory name.
// Useful for deterministic testing.
func WithTimestamp(t time.Time) ContextOption {
	return func(o *contextOptions) {
		o.timestamp = t
	}
}

// NewFactoryContext creates a WorkflowContext for a workflow instance.
// It sets up the run directory structure, merges environment variables
// from factory, workflow, and submission levels, and optionally creates
// a git worktree.
func NewFactoryContext(
	workflowID string,
	factoryEnv map[string]string,
	wfCfg *WorkflowConfig,
	submitParams *SubmitParams,
	opts ...ContextOption,
) (*FactoryContext, error) {
	o := &contextOptions{
		baseDir:   "factory/runs",
		timestamp: time.Now(),
	}
	for _, opt := range opts {
		opt(o)
	}

	ts := o.timestamp.Format(TIMESTAMP_FORMAT)
	runDir := filepath.Join(o.baseDir, workflowID, ts)
	workDir := filepath.Join(runDir, "work")
	artifactDir := filepath.Join(runDir, interfaces.ArtifactsDirectory)

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating work dir: %w", err)
	}
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating artifact dir: %w", err)
	}

	envVars := MergeEnvVars(factoryEnv, factoryConfigEnvironment(wfCfg), submitEnv(submitParams))

	return &FactoryContext{
		FactoryDirectory: workflowID,
		WorkDirectory:    workDir,
		EnvVars:          envVars,
		ArtifactDir:      artifactDir,
		ProjectID:        ResolveProjectID("", wfCfg, submitParams),
	}, nil
}

// MergeEnvVars merges multiple environment variable maps in priority order.
// Later maps override earlier ones. Nil maps are skipped.
func MergeEnvVars(envMaps ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, m := range envMaps {
		if m != nil {
			maps.Copy(merged, m)
		}
	}
	return merged
}

func factoryConfigEnvironment(cfg *WorkflowConfig) map[string]string {
	if cfg == nil {
		return nil
	}
	return cfg.EnvVars
}

func submitEnv(p *SubmitParams) map[string]string {
	if p == nil {
		return nil
	}
	return p.EnvVars
}

// ResolveProjectID applies the project-context precedence used by runtime
// templates: explicit token/request value, submit override, workflow config,
// then the neutral default.
func ResolveProjectID(explicit string, wfCfg *WorkflowConfig, submitParams *SubmitParams) string {
	if project := strings.TrimSpace(explicit); project != "" {
		return project
	}
	if submitParams != nil {
		if project := strings.TrimSpace(submitParams.Project); project != "" {
			return project
		}
	}
	if wfCfg != nil {
		if project := strings.TrimSpace(wfCfg.Project); project != "" {
			return project
		}
	}
	return DefaultProjectID
}
