package application

import (
	"strings"
	"testing"
)

func TestNormalizeSnapshotsArgumentsAndEnvironment(t *testing.T) {
	args := []string{"custom-you", "docs", "--", "", "--topic", "--topic"}
	environment := []string{"PRESENT=first", "EMPTY=", "PRESENT=last"}

	input, err := normalize(Input{Args: args, Env: environment, WorkingDirectory: t.TempDir()})
	if err != nil {
		t.Fatalf("normalize() error = %v", err)
	}
	args[1] = "changed"
	environment[0] = "PRESENT=changed"

	if input.executableName() != "custom-you" {
		t.Fatalf("executableName() = %q, want custom-you", input.executableName())
	}
	wantArguments := []string{"docs", "--", "", "--topic", "--topic"}
	if got := strings.Join(input.argumentsCopy(), "\x00"); got != strings.Join(wantArguments, "\x00") {
		t.Fatalf("argumentsCopy() = %q, want %q", input.argumentsCopy(), wantArguments)
	}
	returned := input.argumentsCopy()
	returned[0] = "mutated"
	if input.argumentsCopy()[0] != "docs" {
		t.Fatal("Arguments() exposed mutable normalized state")
	}
	if value, ok := input.lookupEnv("PRESENT"); !ok || value != "last" {
		t.Fatalf("LookupEnv(PRESENT) = %q, %t; want last, true", value, ok)
	}
	if value, ok := input.lookupEnv("EMPTY"); !ok || value != "" {
		t.Fatalf("LookupEnv(EMPTY) = %q, %t; want empty, true", value, ok)
	}
	if _, ok := input.lookupEnv("ABSENT"); ok {
		t.Fatal("LookupEnv(ABSENT) reported an absent value as present")
	}
}

func TestNormalizeIgnoresWindowsReservedEnvironmentEntries(t *testing.T) {
	t.Parallel()

	input, err := normalize(Input{
		Args:             []string{"you"},
		Env:              []string{"=::=::\\", "PRESENT=value"},
		WorkingDirectory: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("normalize() error = %v", err)
	}
	if value, ok := input.lookupEnv("PRESENT"); !ok || value != "value" {
		t.Fatalf("LookupEnv(PRESENT) = %q, %t; want value, true", value, ok)
	}
	if _, ok := input.lookupEnv(""); ok {
		t.Fatal("LookupEnv reported a Windows reserved environment entry")
	}
}

func TestHomeDirRejectsMissingEnvironment(t *testing.T) {
	input, err := normalize(Input{Args: []string{"you"}, WorkingDirectory: t.TempDir()})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if _, err := homeDir(input); err == nil {
		t.Fatal("homeDir succeeded without a home environment variable")
	}
}

func TestHomeDirUsesSuppliedEnvironmentWithoutHostPlatformSelection(t *testing.T) {
	t.Parallel()

	input, err := normalize(Input{
		Args:             []string{"you"},
		WorkingDirectory: t.TempDir(),
		Env: []string{
			"HOME=/unix-home",
			"USERPROFILE=C:\\operator-home",
			"HOMEDRIVE=D:",
			"HOMEPATH=\\fallback-home",
		},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	home, err := homeDir(input)
	if err != nil {
		t.Fatalf("homeDir: %v", err)
	}
	if home != `C:\operator-home` {
		t.Fatalf("homeDir = %q, want supplied USERPROFILE", home)
	}
}

func TestNormalizeRejectsMissingExecutableAndMalformedEnvironment(t *testing.T) {
	t.Parallel()

	if _, err := normalize(Input{}); err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("normalize(missing executable) error = %v", err)
	}
	if _, err := normalize(Input{Args: []string{"you"}, Env: []string{"MALFORMED"}}); err == nil ||
		!strings.Contains(err.Error(), "environment") {
		t.Fatalf("normalize(malformed environment) error = %v", err)
	}
}

func TestNormalizeRejectsMissingWorkingDirectory(t *testing.T) {
	t.Parallel()

	_, err := normalize(Input{Args: []string{"you"}})
	if err == nil || !strings.Contains(err.Error(), "working directory") {
		t.Fatalf("normalize(missing working directory) error = %v", err)
	}
}
