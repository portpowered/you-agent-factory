//go:build backendconformance

package registeredtagged

import "m/pkg/registeredowner"

func Tagged() { registeredowner.New(nil) } // want "registered-construction"
