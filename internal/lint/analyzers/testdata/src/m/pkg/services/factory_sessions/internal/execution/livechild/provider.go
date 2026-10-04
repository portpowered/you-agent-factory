package livechild

import d "m/durabledefs"

func direct(p d.Provider) {
	p.Infer() // want "durable-live-child-provider:.*Infer"
}

func shared(p d.Worker) { p.Execute() }
