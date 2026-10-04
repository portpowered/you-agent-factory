package execution

import d "m/durabledefs"

func otherFile() {
	d.BuildCanonicalRuntimeSessionEvents()    // Canonical owner is a subtree.
	d.NewJavaScriptRuntimeService(d.Config{}) // want "durable-runtime-construction:.*NewJavaScriptRuntimeService"
	d.NewLazyProjectStore("")                 // want "durable-persistence-construction:.*NewLazyProjectStore"
	d.NewExecutionService()                   // want "durable-application-composition:.*NewExecutionService"
}
