//go:build backendconformance

package durabletagged

import d "m/durabledefs"

func tagged() {
	d.NewJavaScriptRuntimeService(d.Config{}) // want "durable-runtime-construction:.*NewJavaScriptRuntimeService"
}
