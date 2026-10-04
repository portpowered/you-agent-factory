package registeredtagged

import "m/pkg/registeredowner"

func Linux() { registeredowner.New(nil) } // want "registered-construction"
