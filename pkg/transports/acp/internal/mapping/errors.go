package mapping

import "errors"

// ErrMalformedRecord marks a record this package cannot safely project: its
// Kind is unknown, its Kind/Phase pair is not one response-draft validation
// declares legal, or its Kind/Phase pair is declared projectable but its
// Payload does not decode into the shape that combination requires. It
// never carries the payload's contents, a credential, a raw provider
// command, or a filesystem path, so it is safe to cross the ACP protocol
// boundary unchanged -- see protocol.SafeReject, which classifies it the
// same bounded way it classifies every other rejection.
var ErrMalformedRecord = errors.New("acp: malformed projectable record")
