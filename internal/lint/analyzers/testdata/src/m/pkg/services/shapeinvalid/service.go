package shapeinvalid // want "service-root-interface-count: pkg/services/shapeinvalid -> pkg/services/shapeinvalid/service.go:Alpha,pkg/services/shapeinvalid/service.go:Zulu"

type Zulu interface{}
type Alpha interface{}

func New() {} // want "service-root-exported-function: pkg/services/shapeinvalid -> pkg/services/shapeinvalid/service.go#New"
