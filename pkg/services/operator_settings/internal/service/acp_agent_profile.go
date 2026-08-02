package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// acpAgentProfileStoreFileName is the Operator-Settings-owned storage file
// name for the persisted ACP agent profile. It lives beside the operator
// config document but is decoded/encoded independently of the GlobalConfig
// contract, keeping this V0 slice free of any OpenAPI dependency.
const acpAgentProfileStoreFileName = "acp-agent-profile.json"

// acpAgentProfileFile is the on-disk representation of one persisted ACP
// agent profile.
type acpAgentProfileFile struct {
	DefaultFactoryReference string   `json:"defaultFactoryReference"`
	Allowlist               []string `json:"allowlist"`
}

func acpAgentProfileStorePath(configPath string) string {
	return filepath.Join(filepath.Dir(strings.TrimSpace(configPath)), acpAgentProfileStoreFileName)
}

func (s *Service) ResolveACPAgentProfile(
	request operatorsettings.ResolveACPAgentProfileRequest,
) (operatorsettings.ResolveACPAgentProfileResult, error) {
	if s == nil {
		return operatorsettings.ResolveACPAgentProfileResult{}, fmt.Errorf("operator settings service is required")
	}
	s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationResolve, operatorsettings.ACPAgentProfileLogStageAcceptedIntent, nil, nil)

	authored := request.AuthoredProfile
	source := "authored"
	if authored == nil && strings.TrimSpace(request.Path) != "" {
		loaded, err := s.loadACPAgentProfileDocument(request.Path)
		if err != nil {
			s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationResolve, operatorsettings.ACPAgentProfileLogStageFailure, err, nil)
			return operatorsettings.ResolveACPAgentProfileResult{}, err
		}
		authored = loaded
		source = "persisted"
	}
	if authored == nil {
		result := operatorsettings.ResolveACPAgentProfileResult{Profile: operatorsettings.BuiltInACPAgentProfile()}
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationResolve, operatorsettings.ACPAgentProfileLogStageSuccess, nil, map[string]any{
			"source":          "built_in",
			"allowlist_count": len(result.Profile.Allowlist),
		})
		return result, nil
	}
	profile, err := operatorsettings.NormalizeACPAgentProfile(
		authored.DefaultFactoryReference,
		authored.Allowlist,
	)
	if err != nil {
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationResolve, operatorsettings.ACPAgentProfileLogStageFailure, err, map[string]any{"source": source})
		return operatorsettings.ResolveACPAgentProfileResult{}, err
	}
	s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationResolve, operatorsettings.ACPAgentProfileLogStageSuccess, nil, map[string]any{
		"source":          source,
		"allowlist_count": len(profile.Allowlist),
	})
	return operatorsettings.ResolveACPAgentProfileResult{Profile: profile}, nil
}

// UpdateACPAgentProfile validates a complete candidate ACP agent profile and,
// on success, atomically persists it to the Operator-Settings-owned profile
// store beside Path. The operator config document at Path is neither read nor
// modified: profile storage is isolated from unrelated settings.
func (s *Service) UpdateACPAgentProfile(
	ctx context.Context,
	request operatorsettings.UpdateACPAgentProfileRequest,
) (operatorsettings.UpdateACPAgentProfileResult, error) {
	if s == nil {
		return operatorsettings.UpdateACPAgentProfileResult{}, fmt.Errorf("operator settings service is required")
	}
	s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageAcceptedIntent, nil, nil)

	if ctx == nil {
		err := fmt.Errorf("operator settings context is required")
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageFailure, err, nil)
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	if err := request.Validate(); err != nil {
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageFailure, err, nil)
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	profile, err := operatorsettings.NormalizeACPAgentProfile(request.DefaultFactoryReference, request.Allowlist)
	if err != nil {
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageFailure, err, map[string]any{
			"allowlist_count": len(request.Allowlist),
		})
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	fields := map[string]any{"allowlist_count": len(profile.Allowlist)}
	if err := ctx.Err(); err != nil {
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageFailure, err, fields)
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	if s.files == nil {
		err := fmt.Errorf("operator settings filesystem is required")
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageFailure, err, fields)
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	if s.createTemp == nil {
		err := fmt.Errorf("operator settings temporary-file creator is required")
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageFailure, err, fields)
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}

	data, err := json.Marshal(acpAgentProfileFile{
		DefaultFactoryReference: profile.DefaultFactoryReference,
		Allowlist:               profile.Allowlist,
	})
	if err != nil {
		encodeErr := operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindPersist,
			Message: fmt.Sprintf("encode ACP agent profile: %v", err),
		}
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageFailure, encodeErr, fields)
		return operatorsettings.UpdateACPAgentProfileResult{}, encodeErr
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := ctx.Err(); err != nil {
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageFailure, err, fields)
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	if err := s.persistACPAgentProfileFile(ctx, acpAgentProfileStorePath(request.Path), data); err != nil {
		s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageFailure, err, fields)
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	fields["persisted"] = true
	s.logACPAgentProfileEvent(operatorsettings.ACPAgentProfileOperationUpdate, operatorsettings.ACPAgentProfileLogStageSuccess, nil, fields)
	return operatorsettings.UpdateACPAgentProfileResult{Profile: profile, Persisted: true}, nil
}

