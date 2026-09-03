package workflowsource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript/validation"
)

const (
	CodeSourceNotFound  = "workflow.source.notFound"
	CodeSourceConflict  = "workflow.source.conflict"
	CodeUnsupportedKind = "workflow.source.unsupportedKind"
)

type lookupCandidate struct {
	stage         LookupStage
	factoryName   string
	sourceRef     string
	filePath      string
	content       string
	agents        map[string]interfaces.FactoryOrchestratorJavaScriptAgent
	argsSchema    json.RawMessage
	defaultPolicy json.RawMessage
}

func lookupWorkflowByName(ctx Context, name string, allowFactoryLookup bool) (Resolution, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return notFoundResolution(KindWorkflowName, name, "workflow name is empty"), true
	}

	for _, stage := range []struct {
		stage  LookupStage
		root   string
		prefix string
	}{
		{LookupStageProjectClaude, ctx.ProjectWorkflowRoot, ProjectClaudeWorkflowsDir + "/"},
		{LookupStageGlobalUser, ctx.GlobalWorkflowRoot, "global:" + GlobalWorkflowsDirName + "/"},
		{LookupStagePackageRelative, filepath.Join(ctx.PackageRoot, interfaces.WorkflowsDir), interfaces.FactoryDir + "/" + interfaces.WorkflowsDir + "/"},
	} {
		if filePath, sourceRef, ok := findWorkflowFile(ctx.files, stage.root, stage.prefix, trimmed); ok {
			return resolutionFromCandidate(ctx.files, KindWorkflowName, name, lookupCandidate{
				stage:     stage.stage,
				sourceRef: sourceRef,
				filePath:  filePath,
			}), true
		}
	}

	if factoryCandidates := collectNamedJavaScriptFactoryCandidates(ctx, trimmed); len(factoryCandidates) == 1 {
		return resolutionFromCandidate(ctx.files, KindWorkflowName, name, factoryCandidates[0]), true
	} else if len(factoryCandidates) > 1 {
		return conflictResolution(KindWorkflowName, name, factoryCandidates), true
	}

	if allowFactoryLookup {
		if resolution, ok := resolveFactoryID(ctx, trimmed); ok {
			return resolution, true
		}
	}

	return notFoundResolution(KindWorkflowName, name, fmt.Sprintf("workflow %q was not found in project, global, package-relative, or JavaScript factory lookup paths", trimmed)), true
}

func findWorkflowFile(files fileSystem, rootDir, sourcePrefix, name string) (filePath, sourceRef string, ok bool) {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return "", "", false
	}
	for _, candidateName := range workflowFileNames(name) {
		fullPath := filepath.Join(rootDir, candidateName)
		info, err := files.Stat(fullPath)
		if err != nil || info.IsDir() {
			continue
		}
		return fullPath, sourcePrefix + filepath.ToSlash(candidateName), true
	}
	return "", "", false
}

func workflowFileNames(name string) []string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	if ext := strings.ToLower(filepath.Ext(trimmed)); ext != "" {
		return []string{trimmed}
	}
	return []string{
		trimmed + ".js",
		trimmed + ".ts",
		trimmed + ".workflow.js",
		trimmed + ".workflow.ts",
		trimmed + ".mjs",
		trimmed + ".mts",
	}
}

func collectNamedJavaScriptFactoryCandidates(ctx Context, name string) []lookupCandidate {
	var out []lookupCandidate
	for _, root := range []string{ctx.ProjectFactoryRoot, ctx.GlobalFactoryRoot} {
		entries, err := listNamedJavaScriptFactories(ctx.files, root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			candidate, ok := namedJavaScriptFactoryCandidate(ctx.files, entry.Name, entry.FactoryDir, name)
			if ok {
				out = append(out, candidate)
			}
		}
	}
	return out
}

