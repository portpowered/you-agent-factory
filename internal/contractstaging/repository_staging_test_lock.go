package contractstaging

import "sync"

var repositoryStagingTestMu sync.Mutex

// LockRepositoryStagingForTest serializes integration tests that mutate or
// assume exclusive access to the checked-in contract staging tree.
func LockRepositoryStagingForTest() func() {
	repositoryStagingTestMu.Lock()
	return repositoryStagingTestMu.Unlock
}
