package operatorsettings

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrDocumentMalformed reports that a document operation request or stored
// document content is invalid.
var ErrDocumentMalformed = errors.New("operator document is malformed")

// ErrDocumentUnsupported reports that a requested document update is not
// supported by the operator document contract.
var ErrDocumentUnsupported = errors.New("operator document update is unsupported")

// ErrDocumentConflict reports that a document persist was rejected because
// optimistic concurrency or revision facts did not match.
var ErrDocumentConflict = errors.New("operator document persist conflict")

// ErrDocumentNotFound reports that a required operator document is missing.
var ErrDocumentNotFound = errors.New("operator document not found")

// DocumentFailureKind classifies document operation failures peers can branch
// on with errors.Is / errors.As.
type DocumentFailureKind string

const (
	DocumentFailureKindMalformed   DocumentFailureKind = "malformed"
	DocumentFailureKindUnsupported DocumentFailureKind = "unsupported"
	DocumentFailureKindConflict    DocumentFailureKind = "conflict"
	DocumentFailureKindNotFound    DocumentFailureKind = "not_found"
)

// DocumentFailure retains normalized document-operation failure facts without
// exposing storage, codec, or lifecycle construction ports.
type DocumentFailure struct {
	Kind    DocumentFailureKind
	Message string
	Path    string
}

func (failure DocumentFailure) Error() string {
	message := strings.TrimSpace(failure.Message)
	path := strings.TrimSpace(failure.Path)
	switch {
	case message != "" && path != "":
		return fmt.Sprintf("%s: %s (%s)", sentinelForDocumentFailureKind(failure.Kind).Error(), message, path)
	case message != "":
		return fmt.Sprintf("%s: %s", sentinelForDocumentFailureKind(failure.Kind).Error(), message)
	case path != "":
		return fmt.Sprintf("%s (%s)", sentinelForDocumentFailureKind(failure.Kind).Error(), path)
	default:
		return sentinelForDocumentFailureKind(failure.Kind).Error()
	}
}

func (failure DocumentFailure) Unwrap() error {
	return sentinelForDocumentFailureKind(failure.Kind)
}

func sentinelForDocumentFailureKind(kind DocumentFailureKind) error {
	switch kind {
	case DocumentFailureKindMalformed:
		return ErrDocumentMalformed
	case DocumentFailureKindUnsupported:
		return ErrDocumentUnsupported
	case DocumentFailureKindConflict:
		return ErrDocumentConflict
	case DocumentFailureKindNotFound:
		return ErrDocumentNotFound
	default:
		return ErrDocumentMalformed
	}
}

// Document is the detached operator document value peers consume from document
// operations without importing storage or codec construction ports.
type Document struct {
	BackendScopeID string
	Defaults       DocumentDefaults
	Runtime        DocumentRuntimeSettings
	WorkerPresets  []DocumentWorkerPreset
	Workers        DocumentWorkerSettings
}

// Clone returns a detached document copy.
func (document Document) Clone() Document {
	cloned := document
	cloned.Defaults = document.Defaults.Clone()
	cloned.Runtime = document.Runtime.Clone()
	if document.WorkerPresets != nil {
		cloned.WorkerPresets = cloneDocumentWorkerPresets(document.WorkerPresets)
	}
	cloned.Workers = document.Workers.Clone()
	return cloned
}

type DocumentWorkerSettings struct {
	ACP DocumentACPSettings
}

func (settings DocumentWorkerSettings) Clone() DocumentWorkerSettings {
	return DocumentWorkerSettings{ACP: settings.ACP.Clone()}
}

type DocumentACPSettings struct {
	Integrations []ACPIntegration
}

func (settings DocumentACPSettings) Clone() DocumentACPSettings {
	return DocumentACPSettings{Integrations: append([]ACPIntegration(nil), settings.Integrations...)}
}

// DocumentDefaults holds operator default values as a detached peer value.
type DocumentDefaults struct {
	WorkerModelProvider string
	WorkerModel         string
}

// Clone returns a detached defaults copy.
func (defaults DocumentDefaults) Clone() DocumentDefaults {
	return defaults
}

// DocumentWorkerPreset is a reusable file-only worker preset as a detached
// peer value.
type DocumentWorkerPreset struct {
	ID              string
	ModelProvider   string
	Model           string
	ReasoningEffort string
}

// Clone returns a detached worker preset copy.
func (preset DocumentWorkerPreset) Clone() DocumentWorkerPreset {
	return preset
}