func namedJavaScriptFactoryCandidate(files fileSystem, factoryName, factoryDir, workflowName string) (lookupCandidate, bool) {
	cfg, err := readFactoryOrchestratorConfig(files, factoryDir)
	if err != nil || cfg == nil || !interfaces.IsJavaScriptOrchestratorFactory(cfg) {
		return lookupCandidate{}, false
	}
	jsCfg := cfg.Orchestrator.JavaScript
	if jsCfg == nil {
		return lookupCandidate{}, false
	}

	canonicalFactoryName := strings.TrimSpace(factoryName)
	trimmedWorkflowName := strings.TrimSpace(workflowName)
	if canonicalFactoryName == trimmedWorkflowName {
		return factoryWorkflowCandidate(files, factoryName, factoryDir, jsCfg)
	}

	sourceRef := strings.TrimSpace(jsCfg.SourceRef)
	if sourceRef == "" {
		return lookupCandidate{}, false
	}
	base := strings.TrimSuffix(filepath.Base(sourceRef), filepath.Ext(sourceRef))
	if base != trimmedWorkflowName && filepath.Base(sourceRef) != trimmedWorkflowName {
		return lookupCandidate{}, false
	}
	return factoryWorkflowCandidate(files, factoryName, factoryDir, jsCfg)
}

func factoryWorkflowCandidate(files fileSystem, factoryName, factoryDir string, jsCfg *interfaces.FactoryOrchestratorJavaScriptConfig) (lookupCandidate, bool) {
	if jsCfg.InlineSource != nil && strings.TrimSpace(jsCfg.InlineSource.Inline) != "" {
		content := jsCfg.InlineSource.Inline
		return lookupCandidate{
			stage:         LookupStageNamedJavaScript,
			factoryName:   factoryName,
			sourceRef:     fmt.Sprintf("factory:%s:inline", factoryName),
			content:       content,
			agents:        jsCfg.Agents,
			argsSchema:    append(json.RawMessage(nil), jsCfg.ArgsSchema...),
			defaultPolicy: append(json.RawMessage(nil), jsCfg.DefaultPolicy...),
		}, true
	}

	sourceRef := strings.TrimSpace(jsCfg.SourceRef)
	if sourceRef == "" {
		return lookupCandidate{}, false
	}
	reader := workflowvalidation.FileSourceReader(factoryDir, files)
	content, err := reader.ReadWorkflowSource(sourceRef)
	if err != nil {
		return lookupCandidate{}, false
	}
	return lookupCandidate{
		stage:         LookupStageNamedJavaScript,
		factoryName:   factoryName,
		sourceRef:     fmt.Sprintf("factory:%s:%s", factoryName, filepath.ToSlash(sourceRef)),
		filePath:      filepath.Join(factoryDir, filepath.FromSlash(sourceRef)),
		content:       content,
		agents:        jsCfg.Agents,
		argsSchema:    append(json.RawMessage(nil), jsCfg.ArgsSchema...),
		defaultPolicy: append(json.RawMessage(nil), jsCfg.DefaultPolicy...),
	}, true
}

func readFactoryOrchestratorConfig(files fileSystem, factoryDir string) (*interfaces.FactoryConfig, error) {
	payload, err := files.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		return nil, err
	}
	var cfg interfaces.FactoryConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, fmt.Errorf("decode Factory Definition %s: %w", factoryDir, err)
	}
	if cfg.Orchestrator != nil && cfg.Orchestrator.JavaScript != nil {
		cfg.Orchestrator.JavaScript.ArgsSchema = compactJSON(cfg.Orchestrator.JavaScript.ArgsSchema)
		cfg.Orchestrator.JavaScript.DefaultPolicy = compactJSON(cfg.Orchestrator.JavaScript.DefaultPolicy)
	}
	return &cfg, nil
}

func compactJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return append(json.RawMessage(nil), value...)
	}
	return append(json.RawMessage(nil), compact.Bytes()...)
}

