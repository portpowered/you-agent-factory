package globalconfig_test

import (
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	"testing"
)

func TestDirectSessionCapacityCodec(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"null", "0", "-1", "1.5", "true", `"2"`, "9223372036854775808", "1e1"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if _, err := globalconfig.Decode([]byte(fmt.Sprintf(`{"defaults":{"directSessionCapacity":%s}}`, value))); err == nil {
				t.Fatal("invalid authored capacity accepted")
			}
		})
	}
	for _, value := range []string{"1", "2", "9223372036854775807"} {
		t.Run("valid-"+value, func(t *testing.T) {
			t.Parallel()
			config, err := globalconfig.Decode([]byte(fmt.Sprintf(`{"defaults":{"directSessionCapacity":%s,"workerModel":"generic","subagent":{"provider":"codex","model":"test-model"}},"runtime":{"logging":{"maxSizeMB":42}}}`, value)))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := globalconfig.Encode(config)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := globalconfig.Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Defaults.DirectSessionCapacity == nil || *loaded.Defaults.DirectSessionCapacity != *config.Defaults.DirectSessionCapacity || loaded.Defaults.Subagent.Model != "test-model" || loaded.Defaults.WorkerModel != "generic" || loaded.Runtime.Logging.MaxSizeMB != 42 {
				t.Fatal("roundtrip lost settings")
			}
		})
	}
	omitted, err := globalconfig.Decode([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	capacity, err := omitted.Defaults.ResolveDirectSessionCapacity()
	if err != nil || capacity != 16 {
		t.Fatalf("omission=%d %v", capacity, err)
	}
}
