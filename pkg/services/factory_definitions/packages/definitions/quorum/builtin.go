// Package quorum owns the declarative definition of the built-in quorum factory.
package quorum

import "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/internal/authoredsource"

// BuiltInFactoryJSON is the canonical runnable @you/quorum packaged factory payload.
var BuiltInFactoryJSON = authoredsource.MustFactoryJSON("quorum")
