// Package packageassets assembles package-owned assets into packaged factory payloads.
package packageassets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/pkg/factory/packages/promptassets"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	scriptAssetRoot  = "scripts"
	scriptTargetRoot = "factory/scripts"
)

// Definition describes an authored packaged factory and the assets available
// beneath its package-owned asset root.
type Definition = promptassets.Definition

// Assemble resolves all supported package-owned assets and returns a new
// canonical JSON payload. It reads only the supplied asset filesystem and does
// not persist or install the assembled definition.
func Assemble(definition Definition) ([]byte, error) {
	assembled, err := promptassets.Assemble(definition)
	if err != nil {
		return nil, err
	}

	scripts, err := discoverScripts(definition)
	if err != nil {
		return nil, err
	}
	if len(scripts) == 0 {
		return assembled, nil
	}

	var root map[string]any
	if err := json.Unmarshal(assembled, &root); err != nil {
		return nil, fmt.Errorf("assemble package %q scripts: decode assembled definition: %w", definition.Package, err)
	}
	if err := appendScripts(definition.Package, root, scripts); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("assemble package %q scripts: encode assembled definition: %w", definition.Package, err)
	}
	return payload, nil
}

type scriptAsset struct {
	targetPath string
	content    string
}

func discoverScripts(definition Definition) ([]scriptAsset, error) {
	root := path.Join(definition.AssetRoot, scriptAssetRoot)
	var scripts []scriptAsset
	err := fs.WalkDir(definition.Assets, root, func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}

		content, err := fs.ReadFile(definition.Assets, assetPath)
		if err != nil {
			return err
		}
		if !utf8.Valid(content) {
			return fmt.Errorf("script content is not valid UTF-8")
		}
		relativePath := assetPath[len(root)+1:]
		scripts = append(scripts, scriptAsset{
			targetPath: path.Join(scriptTargetRoot, relativePath),
			content:    string(content),
		})
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("assemble package %q script assets under %q: %w", definition.Package, root, err)
	}

	sort.Slice(scripts, func(i, j int) bool {
		return scripts[i].targetPath < scripts[j].targetPath
	})
	return scripts, nil
}

func appendScripts(packageName string, root map[string]any, scripts []scriptAsset) error {
	supportingFiles, exists := root["supportingFiles"]
	if !exists {
		supportingFiles = map[string]any{}
		root["supportingFiles"] = supportingFiles
	}
	manifest, ok := supportingFiles.(map[string]any)
	if !ok {
		return fmt.Errorf("assemble package %q scripts: supportingFiles must be an object", packageName)
	}

	bundledFiles, exists := manifest["bundledFiles"]
	if !exists {
		bundledFiles = []any{}
	}
	entries, ok := bundledFiles.([]any)
	if !ok {
		return fmt.Errorf("assemble package %q scripts: supportingFiles.bundledFiles must be an array", packageName)
	}
	for _, script := range scripts {
		entries = append(entries, map[string]any{
			"id":         script.targetPath,
			"type":       interfaces.BundledFileTypeScript,
			"targetPath": script.targetPath,
			"content": map[string]any{
				"encoding": interfaces.BundledFileEncodingUTF8,
				"inline":   script.content,
			},
		})
	}
	manifest["bundledFiles"] = entries
	return nil
}
