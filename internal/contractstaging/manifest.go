package contractstaging

import (
	"crypto/sha256"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var sourceCommitPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func generateManifest(repositoryRoot string, artifacts map[string][]byte) ([]byte, error) {
	sourceCommit, err := resolveSourceCommit(repositoryRoot)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	exports := make(map[string]any, len(paths))
	for _, repositoryPath := range paths {
		if !strings.HasPrefix(repositoryPath, "packages/api/") {
			continue
		}
		packagePath := strings.TrimPrefix(repositoryPath, "packages/api/")
		id := artifactID(packagePath)
		digest := fmt.Sprintf("%x", sha256.Sum256(artifacts[repositoryPath]))
		exports[id] = manifestExport(id, packagePath, artifactFamily(packagePath), digest)
	}
	manifest := map[string]any{
		"formatVersion":  "1.0.0",
		"packageId":      "you-agent-factory.api",
		"packageVersion": "0.0.0",
		"sourceCommit":   sourceCommit,
		"familyFormatVersions": map[string]any{
			"cli": "1.0.0", "config": "1.0.0", "javascript": "1.0.0",
			"mcp": "1.0.0", "openapi": "1.0.0", "shared": "1.0.0",
		},
		"exports": exports,
	}
	return marshalDocument(manifest)
}

func artifactID(path string) string {
	withoutExtension := strings.TrimSuffix(strings.TrimSuffix(path, filepath.Ext(path)), ".schema")
	replacer := strings.NewReplacer("/", ".", "_", "-", "@", "")
	return strings.Trim(replacer.Replace(strings.ToLower(withoutExtension)), ".")
}

func artifactFamily(path string) string {
	switch {
	case strings.Contains(path, "/schemas/"):
		return "config"
	case strings.Contains(path, "/cli/"):
		return "cli"
	case strings.Contains(path, "/mcp/"):
		return "mcp"
	case strings.Contains(path, "/javascript/"):
		return "javascript"
	case strings.Contains(path, "/openapi/"):
		return "openapi"
	default:
		return "shared"
	}
}

func manifestExport(id, path, family, digest string) map[string]any {
	title := "Published " + id + " contract"
	return map[string]any{
		"path":         path,
		"family":       family,
		"artifactHash": digest,
		"documentation": map[string]any{
			"formatVersion": "1.0.0",
			"itemId":        id,
			"documentation": map[string]any{
				"title": map[string]any{"id": id + ".title", "canonicalEnglish": title},
				"description": map[string]any{
					"id": id + ".description", "canonicalEnglish": title + " as raw JSON or YAML data.",
				},
			},
			"examples":   []any{path},
			"visibility": "public",
			"sourceHash": digest,
		},
		"lifecycle": map[string]any{
			"formatVersion": "1.0.0", "itemId": id, "state": "active", "since": "0.0.0",
		},
	}
}

func resolveSourceCommit(repositoryRoot string) (string, error) {
	head, err := gitOutput(repositoryRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve package source commit: %w", err)
	}
	args := append([]string{"-C", repositoryRoot, "rev-list", "-1", "HEAD", "--"}, SourceIdentityPaths()...)
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve package source commit: %w", err)
	}
	sourceCommit := strings.TrimSpace(string(output))
	if !sourceCommitPattern.MatchString(sourceCommit) {
		return "", fmt.Errorf("resolve package source commit: invalid commit %q", sourceCommit)
	}
	if sourceCommit != head {
		return sourceCommit, nil
	}
	changed, err := sourceIdentityChangedInCommit(repositoryRoot, head)
	if err != nil {
		return "", fmt.Errorf("resolve package source commit: %w", err)
	}
	if changed {
		return sourceCommit, nil
	}
	parent, err := gitOutput(repositoryRoot, "rev-parse", "HEAD^")
	if err != nil {
		shallow, shallowErr := isShallowRepository(repositoryRoot)
		if shallowErr == nil && shallow {
			return "", fmt.Errorf(
				"resolve package source commit: git history is too shallow to determine the last change to package source inputs; fetch full history (for example fetch-depth: 0 in CI)",
			)
		}
		return sourceCommit, nil
	}
	parentArgs := append([]string{"-C", repositoryRoot, "rev-list", "-1", parent, "--"}, SourceIdentityPaths()...)
	parentOutput, err := exec.Command("git", parentArgs...).Output()
	if err != nil {
		return sourceCommit, nil
	}
	parentSource := strings.TrimSpace(string(parentOutput))
	if parentSource != "" && parentSource != sourceCommit {
		merge, mergeErr := isMergeCommit(repositoryRoot, head)
		if mergeErr != nil {
			return "", fmt.Errorf("resolve package source commit: %w", mergeErr)
		}
		if merge {
			// Merge commits can appear in rev-list without modifying source identity
			// paths; an empty diff-tree on HEAD is expected and not shallow history.
			return sourceCommit, nil
		}
		return "", fmt.Errorf(
			"resolve package source commit: git history is too shallow to determine the last change to package source inputs; fetch full history (for example fetch-depth: 0 in CI)",
		)
	}
	return sourceCommit, nil
}

func sourceIdentityChangedInCommit(repositoryRoot, commit string) (bool, error) {
	args := append([]string{"-C", repositoryRoot, "diff-tree", "--no-commit-id", "--name-only", "-r", commit, "--"}, SourceIdentityPaths()...)
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return false, err
	}
	for line := range strings.Lines(string(output)) {
		if strings.TrimSpace(line) != "" {
			return true, nil
		}
	}
	return false, nil
}

func gitOutput(repositoryRoot string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", repositoryRoot}, args...)
	output, err := exec.Command("git", commandArgs...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func isShallowRepository(repositoryRoot string) (bool, error) {
	output, err := gitOutput(repositoryRoot, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return false, err
	}
	return output == "true", nil
}

func isMergeCommit(repositoryRoot, commit string) (bool, error) {
	_, err := gitOutput(repositoryRoot, "rev-parse", "--verify", commit+"^2")
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}
