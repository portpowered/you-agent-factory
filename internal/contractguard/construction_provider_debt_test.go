package contractguard

import "testing"

func TestConstructionFocusedProviderDispatchDebt(t *testing.T) {
	t.Parallel()
	cases := []constructionProviderCycleCase{
		{"cyclic package aliases", `again(p); selected.New(p)`, `var again = next; var next = again`, 1},
		{"package closure changed in caller file", `again = func(selected.Port) {}; again(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 1},
		{"package closure changed in declaration file", `again(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }; func replace() { again = func(selected.Port) {} }`, 1},
		{"package closure address escapes", `_ = &again; again(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 1},
		{"package closure range write", `for _, again = range []func(selected.Port){} {}; again(p); selected.New(p)`, `var again = func(p selected.Port) { Provide(p) }`, 1},

		{"callback parameter", `step(func() {}); selected.New(p)`, `func step(callback func()) { callback() }`, 1},
		{"callback alias", `step(func() {}); selected.New(p)`, `func step(callback func()) { next := callback; next() }`, 1},
		{"nested callback dispatch", `step(func() {}); selected.New(p)`, `func step(callback func()) { func() { callback() }() }`, 1},
		{"deferred callback", `step(func() {}); selected.New(p)`, `func step(callback func()) { defer callback() }`, 1},
		{"mutable local closure", `next := func() {}; next = func() {}; next(); selected.New(p)`, "", 1},
		{"opaque declared value", `var next func(); next(); selected.New(p)`, "", 1},
		{"called helper result", `factory(p)(); selected.New(p)`, `func factory(p selected.Port) func() { return func() { Provide(p) } }`, 1},
		{"parenthesized helper result", `(factory(p))(); selected.New(p)`, `func factory(p selected.Port) func() { return func() { Provide(p) } }`, 1},
		{"deferred helper result", `defer factory(p)(); selected.New(p)`, `func factory(p selected.Port) func() { return func() { Provide(p) } }`, 1},
		{"asynchronous helper result", `go factory(p)(); selected.New(p)`, `func factory(p selected.Port) func() { return func() { Provide(p) } }`, 1},
		{"nested returned callable", `factory(p)()(); selected.New(p)`, `func factory(p selected.Port) func() func() { return func() func() { return func() { Provide(p) } } }`, 1},
		{"helper calls returned callable", `step(p); selected.New(p)`, `func step(p selected.Port) { factory(p)() }; func factory(p selected.Port) func() { return func() { Provide(p) } }`, 1},
		{"opaque acyclic result", `factory()(); selected.New(p)`, `func factory() func() { return func() {} }`, 1},
		{"aliased returned callable", `next := factory(p); next(); selected.New(p)`, `func factory(p selected.Port) func() { return func() { Provide(p) } }`, 1},
		{"uncalled helper result", `_ = factory(p); selected.New(p)`, `func factory(p selected.Port) func() { return func() { Provide(p) } }`, 0},
		{"uncalled returned dispatch closure", `_ = func() { factory(p)() }; selected.New(p)`, `func factory(p selected.Port) func() { return func() { Provide(p) } }`, 0},
		{"uncalled callback", `step(func() {}); selected.New(p)`, `func step(callback func()) { _ = callback }`, 0},
		{"uncalled nested callback", `step(func() {}); selected.New(p)`, `func step(callback func()) { _ = func() { callback() } }`, 0},
		{"shadowed callback", `step(func() {}); selected.New(p)`, `func step(callback func()) { { callback := func() {}; callback() } }`, 0},
		{"builtin and conversion", `_ = len([]int{}); _ = int(1); selected.New(p)`, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkConstructionProviderCase(t, tc, "unresolved-focused-provider-dispatch")
		})
	}
}
