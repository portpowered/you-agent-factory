package ctor

import (
	bb "m/pkg/services/b"
)

func Use() int {
	_ = bb.Plain()
	return bb.NewThing() // want `initializer-product-construction: pkg/initializer/ctor -> pkg/services/b.NewThing`
}
