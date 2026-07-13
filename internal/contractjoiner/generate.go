package contractjoiner

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const diagnosticRootPath = "/"

type stagedDocument struct {
	path    string
	payload []byte
}

// Generate joins and canonicalizes the complete input set before replacing the
// joined staging directory as one prepared set. It never publishes a partial set.
func Generate(input Input) []contractvalidator.Diagnostic {
	if len(input.Roots) == 0 {
		return []contractvalidator.Diagnostic{*generationDiagnostic(
			"generation.input", "at least one authored root is required", "roots",
		)}
	}
	documents, diagnostics := Join(input)
	if len(diagnostics) != 0 {
		return diagnostics
	}

	staged, diagnostic := canonicalDocuments(documents)
	if diagnostic != nil {
		return []contractvalidator.Diagnostic{*diagnostic}
	}
	if diagnostic := publish(input.RepositoryRoot, staged); diagnostic != nil {
		return []contractvalidator.Diagnostic{*diagnostic}
	}
	return nil
}

func canonicalDocuments(documents []Document) ([]stagedDocument, *contractvalidator.Diagnostic) {
	staged := make([]stagedDocument, 0, len(documents))
	for _, document := range documents {
		payload, err := MarshalCanonicalJSON(document.Value)
		if err != nil {
			return nil, generationDiagnostic("generation.serialize", "joined document could not be serialized", document.Path)
		}
		staged = append(staged, stagedDocument{path: document.Path, payload: payload})
	}
	return staged, nil
}

func publish(repositoryRoot string, documents []stagedDocument) *contractvalidator.Diagnostic {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return generationDiagnostic("generation.root", "repository root could not be resolved", "repository")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return generationDiagnostic("generation.root", "repository root could not be resolved", "repository")
	}

	output := filepath.Join(root, filepath.FromSlash(joinedOutputDirectory))
	parent := filepath.Dir(output)
	if !safeExistingAncestor(root, parent) {
		return generationDiagnostic("generation.boundary", "joined output boundary is outside the repository", joinedOutputDirectory)
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return generationDiagnostic("generation.stage", "joined output could not be staged", joinedOutputDirectory)
	}
	canonicalParent, err := filepath.EvalSymlinks(parent)
	if err != nil || !pathContainedBy(root, canonicalParent) {
		return generationDiagnostic("generation.boundary", "joined output boundary is outside the repository", joinedOutputDirectory)
	}
	if canonicalOutput, err := filepath.EvalSymlinks(output); err == nil && !pathContainedBy(root, canonicalOutput) {
		return generationDiagnostic("generation.boundary", "joined output boundary is outside the repository", joinedOutputDirectory)
	} else if err != nil && !os.IsNotExist(err) {
		return generationDiagnostic("generation.boundary", "joined output boundary could not be resolved", joinedOutputDirectory)
	}
	staging, err := os.MkdirTemp(parent, ".joined-stage-")
	if err != nil {
		return generationDiagnostic("generation.stage", "joined output could not be staged", joinedOutputDirectory)
	}
	defer os.RemoveAll(staging)

	for _, document := range documents {
		target := filepath.Join(staging, filepath.FromSlash(document.path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return generationDiagnostic("generation.stage", "joined output could not be staged", document.path)
		}
		if err := os.WriteFile(target, document.payload, 0o644); err != nil {
			return generationDiagnostic("generation.stage", "joined output could not be staged", document.path)
		}
	}

	return replaceDirectory(output, staging)
}

func safeExistingAncestor(root, path string) bool {
	current := path
	for {
		_, err := os.Lstat(current)
		if err == nil {
			canonical, err := filepath.EvalSymlinks(current)
			return err == nil && pathContainedBy(root, canonical)
		}
		if !os.IsNotExist(err) {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func pathContainedBy(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func replaceDirectory(output, staging string) *contractvalidator.Diagnostic {
	backup, err := os.MkdirTemp(filepath.Dir(output), ".joined-backup-")
	if err != nil {
		return generationDiagnostic("generation.publish", "joined output could not be published", joinedOutputDirectory)
	}
	if err := os.Remove(backup); err != nil {
		return generationDiagnostic("generation.publish", "joined output could not be published", joinedOutputDirectory)
	}

	hadOutput := false
	if _, err := os.Stat(output); err == nil {
		hadOutput = true
		if err := os.Rename(output, backup); err != nil {
			return generationDiagnostic("generation.publish", "joined output could not be published", joinedOutputDirectory)
		}
	} else if !os.IsNotExist(err) {
		return generationDiagnostic("generation.publish", "joined output could not be published", joinedOutputDirectory)
	}

	if err := os.Rename(staging, output); err != nil {
		if hadOutput {
			_ = os.Rename(backup, output)
		}
		return generationDiagnostic("generation.publish", "joined output could not be published", joinedOutputDirectory)
	}
	if hadOutput {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func generationDiagnostic(code, message, document string) *contractvalidator.Diagnostic {
	diagnostic := contractvalidator.Diagnostic{
		Code: code, Path: diagnosticRootPath, Message: message, Document: filepath.ToSlash(document),
	}
	return &diagnostic
}
