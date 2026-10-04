package behaviorfixture

import w "m/pkg/services/work"

func mapKind(k w.Kind) w.Kind { return k.Normalized() } // want `transport-work-content-normalization:.*Normalized`
type unrelated struct{}

func (u unrelated) Normalized() unrelated { return u }
func mapOther(u unrelated) unrelated      { return u.Normalized() }
