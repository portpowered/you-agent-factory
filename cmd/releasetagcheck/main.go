package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/releasetag"
)

func main() {
	var tag string
	var pointsAt string
	flag.StringVar(&tag, "tag", "", "explicit release tag to validate")
	flag.StringVar(&pointsAt, "points-at", "", "git revision whose release tag should be resolved")
	flag.Parse()

	resolvedTag, err := resolveTag(context.Background(), tag, pointsAt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("release_tag=%s\n", resolvedTag)
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
	cmd := exec.CommandContext(ctx, "git", "tag", "--points-at", revision)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("list tags pointing at %s: %w\n%s", revision, err, strings.TrimSpace(string(output)))
	}

	var candidates []string
	for _, line := range strings.Split(string(output), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			candidates = append(candidates, trimmed)
		}
	}

	releaseTags := releasetag.FilterSemver(candidates)
	sort.Strings(releaseTags)
	if len(releaseTags) != 1 {
		return "", fmt.Errorf("expected exactly one semver release tag for %s, found %q", revision, releaseTags)
	}
	return releaseTags[0], nil
}
