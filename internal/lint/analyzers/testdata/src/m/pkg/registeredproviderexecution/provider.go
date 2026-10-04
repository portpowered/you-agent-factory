package registeredproviderexecution

import owner "m/pkg/registeredowner"

// direct
func Case01(p owner.Dependency) { owner.New(p) }

// immutable alias
func Case02(p owner.Dependency) { build := owner.New; build(p) }

// conditional synchronous
func Case03(p owner.Dependency) {
	if case03flag {
		owner.New(p)
	}
}

// deferred constructor
func Case04(p owner.Dependency) { defer owner.New(p) } // want "registered-construction.*Case04.*New"
// asynchronous constructor
func Case05(p owner.Dependency) { go owner.New(p) } // want "registered-construction.*Case05.*New"
// deferred alias
func Case06(p owner.Dependency) { build := owner.New; defer build(p) } // want "registered-construction.*Case06.*New"
// asynchronous alias
func Case07(p owner.Dependency) { build := owner.New; go build(p) } // want "registered-construction.*Case07.*New"
// returned factory
func Case08(p owner.Dependency) { case08factory = func() { owner.New(p) } } // want "registered-construction.*Case08.*New"
// immediate closure
func Case09(p owner.Dependency) { func() { owner.New(p) }() } // want "registered-construction.*Case09.*New"
// nested closure
func Case10(p owner.Dependency) { case10factory = func() { case10factory = func() { owner.New(p) } } } // want "registered-construction.*Case10.*New"
// deferred argument evaluated now
func Case11(p owner.Dependency) { defer case11consume(owner.New(p)) }

// asynchronous argument evaluated now
func Case12(p owner.Dependency) { go case12consume(owner.New(p)) }
