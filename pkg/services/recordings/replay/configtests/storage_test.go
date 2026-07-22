package replay_test

import (
	"runtime"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
)

func testReplayStorage() platformreplay.Storage {
	return platformreplay.NewLocal(runtime.GOOS)
}
