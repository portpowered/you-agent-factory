package durableexternal_test

import d "m/durabledefs"

func testComposition() {
	d.NewExecutionService()
	d.NewJavaScriptRuntimeService(d.Config{}) // want "durable-runtime-construction-test: pkg/transports/durableexternal_test"
}
