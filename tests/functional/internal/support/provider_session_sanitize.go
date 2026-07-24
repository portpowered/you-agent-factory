package support

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Forbidden sanitization categories for provider-session golden fixtures.
const (
	ProviderSessionForbiddenCredential         = "credential"
	ProviderSessionForbiddenHostPath           = "host-path"
	ProviderSessionForbiddenPrivateRepoURL     = "private-repo-url"
	ProviderSessionForbiddenEnvDump            = "env-dump"
	ProviderSessionForbiddenUnboundedContent   = "unbounded-content"
	ProviderSessionForbiddenAccountIdentifier  = "account-identifier"
)

const (
	providerSessionMaxPromptRunes  = 4096
	providerSessionMaxStringRunes  = 32 * 1024
	providerSessionEnvDumpMinEntries = 2
)

var (
	providerSessionCredentialPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._\-+=/]{8,}`),
		regexp.MustCompile(`(?i)\bauthorization\s*[:=]\s*['\"]?\S{8,}`),
		regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|client[_-]?secret|refresh[_-]?token)\b\s*[\"']?\s*[:=]\s*[\"']?[^\s\"']{8,}`),
		regexp.MustCompile(`(?i)\bcookie\s*[:=]\s*\S+`),
		regexp.MustCompile(`(?i)\bsk-(?:ant-)?[A-Za-z0-9_\-]{16,}`),
		regexp.MustCompile(`(?i)\bxox[baprs]-[A-Za-z0-9\-]{10,}`),
		regexp.MustCompile(`(?i)\bghp_[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`(?i)\bgithub_pat_[A-Za-z0-9_]{20,}`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	}

	providerSessionHostPathPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:^|[\s\"'=])/(?:Users|home)/[^/\s\"']+`),
		regexp.MustCompile(`(?i)(?:^|[\s\"'=])[A-Za-z]:\\Users\\[^\\\s\"']+`),
		regexp.MustCompile(`(?i)(?:^|[\s\"'=])[A-Za-z]:/Users/[^/\s\"']+`),
	}

	providerSessionPrivateRepoPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bgit@[A-Za-z0-9._\-]+:\S+`),
		regexp.MustCompile(`(?i)\bssh://git@\S+`),
		regexp.MustCompile(`(?i)\bhttps?://[^/\s\"']+:[^/\s\"']+@\S+`),
	}

	providerSessionAccountKeyNames = map[string]struct{}{
		"accountid":      {},
		"account_id":     {},
		"organizationid": {},
		"organization_id": {},
		"orgid":          {},
		"org_id":         {},
		"useremail":      {},
		"user_email":     {},
		"email":          {},
	}

	providerSessionPromptKeyNames = map[string]struct{}{
		"prompt":       {},
		"systemprompt": {},
		"system_prompt": {},
		"userprompt":   {},
		"user_prompt":  {},
		"input":        {},
		"messages":     {},
	}

	providerSessionEnvKeyNames = map[string]struct{}{
		"env":         {},
		"environ":     {},
		"environment": {},
	}

	providerSessionFixtureEmailDomains = []string{
		"example.com",
		"example.org",
		"example.net",
		"test",
		"invalid",
		"localhost",
		"fixture.test",
	}

	providerSessionEmailPattern = regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
)

// ProviderSessionSanitizeError names the forbidden category and fixture path or field.
type ProviderSessionSanitizeError struct {
	CaseID   string
	Category string
	Path     string
	Field    string
	Detail   string
}

func (e *ProviderSessionSanitizeError) Error() string {
	caseID := e.CaseID
	if caseID == "" {
		caseID = "(unknown)"
	}
	location := e.Path
	if e.Field != "" {
		if location == "" {
			location = "field " + e.Field
		} else {
			location = fmt.Sprintf("%s field %s", location, e.Field)
		}
	}
	if location == "" {
		location = "(unknown location)"
	}
	category := e.Category
	if category == "" {
		category = "forbidden"
	}
	return fmt.Sprintf(
		"provider-session golden sanitization case %q rejected category %q at %s: %s",
		caseID,
		category,
		location,
		e.Detail,
	)
}

// ValidateProviderSessionCaseSanitization reads resolved fixture files and rejects
// unsanitized material according to sanitizerVersion 1.
func ValidateProviderSessionCaseSanitization(caseID string, paths ProviderSessionManifestPaths) error {
	caseID = strings.TrimSpace(caseID)
	targets := []struct {
		role string
		path string
	}{
		{role: "request", path: paths.Request},
		{role: "process", path: paths.Process},
		{role: "stdout", path: paths.Stdout},
		{role: "stderr", path: paths.Stderr},
		{role: "expected-provider-session", path: paths.ExpectedProviderSession},
		{role: "expected-response-events", path: paths.ExpectedResponseEvents},
		{role: "expected-invocation-result", path: paths.ExpectedInvocationResult},
	}
	for _, target := range targets {
		if strings.TrimSpace(target.path) == "" {
			continue
		}
		raw, err := os.ReadFile(target.path)
		if err != nil {
			return &ProviderSessionSanitizeError{
				CaseID:   caseID,
				Category: "fixture-read",
				Path:     target.path,
				Detail:   fmt.Sprintf("read %s fixture: %v", target.role, err),
			}
		}
		if err := ValidateProviderSessionFixtureContent(caseID, target.path, raw); err != nil {
			return err
		}
	}
	return nil
}

