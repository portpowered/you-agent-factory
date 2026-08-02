package operatorsettings

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrResolutionInvalidInput reports that effective-resolution inputs are
// incomplete or cannot be resolved (for example unresolved symbolic DEFAULT).
var ErrResolutionInvalidInput = errors.New("operator effective resolution input is invalid")

// ErrResolutionUnsupportedOverride reports that an override layer supplied a
// value the operator settings contract does not support.
var ErrResolutionUnsupportedOverride = errors.New("operator effective resolution override is unsupported")

// ErrResolutionConflict reports that supplied baseline or override facts
// conflict and cannot be reconciled without mutating the operator document.
var ErrResolutionConflict = errors.New("operator effective resolution conflict")

// ResolutionFailureKind classifies effective-resolution failures peers can
// branch on with errors.Is / errors.As.
type ResolutionFailureKind string

const (
	ResolutionFailureKindInvalidInput        ResolutionFailureKind = "invalid_input"
	ResolutionFailureKindUnsupportedOverride ResolutionFailureKind = "unsupported_override"
	ResolutionFailureKindConflict            ResolutionFailureKind = "conflict"
)

// ResolutionFailure retains normalized effective-resolution failure facts
// without exposing storage, codec, or lifecycle construction ports.
type ResolutionFailure struct {
	Kind    ResolutionFailureKind
	Message string
	Field   string
}

func (failure ResolutionFailure) Error() string {
	message := strings.TrimSpace(failure.Message)
	field := strings.TrimSpace(failure.Field)
	switch {
	case message != "" && field != "":
		return fmt.Sprintf("%s: %s (%s)", sentinelForResolutionFailureKind(failure.Kind).Error(), message, field)
	case message != "":
		return fmt.Sprintf("%s: %s", sentinelForResolutionFailureKind(failure.Kind).Error(), message)
	case field != "":
		return fmt.Sprintf("%s (%s)", sentinelForResolutionFailureKind(failure.Kind).Error(), field)
	default:
		return sentinelForResolutionFailureKind(failure.Kind).Error()
	}
}

func (failure ResolutionFailure) Unwrap() error {
	return sentinelForResolutionFailureKind(failure.Kind)
}

func sentinelForResolutionFailureKind(kind ResolutionFailureKind) error {
	switch kind {
	case ResolutionFailureKindInvalidInput:
		return ErrResolutionInvalidInput
	case ResolutionFailureKindUnsupportedOverride:
		return ErrResolutionUnsupportedOverride
	case ResolutionFailureKindConflict:
		return ErrResolutionConflict
	default:
		return ErrResolutionInvalidInput
	}
}

// EffectivePrecedenceChain describes the operator default precedence order
// for diagnostics on effective selections.
const EffectivePrecedenceChain = "file < env < flag"

// EffectiveLayerSource identifies which precedence layer supplied one effective
// default field after resolution.
type EffectiveLayerSource string

const (
	EffectiveLayerSourceFile EffectiveLayerSource = "file"
	EffectiveLayerSourceEnv  EffectiveLayerSource = "env"
	EffectiveLayerSourceFlag EffectiveLayerSource = "flag"
)

// EffectiveOverrideFacts carries invocation or environment override values as
// detached plain facts. Empty fields are treated as unset overrides.
type EffectiveOverrideFacts struct {
	WorkerModelProvider string
	WorkerModel         string
	WorkerPresetID      string
}

// Clone returns a detached override-facts copy.
func (facts EffectiveOverrideFacts) Clone() EffectiveOverrideFacts {
	return facts
}

// EffectiveSelection is the immutable effective operator default selection
// peers consume from effective resolution without owning precedence rules.
type EffectiveSelection struct {
	BackendScopeID            string
	WorkerPresets             []DocumentWorkerPreset
	WorkerModelProvider       string
	WorkerModel               string
	WorkerModelProviderSource EffectiveLayerSource
	WorkerModelSource         EffectiveLayerSource
	ConfigPath                string
}

