//go:build timing_inactive

package timingdebt

import "time"

func Inactive() { time.Sleep(0) }
