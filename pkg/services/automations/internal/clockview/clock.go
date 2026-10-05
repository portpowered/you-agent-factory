// Package clockview adapts selected process time to the external clockwork contract.
// Automations owns the projection: fact reads preserve the process Clock origin,
// while waits preserve its selected TimerSource. The adapter adds no scheduling
// policy and is needed only while Automations' external libraries use clockwork.
package clockview

import (
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
)

type view struct {
	facts     platformclock.Source
	scheduler platformclock.TimerSource
}

// New is inert: resources are allocated only when a consumer requests a wait.
func New(facts platformclock.Source, scheduler platformclock.TimerSource) clockwork.Clock {
	return &view{facts: facts, scheduler: scheduler}
}

func (v *view) Now() time.Time                         { return v.facts.Now() }
func (v *view) Since(t time.Time) time.Duration        { return v.Now().Sub(t) }
func (v *view) Until(t time.Time) time.Duration        { return t.Sub(v.Now()) }
func (v *view) After(d time.Duration) <-chan time.Time { return v.scheduler.After(d) }
func (v *view) Sleep(d time.Duration)                  { <-v.After(d) }
func (v *view) NewTimer(d time.Duration) clockwork.Timer {
	return newTimer(v.scheduler, d, 0, nil)
}
func (v *view) AfterFunc(d time.Duration, f func()) clockwork.Timer {
	return newTimer(v.scheduler, d, 0, f)
}
func (v *view) NewTicker(d time.Duration) clockwork.Ticker {
	validateInterval(d)
	return &ticker{timer: newTimer(v.scheduler, d, d, nil)}
}
