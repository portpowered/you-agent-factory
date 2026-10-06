//go:build !windows && !linux

package process

import "fmt"

func (IncarnationProbe) processStart(int) (string, error) {
	return "", fmt.Errorf("OS process incarnation query unavailable")
}
