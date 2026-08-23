package replay_contracts

import factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

func factoryWorksValue(value *[]factoryapi.Work) []factoryapi.Work {
	if value == nil {
		return nil
	}
	return *value
}

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func stringSlicePointerValue(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}
