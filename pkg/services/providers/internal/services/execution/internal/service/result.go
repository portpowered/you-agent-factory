package service

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Native adapters publish customer content before returning diagnostics.
// Redact declared secrets here; ordinary request text is not a secret.
func safeProgressRequest(request providers.ExecuteRequest, extraSecrets ...string) providers.ExecuteRequest {
	observer := request.ProgressObserver
	if observer == nil {
		return request
	}
	var mu sync.Mutex
	count := 0
	request.ProgressObserver = func(progress providers.ExecuteProgress) {
		mu.Lock()
		defer mu.Unlock()
		if count >= maxProgressFacts {
			return
		}
		count++
		observer(sanitizeCapturedProgress(progress, request, extraSecrets...))
	}
	return request
}

func sanitizeCapturedProgress(progress providers.ExecuteProgress, request providers.ExecuteRequest, extraSecrets ...string) providers.ExecuteProgress {
	// Diagnostic-only fields remain private in normalizeDiagnostics. Removing
	// them from this detached policy input lets ordinary echoes survive capture.
	request.SystemPrompt, request.UserMessage, request.OutputSchema = "", "", ""
	request.WorkingDirectory, request.Worktree = "", ""
	secrets := requestDiagnosticSecrets(request, extraSecrets...)
	var replacements []string
	for _, secret := range secrets {
		if secret != "" {
			replacements = append(replacements, secret, redactedValue)
		}
	}
	replacer := strings.NewReplacer(replacements...)
	redact := func(value string) string {
		return replacer.Replace(strings.ToValidUTF8(value, ""))
	}
	progress.Phase = boundedRunes(redact(progress.Phase), maxProgressPhaseRunes)
	// Adapters own capture limits and report truncation. A diagnostic bound
	// here would silently lose otherwise admitted message/tool content.
	progress.Detail = redact(progress.Detail)
	if progress.Metadata != nil {
		metadata := make(map[string]string, len(progress.Metadata))
		for key, value := range progress.Metadata {
			if containsSensitiveMetadataTerm(key) {
				metadata[key] = redactedValue
			} else {
				metadata[key] = redact(value)
			}
		}
		progress.Metadata = metadata
	}
	return progress
}

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
	"thinking_tokens":         {},
	"cached_input_tokens":     {},
	"total_tokens":            {},
	"cache_read_tokens":       {},
	"cache_write_tokens":      {},
}

func normalizeSuccess(
	result providers.ExecuteResult,
	provider providers.ID,
	request providers.ExecuteRequest,
	extraSecrets ...string,
) (providers.ExecuteResult, error) {
	normalized := result.Clone()
	if err := validateSessionRef(normalized.SessionRef, provider); err != nil {
		return providers.ExecuteResult{}, err
	}
	if normalized.Diagnostics != nil {
		diagnostics := normalizeDiagnostics(*normalized.Diagnostics, request, extraSecrets...)
		normalized.Diagnostics = &diagnostics
	}
	return normalized, nil
}

func validateSessionRef(ref *providers.SessionRef, provider providers.ID) error {
	if ref == nil {
		return nil
	}
	if err := ref.Validate(); err != nil {
		return fmt.Errorf(
			"%w: adapter returned invalid session reference",
			providers.ErrExecuteFailed,
		)
	}
	if ref.Provider != provider {
		return fmt.Errorf(
			"%w: adapter returned session for another provider",
			providers.ErrExecuteFailed,
		)
	}
	return nil
}

func normalizeDiagnostics(
	diagnostics providers.ExecuteDiagnostics,
	request providers.ExecuteRequest,
	extraSecrets ...string,
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
				extraSecrets...,
			),
			Detail: sanitizeDiagnosticText(
				diagnostics.Progress[i].Detail,
				maxDiagnosticRunes,
				request,
				extraSecrets...,
			),
			Metadata: sanitizeMetadata(diagnostics.Progress[i].Metadata, request, extraSecrets...),
		}
	}
	diagnostics.Progress = progress
	diagnostics.Metadata = sanitizeMetadata(diagnostics.Metadata, request, extraSecrets...)
	return diagnostics
}

func sanitizeMetadata(
	metadata map[string]string,
	request providers.ExecuteRequest,
	extraSecrets ...string,
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
			extraSecrets...,
		)
	}
	return sanitized
}

func sanitizeDiagnosticText(
	value string,
	limit int,
	request providers.ExecuteRequest,
	extraSecrets ...string,
) string {
	sanitized := strings.ToValidUTF8(value, "")
	for _, secret := range requestDiagnosticSecrets(request, extraSecrets...) {
		if secret != "" {
			sanitized = strings.ReplaceAll(sanitized, secret, redactedValue)
		}
	}
	return boundedRunes(sanitized, limit)
}

func requestDiagnosticSecrets(request providers.ExecuteRequest, extraSecrets ...string) []string {
	secrets := []string{
		request.SystemPrompt,
		request.UserMessage,
		request.OutputSchema,
		request.WorkingDirectory,
		request.Worktree,
	}
	// Sensitive environment keys classify their values as secrets too. An
	// adapter may echo one inside ordinary tool/progress text or a safe-keyed
	// diagnostic, where filtering metadata keys alone cannot protect it.
	for key, value := range request.EnvVars {
		if containsSensitiveMetadataTerm(key) {
			secrets = append(secrets, value)
		}
	}
	for _, entry := range request.ProcessEnvironment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && containsSensitiveMetadataTerm(key) {
			secrets = append(secrets, value)
		}
	}
	secrets = append(secrets, extraSecrets...)
	// Redact a containing value before its substring; map iteration order must
	// not leave part of a longer classified value in diagnostics.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
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
