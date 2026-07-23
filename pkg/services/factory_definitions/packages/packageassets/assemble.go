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
	docAssetRoot     = "docs"
	docTargetRoot    = "factory/docs"
	inputAssetRoot   = "inputs"
	inputTargetRoot  = "factory/inputs"
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

	assets, err := discoverSupportedAssets(definition)
	if err != nil {
		return nil, err
	}

	var root map[string]any
	if err := json.Unmarshal(assembled, &root); err != nil {
		return nil, fmt.Errorf("assemble package %q scripts: decode assembled definition: %w", definition.Package, err)
	}
	if err := mergeAssets(definition.Package, root, assets); err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return assembled, nil
	}

	payload, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("assemble package %q assets: encode assembled definition: %w", definition.Package, err)
	}
	return payload, nil
}

type scriptAsset struct {
	targetPath string
	content    string
	fileType   string
}

func discoverSupportedAssets(definition Definition) ([]scriptAsset, error) {
	if err := validateScriptAssetRoot(definition.AssetRoot); err != nil {
		return nil, fmt.Errorf("assemble package %q assets: %w", definition.Package, err)
	}
	if definition.Assets == nil {
		return nil, fmt.Errorf("assemble package %q assets under %s: asset filesystem is required", definition.Package, scriptAssetRoot)
	}

	var assets []scriptAsset
	for _, directory := range []struct {
		sourceRoot string
		targetRoot string
		fileType   string
	}{
		{scriptAssetRoot, scriptTargetRoot, interfaces.BundledFileTypeScript},
		{docAssetRoot, docTargetRoot, interfaces.BundledFileTypeDoc},
		{inputAssetRoot, inputTargetRoot, interfaces.BundledFileTypeInput},
	} {
		discovered, err := discoverAssetDirectory(definition, directory.sourceRoot, directory.targetRoot, directory.fileType)
		if err != nil {
			return nil, err
		}
		assets = append(assets, discovered...)
	}
	sort.Slice(assets, func(i, j int) bool {
		return assets[i].targetPath < assets[j].targetPath
	})
	return assets, nil
}

func discoverAssetDirectory(
	definition Definition,
	sourceRoot string,
	targetRoot string,
	fileType string,
) ([]scriptAsset, error) {
	root := path.Join(definition.AssetRoot, sourceRoot)
	var assets []scriptAsset
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
		assets = append(assets, scriptAsset{
			targetPath: path.Join(targetRoot, relativePath),
			content:    string(content),
			fileType:   fileType,
		})
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("assemble package %q %s assets under %q: %w", definition.Package, strings.ToLower(fileType), root, err)
	}
	return assets, nil
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

func mergeAssets(packageName string, root map[string]any, assets []scriptAsset) error {
	manifest, entries, err := bundledManifest(packageName, root, len(assets) > 0)
	if err != nil || manifest == nil {
		return err
	}
	targets, err := existingBundledTargets(packageName, entries)
	if err != nil {
		return err
	}
	for _, asset := range assets {
		if _, duplicate := targets[asset.targetPath]; duplicate {
			return fmt.Errorf("assemble package %q bundled target %q: duplicate canonical target", packageName, asset.targetPath)
		}
		targets[asset.targetPath] = struct{}{}
		entries = append(entries, assetBundledEntry(asset))
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
		targetRoot := supportedTargetRoot(fileType)
		if targetRoot != "" && !strings.HasPrefix(target, targetRoot+"/") {
			return nil, fmt.Errorf("assemble package %q bundled target %q: %s target must be below %s/", packageName, target, fileType, targetRoot)
		}
		if _, duplicate := targets[target]; duplicate {
			return nil, fmt.Errorf("assemble package %q bundled target %q: duplicate canonical target", packageName, target)
		}
		targets[target] = struct{}{}
	}
	return targets, nil
}

func assetBundledEntry(asset scriptAsset) map[string]any {
	fileType := asset.fileType
	if fileType == "" {
		fileType = interfaces.BundledFileTypeScript
	}
	return map[string]any{
		"id":         asset.targetPath,
		"type":       fileType,
		"targetPath": asset.targetPath,
		"content": map[string]any{
			"encoding": interfaces.BundledFileEncodingUTF8,
			"inline":   asset.content,
		},
	}
}

func supportedTargetRoot(fileType string) string {
	switch fileType {
	case interfaces.BundledFileTypeScript:
		return scriptTargetRoot
	case interfaces.BundledFileTypeDoc:
		return docTargetRoot
	case interfaces.BundledFileTypeInput:
		return inputTargetRoot
	default:
		return ""
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
