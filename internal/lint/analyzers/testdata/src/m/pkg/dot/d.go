package dot

import . "m/pkg/services/b"

var Value = NewThing // want `service-construction: pkg/dot -> pkg/services/b.NewThing`
