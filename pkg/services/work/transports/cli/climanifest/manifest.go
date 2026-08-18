package climanifest

import "fmt"

// Manifest is the production CLI command manifest document.
type Manifest struct {
	FormatVersion string             `json:"formatVersion"`
	RootPath      string             `json:"rootPath"`
	Commands      map[string]Command `json:"commands"`
}

// Command is one §4.3 command record from the production manifest.
type Command struct {
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	Path            string                    `json:"path"`
	Aliases         []string                  `json:"aliases"`
	Completeness    string                    `json:"completeness,omitempty"`
	GroupID         string                    `json:"groupId,omitempty"`
	Documentation   Documentation             `json:"documentation"`
	Lifecycle       Lifecycle                 `json:"lifecycle"`
	Visibility      string                    `json:"visibility"`
	Runnable        bool                      `json:"runnable"`
	Placement       PlacementCapability       `json:"placement,omitempty"`
	Usage           Usage                     `json:"usage"`
	Arguments       map[string]Argument       `json:"arguments,omitempty"`
	Flags           map[string]Flag           `json:"flags,omitempty"`
	SourceBindings  map[string]SourceBinding  `json:"sourceBindings,omitempty"`
	HandlerBindings map[string]HandlerBinding `json:"handlerBindings,omitempty"`
	Relationships   map[string]Relationship   `json:"relationships,omitempty"`
	Precedence      Precedence                `json:"precedence,omitempty"`
	Channels        Channels                  `json:"channels,omitempty"`
	Outputs         map[string]Output         `json:"outputs,omitempty"`
	Errors          map[string]SymbolicError  `json:"errors,omitempty"`
	Exits           map[string]Exit           `json:"exits,omitempty"`
	SideEffects     map[string]SideEffect     `json:"sideEffects,omitempty"`
	Constraints     Constraints               `json:"constraints,omitempty"`
	Handler         *Handler                  `json:"handler,omitempty"`
	RootLifecycle   *RootLifecycle            `json:"rootLifecycle,omitempty"`
}

// PlacementCapability declares where a command is allowed to execute. The
// declaration is transport metadata: domain handlers receive normalized
// requests and do not inspect this value themselves.
type PlacementCapability string

const (
	PlacementLocalOnly  PlacementCapability = "local-only"
	PlacementRemoteOnly PlacementCapability = "remote-only"
	PlacementDual       PlacementCapability = "dual"
)

// ExecutionPlacement is the resolved transport decision for one invocation.
type ExecutionPlacement string

const (
	ExecutionPlacementLocal  ExecutionPlacement = "local"
	ExecutionPlacementRemote ExecutionPlacement = "remote"
)

// ResolveRootPlacement turns the persistent --remote input into an explicit
// placement value. The false case is deliberately represented as local rather
// than as an omitted or implicit transport choice.
func ResolveRootPlacement(remote bool) ExecutionPlacement {
	if remote {
		return ExecutionPlacementRemote
	}
	return ExecutionPlacementLocal
}

// ResolvePlacement applies a command's capability to the root placement
// input. Remote-only commands preserve their historical remote-default
// behavior when --remote is omitted; dual commands default to local.
func (c Command) ResolvePlacement(remote bool) (ExecutionPlacement, error) {
	switch c.Placement {
	case PlacementLocalOnly:
		if remote {
			return "", fmt.Errorf("command %q supports local placement only; remove --remote", c.Path)
		}
		return ExecutionPlacementLocal, nil
	case PlacementRemoteOnly:
		return ExecutionPlacementRemote, nil
	case PlacementDual:
		return ResolveRootPlacement(remote), nil
	default:
		return "", fmt.Errorf("command %q has unsupported placement capability %q", c.Path, c.Placement)
	}
}

// SymbolicError is one stable ErrorResponse code declared by a command.
type SymbolicError struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