// ValidateProviderSessionFixtureContent scans one fixture blob for forbidden material.
func ValidateProviderSessionFixtureContent(caseID, fixturePath string, content []byte) error {
	caseID = strings.TrimSpace(caseID)
	displayPath := fixturePath
	if displayPath == "" {
		displayPath = "(inline)"
	}

	text := string(content)
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	// Prefer structured JSON/NDJSON walking so diagnostics can name fields.
	if looksLikeJSONDocument(trimmed) {
		var value any
		if err := json.Unmarshal(content, &value); err == nil {
			return walkProviderSessionJSONValue(caseID, displayPath, "", value)
		}
	}
	if looksLikeNDJSON(trimmed) {
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var value any
			if err := json.Unmarshal([]byte(line), &value); err != nil {
				continue
			}
			field := fmt.Sprintf("line[%d]", i+1)
			if err := walkProviderSessionJSONValue(caseID, displayPath, field, value); err != nil {
				return err
			}
		}
		return nil
	}

	return scanProviderSessionRawText(caseID, displayPath, "", text)
}

// ValidateProviderSessionManifestSanitization scans manifest string fields for
// forbidden material without requiring fixture files.
func ValidateProviderSessionManifestSanitization(manifest ProviderSessionGoldenManifest) error {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return &ProviderSessionSanitizeError{
			CaseID:   strings.TrimSpace(manifest.ID),
			Category: "manifest",
			Path:     ProviderSessionGoldenManifestFile,
			Detail:   fmt.Sprintf("encode manifest: %v", err),
		}
	}
	return ValidateProviderSessionFixtureContent(strings.TrimSpace(manifest.ID), ProviderSessionGoldenManifestFile, raw)
}

func scanProviderSessionRawText(caseID, path, field, text string) error {
	// Check more specific URL/path categories before generic credential heuristics so
	// credential-bearing private repo URLs classify as private-repo-url.
	for _, pattern := range providerSessionHostPathPatterns {
		if match := pattern.FindString(text); match != "" {
			return &ProviderSessionSanitizeError{
				CaseID:   caseID,
				Category: ProviderSessionForbiddenHostPath,
				Path:     path,
				Field:    field,
				Detail:   fmt.Sprintf("matched host path %q", truncateForSanitizeDetail(strings.TrimSpace(match))),
			}
		}
	}
	for _, pattern := range providerSessionPrivateRepoPatterns {
		if match := pattern.FindString(text); match != "" {
			return &ProviderSessionSanitizeError{
				CaseID:   caseID,
				Category: ProviderSessionForbiddenPrivateRepoURL,
				Path:     path,
				Field:    field,
				Detail:   fmt.Sprintf("matched private repository URL %q", truncateForSanitizeDetail(match)),
			}
		}
	}
	for _, pattern := range providerSessionCredentialPatterns {
		if match := pattern.FindString(text); match != "" {
			return &ProviderSessionSanitizeError{
				CaseID:   caseID,
				Category: ProviderSessionForbiddenCredential,
				Path:     path,
				Field:    field,
				Detail:   fmt.Sprintf("matched credential-like material %q", truncateForSanitizeDetail(match)),
			}
		}
	}
	return nil
}

func walkProviderSessionJSONValue(caseID, path, field string, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		if err := rejectProviderSessionEnvDump(caseID, path, field, typed); err != nil {
			return err
		}
		for key, child := range typed {
			childField := joinJSONField(field, key)
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if _, ok := providerSessionAccountKeyNames[lowerKey]; ok {
				if err := rejectProviderSessionAccountIdentifier(caseID, path, childField, child); err != nil {
					return err
				}
			}
			if _, ok := providerSessionPromptKeyNames[lowerKey]; ok {
				if err := rejectProviderSessionUnboundedPrompt(caseID, path, childField, child); err != nil {
					return err
				}
			}
			if err := walkProviderSessionJSONValue(caseID, path, childField, child); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range typed {
			childField := fmt.Sprintf("%s[%d]", fieldOrRoot(field), i)
			if field == "" {
				childField = fmt.Sprintf("[%d]", i)
			}
			if err := walkProviderSessionJSONValue(caseID, path, childField, child); err != nil {
				return err
			}
		}
	case string:
		if utf8.RuneCountInString(typed) > providerSessionMaxStringRunes {
			return &ProviderSessionSanitizeError{
				CaseID:   caseID,
				Category: ProviderSessionForbiddenUnboundedContent,
				Path:     path,
				Field:    field,
				Detail:   fmt.Sprintf("string exceeds %d runes", providerSessionMaxStringRunes),
			}
		}
		if err := scanProviderSessionRawText(caseID, path, field, typed); err != nil {
			return err
		}
		if err := rejectProviderSessionEmailAccount(caseID, path, field, typed); err != nil {
			return err
		}
	}
	return nil
}