// DocumentRuntimeSettings holds operator runtime observability settings as a
// detached peer value.
type DocumentRuntimeSettings struct {
	Logging DocumentRuntimeArtifactSettings
	Metrics DocumentRuntimeArtifactSettings
}

// Clone returns a detached runtime settings copy.
func (settings DocumentRuntimeSettings) Clone() DocumentRuntimeSettings {
	return DocumentRuntimeSettings{
		Logging: settings.Logging.Clone(),
		Metrics: settings.Metrics.Clone(),
	}
}

// DocumentRuntimeArtifactSettings controls one rolling runtime observability
// artifact as a detached peer value.
type DocumentRuntimeArtifactSettings struct {
	Directory  string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

// Clone returns a detached runtime artifact settings copy.
func (settings DocumentRuntimeArtifactSettings) Clone() DocumentRuntimeArtifactSettings {
	return settings
}

// EmptyDocument returns a valid empty operator document with production-default
// runtime artifact settings.
func EmptyDocument() Document {
	return Document{Runtime: defaultDocumentRuntimeSettings()}
}

func defaultDocumentRuntimeSettings() DocumentRuntimeSettings {
	defaults := defaultRuntimeArtifactSettings()
	return DocumentRuntimeSettings{
		Logging: DocumentRuntimeArtifactSettings(defaults),
		Metrics: DocumentRuntimeArtifactSettings(defaults),
	}
}

func cloneDocumentWorkerPresets(presets []DocumentWorkerPreset) []DocumentWorkerPreset {
	if presets == nil {
		return nil
	}
	cloned := make([]DocumentWorkerPreset, len(presets))
	copy(cloned, presets)
	return cloned
}

// LoadDocumentRequest asks for the operator document at Path. When
// RequireExisting is true, a missing document fails with ErrDocumentNotFound
// instead of returning an empty document.
type LoadDocumentRequest struct {
	Path            string
	RequireExisting bool
}

// Validate checks request fields whose validity does not depend on storage state.
func (request LoadDocumentRequest) Validate() error {
	if strings.TrimSpace(request.Path) == "" {
		return fmt.Errorf("%w: path is required", ErrDocumentMalformed)
	}
	return nil
}

// LoadDocumentResult is the detached outcome of one document load.
type LoadDocumentResult struct {
	Document Document
	Path     string
	Found    bool
}

// DocumentProviderModelUpdate distinguishes omitted defaults from explicitly
// supplied values. A nil field preserves the current value; a non-nil field
// replaces it after trimming, including clearing it when the supplied value is
// empty.
type DocumentProviderModelUpdate struct {
	Provider *string
	Model    *string
}

// ApplyDocumentUpdateRequest applies a semantic document change and persists
// the resulting document.
type ApplyDocumentUpdateRequest struct {
	Path                 string
	ExpectedBackendScope string
	ProviderModel        DocumentProviderModelUpdate
}

// Validate checks request fields whose validity does not depend on storage state.
func (request ApplyDocumentUpdateRequest) Validate() error {
	if strings.TrimSpace(request.Path) == "" {
		return fmt.Errorf("%w: path is required", ErrDocumentMalformed)
	}
	if request.ProviderModel.Provider == nil && request.ProviderModel.Model == nil {
		return fmt.Errorf("%w: at least one provider/model field is required", ErrDocumentMalformed)
	}
	return nil
}

// ApplyDocumentUpdateResult reports the post-update document and persist outcome.
type ApplyDocumentUpdateResult struct {
	Document  Document
	Path      string
	Persisted bool
}

// PersistDocumentRequest atomically publishes one complete validated operator
// document at Path.
type PersistDocumentRequest struct {
	Path     string
	Document Document
}

// DocumentOwner is the parent-private document capability consumed by
// ConfigDocumentService. Wire injects the nested document owner implementation.
type DocumentOwner interface {
	LoadDocument(LoadDocumentRequest) (LoadDocumentResult, error)
	MergeDocumentProviderModel(Document, DocumentProviderModelUpdate) (Document, error)
	ApplyDocumentUpdate(ApplyDocumentUpdateRequest) (ApplyDocumentUpdateResult, error)
	PersistDocument(context.Context, PersistDocumentRequest) error
}

// Validate checks request fields whose validity does not depend on storage state.
func (request PersistDocumentRequest) Validate() error {
	if strings.TrimSpace(request.Path) == "" {
		return fmt.Errorf("%w: path is required", ErrDocumentMalformed)
	}
	return nil
}
