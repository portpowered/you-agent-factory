package agypty_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/workers/agypty"
)

func TestArgvFixtures(t *testing.T) {
	t.Parallel()

	fixtures, err := agypty.LoadArgvFixtures()
	if err != nil {
		t.Fatalf("LoadArgvFixtures() error = %v", err)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			switch {
			case fixture.Spec != nil:
				argv, err := agypty.BuildArgv(*fixture.Spec)
				if err != nil {
					t.Fatalf("BuildArgv() error = %v", err)
				}
				if err := agypty.ValidateArgv(argv); err != nil {
					t.Fatalf("ValidateArgv() error = %v", err)
				}
				assertStringSliceEqual(t, argv, fixture.WantArgv)
			case len(fixture.Argv) > 0:
				err := agypty.ValidateArgv(fixture.Argv)
				if fixture.WantError == "" {
					if err != nil {
						t.Fatalf("ValidateArgv() error = %v, want nil", err)
					}
					return
				}
				if err == nil {
					t.Fatal("ValidateArgv() error = nil, want error")
				}
				if !strings.Contains(err.Error(), fixture.WantError) {
					t.Fatalf("ValidateArgv() error = %v, want substring %q", err, fixture.WantError)
				}
			default:
				t.Fatal("fixture must set spec or argv")
			}
		})
	}
}

func TestBuildArgv_RejectsEmptyExecutable(t *testing.T) {
	t.Parallel()

	if _, err := agypty.BuildArgv(agypty.ArgvSpec{}); err == nil {
		t.Fatal("BuildArgv() error = nil, want error")
	}
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q (full %#v)", i, got[i], want[i], got)
		}
	}
}
