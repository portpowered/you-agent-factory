package service

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const (
	maxProgressFacts      = 128
	maxMetadataEntries    = 32
	maxProgressPhaseRunes = 64
	maxDiagnosticRunes    = 512
	maxMetadataKeyRunes   = 64
	redactedValue         = "<redacted>"
)

var sensitiveMetadataTerms = []string{
	"api_key",
	"apikey",
	"authorization",
	"command",
	"credential",
	"env",
	"output",
	"password",
	"prompt",
	"secret",
	"stderr",
	"stdout",
	"token",
}

// allowedSensitiveMetadataKeys are progress metadata keys that may contain
// sensitive-looking substrings such as "token" while carrying bounded numeric
// usage facts rather than secret material.
var allowedSensitiveMetadataKeys = map[string]struct{}{
	"input_tokens":            {},
	"output_tokens":           {},
	"reasoning_tokens":        {},
	"reasoning_output_tokens": {},
	"cached_input_tokens":     {},
	"total_tokens":            {},
	"cache_read_tokens":       {},
	"cache_write_tokens":      {},
}

func normalizeSuccess(
	result providers.ExecuteResult,
	provider providers.ID,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	normalized := result.Clone()
	if normalized.SessionRef != nil {
		if err := normalized.SessionRef.Validate(); err != nil {
			return providers.ExecuteResult{}, fmt.Errorf(
				"%w: adapter returned invalid session reference",
				providers.ErrExecuteFailed,
			)
		}
		if normalized.SessionRef.Provider != provider {
			return providers.ExecuteResult{}, fmt.Errorf(
				"%w: adapter returned session for another provider",
				providers.ErrExecuteFailed,
			)
		}
	}
	if normalized.Diagnostics != nil {
		diagnostics := normalizeDiagnostics(*normalized.Diagnostics, request)
		normalized.Diagnostics = &diagnostics
	}
	return normalized, nil
}

func normalizeDiagnostics(
	diagnostics providers.ExecuteDiagnostics,
	request providers.ExecuteRequest,
) providers.ExecuteDiagnostics {
	if diagnostics.DurationMillis < 0 {
		diagnostics.DurationMillis = 0
	}
	if len(diagnostics.Progress) > maxProgressFacts {
		diagnostics.Progress = diagnostics.Progress[:maxProgressFacts]
	}
	progress := make([]providers.ExecuteProgress, len(diagnostics.Progress))
	for i := range diagnostics.Progress {
		progress[i] = providers.ExecuteProgress{
			Phase: sanitizeDiagnosticText(
				diagnostics.Progress[i].Phase,
				maxProgressPhaseRunes,
				request,
			),
			Detail: sanitizeDiagnosticText(
				diagnostics.Progress[i].Detail,
				maxDiagnosticRunes,
				request,
			),
			Metadata: sanitizeMetadata(diagnostics.Progress[i].Metadata, request),
		}
	}
	diagnostics.Progress = progress
	diagnostics.Metadata = sanitizeMetadata(diagnostics.Metadata, request)
	return diagnostics
}

func sanitizeMetadata(
	metadata map[string]string,
	request providers.ExecuteRequest,
) map[string]string {
	if metadata == nil {
		return nil
	}
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > maxMetadataEntries {
		keys = keys[:maxMetadataEntries]
	}
	sanitized := make(map[string]string, len(keys))
	for _, key := range keys {
		safeKey := boundedRunes(strings.TrimSpace(key), maxMetadataKeyRunes)
		if safeKey == "" {
			continue
		}
		if containsSensitiveMetadataTerm(safeKey) {
			sanitized[safeKey] = redactedValue
			continue
		}
		sanitized[safeKey] = sanitizeDiagnosticText(
			metadata[key],
			maxDiagnosticRunes,
			request,
		)
	}
	return sanitized
}

func sanitizeDiagnosticText(
	value string,
	limit int,
	request providers.ExecuteRequest,
) string {
	sanitized := strings.ToValidUTF8(value, "")
	for _, secret := range requestDiagnosticSecrets(request) {
		if utf8.RuneCountInString(secret) >= 4 {
			sanitized = strings.ReplaceAll(sanitized, secret, redactedValue)
		}
	}
	return boundedRunes(sanitized, limit)
}

func requestDiagnosticSecrets(request providers.ExecuteRequest) []string {
	secrets := []string{
		request.SystemPrompt,
		request.UserMessage,
		request.OutputSchema,
		request.WorkingDirectory,
		request.Worktree,
	}
	if request.ResumeSession != nil {
		secrets = append(secrets, request.ResumeSession.ID)
	}
	return secrets
}

func containsSensitiveMetadataTerm(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	if _, allowed := allowedSensitiveMetadataKeys[normalized]; allowed {
		return false
	}
	for _, term := range sensitiveMetadataTerms {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	return false
}

func boundedRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
