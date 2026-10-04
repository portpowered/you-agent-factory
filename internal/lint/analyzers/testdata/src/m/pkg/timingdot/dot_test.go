package timingdot

import . "time"

func Dot() {
	Sleep(0) // want "testsleep-sleep-test.*Dot::sleep::1"
	_ = After(Second) // want "testsleep-deadline-test.*Dot::deadline::1"
	_ = Since(Now()) < Second // want "testsleep-elapsed-test.*Dot::elapsed::1"
}