// logACPAgentProfileEvent emits one safe ACP agent profile structured log
// record when a logger is injected. err is mapped to a typed failure kind
// without exposing its message, which may otherwise carry candidate Factory
// reference values.
func (s *Service) logACPAgentProfileEvent(operation, stage string, err error, fields map[string]any) {
	if s == nil || s.logACPAgentProfile == nil {
		return
	}
	record := operatorsettings.ACPAgentProfileLogRecord{
		Operation: operation,
		Stage:     stage,
		Fields:    fields,
	}
	if err != nil {
		record.FailureKind = acpAgentProfileLogFailureKind(err)
	}
	s.logACPAgentProfile(record)
}

func acpAgentProfileLogFailureKind(err error) string {
	var failure operatorsettings.ACPAgentProfileFailure
	if errors.As(err, &failure) {
		return string(failure.Kind)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	return "unknown"
}

func (s *Service) loadACPAgentProfileDocument(configPath string) (*operatorsettings.DocumentACPAgentProfile, error) {
	if s.files == nil {
		return nil, fmt.Errorf("operator settings filesystem is required")
	}
	data, err := s.files.ReadFile(acpAgentProfileStorePath(configPath))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindPersist,
			Message: fmt.Sprintf("read ACP agent profile: %v", err),
		}
	}
	var stored acpAgentProfileFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindInvalid,
			Message: fmt.Sprintf("decode ACP agent profile: %v", err),
		}
	}
	return &operatorsettings.DocumentACPAgentProfile{
		DefaultFactoryReference: stored.DefaultFactoryReference,
		Allowlist:               append([]string(nil), stored.Allowlist...),
	}, nil
}

func (s *Service) persistACPAgentProfileFile(ctx context.Context, path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := s.files.MkdirAll(dir, 0o755); err != nil {
		return operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindPersist,
			Message: fmt.Sprintf("create ACP agent profile directory %q: %v", dir, err),
		}
	}
	tmp, err := s.createTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindPersist,
			Message: fmt.Sprintf("create ACP agent profile temp file: %v", err),
		}
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = s.files.Remove(tmpPath)
		}
	}()

	written, err := tmp.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		_ = tmp.Close()
		return operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindPersist,
			Message: fmt.Sprintf("write ACP agent profile temp file: %v", err),
		}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindPersist,
			Message: fmt.Sprintf("sync ACP agent profile temp file: %v", err),
		}
	}
	if err := tmp.Close(); err != nil {
		return operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindPersist,
			Message: fmt.Sprintf("close ACP agent profile temp file: %v", err),
		}
	}
	if err := s.files.Chmod(tmpPath, 0o600); err != nil {
		return operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindPersist,
			Message: fmt.Sprintf("set ACP agent profile temp file permissions: %v", err),
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.files.Rename(tmpPath, path); err != nil {
		return operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindPersist,
			Message: fmt.Sprintf("replace ACP agent profile with temp file: %v", err),
		}
	}
	committed = true
	return nil
}