func resolveWorkflowFile(ctx Context, fileRef string) (Resolution, bool) {
	trimmed := strings.TrimSpace(fileRef)
	if trimmed == "" {
		return notFoundResolution(KindWorkflowFile, fileRef, "workflow file reference is empty"), true
	}

	if resolution, ok := resolveExplicitWorkflowFileRef(ctx, trimmed); ok {
		return resolution, true
	}

	if filepath.IsAbs(trimmed) {
		content, err := ctx.files.ReadFile(trimmed)
		if err != nil {
			return notFoundResolution(KindWorkflowFile, fileRef, fmt.Sprintf("workflow file %q is not readable: %v", trimmed, err)), true
		}
		loaded, issues := workflowvalidation.Load(workflowvalidation.LoadRequest{
			SourceRef: filepath.Base(trimmed),
			Content:   string(content),
		})
		if len(issues) > 0 {
			return resolutionWithLoadIssues(KindWorkflowFile, fileRef, LookupStageExplicitSourceKind, filepath.Base(trimmed), issues), true
		}
		return Resolution{
			RequestKind:      KindWorkflowFile,
			RequestValue:     fileRef,
			ResolvedKind:     KindWorkflowFile,
			LookupStage:      LookupStageExplicitSourceKind,
			SourceRef:        filepath.Base(trimmed),
			SourceHash:       loaded.SourceHash,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          dialectForLoaded(loaded),
			Content:          string(content),
			Found:            true,
		}, true
	}

	for _, stage := range []struct {
		stage  LookupStage
		root   string
		prefix string
	}{
		{LookupStageProjectClaude, ctx.ProjectWorkflowRoot, ProjectClaudeWorkflowsDir + "/"},
		{LookupStageGlobalUser, ctx.GlobalWorkflowRoot, "global:" + GlobalWorkflowsDirName + "/"},
		{LookupStagePackageRelative, filepath.Join(ctx.PackageRoot, interfaces.WorkflowsDir), interfaces.FactoryDir + "/" + interfaces.WorkflowsDir + "/"},
	} {
		fullPath := filepath.Join(stage.root, filepath.FromSlash(trimmed))
		content, err := ctx.files.ReadFile(fullPath)
		if err != nil {
			continue
		}
		loaded, issues := workflowvalidation.Load(workflowvalidation.LoadRequest{
			SourceRef: trimmed,
			Content:   string(content),
		})
		if len(issues) > 0 {
			return resolutionWithLoadIssues(KindWorkflowFile, fileRef, stage.stage, stage.prefix+filepath.ToSlash(trimmed), issues), true
		}
		return Resolution{
			RequestKind:      KindWorkflowFile,
			RequestValue:     fileRef,
			ResolvedKind:     KindWorkflowFile,
			LookupStage:      stage.stage,
			SourceRef:        stage.prefix + filepath.ToSlash(trimmed),
			SourceHash:       loaded.SourceHash,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          dialectForLoaded(loaded),
			Content:          string(content),
			Found:            true,
		}, true
	}

	return notFoundResolution(KindWorkflowFile, fileRef, fmt.Sprintf("workflow file %q was not found", trimmed)), true
}

func resolveExplicitWorkflowFileRef(ctx Context, trimmed string) (Resolution, bool) {
	readers := []struct {
		stage     LookupStage
		root      string
		sourceRef string
	}{
		{LookupStageExplicitSourceKind, ctx.ProjectRoot, filepath.ToSlash(trimmed)},
		{LookupStagePackageRelative, ctx.PackageRoot, interfaces.FactoryDir + "/" + filepath.ToSlash(trimmed)},
	}
	for _, readerCfg := range readers {
		reader := workflowvalidation.FileSourceReader(readerCfg.root, ctx.files)
		content, err := reader.ReadWorkflowSource(trimmed)
		if err != nil {
			continue
		}
		loaded, issues := workflowvalidation.Load(workflowvalidation.LoadRequest{
			SourceRef: readerCfg.sourceRef,
			Content:   content,
		})
		if len(issues) > 0 {
			return resolutionWithLoadIssues(KindWorkflowFile, trimmed, readerCfg.stage, readerCfg.sourceRef, issues), true
		}
		return Resolution{
			RequestKind:      KindWorkflowFile,
			RequestValue:     trimmed,
			ResolvedKind:     KindWorkflowFile,
			LookupStage:      readerCfg.stage,
			SourceRef:        readerCfg.sourceRef,
			SourceHash:       loaded.SourceHash,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          dialectForLoaded(loaded),
			Content:          content,
			Found:            true,
		}, true
	}
	return Resolution{}, false
}

