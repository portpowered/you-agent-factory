// Package builtinloop assembles the factory definition shipped as @you/loop.
package builtinloop

import "embed"

//go:embed factory.json
var factoryJSON []byte

//go:embed prompts/*.md
var promptAssets embed.FS

// BuiltInLoopFactoryJSON is the canonical runnable @you/loop packaged factory
// payload assembled from its authored factory scaffold and prompt assets.
var BuiltInLoopFactoryJSON = mustAssembleBuiltInLoopFactoryJSON()
