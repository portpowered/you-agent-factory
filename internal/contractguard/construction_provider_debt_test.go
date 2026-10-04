package contractguard

import "testing"

func TestConstructionReturnedCallableBoundaries(t *testing.T) {
	t.Parallel()
	cases := []constructionProviderCycleCase{
		{"acyclic declaration", `factory()(); selected.New(p)`, `func factory() func() { return consume }; func consume() {}`, 0},
		{"acyclic parentheses", `(factory())(); selected.New(p)`, `func factory() func() { return func() {} }`, 0},
		{"acyclic defer", `defer factory()(); selected.New(p)`, `func factory() func() { return func() {} }`, 0},
		{"acyclic go", `go factory()(); selected.New(p)`, `func factory() func() { return func() {} }`, 0},
		{"acyclic alias", `next := factory(); next(); selected.New(p)`, `func factory() func() { return func() {} }`, 0},
		{"mixed declarations", `factory()(p); selected.New(p)`, `func factory() func(selected.Port) { if flag { return Provide }; return consume }; func consume(selected.Port) {}`, 1},
		{"distinct closures", `factory(p)(); selected.New(p)`, `func factory(p selected.Port) func() { if flag { return func() { Provide(p) } }; return func() { Provide(p) } }`, 1},
		{"mixed function and closure", `factory()(); selected.New(p)`, `func factory() func() { if flag { return consume }; return func() {} }; func consume() {}`, 1},
		{"unknown alternative", `factory(nil)(); selected.New(p)`, `func factory(callback func()) func() { if flag { return func() {} }; return callback }`, 1},
		{"named explicit result", `factory()(); selected.New(p)`, `func factory() (next func()) { return func() {} }`, 1},
		{"named naked result", `factory()(); selected.New(p)`, `func factory() (next func()) { next = func() {}; return }`, 1},
		{"tuple result", `next, _ := factory(); next(); selected.New(p)`, `func factory() (func(), int) { return func() {}, 0 }`, 1},
		{"grouped result", `next, _ := factory(); next(); selected.New(p)`, `func factory() (first, second func()) { return func() {}, func() {} }`, 1},
		{"no explicit return", `factory()(); selected.New(p)`, `func factory() func() { panic("no result") }`, 1},
		{"unavailable body", `factory()(); selected.New(p)`, `func factory() func()`, 1},
		{"ambiguous body", `factory()(); selected.New(p)`, `func factory() func() { return func() {} }; func factory() func() { return func() {} }`, 1},
		{"imported producer", `selected.Factory()(); selected.New(p)`, "", 1},
		{"imported identity", `factory()(); selected.New(p)`, `func factory() func() { return selected.Callback }`, 1},
		{"returned callback", `factory(func() {})(); selected.New(p)`, `func factory(callback func()) func() { return callback }`, 1},
		{"returned field", `factory()(); selected.New(p)`, `type Hook struct { Run func() }; func factory() func() { h := Hook{}; return h.Run }`, 1},
		{"returned interface", `factory(nil)(); selected.New(p)`, `type Hook interface { Run() }; func factory(h Hook) func() { return h.Run }`, 1},
		{"self summary cycle", `factory()(); selected.New(p)`, `func factory() func() { return factory() }`, 1},
		{"mutual summary cycle", `factory()(); selected.New(p)`, `func factory() func() { return next() }; func next() func() { return factory() }`, 1},
		{"alias summary cycle", `factory()(); selected.New(p)`, `func factory() func() { next := factory(); return next }`, 1},
		{"closure summary cycle", `factory()(); selected.New(p)`, `func factory() func() { next := func() func() { return factory() }; return next() }`, 1},
		{"mutated result", `next := factory(); next = func() {}; next(); selected.New(p)`, `func factory() func() { return func() {} }`, 1},
		{"closure written result", `next := factory(); _ = func() { next = func() {} }; next(); selected.New(p)`, `func factory() func() { return func() {} }`, 1},
		{"range written result", `next := factory(); for _, next = range []func(){} {}; next(); selected.New(p)`, `func factory() func() { return func() {} }`, 1},
		{"address escaped result", `next := factory(); _ = &next; next(); selected.New(p)`, `func factory() func() { return func() {} }`, 1},
		{"callback escaped result", `next := factory(); consume(next); next(); selected.New(p)`, `func factory() func() { return func() {} }; func consume(func()) {}`, 1},
		{"field stored result", `next := factory(); h := Hook{Run: next}; _ = h; next(); selected.New(p)`, `type Hook struct { Run func() }; func factory() func() { return func() {} }`, 1},
		{"package stored result", `next := factory(); stored = next; next(); selected.New(p)`, `var stored func(); func factory() func() { return func() {} }`, 1},
		{"escaped result alias", `next := factory(); alias := next; consume(alias); next(); selected.New(p)`, `func factory() func() { return func() {} }; func consume(func()) {}`, 1},
		{"mutated return binding", `factory()(); selected.New(p)`, `func factory() func() { next := func() {}; next = func() {}; return next }`, 1},
		{"escaped return binding", `factory()(); selected.New(p)`, `func factory() func() { next := func() {}; consume(next); return next }; func consume(func()) {}`, 1},
		{"uncalled recursive result", `next := factory(p); _ = next; selected.New(p)`, `func factory(p selected.Port) func() { return func() { Provide(p) } }`, 0},
		{"shadowed producer", `factory := func() func() { return func() {} }; factory()(); selected.New(p)`, `func factory() func() { return func() { Provide(nil) } }`, 0},
		{"nested closure return excluded", `factory()(); selected.New(p)`, `func factory() func() { _ = func() func() { return func() { Provide(nil) } }; return func() {} }`, 0},
		{"unrelated returned call cycle", `factory()(); selected.New(p)`, `func factory() func() { return next }; func next() { factory()() }`, 0},
		{"domain state resources and unrelated New", `_ = factory(); New(); selected.New(p)`, `type State struct{}; func factory() State { return State{} }; func New() { _ = make(chan int) }`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkConstructionProviderCase(t, tc, "unresolved-focused-provider-dispatch")
		})
	}
}

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
		{"proved acyclic result", `factory()(); selected.New(p)`, `func factory() func() { return func() {} }`, 0},
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
