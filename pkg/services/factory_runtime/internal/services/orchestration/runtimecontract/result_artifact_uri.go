package runtimecontract

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
)

const ArtifactURIScheme = orchestratorcontract.ArtifactURIScheme

// FormatArtifactURI builds the canonical session-scoped artifact URI.
func FormatArtifactURI(sessionID, artifactID string) string {
	return fmt.Sprintf("%s://sessions/%s/artifacts/%s", ArtifactURIScheme, sessionID, artifactID)
}

// ParsedArtifactURI is a validated you-artifact URI.
type ParsedArtifactURI struct {
	SessionID  string
	ArtifactID string
}

// ParseArtifactURI parses one you-artifact session artifact URI.
func ParseArtifactURI(raw string) (ParsedArtifactURI, []ResultIssue) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ParsedArtifactURI{}, []ResultIssue{malformedArtifactURI(raw, "artifact URI is required")}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme != ArtifactURIScheme {
		return ParsedArtifactURI{}, []ResultIssue{malformedArtifactURI(raw, "artifact URI must use the you-artifact scheme")}
	}
	if parsed.Host != "" && parsed.Host != "sessions" {
		return ParsedArtifactURI{}, []ResultIssue{malformedArtifactURI(raw, "artifact URI host must be sessions")}
	}
	if strings.Contains(parsed.Path, "..") {
		return ParsedArtifactURI{}, []ResultIssue{{
			Code:    CodeArtifactURIPathTraversal,
			Message: fmt.Sprintf("artifact URI %q must not include path traversal", raw),
			Path:    "url",
		}}
	}
	if looksLikeHostPath(trimmed) {
		return ParsedArtifactURI{}, []ResultIssue{{
			Code:    CodeArtifactURIHostPath,
			Message: fmt.Sprintf("artifact URI %q must not reference host filesystem paths", raw),
			Path:    "url",
		}}
	}

	sessionID, artifactID, ok := parseArtifactURIPath(parsed.Host, parsed.Path, parsed.Opaque)
	if !ok {
		return ParsedArtifactURI{}, []ResultIssue{malformedArtifactURI(raw, "artifact URI must match you-artifact://sessions/{session_id}/artifacts/{artifact_id}")}
	}
	if issue := validateArtifactID("sessionId", sessionID); issue != nil {
		return ParsedArtifactURI{}, []ResultIssue{*issue}
	}
	if issue := validateArtifactID("artifactId", artifactID); issue != nil {
		return ParsedArtifactURI{}, []ResultIssue{*issue}
	}
	return ParsedArtifactURI{SessionID: sessionID, ArtifactID: artifactID}, nil
}

// ValidateArtifactURIForSession checks that one artifact URI belongs to the
// requested session.
func ValidateArtifactURIForSession(raw, expectedSessionID string) []ResultIssue {
	parsed, issues := ParseArtifactURI(raw)
	if len(issues) > 0 {
		return issues
	}
	expected := strings.TrimSpace(expectedSessionID)
	if expected == "" {
		return []ResultIssue{malformedArtifactURI(raw, "expected session id is required")}
	}
	if parsed.SessionID != expected {
		return []ResultIssue{{
			Code:    CodeArtifactURISessionMismatch,
			Message: fmt.Sprintf("artifact URI session %q does not match requested session %q", parsed.SessionID, expected),
			Path:    "url",
		}}
	}
	return nil
}

func parseArtifactURIPath(host, path, opaque string) (sessionID, artifactID string, ok bool) {
	candidate := strings.TrimSpace(path)
	if candidate == "" || candidate == "/" {
		candidate = "/" + strings.TrimPrefix(strings.TrimSpace(opaque), "/")
	}
	segments := strings.Split(strings.Trim(candidate, "/"), "/")
	if strings.TrimSpace(host) == "sessions" {
		if len(segments) != 3 || segments[1] != "artifacts" {
			return "", "", false
		}
		return segments[0], segments[2], true
	}
	if len(segments) != 4 || segments[0] != "sessions" || segments[2] != "artifacts" {
		return "", "", false
	}
	return segments[1], segments[3], true
}

func validateArtifactID(field, value string) *ResultIssue {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return &ResultIssue{
			Code:    CodeArtifactURIInvalidID,
			Message: fmt.Sprintf("%s is required", field),
			Path:    field,
		}
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "..") {
		return &ResultIssue{
			Code:    CodeArtifactURIInvalidID,
			Message: fmt.Sprintf("%s %q is malformed", field, trimmed),
			Path:    field,
		}
	}
	return nil
}

func malformedArtifactURI(raw, message string) ResultIssue {
	return ResultIssue{
		Code:    CodeArtifactURIMalformed,
		Message: message,
		Path:    "url",
	}
}
