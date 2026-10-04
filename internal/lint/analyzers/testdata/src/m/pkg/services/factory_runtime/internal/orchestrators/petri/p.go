package petri

type Marking map[string]int
type Token struct{ Value string }
type Color string
type Net struct{ Next *Net }
