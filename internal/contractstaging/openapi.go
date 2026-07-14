package contractstaging

import (
	"bytes"
	"fmt"
)

const (
	// CanonicalOpenAPIPath is the repository-owned bundled REST contract.
	CanonicalOpenAPIPath = "api/openapi.yaml"
	// StagedOpenAPIPath is the package publication projection of the bundled contract.
	StagedOpenAPIPath = "packages/api/generated/openapi/openapi.yaml"
)

// OpenAPIBytePolicy documents how staged OpenAPI may differ from the canonical
// bundle. The only allowed transform today is none: staged OpenAPI must remain
// byte-identical to api/openapi.yaml. Semantic equivalence is therefore implied
// by byte identity. Any future transform must be added here explicitly and
// covered by focused parity tests before it can change staged bytes.
type OpenAPIBytePolicy struct {
	AllowByteIdenticalCopy bool
}

// ReviewedOpenAPIBytePolicy is the enforced publication policy for staged OpenAPI.
var ReviewedOpenAPIBytePolicy = OpenAPIBytePolicy{
	AllowByteIdenticalCopy: true,
}

// ProjectStagedOpenAPI returns the reviewed package-facing OpenAPI bytes for the
// supplied canonical bundle. It rejects canonical input that would require an
// undocumented transform.
func ProjectStagedOpenAPI(canonical []byte, policy OpenAPIBytePolicy) ([]byte, error) {
	if !policy.AllowByteIdenticalCopy {
		return nil, fmt.Errorf("staged OpenAPI byte policy allows no transforms")
	}
	return append([]byte(nil), canonical...), nil
}

// VerifyStagedOpenAPIParity enforces the reviewed byte policy against staged
// package OpenAPI and fails when staged bytes diverge from the canonical bundle.
func VerifyStagedOpenAPIParity(canonical, staged []byte, policy OpenAPIBytePolicy) error {
	projected, err := ProjectStagedOpenAPI(canonical, policy)
	if err != nil {
		return err
	}
	if !bytes.Equal(projected, staged) {
		return fmt.Errorf(
			"staged OpenAPI at %s diverges from %s under the reviewed byte policy",
			StagedOpenAPIPath,
			CanonicalOpenAPIPath,
		)
	}
	return nil
}
