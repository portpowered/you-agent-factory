// Package promptassets assembles declarative prompt files into packaged factory payloads.
package promptassets

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

const (
	workerCollection      = "workers"
	workstationCollection = "workstations"
)

// Definition describes an authored packaged factory and the assets available
// beneath its package-owned asset root.
type Definition struct {
	Package     string
	FactoryJSON []byte
	Assets      fs.FS
	AssetRoot   string
}

// Assemble resolves worker and workstation promptFile declarations and returns
// a new canonical JSON payload. It reads only the supplied asset filesystem and
// does not persist or install the assembled definition.
func Assemble(definition Definition) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(definition.FactoryJSON, &root); err != nil {
		return nil, fmt.Errorf("assemble package %q: decode factory definition: %w", definition.Package, err)
	}

	for _, collection := range []subjectCollection{
		{key: workerCollection, subjectKind: "worker", preservePromptFile: false},
		{key: workstationCollection, subjectKind: "workstation", preservePromptFile: true},
	} {
		if err := assembleCollection(definition, root, collection); err != nil {
			return nil, err
		}
	}

	payload, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("assemble package %q: encode factory definition: %w", definition.Package, err)
	}
	return payload, nil
}

type subjectCollection struct {
	key                string
	subjectKind        string
	preservePromptFile bool
}

func assembleCollection(definition Definition, root map[string]any, collection subjectCollection) error {
	entries, exists := root[collection.key]
	if !exists {
		return nil
	}
	subjects, ok := entries.([]any)
	if !ok {
		return fmt.Errorf("assemble package %q: %s must be an array", definition.Package, collection.key)
	}

	for index, entry := range subjects {
		subject, ok := entry.(map[string]any)
		if !ok {
			return fmt.Errorf("assemble package %q: %s at index %d must be an object", definition.Package, collection.subjectKind, index)
		}
		if err := assembleSubject(definition, subject, collection); err != nil {
			return err
		}
	}
	return nil
}

func assembleSubject(definition Definition, subject map[string]any, collection subjectCollection) error {
	name, nameValid := subject["name"].(string)
	nameValid = nameValid && strings.TrimSpace(name) != ""
	promptValue, hasPromptFile := subject["promptFile"]
	promptFile, promptFileValid := promptValue.(string)
	if hasPromptFile && !promptFileValid {
		return promptReferenceError(definition, collection, name, "<non-string>", "declared prompt path must be a string")
	}
	if !nameValid {
		if hasPromptFile {
			return promptReferenceError(definition, collection, name, promptFile, "subject name must be a non-empty string")
		}
		return fmt.Errorf("assemble package %q: %s name must be a non-empty string", definition.Package, collection.subjectKind)
	}
	if !hasPromptFile {
		return nil
	}
	if promptFile == "" {
		return promptReferenceError(definition, collection, name, promptFile, "declared prompt path must be non-empty")
	}
	if inlineValue, hasInlineBody := subject["body"]; hasInlineBody {
		inlineBody, ok := inlineValue.(string)
		if !ok {
			return promptReferenceError(definition, collection, name, promptFile, "inline body must be a string")
		}
		if inlineBody != "" {
			return promptReferenceError(definition, collection, name, promptFile, "declares both promptFile and non-empty inline body")
		}
	}
	if err := validatePromptPath(promptFile); err != nil {
		return promptReferenceError(definition, collection, name, promptFile, err.Error())
	}

	assetPath := path.Join(definition.AssetRoot, promptFile)
	content, err := fs.ReadFile(definition.Assets, assetPath)
	if err != nil {
		return fmt.Errorf(
			"%s: read asset: %w",
			promptReferenceContext(definition, collection, name, promptFile),
			err,
		)
	}

	subject["body"] = string(content)
	if !collection.preservePromptFile {
		delete(subject, "promptFile")
	}
	return nil
}

func validatePromptPath(promptFile string) error {
	if path.IsAbs(promptFile) {
		return fmt.Errorf("declared prompt path must be package-relative")
	}
	cleaned := path.Clean(promptFile)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("declared prompt path escapes the package asset root")
	}
	return nil
}

func promptReferenceError(
	definition Definition,
	collection subjectCollection,
	name string,
	promptFile string,
	detail string,
) error {
	return fmt.Errorf("%s: %s", promptReferenceContext(definition, collection, name, promptFile), detail)
}

func promptReferenceContext(
	definition Definition,
	collection subjectCollection,
	name string,
	promptFile string,
) string {
	if name == "" {
		name = "<unnamed>"
	}
	return fmt.Sprintf(
		"assemble package %q %s %q prompt %q",
		definition.Package,
		collection.subjectKind,
		name,
		promptFile,
	)
}
