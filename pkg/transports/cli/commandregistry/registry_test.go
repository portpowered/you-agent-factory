package commandregistry_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/spf13/cobra"
)

func TestRegistry_RegisterRejectsDuplicateCommandID(t *testing.T) {
	registry := commandregistry.NewRegistry()
	handler := func(cmd *cobra.Command, args []string) error { return nil }
	if err := registry.Register("you.session.show", handler); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register("you.session.show", handler); err == nil {
		t.Fatal("Register() duplicate = nil, want error")
	}
}

func TestRegistry_LookupRejectsMissingCommandID(t *testing.T) {
	registry := commandregistry.NewRegistry()
	if _, err := registry.Lookup("you.session.show"); err == nil {
		t.Fatal("Lookup() missing = nil, want error")
	}
}

func TestRegistry_AttachRunESetsHandwrittenHandler(t *testing.T) {
	registry := commandregistry.NewRegistry()
	wantErr := errors.New("handwritten handler invoked")
	if err := registry.Register("you.session.show", func(cmd *cobra.Command, args []string) error {
		return wantErr
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	cmd := &cobra.Command{Use: "show"}
	if err := registry.AttachRunE(cmd, "you.session.show"); err != nil {
		t.Fatalf("AttachRunE() error = %v", err)
	}
	if cmd.RunE == nil {
		t.Fatal("AttachRunE() left RunE nil")
	}
	if err := cmd.RunE(cmd, nil); !errors.Is(err, wantErr) {
		t.Fatalf("RunE() error = %v, want %v", err, wantErr)
	}
}

func TestRegistry_RejectsNilRegistryOperations(t *testing.T) {
	var registry *commandregistry.Registry
	if err := registry.Register("you", noopRegistryRunE); err == nil {
		t.Fatal("Register() on nil registry = nil, want error")
	}
	if _, err := registry.Lookup("you"); err == nil {
		t.Fatal("Lookup() on nil registry = nil, want error")
	}
	if err := registry.AttachRunE(&cobra.Command{}, "you"); err == nil {
		t.Fatal("AttachRunE() on nil registry = nil, want error")
	}
}

func TestRegistry_RegisterRejectsInvalidInput(t *testing.T) {
	registry := commandregistry.NewRegistry()
	handler := noopRegistryRunE
	if err := registry.Register("", handler); err == nil {
		t.Fatal("Register() empty command ID = nil, want error")
	}
	if err := registry.Register("you.session.show", nil); err == nil {
		t.Fatal("Register() nil handler = nil, want error")
	}
}

func TestRegistry_AttachRunERejectsNilCommand(t *testing.T) {
	registry := commandregistry.NewRegistry()
	if err := registry.AttachRunE(nil, "you.session.show"); err == nil {
		t.Fatal("AttachRunE() nil command = nil, want error")
	}
}

func noopRegistryRunE(cmd *cobra.Command, args []string) error { return nil }

func noopSubmitHandler(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error {
	return nil
}

func TestNewSubmitRegistryRequiresCompleteStableHandlerCoverage(t *testing.T) {
	tests := []struct {
		name     string
		handlers commandregistry.SubmitHandlers
		wantID   string
	}{
		{
			name:     "missing unary",
			handlers: commandregistry.SubmitHandlers{SubmitBatch: noopSubmitHandler},
			wantID:   commandregistry.SubmitHandlerID,
		},
		{
			name:     "missing batch",
			handlers: commandregistry.SubmitHandlers{Submit: noopSubmitHandler},
			wantID:   commandregistry.SubmitBatchHandlerID,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := commandregistry.NewSubmitRegistry(test.handlers)
			if err == nil || !strings.Contains(err.Error(), test.wantID) {
				t.Fatalf("NewSubmitRegistry() error = %v, want stable handler ID %q", err, test.wantID)
			}
		})
	}
}

func TestSubmitRegistryVerifiesOnlyCanonicalHandlerIDs(t *testing.T) {
	registry := mustSubmitRegistry(t)
	manifest := submitRegistryManifest(t)
	if err := registry.Verify(manifest); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	for _, handlerID := range []string{
		commandregistry.SubmitHandlerID,
		commandregistry.SubmitBatchHandlerID,
	} {
		if _, err := registry.Lookup(handlerID); err != nil {
			t.Fatalf("Lookup(%q) error = %v", handlerID, err)
		}
	}
	if _, err := registry.Lookup("you.submit.unknown.handler"); err == nil {
		t.Fatal("Lookup(unknown) error = nil, want rejection")
	}
}

func TestSubmitRegistryRejectsInvalidManifestHandlers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(climanifest.Manifest)
	}{
		{
			name: "unknown",
			mutate: func(manifest climanifest.Manifest) {
				record := manifest.Commands["you.submit"]
				record.Handler.ID = "you.submit.unknown.handler"
				manifest.Commands[record.ID] = record
			},
		},
		{
			name: "missing",
			mutate: func(manifest climanifest.Manifest) {
				record := manifest.Commands["you.submit"]
				record.Handler = nil
				manifest.Commands[record.ID] = record
			},
		},
		{
			name: "duplicate",
			mutate: func(manifest climanifest.Manifest) {
				record := manifest.Commands["you.submit.batch"]
				record.Handler.ID = commandregistry.SubmitHandlerID
				manifest.Commands[record.ID] = record
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := submitRegistryManifest(t)
			test.mutate(manifest)
			if err := mustSubmitRegistry(t).Verify(manifest); err == nil {
				t.Fatal("Verify() error = nil, want rejection")
			}
		})
	}
}

