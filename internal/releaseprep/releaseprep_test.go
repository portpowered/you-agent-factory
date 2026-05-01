package releaseprep

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunValidatesVersionBeforeShellingOut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "missing version", version: "", want: "release version is required"},
		{name: "invalid version", version: "1.2.3", want: "must match vMAJOR.MINOR.PATCH"},
		{name: "prerelease version", version: "v1.2.3-rc1", want: "must match vMAJOR.MINOR.PATCH"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{}
			err := Run(context.Background(), Options{
				Version: tt.version,
				Runner:  runner,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run() error = %v, want substring %q", err, tt.want)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("Run() made %d command calls, want none", len(runner.calls))
			}
		})
	}
}

func TestRunRejectsWrongBranch(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		outputs: map[string]result{
			"git branch --show-current": {stdout: "feature\n"},
		},
	}

	err := Run(context.Background(), Options{
		Version: "v1.2.3",
		Runner:  runner,
	})
	if err == nil || !strings.Contains(err.Error(), "must run from main") {
		t.Fatalf("Run() error = %v, want main branch failure", err)
	}
}

func TestRunRejectsDirtyWorkingTree(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		outputs: map[string]result{
			"git branch --show-current": {stdout: "main\n"},
			"git status --short":        {stdout: " M Makefile\n"},
		},
	}

	err := Run(context.Background(), Options{
		Version: "v1.2.3",
		Runner:  runner,
	})
	if err == nil || !strings.Contains(err.Error(), "clean working tree") {
		t.Fatalf("Run() error = %v, want clean-tree failure", err)
	}
}

func TestRunRejectsExistingTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outputs map[string]result
		want    string
	}{
		{
			name: "local tag exists",
			outputs: map[string]result{
				"git branch --show-current":                    {stdout: "main\n"},
				"git status --short":                           {},
				"git tag --list v1.2.3":                        {stdout: "v1.2.3\n"},
				"git ls-remote --tags origin refs/tags/v1.2.3": {},
			},
			want: "already exists locally",
		},
		{
			name: "remote tag exists",
			outputs: map[string]result{
				"git branch --show-current":                    {stdout: "main\n"},
				"git status --short":                           {},
				"git tag --list v1.2.3":                        {},
				"git ls-remote --tags origin refs/tags/v1.2.3": {stdout: "abc\trefs/tags/v1.2.3\n"},
			},
			want: "already exists on origin",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeRunner{outputs: tt.outputs}
			err := Run(context.Background(), Options{
				Version: "v1.2.3",
				Runner:  runner,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRunExecutesReadinessTargetsThenTagsAndPushes(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		outputs: map[string]result{
			"git branch --show-current":                    {stdout: "main\n"},
			"git status --short":                           {},
			"git tag --list v1.2.3":                        {},
			"git ls-remote --tags origin refs/tags/v1.2.3": {},
			"make ui-deps":                                 {},
			"make typecheck":                               {},
			"make ui-build":                                {},
			"make build":                                   {},
			"make lint":                                    {},
			"make api-smoke":                               {},
			"make ui-test":                                 {},
			"make test":                                    {},
			"git tag v1.2.3":                               {},
			"git push origin refs/tags/v1.2.3":             {},
		},
	}

	var progress strings.Builder
	err := Run(context.Background(), Options{
		Version:        "v1.2.3",
		Runner:         runner,
		ProgressWriter: &progress,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantCalls := []string{
		"git branch --show-current",
		"git status --short",
		"git tag --list v1.2.3",
		"git ls-remote --tags origin refs/tags/v1.2.3",
		"make ui-deps",
		"make typecheck",
		"make ui-build",
		"make build",
		"make lint",
		"make api-smoke",
		"make ui-test",
		"make test",
		"git tag v1.2.3",
		"git push origin refs/tags/v1.2.3",
	}
	if strings.Join(runner.calls, "\n") != strings.Join(wantCalls, "\n") {
		t.Fatalf("Run() calls = %v, want %v", runner.calls, wantCalls)
	}
	if !strings.Contains(progress.String(), "prepared v1.2.3 from main") {
		t.Fatalf("Run() progress = %q, want completion message", progress.String())
	}
}

type fakeRunner struct {
	calls   []string
	outputs map[string]result
}

type result struct {
	stdout string
	err    error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, key)
	if f.outputs == nil {
		return "", nil
	}
	res, ok := f.outputs[key]
	if !ok {
		return "", errors.New("unexpected command: " + key)
	}
	return res.stdout, res.err
}
