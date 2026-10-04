package registeredhelperlisted // want "stale baseline entry.*required-dependency-guard.*Removed"

type Service struct{}

func New(p any) *Service {
	check(p)
	ambiguous(p)
	return &Service{}
}
func check(p any) {
	if p == nil {
	}
}
func ambiguous(p any) {
	if mixed(p) == nil {
	}
}
func mixed(p any) any {
	if false {
		return nil
	}
	return p
}
