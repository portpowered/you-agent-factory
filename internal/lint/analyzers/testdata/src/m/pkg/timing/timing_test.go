package timing

import (
	ctx "context"
	tm "time"
)

var packageTimer = tm.After(tm.Second) // want "testsleep-deadline-test.*<package>::deadline::1"

func Calls() {
	tm.Sleep(0) // want "testsleep-sleep-test.*Calls::sleep::1"
	_ = tm.After(5 * tm.Second) // want "testsleep-deadline-test.*Calls::deadline::1"
	_ = tm.NewTimer(tm.Second / 2) // want "testsleep-deadline-test.*Calls::deadline::2"
	_ = tm.AfterFunc(5.0 * tm.Second, func() {}) // want "testsleep-deadline-test.*Calls::deadline::3"
	_, cancel := ctx.WithTimeout(ctx.Background(), tm.Duration(1_000)) // want "testsleep-deadline-test.*Calls::deadline::4"
	cancel()
	_, cancel = ctx.WithDeadline(ctx.Background(), tm.Now().Add((2 + 3) * tm.Second)) // want "testsleep-deadline-test.*Calls::deadline::5"
	cancel()
	_ = tm.After(5*tm.Second + 1)
	_ = tm.NewTimer(30 * tm.Second)
	_ = tm.AfterFunc(tm.Minute, func() {})
	duration := tm.Second
	const ceiling = tm.Second
	_ = tm.After(duration)
	_ = tm.After(ceiling)
	_, cancel = ctx.WithTimeout(ctx.Background(), tm.Minute)
	cancel()
	_, cancel = ctx.WithDeadline(ctx.Background(), tm.Now().Add(duration))
	cancel()
	start := tm.Now()
	_ = tm.Since(start) < 2*tm.Second // want "testsleep-elapsed-test.*Calls::elapsed::1"
	_ = 1*tm.Second >= tm.Now().Sub(start) // want "testsleep-elapsed-test.*Calls::elapsed::2"
	_ = tm.Since(start) == tm.Second
	_ = tm.Since(start) < duration
	_ = duration > tm.Second
}

func Exemptions() {
	tm.Sleep(0) //nolint:testsleep // tests the exemption
	//nolint:testsleep // tests preceding-line exemption
	tm.Sleep(0)
	/* want "requires a reason" */ //nolint:testsleep
	tm.Sleep(0) // want "testsleep-sleep-test.*Exemptions::sleep::1"
}

type generic[T any] struct{}
func (*generic[T]) Method() {
	tm.Sleep(0) // want "testsleep-sleep-test.*generic.Method::sleep::1"
}

func Shadowing() {
	tm := sleeper{}
	tm.Sleep(0)
}
