package session

import (
	"bytes"
	"encoding/json"
	"errors"
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

const createRequestTimeout = 10 * time.Second

// ErrFactorySessionTargetsRequireSelection is returned when the API reports runnable
// targets but does not open a session until the operator disambiguates.
var ErrFactorySessionTargetsRequireSelection = errors.New("factory session target selection required")

// CreateConfig holds parameters for the session create command.
type CreateConfig struct {
	Port           int
	Dir            string
	InitNewFactory bool
	ValidateOnly   bool
	TargetKind     string
	TargetName     string
	JSON           bool
	Verbose        bool
	Debug          bool
	Output         io.Writer
	Diagnostics    io.Writer
}

// Create opens a live factory session on a running host via HTTP.
func Create(cfg CreateConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	folderPath := strings.TrimSpace(cfg.Dir)
	if folderPath == "" {
		return fmt.Errorf("folder path is required (--dir)")
	}
	if cfg.InitNewFactory && cfg.ValidateOnly {
		return fmt.Errorf("init-new-factory cannot be combined with validate-only")
	}

	target, err := targetRefFromFlags(cfg.TargetKind, cfg.TargetName)
	if err != nil {
		return err
	}

	request := factoryapi.OpenFactorySessionRequest{
		FolderPath: folderPath,
		Target:     target,
	}
	if cfg.InitNewFactory {
		initNewFactory := true
		request.InitNewFactory = &initNewFactory
	}
	if cfg.ValidateOnly {
		validateOnly := true
		request.ValidateOnly = &validateOnly
	}

	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal open factory session request: %w", err)
	}

	endpoint := createEndpoint(cfg)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session create request endpointPath=%s endpoint=%s port=%d folderPath=%s validateOnly=%t initNewFactory=%t",
		endpoint.Path,
		endpoint.String(),
		cfg.Port,
		folderPath,
		cfg.ValidateOnly,
		cfg.InitNewFactory,
	)

	client := &http.Client{Timeout: createRequestTimeout}
	started := time.Now()
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build open factory session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session create response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, time.Since(started).Milliseconds())
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var result factoryapi.OpenFactorySessionResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}
		clidiag.Printf(
			cfg.Diagnostics,
			cfg.Verbose,
			"session create response endpointPath=%s status=%d durationMillis=%d hasSession=%t targetCount=%d",
			endpoint.Path,
			resp.StatusCode,
			time.Since(started).Milliseconds(),
			result.Session != nil,
			targetCount(result.Targets),
		)
		return renderCreateResult(cfg, result)
	case http.StatusBadRequest:
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session create response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds())
		return createStatusError(resp)
	default:
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session create response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds())
		return createStatusError(resp)
	}
}

func createEndpoint(cfg CreateConfig) url.URL {
	return url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", cfg.Port),
		Path:   "/factory-sessions",
	}
}

func targetRefFromFlags(kind, name string) (*factoryapi.FactorySessionTargetRef, error) {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	if kind == "" && name == "" {
		return nil, nil
	}
	if kind == "" {
		return nil, fmt.Errorf("target kind is required when --target-name is set")
	}

	switch factoryapi.FactorySessionTargetRefKind(kind) {
	case factoryapi.FactorySessionTargetRefKindDefault:
		if name != "" {
			return nil, fmt.Errorf("target name cannot be set when --target-kind is default")
		}
		return &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindDefault,
		}, nil
	case factoryapi.FactorySessionTargetRefKindNamed:
		ref := &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindNamed,
		}
		if name != "" {
			ref.Name = &name
		}
		return ref, nil
	default:
		return nil, fmt.Errorf("unsupported target kind %q (want default or named)", kind)
	}
}

func renderCreateResult(cfg CreateConfig, result factoryapi.OpenFactorySessionResponse) error {
	if result.Session != nil {
		if cfg.JSON {
			return json.NewEncoder(cfg.Output).Encode(result)
		}
		return renderCreateSessionSuccess(cfg.Output, *result.Session)
	}

	if result.Targets != nil && len(*result.Targets) > 0 {
		if cfg.JSON {
			if err := json.NewEncoder(cfg.Output).Encode(result); err != nil {
				return err
			}
			return fmt.Errorf("%w", ErrFactorySessionTargetsRequireSelection)
		}
		return renderCreateTargetPicker(cfg.Output, *result.Targets)
	}

	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}
	return nil
}

func renderCreateSessionSuccess(output io.Writer, session factoryapi.FactorySessionSummary) error {
	if _, err := fmt.Fprintf(
		output,
		"Opened factory session %s\nProject: %s\nFolder path: %s\nDefault: %s\n",
		session.Id,
		session.Project,
		session.FolderPath,
		defaultMarker(session.IsDefault),
	); err != nil {
		return err
	}
	return nil
}

func renderCreateTargetPicker(output io.Writer, targets []factoryapi.FactorySessionTarget) error {
	if _, err := fmt.Fprintln(output, "Multiple factory targets are available; choose one with --target-kind and --target-name:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "LABEL\tREF"); err != nil {
		return err
	}
	for _, target := range targets {
		if _, err := fmt.Fprintf(output, "%s\t%s\n", target.Label, formatTargetRef(target.Ref)); err != nil {
			return err
		}
	}
	return fmt.Errorf("%w", ErrFactorySessionTargetsRequireSelection)
}

func formatTargetRef(ref factoryapi.FactorySessionTargetRef) string {
	if ref.Kind == factoryapi.FactorySessionTargetRefKindDefault {
		return "default"
	}
	name := ""
	if ref.Name != nil {
		name = strings.TrimSpace(*ref.Name)
	}
	return "named:" + name
}

func createStatusError(resp *http.Response) error {
	var errResp factoryapi.ErrorResponse
	if json.NewDecoder(resp.Body).Decode(&errResp) == nil && errResp.Message != "" {
		return fmt.Errorf("open factory session failed (%d): %s", resp.StatusCode, errResp.Message)
	}
	return fmt.Errorf("open factory session failed (%d)", resp.StatusCode)
}

func targetCount(targets *[]factoryapi.FactorySessionTarget) int {
	if targets == nil {
		return 0
	}
	return len(*targets)
}
