package registeredproviderlisted // want "stale baseline entry.*unresolved-focused-provider-dispatch.*Removed"

import owner "m/pkg/registeredowner"

func Recursive(p owner.Dependency)             { Recursive(p); owner.New(p) }
func Callback(p owner.Dependency, next func()) { next(); owner.New(p) }
