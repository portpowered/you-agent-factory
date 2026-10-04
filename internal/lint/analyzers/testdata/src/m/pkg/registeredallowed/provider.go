package registeredallowed

import "m/pkg/registeredowner"

func Provide(dep registeredowner.Dependency) *registeredowner.Service {
	return registeredowner.New(dep)
}