func rejectProviderSessionEnvDump(caseID, path, field string, object map[string]any) error {
	for key, child := range object {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if _, ok := providerSessionEnvKeyNames[lowerKey]; !ok {
			continue
		}
		childField := joinJSONField(field, key)
		switch envValue := child.(type) {
		case map[string]any:
			if len(envValue) >= providerSessionEnvDumpMinEntries {
				return &ProviderSessionSanitizeError{
					CaseID:   caseID,
					Category: ProviderSessionForbiddenEnvDump,
					Path:     path,
					Field:    childField,
					Detail:   fmt.Sprintf("raw environment object has %d entries", len(envValue)),
				}
			}
		case []any:
			envLike := 0
			for _, entry := range envValue {
				text, ok := entry.(string)
				if !ok {
					continue
				}
				if strings.Contains(text, "=") {
					envLike++
				}
			}
			if envLike >= providerSessionEnvDumpMinEntries {
				return &ProviderSessionSanitizeError{
					CaseID:   caseID,
					Category: ProviderSessionForbiddenEnvDump,
					Path:     path,
					Field:    childField,
					Detail:   fmt.Sprintf("raw environment list has %d KEY=value entries", envLike),
				}
			}
		}
	}
	return nil
}

func rejectProviderSessionUnboundedPrompt(caseID, path, field string, value any) error {
	switch typed := value.(type) {
	case string:
		if utf8.RuneCountInString(typed) > providerSessionMaxPromptRunes {
			return &ProviderSessionSanitizeError{
				CaseID:   caseID,
				Category: ProviderSessionForbiddenUnboundedContent,
				Path:     path,
				Field:    field,
				Detail:   fmt.Sprintf("prompt-like field exceeds %d runes", providerSessionMaxPromptRunes),
			}
		}
	case []any:
		total := 0
		for _, item := range typed {
			switch message := item.(type) {
			case string:
				total += utf8.RuneCountInString(message)
			case map[string]any:
				for _, key := range []string{"content", "text", "prompt"} {
					if text, ok := message[key].(string); ok {
						total += utf8.RuneCountInString(text)
					}
				}
			}
		}
		if total > providerSessionMaxPromptRunes {
			return &ProviderSessionSanitizeError{
				CaseID:   caseID,
				Category: ProviderSessionForbiddenUnboundedContent,
				Path:     path,
				Field:    field,
				Detail:   fmt.Sprintf("prompt-like messages exceed %d runes", providerSessionMaxPromptRunes),
			}
		}
	}
	return nil
}

func rejectProviderSessionAccountIdentifier(caseID, path, field string, value any) error {
	text, ok := value.(string)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if providerSessionLooksLikeFixtureIdentifier(trimmed) {
		return nil
	}
	return &ProviderSessionSanitizeError{
		CaseID:   caseID,
		Category: ProviderSessionForbiddenAccountIdentifier,
		Path:     path,
		Field:    field,
		Detail:   fmt.Sprintf("provider account identifier %q is not a fixture placeholder", truncateForSanitizeDetail(trimmed)),
	}
}

func rejectProviderSessionEmailAccount(caseID, path, field string, text string) error {
	matches := providerSessionEmailPattern.FindAllString(text, -1)
	for _, email := range matches {
		if providerSessionIsFixtureEmail(email) {
			continue
		}
		return &ProviderSessionSanitizeError{
			CaseID:   caseID,
			Category: ProviderSessionForbiddenAccountIdentifier,
			Path:     path,
			Field:    field,
			Detail:   fmt.Sprintf("non-fixture account email %q", email),
		}
	}
	return nil
}

func providerSessionLooksLikeFixtureIdentifier(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"fixture", "example", "test", "fake", "sanitized", "dummy", "sample"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if providerSessionIsFixtureEmail(value) {
		return true
	}
	return false
}

func providerSessionIsFixtureEmail(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, allowed := range providerSessionFixtureEmailDomains {
		if domain == allowed || strings.HasSuffix(domain, "."+allowed) {
			return true
		}
	}
	return false
}

func looksLikeJSONDocument(text string) bool {
	return strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[")
}

func looksLikeNDJSON(text string) bool {
	if !strings.Contains(text, "\n") {
		return false
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") && !strings.HasPrefix(line, "[") {
			return false
		}
	}
	return true
}

func joinJSONField(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func fieldOrRoot(field string) string {
	if field == "" {
		return "$"
	}
	return field
}

func truncateForSanitizeDetail(value string) string {
	const max = 80
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max]) + "…"
}

// ProviderSessionFixtureRelativePath returns a stable display path under the case dir when possible.
func ProviderSessionFixtureRelativePath(caseDir, absPath string) string {
	caseDir = filepath.Clean(caseDir)
	absPath = filepath.Clean(absPath)
	rel, err := filepath.Rel(caseDir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}
