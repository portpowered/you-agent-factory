package registeredproviderdebt

import owner "m/pkg/registeredowner"

// callback parameter
func Case06(p owner.Dependency) { case06step(func() {}); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case06.*New"
// callback alias
func Case07(p owner.Dependency) { case07step(func() {}); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case07.*New"
// nested callback dispatch
func Case08(p owner.Dependency) { case08step(func() {}); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case08.*New"
// deferred callback
func Case09(p owner.Dependency) { case09step(func() {}); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case09.*New"
// mutable local closure
func Case10(p owner.Dependency) { next := func() {}; next = func() {}; next(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case10.*New"
// opaque declared value
func Case11(p owner.Dependency) { var next func(); next(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case11.*New"
// called helper result
func Case12(p owner.Dependency) { case12factory(p)(); owner.New(p) } // want "registered-construction.*Case12.*New"
// parenthesized helper result
func Case13(p owner.Dependency) { (case13factory(p))(); owner.New(p) } // want "registered-construction.*Case13.*New"
// deferred helper result
func Case14(p owner.Dependency) { defer case14factory(p)(); owner.New(p) } // want "registered-construction.*Case14.*New"
// asynchronous helper result
func Case15(p owner.Dependency) { go case15factory(p)(); owner.New(p) } // want "registered-construction.*Case15.*New"
// nested returned callable
func Case16(p owner.Dependency) { case16factory(p)()(); owner.New(p) } // want "registered-construction.*Case16.*New"
// helper calls returned callable
func Case17(p owner.Dependency) { case17step(p); owner.New(p) } // want "registered-construction.*Case17.*New"
// opaque acyclic result
func Case18(p owner.Dependency) { case18factory()(); owner.New(p) }

// aliased returned callable
func Case19(p owner.Dependency) { next := case19factory(p); next(); owner.New(p) } // want "registered-construction.*Case19.*New"
// function field
func Case20(p owner.Dependency) { h := case20Hook{}; h.Run(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case20.*New"
// parenthesized function field
func Case21(p owner.Dependency) { h := case21Hook{}; (h.Run)(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case21.*New"
// deferred function field
func Case22(p owner.Dependency) { h := case22Hook{}; defer h.Run(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case22.*New"
// asynchronous function field
func Case23(p owner.Dependency) { h := case23Hook{}; go h.Run(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case23.*New"
// promoted function field
func Case24(p owner.Dependency) { h := case24Wrapper{}; h.Run(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case24.*New"
// nested function field
func Case25(p owner.Dependency) { h := case25Wrapper{}; h.case25Hook.Run(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case25.*New"
// returned owner function field
func Case26(p owner.Dependency) { case26factory().Run(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case26.*New"
// function field in helper
func Case27(p owner.Dependency) { case27step(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case27.*New"
// function field in invoked closure
func Case28(p owner.Dependency) { h := case28Hook{}; func() { h.Run() }(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case28.*New"
// import shadow function field
func Case29(p owner.Dependency) {
	{
		selected := case29Hook{}
		selected.Run()
	}
	owner.New(p) // want "unresolved-focused-provider-dispatch.*Case29.*New"
}
// interface method dispatch
func Case30(p owner.Dependency) { var h case30Hook; h.Run(); owner.New(p) } // want "unresolved-focused-provider-dispatch.*Case30.*New"
// uncalled function field
func Case31(p owner.Dependency) { h := case31Hook{}; _ = h.Run; owner.New(p) }

// function field in uncalled closure
func Case32(p owner.Dependency) { h := case32Hook{}; _ = func() { h.Run() }; owner.New(p) }

// concrete method
func Case33(p owner.Dependency) { h := case33Hook{}; h.Run(); owner.New(p) }

// promoted concrete method
func Case34(p owner.Dependency) { h := case34Wrapper{}; h.Run(); owner.New(p) }

// concrete method expression
func Case35(p owner.Dependency) { case35Hook.Run(case35Hook{}); owner.New(p) }

// uncalled helper result
func Case36(p owner.Dependency) { _ = case36factory(p); owner.New(p) }

// uncalled returned dispatch closure
func Case37(p owner.Dependency) { _ = func() { case37factory(p)() }; owner.New(p) }

// uncalled callback
func Case38(p owner.Dependency) { case38step(func() {}); owner.New(p) }

// uncalled nested callback
func Case39(p owner.Dependency) { case39step(func() {}); owner.New(p) }

// shadowed callback
func Case40(p owner.Dependency) { case40step(func() {}); owner.New(p) }

// builtin and conversion
func Case41(p owner.Dependency) { _ = len([]int{}); _ = int(1); owner.New(p) }
