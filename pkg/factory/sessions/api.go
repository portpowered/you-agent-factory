package factorysessions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// CurrentFactoryName is the domain identifier for the current Factory selector.
const CurrentFactoryName = "UNDEFINED"

// OpenRequest is the transport-independent request to discover, validate, or
// open a Factory Session from a folder.
type OpenRequest struct {
	FolderPath     string
	Target         *TargetRef
	ValidateOnly   bool
	InitNewFactory bool
}

// NewLiveSession constructs a registry entry for a started session.
func NewLiveSession(
	sessionID string,
	factoryDir string,
	folderPath string,
	executionBaseDir string,
	target TargetRef,
	handle any,
	isDefault bool,
	project string,
) *LiveSession {
	session := &LiveSession{
		ID: sessionID,
		SessionState: SessionState{
			FactoryDir:       factoryDir,
			FolderPath:       folderPath,
			ExecutionBaseDir: executionBaseDir,
		},
		Handle:    handle,
		IsDefault: isDefault,
		Project:   project,
		Target:    target,
	}
	EnsureRuntimeFactorySessionID(session)
	session.ResponseEvents = NewSessionResponseEventStore(CanonicalFactorySessionID(session))
	return session
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

// FactoryName derives the domain factory name for a session runtime config.
func FactoryName(rootDir string, runtimeCfg *factoryconfig.LoadedFactoryConfig) string {
	if runtimeCfg == nil {
		return CurrentFactoryName
	}
	factoryDir := runtimeCfg.FactoryDir()
	cleanRoot := filepath.Clean(rootDir)
	if SameFactoryDir(factoryDir, cleanRoot) {
		return CurrentFactoryName
	}
	if rootDir != "" && filepath.Dir(factoryDir) == cleanRoot {
		name := filepath.Base(factoryDir)
		if err := factoryconfig.ValidateNamedFactoryName(name); err == nil {
			return name
		}
	}
	cfg := runtimeCfg.FactoryConfig()
	if cfg != nil {
		if name := strings.TrimSpace(cfg.Name); name != "" {
			return name
		}
		if project := strings.TrimSpace(cfg.Project); project != "" {
			return project
		}
	}
	return "factory"
}

func stringPointerOrNil(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// ValidateInitNewFactoryNestedDir rejects init-new-factory when the canonical nested
// factory directory already exists with content that init cannot populate without
// overwrite or cleanup.
func ValidateInitNewFactoryNestedDir(resolvedFolder string) error {
	nestedFactoryDir := filepath.Join(resolvedFolder, interfaces.FactoryDir)
	info, err := os.Stat(nestedFactoryDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return NewValidationError(
			validationReasonUnreadable,
			"folderPath",
			fmt.Errorf("inspect nested factory directory %s: %w", nestedFactoryDir, err),
		)
	}
	if !info.IsDir() {
		return NewValidationError(
			validationReasonConflict,
			"folderPath",
			fmt.Errorf(
				"cannot initialize factory scaffold: %q exists and is not a directory",
				nestedFactoryDir,
			),
		)
	}

	entries, err := os.ReadDir(nestedFactoryDir)
	if err != nil {
		return NewValidationError(
			validationReasonUnreadable,
			"folderPath",
			fmt.Errorf("read nested factory directory %s: %w", nestedFactoryDir, err),
		)
	}
	if len(entries) > 0 {
		return NewValidationError(
			validationReasonConflict,
			"folderPath",
			fmt.Errorf(
				"cannot initialize factory scaffold: %q already exists with conflicting content",
				nestedFactoryDir,
			),
		)
	}
	return nil
}
