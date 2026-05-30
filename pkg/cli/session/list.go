// Package session implements factory-session lifecycle command behavior.
package session

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
)

const listRequestTimeout = 10 * time.Second

// ListConfig holds parameters for the session list command.
type ListConfig struct {
	Port        int
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// List requests live factory sessions from a running host via HTTP.
func List(cfg ListConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	endpoint := listEndpoint(cfg)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session list request endpointPath=%s endpoint=%s port=%d",
		endpoint.Path,
		endpoint.String(),
		cfg.Port,
	)

	client := &http.Client{Timeout: listRequestTimeout}
	started := time.Now()
	resp, err := client.Get(endpoint.String())
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session list response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, time.Since(started).Milliseconds())
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session list response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds())
		var errResp factoryapi.ErrorResponse
		if json.NewDecoder(resp.Body).Decode(&errResp) == nil && errResp.Message != "" {
			return fmt.Errorf("list factory sessions failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("list factory sessions failed (%d)", resp.StatusCode)
	}

	var result factoryapi.ListFactorySessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session list response endpointPath=%s status=%d durationMillis=%d sessionCount=%d",
		endpoint.Path,
		resp.StatusCode,
		time.Since(started).Milliseconds(),
		len(result.Sessions),
	)
	if cfg.JSON {
		encoder := json.NewEncoder(cfg.Output)
		return encoder.Encode(result)
	}
	return renderListResult(cfg.Output, result)
}

func listEndpoint(cfg ListConfig) url.URL {
	return url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", cfg.Port),
		Path:   "/factory-sessions",
	}
}

func renderListResult(output io.Writer, result factoryapi.ListFactorySessionsResponse) error {
	if len(result.Sessions) == 0 {
		_, err := fmt.Fprintln(output, "No live factory sessions were found.")
		return err
	}

	if _, err := fmt.Fprintln(output, "SESSION ID\tPROJECT\tFOLDER PATH\tFACTORY DIR\tDEFAULT\tTARGET KIND\tTARGET NAME"); err != nil {
		return err
	}
	for _, session := range result.Sessions {
		if _, err := fmt.Fprintf(
			output,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			session.Id,
			session.Project,
			session.FolderPath,
			session.FactoryDir,
			defaultMarker(session.IsDefault),
			session.Target.Kind,
			targetName(session.Target.Name),
		); err != nil {
			return err
		}
	}
	return nil
}

func defaultMarker(isDefault bool) string {
	if isDefault {
		return "yes"
	}
	return "no"
}

func targetName(name *string) string {
	if name == nil {
		return ""
	}
	return strings.TrimSpace(*name)
}
