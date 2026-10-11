package globalconfig

import (
	"bytes"
	"encoding/json"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// Messaging is decoded separately from generated GlobalConfig: a wrong type
// in this optional section must quarantine only Messaging, never Factory
// startup or unrelated settings updates. The exact invalid JSON is retained
// solely for persistence; no parser diagnostic or authored value is published.
func detachMessaging(raw []byte) ([]byte, *operatorsettings.MessagingSettings, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return nil, nil, err
	}
	if opening != json.Delim('{') {
		return raw, nil, nil
	}
	object := make(map[string]json.RawMessage)
	var section json.RawMessage
	// Like encoding/json, the last authored occurrence wins, including
	// case-insensitive aliases. Map iteration must never choose activation.
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, nil, err
		}
		key := keyToken.(string)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, err
		}
		if strings.EqualFold(key, "messaging") {
			section = value
		} else {
			object[key] = value
		}
	}
	remaining, err := json.Marshal(object)
	if err != nil || section == nil {
		return remaining, nil, err
	}
	return remaining, decodeMessagingSection(section), nil
}

func decodeMessagingSection(section []byte) *operatorsettings.MessagingSettings {
	settings := &operatorsettings.MessagingSettings{}
	sectionDecoder := json.NewDecoder(bytes.NewReader(section))
	sectionDecoder.DisallowUnknownFields()
	if err := sectionDecoder.Decode(settings); err != nil || messagingHasNull(section) {
		return &operatorsettings.MessagingSettings{InvalidJSON: append([]byte(nil), section...)}
	}
	return settings
}

func messagingHasNull(raw []byte) bool {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return true
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) == nil && fields != nil {
		for _, field := range fields {
			if messagingHasNull(field) {
				return true
			}
		}
	}
	return false
}

func attachMessaging(raw []byte, settings *operatorsettings.MessagingSettings) ([]byte, error) {
	if settings == nil {
		return raw, nil
	}
	section := settings.InvalidJSON
	if len(section) == 0 {
		var err error
		section, err = json.Marshal(settings)
		if err != nil {
			return nil, err
		}
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	object["messaging"] = section
	return json.MarshalIndent(object, "", "  ")
}
