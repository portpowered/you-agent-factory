package operatorsettings

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// LocalBackendScopePrefix is the required prefix for generated local backend scopes.
	LocalBackendScopePrefix = "local-"
)

// BackendScopeOutcome reports whether a backend scope was reused or generated.
type BackendScopeOutcome string

const (
	BackendScopeOutcomeReused    BackendScopeOutcome = "reused"
	BackendScopeOutcomeGenerated BackendScopeOutcome = "generated"
)

var localBackendScopePattern = regexp.MustCompile(
	`^local-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
)

// ResolvedBackendScope is the effective backend scope after load or generation.
type ResolvedBackendScope struct {
	BackendScopeID string
	Outcome        BackendScopeOutcome
	ConfigPath     string
}

// EnsureLocalBackendScope loads backendScopeID from configPath, generates
// local-<uuid> when missing, and persists a newly generated value before returning.
func EnsureLocalBackendScope(
	files FileSystem,
	createTemp CreateTemporaryFile,
	generateID IDGenerator,
	decode ConfigDecoder,
	encode ConfigEncoder,
	configPath string,
) (ResolvedBackendScope, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return ResolvedBackendScope{}, fmt.Errorf("system config path is required to resolve backend scope")
	}

	config, err := LoadFileConfig(files, decode, configPath)
	if err != nil {
		return ResolvedBackendScope{}, err
	}
	if trimmed := strings.TrimSpace(config.BackendScopeID); trimmed != "" {
		return ResolvedBackendScope{
			BackendScopeID: trimmed,
			Outcome:        BackendScopeOutcomeReused,
			ConfigPath:     configPath,
		}, nil
	}

	generated := GenerateLocalBackendScopeID(generateID)
	config.BackendScopeID = generated
	if err := persistBackendScopeID(files, createTemp, encode, configPath, config); err != nil {
		return ResolvedBackendScope{}, fmt.Errorf(
			"persist generated backend scope ID to system config %q: %w; local backends require a stable backendScopeID before exposing session identity",
			configPath,
			err,
		)
	}
	return ResolvedBackendScope{
		BackendScopeID: generated,
		Outcome:        BackendScopeOutcomeGenerated,
		ConfigPath:     configPath,
	}, nil
}

// GenerateLocalBackendScopeID returns a new local backend scope identifier.
func GenerateLocalBackendScopeID(generateID IDGenerator) string {
	return LocalBackendScopePrefix + generateID()
}

// IsLocalBackendScopeID reports whether value matches the local-<uuid> shape.
func IsLocalBackendScopeID(value string) bool {
	return localBackendScopePattern.MatchString(strings.TrimSpace(value))
}

// DiagnosticsLine returns a redacted diagnostic line for backend scope resolution.
func (r ResolvedBackendScope) DiagnosticsLine() string {
	return fmt.Sprintf(
		"backendScope outcome=%s backendScopeID=%s configPath=%q",
		r.Outcome,
		diagnosticsBackendScopeID(r.BackendScopeID),
		r.ConfigPath,
	)
}

func diagnosticsBackendScopeID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unset"
	}
	return trimmed
}

func persistBackendScopeID(files FileSystem, createTemp CreateTemporaryFile, encode ConfigEncoder, configPath string, config Config) error {
	backendScopeID := strings.TrimSpace(config.BackendScopeID)
	if backendScopeID == "" {
		return fmt.Errorf("backend scope ID is required")
	}
	if !IsLocalBackendScopeID(backendScopeID) {
		return fmt.Errorf("backend scope ID %q is not a valid local backend scope", backendScopeID)
	}

	if encode == nil {
		return fmt.Errorf("global config encoder is required")
	}
	data, err := encode(config)
	if err != nil {
		return err
	}
	return writeConfig(files, createTemp, configPath, data)
}

func writeConfig(files FileSystem, createTemp CreateTemporaryFile, configPath string, data []byte) error {
	dir := filepath.Dir(configPath)
	if err := files.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create system config directory %q: %w", dir, err)
	}

	tmp, err := createTemp(dir, filepath.Base(configPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create system config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = files.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write system config temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync system config temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close system config temp file: %w", err)
	}
	if err := files.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("set system config temp file permissions: %w", err)
	}
	if err := files.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("replace system config with temp file: %w", err)
	}
	cleanupTemp = false
	return nil
}
