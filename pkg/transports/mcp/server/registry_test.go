package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNewRegistryCopiesDefinitionsAndForwardsRawCalls(t *testing.T) {
	t.Parallel()

	arguments := json.RawMessage(`{"nested":{"value":true}}`)
	inputSchema := json.RawMessage(`{"type":"object","properties":{"nested":{"type":"object"}}}`)
	callerContext := context.WithValue(context.Background(), struct{}{}, "caller")
	var seenContext context.Context
	var seenName string
	var seenArguments json.RawMessage
	registry, err := NewRegistry([]ToolDefinition{{
		Name:        " owner.tool ",
		Description: "description",
		InputSchema: inputSchema,
		Operation: func(ctx context.Context, name string, got json.RawMessage) (json.RawMessage, error) {
			seenContext = ctx
			seenName = name
			seenArguments = got
			return json.RawMessage(`{"ok":true}`), nil
		},
	}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	// Mutations after construction must not change the composed protocol
	// definition or the argument bytes observed by the owner operation.
	inputSchema[0] = 'x'
	definitions := registry.Definitions()
	if len(definitions) != 1 || definitions[0].Name != "owner.tool" {
		t.Fatalf("Definitions() = %#v, want normalized one-tool catalog", definitions)
	}
	definitions[0].InputSchema[0] = 'x'
	if string(registry.Definitions()[0].InputSchema) != `{"type":"object","properties":{"nested":{"type":"object"}}}` {
		t.Fatalf("registry schema was not detached: %s", registry.Definitions()[0].InputSchema)
	}

	result, err := registry.Call(callerContext, "owner.tool", arguments)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if string(result) != `{"ok":true}` {
		t.Fatalf("Call() result = %s, want owner response", result)
	}
	if seenContext != callerContext || seenName != "owner.tool" || string(seenArguments) != string(arguments) {
		t.Fatalf("forwarded call = (context %v, name %q, arguments %s), want unchanged values", seenContext, seenName, seenArguments)
	}
}

func TestNewRegistryRejectsInvalidDefinitions(t *testing.T) {
	t.Parallel()

	operation := func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	}
	cases := []struct {
		name        string
		definitions []ToolDefinition
		want        string
	}{
		{name: "empty", want: "at least one tool"},
		{name: "missing name", definitions: []ToolDefinition{{Operation: operation, InputSchema: json.RawMessage(`{"type":"object"}`)}}, want: "has no name"},
		{name: "missing operation", definitions: []ToolDefinition{{Name: "tool", InputSchema: json.RawMessage(`{"type":"object"}`)}}, want: "has no operation"},
		{name: "missing schema", definitions: []ToolDefinition{{Name: "tool", Operation: operation}}, want: "has no input schema"},
		{name: "duplicate", definitions: []ToolDefinition{
			{Name: "tool", InputSchema: json.RawMessage(`{"type":"object"}`), Operation: operation},
			{Name: " tool ", InputSchema: json.RawMessage(`{"type":"object"}`), Operation: operation},
		}, want: "duplicate tool"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewRegistry(test.definitions)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewRegistry() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRegistryCallReportsUnknownToolAndPreservesCancellation(t *testing.T) {
	t.Parallel()

	operation := func(ctx context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
	registry, err := NewRegistry([]ToolDefinition{{
		Name: "tool", InputSchema: json.RawMessage(`{"type":"object"}`), Operation: operation,
	}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	if _, err := registry.Call(context.Background(), "missing", nil); err == nil || !strings.Contains(err.Error(), `unknown tool "missing"`) {
		t.Fatalf("unknown Call() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := registry.Call(ctx, "tool", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Call() error = %v, want context.Canceled", err)
	}
}
