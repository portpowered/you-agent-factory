package prepare

import (
	"bytes"
	"encoding/json"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Import decodes one detached snapshot payload and returns Definitions-owned
// portable import facts without peer imports of public portableconfig.
func Import(
	payload []byte,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
) (factorydefinitions.PrepareFactorySnapshotImportResult, error) {
	if decodeSnapshot == nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}
	if !json.Valid(trimmed) {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}

	snapshot, err := decodeSnapshot(trimmed)
	if err != nil || snapshot == nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}

	var object map[string]any
	if err := snapshot.Decode(&object); err != nil {
		return factorydefinitions.PrepareFactorySnapshotImportResult{}, factorydefinitions.ErrInvalidFactorySnapshotPayload
	}

	name := stringField(object, "name")
	factoryDir := stringField(object, "factoryDirectory")
	return factorydefinitions.PrepareFactorySnapshotImportResult{
		Snapshot: snapshot,
		Name:     name,
		Portable: factorydefinitions.PortableFactorySnapshotFacts{
			FactoryDir: factoryDir,
			Assets:     portableAssets(object),
		},
	}, nil
}

func stringField(object map[string]any, key string) string {
	value, ok := object[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func portableAssets(object map[string]any) []factorydefinitions.PortableSnapshotAssetFact {
	manifest, ok := object["resourceManifest"].(map[string]any)
	if !ok {
		return nil
	}
	rawFiles, ok := manifest["bundledFiles"].([]any)
	if !ok || len(rawFiles) == 0 {
		return nil
	}
	assets := make([]factorydefinitions.PortableSnapshotAssetFact, 0, len(rawFiles))
	for _, raw := range rawFiles {
		file, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		targetPath := stringField(file, "targetPath")
		if targetPath == "" {
			continue
		}
		assets = append(assets, factorydefinitions.PortableSnapshotAssetFact{
			TargetPath: targetPath,
		})
	}
	if len(assets) == 0 {
		return nil
	}
	return assets
}
