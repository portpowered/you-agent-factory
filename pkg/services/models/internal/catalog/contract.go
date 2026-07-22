// Package catalog retains same-service compatibility names while canonical
// catalog contracts live at pkg/services/models.
package catalog

import models "github.com/portpowered/infinite-you/pkg/services/models"

var ErrUnsupportedOperation = models.ErrUnsupportedOperation

type Status = models.Status

const (
	StatusReady       = models.StatusReady
	StatusUnavailable = models.StatusUnavailable
)

type LoadState = models.LoadState

const (
	LoadStateUnloaded      = models.LoadStateUnloaded
	LoadStateNotApplicable = models.LoadStateNotApplicable
)

type ResourceSummary = models.ResourceSummary
type Capability = models.Capability
type Summary = models.Summary
type Detail = models.Detail
type Entry = models.Entry
type List = models.List
