package runtime

import "time"

type testRuntimeClock struct{}

func (testRuntimeClock) Now() time.Time { return time.Now() }
