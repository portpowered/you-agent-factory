// Package replay owns policy-free replay artifact persistence mechanics.
package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	artifactReplaceAttempts = 20
	artifactReplaceDelay    = 10 * time.Millisecond
)

// WriteFile atomically replaces path with a completed artifact snapshot.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create replay artifact directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create replay artifact temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write replay artifact temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync replay artifact temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close replay artifact temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err == nil {
		cleanupTemp = false
		return nil
	} else if runtime.GOOS != "windows" {
		return fmt.Errorf("replace replay artifact with temp file: %w; temp artifact left at %s", err, tmpPath)
	}

	var replaceErr error
	for range artifactReplaceAttempts {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			replaceErr = fmt.Errorf("remove previous replay artifact before replace: %w", err)
		} else if err := os.Rename(tmpPath, path); err != nil {
			replaceErr = fmt.Errorf("replace replay artifact from temp file: %w", err)
		} else {
			cleanupTemp = false
			return nil
		}
		time.Sleep(artifactReplaceDelay)
	}
	return fmt.Errorf("%w; temp artifact left at %s", replaceErr, tmpPath)
}

// ReadFile reads one artifact snapshot, retrying transient Windows replacement
// races without interpreting the artifact contents.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil || runtime.GOOS != "windows" {
		return data, err
	}

	lastErr := err
	for range artifactReplaceAttempts {
		time.Sleep(artifactReplaceDelay)
		data, err = os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

const unavailableHistoricalFailureMessage = "Failure details were not recorded in this historical event."

var canonicalFailureReasons = map[string]struct{}{
	"auth_failure": {}, "misconfigured": {}, "permanent_bad_request": {},
	"internal_server_error": {}, "throttled": {}, "timeout": {}, "unknown": {},
}

// NormalizeHistoricalFailureDetails translates legacy diagnostic fields before
// the domain-owned artifact codec decodes its canonical events.
func NormalizeHistoricalFailureDetails(data []byte) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if payload, ok := root["payload"].(map[string]any); ok {
		normalizeHistoricalFailureObject(payload)
	}
	if events, ok := root["events"].([]any); ok {
		for _, value := range events {
			event, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if payload, ok := event["payload"].(map[string]any); ok {
				normalizeHistoricalFailureObject(payload)
			}
		}
	}
	return json.Marshal(root)
}

func normalizeHistoricalFailureObject(object map[string]any) {
	detail, validCanonical := validHistoricalFailureDetail(object["failureDetail"])
	legacyReason, hasLegacyReason := trimmedString(object["failureReason"])
	legacyMessage, hasLegacyMessage := trimmedString(object["failureMessage"])
	errorClass, hasErrorClass := trimmedString(object["errorClass"])
	delete(object, "failureReason")
	delete(object, "failureMessage")
	delete(object, "errorClass")
	if validCanonical {
		object["failureDetail"] = detail
		return
	}
	if !hasLegacyReason && !hasLegacyMessage && !hasErrorClass {
		return
	}
	reason := "unknown"
	if hasLegacyReason {
		reason = normalizedHistoricalFailureReason(legacyReason)
	} else if hasErrorClass {
		reason = normalizedHistoricalFailureReason(errorClass)
	}
	if !hasLegacyMessage {
		legacyMessage = unavailableHistoricalFailureMessage
	}
	object["failureDetail"] = map[string]any{"reason": reason, "message": legacyMessage}
}

func validHistoricalFailureDetail(value any) (map[string]any, bool) {
	detail, ok := value.(map[string]any)
	if !ok || len(detail) != 2 {
		return nil, false
	}
	reason, hasReason := trimmedString(detail["reason"])
	message, hasMessage := trimmedString(detail["message"])
	if !hasReason || !hasMessage {
		return nil, false
	}
	if _, ok := canonicalFailureReasons[reason]; !ok {
		return nil, false
	}
	return map[string]any{"reason": reason, "message": message}, true
}

func normalizedHistoricalFailureReason(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	if _, ok := canonicalFailureReasons[normalized]; ok {
		return normalized
	}
	return "unknown"
}

func trimmedString(value any) (string, bool) {
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	return text, ok && text != ""
}
