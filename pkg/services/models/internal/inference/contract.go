// Package inference retains same-service compatibility names while canonical
// invocation contracts live at pkg/services/models.
package inference

import models "github.com/portpowered/infinite-you/pkg/services/models"

var ErrUnsupportedResponseMode = models.ErrUnsupportedResponseMode

type ResponseMode = models.ResponseMode

const ResponseModeAudioStream = models.ResponseModeAudioStream

type Options = models.Options
type Request = models.Request
type Result = models.Result
type ResolvedModelOperationBinding = models.ResolvedModelOperationBinding
type TargetError = models.TargetError