func mustSubmitRegistry(t *testing.T) *commandregistry.SubmitRegistry {
	t.Helper()
	registry, err := commandregistry.NewSubmitRegistry(commandregistry.SubmitHandlers{
		Submit:      noopSubmitHandler,
		SubmitBatch: noopSubmitHandler,
	})
	if err != nil {
		t.Fatalf("NewSubmitRegistry() error = %v", err)
	}
	return registry
}

func submitRegistryManifest(t *testing.T) climanifest.Manifest {
	t.Helper()
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		t.Fatalf("RunSubmitFamilyManifest() error = %v", err)
	}
	return climanifest.Manifest{
		FormatVersion: manifest.FormatVersion,
		RootPath:      manifest.RootPath,
		Commands: map[string]climanifest.Command{
			"you.submit":       manifest.Commands["you.submit"],
			"you.submit.batch": manifest.Commands["you.submit.batch"],
		},
	}
}

func TestUnarySubmitHandlerBuildsFreshConfigFromStableInputs(t *testing.T) {
	ctx := context.WithValue(context.Background(), submitContextKey{}, "invocation")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := &cobra.Command{Use: "submit"}
	cmd.SetContext(ctx)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	local := resolveSubmitInputs(t, map[string]resolvedinput.Value{
		"you.submit.flag.name":           resolvedinput.StringValue("work-one"),
		"you.submit.flag.work-type-name": resolvedinput.StringValue("REVIEW"),
		"you.submit.flag.payload":        resolvedinput.StringValue("payload.json"),
		"you.submit.flag.session":        resolvedinput.StringValue("session-alpha"),
	})
	inherited := resolveSubmitInputs(t, map[string]resolvedinput.Value{
		"you.flag.server":  resolvedinput.StringValue("HTTPS://FACTORY.EXAMPLE/base/?token=secret"),
		"you.flag.json":    resolvedinput.BoolValue(true),
		"you.flag.verbose": resolvedinput.BoolValue(false),
		"you.flag.debug":   resolvedinput.BoolValue(true),
	})

	var received []submitcli.SubmitConfig
	handler := commandregistry.UnarySubmitHandler(func(cfg submitcli.SubmitConfig) error {
		received = append(received, cfg)
		return nil
	})
	if err := handler(cmd, local, inherited); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if len(received) != 1 {
		t.Fatalf("operation calls = %d, want 1", len(received))
	}
	assertUnarySubmitConfig(t, received[0], ctx, &stdout, &stderr)

	secondLocal := resolveSubmitInputs(t, map[string]resolvedinput.Value{
		"you.submit.flag.name":           resolvedinput.StringValue("work-two"),
		"you.submit.flag.work-type-name": resolvedinput.StringValue("BUILD"),
		"you.submit.flag.payload":        resolvedinput.StringValue("payload.md"),
		"you.submit.flag.session":        resolvedinput.StringValue(""),
	})
	if err := handler(cmd, secondLocal, inherited); err != nil {
		t.Fatalf("second handler() error = %v", err)
	}
	if len(received) != 2 || received[0].Name != "work-one" ||
		received[1].Name != "work-two" || received[1].SessionID != "" {
		t.Fatalf("invocation configs retained mutable state: %#v", received)
	}
}

func assertUnarySubmitConfig(
	t *testing.T,
	got submitcli.SubmitConfig,
	ctx context.Context,
	stdout, stderr *bytes.Buffer,
) {
	t.Helper()
	if got.Context != ctx || got.Context.Value(submitContextKey{}) != "invocation" {
		t.Fatal("context was not propagated from the invocation")
	}
	if got.Name != "work-one" || got.WorkTypeName != "REVIEW" ||
		got.Payload != "payload.json" || got.SessionID != "session-alpha" {
		t.Fatalf("local config = %#v", got)
	}
	if got.Server != "https://factory.example/base" {
		t.Fatalf("server = %q, want normalized URI without query credentials", got.Server)
	}
	if !got.JSON || !got.Verbose || !got.Debug {
		t.Fatalf("inherited modes = JSON:%t Verbose:%t Debug:%t, want all enabled", got.JSON, got.Verbose, got.Debug)
	}
	if got.Output != stdout || got.Diagnostics != stderr {
		t.Fatal("stdout/stderr writers were not propagated")
	}
}

