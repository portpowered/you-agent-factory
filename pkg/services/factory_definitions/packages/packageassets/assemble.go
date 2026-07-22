// Package packageassets assembles package-owned assets into packaged factory payloads.
package packageassets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/promptassets"
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

	var root map[string]any
	if err := json.Unmarshal(assembled, &root); err != nil {
		return nil, fmt.Errorf("assemble package %q scripts: decode assembled definition: %w", definition.Package, err)
	}
	if err := mergeScripts(definition.Package, root, scripts); err != nil {
		return nil, err
	}
	if len(scripts) == 0 {
		return assembled, nil
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
	if err := validateScriptAssetRoot(definition.AssetRoot); err != nil {
		return nil, fmt.Errorf("assemble package %q script assets: %w", definition.Package, err)
	}
	root := path.Join(definition.AssetRoot, scriptAssetRoot)
	if definition.Assets == nil {
		return nil, fmt.Errorf("assemble package %q script assets under %q: asset filesystem is required", definition.Package, root)
	}
	var scripts []scriptAsset
	err := fs.WalkDir(definition.Assets, root, func(assetPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("inspect asset %q: %w", assetPath, walkErr)
		}
		if entry.IsDir() {
			return nil
		}
		if assetPath == root {
			return fmt.Errorf("asset root %q must be a directory", root)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect asset %q: %w", assetPath, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("asset %q has unsupported non-regular file type %s", assetPath, info.Mode().Type())
		}

		content, err := fs.ReadFile(definition.Assets, assetPath)
		if err != nil {
			return fmt.Errorf("read asset %q: %w", assetPath, err)
		}
		if !utf8.Valid(content) {
			return fmt.Errorf("asset %q content is not valid UTF-8", assetPath)
		}
		relativePath := strings.TrimPrefix(assetPath, root+"/")
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

func validateScriptAssetRoot(assetRoot string) error {
	if assetRoot == "" {
		return nil
	}
	if path.IsAbs(assetRoot) || !fs.ValidPath(assetRoot) || path.Clean(assetRoot) != assetRoot {
		return fmt.Errorf("asset root %q must be a canonical package-relative path", assetRoot)
	}
	return nil
}

func mergeScripts(packageName string, root map[string]any, scripts []scriptAsset) error {
	manifest, entries, err := bundledManifest(packageName, root, len(scripts) > 0)
	if err != nil || manifest == nil {
		return err
	}
	targets, err := existingBundledTargets(packageName, entries)
	if err != nil {
		return err
	}
	for _, script := range scripts {
		if _, duplicate := targets[script.targetPath]; duplicate {
			return fmt.Errorf("assemble package %q bundled target %q: duplicate canonical target", packageName, script.targetPath)
		}
		targets[script.targetPath] = struct{}{}
		entries = append(entries, scriptBundledEntry(script))
	}
	manifest["bundledFiles"] = entries
	return nil
}

func bundledManifest(packageName string, root map[string]any, create bool) (map[string]any, []any, error) {
	supportingFiles, exists := root["supportingFiles"]
	if !exists {
		if !create {
			return nil, nil, nil
		}
		supportingFiles = map[string]any{}
		root["supportingFiles"] = supportingFiles
	}
	manifest, ok := supportingFiles.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("assemble package %q scripts: supportingFiles must be an object", packageName)
	}

	bundledFiles, exists := manifest["bundledFiles"]
	if !exists {
		bundledFiles = []any{}
	}
	entries, ok := bundledFiles.([]any)
	if !ok {
		return nil, nil, fmt.Errorf("assemble package %q scripts: supportingFiles.bundledFiles must be an array", packageName)
	}
	return manifest, entries, nil
}

func existingBundledTargets(packageName string, entries []any) (map[string]struct{}, error) {
	targets := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		file, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("assemble package %q scripts: bundled file at index %d must be an object", packageName, index)
		}
		target, ok := file["targetPath"].(string)
		if !ok || target == "" {
			return nil, fmt.Errorf("assemble package %q scripts: bundled file at index %d targetPath must be a non-empty string", packageName, index)
		}
		if err := validateBundledTarget(target); err != nil {
			return nil, fmt.Errorf("assemble package %q bundled target %q: %w", packageName, target, err)
		}
		fileType, _ := file["type"].(string)
		if fileType == interfaces.BundledFileTypeScript && !strings.HasPrefix(target, scriptTargetRoot+"/") {
			return nil, fmt.Errorf("assemble package %q bundled target %q: SCRIPT target must be below %s/", packageName, target, scriptTargetRoot)
		}
		if _, duplicate := targets[target]; duplicate {
			return nil, fmt.Errorf("assemble package %q bundled target %q: duplicate canonical target", packageName, target)
		}
		targets[target] = struct{}{}
	}
	return targets, nil
}

func scriptBundledEntry(script scriptAsset) map[string]any {
	return map[string]any{
		"id":         script.targetPath,
		"type":       interfaces.BundledFileTypeScript,
		"targetPath": script.targetPath,
		"content": map[string]any{
			"encoding": interfaces.BundledFileEncodingUTF8,
			"inline":   script.content,
		},
	}
}

func validateBundledTarget(target string) error {
	if path.IsAbs(target) || strings.HasPrefix(target, "\\") || hasWindowsVolumePrefix(target) {
		return fmt.Errorf("target must be factory-relative, not absolute")
	}
	if strings.Contains(target, "\\") {
		return fmt.Errorf("target must use forward slashes")
	}
	cleaned := path.Clean(target)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("target cannot escape the factory root")
	}
	if cleaned != target {
		return fmt.Errorf("target must be canonical and must not contain '.' or '..' segments")
	}
	return nil
}

func hasWindowsVolumePrefix(target string) bool {
	return len(target) >= 3 &&
		((target[0] >= 'A' && target[0] <= 'Z') || (target[0] >= 'a' && target[0] <= 'z')) &&
		target[1] == ':' && (target[2] == '/' || target[2] == '\\')
}
