package releaseprep

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

const (
	defaultBranch = "main"
	defaultRemote = "origin"
)

var semverTagPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

var readinessTargets = []string{
	"typecheck",
	"ui-deps",
	"ui-build",
	"build",
	"lint",
	"api-smoke",
	"ui-test",
	"test",
}

type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type Options struct {
	Version        string
	Branch         string
	Remote         string
	MakeCommand    string
	ProgressWriter io.Writer
	Runner         Runner
}

func Run(ctx context.Context, opts Options) error {
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		return errors.New("release version is required; use make release VERSION=v1.2.3")
	}
	if !semverTagPattern.MatchString(version) {
		return fmt.Errorf("release version %q must match vMAJOR.MINOR.PATCH", version)
	}

	branch := valueOrDefault(opts.Branch, defaultBranch)
	remote := valueOrDefault(opts.Remote, defaultRemote)
	makeCommand := valueOrDefault(opts.MakeCommand, "make")
	progress := opts.ProgressWriter
	if progress == nil {
		progress = io.Discard
	}
	if opts.Runner == nil {
		opts.Runner = commandRunner{}
	}

	currentBranch, err := runTrimmed(ctx, opts.Runner, "git", "branch", "--show-current")
	if err != nil {
		return fmt.Errorf("detect current branch: %w", err)
	}
	if currentBranch != branch {
		return fmt.Errorf("release preparation must run from %s; current branch is %s", branch, currentBranch)
	}

	status, err := runTrimmed(ctx, opts.Runner, "git", "status", "--short")
	if err != nil {
		return fmt.Errorf("check working tree status: %w", err)
	}
	if status != "" {
		return errors.New("release preparation requires a clean working tree; commit or stash local changes first")
	}

	localTags, err := runTrimmed(ctx, opts.Runner, "git", "tag", "--list", version)
	if err != nil {
		return fmt.Errorf("check local tag %s: %w", version, err)
	}
	if tagExists(localTags, version) {
		return fmt.Errorf("release tag %s already exists locally", version)
	}

	remoteTags, err := runTrimmed(ctx, opts.Runner, "git", "ls-remote", "--tags", remote, "refs/tags/"+version)
	if err != nil {
		return fmt.Errorf("check remote tag %s on %s: %w", version, remote, err)
	}
	if strings.TrimSpace(remoteTags) != "" {
		return fmt.Errorf("release tag %s already exists on %s", version, remote)
	}

	for _, target := range readinessTargets {
		fmt.Fprintf(progress, "[release] make %s\n", target)
		if _, err := opts.Runner.Run(ctx, makeCommand, target); err != nil {
			return fmt.Errorf("run readiness target %s: %w", target, err)
		}
	}

	fmt.Fprintf(progress, "[release] git tag %s\n", version)
	if _, err := opts.Runner.Run(ctx, "git", "tag", version); err != nil {
		return fmt.Errorf("create git tag %s: %w", version, err)
	}

	fmt.Fprintf(progress, "[release] git push %s refs/tags/%s\n", remote, version)
	if _, err := opts.Runner.Run(ctx, "git", "push", remote, "refs/tags/"+version); err != nil {
		return fmt.Errorf("push git tag %s to %s: %w", version, remote, err)
	}

	fmt.Fprintf(progress, "[release] prepared %s from %s and handed publication to GitHub Actions\n", version, branch)
	return nil
}

type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func runTrimmed(ctx context.Context, runner Runner, name string, args ...string) (string, error) {
	out, err := runner.Run(ctx, name, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func valueOrDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func tagExists(output string, version string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == version {
			return true
		}
	}
	return false
}
