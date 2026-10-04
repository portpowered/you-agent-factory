package durable

import d "m/durabledefs"

func build() {
	d.NewExecutionService() // want "durable-application-composition:.*NewExecutionService"
}
