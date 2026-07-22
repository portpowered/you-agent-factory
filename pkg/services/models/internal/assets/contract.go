package assets

import models "github.com/portpowered/infinite-you/pkg/services/models"

var (
	ErrNotAvailable      = models.ErrNotAvailable
	ErrPullUnsupported   = models.ErrPullUnsupported
	ErrSourceFetchFailed = models.ErrSourceFetchFailed
)

type DownloadedFile = models.DownloadedFile
type PullResult = models.PullResult
type PullError = models.PullError
