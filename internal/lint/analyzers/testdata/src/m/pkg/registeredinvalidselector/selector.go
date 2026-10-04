package registeredinvalidselector

type Left struct{}
type Right struct{}

func (Left) Lookup()  {}
func (Right) Lookup() {}

type View struct {
	Left
	Right
}

func Run(view View) { view.Lookup() }
