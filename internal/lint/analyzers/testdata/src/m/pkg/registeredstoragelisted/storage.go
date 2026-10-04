package registeredstoragelisted // want "stale baseline entry.*required-dependency-guard\\|pkg/registeredstoragelisted\\|.*Removed"

type Service struct{ port any }

func New(p any) *Service { return &Service{port: p} }
func (s *Service) Guard() {
	if s.port == nil {
	}
}
func (s *Service) Assertion() {
	_, ok := s.port.(int)
	if !ok {
	}
}
func (s *Service) Mutated() {
	s.port = nil
	if s.port == nil {
	}
}
