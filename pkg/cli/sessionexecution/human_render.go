package sessionexecution

import (
	"fmt"
	"io"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func writeOptionalTrimmedLine(output io.Writer, label, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	_, err := fmt.Fprintf(output, "%s: %s\n", label, trimmed)
	return err
}

func writeResolvedSourceHuman(
	output io.Writer,
	sourceHash *string,
	resolvedSource factoryapi.FactorySessionResolvedSourceIdentity,
) error {
	if sourceHash != nil && strings.TrimSpace(*sourceHash) != "" {
		_, err := fmt.Fprintf(output, "Source hash: %s\n", strings.TrimSpace(*sourceHash))
		return err
	}
	if ref := resolvedSource.SourceRef; ref != nil {
		return writeOptionalTrimmedLine(output, "Source ref", *ref)
	}
	return nil
}

func writeExecutionLinksHuman(output io.Writer, links *factoryapi.FactorySessionExecutionLinks) error {
	if links == nil {
		return nil
	}
	if links.Status != nil {
		if err := writeOptionalTrimmedLine(output, "Status link", *links.Status); err != nil {
			return err
		}
	}
	if links.Session != nil {
		if err := writeOptionalTrimmedLine(output, "Session link", *links.Session); err != nil {
			return err
		}
	}
	if links.Results != nil {
		if err := writeOptionalTrimmedLine(output, "Results link", *links.Results); err != nil {
			return err
		}
	}
	return nil
}

func writeResultAvailabilityHuman(output io.Writer, availability *factoryapi.FactorySessionResultAvailabilityDetail) error {
	if availability == nil {
		return nil
	}
	if reason := availability.Reason; reason != nil {
		if err := writeOptionalTrimmedLine(output, "Availability reason", *reason); err != nil {
			return err
		}
	}
	if message := availability.Message; message != nil {
		if err := writeOptionalTrimmedLine(output, "Availability message", *message); err != nil {
			return err
		}
	}
	if retryable := availability.Retryable; retryable != nil && *retryable {
		if _, err := fmt.Fprintln(output, "Retryable: true"); err != nil {
			return err
		}
	}
	return nil
}

func writeResultFailureHuman(output io.Writer, failure *factoryapi.FactorySessionDurableFailureDetail) error {
	if failure == nil {
		return nil
	}
	if reason := failure.Reason; reason != nil {
		if err := writeOptionalTrimmedLine(output, "Failure reason", *reason); err != nil {
			return err
		}
	}
	if message := failure.Message; message != nil {
		if err := writeOptionalTrimmedLine(output, "Failure message", *message); err != nil {
			return err
		}
	}
	return nil
}
