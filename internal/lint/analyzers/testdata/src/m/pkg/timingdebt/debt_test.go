package timingdebt // want "stale baseline entry.*Reduced::sleep::2"

import "time"

// Blank lines and comments never affect occurrence keys.
func Moved() {

	time.Sleep(0)

	time.Sleep(0)
	time.Sleep(0) // want "testsleep-sleep-test.*Moved::sleep::3"
}

func Reduced() { time.Sleep(0) }

func Renamed() { time.Sleep(0) } // want "testsleep-sleep-test.*Renamed::sleep::1"
