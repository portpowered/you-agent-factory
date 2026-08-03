package service

import (
	"regexp"
	"strings"
)

const (
	// maxFailureDetailLength bounds any Detail text that passes redaction,
	// so a verbose raw error or panic dump cannot itself become an unbounded
	// exfiltration surface even when it matches no known sensitive shape.
	maxFailureDetailLength = 200
	// redactedFailureDetail is the fixed placeholder returned whenever raw
	// diagnostic text matches a known sensitive shape. The FailureCause.Kind
	// already identifies the failure category (including EXECUTOR_PANIC),
	// so redacting Detail never hides the classification itself.
	redactedFailureDetail = "redacted: diagnostic detail withheld (matched a sensitive-content pattern)"
)

// sensitiveDetailPattern matches free-form Workers WorkResult/adapter error
// text that plausibly carries credentials, tokens, environment values,
// prompts, file paths, or raw provider commands: neither Workers nor the
// adapter boundary establishes this text as safe, so FailureCause.Detail
// (public W2 API surface) must not trust it verbatim.
var sensitiveDetailPattern = regexp.MustCompile(
	`(?i)(api[_-]?key|authorization|bearer\s|password|secret|token|credential|` +
		`[a-z]:\\|\\\\|https?://|` +
		`(?:^|[\s"'=])/(?:[^\s"']+/)+[^\s"']*|` +
		`\b[A-Za-z_][A-Za-z0-9_]*=\S+)`,
)

// redactDetail bounds and scrubs raw Workers/adapter diagnostic text before
// it can reach the public FailureCause.Detail. Text matching a known
// sensitive shape (credential/token keyword, Windows or UNC path, URL,
// absolute file path, or KEY=VALUE assignment) is replaced wholesale with a
// fixed safe placeholder; any remaining text is length-bounded. redactDetail
// never affects classification, only the exposed detail string.
func redactDetail(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if sensitiveDetailPattern.MatchString(trimmed) {
		return redactedFailureDetail
	}
	if len(trimmed) > maxFailureDetailLength {
		return trimmed[:maxFailureDetailLength]
	}
	return trimmed
}
