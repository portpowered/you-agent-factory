package agent_test

import platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"

var deterministicRetryRandom = platformrandom.SourceFunc(func(int64) (int64, error) {
	return 0, nil
})
