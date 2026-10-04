package shapeos

type Platform interface{}

func Existing() {}
func Added()    {} // want "service-root-exported-function: pkg/services/shapeos -> pkg/services/shapeos/service_linux.go#Added"
