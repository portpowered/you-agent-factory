package registeredprovidercycles

import owner "m/pkg/registeredowner"

// direct recursion
func Case01(p owner.Dependency) { Case01(p); owner.New(p) } // want "registered-construction.*Case01.*New"
// recursion beside opaque dispatch
func Case02(p owner.Dependency) { var next func(); next(); Case02(p); owner.New(p) } // want "registered-construction.*Case02.*New"
// conditional recursion
func Case03(p owner.Dependency) {
	if case03flag {
		Case03(p)
	}
	owner.New(p) // want "registered-construction.*Case03.*New"
}
// provider alias
func Case04(p owner.Dependency) { again := Case04; again(p); owner.New(p) } // want "registered-construction.*Case04.*New"
// mutual recursion
func Case05(p owner.Dependency) { case05step(p); owner.New(p) } // want "registered-construction.*Case05.*New"
// helper alias recursion
func Case06(p owner.Dependency) { case06step(p); owner.New(p) } // want "registered-construction.*Case06.*New"
// long cycle
func Case07(p owner.Dependency) { case07step(p); owner.New(p) } // want "registered-construction.*Case07.*New"
// deferred recursion
func Case08(p owner.Dependency) { defer Case08(p); owner.New(p) } // want "registered-construction.*Case08.*New"
// asynchronous recursion
func Case09(p owner.Dependency) { go Case09(p); owner.New(p) } // want "registered-construction.*Case09.*New"
// generic helper recursion
func Case10(p owner.Dependency) { case10step[int](p); owner.New(p) } // want "registered-construction.*Case10.*New"
// method helper recursion
func Case11(p owner.Dependency) { case11helper{}.step(p); owner.New(p) } // want "registered-construction.*Case11.*New"
// invoked literal
func Case12(p owner.Dependency) { func() { Case12(p) }(); owner.New(p) } // want "registered-construction.*Case12.*New"
// parenthesized literal
func Case13(p owner.Dependency) { (func() { Case13(p) })(); owner.New(p) } // want "registered-construction.*Case13.*New"
// invoked local closure
func Case14(p owner.Dependency) { again := func() { Case14(p) }; again(); owner.New(p) } // want "registered-construction.*Case14.*New"
// closure alias
func Case15(p owner.Dependency) { again := func() { Case15(p) }; next := again; next(); owner.New(p) } // want "registered-construction.*Case15.*New"
// declared closure
func Case16(p owner.Dependency) { var again = func() { Case16(p) }; again(); owner.New(p) } // want "registered-construction.*Case16.*New"
// nested invoked closure
func Case17(p owner.Dependency) { func() { func() { Case17(p) }() }(); owner.New(p) } // want "registered-construction.*Case17.*New"
// helper invokes closure
func Case18(p owner.Dependency) { case18step(p); owner.New(p) } // want "registered-construction.*Case18.*New"
// closure calls helper
func Case19(p owner.Dependency) { func() { case19step(p) }(); owner.New(p) } // want "registered-construction.*Case19.*New"
// deferred closure
func Case20(p owner.Dependency) { defer func() { Case20(p) }(); owner.New(p) } // want "registered-construction.*Case20.*New"
// asynchronous closure alias
func Case21(p owner.Dependency) { again := func() { Case21(p) }; go again(); owner.New(p) } // want "registered-construction.*Case21.*New"
// uncalled nested closure
func Case22(p owner.Dependency) { func() { _ = func() { Case22(p) } }(); owner.New(p) }

// acyclic invoked closure
func Case23(p owner.Dependency) { func() { case23consume(p) }(); owner.New(p) }

// shadowed closure binding
func Case24(p owner.Dependency) {
	again := func() { Case24(p) }
	_ = again
	{
		again := func() {}
		again()
	}
	owner.New(p)
}

// closure argument without invocation
func Case25(p owner.Dependency) { case25consume(func() { Case25(p) }); owner.New(p) }

// uncalled package closure
func Case32(p owner.Dependency) { owner.New(p) }

// shadowed package closure
func Case33(p owner.Dependency) {
	case33again := func(owner.Dependency) {}
	case33again(p)
	owner.New(p)
}

// acyclic package closure
func Case34(p owner.Dependency) { case34again(p); owner.New(p) }

// acyclic helper
func Case38(p owner.Dependency) { case38step(p); owner.New(p) }

// unrelated helper cycle
func Case39(p owner.Dependency) { case39step(p); owner.New(p) }

// shadowed provider
func Case40(p owner.Dependency) { Case40 := func(owner.Dependency) {}; Case40(p); owner.New(p) }

// uninvoked helper
func Case41(p owner.Dependency) { owner.New(p) }

// uninvoked closure
func Case42(p owner.Dependency) { _ = func() { Case42(p) }; owner.New(p) }

// shadowed helper
func Case43(p owner.Dependency) { case43step := func(owner.Dependency) {}; case43step(p); owner.New(p) }
