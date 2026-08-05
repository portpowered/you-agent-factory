// Package snapshots_portability defines the Factory Definitions-owned private
// snapshot portability capability for detached Factory snapshot capture,
// prepare-import, and bundled-asset materialize behind the CTR-DEF root
// snapshot slice.
//
// Consumers outside Factory Definitions use the outer Factory Definitions root
// Service instead of this private subservice contract.
//
// The public surface exposes only CTR-DEF snapshot vocabulary and exact
// injected host-effect ports. It does not declare Runtime/Recordings types,
// peer service implementations, Wire/root construction ownership, filesystem
// effect concrete types, or sibling catalog/authoring_layout/compilation/
// validation/distribution APIs.
package snapshotsportability

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// Service is the private name retained for root compatibility forwarding. New
// consumers receive the public focused Snapshots capability directly.
type Service = factorydefinitions.Snapshots
