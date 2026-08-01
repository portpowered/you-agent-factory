package service

import (
	"fmt"
	"path/filepath"
	"strings"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type resumeCursor struct {
	cursor     automations.Cursor
	checkpoint string
}

func scriptPollerCommandRequest(
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	resolveTemplates workers.TemplateFieldResolver,
	resume resumeCursor,
) (workers.CommandRequest, error) {
	if runtimeCfg == nil {
		return workers.CommandRequest{}, fmt.Errorf("runtime config is required")
	}
	if workerDef == nil {
		return workers.CommandRequest{}, fmt.Errorf("script poller worker is required")
	}
	if strings.TrimSpace(workerDef.Command) == "" {
		return workers.CommandRequest{}, fmt.Errorf("script poller worker %q is missing command", workerDef.Name)
	}

	requestContext := &workerexecution.Context{}
	if resolved, err := resolveWorkerTemplateFields(
		resolveTemplates,
		workstation.WorkingDirectory,
		workstation.Env,
		nil,
		requestContext,
		workstation.Worktree,
	); err != nil {
		return workers.CommandRequest{}, fmt.Errorf("resolve poller workstation fields: %w", err)
	} else if resolved != nil {
		requestContext.WorkDirectory = resolved.WorkingDirectory
		requestContext.EnvVars = resolved.Env
		if requestContext.WorkDirectory == "" {
			requestContext.WorkDirectory = resolved.Worktree
		}
	}

	workDir := requestContext.WorkDirectory
	if workDir != "" && !filepath.IsAbs(workDir) {
		baseDir := runtimeCfg.RuntimeBaseDir()
		if baseDir == "" {
			baseDir = runtimeCfg.FactoryDir()
		}
		if baseDir != "" {
			workDir = filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(workDir)))
		}
	}
	if workDir == "" {
		workDir = pollerRuntimeWorkingDirectory(runtimeCfg)
	}

	req := workers.CommandRequest{
		Command:         resolvePortableFactoryScriptReference(runtimeCfg.FactoryDir(), workerDef.Command),
		Args:            resolvePortableFactoryScriptReferences(runtimeCfg.FactoryDir(), workerDef.Args),
		Env:             commandEnvWithResolvedVars(requestContext.EnvVars, resume),
		WorkDir:         workDir,
		WorkerType:      workerDef.Name,
		WorkstationName: workstation.Name,
	}
	return req, nil
}

func commandEnvWithResolvedVars(vars map[string]string, resume resumeCursor) []string {
	env := make([]string, 0, len(vars)+2)
	for key, value := range vars {
		env = append(env, key+"="+value)
	}
	if cursor := strings.TrimSpace(string(resume.cursor)); cursor != "" {
		env = append(env, scriptPollerCursorEnvVar+"="+cursor)
	}
	if checkpoint := strings.TrimSpace(resume.checkpoint); checkpoint != "" {
		env = append(env, scriptPollerCheckpointEnvVar+"="+checkpoint)
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

func resolveWorkerTemplateFields(
	resolveTemplates workers.TemplateFieldResolver,
	workingDirectory string,
	environment map[string]string,
	tokens []workers.Token,
	context *workers.Context,
	worktree string,
) (*workers.ResolvedTemplateFields, error) {
	if resolveTemplates == nil {
		env := make(map[string]string, len(environment))
		for key, value := range environment {
			env[key] = value
		}
		return &workers.ResolvedTemplateFields{
			WorkingDirectory: workingDirectory,
			Worktree:         worktree,
			Env:              env,
		}, nil
	}
	return resolveTemplates(workingDirectory, environment, tokens, context, worktree)
}

func pollerRuntimeWorkingDirectory(runtimeCfg factorydefinitions.RuntimeConfigLookup) string {
	if runtimeCfg == nil {
		return ""
	}
	baseDir := strings.TrimSpace(runtimeCfg.RuntimeBaseDir())
	if baseDir == "" {
		baseDir = strings.TrimSpace(runtimeCfg.FactoryDir())
	}
	if baseDir == "" {
		return ""
	}
	return filepath.Clean(baseDir)
}

func resolvePortableFactoryScriptReferences(factoryDir string, args []string) []string {
	if len(args) == 0 {
		return nil
	}
	resolved := make([]string, len(args))
	for i, arg := range args {
		resolved[i] = resolvePortableFactoryScriptReference(factoryDir, arg)
	}
	return resolved
}

func resolvePortableFactoryScriptReference(factoryDir, raw string) string {
	if strings.TrimSpace(factoryDir) == "" {
		return raw
	}

	trimmed := strings.TrimSpace(raw)
	normalized := filepath.ToSlash(trimmed)
	if !strings.HasPrefix(normalized, "factory/scripts/") {
		return raw
	}

	relativePath := strings.TrimPrefix(normalized, "factory/scripts/")
	if relativePath == "" {
		return raw
	}
	return filepath.Join(factoryDir, "scripts", filepath.FromSlash(relativePath))
}
