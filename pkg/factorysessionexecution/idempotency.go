package factorysessionexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// IdempotencyTupleHash returns a stable digest for the normalized execution tuple
// used to compare replay safety for one requestId.
func IdempotencyTupleHash(req StartRequest) (string, error) {
	normalized, err := normalizeIdempotencyDocument(req)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal idempotency tuple: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// CheckRequestIDReplay reports ErrExecutionRequestIDConflict when requestId was
// previously recorded with a different normalized tuple.
func CheckRequestIDReplay(requestID, recordedHash, incomingHash string) error {
	if strings.TrimSpace(requestID) == "" {
		return NewValidationError("requestId", "requestId is required")
	}
	if recordedHash == "" || recordedHash == incomingHash {
		return nil
	}
	return ErrExecutionRequestIDConflict
}

func normalizeIdempotencyDocument(req StartRequest) (map[string]any, error) {
	source, err := normalizeSourceForIdempotency(req.Source)
	if err != nil {
		return nil, err
	}
	document := map[string]any{
		"source": source,
	}
	if len(req.Args) > 0 {
		args, err := canonicalizeMap(req.Args)
		if err != nil {
			return nil, err
		}
		document["args"] = args
	}
	if req.Orchestrator != nil {
		orchestrator, err := canonicalizeRawJSON(req.Orchestrator.Raw)
		if err != nil {
			return nil, err
		}
		document["orchestrator"] = orchestrator
	}
	if policy := normalizeRequestedPolicyForIdempotency(req.RequestedPolicy); policy != nil {
		document["requestedPolicy"] = policy
	}
	return document, nil
}

func normalizeSourceForIdempotency(source Source) (map[string]any, error) {
	document := map[string]any{
		"kind": string(source.Kind),
	}
	switch source.Kind {
	case workflowsource.KindFactoryID:
		document["factoryId"] = strings.TrimSpace(source.FactoryID)
	case workflowsource.KindFactoryInline:
		inline, err := canonicalizeRawJSON(source.FactoryInline)
		if err != nil {
			return nil, err
		}
		document["factoryInline"] = inline
	case workflowsource.KindWorkflowFile:
		document["workflowFile"] = strings.TrimSpace(source.WorkflowFile)
	case workflowsource.KindWorkflowName:
		document["workflowName"] = strings.TrimSpace(source.WorkflowName)
	case workflowsource.KindInlineWorkflow:
		if source.InlineWorkflow == nil {
			return nil, NewValidationError("source.inlineWorkflow", "inlineWorkflow is required when source.kind is INLINE_WORKFLOW")
		}
		inline := map[string]any{
			"inlineSource": strings.TrimSpace(source.InlineWorkflow.InlineSource),
		}
		if dialect := strings.TrimSpace(source.InlineWorkflow.Dialect); dialect != "" {
			inline["dialect"] = dialect
		}
		if entrypoint := strings.TrimSpace(source.InlineWorkflow.Entrypoint); entrypoint != "" {
			inline["entrypoint"] = entrypoint
		}
		if len(source.InlineWorkflow.Metadata) > 0 {
			inline["metadata"] = canonicalizeStringMap(source.InlineWorkflow.Metadata)
		}
		document["inlineWorkflow"] = inline
	default:
		return nil, NewValidationError("source.kind", "source.kind is invalid")
	}
	return document, nil
}

func normalizeRequestedPolicyForIdempotency(policy map[string]any) any {
	if len(policy) == 0 {
		return nil
	}
	if hash, ok := policy["policyHash"].(string); ok && strings.TrimSpace(hash) != "" {
		return map[string]string{"policyHash": strings.TrimSpace(hash)}
	}
	canonical, err := canonicalizeMap(policy)
	if err != nil {
		return policy
	}
	return canonical
}

func canonicalizeRawJSON(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("parse json document: %w", err)
	}
	return canonicalizeValue(value)
}

func canonicalizeMap(document map[string]any) (map[string]any, error) {
	if len(document) == 0 {
		return nil, nil
	}
	value, err := canonicalizeValue(document)
	if err != nil {
		return nil, err
	}
	canonical, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected object, got %T", value)
	}
	return canonical, nil
}

func canonicalizeStringMap(document map[string]string) map[string]string {
	if len(document) == 0 {
		return nil
	}
	keys := make([]string, 0, len(document))
	for key := range document {
		keys = append(keys, key)
	}
	sortStrings(keys)
	out := make(map[string]string, len(document))
	for _, key := range keys {
		out[key] = document[key]
	}
	return out
}

func canonicalizeValue(value any) (any, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sortStrings(keys)
		out := make(map[string]any, len(typed))
		for _, key := range keys {
			canonical, err := canonicalizeValue(typed[key])
			if err != nil {
				return nil, err
			}
			out[key] = canonical
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			canonical, err := canonicalizeValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, canonical)
		}
		return out, nil
	default:
		return typed, nil
	}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