// Clone returns a detached effective-selection copy.
func (selection EffectiveSelection) Clone() EffectiveSelection {
	cloned := selection
	if selection.WorkerPresets != nil {
		cloned.WorkerPresets = cloneDocumentWorkerPresets(selection.WorkerPresets)
	}
	return cloned
}

// ResolveEffectiveRequest asks for an effective selection from detached
// document baseline and override facts. Resolution does not mutate the operator
// document; document mutation remains on document operations.
type ResolveEffectiveRequest struct {
	DocumentBaseline         DocumentDefaults
	BackendScopeID           string
	WorkerPresets            []DocumentWorkerPreset
	ExpectedDocumentBaseline *DocumentDefaults
	EnvironmentOverrides     EffectiveOverrideFacts
	InvocationOverrides      EffectiveOverrideFacts
	ConfigPath               string
}

// Validate checks request fields whose validity does not depend on resolution
// state. Baseline mismatch against ExpectedDocumentBaseline fails before
// precedence resolution.
func (request ResolveEffectiveRequest) Validate() error {
	if request.ExpectedDocumentBaseline == nil {
		return nil
	}
	expected := request.ExpectedDocumentBaseline
	if expected.WorkerModelProvider != request.DocumentBaseline.WorkerModelProvider ||
		expected.WorkerModel != request.DocumentBaseline.WorkerModel {
		return ResolutionFailure{
			Kind:    ResolutionFailureKindConflict,
			Message: "document baseline mismatch",
			Field:   "documentBaseline",
		}
	}
	return nil
}

// ResolveEffectiveResult is the detached outcome of one effective-resolution
// operation.
type ResolveEffectiveResult struct {
	Selection EffectiveSelection
}

// DefaultACPAgentFactoryReference is the safe built-in Factory reference used
// as both the effective default and the sole allowlist entry when no ACP
// agent profile has been authored.
const DefaultACPAgentFactoryReference = "@you/factory-builder"

// ErrACPAgentProfileInvalid reports that a candidate or stored ACP agent
// profile is not a valid detached ACPAgentProfile value.
var ErrACPAgentProfileInvalid = errors.New("ACP agent profile is invalid")

// ErrACPAgentProfilePersistFailed reports that a validated ACP agent profile
// could not be read from or atomically published to its owned storage.
var ErrACPAgentProfilePersistFailed = errors.New("ACP agent profile persist failed")

// ACPAgentProfileFailureKind classifies ACP agent profile failures peers can
// branch on with errors.Is / errors.As.
type ACPAgentProfileFailureKind string

const (
	ACPAgentProfileFailureKindInvalid ACPAgentProfileFailureKind = "invalid"
	ACPAgentProfileFailureKindPersist ACPAgentProfileFailureKind = "persist"
)

// ACPAgentProfileFailure retains normalized ACP agent profile failure facts
// without exposing storage, codec, or lifecycle construction ports.
type ACPAgentProfileFailure struct {
	Kind    ACPAgentProfileFailureKind
	Message string
	Field   string
}

func (failure ACPAgentProfileFailure) Error() string {
	message := strings.TrimSpace(failure.Message)
	field := strings.TrimSpace(failure.Field)
	switch {
	case message != "" && field != "":
		return fmt.Sprintf("%s: %s (%s)", sentinelForACPAgentProfileFailureKind(failure.Kind).Error(), message, field)
	case message != "":
		return fmt.Sprintf("%s: %s", sentinelForACPAgentProfileFailureKind(failure.Kind).Error(), message)
	case field != "":
		return fmt.Sprintf("%s (%s)", sentinelForACPAgentProfileFailureKind(failure.Kind).Error(), field)
	default:
		return sentinelForACPAgentProfileFailureKind(failure.Kind).Error()
	}
}

func (failure ACPAgentProfileFailure) Unwrap() error {
	return sentinelForACPAgentProfileFailureKind(failure.Kind)
}

