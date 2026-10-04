package behaviorpolicyexternal_test

import d "m/pkg/services/factory_definitions"

func testPolicy() { d.MapDir() } // want `transport-named-factory-path-policy-test: pkg/transports/behaviorpolicyexternal_test -> pkg/services/factory_definitions.MapDir`
