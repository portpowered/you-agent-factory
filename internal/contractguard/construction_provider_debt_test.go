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
		{"function field", `h := Hook{}; h.Run(); selected.New(p)`, `type Hook struct { Run func() }`, 1},
		{"parenthesized function field", `h := Hook{}; (h.Run)(); selected.New(p)`, `type Hook struct { Run func() }`, 1},
		{"deferred function field", `h := Hook{}; defer h.Run(); selected.New(p)`, `type Hook struct { Run func() }`, 1},
		{"asynchronous function field", `h := Hook{}; go h.Run(); selected.New(p)`, `type Hook struct { Run func() }`, 1},
		{"promoted function field", `h := Wrapper{}; h.Run(); selected.New(p)`, `type Hook struct { Run func() }; type Wrapper struct { Hook }`, 1},
		{"nested function field", `h := Wrapper{}; h.Hook.Run(); selected.New(p)`, `type Hook struct { Run func() }; type Wrapper struct { Hook Hook }`, 1},
		{"returned owner function field", `factory().Run(); selected.New(p)`, `type Hook struct { Run func() }; func factory() Hook { return Hook{} }`, 1},
		{"function field in helper", `step(); selected.New(p)`, `type Hook struct { Run func() }; func step() { h := Hook{}; h.Run() }`, 1},
		{"function field in invoked closure", `h := Hook{}; func() { h.Run() }(); selected.New(p)`, `type Hook struct { Run func() }`, 1},
		{"import shadow function field", `{ selected := Hook{}; selected.Run() }; selected.New(p)`, `type Hook struct { Run func() }`, 1},
		{"interface method dispatch", `var h Hook; h.Run(); selected.New(p)`, `type Hook interface { Run() }`, 1},
		{"uncalled function field", `h := Hook{}; _ = h.Run; selected.New(p)`, `type Hook struct { Run func() }`, 0},
		{"function field in uncalled closure", `h := Hook{}; _ = func() { h.Run() }; selected.New(p)`, `type Hook struct { Run func() }`, 0},
		{"concrete method", `h := Hook{}; h.Run(); selected.New(p)`, `type Hook struct{}; func (Hook) Run() {}`, 0},
		{"promoted concrete method", `h := Wrapper{}; h.Run(); selected.New(p)`, `type Hook struct{}; func (Hook) Run() {}; type Wrapper struct { Hook }`, 0},
		{"concrete method expression", `Hook.Run(Hook{}); selected.New(p)`, `type Hook struct{}; func (Hook) Run() {}`, 0},
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