func TestUnarySubmitHandlerRejectsInvalidRequiredStableInputsBeforeOperation(t *testing.T) {
	requiredIDs := []string{
		"you.submit.flag.name",
		"you.submit.flag.work-type-name",
		"you.submit.flag.payload",
	}
	for _, inputID := range requiredIDs {
		for _, invalid := range []struct {
			name  string
			value *resolvedinput.Value
		}{
			{name: "missing"},
			{name: "blank", value: submitValuePtr(resolvedinput.StringValue(" \t "))},
			{name: "wrong type", value: submitValuePtr(resolvedinput.BoolValue(true))},
		} {
			t.Run(inputID+"/"+invalid.name, func(t *testing.T) {
				localValues := validUnaryLocalValues()
				if invalid.value == nil {
					delete(localValues, inputID)
				} else {
					localValues[inputID] = *invalid.value
				}
				calls := 0
				handler := commandregistry.UnarySubmitHandler(func(submitcli.SubmitConfig) error {
					calls++
					return nil
				})
				err := handler(
					&cobra.Command{Use: "submit"},
					resolveSubmitInputs(t, localValues),
					validUnaryInheritedInputs(t),
				)
				if err == nil || !strings.Contains(err.Error(), inputID) {
					t.Fatalf("handler() error = %v, want stable input ID %q", err, inputID)
				}
				if strings.Contains(err.Error(), "payload-secret") {
					t.Fatalf("handler() leaked an input value: %v", err)
				}
				if calls != 0 {
					t.Fatalf("operation calls = %d, want 0", calls)
				}
			})
		}
	}
}

func TestUnarySubmitHandlerRejectsUnsafeServerWithoutEchoingIt(t *testing.T) {
	const unsafeServer = "https://user:credential@factory.example"
	inherited := resolveSubmitInputs(t, map[string]resolvedinput.Value{
		"you.flag.server":  resolvedinput.StringValue(unsafeServer),
		"you.flag.json":    resolvedinput.BoolValue(false),
		"you.flag.verbose": resolvedinput.BoolValue(false),
		"you.flag.debug":   resolvedinput.BoolValue(false),
	})
	calls := 0
	err := commandregistry.UnarySubmitHandler(func(submitcli.SubmitConfig) error {
		calls++
		return nil
	})(&cobra.Command{Use: "submit"}, resolveSubmitInputs(t, validUnaryLocalValues()), inherited)
	if err == nil || !strings.Contains(err.Error(), "you.flag.server") {
		t.Fatalf("handler() error = %v, want stable server input ID", err)
	}
	if strings.Contains(err.Error(), unsafeServer) || strings.Contains(err.Error(), "credential") {
		t.Fatalf("handler() leaked server credentials: %v", err)
	}
	if calls != 0 {
		t.Fatalf("operation calls = %d, want 0", calls)
	}
}

type submitContextKey struct{}

func validUnaryLocalValues() map[string]resolvedinput.Value {
	return map[string]resolvedinput.Value{
		"you.submit.flag.name":           resolvedinput.StringValue("work"),
		"you.submit.flag.work-type-name": resolvedinput.StringValue("REVIEW"),
		"you.submit.flag.payload":        resolvedinput.StringValue("payload-secret.json"),
		"you.submit.flag.session":        resolvedinput.StringValue(""),
	}
}

func validUnaryInheritedInputs(t *testing.T) resolvedinput.Inputs {
	t.Helper()
	return resolveSubmitInputs(t, map[string]resolvedinput.Value{
		"you.flag.server":  resolvedinput.StringValue("http://localhost:7437"),
		"you.flag.json":    resolvedinput.BoolValue(false),
		"you.flag.verbose": resolvedinput.BoolValue(false),
		"you.flag.debug":   resolvedinput.BoolValue(false),
	})
}

func resolveSubmitInputs(
	t *testing.T,
	values map[string]resolvedinput.Value,
) resolvedinput.Inputs {
	t.Helper()
	definitions := make([]resolvedinput.Definition, 0, len(values))
	candidates := make([]resolvedinput.Candidate, 0, len(values))
	for inputID, value := range values {
		definitions = append(definitions, resolvedinput.Definition{
			ID: inputID, Kind: value.Kind(),
			Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag},
		})
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: inputID, Source: resolvedinput.SourceCLIFlag, Value: value,
		})
	}
	inputs, err := resolvedinput.Resolve(definitions, candidates)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	return inputs
}

func submitValuePtr(value resolvedinput.Value) *resolvedinput.Value { return &value }
