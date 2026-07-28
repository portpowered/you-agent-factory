// Package providers owns provider identity, catalog inspection, and one
// provider-native execution attempt. It intentionally uses detached contracts
// so Workers can translate Factory-owned state at the service boundary.
package providers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type ID string

var providerIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

func (id ID) Validate() error {
	value := string(id)
	if len(value) > 128 || value != strings.TrimSpace(value) || !providerIDPattern.MatchString(value) {
		return fmt.Errorf("provider identity %q must use canonical lowercase letters, digits, dots, or hyphens", value)
	}
	return nil
}

type ExecutionKind string

const ExecutionKindACP ExecutionKind = "ACP"

type AvailabilityState string

const (
	AvailabilityAvailable   AvailabilityState = "AVAILABLE"
	AvailabilityUnavailable AvailabilityState = "UNAVAILABLE"
)

type Availability struct {
	State  AvailabilityState
	Detail string
}

type Provider struct {
	ID            ID
	DisplayName   string
	ExecutionKind ExecutionKind
	Command       string
	Arguments     []string
	Availability  Availability
}

// Integration is the detached, settings-selected provider configuration
// consumed when constructing one invocation-local Providers catalog.
type Integration struct {
	ID        string
	Name      ID
	Transport string
	Command   string
}

type Factory func([]Integration) (Service, error)

type ListRequest struct{}
type ListResponse struct{ Providers []Provider }
type GetRequest struct{ ID ID }
type GetResponse struct{ Provider Provider }

type Correlation struct {
	FactorySessionID string
	DispatchID       string
	Workstation      string
}

type ContentKind string

const (
	ContentKindText         ContentKind = "text"
	ContentKindResourceLink ContentKind = "resource_link"
)

type ContentPart struct {
	Kind     ContentKind
	Text     string
	Name     string
	URI      string
	MimeType string
}

type EnvironmentEntry struct {
	Name  string
	Value string
}

type ExecuteRequest struct {
	ProviderID       ID
	Correlation      Correlation
	Instructions     string
	Prompt           []ContentPart
	Model            string
	WorkingDirectory string
	Environment      []EnvironmentEntry
	SkipPermissions  bool
}

type SessionRef struct {
	ProviderID ID
	Kind       string
	ID         string
}

type SafeDiagnostics struct {
	Metadata map[string]string
}

// ExecutionUpdateKind identifies one provider-neutral update produced during
// an execution attempt. Provider-specific protocol values remain in NativeType.
type ExecutionUpdateKind string

const (
	ExecutionUpdateMessage    ExecutionUpdateKind = "MESSAGE"
	ExecutionUpdateReasoning  ExecutionUpdateKind = "REASONING"
	ExecutionUpdateTool       ExecutionUpdateKind = "TOOL"
	ExecutionUpdateFileChange ExecutionUpdateKind = "FILE_CHANGE"
	ExecutionUpdatePlan       ExecutionUpdateKind = "PLAN"
	ExecutionUpdateUsage      ExecutionUpdateKind = "USAGE"
	ExecutionUpdateSession    ExecutionUpdateKind = "SESSION"
	ExecutionUpdateError      ExecutionUpdateKind = "ERROR"
)

type ToolUpdate struct {
	ID        string
	Name      string
	Status    string
	RawInput  any
	RawOutput any
}

type PlanEntry struct {
	ID          string
	Description string
	Status      string
}

type UsageUpdate struct {
	UsedTokens int64
	MaxTokens  int64
}

type FileChangeUpdate struct {
	Path      string
	Operation string
	Summary   string
}

type ErrorUpdate struct {
	Code      string
	Message   string
	Retryable bool
}

// ExecutionUpdate is detached from ACP and from Factory Session response
// events. Workers maps it into the one existing response-event vocabulary.
type ExecutionUpdate struct {
	Sequence          int64
	Kind              ExecutionUpdateKind
	NativeType        string
	ItemID            string
	ProviderSessionID string
	Text              string
	Final             bool
	Partial           bool
	Tool              *ToolUpdate
	FileChange        *FileChangeUpdate
	Plan              []PlanEntry
	Usage             *UsageUpdate
	Error             *ErrorUpdate
	Metadata          map[string]string
}

type ExecuteResponse struct {
	Content     string
	Session     *SessionRef
	Diagnostics *SafeDiagnostics
}

type ExecuteOutcome struct {
	Response ExecuteResponse
	Err      error
}

// ExecutionStream is one provider-owned attempt stream. The single buffered
// outcome follows closure of Updates. Close cancels the attempt and is safe to
// call more than once.
type ExecutionStream struct {
	Updates <-chan ExecutionUpdate
	Outcome <-chan ExecuteOutcome
	Close   func()
}

type Service interface {
	List(context.Context, ListRequest) (ListResponse, error)
	Get(context.Context, GetRequest) (GetResponse, error)
	Execute(context.Context, ExecuteRequest) (ExecuteResponse, error)
	ExecuteStream(context.Context, ExecuteRequest) (*ExecutionStream, error)
}

var (
	ErrUnknownProvider        = errors.New("unknown provider")
	ErrUnavailableProvider    = errors.New("unavailable provider")
	ErrUnsupportedExecutor    = errors.New("unsupported provider execution kind")
	ErrInvalidRequest         = errors.New("invalid provider execution request")
	ErrAuthenticationRequired = errors.New("provider authentication required")
	ErrIncompatibleProtocol   = errors.New("incompatible provider protocol")
	ErrProtocol               = errors.New("provider protocol failure")
)

func (value Provider) Clone() Provider {
	value.Arguments = append([]string(nil), value.Arguments...)
	return value
}
