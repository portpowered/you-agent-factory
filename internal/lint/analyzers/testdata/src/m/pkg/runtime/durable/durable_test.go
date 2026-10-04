package durable

import d "m/durabledefs"

func testDouble() {
	d.NewJavaScriptRuntimeService(d.Config{PersistSessions: true})
	d.NewLazyProjectStore(d.DirForProjectRoot(""))
	_ = d.DirectoryStore{}
	d.BuildCanonicalRuntimeSessionEvents()
}
