package petrilookalike

type Marking map[string]int
type Net struct{ Next *Net }

const Kind = "PETRI"
