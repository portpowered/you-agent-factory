package factorycontracts

import "io/fs"

// PackagedFactoryAssetDefinition describes one authored packaged Factory and
// the assets available beneath its package-owned asset root.
type PackagedFactoryAssetDefinition struct {
	Package     string
	FactoryJSON []byte
	Assets      fs.FS
	AssetRoot   string
}
