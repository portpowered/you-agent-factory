package timinghelper

import "time"

func Helper() { time.Sleep(0) } // want "testsleep-sleep:.*Helper::sleep::1"