func resolveFactoryID(ctx Context, factoryID string) (Resolution, bool) {
	trimmed := strings.TrimSpace(factoryID)
	if trimmed == "" {
		return notFoundResolution(KindFactoryID, factoryID, "factory id is empty"), true
	}

	factoryDir, err := resolveNamedJavaScriptFactoryDir(ctx.files, ctx.ProjectFactoryRoot, ctx.GlobalFactoryRoot, trimmed)
	if err != nil {
		return notFoundResolution(KindFactoryID, factoryID, fmt.Sprintf("factory %q was not found: %v", trimmed, err)), true
	}

	cfg, err := readFactoryOrchestratorConfig(ctx.files, factoryDir)
	if err != nil {
		return notFoundResolution(KindFactoryID, factoryID, fmt.Sprintf("factory %q is not readable: %v", trimmed, err)), true
	}
	if !interfaces.IsJavaScriptOrchestratorFactory(cfg) || cfg.Orchestrator.JavaScript == nil {
		return notFoundResolution(KindFactoryID, factoryID, fmt.Sprintf("factory %q is not a JavaScript workflow factory", trimmed)), true
	}

	candidate, ok := factoryWorkflowCandidate(ctx.files, trimmed, factoryDir, cfg.Orchestrator.JavaScript)
	if !ok {
		return notFoundResolution(KindFactoryID, factoryID, fmt.Sprintf("factory %q does not declare workflow source identity", trimmed)), true
	}
	candidate.stage = LookupStageExplicitFactory
	res := resolutionFromCandidate(ctx.files, KindFactoryID, factoryID, candidate)
	return res, true
}

func resolveFactoryInline(value string) (Resolution, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return notFoundResolution(KindFactoryInline, value, "inline Factory definition is empty"), true
	}
	var cfg interfaces.FactoryConfig
	if err := json.Unmarshal([]byte(trimmed), &cfg); err != nil {
		return notFoundResolution(
			KindFactoryInline,
			value,
			fmt.Sprintf("inline Factory definition is invalid JSON: %v", err),
		), true
	}
	if !interfaces.IsJavaScriptOrchestratorFactory(&cfg) || cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil {
		return notFoundResolution(
			KindFactoryInline,
			value,
			"inline Factory definition is not a JavaScript workflow Factory",
		), true
	}
	js := cfg.Orchestrator.JavaScript
	if js.InlineSource == nil || strings.TrimSpace(js.InlineSource.Inline) == "" {
		return notFoundResolution(
			KindFactoryInline,
			value,
			"inline Factory definition must carry JavaScript inlineSource",
		), true
	}
	factoryName := strings.TrimSpace(cfg.Name)
	return resolutionFromCandidate(nil, KindFactoryInline, value, lookupCandidate{
		stage:         LookupStageExplicitSourceKind,
		factoryName:   factoryName,
		sourceRef:     fmt.Sprintf("factory:%s:inline", factoryName),
		content:       js.InlineSource.Inline,
		agents:        js.Agents,
		argsSchema:    append(json.RawMessage(nil), js.ArgsSchema...),
		defaultPolicy: append(json.RawMessage(nil), js.DefaultPolicy...),
	}), true
}

