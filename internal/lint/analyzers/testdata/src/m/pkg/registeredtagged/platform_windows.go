package registeredtagged

import "m/pkg/registeredowner"

func Windows() { registeredowner.New(nil) } // want "registered-construction"
