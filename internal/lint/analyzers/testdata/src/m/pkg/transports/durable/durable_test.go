package durable

import d "m/durabledefs"

func testComposition() {
	d.NewExecutionService()                   // Transport tests retain their composition exception.
	d.NewJavaScriptRuntimeService(d.Config{}) // want "durable-runtime-construction-test:.*NewJavaScriptRuntimeService"
	d.NewLazyProjectStore("")                 // want "durable-persistence-construction-test:.*NewLazyProjectStore"
	_ = d.Config{PersistSessions: true}       // want "durable-persistence-boolean-test:.*PersistSessions"
	d.BuildCanonicalRuntimeSessionEvents()    // want "durable-canonical-events-test:.*BuildCanonicalRuntimeSessionEvents"
}
