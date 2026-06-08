package workflowsource

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	CodeArtifactRootInvalid   = "workflow.source.artifactRootInvalid"
	CodeArtifactRootInsideRepo = "workflow.source.artifactRootInsideRepo"
)

func resolveArtifactRoot(projectRoot, requested string) ArtifactRootDecision {
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

	if strings.TrimSpace(projectRoot) != "" && pathWithinRoot(projectRoot, cleaned) {
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

	resolved, err := filepath.EvalSymlinks(cleaned)
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

	return ArtifactRootDecision{
		Requested: requested,
		Effective: resolved,
		Allowed:   true,
	}
}

func pathWithinRoot(root, candidate string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	candidate = filepath.Clean(strings.TrimSpace(candidate))
	if root == "" || candidate == "" {
		return false
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
