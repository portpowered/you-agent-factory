package factorysessions

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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
	clock factory.Clock,
	generateSessionID SessionIDGenerator,
	eventIDs ResponseEventIDGenerator,
) *LiveSession {
	if clock == nil || generateSessionID == nil || eventIDs == nil {
		return nil
	}
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
	if err := EnsureRuntimeFactorySessionID(session, generateSessionID); err != nil {
		return nil
	}
	session.ResponseEvents = NewSessionResponseEventStore(CanonicalFactorySessionID(session), clock, eventIDs)
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
	if session.FactoryDir == "" || !sameFactoryDir(session.FactoryDir, session.FolderPath) {
		return rootDir
	}
	serviceRoot := filepath.Clean(serviceRootDir)
	if serviceRoot != "" && filepath.Dir(session.FactoryDir) == serviceRoot {
		return serviceRoot
	}
	return rootDir
}

// SessionFactoryPersistRoot resolves the on-disk definition persistence root
// without requiring Factory Sessions to call a Factory Definition
// implementation package.
func SessionFactoryPersistRoot(serviceRootDir string, session *LiveSession) string {
	if session != nil && !session.IsDefault && strings.TrimSpace(session.FolderPath) != "" {
		return session.FolderPath
	}
	return SessionFactoryRootDir(serviceRootDir, session)
}

// FactoryName derives the domain factory name for a session runtime config.
func FactoryName(rootDir string, runtimeCfg interfaces.RuntimeConfigLookup) string {
	if runtimeCfg == nil {
		return CurrentFactoryName
	}
	factoryDir := runtimeCfg.FactoryDir()
	cleanRoot := filepath.Clean(rootDir)
	if sameFactoryDir(factoryDir, cleanRoot) {
		return CurrentFactoryName
	}
	if rootDir != "" && filepath.Dir(factoryDir) == cleanRoot {
		name := filepath.Base(factoryDir)
		if _, err := interfaces.PathSegments(name); err == nil {
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

func sameFactoryDir(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
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
func ValidateInitNewFactoryNestedDir(
	resolvedFolder string,
	directories DirectoryInspection,
) error {
	if directories == nil {
		return NewValidationError(
			validationReasonUnreadable,
			"folderPath",
			fmt.Errorf("inspect nested factory directory: directory inspection is required"),
		)
	}
	nestedFactoryDir := filepath.Join(resolvedFolder, interfaces.FactoryDir)
	info, err := directories.Stat(nestedFactoryDir)
	if errors.Is(err, fs.ErrNotExist) {
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

	entries, err := directories.ReadDir(nestedFactoryDir)
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
