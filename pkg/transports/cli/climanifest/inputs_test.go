package climanifest_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

func TestCommand_InputLookups(t *testing.T) {
	command := climanifest.Command{
		ID: "you.session.show",
		Arguments: map[string]climanifest.Argument{
			"session-id": {
				Position: 0,
				Name:     "session-id",
			},
		},
		Flags: map[string]climanifest.Flag{
			"json": {Long: "json"},
		},
	}

	arg, ok := command.ArgumentAt(0)
	if !ok || arg.Name != "session-id" {
		t.Fatalf("ArgumentAt(0) = %+v, %t; want session-id, true", arg, ok)
	}
	if _, ok := command.ArgumentAt(1); ok {
		t.Fatal("ArgumentAt(1) ok = true, want false")
	}

	flag, ok := command.FlagByLong("json")
	if !ok || flag.Long != "json" {
		t.Fatalf("FlagByLong(json) = %+v, %t; want json, true", flag, ok)
	}
	if _, ok := command.FlagByLong("missing"); ok {
		t.Fatal("FlagByLong(missing) ok = true, want false")
	}

	if _, err := command.RequireArgumentAt(0); err != nil {
		t.Fatalf("RequireArgumentAt(0) error = %v", err)
	}
	if _, err := command.RequireArgumentAt(1); err == nil {
		t.Fatal("RequireArgumentAt(1) error = nil, want missing argument failure")
	}
	if _, err := command.RequireFlagByLong("json"); err != nil {
		t.Fatalf("RequireFlagByLong(json) error = %v", err)
	}
	if _, err := command.RequireFlagByLong("missing"); err == nil {
		t.Fatal("RequireFlagByLong(missing) error = nil, want missing flag failure")
	}
}

func TestCommand_InputLookups_NilMaps(t *testing.T) {
	command := climanifest.Command{ID: "you"}
	if _, ok := command.ArgumentAt(0); ok {
		t.Fatal("ArgumentAt on nil map ok = true, want false")
	}
	if _, ok := command.FlagByLong("json"); ok {
		t.Fatal("FlagByLong on nil map ok = true, want false")
	}
}
