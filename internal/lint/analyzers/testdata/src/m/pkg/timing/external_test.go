package timing_test

import "time"

func External() { time.Sleep(0) } // want "testsleep-sleep-test.*pkg/timing_test.*External::sleep::1"
