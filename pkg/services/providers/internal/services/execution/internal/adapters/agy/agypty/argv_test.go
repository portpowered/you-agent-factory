package agypty_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
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

func TestWorkspaceFixtures(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	fixtures, err := agypty.LoadWorkspaceFixtures()
	if err != nil {
		t.Fatalf("LoadWorkspaceFixtures() error = %v", err)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			root := factoryRoot
			if fixture.FactoryRoot != "FACTORY_ROOT" {
				root = fixture.FactoryRoot
			}

			got, err := agypty.ResolveWorkspaceDir(root, fixture.RawPath)
			if fixture.WantError != "" {
				if err == nil {
					t.Fatal("ResolveWorkspaceDir() error = nil, want error")
				}
				if !strings.Contains(err.Error(), fixture.WantError) {
					t.Fatalf("ResolveWorkspaceDir() error = %v, want substring %q", err, fixture.WantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveWorkspaceDir() error = %v", err)
			}

			want := filepath.Join(append([]string{root}, fixture.WantSuffix...)...)
			if got != want {
				t.Fatalf("ResolveWorkspaceDir() = %q, want %q", got, want)
			}
			if !strings.HasPrefix(got, root+string(filepath.Separator)) && got != root {
				t.Fatalf("resolved path %q is not under factory root %q", got, root)
			}
		})
	}
}

func TestResolveWorkspaceDir_RejectsEmptyFactoryRoot(t *testing.T) {
	t.Parallel()

	if _, err := agypty.ResolveWorkspaceDir("", "workspaces/a"); err == nil {
		t.Fatal("ResolveWorkspaceDir() error = nil, want error")
	}
}

func TestResolveWorkspaceDir_RejectsAbsoluteOutsideRoot(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	outside := filepath.Join(filepath.Dir(factoryRoot), "outside-agypty-workspace")

	if _, err := agypty.ResolveWorkspaceDir(factoryRoot, outside); err == nil {
		t.Fatal("ResolveWorkspaceDir() error = nil, want error for path outside factory root")
	}
}

func TestTerminalCleaningCorpus(t *testing.T) {
	t.Parallel()

	corpus, err := agypty.LoadTerminalCleaningCorpus()
	if err != nil {
		t.Fatalf("LoadTerminalCleaningCorpus() error = %v", err)
	}

	for _, fixture := range corpus.Cases() {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			raw, err := fixture.RawBytes()
			if err != nil {
				t.Fatalf("RawBytes() error = %v", err)
			}
			got := agypty.CleanTerminal(raw)
			if fixture.Empty {
				if got != "" {
					t.Fatalf("CleanTerminal() = %q, want empty", got)
				}
				return
			}
			if got != fixture.Want {
				t.Fatalf("CleanTerminal() = %q, want %q", got, fixture.Want)
			}
			if agypty.ContainsTerminalEscapeOrControl(got) {
				t.Fatalf("CleanTerminal() = %q still contains terminal escape or control bytes", got)
			}
		})
	}
}

func TestCleanTerminal_StripsANSICarriageReturnNoise(t *testing.T) {
	t.Parallel()

	raw := []byte("spinning\ranswer\x1b[2K\nignored blank\n")
	got := agypty.CleanTerminal(raw)
	want := "answer\nignored blank"
	if got != want {
		t.Fatalf("CleanTerminal() = %q, want %q", got, want)
	}
	if agypty.ContainsTerminalEscapeOrControl(got) {
		t.Fatalf("CleanTerminal() = %q still contains terminal escape or control bytes", got)
	}
}

func TestSessionResult_KeepsRawBytesInternal(t *testing.T) {
	t.Parallel()

	raw := []byte("spinning\ranswer\x1b[2K\n")
	cleaned := agypty.CleanTerminal(raw)
	result := agypty.SessionResult{
		ExitCode:    124,
		RawBytes:    append([]byte(nil), raw...),
		CleanedText: cleaned,
		TimedOut:    true,
	}
	if string(result.RawBytes) != string(raw) {
		t.Fatalf("RawBytes = %q, want preserved raw capture %q", result.RawBytes, raw)
	}
	if result.CleanedText != "answer" {
		t.Fatalf("CleanedText = %q, want cleaned public-safe text", result.CleanedText)
	}
	if agypty.ContainsTerminalEscapeOrControl(result.CleanedText) {
		t.Fatalf("CleanedText = %q still contains terminal escape or control bytes", result.CleanedText)
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
