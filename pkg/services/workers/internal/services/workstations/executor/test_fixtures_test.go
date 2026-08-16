package executor

import "github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"

// staticRuntimeConfig is shared by the retained workstation prompt/route
// tests. Agent execution fixtures live with the detached runner tests now.
type staticRuntimeConfig = runtimefixtures.RuntimeConfigLookupFixture
