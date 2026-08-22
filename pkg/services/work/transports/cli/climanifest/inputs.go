package climanifest

import "fmt"

// Argument is one positional input record from the production manifest.
type Argument struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Position         int         `json:"position"`
	Kind             string      `json:"kind"`
	ValueType        string      `json:"valueType"`
	Scope            string      `json:"scope,omitempty"`
	Required         bool        `json:"required"`
	MinCardinality   int         `json:"minCardinality"`
	MaxCardinality   int         `json:"maxCardinality"`
	Variadic         bool        `json:"variadic"`
	DefaultValue     *InputValue `json:"defaultValue,omitempty"`
	Enum             []string    `json:"enum"`
	Pattern          string      `json:"pattern"`
	DoubleDash       string      `json:"doubleDash"`
	Completion       string      `json:"completion"`
	Channels         []string    `json:"channels"`
	AcceptedSources  []string    `json:"acceptedSources,omitempty"`
	HandlerBindingID string      `json:"handlerBindingId,omitempty"`
	Visibility       string      `json:"visibility,omitempty"`
	Lifecycle        Lifecycle   `json:"lifecycle,omitempty"`
}

// Flag is one flag input record from the production manifest.
type Flag struct {
	ID               string      `json:"id"`
	Long             string      `json:"long"`
	Shorthand        string      `json:"shorthand"`
	Aliases          []string    `json:"aliases"`
	Scope            string      `json:"scope"`
	InheritedFromID  string      `json:"inheritedFromInputId,omitempty"`
	ValueType        string      `json:"valueType"`
	Enum             []string    `json:"enum,omitempty"`
	Required         bool        `json:"required"`
	Default          string      `json:"default"`
	ChangedDefault   bool        `json:"changedDefault"`
	NoOptionDefault  string      `json:"noOptionDefault"`
	Repeatable       bool        `json:"repeatable"`
	Normalization    string      `json:"normalization"`
	Completion       string      `json:"completion"`
	Binding          string      `json:"binding"`
	Visibility       string      `json:"visibility"`
	Lifecycle        Lifecycle   `json:"lifecycle"`
	Kind             string      `json:"kind,omitempty"`
	MinCardinality   int         `json:"minCardinality,omitempty"`
	MaxCardinality   int         `json:"maxCardinality,omitempty"`
	DefaultValue     *InputValue `json:"defaultValue,omitempty"`
	NoOptionValue    *InputValue `json:"noOptionDefaultValue,omitempty"`
	AcceptedSources  []string    `json:"acceptedSources,omitempty"`
	HandlerBindingID string      `json:"handlerBindingId,omitempty"`
	Usage            string      `json:"usage,omitempty"`
	Sensitivity      string      `json:"sensitivity,omitempty"`
}

// InputValue is a transport-neutral, explicitly typed scalar or repeated input
// value. Exactly one member is authored; semantic validation enforces that the
// selected member agrees with the input's valueType.
type InputValue struct {
	Boolean     *bool     `json:"boolean,omitempty"`
	String      *string   `json:"string,omitempty"`
	Int         *int      `json:"int,omitempty"`
	Int64       *int64    `json:"int64,omitempty"`
	StringArray *[]string `json:"stringArray,omitempty"`
}

// ArgumentAt returns the argument record for one position.
func (c Command) ArgumentAt(position int) (Argument, bool) {
	if c.Arguments == nil {
		return Argument{}, false
	}
	for _, arg := range c.Arguments {
		if arg.Position == position {
			return arg, true
		}
	}
	return Argument{}, false
}

// FlagByLong returns the flag record for one long name.
func (c Command) FlagByLong(longName string) (Flag, bool) {
	if c.Flags == nil {
		return Flag{}, false
	}
	for _, flag := range c.Flags {
		if flag.Long == longName {
			return flag, true
		}
	}
	return Flag{}, false
}

// RequireArgumentAt returns an argument record or an error.
func (c Command) RequireArgumentAt(position int) (Argument, error) {
	arg, ok := c.ArgumentAt(position)
	if !ok {
		return Argument{}, fmt.Errorf("command %q missing argument position %d", c.ID, position)
	}
	return arg, nil
}

// RequireFlagByLong returns a flag record or an error.
func (c Command) RequireFlagByLong(longName string) (Flag, error) {
	flag, ok := c.FlagByLong(longName)
	if !ok {
		return Flag{}, fmt.Errorf("command %q missing flag %q", c.ID, longName)
	}
	return flag, nil
}
