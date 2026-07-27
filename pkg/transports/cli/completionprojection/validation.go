package completionprojection

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

var (
	// ErrInvalidSchema is returned without schema details so malformed
	// sensitive values cannot enter completion diagnostics.
	ErrInvalidSchema = errors.New("selected-Factory completion schema is invalid")
	// ErrSchemaCollision reports a detached static/dynamic or dynamic/dynamic
	// input collision without retaining either colliding value.
	ErrSchemaCollision = errors.New("selected-Factory completion schema has colliding inputs")
	// ErrCancelled is stable for callers and still matches context.Canceled.
	ErrCancelled = fmt.Errorf("selected-Factory completion projection cancelled: %w", context.Canceled)
)

func cancellationError(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrCancelled
	}
	return nil
}

func validateSchema(ctx context.Context, schema climanifest.EffectiveInputSchema) error {
	if err := cancellationError(ctx); err != nil {
		return err
	}
	signatureMode, err := validateSchemaShape(schema)
	if err != nil || !signatureMode {
		return err
	}

	staticSpellings, staticBindingIDs := staticInputFacts(schema.StaticInputs)
	collisions := collisionFacts{
		staticSpellings:   staticSpellings,
		staticBindingIDs:  staticBindingIDs,
		dynamicSpellings:  make(map[string]string),
		dynamicBindingIDs: make(map[string]struct{}),
	}
	for _, parameter := range schema.FactoryParameters {
		if err := cancellationError(ctx); err != nil {
			return err
		}
		if err := collisions.addParameter(parameter); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaShape(schema climanifest.EffectiveInputSchema) (bool, error) {
	if strings.TrimSpace(schema.CommandID) == "" {
		return false, ErrInvalidSchema
	}
	switch schema.FactoryInputMode {
	case climanifest.EffectiveFactoryInputModeCompatibility:
		if len(schema.FactoryParameters) != 0 ||
			schema.UnknownNamedArgumentPolicy != "" {
			return false, ErrInvalidSchema
		}
		return false, nil
	case climanifest.EffectiveFactoryInputModeSignature:
	default:
		return false, ErrInvalidSchema
	}
	switch schema.UnknownNamedArgumentPolicy {
	case work.InvocationUnknownNamedArgumentPolicyReject,
		work.InvocationUnknownNamedArgumentPolicyAllow,
		work.InvocationUnknownNamedArgumentPolicyCollect:
	default:
		return false, ErrInvalidSchema
	}
	return true, nil
}

type collisionFacts struct {
	staticSpellings   map[string]struct{}
	staticBindingIDs  map[string]struct{}
	dynamicSpellings  map[string]string
	dynamicBindingIDs map[string]struct{}
}

func (facts *collisionFacts) addParameter(
	parameter climanifest.EffectiveFactoryParameter,
) error {
	if err := validateParameter(parameter); err != nil {
		return err
	}
	if _, exists := facts.staticBindingIDs[parameter.BindingID]; exists {
		return ErrSchemaCollision
	}
	if _, exists := facts.dynamicBindingIDs[parameter.BindingID]; exists {
		return ErrSchemaCollision
	}
	facts.dynamicBindingIDs[parameter.BindingID] = struct{}{}

	parameterSpellings, err := uniqueParameterSpellings(parameter)
	if err != nil {
		return err
	}
	for spelling := range parameterSpellings {
		if _, exists := facts.staticSpellings[spelling]; exists {
			return ErrSchemaCollision
		}
		if owner, exists := facts.dynamicSpellings[spelling]; exists &&
			owner != parameter.BindingID {
			return ErrSchemaCollision
		}
		facts.dynamicSpellings[spelling] = parameter.BindingID
	}
	return nil
}

func staticInputFacts(
	inputs []climanifest.EffectiveStaticInput,
) (map[string]struct{}, map[string]struct{}) {
	spellings := make(map[string]struct{})
	bindingIDs := make(map[string]struct{})
	for _, input := range inputs {
		if input.ID != "" {
			bindingIDs[input.ID] = struct{}{}
		}
		if input.HandlerBindingID != "" {
			bindingIDs[input.HandlerBindingID] = struct{}{}
		}
		if input.Kind == "flag" {
			for _, spelling := range input.PublicSpellings {
				if spelling = strings.TrimSpace(spelling); spelling != "" {
					spellings[spelling] = struct{}{}
				}
			}
		}
	}
	return spellings, bindingIDs
}

func validateParameter(parameter climanifest.EffectiveFactoryParameter) error {
	if parameter.BindingID == "" ||
		parameter.BindingID != strings.TrimSpace(parameter.BindingID) ||
		parameter.CanonicalName != parameter.BindingID {
		return ErrInvalidSchema
	}
	if parameter.PreferredExternalName != strings.TrimSpace(parameter.PreferredExternalName) {
		return ErrInvalidSchema
	}
	if parameter.DefaultValue != nil && len(parameter.DefaultValues) != 0 {
		return ErrInvalidSchema
	}
	for _, binding := range parameter.Bindings {
		switch binding.Kind {
		case work.InvocationParameterBindingKindNamed,
			work.InvocationParameterBindingKindNamedRest,
			work.InvocationParameterBindingKindPositional,
			work.InvocationParameterBindingKindStdin:
		default:
			return ErrInvalidSchema
		}
		if binding.Kind == work.InvocationParameterBindingKindNamed &&
			strings.TrimSpace(parameter.PreferredExternalName) == "" {
			return ErrInvalidSchema
		}
	}
	for _, alias := range parameter.Aliases {
		if alias == "" || alias != strings.TrimSpace(alias) {
			return ErrInvalidSchema
		}
	}
	return nil
}

func uniqueParameterSpellings(
	parameter climanifest.EffectiveFactoryParameter,
) (map[string]struct{}, error) {
	spellings := make(map[string]struct{}, len(parameter.Aliases)+2)
	spellings[parameter.CanonicalName] = struct{}{}
	if parameter.PreferredExternalName != "" &&
		parameter.PreferredExternalName != parameter.CanonicalName {
		spellings[parameter.PreferredExternalName] = struct{}{}
	}
	for _, alias := range parameter.Aliases {
		if _, exists := spellings[alias]; exists {
			return nil, ErrSchemaCollision
		}
		spellings[alias] = struct{}{}
	}
	return spellings, nil
}
