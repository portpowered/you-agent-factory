package factorysessionexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// TypedFailureKind names one stable fixture-provider failure category downstream
// API, CLI, MCP, and UI cells can assert against without parsing free-form text.
type TypedFailureKind string

const (
	TypedFailureUnknownScenario          TypedFailureKind = "UNKNOWN_SCENARIO"
	TypedFailureMalformedRequest         TypedFailureKind = "MALFORMED_REQUEST"
	TypedFailureSessionNotFound          TypedFailureKind = "SESSION_NOT_FOUND"
	TypedFailureDispatchNotFound         TypedFailureKind = "DISPATCH_NOT_FOUND"
	TypedFailureArtifactNotFound         TypedFailureKind = "ARTIFACT_NOT_FOUND"
	TypedFailureReconnectCursorNotFound  TypedFailureKind = "RECONNECT_CURSOR_NOT_FOUND"
	TypedFailureExecutionRequestConflict TypedFailureKind = "EXECUTION_REQUEST_ID_CONFLICT"
	TypedFailureLifecycleConflict        TypedFailureKind = "LIFECYCLE_CONFLICT"
	TypedFailureLifecycleInvalidState    TypedFailureKind = "LIFECYCLE_INVALID_STATE"
	TypedFailureLifecycleTerminalSession TypedFailureKind = "LIFECYCLE_TERMINAL_SESSION"
)

// TypedFailureIdentity captures structured context for one fixture-provider failure.
type TypedFailureIdentity struct {
	Kind      TypedFailureKind
	Field     string
	Operation LifecycleControlKind
	Outcome   LifecycleControlOutcome
	Status    LifecycleStatus
}

// TypedFailureIdentityFromError maps one service error to a stable typed identity.
func TypedFailureIdentityFromError(err error) (TypedFailureIdentity, bool) {
	if err == nil {
		return TypedFailureIdentity{}, false
	}

	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		kind := TypedFailureMalformedRequest
		if validationErr.Field == "requestId" && strings.Contains(validationErr.Message, "unknown fake scenario") {
			kind = TypedFailureUnknownScenario
		}
		return TypedFailureIdentity{
			Kind:  kind,
			Field: validationErr.Field,
		}, true
	}

	var controlErr *ControlError
	if errors.As(err, &controlErr) {
		kind := TypedFailureLifecycleConflict
		switch controlErr.Outcome {
		case LifecycleControlOutcomeInvalidState:
			kind = TypedFailureLifecycleInvalidState
		case LifecycleControlOutcomeTerminalSession:
			kind = TypedFailureLifecycleTerminalSession
		case LifecycleControlOutcomeConflict:
			kind = TypedFailureLifecycleConflict
		default:
			return TypedFailureIdentity{}, false
		}
		return TypedFailureIdentity{
			Kind:      kind,
			Operation: controlErr.Operation,
			Outcome:   controlErr.Outcome,
			Status:    controlErr.Status,
		}, true
	}

	switch {
	case errors.Is(err, ErrSessionNotFound):
		return TypedFailureIdentity{Kind: TypedFailureSessionNotFound}, true
	case errors.Is(err, ErrDispatchNotFound):
		return TypedFailureIdentity{Kind: TypedFailureDispatchNotFound}, true
	case errors.Is(err, ErrArtifactNotFound):
		return TypedFailureIdentity{Kind: TypedFailureArtifactNotFound}, true
	case errors.Is(err, ErrReconnectCursorNotFound):
		return TypedFailureIdentity{Kind: TypedFailureReconnectCursorNotFound}, true
	case errors.Is(err, ErrExecutionRequestIDConflict):
		return TypedFailureIdentity{Kind: TypedFailureExecutionRequestConflict}, true
	default:
		return TypedFailureIdentity{}, false
	}
}

// ForbiddenFixtureVocabularyTerms are resource names that must not appear in public
// fixture fields, error names, or downstream-facing test descriptions.
func ForbiddenFixtureVocabularyTerms() []string {
	return []string{"DynamicWorkflowRun", "workflow run"}
}

// TypedFailureHash returns a stable digest for one typed failure identity.
func TypedFailureHash(identity TypedFailureIdentity) (string, error) {
	document := map[string]any{
		"kind": string(identity.Kind),
	}
	if field := strings.TrimSpace(identity.Field); field != "" {
		document["field"] = field
	}
	if identity.Operation != "" {
		document["operation"] = string(identity.Operation)
	}
	if identity.Outcome != "" {
		document["outcome"] = string(identity.Outcome)
	}
	if identity.Status != "" {
		document["status"] = string(identity.Status)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal typed failure identity: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
