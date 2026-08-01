package wire

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	transportcli "github.com/portpowered/infinite-you/pkg/transports/cli"
	transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
	"github.com/spf13/cobra"
)

func TestNewTransportAggregateRetainsOneIdentityPerProtocol(t *testing.T) {
	t.Parallel()

	httpForwarder := testHTTPForwarder(t)
	cliRegistry, err := transportcli.NewFamilyRegistry([]transportcli.CommandFamily{{
		Name: "owner", Handler: func(*cobra.Command, []string) error { return nil },
	}})
	if err != nil {
		t.Fatalf("NewFamilyRegistry() error = %v", err)
	}
	mcpRegistry, err := mcpserver.NewRegistry([]mcpserver.ToolDefinition{{
		Name:        "owner.tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Operation: func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		},
	}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	aggregate, err := NewTransportAggregate(httpForwarder, cliRegistry, mcpRegistry)
	if err != nil {
		t.Fatalf("NewTransportAggregate() error = %v", err)
	}
	if aggregate.HTTP != httpForwarder || aggregate.HTTPHandler == nil || aggregate.CLI != cliRegistry || aggregate.MCP != mcpRegistry {
		t.Fatalf("aggregate identities = (HTTP:%p HTTPHandler:%T CLI:%p MCP:%T), want supplied protocol roles", aggregate.HTTP, aggregate.HTTPHandler, aggregate.CLI, aggregate.MCP)
	}
}

func TestNewTransportAggregateRejectsMissingProtocolRole(t *testing.T) {
	t.Parallel()

	httpForwarder := testHTTPForwarder(t)
	cliRegistry, err := transportcli.NewFamilyRegistry([]transportcli.CommandFamily{{
		Name: "owner", Handler: func(*cobra.Command, []string) error { return nil },
	}})
	if err != nil {
		t.Fatalf("NewFamilyRegistry() error = %v", err)
	}
	mcpRegistry, err := mcpserver.NewRegistry([]mcpserver.ToolDefinition{{
		Name:        "owner.tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Operation: func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
			return nil, nil
		},
	}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	for _, test := range []struct {
		name string
		call func() (*TransportAggregate, error)
		want string
	}{
		{name: "http", call: func() (*TransportAggregate, error) {
			return NewTransportAggregate(nil, cliRegistry, mcpRegistry)
		}, want: "HTTP forwarder"},
		{name: "cli", call: func() (*TransportAggregate, error) {
			return NewTransportAggregate(httpForwarder, nil, mcpRegistry)
		}, want: "CLI family registry"},
		{name: "mcp", call: func() (*TransportAggregate, error) {
			return NewTransportAggregate(httpForwarder, cliRegistry, nil)
		}, want: "MCP tool registry"},
	} {
		t.Run(test.name, func(t *testing.T) {
			aggregate, err := test.call()
			if aggregate != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewTransportAggregate() = (%#v, %v), want %q error", aggregate, err, test.want)
			}
		})
	}
}

func testHTTPForwarder(t *testing.T) *transporthttp.Forwarder {
	t.Helper()
	var handlers transporthttp.ForwarderHandlers
	value := reflect.ValueOf(&handlers).Elem()
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		field.Set(reflect.MakeFunc(field.Type(), func([]reflect.Value) []reflect.Value { return nil }))
	}
	forwarder, err := transporthttp.NewForwarder(handlers)
	if err != nil {
		t.Fatalf("NewForwarder() error = %v", err)
	}
	return forwarder
}
