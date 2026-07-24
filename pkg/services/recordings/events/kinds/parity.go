package factoryeventkinds

import (
	"fmt"
	"sort"
	"strings"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// FactoryEventKindParityDrift names kinds that differ between the public
// runtime-emittable inventory and the OpenAPI FactoryEvent discriminator mapping.
type FactoryEventKindParityDrift struct {
	RuntimeOnlyKinds  []factorycontracts.FactoryEventType
	ContractOnlyKinds []factorycontracts.FactoryEventType
}

// FactoryEventKindParityInput carries the inventories compared by parity checks.
type FactoryEventKindParityInput struct {
	RuntimeKinds        []PublicEmittableKind
	ContractOnlyKinds   []ContractOnlyKind
	OpenAPIMappingKinds []factorycontracts.FactoryEventType
}

// CompareFactoryEventKindParity returns drift between the runtime public inventory
// and OpenAPI mapping keys. Contract-only kinds that are already classified with
// non-empty evidence are not reported as unexplained contract-only drift.
func CompareFactoryEventKindParity(input FactoryEventKindParityInput) FactoryEventKindParityDrift {
	runtimeSet := make(map[factorycontracts.FactoryEventType]struct{}, len(input.RuntimeKinds))
	for _, entry := range input.RuntimeKinds {
		runtimeSet[entry.Kind] = struct{}{}
	}

	classifiedContractOnly := make(map[factorycontracts.FactoryEventType]struct{}, len(input.ContractOnlyKinds))
	for _, entry := range input.ContractOnlyKinds {
		if strings.TrimSpace(entry.Evidence) == "" {
			continue
		}
		classifiedContractOnly[entry.Kind] = struct{}{}
	}

	openAPISet := make(map[factorycontracts.FactoryEventType]struct{}, len(input.OpenAPIMappingKinds))
	for _, kind := range input.OpenAPIMappingKinds {
		openAPISet[kind] = struct{}{}
	}

	var runtimeOnly []factorycontracts.FactoryEventType
	for kind := range runtimeSet {
		if _, ok := openAPISet[kind]; !ok {
			runtimeOnly = append(runtimeOnly, kind)
		}
	}
	sort.Slice(runtimeOnly, func(i, j int) bool {
		return runtimeOnly[i] < runtimeOnly[j]
	})

	var contractOnly []factorycontracts.FactoryEventType
	for kind := range openAPISet {
		if _, inRuntime := runtimeSet[kind]; inRuntime {
			continue
		}
		if _, classified := classifiedContractOnly[kind]; classified {
			continue
		}
		contractOnly = append(contractOnly, kind)
	}
	sort.Slice(contractOnly, func(i, j int) bool {
		return contractOnly[i] < contractOnly[j]
	})

	return FactoryEventKindParityDrift{
		RuntimeOnlyKinds:  runtimeOnly,
		ContractOnlyKinds: contractOnly,
	}
}

// ValidateFactoryEventKindParity fails closed when any runtime-only or unexplained
// contract-only public FactoryEvent kinds exist.
func ValidateFactoryEventKindParity(input FactoryEventKindParityInput) error {
	drift := CompareFactoryEventKindParity(input)
	if len(drift.RuntimeOnlyKinds) == 0 && len(drift.ContractOnlyKinds) == 0 {
		return nil
	}
	return drift
}

func (d FactoryEventKindParityDrift) Error() string {
	parts := make([]string, 0, 2)
	if len(d.RuntimeOnlyKinds) > 0 {
		parts = append(parts, fmt.Sprintf(
			"runtime-only factory event kinds: %s",
			formatFactoryEventKinds(d.RuntimeOnlyKinds),
		))
	}
	if len(d.ContractOnlyKinds) > 0 {
		parts = append(parts, fmt.Sprintf(
			"contract-only factory event kinds: %s",
			formatFactoryEventKinds(d.ContractOnlyKinds),
		))
	}
	return strings.Join(parts, "; ")
}

// CurrentFactoryEventKindInventory returns the Recordings-owned compile-time
// vocabulary used by ownership and parity consumers.
func CurrentFactoryEventKindInventory() FactoryEventKindInventory {
	return FactoryEventKindInventory{
		PublicEmittable: PublicEmittableFactoryEventKinds(),
		Excluded:        ExcludedNonPublicFactoryEventKinds(),
		ContractOnly:    ContractOnlyFactoryEventKinds(),
	}
}

// ValidateBundledFactoryEventKindParity proves public runtime-emittable FactoryEvent
// kinds have closed parity against the bundled OpenAPI discriminator mapping
// through the Recordings-owned inventory: the frozen vocabulary validates, then
// parity reports zero runtime-only kinds and zero unexplained contract-only kinds.
func ValidateBundledFactoryEventKindParity(openAPIYAML []byte) error {
	if err := ValidateFactoryEventKindInventory(CurrentFactoryEventKindInventory()); err != nil {
		return fmt.Errorf("recordings-owned factory event kind inventory: %w", err)
	}
	input, err := LoadFactoryEventKindParityInputFromOpenAPIYAML(openAPIYAML)
	if err != nil {
		return err
	}
	return ValidateFactoryEventKindParity(input)
}

func formatFactoryEventKinds(kinds []factorycontracts.FactoryEventType) string {
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, string(kind))
	}
	return strings.Join(names, ", ")
}
