package provider

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	cursorResultTypeResult = "result"

	cursorResultSubtypeSuccess = "success"

	cursorResponseMetadataRequestID       = "request_id"
	cursorResponseMetadataDurationMS      = "duration_ms"
	cursorResponseMetadataDurationAPIMS   = "duration_api_ms"
	cursorResponseMetadataInputTokens     = "input_tokens"
	cursorResponseMetadataOutputTokens    = "output_tokens"
	cursorResponseMetadataCacheReadTokens = "cache_read_tokens"
	cursorResponseMetadataCacheWriteTokens = "cache_write_tokens"
)

type cursorResultPayload struct {
	Type          string              `json:"type"`
	Subtype       string              `json:"subtype"`
	IsError       bool                `json:"is_error"`
	DurationMS    int64               `json:"duration_ms"`
	DurationAPIMS int64               `json:"duration_api_ms"`
	Result        string              `json:"result"`
	SessionID     string              `json:"session_id"`
	RequestID     string              `json:"request_id"`
	Usage         *cursorResultUsage  `json:"usage,omitempty"`
}

type cursorResultUsage struct {
	InputTokens      *int `json:"inputTokens,omitempty"`
	OutputTokens     *int `json:"outputTokens,omitempty"`
	CacheReadTokens  *int `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens *int `json:"cacheWriteTokens,omitempty"`
}

type cursorInferenceResult struct {
	Content          string
	ProviderSession  *interfaces.ProviderSessionMetadata
	ResponseMetadata map[string]string
}

func parseCursorInferenceResult(provider string, stdout []byte) (*cursorInferenceResult, *ProviderError) {
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed == "" {
		return nil, cursorResultParseError(provider, "cursor JSON output was empty", nil)
	}

	var payload cursorResultPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, cursorResultParseError(provider, fmt.Sprintf("cursor JSON output was not valid JSON: %v", err), err)
	}

	if payload.Type != cursorResultTypeResult {
		return nil, cursorResultParseError(
			provider,
			fmt.Sprintf("cursor JSON output had unexpected type %q, want %q", payload.Type, cursorResultTypeResult),
			nil,
		)
	}
	if payload.Subtype != cursorResultSubtypeSuccess {
		return nil, cursorResultErrorSubtype(provider, payload)
	}
	if payload.IsError {
		return nil, newProviderErrorWithDiagnostics(
			interfaces.WorkFailureTypeInternalServerError,
			"cursor JSON result reported is_error=true",
			nil,
			nil,
			nil,
		)
	}

	sessionID := strings.TrimSpace(payload.SessionID)
	if sessionID == "" {
		return nil, cursorResultParseError(provider, "cursor JSON success result is missing session_id", nil)
	}

	return &cursorInferenceResult{
		Content: payload.Result,
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: provider,
			Kind:     providerSessionKindSessionID,
			ID:       sessionID,
		},
		ResponseMetadata: cursorResponseMetadataFromPayload(payload),
	}, nil
}

func cursorResultErrorSubtype(provider string, payload cursorResultPayload) *ProviderError {
	message := fmt.Sprintf("cursor JSON output had subtype %q", payload.Subtype)
	if strings.TrimSpace(payload.Result) != "" {
		message += ": " + strings.TrimSpace(payload.Result)
	}
	errorType := interfaces.WorkFailureTypeInternalServerError
	if payload.IsError {
		errorType = interfaces.WorkFailureTypeInternalServerError
	}
	return newProviderErrorWithDiagnostics(errorType, message, nil, nil, nil)
}

func cursorResultParseError(provider, message string, cause error) *ProviderError {
	return newProviderErrorWithDiagnostics(
		interfaces.WorkFailureTypePermanentBadRequest,
		message,
		cause,
		nil,
		nil,
	)
}

func cursorResponseMetadataFromPayload(payload cursorResultPayload) map[string]string {
	metadata := make(map[string]string)
	if requestID := strings.TrimSpace(payload.RequestID); requestID != "" {
		metadata[cursorResponseMetadataRequestID] = requestID
	}
	if payload.DurationMS > 0 {
		metadata[cursorResponseMetadataDurationMS] = strconv.FormatInt(payload.DurationMS, 10)
	}
	if payload.DurationAPIMS > 0 {
		metadata[cursorResponseMetadataDurationAPIMS] = strconv.FormatInt(payload.DurationAPIMS, 10)
	}
	if payload.Usage == nil {
		return metadata
	}
	if payload.Usage.InputTokens != nil {
		metadata[cursorResponseMetadataInputTokens] = strconv.Itoa(*payload.Usage.InputTokens)
	}
	if payload.Usage.OutputTokens != nil {
		metadata[cursorResponseMetadataOutputTokens] = strconv.Itoa(*payload.Usage.OutputTokens)
	}
	if payload.Usage.CacheReadTokens != nil {
		metadata[cursorResponseMetadataCacheReadTokens] = strconv.Itoa(*payload.Usage.CacheReadTokens)
	}
	if payload.Usage.CacheWriteTokens != nil {
		metadata[cursorResponseMetadataCacheWriteTokens] = strconv.Itoa(*payload.Usage.CacheWriteTokens)
	}
	return metadata
}

func withCursorResponseMetadata(diagnostics *interfaces.WorkDiagnostics, metadata map[string]string) *interfaces.WorkDiagnostics {
	if len(metadata) == 0 {
		return diagnostics
	}
	diagnostics = interfaces.CloneWorkDiagnostics(diagnostics)
	if diagnostics == nil {
		diagnostics = &interfaces.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &interfaces.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string, len(metadata))
	}
	for key, value := range metadata {
		diagnostics.Provider.ResponseMetadata[key] = value
	}
	return diagnostics
}
