package operatorsettings

import (
	"strings"
	"testing"
)

// TestImplementationConstructionPortsRemainOwnerLocal proves existing Operator
// Settings implementation and construction ports remain in place for later
// IMP-SET-* absorption while the published Service seam stays peer-facing.
func TestImplementationConstructionPortsRemainOwnerLocal(t *testing.T) {
	t.Parallel()

	// Construction ports remain owner-local; peer Service method signatures do
	// not accept them as caller parameters.
	var (
		_ FileSystem
		_ CreateTemporaryFile
		_ ConfigDecoder
		_ ConfigEncoder
		_ ConfigLoader
		_ IDGenerator
		_ ProviderCatalog
		_ Config
		_ ConfigDocument
	)
}

func TestRuntimeArtifactSettingsInputNormalize_OwnsDefaultsNormalizationAndDetachedResult(t *testing.T) {
	t.Parallel()

	directory := " logs/runtime "
	maxSizeMB := 11
	maxBackups := 12
	maxAgeDays := 13
	compress := true
	got, err := (RuntimeArtifactSettingsInput{
		Directory:  &directory,
		MaxSizeMB:  &maxSizeMB,
		MaxBackups: &maxBackups,
		MaxAgeDays: &maxAgeDays,
		Compress:   &compress,
	}).Normalize("runtime.logging")
	if err != nil {
		t.Fatalf("RuntimeArtifactSettingsInput.Normalize() error = %v", err)
	}
	want := RuntimeArtifactSettings{
		Directory: "logs/runtime", MaxSizeMB: 11, MaxBackups: 12, MaxAgeDays: 13, Compress: true,
	}
	if got != want {
		t.Fatalf("normalized settings = %#v, want %#v", got, want)
	}

	directory = "mutated"
	maxSizeMB = 99
	maxBackups = 98
	maxAgeDays = 97
	compress = false
	if got != want {
		t.Fatalf("normalized settings changed with input mutation = %#v, want %#v", got, want)
	}
}

func TestRuntimeArtifactSettingsInputNormalize_UsesOwnerDefaultsForAbsentValues(t *testing.T) {
	t.Parallel()

	got, err := (RuntimeArtifactSettingsInput{}).Normalize("runtime.metrics")
	if err != nil {
		t.Fatalf("RuntimeArtifactSettingsInput.Normalize() error = %v", err)
	}
	want := RuntimeArtifactSettings{
		MaxSizeMB:  DefaultRuntimeArtifactMaxSizeMB,
		MaxBackups: DefaultRuntimeArtifactBackups,
		MaxAgeDays: DefaultRuntimeArtifactMaxAge,
	}
	if got != want {
		t.Fatalf("absent settings = %#v, want %#v", got, want)
	}
}

func TestRuntimeArtifactSettingsInputNormalize_RejectsInvalidNumericValues(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		field string
		value int
	}{
		{name: "max size zero", field: "maxSizeMB", value: 0},
		{name: "max size negative", field: "maxSizeMB", value: -1},
		{name: "max backups zero", field: "maxBackups", value: 0},
		{name: "max age negative", field: "maxAgeDays", value: -1},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			value := test.value
			input := RuntimeArtifactSettingsInput{}
			switch test.field {
			case "maxSizeMB":
				input.MaxSizeMB = &value
			case "maxBackups":
				input.MaxBackups = &value
			case "maxAgeDays":
				input.MaxAgeDays = &value
			}
			_, err := input.Normalize("runtime.logging")
			if err == nil || !strings.Contains(err.Error(), "runtime.logging."+test.field+" must be at least 1") {
				t.Fatalf("RuntimeArtifactSettingsInput.Normalize() error = %v, want invalid %s diagnostic", err, test.field)
			}
		})
	}
}
