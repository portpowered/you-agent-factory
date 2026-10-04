package clockview

import "time"

type ticker struct{ timer *timer }

func (t *ticker) Chan() <-chan time.Time { return t.timer.Chan() }
func (t *ticker) Stop()                  { t.timer.Stop() }
func (t *ticker) Reset(d time.Duration) {
	validateInterval(d)
	t.timer.replace(d, d)
}

func validateInterval(d time.Duration) {
	if d <= 0 {
		panic("non-positive interval for ticker")
	}
}
