package durabletagged

import d "m/durabledefs"

func Linux() {
	d.NewLazyProjectStore("linux") // want "durable-persistence-construction:.*NewLazyProjectStore"
}