func sentinelForACPAgentProfileFailureKind(kind ACPAgentProfileFailureKind) error {
	switch kind {
	case ACPAgentProfileFailureKindInvalid:
		return ErrACPAgentProfileInvalid
	case ACPAgentProfileFailureKindPersist:
		return ErrACPAgentProfilePersistFailed
	default:
		return ErrACPAgentProfileInvalid
	}
}

// ACPAgentProfile is the detached effective ACP agent target policy: one
// authoritative default Factory reference and one customer-owned Factory
// allowlist. Later ACP Chat target selection consumes this value instead of
// interpreting the raw operator settings document.
type ACPAgentProfile struct {
	DefaultFactoryReference string
	Allowlist               []string
}

// Clone returns a detached profile copy; mutating the result cannot change
// later resolution results or the persisted document.
func (profile ACPAgentProfile) Clone() ACPAgentProfile {
	cloned := profile
	if profile.Allowlist != nil {
		cloned.Allowlist = append([]string(nil), profile.Allowlist...)
	}
	return cloned
}

// DocumentACPAgentProfile is the detached authored-document peer value for one
// stored ACP agent profile. A nil *DocumentACPAgentProfile means no profile
// has been authored.
type DocumentACPAgentProfile struct {
	DefaultFactoryReference string
	Allowlist               []string
}

// Clone returns a detached authored-profile copy.
func (profile DocumentACPAgentProfile) Clone() DocumentACPAgentProfile {
	cloned := profile
	if profile.Allowlist != nil {
		cloned.Allowlist = append([]string(nil), profile.Allowlist...)
	}
	return cloned
}

// BuiltInACPAgentProfile returns the safe deterministic profile used when no
// profile has been authored: DefaultACPAgentFactoryReference as both the
// effective default and the sole allowed Factory reference.
func BuiltInACPAgentProfile() ACPAgentProfile {
	return ACPAgentProfile{
		DefaultFactoryReference: DefaultACPAgentFactoryReference,
		Allowlist:               []string{DefaultACPAgentFactoryReference},
	}
}

// acpAgentFactoryReferencePattern is the V0 lexical Factory-reference grammar
// enforced by Operator Settings without enumerating, canonicalizing, or
// verifying the existence of Factory targets: a scoped "@scope/name"
// reference using lowercase letters, digits, and internal single hyphens in
// each segment (matching DefaultACPAgentFactoryReference's shape).
var acpAgentFactoryReferencePattern = regexp.MustCompile(`^@[a-z0-9]+(?:-[a-z0-9]+)*/[a-z0-9]+(?:-[a-z0-9]+)*$`)

// NormalizeACPAgentProfile validates and normalizes a candidate default
// Factory reference and allowlist. It rejects blank or surrounding-whitespace
// references, malformed reference shapes, duplicate references, and a
// default absent from the allowlist, without enumerating, canonicalizing, or
// verifying the existence of Factory targets.
func NormalizeACPAgentProfile(defaultFactoryReference string, allowlist []string) (ACPAgentProfile, error) {
	normalizedDefault, err := normalizeACPAgentFactoryReference("defaultFactoryReference", defaultFactoryReference)
	if err != nil {
		return ACPAgentProfile{}, err
	}
	seen := make(map[string]struct{}, len(allowlist))
	normalizedAllowlist := make([]string, 0, len(allowlist))
	for index, reference := range allowlist {
		field := fmt.Sprintf("allowlist[%d]", index)
		normalized, err := normalizeACPAgentFactoryReference(field, reference)
		if err != nil {
			return ACPAgentProfile{}, err
		}
		if _, exists := seen[normalized]; exists {
			return ACPAgentProfile{}, ACPAgentProfileFailure{
				Kind:    ACPAgentProfileFailureKindInvalid,
				Message: fmt.Sprintf("allowlist reference %q is duplicated", normalized),
				Field:   field,
			}
		}
		seen[normalized] = struct{}{}
		normalizedAllowlist = append(normalizedAllowlist, normalized)
	}
	if _, ok := seen[normalizedDefault]; !ok {
		return ACPAgentProfile{}, ACPAgentProfileFailure{
			Kind:    ACPAgentProfileFailureKindInvalid,
			Message: fmt.Sprintf("default Factory reference %q must be present in the allowlist", normalizedDefault),
			Field:   "defaultFactoryReference",
		}
	}
	return ACPAgentProfile{DefaultFactoryReference: normalizedDefault, Allowlist: normalizedAllowlist}, nil
}