func resolveInlineSource(kind Kind, value, inline string) (Resolution, bool) {
	content := strings.TrimSpace(inline)
	if content == "" {
		content = strings.TrimSpace(value)
	}
	if content == "" {
		return notFoundResolution(kind, value, "inline workflow source is empty"), true
	}

	loaded, issues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: "inline",
		Content:   content,
	})
	if len(issues) > 0 {
		return resolutionWithLoadIssues(kind, value, LookupStageExplicitSourceKind, "inline", issues), true
	}
	factoryName := ""
	if kind == KindFactoryInline {
		var declaration struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(value), &declaration); err == nil {
			factoryName = strings.TrimSpace(declaration.Name)
		}
	}

	return Resolution{
		RequestKind:      kind,
		RequestValue:     value,
		ResolvedKind:     kind,
		LookupStage:      LookupStageExplicitSourceKind,
		FactoryName:      factoryName,
		SourceRef:        "inline",
		SourceHash:       loaded.SourceHash,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          dialectForLoaded(loaded),
		Content:          content,
		Found:            true,
	}, true
}

func resolutionFromCandidate(files fileSystem, requestKind Kind, requestValue string, candidate lookupCandidate) Resolution {
	content := candidate.content
	if content == "" && candidate.filePath != "" {
		body, err := files.ReadFile(candidate.filePath)
		if err != nil {
			return notFoundResolution(requestKind, requestValue, fmt.Sprintf("workflow source %q is not readable: %v", candidate.sourceRef, err))
		}
		content = string(body)
	}

	loaded, issues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: candidate.sourceRef,
		Content:   content,
	})
	if len(issues) > 0 {
		return resolutionWithLoadIssues(requestKind, requestValue, candidate.stage, candidate.sourceRef, issues)
	}

	return Resolution{
		RequestKind:      requestKind,
		RequestValue:     requestValue,
		ResolvedKind:     KindWorkflowFile,
		LookupStage:      candidate.stage,
		FactoryName:      strings.TrimSpace(candidate.factoryName),
		SourceRef:        candidate.sourceRef,
		SourceHash:       loaded.SourceHash,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          dialectForLoaded(loaded),
		Content:          content,
		Agents:           candidate.agents,
		ArgsSchema:       append(json.RawMessage(nil), candidate.argsSchema...),
		DefaultPolicy:    append(json.RawMessage(nil), candidate.defaultPolicy...),
		Found:            true,
	}
}

func resolutionWithLoadIssues(requestKind Kind, requestValue string, stage LookupStage, sourceRef string, issues []workflowvalidation.Issue) Resolution {
	diagnostics := make([]Diagnostic, 0, len(issues))
	for _, issue := range issues {
		diagnostics = append(diagnostics, Diagnostic{
			Code:    issue.Code,
			Message: issue.Message,
		})
	}
	return Resolution{
		RequestKind:      requestKind,
		RequestValue:     requestValue,
		ResolvedKind:     requestKind,
		LookupStage:      stage,
		SourceRef:        sourceRef,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Diagnostics:      diagnostics,
		Found:            false,
	}
}

func notFoundResolution(requestKind Kind, requestValue, message string) Resolution {
	return Resolution{
		RequestKind:      requestKind,
		RequestValue:     requestValue,
		ResolvedKind:     requestKind,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Diagnostics: []Diagnostic{{
			Code:    CodeSourceNotFound,
			Message: message,
		}},
		Found: false,
	}
}

func conflictResolution(requestKind Kind, requestValue string, candidates []lookupCandidate) Resolution {
	refs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		refs = append(refs, candidate.sourceRef)
	}
	return Resolution{
		RequestKind:      requestKind,
		RequestValue:     requestValue,
		ResolvedKind:     requestKind,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Diagnostics: []Diagnostic{{
			Code:    CodeSourceConflict,
			Message: fmt.Sprintf("workflow %q matched multiple sources: %s", requestValue, strings.Join(refs, ", ")),
		}},
		Found: false,
	}
}

func dialectForLoaded(loaded workflowvalidation.LoadedSource) string {
	switch strings.TrimSpace(loaded.Format) {
	case workflowvalidation.FormatTypeScript:
		return "typescript"
	default:
		return defaultDialect
	}
}
