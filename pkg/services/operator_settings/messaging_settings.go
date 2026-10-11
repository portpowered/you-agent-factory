package operatorsettings

import (
	"errors"
	"strings"
	"time"
)

// ErrMessagingSettingsInvalid quarantines Messaging without invalidating the
// operator's unrelated Factory configuration. Diagnostics never include values.
var ErrMessagingSettingsInvalid = errors.New("messaging settings are invalid")

// MessagingSettings retains authored presence, including explicit false/zero.
// InvalidJSON preserves a malformed Messaging section during unrelated edits;
// only the global-config codec populates it and Effective refuses to use it.
type MessagingSettings struct {
	Enabled       *bool            `json:"enabled,omitempty"`
	RetentionDays *int             `json:"retentionDays,omitempty"`
	Policy        *MessagingPolicy `json:"policy,omitempty"`
	Limits        *MessagingLimits `json:"limits,omitempty"`
	InvalidJSON   []byte           `json:"-"`
}

type MessagingPolicy struct {
	ScopeLabelKeys  *[]string `json:"scopeLabelKeys,omitempty"`
	AllowCrossScope *bool     `json:"allowCrossScope,omitempty"`
}

type MessagingLimits struct {
	SendsPerChainPerHour *int `json:"sendsPerChainPerHour,omitempty"`
	MessagesPerThread    *int `json:"messagesPerThread,omitempty"`
	MaxBodyBytes         *int `json:"maxBodyBytes,omitempty"`
	MaxHop               *int `json:"maxHop,omitempty"`
	DefaultExpirySeconds *int `json:"defaultExpirySeconds,omitempty"`
}

// EffectiveMessagingSettings is the validated, detached startup configuration.
// T5 never enables cross-scope policy; label keys are relationship selectors,
// never authority on their own.
type EffectiveMessagingSettings struct {
	Enabled              bool
	RetentionDays        int
	ScopeLabelKeys       []string
	SendsPerChainPerHour int
	MessagesPerThread    int
	MaxBodyBytes         int
	MaxHop               int
	DefaultExpirySeconds int
}

func (settings *MessagingSettings) Effective() (EffectiveMessagingSettings, error) {
	result := EffectiveMessagingSettings{
		Enabled: true, RetentionDays: 30, ScopeLabelKeys: []string{"tag:project"},
		SendsPerChainPerHour: 10, MessagesPerThread: 8, MaxBodyBytes: 8192,
		MaxHop: 3, DefaultExpirySeconds: 86400,
	}
	if settings == nil {
		return result, nil
	}
	if len(settings.InvalidJSON) != 0 {
		return EffectiveMessagingSettings{}, ErrMessagingSettingsInvalid
	}
	if settings.Enabled != nil {
		result.Enabled = *settings.Enabled
	}
	result.RetentionDays = messagingValue(settings.RetentionDays, result.RetentionDays)
	if err := settings.applyPolicy(&result); err != nil {
		return EffectiveMessagingSettings{}, err
	}
	if settings.Limits != nil {
		limits := settings.Limits
		result.SendsPerChainPerHour = messagingValue(limits.SendsPerChainPerHour, result.SendsPerChainPerHour)
		result.MessagesPerThread = messagingValue(limits.MessagesPerThread, result.MessagesPerThread)
		result.MaxBodyBytes = messagingValue(limits.MaxBodyBytes, result.MaxBodyBytes)
		result.MaxHop = messagingValue(limits.MaxHop, result.MaxHop)
		result.DefaultExpirySeconds = messagingValue(limits.DefaultExpirySeconds, result.DefaultExpirySeconds)
	}
	if !result.valid() {
		return EffectiveMessagingSettings{}, ErrMessagingSettingsInvalid
	}
	return result, nil
}

func (settings *MessagingSettings) applyPolicy(result *EffectiveMessagingSettings) error {
	if settings.Policy != nil {
		if settings.Policy.AllowCrossScope != nil && *settings.Policy.AllowCrossScope {
			return ErrMessagingSettingsInvalid
		}
		if settings.Policy.ScopeLabelKeys != nil {
			result.ScopeLabelKeys = append([]string{}, (*settings.Policy.ScopeLabelKeys)...)
		}
		for _, key := range result.ScopeLabelKeys {
			if strings.TrimSpace(key) == "" {
				return ErrMessagingSettingsInvalid
			}
		}
	}
	return nil
}

func (result EffectiveMessagingSettings) valid() bool {
	// Bound retention before the later duration conversion, including 32-bit
	// builds. The upper bound is representability, not a new product limit.
	return result.RetentionDays >= 1 && int64(result.RetentionDays) <= (1<<63-1)/int64(24*time.Hour) &&
		result.SendsPerChainPerHour >= 1 && result.MessagesPerThread >= 1 &&
		result.MaxBodyBytes >= 1 && result.MaxBodyBytes <= 8192 && result.MaxHop >= 0 && result.MaxHop <= 3 &&
		result.DefaultExpirySeconds >= 60 && result.DefaultExpirySeconds <= 604800
}

func messagingValue[T any](value *T, fallback T) T {
	if value != nil {
		return *value
	}
	return fallback
}

func messagingPointerCopy[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func (settings *MessagingSettings) Clone() *MessagingSettings {
	if settings == nil {
		return nil
	}
	cloned := *settings
	cloned.Enabled = messagingPointerCopy(settings.Enabled)
	cloned.RetentionDays = messagingPointerCopy(settings.RetentionDays)
	cloned.InvalidJSON = append([]byte(nil), settings.InvalidJSON...)
	if settings.Policy != nil {
		policy := *settings.Policy
		policy.AllowCrossScope = messagingPointerCopy(policy.AllowCrossScope)
		if policy.ScopeLabelKeys != nil {
			keys := append([]string{}, (*policy.ScopeLabelKeys)...)
			policy.ScopeLabelKeys = &keys
		}
		cloned.Policy = &policy
	}
	if settings.Limits != nil {
		limits := *settings.Limits
		limits.SendsPerChainPerHour = messagingPointerCopy(limits.SendsPerChainPerHour)
		limits.MessagesPerThread = messagingPointerCopy(limits.MessagesPerThread)
		limits.MaxBodyBytes = messagingPointerCopy(limits.MaxBodyBytes)
		limits.MaxHop = messagingPointerCopy(limits.MaxHop)
		limits.DefaultExpirySeconds = messagingPointerCopy(limits.DefaultExpirySeconds)
		cloned.Limits = &limits
	}
	return &cloned
}
