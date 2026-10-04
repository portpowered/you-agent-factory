package dotdurable

import . "m/durabledefs"

func build() {
	NewJavaScriptRuntimeService(Config{}) // want "durable-runtime-construction:.*NewJavaScriptRuntimeService"
	NewLazyProjectStore("")               // want "durable-persistence-construction:.*NewLazyProjectStore"
	// A local shadow has no connection to the imported constructor, but the
	// legacy durable policy still forbids the declaration name.
	NewExecutionService := func() {}
	NewExecutionService() // want "durable-application-composition:.*NewExecutionService"
}
