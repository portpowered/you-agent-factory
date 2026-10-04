package timing

import "time"

// Ordinary production files are outside timing scope.
func Production() { time.Sleep(time.Second) }

type sleeper struct{}
func (sleeper) Sleep(time.Duration) {}
