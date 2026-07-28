package factory_context

import (
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	TIMESTAMP_FORMAT = "20060102T150405"
	ProjectTagKey    = workerexecution.ProjectTagKey
	DefaultProjectID = workerexecution.DefaultProjectID
	// DefaultSessionID is the stable alias for the primary live factory session.
	DefaultSessionID = workerexecution.DefaultSessionID
)

// FactoryContext is the shared execution environment passed to workers.
// It provides filesystem paths, environment variables, and identifiers
// that workers need to interact with the execution environment.
type FactoryContext = workerexecution.Context

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

// DirectoryCreator is the exact filesystem effect required to materialize a
// Factory Runtime execution context.
type DirectoryCreator interface {
	MkdirAll(string, fs.FileMode) error
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
	baseDir string,
	timestamp time.Time,
	directories DirectoryCreator,
) (*FactoryContext, error) {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "factory/runs"
	}
	if timestamp.IsZero() {
		return nil, fmt.Errorf("Factory Runtime context timestamp is required")
	}
	if directories == nil {
		return nil, fmt.Errorf("Factory Runtime context directory creator is required")
	}

	ts := timestamp.Format(TIMESTAMP_FORMAT)
	runDir := filepath.Join(baseDir, workflowID, ts)
	workDir := filepath.Join(runDir, "work")
	artifactDir := filepath.Join(runDir, interfaces.ArtifactsDirectory)

	if err := directories.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating work dir: %w", err)
	}
	if err := directories.MkdirAll(artifactDir, 0o755); err != nil {
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