func normalizeACPAgentFactoryReference(field, reference string) (string, error) {
	if reference == "" {
		return "", ACPAgentProfileFailure{
			Kind:    ACPAgentProfileFailureKindInvalid,
			Message: "Factory reference must be non-empty",
			Field:   field,
		}
	}
	if strings.TrimSpace(reference) != reference {
		return "", ACPAgentProfileFailure{
			Kind:    ACPAgentProfileFailureKindInvalid,
			Message: fmt.Sprintf("Factory reference %q must not have surrounding whitespace", reference),
			Field:   field,
		}
	}
	if !acpAgentFactoryReferencePattern.MatchString(reference) {
		return "", ACPAgentProfileFailure{
			Kind:    ACPAgentProfileFailureKindInvalid,
			Message: fmt.Sprintf("Factory reference %q must be a scoped @scope/name reference using lowercase letters, digits, and hyphens", reference),
			Field:   field,
		}
	}
	return reference, nil
}

// ResolveACPAgentProfileRequest asks for the effective ACP agent profile from
// a detached authored-document fact. A nil AuthoredProfile resolves to
// BuiltInACPAgentProfile unless Path is non-blank, in which case the
// previously persisted profile at Path (if any) is used instead. Resolution
// does not read or mutate the operator document; document load/persist
// remain on document operations. AuthoredProfile always takes precedence
// over Path when both are supplied.
type ResolveACPAgentProfileRequest struct {
	Path            string
	AuthoredProfile *DocumentACPAgentProfile
}

// ResolveACPAgentProfileResult is the detached outcome of one ACP agent
// profile resolution.
type ResolveACPAgentProfileResult struct {
	Profile ACPAgentProfile
}

// UpdateACPAgentProfileRequest asks for a complete candidate ACP agent profile
// to be validated and atomically persisted at Path.
type UpdateACPAgentProfileRequest struct {
	Path                    string
	DefaultFactoryReference string
	Allowlist               []string
}

// Validate checks request fields whose validity does not depend on storage
// state.
func (request UpdateACPAgentProfileRequest) Validate() error {
	if strings.TrimSpace(request.Path) == "" {
		return ACPAgentProfileFailure{
			Kind:    ACPAgentProfileFailureKindInvalid,
			Message: "path is required",
			Field:   "path",
		}
	}
	return nil
}

// UpdateACPAgentProfileResult is the detached outcome of one ACP agent
// profile update.
type UpdateACPAgentProfileResult struct {
	Profile   ACPAgentProfile
	Persisted bool
}

// Stable ACP agent profile operation names for structured logs. Peers may
// branch on these values; they do not change across releases within L1 V0.
const (
	ACPAgentProfileOperationResolve = "operator_settings.resolve_acp_agent_profile"
	ACPAgentProfileOperationUpdate  = "operator_settings.update_acp_agent_profile"
)

// ACP agent profile structured log stages: one accepted-intent log per
// invocation, followed by exactly one terminal success or failure log.
const (
	ACPAgentProfileLogStageAcceptedIntent = "accepted_intent"
	ACPAgentProfileLogStageSuccess        = "success"
	ACPAgentProfileLogStageFailure        = "failure"
)

// ACPAgentProfileLogRecord is one safe structured ACP agent profile operation
// log record. Fields never include raw settings documents, full allowlists,
// credentials, prompts, raw provider commands, or filesystem paths.
type ACPAgentProfileLogRecord struct {
	Operation   string
	Stage       string
	FailureKind string
	Fields      map[string]any
}

// ACPAgentProfileLogger receives safe ACP agent profile operation log
// records. A nil logger disables logging without changing operation outcomes.
type ACPAgentProfileLogger func(ACPAgentProfileLogRecord)
