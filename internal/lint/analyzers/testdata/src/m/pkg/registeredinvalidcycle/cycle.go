package registeredinvalidcycle

// These were formerly parseable scanner cases, but no compiled provider can
// reach this initializer: the compiler rejects both forms.
var next = func() { Provide() }

func Provide() { next() }

var first = second
var second = first
