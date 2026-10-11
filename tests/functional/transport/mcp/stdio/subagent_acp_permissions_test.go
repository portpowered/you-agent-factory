package stdio_test

import "testing"

func mcpToolResultText(t *testing.T, result map[string]any) string {
	t.Helper()
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("tool result content = %#v, want non-empty content", result["content"])
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("tool result content item = %#v, want object", content[0])
	}
	text, _ := item["text"].(string)
	if text == "" {
		t.Fatalf("tool result content item text = %#v, want non-empty text", item["text"])
	}
	return text
}
