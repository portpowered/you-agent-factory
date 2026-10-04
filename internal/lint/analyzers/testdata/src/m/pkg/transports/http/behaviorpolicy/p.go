package behaviorpolicy

func statusFromEngineStateSnapshot() {} // want `transport-factory-status-projection:.*statusFromEngineStateSnapshot`
type workflow interface{ DefaultSourceContext() }

func run(w workflow) { w.DefaultSourceContext() } // want `transport-source-default-selection:.*DefaultSourceContext`
