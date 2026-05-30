package factorysessions

import (
	"path/filepath"
	"sort"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

// ListSummaries builds API session summaries from registered live sessions.
func ListSummaries(registry *Registry) []factoryapi.FactorySessionSummary {
	if registry == nil {
		return nil
	}
	sessionIDs := registry.IDs()
	summaries := make([]factoryapi.FactorySessionSummary, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		session := registry.Get(sessionID)
		if session == nil {
			continue
		}
		summaries = append(summaries, SummaryResponse(session))
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].IsDefault != summaries[j].IsDefault {
			return summaries[i].IsDefault
		}
		return summaries[i].Id < summaries[j].Id
	})
	return summaries
}

// SummaryResponse maps a live session to the API summary shape.
func SummaryResponse(session *LiveSession) factoryapi.FactorySessionSummary {
	return factoryapi.FactorySessionSummary{
		FactoryDir: session.FactoryDir,
		FolderPath: session.FolderPath,
		Id:         session.ID,
		IsDefault:  session.IsDefault,
		Project:    session.Project,
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKind(session.Target.Kind),
			Name: stringPointerOrNil(session.Target.Name),
		},
	}
}

// TargetResponse maps a discovered target to the API target shape.
func TargetResponse(target Target) factoryapi.FactorySessionTarget {
	return factoryapi.FactorySessionTarget{
		FactoryDir: target.FactoryDir,
		FolderPath: target.FolderPath,
		Label:      target.Label,
		Project:    target.Project,
		Ref: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKind(target.Ref.Kind),
			Name: stringPointerOrNil(target.Ref.Name),
		},
	}
}

// TargetsResponse maps discovered targets to API targets.
func TargetsResponse(targets []Target) []factoryapi.FactorySessionTarget {
	if len(targets) == 0 {
		return nil
	}
	response := make([]factoryapi.FactorySessionTarget, 0, len(targets))
	for _, target := range targets {
		response = append(response, TargetResponse(target))
	}
	return response
}

// NewLiveSession constructs a registry entry for a started session.
func NewLiveSession(
	sessionID string,
	factoryDir string,
	folderPath string,
	target TargetRef,
	handle any,
	isDefault bool,
	project string,
) *LiveSession {
	return &LiveSession{
		ID:         sessionID,
		FactoryDir: factoryDir,
		FolderPath: folderPath,
		Handle:     handle,
		IsDefault:  isDefault,
		Project:    project,
		Target:     target,
	}
}

// SessionFactoryRootDir resolves the editable-definition root for a live session.
func SessionFactoryRootDir(serviceRootDir string, session *LiveSession) string {
	if session == nil {
		return ""
	}
	rootDir := session.FolderPath
	if session.FolderPath == "" {
		return rootDir
	}
	if session.FactoryDir == "" || !SameFactoryDir(session.FactoryDir, session.FolderPath) {
		return rootDir
	}
	serviceRoot := filepath.Clean(serviceRootDir)
	if serviceRoot != "" && filepath.Dir(session.FactoryDir) == serviceRoot {
		return serviceRoot
	}
	return rootDir
}

// FactoryName derives the API factory name for a session runtime config.
func FactoryName(rootDir string, runtimeCfg *factoryconfig.LoadedFactoryConfig) factoryapi.FactoryName {
	if runtimeCfg == nil {
		return apisurface.DefaultCurrentFactoryName
	}
	factoryDir := runtimeCfg.FactoryDir()
	cleanRoot := filepath.Clean(rootDir)
	if SameFactoryDir(factoryDir, cleanRoot) {
		return apisurface.DefaultCurrentFactoryName
	}
	if rootDir != "" && filepath.Dir(factoryDir) == cleanRoot {
		name := filepath.Base(factoryDir)
		if err := factoryconfig.ValidateNamedFactoryName(name); err == nil {
			return factoryapi.FactoryName(name)
		}
	}
	cfg := runtimeCfg.FactoryConfig()
	if cfg != nil {
		if name := strings.TrimSpace(cfg.Name); name != "" {
			return factoryapi.FactoryName(name)
		}
		if project := strings.TrimSpace(cfg.Project); project != "" {
			return factoryapi.FactoryName(project)
		}
	}
	return factoryapi.FactoryName("factory")
}

func stringPointerOrNil(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
