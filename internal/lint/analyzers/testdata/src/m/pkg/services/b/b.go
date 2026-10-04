package b

// NewThing looks like construction.
func NewThing() int { return 1 }

// Plain is not construction-shaped.
func Plain() int { return 2 }

func EnsureThing() int { return 0 }
func Newthing() int    { return 0 }

var Owned = NewThing
