package cli

import (
	"net/http"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type rootTestHTTPClock struct{}

func (rootTestHTTPClock) Now() time.Time { return time.Unix(1, 0) }

func rootTestHTTPProtocol() clihttp.Protocol {
	protocol, err := clihttp.NewProtocol(&http.Client{}, rootTestHTTPClock{})
	if err != nil {
		panic(err)
	}
	return protocol
}
