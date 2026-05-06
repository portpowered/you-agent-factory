package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/releasetag"
)

var (
	commandMain                     = run
	exitFunc                        = os.Exit
	stdout                io.Writer = os.Stdout
	stderr                io.Writer = os.Stderr
	listGitTagsPointingAt           = gitTagsPointingAt
)

func main() {
	exitFunc(commandMain(os.Args[1:], stdout, stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	tag, pointsAt, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	resolvedTag, err := resolveTag(context.Background(), tag, pointsAt)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "release_tag=%s\n", resolvedTag)
	return 0
}

func parseArgs(args []string) (string, string, error) {
	var tag string
	var pointsAt string
	flags := flag.NewFlagSet("releasetagcheck", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&tag, "tag", "", "explicit release tag to validate")
	flags.StringVar(&pointsAt, "points-at", "", "git revision whose release tag should be resolved")
	if err := flags.Parse(args); err != nil {
		return "", "", err
	}
	return tag, pointsAt, nil
}

func resolveTag(ctx context.Context, tag string, pointsAt string) (string, error) {
	switch {
	case strings.TrimSpace(tag) != "" && strings.TrimSpace(pointsAt) != "":
		return "", errors.New("use either -tag or -points-at, not both")
	case strings.TrimSpace(tag) != "":
		return releasetag.RequireSemver(tag)
	case strings.TrimSpace(pointsAt) != "":
		return resolveGitPointingTag(ctx, strings.TrimSpace(pointsAt))
	default:
		return "", errors.New("provide -tag or -points-at")
	}
}

func resolveGitPointingTag(ctx context.Context, revision string) (string, error) {
	candidates, err := listGitTagsPointingAt(ctx, revision)
	if err != nil {
		return "", err
	}

	releaseTags := releasetag.FilterSemver(candidates)
	sort.Strings(releaseTags)
	if len(releaseTags) != 1 {
		return "", fmt.Errorf("expected exactly one semver release tag for %s, found %q", revision, releaseTags)
	}
	return releaseTags[0], nil
}

func gitTagsPointingAt(ctx context.Context, revision string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "tag", "--points-at", revision)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list tags pointing at %s: %w\n%s", revision, err, strings.TrimSpace(string(output)))
	}

	var tags []string
	for _, line := range strings.Split(string(output), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}

	return tags, nil
}
