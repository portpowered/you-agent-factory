package resolvedinput

// ValueKind is one canonical CLI input value kind.
type ValueKind string

const (
	ValueKindBool        ValueKind = "bool"
	ValueKindString      ValueKind = "string"
	ValueKindInt         ValueKind = "int"
	ValueKindInt64       ValueKind = "int64"
	ValueKindStringArray ValueKind = "stringArray"
)

// Value retains one canonical CLI value without serializing it through text.
// Collection data is detached when the value enters or leaves this package.
type Value struct {
	kind        ValueKind
	boolValue   bool
	stringValue string
	intValue    int
	int64Value  int64
	strings     []string
}

func BoolValue(value bool) Value {
	return Value{kind: ValueKindBool, boolValue: value}
}

func StringValue(value string) Value {
	return Value{kind: ValueKindString, stringValue: value}
}

func IntValue(value int) Value {
	return Value{kind: ValueKindInt, intValue: value}
}

func Int64Value(value int64) Value {
	return Value{kind: ValueKindInt64, int64Value: value}
}

func StringArrayValue(value []string) Value {
	return Value{kind: ValueKindStringArray, strings: cloneStrings(value)}
}

// Kind reports the canonical kind retained by the value.
func (v Value) Kind() ValueKind {
	return v.kind
}

func (v Value) clone() Value {
	v.strings = cloneStrings(v.strings)
	return v
}

func cloneStrings(value []string) []string {
	if value == nil {
		return nil
	}
	return append([]string{}, value...)
}
