package process

import "time"

// defaultPostRunCleanupGracePeriod is the default bounded wait after graceful
// termination before force-killing remaining supervised process-tree members.
const defaultPostRunCleanupGracePeriod = 10 * time.Second

// postRunCleanupGracePeriodForTest, when positive, overrides
// defaultPostRunCleanupGracePeriod for post-run cleanup paths in tests only.
var postRunCleanupGracePeriodForTest time.Duration

func postRunCleanupGracePeriod() time.Duration {
	if postRunCleanupGracePeriodForTest > 0 {
		return postRunCleanupGracePeriodForTest
	}
	return defaultPostRunCleanupGracePeriod
}
