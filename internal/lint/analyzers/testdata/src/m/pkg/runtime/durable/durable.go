package durable

import d "m/durabledefs"

type StoreAlias = d.DirectoryStore

func build(root string) {
	d.NewJavaScriptRuntimeService(d.Config{}) // want "durable-runtime-construction:.*NewJavaScriptRuntimeService"
	d.NewLazyProjectStore(root)               // want "durable-persistence-construction:.*NewLazyProjectStore"
	d.DirForProjectRoot(root)                 // want "durable-persistence-construction:.*DirForProjectRoot"
	_ = d.DirectoryStore{}                    // want "durable-persistence-construction:.*DirectoryStore literal"
	_ = StoreAlias{}                          // Deduplicated by exact declaration key.
	_ = d.Config{PersistSessions: false}      // want "durable-persistence-boolean:.*PersistSessions"
	d.AppendDispatchInterruptedEvent()        // want "durable-canonical-events:.*AppendDispatchInterruptedEvent"
	d.BuildCanonicalRuntimeSessionEvents()    // want "durable-canonical-events:.*BuildCanonicalRuntimeSessionEvents"
	d.MapCanonicalRuntimeSessionEvents()      // want "durable-canonical-events:.*MapCanonicalRuntimeSessionEvents"
	d.BuildInvocationBootstrap()              // want "durable-application-composition:.*BuildInvocationBootstrap"
	d.NewExecutionService()                   // want "durable-application-composition:.*NewExecutionService"
	d.NewFakeServiceFromContractFixtures()    // want "durable-application-composition:.*NewFakeServiceFromContractFixtures"
	d.ProjectPersistence()                    // want "durable-application-composition:.*ProjectPersistence"
	// Ordinary values and map keys are not persistence fields.
	PersistSessions := "domain-key"
	_ = map[string]bool{PersistSessions: true}
}
