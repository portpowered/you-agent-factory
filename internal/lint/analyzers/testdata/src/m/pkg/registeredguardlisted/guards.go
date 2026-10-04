package registeredguardlisted // want "stale baseline entry.*Removed"

type Service struct{}

func New(p any) *Service {
	if p == nil {
	}
	_, ok := p.(int)
	if !ok {
	}
	return nil
}
func (s *Service) Check() {
	if s == nil {
	}
}
func (s *Service) Mutated() {
	s = nil
	if s == nil {
	}
}
