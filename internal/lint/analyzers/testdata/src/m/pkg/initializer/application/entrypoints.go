package application

import d "m/durabledefs"

func compose() {
	d.BuildInvocationBootstrap()
	d.NewExecutionService()
	d.NewFakeServiceFromContractFixtures()
	d.ProjectPersistence()
}
