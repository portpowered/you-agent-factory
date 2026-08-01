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

// ResolvedBackendScope is the effective backend scope after load or generation.
type ResolvedBackendScope struct {
	BackendScopeID string
	Outcome        BackendScopeOutcome
	ConfigPath     string
}

// EnsureLocalBackendScope is a stateless compatibility helper for callers
// that still own explicit identity and persistence ports. Production
// composition should use Service.EnsureLocalBackendScope so the complete
// Operator Settings dependency set remains behind one root authority.
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

func persistBackendScopeID(
	files FileSystem,
	createTemp CreateTemporaryFile,
	encode ConfigEncoder,
	configPath string,
	config Config,
) error {
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
	if files == nil {
		return fmt.Errorf("operator settings filesystem is required")
	}
	if createTemp == nil {
		return fmt.Errorf("operator settings temporary-file creator is required")
	}
	data, err := encode(config)
	if err != nil {
		return err
	}
	dir := filepath.Dir(configPath)
	if err := files.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create system config directory %q: %w", dir, err)
	}
	tmp, err := createTemp(dir, filepath.Base(configPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create system config temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = files.Remove(tmpPath)
		}
	}()
	if written, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write system config temp file: %w", err)
	} else if written != len(data) {
		_ = tmp.Close()
		return fmt.Errorf("write system config temp file: short write: wrote %d of %d bytes", written, len(data))
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
	cleanup = false
	return nil
}

var localBackendScopePattern = regexp.MustCompile(
	`^local-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
)

// GenerateLocalBackendScopeID prefixes one generated UUID-shaped component for
// callers that only need to form a detached identity value.
func GenerateLocalBackendScopeID(generateID IDGenerator) string {
	if generateID == nil {
		return LocalBackendScopePrefix
	}
	return LocalBackendScopePrefix + generateID()
}

// IsLocalBackendScopeID reports whether value matches the local-<uuid> shape.
func IsLocalBackendScopeID(value string) bool {
	return localBackendScopePattern.MatchString(strings.TrimSpace(value))
}

func sanitizeBackendScopeSegment(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", "|", "-")
	return replacer.Replace(trimmed)
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
