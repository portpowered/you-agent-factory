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
	authored := request.AuthoredProfile
	if authored == nil && strings.TrimSpace(request.Path) != "" {
		loaded, err := s.loadACPAgentProfileDocument(request.Path)
		if err != nil {
			return operatorsettings.ResolveACPAgentProfileResult{}, err
		}
		authored = loaded
	}
	if authored == nil {
		return operatorsettings.ResolveACPAgentProfileResult{Profile: operatorsettings.BuiltInACPAgentProfile()}, nil
	}
	profile, err := operatorsettings.NormalizeACPAgentProfile(
		authored.DefaultFactoryReference,
		authored.Allowlist,
	)
	if err != nil {
		return operatorsettings.ResolveACPAgentProfileResult{}, err
	}
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
	if ctx == nil {
		return operatorsettings.UpdateACPAgentProfileResult{}, fmt.Errorf("operator settings context is required")
	}
	if err := request.Validate(); err != nil {
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	profile, err := operatorsettings.NormalizeACPAgentProfile(request.DefaultFactoryReference, request.Allowlist)
	if err != nil {
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	if s.files == nil {
		return operatorsettings.UpdateACPAgentProfileResult{}, fmt.Errorf("operator settings filesystem is required")
	}
	if s.createTemp == nil {
		return operatorsettings.UpdateACPAgentProfileResult{}, fmt.Errorf("operator settings temporary-file creator is required")
	}

	data, err := json.Marshal(acpAgentProfileFile{
		DefaultFactoryReference: profile.DefaultFactoryReference,
		Allowlist:               profile.Allowlist,
	})
	if err != nil {
		return operatorsettings.UpdateACPAgentProfileResult{}, operatorsettings.ACPAgentProfileFailure{
			Kind:    operatorsettings.ACPAgentProfileFailureKindPersist,
			Message: fmt.Sprintf("encode ACP agent profile: %v", err),
		}
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := ctx.Err(); err != nil {
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	if err := s.persistACPAgentProfileFile(ctx, acpAgentProfileStorePath(request.Path), data); err != nil {
		return operatorsettings.UpdateACPAgentProfileResult{}, err
	}
	return operatorsettings.UpdateACPAgentProfileResult{Profile: profile, Persisted: true}, nil
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
