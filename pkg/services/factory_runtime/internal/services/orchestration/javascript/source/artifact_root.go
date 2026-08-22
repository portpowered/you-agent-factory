package workflowsource

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	CodeArtifactRootInvalid    = "workflow.source.artifactRootInvalid"
	CodeArtifactRootInsideRepo = "workflow.source.artifactRootInsideRepo"
)

func resolveArtifactRoot(
	projectRoot,
	requested string,
	resolveSymlinks func(string) (string, error),
) ArtifactRootDecision {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return ArtifactRootDecision{Allowed: true}
	}

	cleaned := filepath.Clean(requested)
	if !filepath.IsAbs(cleaned) {
		return ArtifactRootDecision{
			Requested: requested,
			Allowed:   false,
			Diagnostic: &Diagnostic{
				Code:    CodeArtifactRootInvalid,
				Message: fmt.Sprintf("artifact root %q must be an absolute path", requested),
			},
		}
	}
	if resolveSymlinks == nil {
		return ArtifactRootDecision{
			Requested: requested,
			Allowed:   false,
			Diagnostic: &Diagnostic{
				Code:    CodeArtifactRootInvalid,
				Message: "workflow source symlink resolver is required",
			},
		}
	}

	if strings.TrimSpace(projectRoot) != "" && pathWithinRoot(projectRoot, cleaned, resolveSymlinks) {
		return ArtifactRootDecision{
			Requested: requested,
			Effective: cleaned,
			Allowed:   false,
			Diagnostic: &Diagnostic{
				Code:    CodeArtifactRootInsideRepo,
				Message: fmt.Sprintf("artifact root %q must be outside the target repository %q", cleaned, projectRoot),
			},
		}
	}

	resolved, err := resolveSymlinks(cleaned)
	if err != nil {
		if !os.IsNotExist(err) {
			return ArtifactRootDecision{
				Requested: requested,
				Allowed:   false,
				Diagnostic: &Diagnostic{
					Code:    CodeArtifactRootInvalid,
					Message: fmt.Sprintf("artifact root %q is not accessible: %v", requested, err),
				},
			}
		}
		resolved = cleaned
	}

	if strings.TrimSpace(projectRoot) != "" && pathWithinRoot(projectRoot, resolved, resolveSymlinks) {
		return ArtifactRootDecision{
			Requested: requested,
			Effective: resolved,
			Allowed:   false,
			Diagnostic: &Diagnostic{
				Code:    CodeArtifactRootInsideRepo,
				Message: fmt.Sprintf("artifact root %q resolves inside the target repository %q", resolved, projectRoot),
			},
		}
	}

	return ArtifactRootDecision{
		Requested: requested,
		Effective: resolved,
		Allowed:   true,
	}
}

func pathWithinRoot(
	root,
	candidate string,
	resolveSymlinks func(string) (string, error),
) bool {
	root = resolvedComparablePath(root, resolveSymlinks)
	candidate = resolvedComparablePath(candidate, resolveSymlinks)
	if root == "" || candidate == "" {
		return false
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func resolvedComparablePath(path string, resolveSymlinks func(string) (string, error)) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	resolved, err := resolveSymlinks(path)
	if err == nil {
		return resolved
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	resolvedParent, err := resolveSymlinks(parent)
	if err != nil {
		return path
	}
	return filepath.Join(resolvedParent, filepath.Base(path))
}
