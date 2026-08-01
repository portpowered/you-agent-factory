package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNewFamilyRegistryCopiesOrderAndForwardsRawCLIValues(t *testing.T) {
	t.Parallel()

	wantCommand := &cobra.Command{Use: "owner"}
	wantArguments := []string{"--raw", "value"}
	wantError := errors.New("owner failed")
	var gotCommand *cobra.Command
	var gotArguments []string
	registry, err := NewFamilyRegistry([]CommandFamily{{
		Name: " owner ",
		Handler: func(command *cobra.Command, arguments []string) error {
			gotCommand, gotArguments = command, arguments
			return wantError
		},
	}})
	if err != nil {
		t.Fatalf("NewFamilyRegistry() error = %v", err)
	}

	families := registry.Families()
	families[0].Name = "mutated"
	if registry.Families()[0].Name != "owner" {
		t.Fatalf("Families() did not return a detached catalog: %#v", registry.Families())
	}
	if err := registry.Dispatch("owner", wantCommand, wantArguments); !errors.Is(err, wantError) {
		t.Fatalf("Dispatch() error = %v, want owner error %v", err, wantError)
	}
	if gotCommand != wantCommand || len(gotArguments) != len(wantArguments) || &gotArguments[0] != &wantArguments[0] {
		t.Fatalf("Dispatch() forwarded (%p, %#v), want exact command and argument slice", gotCommand, gotArguments)
	}
}

func TestNewFamilyRegistryRejectsInvalidFamiliesAndDispatchInputs(t *testing.T) {
	t.Parallel()

	handler := func(*cobra.Command, []string) error { return nil }
	for _, test := range []struct {
		name     string
		families []CommandFamily
		want     string
	}{
		{name: "empty", want: "at least one family"},
		{name: "missing name", families: []CommandFamily{{Handler: handler}}, want: "has no name"},
		{name: "missing handler", families: []CommandFamily{{Name: "owner"}}, want: "has no handler"},
		{name: "duplicate", families: []CommandFamily{{Name: "owner", Handler: handler}, {Name: " owner ", Handler: handler}}, want: "duplicate family"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewFamilyRegistry(test.families)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewFamilyRegistry() error = %v, want substring %q", err, test.want)
			}
		})
	}

	registry, err := NewFamilyRegistry([]CommandFamily{{Name: "owner", Handler: handler}})
	if err != nil {
		t.Fatalf("NewFamilyRegistry() error = %v", err)
	}
	if err := registry.Dispatch("missing", &cobra.Command{}, nil); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unknown Dispatch() error = %v", err)
	}
	if err := registry.Dispatch("owner", nil, nil); err == nil || !strings.Contains(err.Error(), "requires a command") {
		t.Fatalf("nil command Dispatch() error = %v", err)
	}
}
