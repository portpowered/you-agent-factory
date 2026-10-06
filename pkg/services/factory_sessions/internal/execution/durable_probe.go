package factorysessionexecution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// inspectDurableSnapshot uses the canonical persistence field types and their
// JSON validators, but discards collection members after validation. Startup
// needs identity, not a second decoded history alongside recording recovery.
func inspectDurableSnapshot(ctx context.Context, snapshot []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !json.Valid(snapshot) {
		return "", errors.New("invalid durable snapshot JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(snapshot))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return "", errors.New("durable snapshot must be an object")
	}
	var persisted PersistedRuntimeSessionState
	value := reflect.ValueOf(&persisted).Elem()
	for decoder.More() {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		key, err := decoder.Token()
		if err != nil {
			return "", err
		}
		field := durableProbeField(value, key.(string))
		if !field.IsValid() {
			var ignored json.RawMessage
			err = decoder.Decode(&ignored)
		} else if field.Kind() == reflect.Slice || field.Kind() == reflect.Map {
			err = inspectDurableCollection(ctx, decoder, field.Type())
		} else {
			err = decoder.Decode(field.Addr().Interface())
		}
		if err != nil {
			return "", err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return persisted.Session.SessionID, nil
}

// Match encoding/json's exact-name-first, then case-insensitive field lookup.
func durableProbeField(value reflect.Value, key string) reflect.Value {
	typ := value.Type()
	folded := -1
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" {
			name = field.Name
		}
		if name == key {
			return value.Field(i)
		}
		if folded < 0 && strings.EqualFold(name, key) {
			folded = i
		}
	}
	if folded >= 0 {
		return value.Field(folded)
	}
	return reflect.Value{}
}

func inspectDurableCollection(ctx context.Context, decoder *json.Decoder, typ reflect.Type) error {
	opening, err := decoder.Token()
	if err != nil || opening == nil {
		return err
	}
	want := json.Delim('[')
	if typ.Kind() == reflect.Map {
		want = json.Delim('{')
	}
	if opening != want {
		return fmt.Errorf("invalid durable collection type %s", typ)
	}
	member := reflect.New(typ.Elem())
	for decoder.More() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if typ.Kind() == reflect.Map {
			if _, err := decoder.Token(); err != nil {
				return err
			}
		}
		// A fresh value preserves canonical UnmarshalJSON validation, including
		// the tagged-record union, without retaining previous history members.
		member.Elem().SetZero()
		if err := decoder.Decode(member.Interface()); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
