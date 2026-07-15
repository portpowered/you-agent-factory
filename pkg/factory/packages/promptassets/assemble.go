// Package promptassets assembles declarative prompt files into packaged factory payloads.
package promptassets

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
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
		{key: workerCollection, preservePromptFile: false},
		{key: workstationCollection, preservePromptFile: true},
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

	for _, entry := range subjects {
		subject, ok := entry.(map[string]any)
		if !ok {
			return fmt.Errorf("assemble package %q: %s entry must be an object", definition.Package, collection.key)
		}
		if err := assembleSubject(definition, subject, collection); err != nil {
			return err
		}
	}
	return nil
}

func assembleSubject(definition Definition, subject map[string]any, collection subjectCollection) error {
	promptFile, _ := subject["promptFile"].(string)
	if promptFile == "" {
		return nil
	}

	assetPath := path.Join(definition.AssetRoot, promptFile)
	content, err := fs.ReadFile(definition.Assets, assetPath)
	if err != nil {
		return fmt.Errorf("assemble package %q %s %q prompt %q: %w", definition.Package, collection.key, subject["name"], promptFile, err)
	}

	subject["body"] = string(content)
	if !collection.preservePromptFile {
		delete(subject, "promptFile")
	}
	return nil
}