// RootLifecycle declares discovery behavior and lifecycle ownership without
// embedding executable policy in the transport contract.
type RootLifecycle struct {
	NoArguments string        `json:"noArguments"`
	HelpOutput  string        `json:"helpOutput"`
	ExitCode    int           `json:"exitCode"`
	SideEffects string        `json:"sideEffects"`
	Ownership   RootOwnership `json:"ownership"`
}

// RootOwnership records the stable manifest items that own the root discovery
// surface and the adjacent init, run, and server lifecycles.
type RootOwnership struct {
	Help   string `json:"help"`
	Init   string `json:"init"`
	Run    string `json:"run"`
	Server string `json:"server"`
}

// SourceBinding routes one non-CLI source into a stable input identity.
type SourceBinding struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	ExternalKey string `json:"externalKey,omitempty"`
	InputID     string `json:"inputId"`
}

// HandlerBinding routes one stable input identity into a handler-owned slot.
type HandlerBinding struct {
	ID      string `json:"id"`
	InputID string `json:"inputId"`
}

// Relationship describes a parse-time constraint between command inputs.
type Relationship struct {
	ID           string           `json:"id"`
	Kind         string           `json:"kind"`
	Participants []ParticipantRef `json:"participants"`
	When         *ParticipantRef  `json:"when,omitempty"`
}

// ParticipantRef points to one flag or positional argument in a relationship.
type ParticipantRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Precedence records input sources from highest to lowest priority.
type Precedence struct {
	Order            []string       `json:"order"`
	WithinTier       WithinTierRule `json:"withinTier"`
	AcrossTiers      string         `json:"acrossTiers"`
	MultipleBindings string         `json:"multipleBindings"`
}

// WithinTierRule defines deterministic handling for repeated observations from
// one source binding at the same precedence tier.
type WithinTierRule struct {
	Scalar   string `json:"scalar"`
	Repeated string `json:"repeated"`
}

// Handler carries stable handler identity and optional OpenAPI operation binding.
type Handler struct {
	ID          string `json:"id"`
	OperationID string `json:"operationId,omitempty"`
}

// Channels carries declared input and output channel surfaces.
type Channels struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// Output is one declared command output record.
type Output struct {
	ID      string `json:"id"`
	Channel string `json:"channel"`
	Format  string `json:"format"`
}

// Exit is one declared process exit outcome.
type Exit struct {
	ID   string `json:"id"`
	Code int    `json:"code"`
	Kind string `json:"kind"`
}

// SideEffect is one declared command side effect.
type SideEffect struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// Constraints carries runtime and platform declarations.
type Constraints struct {
	Runtime   []string `json:"runtime"`
	Platforms []string `json:"platforms"`
}

// Documentation carries shared documentation-schema fields used for help parity.
type Documentation struct {
	Documentation DocumentationCopy `json:"documentation"`
	Examples      []string          `json:"examples"`
}

// DocumentationCopy holds canonical English help copy.
type DocumentationCopy struct {
	Title       DocumentationField `json:"title"`
	Description DocumentationField `json:"description"`
}

// DocumentationField is one canonical documentation string.
type DocumentationField struct {
	CanonicalEnglish string `json:"canonicalEnglish"`
}

// Lifecycle carries the stable lifecycle metadata shared by commands and inputs.
type Lifecycle struct {
	FormatVersion string              `json:"formatVersion"`
	ItemID        string              `json:"itemId"`
	State         string              `json:"state"`
	Since         string              `json:"since"`
	Deprecated    string              `json:"deprecated,omitempty"`
	Removed       string              `json:"removed,omitempty"`
	Successor     *LifecycleSuccessor `json:"successor,omitempty"`
}

// LifecycleSuccessor carries canonical migration guidance for a deprecated or
// removed manifest item.
type LifecycleSuccessor struct {
	TargetItemID     string `json:"targetItemId"`
	CanonicalEnglish string `json:"canonicalEnglish"`
}

// Usage carries invocation-line and example metadata.
type Usage struct {
	Line    string `json:"line"`
	Example string `json:"example,omitempty"`
}
