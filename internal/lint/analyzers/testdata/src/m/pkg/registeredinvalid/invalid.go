package registeredinvalid // want "construction-metadata: parameter type mismatch"

type Service struct{}
func New(dep int) *Service { return &Service{} }
