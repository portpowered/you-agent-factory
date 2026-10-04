package durabletagged

import d "m/durabledefs"

func Windows() {
	d.NewLazyProjectStore("windows") // want "durable-persistence-construction:.*NewLazyProjectStore"
}
