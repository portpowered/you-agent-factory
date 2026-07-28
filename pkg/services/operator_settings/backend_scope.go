package operatorsettings

import (
	"fmt"
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

// IdentityInputInventoryOperations wires private identity and input-inventory
// behavior into the published Operator Settings root without importing the
// private implementation package from the peer root.
type IdentityInputInventoryOperations struct {
	EnsureLocalBackendScope func(
		FileSystem,
		CreateTemporaryFile,
		IDGenerator,
		ConfigDecoder,
		ConfigEncoder,
		string,
	) (ResolvedBackendScope, error)
	ProjectInputInventory func() InputInventory
}

var identityInputInventoryOperations IdentityInputInventoryOperations

// ConfigureIdentityInputInventoryOperations registers private identity and
// input-inventory behavior for the published Operator Settings root surface.
func ConfigureIdentityInputInventoryOperations(operations IdentityInputInventoryOperations) {
	identityInputInventoryOperations = operations
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
	if identityInputInventoryOperations.EnsureLocalBackendScope == nil {
		panic("operator settings identity input inventory operations are required")
	}
	return identityInputInventoryOperations.EnsureLocalBackendScope(
		files,
		createTemp,
		generateID,
		decode,
		encode,
		configPath,
	)
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
