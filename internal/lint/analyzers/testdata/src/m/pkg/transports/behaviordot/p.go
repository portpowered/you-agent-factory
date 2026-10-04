package behaviordot

import . "time" // want `transport-opaque-import:.*time`
func run()      { Sleep(0) } // want `transport-lifecycle:.*time.Sleep`
